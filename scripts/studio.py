#!/usr/bin/env python3
"""th studio — живой поиск по Threads.

Запуск:
    python3 scripts/studio.py     ->  http://127.0.0.1:4200

Ничего заранее собранного не используется. Каждый прогон идёт в Threads
за постами, которым несколько секунд от роду, и разбирает их агентом
из твоего терминала по твоей подписке. Ключи и внешние модели не нужны.
"""

from __future__ import annotations

import errno
import json
import os
import queue
import re
import shutil
import subprocess
import sys
import threading
import time
import webbrowser
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
TH = os.environ.get("TH", str(ROOT / "bin" / "th"))
STUDIO_HTML = ROOT / "viewer" / "studio.html"

HOST = "127.0.0.1"
PORT = int(os.environ.get("STUDIO_PORT", "4200"))
BATCH = 40          # постов в одном вызове агента
WORKERS = 3         # параллельных вызовов агента
SCANNERS = 4        # параллельных запросов в Threads
PER_SCAN = 60       # постов за один запрос
MILESTONE = 500     # через столько просмотренных постов отмечаемся в консоли
FRESH_HOURS = 24    # старше суток не берём: смысл в том, чтобы успеть первым
MAX_RUN = 1800      # предохранитель, если забыл нажать «Стоп»

STATE_LOCK = threading.Lock()
STATE: dict = {}
STOP = threading.Event()


def reset_state() -> None:
    with STATE_LOCK:
        STATE.clear()
        STATE.update(
            stage="idle", query="", agent=None, error=None, words=[],
            collecting=False, stopping=False,
            batches_done=0, batches_total=0,
            fresh=0, scans=0, analyzed=0, skipped_ukr=0, elapsed=0,
            log=[], results=[],
        )


def log(line: str, kind: str = "") -> None:
    with STATE_LOCK:
        STATE["log"].append({"text": line, "kind": kind})


def plural(n: int, one: str, few: str, many: str) -> str:
    """1 пост, 2 поста, 5 постов."""
    tail, last = abs(n) % 100, abs(n) % 10
    if 10 < tail < 20:
        return many
    if 1 < last < 5:
        return few
    return one if last == 1 else many


def patch(**kw) -> None:
    with STATE_LOCK:
        STATE.update(kw)


def bump(key: str, by: int = 1) -> None:
    with STATE_LOCK:
        STATE[key] = STATE.get(key, 0) + by


# ------------------------------------------------------------------ агент


def detect_agent():
    """Ищем агента по полному пути, а не по имени.

    На Windows npm ставит агентов как codex.cmd и claude.cmd. which их
    находит, а запуск по голому имени нет. Полный путь работает везде.
    """
    codex = shutil.which("codex")
    if codex:
        try:
            probe = subprocess.run([codex, "exec", "--help"],
                                   capture_output=True, text=True, timeout=40)
            if probe.returncode == 0:
                return "codex", [codex, "exec", "--skip-git-repo-check"]
        except (subprocess.SubprocessError, OSError):
            pass
    claude = shutil.which("claude")
    if claude:
        try:
            probe = subprocess.run([claude, "-p", "ответь одним словом: ок"],
                                   capture_output=True, text=True, timeout=90)
            if probe.returncode == 0 and "Failed to authenticate" not in probe.stdout:
                return "claude", [claude, "-p"]
        except (subprocess.SubprocessError, OSError):
            pass
    return None


WORDS_PROMPT = """Поиск Threads принимает одно слово за раз и не понимает фразы.

Запрос пользователя: {query}

Верни JSON-массив из 12 отдельных русских слов, которые реально встречаются
в тексте постов по этому запросу. Разные формы одного слова считаются разными
словами и это полезно. Без фраз, без хештегов, без латиницы.

Пример для «кто ищет подрядчика на сайт»:
["подрядчик","подрядчика","разработчик","сайт","лендинг","верстальщик","фрилансер","посоветуйте","ищу","нужен","заказать","бюджет"]

Верни только массив."""


PROMPT = """Ты фильтруешь посты из Threads под запрос пользователя.

ЗАПРОС: {query}

Ниже пронумерованные посты. Верни ТОЛЬКО JSON-массив, без markdown и пояснений.
Каждый элемент: {{"i": номер, "why": "одно предложение по-русски, почему подходит"}}

Включай пост, только если он действительно отвечает запросу по смыслу.
Реклама, мотивационные цитаты и общие рассуждения не подходят.
Если не подходит ничего, верни [].

ПОСТЫ:
{posts}"""


def run_agent(cmd, prompt: str, timeout: int = 300) -> str:
    proc = subprocess.run(cmd, input=prompt, capture_output=True,
                          text=True, timeout=timeout, cwd=str(ROOT))
    return proc.stdout


