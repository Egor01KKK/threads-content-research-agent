# Public release test report

Date: 2026-09-10

This report records the local release-readiness checks for the public repository.
It does not publish a repository, create a tag, contact Threads, or use any paid
API. The viewer checks use the synthetic fixture in `examples/`.

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
- A fresh checkout-like copy containing only public source and documentation
  passed `make check`, `make build`, help, `th id`, viewer route checks, and
  workflow YAML parsing.
- GoReleaser configuration validation with GoReleaser v2.18.1.
- GoReleaser snapshot build with `--snapshot --clean`: five archives were
  produced for macOS arm64/amd64, Linux arm64/amd64, and Windows amd64, plus
  `checksums.txt`. No release was published.
- The viewer served the synthetic fixture in a browser. The feed scroll region
  had a smaller viewport than its content and its scroll position changed when
  scrolled.

## Deliberately not run

- `make smoke`: it performs bounded live/offline smoke checks and the live path
  contacts public Threads endpoints. It is not a deterministic release test.
- GitHub Actions themselves, including the docs build. The docs workflow still
  depends on the external `tamnd/tago` submodule/tool and is configured as a
  build-only artifact workflow, not a deploy workflow.
- `golangci-lint` and `govulncheck` locally; neither executable was installed.
  Both checks remain configured in CI.

## Warnings before publishing

- The current Git history and repository metadata need an ownership review. The
  repository contains upstream attribution and local author metadata. Confirm
  redistribution rights, preserve the Apache 2.0 notice, and decide whether the
  history should be cleaned before a public push.
- The first GitHub run should verify CI, the docs artifact build, and a version
  tag release before the project is described as published.
- Public Threads availability, rate limits, login walls, and search-result
  quality can change. The CLI is intentionally bounded and read-only; live
  collection is not guaranteed to be complete.
