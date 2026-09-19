# Agentry Desktop. Depends on the private modules
# github.com/blaktron/agentry and github.com/blaktron/agentry-cli:
#   export GOPRIVATE=github.com/blaktron/*
# plus git credentials for github.com/blaktron (a PAT via credential helper
# or an url.<...>.insteadOf rewrite).
export PATH := $(HOME)/go/bin:/opt/homebrew/bin:$(PATH)
GO ?= go
# The version is git describe without the tag's leading v: a tagged commit
# gives 0.1.0, which is what wails.json's productVersion must equal (the
# release workflow refuses a tag that does not), so the bundle metadata, the
# binary and the artifact names all say the same thing; between tags it is
# 0.1.0-3-g<sha>[-dirty], and a checkout with no tag at all is the bare sha.
VERSION := $(shell (git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev) | sed 's/^v//')
# EXTRA_LDFLAGS lets a caller add to the linker flags (a baked-in default
# registry, -w -s) without taking the version stamp away from this file.
EXTRA_LDFLAGS ?=
LDFLAGS := -X github.com/blaktron/agentry-desktop/internal/version.Version=$(VERSION) $(EXTRA_LDFLAGS)
# Extra flags for `wails build` in package-darwin. scripts/build-darwin.sh
# passes `-m` (skip `go mod tidy`): a tree shipped with vendor/ and no GitHub
# credential builds in vendor mode on its own, but tidy walks the whole module
# graph past vendor/ and asks GitHub for the private modules.
WAILS_FLAGS ?=

.PHONY: dev build check lint test copy frontend package package-darwin package-windows package-linux version-check version clean bindings

# Live-reload development shell (wails dev). -skipbindings on every build:
# the checked-in bindings are the ones `make bindings` produced (see there),
# and a build that regenerated them would undo that. Run `make bindings`
# after changing a bound Go method or type.
dev:
	wails dev -skipbindings -ldflags '$(LDFLAGS)'

# Production build for the host platform (macOS: build/bin/*.app).
build:
	wails build $(WAILS_FLAGS) -skipbindings -ldflags '$(LDFLAGS)'

# Regenerate the TypeScript bindings after changing bound Go methods/types.
# The generator spells a Go map as TypeScript's generic Record<K, V>; inside
# the `installed` namespace of models.ts that name is the CLI's install
# record class, so the type-check reads the generic as the class and fails
# ("Type 'Record' is not generic", desktop#97). An index signature is the
# same type without the shadowed name, so every map in models.ts is
# respelled that way (the per-package .d.ts files keep Record<>: nothing
# shadows it there). A shape the sed cannot respell — a nested generic —
# fails here by name rather than later in tsc. CI regenerates and fails on
# drift, so this is the only way models.ts changes. Generating compiles
# the app, whose go:embed needs frontend/dist to hold a file: on a fresh
# tree (the release preflight, a new clone) an empty index.html stands in
# and is removed again, so nothing downstream can mistake it for a bundle.
bindings:
	@stub=0; if [ ! -f frontend/dist/index.html ]; then mkdir -p frontend/dist && : > frontend/dist/index.html && stub=1 && echo "frontend/dist was missing: an empty index.html stands in for the embed while the bindings generate (go:embed all:frontend/dist must match) and is removed afterwards; make frontend builds the real one"; fi; \
	wails generate module; st=$$?; if [ "$$stub" = 1 ]; then rm -rf frontend/dist; fi; exit $$st
	sed -i.bak -E 's/Record<string, ([^<>]+)>/{[key: string]: \1}/g' frontend/wailsjs/go/models.ts && rm -f frontend/wailsjs/go/models.ts.bak
	@if grep -n 'Record<' frontend/wailsjs/go/models.ts; then echo "models.ts still carries a Record<> the bindings sed cannot respell (a nested generic?): widen the sed in this target"; exit 1; fi

lint:
	@fmt_out=$$(gofmt -l . | grep -v -e '^frontend/' -e '^vendor/' || true); if [ -n "$$fmt_out" ]; then echo "gofmt needed on:"; echo "$$fmt_out"; exit 1; fi
	$(GO) vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; else echo "golangci-lint not installed; skipped"; fi

test:
	$(GO) test ./...

# The honest-posture rule as a check over the copy this app ships (plan
# hosted-runs.md §3): no "safe", "secure", "guaranteed" or "trusted".
copy:
	./scripts/copy-check.sh

# Frontend type-check plus the dist bundle main.go embeds. `go build ./...`,
# `go vet ./...` and `go test ./...` all fail on a fresh tree without
# frontend/dist, so `check` runs this target first; wails builds its own
# bundle at packaging time.
frontend:
	cd frontend && npm install --no-audit --no-fund && npx tsc --noEmit && npx vite build

check: frontend lint test copy
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

# Windows: cross-compiled from any host (DEV-097: Wails v2 loads WebView2
# through a pure-Go loader, so windows/amd64 builds with cgo off on Linux
# and macOS alike; the wails CLI supplies the icon, the manifest and the GUI
# subsystem a bare go build would not). Unsigned (agentry-desktop#54).
# Output: build/bin/agentry-desktop-$(VERSION)-windows-amd64.exe and its zip.
package-windows:
	wails build $(WAILS_FLAGS) -platform windows/amd64 -ldflags '$(LDFLAGS)'
	cd build/bin && cp agentry-desktop.exe agentry-desktop-$(VERSION)-windows-amd64.exe && rm -f agentry-desktop-$(VERSION)-windows-amd64.zip && zip -q agentry-desktop-$(VERSION)-windows-amd64.zip agentry-desktop.exe

# Linux: built inside a Debian container that carries the GTK 3 and WebKit2GTK
# 4.1 headers, from the vendored tree, so the host needs Docker and nothing
# else (scripts/build-linux.sh). Output: build/linux/agentry-desktop-$(VERSION)-linux-amd64
package-linux:
	DESKTOP_VERSION='$(VERSION)' EXTRA_LDFLAGS='$(EXTRA_LDFLAGS)' scripts/build-linux.sh

package: package-darwin package-windows package-linux

# A release is a tag v<productVersion>: the tag names the version wails.json
# carries into the macOS bundle and the Windows version-info block, and the
# Makefile stamps the same string into the binary. RELEASE_TAG is the tag
# being cut (the release workflow passes github.ref_name).
version-check:
	@tag='$(RELEASE_TAG)'; pv=$$(jq -r .info.productVersion wails.json); \
	if [ -z "$$tag" ]; then echo "RELEASE_TAG is not set"; exit 2; fi; \
	if [ "$${tag#v}" != "$$pv" ]; then echo "tag $$tag does not name wails.json productVersion $$pv: bump productVersion in a commit on main first, then tag"; exit 1; fi; \
	echo "tag $$tag names productVersion $$pv"

clean:
	rm -rf build/bin build/linux frontend/dist

# The version the Makefile stamps (the release workflow names the bundle by it).
version:
	@echo $(VERSION)
