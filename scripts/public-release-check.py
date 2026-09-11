#!/usr/bin/env python3
"""Fail closed on accidentally tracked private release data or real secrets.

This is intentionally a small, high-signal guard for the public repository. It
does not try to be a general secret scanner: runtime API-key plumbing and short
synthetic provider fixtures are valid source/test material and are covered by
the self-test below.
"""

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path, PurePosixPath


FORBIDDEN_PATH_COMPONENTS = frozenset(
    {
        "validation-output",
        "video-prep",
        "analysis-independent",
        "research-output",
    }
)
FORBIDDEN_FILENAMES = frozenset({"complete-readable.json"})

# Keep local absolute-path checks exact. The guard's own source constructs the
# values below from pieces so it does not report its own policy definitions.
FORBIDDEN_TEXT = (
    "/" + "Users" + "/" + "egorandrienko",
    "База" + " " + "Егорчика",
)

ALLOWLISTED_TEST_VALUES = frozenset(
    {
        "test-openai-key",
        "test-anthropic-key",
        "test-key",
        "env-secret",
    }
)

SECRET_PATTERNS: tuple[tuple[str, re.Pattern[str]], ...] = (
    ("private key", re.compile(r"-----BEGIN [A-Z0-9 ]+ PRIVATE KEY-----")),
    ("AWS access key", re.compile(r"\b(?:AKIA|ASIA)[0-9A-Z]{16}\b")),
    (
        "GitHub token",
        re.compile(r"\b(?:gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,})\b"),
    ),
    ("GitLab token", re.compile(r"\bglpat-[A-Za-z0-9_-]{20,}\b")),
    ("Slack token", re.compile(r"\bxox[baprs]-[A-Za-z0-9-]{20,}\b")),
    (
        "OpenAI/Anthropic-style key",
        re.compile(r"\bsk-(?:proj-|ant-[A-Za-z0-9]+-)?[A-Za-z0-9_-]{20,}\b"),
    ),
    ("Google API key", re.compile(r"\bAIza[0-9A-Za-z_-]{30,}\b")),
    (
        "Bearer token",
        re.compile(r"(?i)\bBearer[ \t]+([A-Za-z0-9._~+/=-]{16,})\b"),
    ),
    (
        "session or CSRF value",
        re.compile(
            r"(?i)\b(?:THREADS_SESSION|THREADS_CSRF)\b[ \t]*[=:,][ \t]*[\"']?"
            r"[A-Za-z0-9._~+/=-]{24,}"
        ),
    ),
    (
        "provider API key value",
        re.compile(
            r"(?i)\b(?:OPENAI|ANTHROPIC)_API_KEY\b[ \t]*[=:,][ \t]*[\"']?"
            r"[A-Za-z0-9._~+/=-]{24,}"
        ),
    ),
)


def tracked_files() -> list[str]:
    result = subprocess.run(
        ["git", "ls-files", "-z"],
        check=True,
        stdout=subprocess.PIPE,
    )
    return [name for name in result.stdout.decode("utf-8").split("\0") if name]


def path_findings(name: str) -> list[str]:
    path = PurePosixPath(name)
    lowered_parts = {part.casefold() for part in path.parts}
    findings: list[str] = []
    if lowered_parts & FORBIDDEN_PATH_COMPONENTS:
        findings.append("forbidden private-output directory")
    if path.name.casefold() in FORBIDDEN_FILENAMES:
        findings.append("forbidden private dataset filename")
    if path.suffix.casefold() in {".db", ".sqlite", ".sqlite3", ".jsonl", ".log"}:
        findings.append("local dataset or log artifact")
    return findings


def content_findings(text: str) -> list[str]:
    findings: list[str] = []
    for marker in FORBIDDEN_TEXT:
        if marker in text:
            findings.append("local absolute path")
            break

    for label, pattern in SECRET_PATTERNS:
        for match in pattern.finditer(text):
            matched = match.group(0)
            if any(value in matched for value in ALLOWLISTED_TEST_VALUES):
                continue
            findings.append(label)
            break
    return findings


def scan() -> list[tuple[str, str]]:
    findings: list[tuple[str, str]] = []
    for name in tracked_files():
        for finding in path_findings(name):
            findings.append((name, finding))
        # A file removed in the working tree is not part of the next commit.
        # This keeps local checks useful before the deletion is staged.
        if not Path(name).exists():
            continue
        try:
            text = Path(name).read_bytes().decode("utf-8", errors="replace")
        except OSError as exc:
            findings.append((name, f"could not read tracked file: {exc.__class__.__name__}"))
            continue
        for finding in content_findings(text):
            findings.append((name, finding))
    return findings


def self_test() -> None:
    safe_samples = (
        'req.Header.Set("Authorization", "Bearer "+p.config.APIKey)',
        'Authorization: Bearer + runtime APIKey',
        'Authorization: Bearer test-openai-key',
        'APIKey: "test-anthropic-key"',
    )
    for sample in safe_samples:
        findings = content_findings(sample)
        if findings:
            raise AssertionError(f"safe fixture was flagged: {findings}")


def main() -> int:
    if "--self-test" in sys.argv[1:]:
        self_test()
        print("public release guard self-test: PASS")
        if len(sys.argv) == 2:
            return 0

    findings = scan()
    if findings:
        print("public release guard: FAIL")
        for name, finding in findings:
            # Never print matching secret material, only the file and category.
            print(f"- {name}: {finding}")
        return 1

    print(f"public release guard: PASS ({len(tracked_files())} tracked files scanned)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
