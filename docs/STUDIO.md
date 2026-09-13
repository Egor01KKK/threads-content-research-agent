# Live search studio

A local web app that watches what Russian-speaking Threads users are posting
right now and keeps only the posts that answer a question you type in your own
words.

It uses no Threads login, no API key, and no paid model. Collection is the same
anonymous public surface the rest of `th` reads. Judgement is delegated to a
coding agent you already run in your terminal, under your own subscription.

The studio is Russian-only by design: its interface is in Russian, it requests
the `ru-RU` locale, and it discards Ukrainian-language posts. If you do not work
in Russian, this tool will not be useful to you.

## Requirements

- Everything in the main [Requirements](../README.md#requirements) section.
- Python 3.10 or newer.
- One terminal agent, either [Codex CLI](https://github.com/openai/codex) or
  [Claude Code](https://claude.com/claude-code), installed and signed in.

## Connecting the agent

The studio shells out to whichever agent it finds, Codex first, Claude second.
Nothing is configured in the app itself; if the command works in your terminal,
the studio will use it.

For Codex:

```sh
npm install -g @openai/codex
codex login
```

For Claude Code:

```sh
npm install -g @anthropic-ai/claude-code
claude login
```

To confirm the studio can see it:

```sh
codex exec --skip-git-repo-check <<< 'reply with one word: ok'
```

If neither agent is available the app refuses to start a run and says so on
screen rather than failing silently.

## Running

```sh
make studio
```

That builds `./bin/th`, starts a server on `127.0.0.1:4200`, and opens a
browser. The port is not exposed beyond loopback. If another program already
holds port 4200, choose a different one:

```sh
STUDIO_PORT=4300 make studio
```

Type what you are looking for in ordinary Russian, for example
`кто ищет того, кто сделает сайт`, and press **Найти**. Results appear while
collection is still running. Press **Стоп** when you have seen enough; posts
already collected are finished before the run closes. A run also stops on its
own after thirty minutes.

[docs/ТЕСТЫ-ЗАПРОСОВ.md](%D0%A2%D0%95%D0%A1%D0%A2%D0%AB-%D0%97%D0%90%D0%9F%D0%A0%D0%9E%D0%A1%D0%9E%D0%92.md)
is a blank worksheet of fourteen queries for comparing result quality across
question types.

## How a run works

1. Your question is split into separate Russian words, and a set of words that
   the public search reliably answers is added to them.
2. Each word is sent to `th search --lang ru-RU --no-cache`. One call returns
   roughly fifty posts, most of them seconds or minutes old.
3. Four calls run in parallel on a loop until you stop. Duplicates are dropped
   by post id.
4. Surviving posts go to the agent in batches of forty, three batches at a
   time. It replies with which posts match and why.

Posts a word from your question appears in are sent for judgement first, so the
most promising ones surface early.

## What gets discarded before the agent sees it

- Anything older than 24 hours. A three-week-old request is already answered.
- Ukrainian-language posts, detected by letters absent from Russian
  (`і ї є ґ`) weighed against letters absent from Ukrainian (`ы ъ э`).
- Posts under 25 characters, posts with no Cyrillic, and bare hashtag strings.

## Limits worth knowing

The anonymous search surface is not a keyword search. It returns a live window
of recent posts with only a weak tilt toward the word you sent. In one measured
sample, seven of fifty posts returned for `ремонт` actually contained the word,
and two unrelated words shared thirty-six of their fifty results. Multi-word
phrases return nothing at all. This is why the studio sends many single words
and lets the agent do the real filtering, and why a run reads several hundred
posts to surface a handful.

`th` also reads `THREADS_SESSION` and `THREADS_CSRF`, which switch search to the
persisted GraphQL document used by authenticated clients. That path is not
exercised or supported here, and driving a personal account through automated
requests risks that account. See
[Responsible use](../README.md#responsible-use).

Threads can change or withdraw the public surface at any time, so treat every
run as a bounded sample.

## When something goes wrong

**`порт 4200 уже занят`** — either the studio is already running in another
window, in which case open `http://127.0.0.1:4200` or stop it with
`pkill -f scripts/studio.py`, or another program holds the port, in which case
start on a different one with `STUDIO_PORT=4300 make studio`.

**`Threads не отвечает`** — the machine cannot reach `www.threads.com`. Check
that the site opens in a browser. Where Meta services are blocked, the studio
needs the same network access a browser would.

**`Агент в терминале недоступен`** — no signed-in Codex or Claude was found.
Follow [Connecting the agent](#connecting-the-agent) above.

**A run finds nothing** — normal for a narrow question. Live requests on any
one topic are rare, and repeating the same query soon after tends to surface
the same people. Try a broader question, or leave the run going longer.
