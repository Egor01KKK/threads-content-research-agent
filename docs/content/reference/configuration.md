---
title: "Configuration"
description: "Environment variables and global flags."
weight: 20
---

`th` needs almost no configuration: every read works anonymously out of the box.
What you can tune is how it paces requests, where it caches, and the optional
session credentials that add depth for your own account.

## Global flags

| Flag | Default | Meaning |
|---|---|---|
| `-o, --output` | `auto` | Output format (`table`, `json`, `jsonl`, `csv`, `tsv`, `yaml`, `url`, `raw`) |
| `--fields` | | Comma-separated columns to keep and order |
| `-n, --limit` | `0` | Max records emitted (`0` is unlimited) |
| `--delay` | `1s` | Minimum delay between requests |
| `--retries` | `4` | Retry attempts on 429/5xx |
| `--timeout` | `30s` | Per-request timeout |
| `--no-cache` | `false` | Bypass the on-disk cache |
| `--cache-ttl` | `1h` | Cache freshness window |
| `--proxy` | | HTTP/SOCKS proxy URL |
| `-v, --verbose` | | Increase verbosity (repeatable) |

## Environment variables

All optional. With no session values the client stays anonymous; set session
values only when you deliberately want additional depth on your own account.

| Variable | Meaning |
|---|---|
| `THREADS_SESSION` | Logged-in session id cookie |
| `THREADS_CSRF` | Session CSRF token |
| `XDG_CACHE_HOME` | Move the cache out of `~/.cache/th` |
| `XDG_DATA_HOME` | Move data out of `~/.local/share/th` |

`THREADS_SESSION` and `THREADS_CSRF` have matching flags (`--session` and
`--csrf`). Run `th config show`
to see the resolved values and `th whoami` to see which access mode is active.
