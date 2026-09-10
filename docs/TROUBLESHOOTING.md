# Troubleshooting

## `go: command not found`

Install Go 1.26 or newer, then confirm:

```sh
go version
```

If you do not want to build from source, use a published release archive when
one is available. The repository does not install Go for you.

## Search returns zero records

Zero records is a valid outcome for the public search surface. The query may be
too narrow, the public page may not expose search results in your region, or
Threads may have changed its anonymous search response. Try a broader phrase,
wait and retry later with a reasonable delay, and compare with a public browser
view. Do not interpret one empty run as proof that the topic has no posts.

An exit code of `0` with no JSONL rows is different from an exit code of `3`,
`4`, `5`, or `6`, which indicates an unavailable page, login wall, rate limit,
or network failure.

## Search or pagination reports stale provider shape

Threads rotates undocumented persisted query IDs. The server-rendered window may
still be returned, but continuation can stop early. Run with `--verbose` to see
request diagnostics and treat the result as partial. Do not attempt to bypass a
rate limit or login wall.

## Cyrillic or shell quoting

Quote multi-word and non-ASCII queries:

```sh
./bin/th search "проблемы малого бизнеса" --lang ru-RU --limit 10
```

If your terminal encoding is unusual, save the command in a UTF-8 shell file or
use the system terminal's normal UTF-8 locale.

## Viewer cannot find the input

Run it from the repository root or pass the correct path:

```sh
python3 scripts/research-viewer.py --input path/to/export.json
```

The input must be valid JSON with a top-level `corpus` or `posts` array (or be
a JSON array itself). Validate it with:

```sh
python3 -m json.tool path/to/export.json >/dev/null
```

## Viewer port is busy

Choose another local port:

```sh
python3 scripts/research-viewer.py --input examples/sample-export.json --port 4174
```

Then open `http://127.0.0.1:4174/research`.

## SQLite errors

Make sure the parent directory for `--db` exists and the file is writable. Use
one process per database file for a simple local run. If a database is locked,
stop another process using it or choose a new output path. SQLite files are
local generated artifacts; do not commit them with collected text.

## Optional provider analysis

The core CLI does not require an analysis provider. If you explicitly use
`--analyze`, check the provider, model, endpoint, and environment variables in
your own shell. Never paste API keys, cookies, or private account data into an
issue or commit. Deep mode intentionally rejects `--analyze`.

## Network and access errors

Use `--verbose` for request-level diagnostics. Increase `--delay` when receiving
429 responses. Public pages can be unavailable, login-walled, or changed by
Threads; the project cannot guarantee live coverage.