def parse_json_array(text: str):
    """Достаёт JSON-массив из вывода агента.

    Codex печатает ответ, затем «tokens used», затем ответ ещё раз.
    Жадная регулярка склеила бы это в кашу, поэтому идём по скобкам
    и берём самый содержательный валидный массив.
    """
    best = []
    for start in (m.start() for m in re.finditer(r"\[", text)):
        depth = 0
        for pos in range(start, len(text)):
            if text[pos] == "[":
                depth += 1
            elif text[pos] == "]":
                depth -= 1
                if depth == 0:
                    try:
                        data = json.loads(text[start:pos + 1])
                    except json.JSONDecodeError:
                        break
                    if isinstance(data, list) and len(data) > len(best):
                        best = data
                    break
    return best


CYRILLIC = re.compile(r"[а-яё]", re.I)
RU_WORD = re.compile(r"[а-яё]{3,20}", re.I)

# На часть слов поиск Threads молчит всегда: «подрядчик», «нужен», «нужна».
# Эти проверены и отдают по полсотни свежих постов, почти все по-русски.
RESCUE = ["работа", "ищу", "бизнес", "клиенты", "услуги", "помогите",
          "посоветуйте", "заказ", "сайт", "деньги"]


def split_query(query: str):
    """Запасные слова, если агент не ответил: берём сам запрос."""
    words = [w.lower() for w in re.findall(r"[А-Яа-яЁё]{4,}", query)]
    return list(dict.fromkeys(words))[:12]


def pick_words(cmd, query: str):
    try:
        raw = run_agent(cmd, WORDS_PROMPT.format(query=query), timeout=120)
    except (subprocess.SubprocessError, OSError):
        raw = ""
    words = [w.strip().lower() for w in parse_json_array(raw) if isinstance(w, str)]
    words = [w for w in words if RU_WORD.fullmatch(w)]
    words = list(dict.fromkeys(words))[:14]
    return words or split_query(query)


# ------------------------------------------------------------------ сбор


def run_th(args, timeout: int = 60):
    """Всегда --no-cache: у th кеш на час, иначе «живой» сбор врёт."""
    try:
        proc = subprocess.run([TH, *args, "--no-cache", "--output", "jsonl", "-q"],
                              capture_output=True, text=True, timeout=timeout)
    except (subprocess.SubprocessError, OSError):
        return []
    out = []
    for line in proc.stdout.splitlines():
        line = line.strip()
        if line.startswith("{"):
            try:
                out.append(json.loads(line))
            except json.JSONDecodeError:
                pass
    return out


UKRAINIAN = re.compile(r"[іїєґ]", re.I)
RUSSIAN_ONLY = re.compile(r"[ыъэ]", re.I)


def is_ukrainian(text: str) -> bool:
    """Украинский от русского отличаем по буквам, которых нет в другом языке.

    У украинского это і, ї, є, ґ; у русского ы, ъ, э. Смешанный текст
    относим туда, чьих букв больше.

    Проверено на 169 живых постах: правило нашло 15 украинских и ни разу
    не приняло русский за украинский. Постов, украинских по словам, но без
    і/ї/є/ґ, в выборке не оказалось вовсе, поэтому словаря не завожу.
    """
    ukr = len(UKRAINIAN.findall(text))
    return ukr > 0 and ukr >= len(RUSSIAN_ONLY.findall(text))


def fresh_enough(post, cutoff: str) -> bool:
    """Поиск подмешивает старьё: в выдаче примерно четверть постов старше суток.

    Замерено на 188 постах: меньше часа 42%, до суток 72%, остальное вплоть
    до трёхнедельной давности. Заявка трёхнедельной давности уже закрыта.
    """
    return (post.get("timestamp") or "") >= cutoff


def worth_asking(post) -> bool:
    """Отсев до агента: пустое, нерусское и голые хештеги не тратят вызов."""
    text = (post.get("text") or "").strip()
    if len(text) < 25 or not CYRILLIC.search(text):
        return False
    words = [w for w in re.findall(r"\S+", text) if not w.startswith("#")]
    return len(words) >= 4


