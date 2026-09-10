# CLI reference

`th` is a read-only command-line client for the public Threads web surface.
Run `./bin/th <command> --help` for the complete flag list.

## Commands

| Command | Purpose | Network |
| --- | --- | --- |
| `id <input>` | Classify a handle, numeric ID, shortcode, or URL | No |
| `profile <handle>` | Fetch public profile metadata | Yes |
| `profile <handle> --posts` | Stream recent profile posts | Yes |
| `profile <handle> --replies` | Stream the profile's replies feed | Yes |
| `feed <handle>` | Alias for `profile --posts` | Yes |
| `post <url>` | Fetch one public post | Yes |
| `post <url> --replies` | Stream visible replies | Yes |
| `replies <url>` | Alias for post replies | Yes |
| `search <query>` | Search public posts | Yes |
| `research <topic>` | Run bounded topic research | Yes |
| `semantic-audit <file>` | Reclassify an existing deep export locally | No |
| `db build <handle>` | Store a profile feed in SQLite | Yes |
| `db query <sql>` | Query a local SQLite dataset | No |
| `whoami` | Show the current access mode and crawler UA | No |
| `config show` / `config path` | Show resolved settings and directories | No |
| `cache dir` / `cache clear` | Inspect or clear the response cache | No |
| `completion <shell>` | Generate bash, zsh, fish, or PowerShell completion | No |
| `version` | Show build and runtime version information | No |

## Output

All data commands share these formats:

```sh
./bin/th search "workflow" --limit 10 --output table
./bin/th search "workflow" --limit 10 --output json
./bin/th search "workflow" --limit 10 --output jsonl
./bin/th search "workflow" --limit 10 --output csv
./bin/th search "workflow" --limit 10 --output url
```

`json` is one array; `jsonl` is one object per line. When `--output auto` is
used, an interactive terminal receives a table and a pipe receives JSONL.
`--fields` projects columns, and `--template` renders one Go template per row.

## Search

The search command accepts one or more words and joins them into a query:

```sh
./bin/th search "finding clients for freelancers" --limit 10 --output jsonl
```

The current provider surface supplies its own ordering. `--type top` is the
only supported value; `--type recent` is rejected clearly rather than silently
pretending to change the result set.

## Profiles and posts

```sh
./bin/th profile example_creator --output json
./bin/th profile example_creator --posts --limit 20 --output jsonl
./bin/th post "https://www.threads.com/@example_creator/post/AbCdEf" --output json
./bin/th post "https://www.threads.com/@example_creator/post/AbCdEf" --replies --limit 50 --output jsonl
```

The public page may expose only a recent/rendered window. A missing metric is
left empty; it is not inferred from another metric.

## SQLite

```sh
mkdir -p output
./bin/th db build example_creator --limit 100 --db output/profile.db
./bin/th db query "select username, count(*) from posts group by username" --db output/profile.db
```

`db query` intentionally accepts SQL. Only run SQL you trust against a local
file.

## Global flags

The most useful flags are:

| Flag | Default | Purpose |
| --- | --- | --- |
| `--output` | `auto` | Select the output encoding |
| `--limit` | `0` | Maximum emitted records; zero means no row limit |
| `--delay` | `1s` | Minimum delay between requests |
| `--retries` | `4` | Retries for transient HTTP failures |
| `--timeout` | `30s` | Per-request timeout |
| `--no-cache` | false | Bypass the local response cache |
| `--cache-ttl` | `1h` | Cache freshness window |
| `--lang` | `en-US` | `Accept-Language` value |
| `--quiet` | false | Keep progress messages off stderr |
| `--verbose` | 0 | Increase request diagnostics |
| `--proxy` | | HTTP or SOCKS proxy URL |
| `--session` | | Supplied session id cookie; optional compatibility path |
| `--csrf` | | Supplied session CSRF token; optional compatibility path |

The credential flags are retained for the existing client compatibility path,
but are not needed for public anonymous research. `th` never logs in or obtains
credentials for the user.

## Exit codes

| Code | Meaning |
| ---: | --- |
| `0` | Success, including a valid empty result |
| `1` | Generic local/runtime error |
| `2` | Invalid command, argument, or flag |
| `3` | Content unavailable, not found, or stale provider shape |
| `4` | Anonymous access met a login wall |
| `5` | Rate limited after retries |
| `6` | Network failure |
