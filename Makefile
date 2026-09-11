# Agentry Desktop. Depends on the private modules
# github.com/blaktron/agentry and github.com/blaktron/agentry-cli:
#   export GOPRIVATE=github.com/blaktron/*
# plus git credentials for github.com/blaktron (a PAT via credential helper
# or an url.<...>.insteadOf rewrite).
export PATH := $(HOME)/go/bin:/opt/homebrew/bin:$(PATH)
GO ?= go
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)
# EXTRA_LDFLAGS lets a caller add to the linker flags (a baked-in default
# registry, -w -s) without taking the version stamp away from this file.
EXTRA_LDFLAGS ?=
LDFLAGS := -X github.com/blaktron/agentry-desktop/internal/version.Version=$(VERSION) $(EXTRA_LDFLAGS)
# Extra flags for `wails build` in package-darwin. scripts/build-darwin.sh
# passes `-m` (skip `go mod tidy`): a tree shipped with vendor/ and no GitHub
# credential builds in vendor mode on its own, but tidy walks the whole module
# graph past vendor/ and asks GitHub for the private modules.
WAILS_FLAGS ?=

.PHONY: dev build check lint test frontend package package-darwin package-windows clean bindings

# Live-reload development shell (wails dev).
dev:
	wails dev -ldflags '$(LDFLAGS)'

# Production build for the host platform (macOS: build/bin/*.app).
build:
	wails build -ldflags '$(LDFLAGS)'

# Regenerate the TypeScript bindings after changing bound Go methods/types.
bindings:
	wails generate module

lint:
	@fmt_out=$$(gofmt -l . | grep -v -e '^frontend/' -e '^vendor/' || true); if [ -n "$$fmt_out" ]; then echo "gofmt needed on:"; echo "$$fmt_out"; exit 1; fi
	$(GO) vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; else echo "golangci-lint not installed; skipped"; fi

test:
	$(GO) test ./...

# Frontend type-check plus the dist bundle main.go embeds. `go build ./...`,
# `go vet ./...` and `go test ./...` all fail on a fresh tree without
# frontend/dist, so `check` runs this target first; wails builds its own
# bundle at packaging time.
frontend:
	cd frontend && npm install --no-audit --no-fund && npx tsc --noEmit && npx vite build

check: frontend lint test
	$(GO) build ./...

# --- packaging -------------------------------------------------------------
# Package names follow the shape the download manifest matches — product,
# version, os, arch, lowercase — so a built artifact can reach the download
# page instead of being dropped for its name (agentry-web#59 teaches the
# manifest about the archive extension a .app has to ship in).
#
# macOS: a universal .app zipped for distribution (unsigned/un-notarized —
# see DEVIATIONS.md D-001).
# Output: build/bin/agentry-desktop-$(VERSION)-darwin-universal.zip
package-darwin:
	wails build $(WAILS_FLAGS) -platform darwin/universal -ldflags '$(LDFLAGS)'
	cd build/bin && ditto -c -k --keepParent agentry-desktop.app agentry-desktop-$(VERSION)-darwin-universal.zip

# Windows: cross-compiled from macOS (Wails v2 supports this; webview2
# loader is compile-time). Output: build/bin/agentry-desktop.exe
package-windows:
	wails build -platform windows/amd64 -ldflags '$(LDFLAGS)'
	cd build/bin && zip -q agentry-desktop-$(VERSION)-windows-amd64.zip agentry-desktop.exe

# Linux is built on a Linux host (webkit2gtk links natively); see README.
package: package-darwin package-windows

clean:
	rm -rf build/bin frontend/dist
