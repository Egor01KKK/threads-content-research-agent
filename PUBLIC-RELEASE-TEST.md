# Public release test report

Date: 2026-09-10

This report records the local and post-publication verification checks for the
public repository. It does not collect a new Threads corpus or use any paid
LLM API. The viewer checks use the synthetic fixture in `examples/`.

## Passed

- `go version` was available (`go1.27.1`, darwin/arm64).
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `make check` (Go tests, vet, Python viewer checks, and route tests)
- `make build`
- Root help and command help for `search`, `post`, `research`, `profile`,
  `feed`, `id`, `db`, `version`, `whoami`, and `config`.
- Representative bad-input checks: missing search input, an unknown command,
  an invalid post identifier, a missing semantic input file, and an unsupported
  search type all return concise errors and non-zero exit codes.
- A fresh clone of the public repository passed `go mod verify`, `make check`,
  `make build`, offline version/help, and viewer route/data checks.
- `golangci-lint v2.13.2` reports 0 issues locally.
- `govulncheck` reports 0 reachable vulnerabilities locally after upgrading
  `golang.org/x/text` to its fixed release.
- GitHub CI passed on the final public release source, including macOS/Linux
  tests, tidy, vet, lint, govulncheck, and release-config.
- The docs workflow built and uploaded its docs artifact.
- The tag-only release workflow published `v0.1.0` with five archives and
  `checksums.txt`; one macOS arm64 archive passed checksum and binary smoke
  verification.
- The viewer served the synthetic fixture in a browser. Search filtered the
  feed, inspector opened records with and without optional metrics, and the
  feed scroll region changed position when scrolled.

## Deliberately not run

- `make smoke`: it performs bounded live/offline smoke checks and the live path
  contacts public Threads endpoints. It is not a deterministic release test.
- A large or persistent live research collection. The release validation uses
  only the synthetic viewer fixture and public source checks.
- A local GoReleaser install in the final pass; the current tag release itself
  was built and published successfully by GoReleaser in GitHub Actions.

## Current warnings

- GitHub reports a Node.js 20 deprecation warning for the current
  `golangci/golangci-lint-action` runtime; the lint job itself passes.
- Public Threads availability, rate limits, login walls, and search-result
  quality can change. The CLI is intentionally bounded and read-only; live
  collection is not guaranteed to be complete.