class Collector(threading.Thread):
    """Крутит живой поиск по словам, пока не выйдет время.

    Один вызов th search отдаёт до шестидесяти постов возрастом в секунды.
    Это поток свежего русского Threads со слабым уклоном в слово: примерно
    каждый шестой пост содержит его прямо, остальные просто свежие. Поэтому
    слов держим много, а разбор идёт одновременно со сбором.

    Слово, дважды подряд вернувшее пусто, выбрасываем: поиск на него молчит
    и будет молчать. Когда живых слов не остаётся, берём проверенные.
    """

    def __init__(self, words, cutoff: str, sink):
        super().__init__(daemon=True)
        self.words = list(words)
        self.cutoff = cutoff
        self.sink = sink
        self.seen: set = set()
        self.raw = 0
        self.empty: dict = {}
        self.mark = MILESTONE
        self.order = 0
        self.lock = threading.Lock()

    def add_words(self, words) -> None:
        with self.lock:
            for word in words:
                if word not in self.words and word not in self.empty:
                    self.words.append(word)

    def take(self, records):
        fresh = 0
        with self.lock:
            self.raw += len(records)
        for rec in records:
            key = rec.get("id") or rec.get("permalink")
            if not key or not fresh_enough(rec, self.cutoff):
                continue
            with self.lock:
                if key in self.seen:
                    continue
                self.seen.add(key)
                self.order += 1
                order = self.order
            fresh += 1
            if is_ukrainian(rec.get("text") or ""):
                bump("skipped_ukr")
                continue
            if worth_asking(rec):
                # Пост со словом из запроса идёт к агенту первым.
                text = (rec.get("text") or "").lower()
                near = any(w[:5] in text for w in self.words)
                self.sink.put((0 if near else 1, order, rec))
        bump("fresh", fresh)
        with self.lock:
            if len(self.seen) >= self.mark:
                while len(self.seen) >= self.mark:
                    self.mark += MILESTONE
                reached = self.mark - MILESTONE
            else:
                reached = 0
        if reached:
            log("просмотрел %d постов" % reached)
        return fresh

    def scan(self, word: str):
        got = self.take(run_th(["search", word, "-n", str(PER_SCAN), "--lang", "ru-RU"]))
        bump("scans")
        with self.lock:
            misses = self.empty.get(word, 0)
            self.empty[word] = 0 if got else misses + 1
            if self.empty[word] >= 2 and word in self.words:
                self.words.remove(word)   # поиск на него молчит и будет молчать
        return word, got

    def run(self):
        deadline = time.time() + MAX_RUN
        patch(collecting=True)

        with ThreadPoolExecutor(max_workers=SCANNERS) as pool:
            while not STOP.is_set() and time.time() < deadline:
                with self.lock:
                    alive = list(self.words)
                if not alive:
                    self.add_words(RESCUE)   # слова запроса не ищутся, берём общие
                    continue
                for _word, _got in pool.map(self.scan, alive):
                    if STOP.is_set():
                        break
                # Полный круг по десятку слов даёт сотни постов. Ноль значит,
                # что до Threads не достучаться, а не что тема пустая.
                if not self.raw:
                    patch(error="Threads не отвечает. Проверь, открывается ли "
                                "www.threads.com в браузере: без доступа к нему "
                                "студия ничего не соберёт.")
                    STOP.set()

        patch(collecting=False)

        self.sink.put((9, 1 << 40, None))


# ------------------------------------------------------------------ прогон


