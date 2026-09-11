---
title: "Installation"
description: "Install th from a release, with go install, or from source."
weight: 20
---

## Prebuilt binaries

When a release is published, its release page carries archives for macOS
arm64/amd64, Linux arm64/amd64, and Windows amd64, plus a plain SHA256
`checksums.txt`. Download the archive for your platform, unpack it, and put
`th` on your `PATH`.

If the repository has no published release yet, use `go install` or build from
source below.

## With Go

```bash
go install github.com/Egor01KKK/threads-content-research-agent/cmd/th@latest
```

That puts `th` in `$(go env GOPATH)/bin`, which is `~/go/bin` unless
you moved it. Make sure that directory is on your `PATH`.

## From source

```bash
git clone https://github.com/Egor01KKK/threads-content-research-agent.git
cd threads-content-research-agent
make build        # produces ./bin/th
./bin/th version
```

## Checking the install

```bash
th version
```

prints the version and exits.
