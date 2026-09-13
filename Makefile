# Build into bin/ (gitignored) so the binary never collides with the threads/
# source package at the repo root.
BINARY  := bin/th
PKG     := ./cmd/th
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VIEWER_INPUT ?= examples/sample-export.json
VIEWER_HOST  ?= 127.0.0.1
VIEWER_PORT  ?= 4173
LDFLAGS := -s -w \
	-X github.com/Egor01KKK/threads-content-research-agent/cli.Version=$(VERSION) \
	-X github.com/Egor01KKK/threads-content-research-agent/cli.Commit=$(COMMIT) \
	-X github.com/Egor01KKK/threads-content-research-agent/cli.Date=$(DATE)

.PHONY: build install test vet fmt check public-check viewer-check viewer studio clean run smoke

build:
	@mkdir -p $(dir $(BINARY))
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

install:
	CGO_ENABLED=0 go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

test:
	go test ./...

vet:
	go vet ./...

check: test vet public-check viewer-check

public-check:
	python3 scripts/public-release-check.py

viewer-check:
	python3 -m py_compile scripts/research-viewer.py
	python3 scripts/research-viewer-test.py
	@if command -v node >/dev/null 2>&1; then node --check viewer/app.js; else echo "warning: node not installed; skipped viewer JavaScript syntax check"; fi

fmt:
	gofmt -w -s .

clean:
	rm -rf bin dist

smoke: build
	TH=./$(BINARY) ./scripts/smoke.sh

viewer: viewer-check
	python3 scripts/research-viewer.py --input "$(VIEWER_INPUT)" --host "$(VIEWER_HOST)" --port "$(VIEWER_PORT)" $(if $(VIEWER_PREP),--prep "$(VIEWER_PREP)",)

studio: build
	python3 scripts/studio.py

run: build
	./$(BINARY) $(ARGS)