def pipeline(query: str) -> None:
    started = time.time()
    try:
        STOP.clear()
        reset_state()
        patch(stage="running", query=query)

        agent = detect_agent()
        if not agent:
            patch(stage="error", error=(
                "Агент в терминале недоступен. Codex: npm i -g @openai/codex, "
                "затем codex login. Или Claude: claude login."))
            return
        name, cmd = agent
        patch(agent=name)

        # Сбор стартует сразу на словах из самого запроса: ждать агента
        # полминуты, пока Threads уже отдаёт свежее, незачем. Его слова
        # доедут следом и встанут в ту же очередь.
        #
        # Проверенные слова идут с первой секунды вместе со словами запроса.
        # Иначе бывает так: на «кому нужна автоматизация или бот» остаётся
        # одно живое слово из трёх, и поток пересыхает до прихода агента.
        # На порядок выдачи это не влияет: пост со словом запроса всё равно
        # уходит к агенту первым.
        own = split_query(query)
        words = own + [w for w in RESCUE if w not in own]
        patch(words=words)
        log("смотрю, что пишут прямо сейчас")

        cutoff = (datetime.now(timezone.utc)
                  - timedelta(hours=FRESH_HOURS)).strftime("%Y-%m-%dT%H:%M:%SZ")
        sink: queue.PriorityQueue = queue.PriorityQueue()
        collector = Collector(words, cutoff, sink)
        collector.start()

        def widen():
            extra = [w for w in pick_words(cmd, query) if w not in words]
            if extra:
                collector.add_words(extra)
                patch(words=words + extra)

        threading.Thread(target=widen, daemon=True).start()

        results = []
        lock = threading.Lock()

        def publish(found):
            with lock:
                results.extend(found)
                patch(results=sorted(results, key=lambda r: r["timestamp"] or "",
                                     reverse=True))

        def analyse(batch):
            listing = "\n".join(
                "%d. @%s | %s" % (i, p.get("username"), (p.get("text") or "")[:400])
                for i, p in enumerate(batch))
            try:
                raw = run_agent(cmd, PROMPT.format(query=query, posts=listing))
            except (subprocess.SubprocessError, OSError):
                bump("batches_done")
                return
            found = []
            for hit in parse_json_array(raw):
                if not isinstance(hit, dict):
                    continue
                index = hit.get("i")
                if not isinstance(index, int) or not 0 <= index < len(batch):
                    continue
                post = batch[index]
                found.append({
                    "username": post.get("username"),
                    "text": post.get("text"),
                    "permalink": post.get("permalink"),
                    "timestamp": post.get("timestamp"),
                    "likes": post.get("like_count"),
                    "replies": post.get("reply_count"),
                    "why": hit.get("why", ""),
                })
            publish(found)
            bump("batches_done")
            bump("analyzed", len(batch))
            for hit in found:
                line = " ".join((hit["text"] or "").split())
                log("нашёл @%s · %s" % (
                    hit["username"],
                    line[:62] + "…" if len(line) > 62 else line), "ok")

        pool = ThreadPoolExecutor(max_workers=WORKERS)
        pending, batch, closed = [], [], False
        while not closed:
            patch(elapsed=int(time.time() - started))
            try:
                item = sink.get(timeout=2)
            except queue.Empty:
                item = False           # пауза в сборе, но поток ещё жив
            if item is not False and item[2] is None:
                item = None
            if item is None:
                closed = True
            elif item is not False:
                batch.append(item[2])
            if len(batch) >= BATCH or (closed and batch):
                bump("batches_total")
                pending.append(pool.submit(analyse, batch))
                batch = []

        for task in pending:
            task.result()
        pool.shutdown()

        elapsed = int(time.time() - started)
        patch(stage="error" if STATE.get("error") else "done",
              elapsed=elapsed, stopping=False)
        log("%s: %d %s за %d сек" % (
            "остановил" if STOP.is_set() else "готово",
            len(results), plural(len(results), "находка", "находки", "находок"),
            elapsed), "ok")

    except Exception as exc:  # noqa: BLE001 — причину показываем на экране
        patch(stage="error", error="%s: %s" % (type(exc).__name__, exc))


# ------------------------------------------------------------------ сервер


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def _send(self, code, body, ctype):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        path = self.path.split("?")[0]
        if path in ("/", "/index.html", "/studio.html"):
            self._send(200, STUDIO_HTML.read_bytes(), "text/html; charset=utf-8")
        elif path == "/api/state":
            with STATE_LOCK:
                body = json.dumps(STATE, ensure_ascii=False).encode()
            self._send(200, body, "application/json; charset=utf-8")
        else:
            self._send(404, b"not found", "text/plain; charset=utf-8")

    def do_POST(self):
        if self.path.startswith("/api/stop"):
            STOP.set()
            patch(stopping=True)
            log("останавливаю: доразбираю то, что уже поймал", "warn")
            self._send(200, b'{"ok":true}', "application/json")
            return
        if not self.path.startswith("/api/run"):
            self._send(404, b"not found", "text/plain; charset=utf-8")
            return
        length = int(self.headers.get("Content-Length") or 0)
        payload = json.loads(self.rfile.read(length) or b"{}")
        query = (payload.get("query") or "").strip()
        if not query:
            self._send(400, b'{"error":"empty query"}', "application/json")
            return
        if STATE.get("stage") == "running":
            self._send(409, b'{"error":"already running"}', "application/json")
            return
        threading.Thread(target=pipeline, daemon=True, args=(query,)).start()
        self._send(200, b'{"ok":true}', "application/json")


def main():
    if not Path(TH).exists():
        sys.exit("нет бинарника %s — собери через make build" % TH)
    reset_state()
    url = "http://%s:%d" % (HOST, PORT)
    try:
        server = ThreadingHTTPServer((HOST, PORT), Handler)
    except OSError as exc:
        if exc.errno != errno.EADDRINUSE:
            raise
        sys.exit("порт %d уже занят.\n"
                 "Если это студия в другом окне, открой %s\n"
                 "или погаси её: pkill -f scripts/studio.py\n"
                 "Если порт занят другой программой: STUDIO_PORT=4300 make studio"
                 % (PORT, url))
    print("th studio: %s\nCtrl-C чтобы остановить" % url)
    try:
        webbrowser.open(url)
    except Exception:  # noqa: BLE001
        pass
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nостановлено")


if __name__ == "__main__":
    main()
