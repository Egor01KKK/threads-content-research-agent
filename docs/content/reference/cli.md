---
title: "CLI"
description: "Every command and subcommand, with the flags that matter."
weight: 10
---

```
th <command> [subcommand] [flags]
```

Run `th <command> --help` for the full flag list on any command.

## Commands

| Command | What it does |
|---|---|
| `profile <@handle\|id\|url>` | A profile's full record |
| `profile <h> --posts` | Walk the profile's recent posts |
| `profile <h> --replies` | Walk the profile's replies feed |
| `post <url\|shortcode\|id>` | A single post in full |
| `post <url> --replies` | The post's reply thread |
| `post <url> --raw` | The upstream HTML, untouched |
| `replies <url>` | Stream replies to a post as their own records |
| `feed <@handle>` | Walk a profile's recent posts (alias for `profile --posts`) |
| `search <query>` | Keyword search across public posts |
| `id <input>` | Classify any handle, id, shortcode, or URL (offline) |
| `db build <@handle> --db F` | Crawl a profile's posts into SQLite |
| `db query --db F "<sql>"` | Query the local dataset |
| `research <topic> --mode deep` | Build a bounded anonymous Russian small-business pain corpus |
| `semantic-audit <complete-readable.json>` | Locally reclassify an existing deep corpus into auditable business-evidence views |
| `whoami` | Report how `th` is accessing Threads |
| `config show\|path` | Show resolved configuration and paths |
| `cache dir\|clear` | Inspect or clear the on-disk cache |
| `completion bash\|zsh\|fish` | Generate a shell completion script |
| `version` | Print version, commit, and build date |

## Global flags

These apply to every command.

| Flag | Default | Meaning |
|---|---|---|
| `-o, --output` | `auto` | `table`, `json`, `jsonl`, `csv`, `tsv`, `yaml`, `url`, `raw` |
| `--fields` | | Comma-separated columns to keep and order |
| `--no-header` | `false` | Omit the header row (table/csv/tsv) |
| `--template` | | Go `text/template` applied per record |
| `-n, --limit` | `0` | Max records emitted (`0` is unlimited) |
| `--delay` | `1s` | Minimum delay between requests |
| `--retries` | `4` | Retry attempts on 429/5xx |
| `--timeout` | `30s` | Per-request timeout |
| `--no-cache` | `false` | Bypass the on-disk cache |
| `--cache-ttl` | `1h` | Cache freshness window |
| `--lang` | `en-US` | `Accept-Language` / locale |
| `-q, --quiet` | `false` | Suppress progress on stderr |
| `-v, --verbose` | | Increase verbosity (repeatable) |
| `--proxy` | | HTTP/SOCKS proxy URL |
| `--user-agent` | | Override the default crawler UA |
| `--session` | | Logged-in session id (or `THREADS_SESSION`) |
| `--csrf` | | Session CSRF token (or `THREADS_CSRF`) |

## Access modes

By default `th` reads anonymously as a crawler. An optional session mode can
add depth on your own account when you explicitly supply `--session` and
`--csrf` (or `THREADS_SESSION` and `THREADS_CSRF`). There is no login flow.

`th whoami` always reports which mode is active.

## Deep research mode

`research --mode deep` is a separate Russian small-business collection path. It
uses short, fixed Russian anchors rather than sending the long objective to
search; then it applies deterministic owner/pain gates, bounded profile-first
collection, observed secondary anchors, and limited reply evidence. It is
anonymous even when credentials are configured globally, and `--analyze` is
rejected in this mode.

The default safety envelope is 1,000 candidate seed profiles, 300 verified
owner/operator profiles, 50 recent posts per verified profile, and 15,000
total profile posts. Override it explicitly when needed with
`--max-seed-profiles`, `--max-verified-profiles`, `--profile-posts`, and
`--max-total-profile-posts` (the maximums are 1,000, 300, 50, and 15,000).
Candidate profile metadata is inspected before recent posts are fetched; the
export records acceptance/rejection reasons and business-type coverage. Use
`--resume <run-id>` to continue a checkpointed SQLite run.

Deep mode writes its export pack to `--export-dir` (or
`research-output/deep-research-<run-id>`): a complete readable JSON report,
JSONL corpus, all seed candidates, verified/rejected owner files, business-type
coverage, pain signal files, query diagnostics, run summary, and the source
SQLite database. Pagination flags identify SSR-window exhaustion,
per-query/profile ceilings, access degradation, and missing GraphQL continuation;
they do not imply complete Threads history.

## Local semantic audit

`semantic-audit` reads an existing deep `complete-readable.json` and writes a
separate `semantic-v2` evidence pack. It does not recollect posts, use Threads
credentials, or call an LLM provider. The pack contains the complete
side-by-side corpus plus `verified-pains.json`, `operational-workloads.json`,
`solution-seeking.json`, `strong-signals.json`, `gold-signals-v2.json`,
`workflow-clusters.json`, `author-business-context.json`, and
`semantic-audit.md`.

```sh
./bin/th semantic-audit research-output/deep-research-<run-id>/export/complete-readable.json \
  --output-dir research-output/deep-research-<run-id>/semantic-v2
```

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Generic error |
| `2` | Usage error |
| `3` | Content not found or unavailable |
| `4` | Login wall: the content is not public |
| `5` | Rate limited |
| `6` | Network error |
