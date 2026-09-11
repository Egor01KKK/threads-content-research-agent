# th

An open-source, read-only CLI for researching public [Threads](https://www.threads.com/) profiles, posts, replies, and keyword search. It exports structured JSON/JSONL/CSV and local SQLite data, with an optional browser viewer for existing exports.

The core workflow needs no Threads login, browser automation, ChatGPT, or paid API. It reads the public server-rendered surface that Threads makes available to anonymous crawlers, so coverage can change and some queries can return nothing.

## What it does

- Resolves handles, numeric IDs, shortcodes, and Threads URLs with `th id`.
- Fetches public profiles, recent profile posts, individual posts, and visible replies.
- Searches public posts with the current server-rendered search surface and a compatibility fallback.
- Runs bounded topic research with deterministic query expansion, deduplication, author context, engagement metrics, baseline comparisons, and evidence-linked reports.
- Stores collected research in SQLite and emits JSON, JSONL, CSV, TSV, YAML, URL, or table output.
- Opens an existing JSON export in the optional read-only Research Viewer.

## What it is not

`th` is not an official Meta/Threads API client, a guaranteed full-platform search engine, a private-account scraper, a growth bot, a posting tool, or a login/session automation tool. It does not bypass authentication, CAPTCHAs, rate limits, or platform restrictions. Deterministic labels in research reports are heuristics, not ground truth. LLM analysis is not required by the core product and is not run by default.

## Status and limitations

The public Threads surface used by this project is undocumented and can change without notice. Search may return zero records even when a conversation exists, and profile/post pages may be login-walled or unavailable. The persisted GraphQL query IDs used for limited continuation are rotated by Threads; when they become stale, the already-parsed server-rendered window can still be useful but is partial. Treat every run as a bounded sample, not a census.

The current repository is an experimental `v0.1.x`-style productization of the existing collector. The anonymous profile/post/search commands and local exports are the supported core. Topic research and the viewer are useful but limited. The optional `--analyze` provider path is experimental and requires credentials supplied by the user; it is not part of the five-minute setup.

## Requirements

- Go 1.26 or newer for building from source (`go.mod` is the source of truth).
- Git for cloning and `make` for the documented source-build shortcuts.
- Python 3.10 or newer only for the optional viewer.
- Network access to `www.threads.com` for live collection; `th id`, formatting, tests, and the viewer fixture work offline.

The core CLI is pure Go and is expected to build on macOS, Linux, and Windows. The Makefile and viewer are tested on Unix-like systems; Windows users can use `go build ./cmd/th` and run the resulting binary directly. Prebuilt release artifacts are prepared for macOS arm64/amd64, Linux arm64/amd64, and Windows amd64 when a version tag is published.

## Five-minute quick start

From a fresh checkout:

```sh
git clone https://github.com/Egor01KKK/threads-content-research-agent.git
cd threads-content-research-agent
make build
./bin/th --help
```

These deterministic commands work offline:

```sh
./bin/th version
./bin/th --help
./bin/th id "@example_creator" -o json
```

These commands read the current public Threads surface and require network access:

```sh
./bin/th profile <handle>
./bin/th post "<threads-post-url>" --output json
./bin/th search "AI automation" --limit 5 --output jsonl
./bin/th research "finding clients for freelancers" --db output/research.db
```

Try a small public search. Search output is JSON Lines when piped; an empty file is a valid result, while a non-zero exit code is an error or access limitation:

```sh
mkdir -p output
./bin/th search "AI automation" --limit 5 --output jsonl > output/search.jsonl
```

Open the included synthetic fixture in the viewer without collecting anything:

```sh
python3 scripts/research-viewer.py --input examples/sample-export.json
```

Then open <http://127.0.0.1:4173/research>. Stop the server with `Ctrl-C`. The fixture is deliberately synthetic; its placeholder Threads URLs are for UI testing and are not claims about real posts.

## Installation

### From source

```sh
git clone https://github.com/Egor01KKK/threads-content-research-agent.git
cd threads-content-research-agent
make build                 # writes ./bin/th
./bin/th version
```

### With Go

The module path in this checkout is `github.com/Egor01KKK/threads-content-research-agent`:

```sh
go install github.com/Egor01KKK/threads-content-research-agent/cmd/th@latest
```

Go installs the binary into `$(go env GOPATH)/bin`; add that directory to `PATH` if `th` is not found. Releases, when published, will provide standalone archives; do not assume a release exists until the repository owner publishes one.

## Common commands

```sh
./bin/th profile <handle>                         # public profile metadata
./bin/th profile <handle> --posts --limit 20      # recent posts
./bin/th profile <handle> --replies --limit 20    # profile replies
./bin/th post "<threads-post-url>" --output json  # one public post
./bin/th post "<threads-post-url>" --replies --output jsonl
./bin/th search "<keywords>" --limit 10 --output jsonl
./bin/th db build <handle> --db output/profile.db
./bin/th db query "select count(*) from posts" --db output/profile.db
```

Run `./bin/th <command> --help` for the exact arguments and flags. Use `--no-cache` for a fresh request, `--delay` to increase pacing, and `--quiet` when stdout must contain only structured output.

## Topic research

The default bounded research mode expands a topic into a small deterministic query set, searches public posts, deduplicates by post ID, enriches a bounded set of authors, stores independent recent-post baselines, and renders a report with source URLs. It does not require an LLM:

```sh
mkdir -p output
./bin/th research "finding clients for freelancers" \
  --queries 5 --per-query 10 --profile-limit 10 \
  --db output/research.db --output jsonl > output/research.jsonl
```

`research-output/` is the default location for deep-mode exports. Override it explicitly when you need a different location:

```sh
./bin/th research "problems small businesses want to automate" \
  --mode deep --lang ru-RU --db output/deep.db \
  --export-dir output/deep-research --output jsonl > output/deep-report.jsonl
```

Deep mode is a bounded, experimental Russian small-business workflow. It is not a generic semantic search engine and rejects topics outside its supported domain. See [docs/RESEARCH-MODE.md](docs/RESEARCH-MODE.md) for the data flow and safety limits.

The existing `--analyze` option is an opt-in provider integration. It may send selected post text to a user-configured external provider and is intentionally absent from the quick start. Core collection, deterministic metrics, and the viewer do not call it.

## Output and viewer

Use `--output json` for one JSON array, `--output jsonl` for one record per line, `--output url` for permalinks, and `--fields` or `--template` to project fields. Research stores source records and derived metrics separately; missing metrics remain missing rather than being guessed.

Launch the viewer for any compatible export:

```sh
python3 scripts/research-viewer.py --input path/to/complete-readable.json
```

To load the optional recording package as well:

```sh
python3 scripts/research-viewer.py \
  --input path/to/complete-readable.json \
  --prep path/to/video-prep.json
```

The server binds to `127.0.0.1` and serves only the viewer assets plus the two explicitly selected JSON routes. See [docs/VIEWER.md](docs/VIEWER.md).

## Development

```sh
make test          # go test ./...
make vet           # go vet ./...
make check         # tests, vet, and lightweight viewer checks
make build         # ./bin/th
make smoke         # bounded live/offline smoke checks; requires Threads access for live steps
make viewer        # serve the synthetic fixture
```

The Go package layout is described in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). The output contract is in [docs/OUTPUT-SCHEMA.md](docs/OUTPUT-SCHEMA.md), and common failures are covered by [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md).
Maintainers can use [docs/RELEASING.md](docs/RELEASING.md) for the tag and
artifact checklist. The public-surface decisions are recorded in
[RELEASE-SANITIZATION.md](RELEASE-SANITIZATION.md), with the local verification
results in [PUBLIC-RELEASE-TEST.md](PUBLIC-RELEASE-TEST.md).

## Responsible use

Use reasonable delays and bounded limits. You are responsible for complying with applicable laws and platform terms when using public data. This project is designed for public, read-only access and does not provide authentication bypass or access to private content. Do not publish credentials, cookies, private account data, or personal research exports.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) for the local development workflow and [SECURITY.md](SECURITY.md) for privacy and vulnerability-reporting guidance.

## License

The repository is licensed under the Apache License 2.0 in [LICENSE](LICENSE). This project was originally derived from [tamnd/threads-cli](https://github.com/tamnd/threads-cli); the upstream copyright notice and license terms are retained.
