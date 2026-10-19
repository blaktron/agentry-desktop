# Agentry Desktop. Depends on the modules github.com/blaktron/agentry and
# github.com/blaktron/agentry-cli. Where they are not fetchable anonymously:
#   export GOPRIVATE=github.com/blaktron/*
# plus git credentials for github.com/blaktron (a PAT via credential helper
# or an url.<...>.insteadOf rewrite).
export PATH := $(HOME)/go/bin:/opt/homebrew/bin:$(PATH)
GO ?= go
# The version is git describe without the tag's leading v: a tagged commit
# gives 0.6.0, or 0.6.1-dev.2 for a dev prerelease, whose X.Y.Z is what
# wails.json's productVersion must equal (version-check refuses a tag that
# does not), so the bundle metadata, the binary and the artifact names
# agree; between tags it is 0.6.0-3-g<sha>[-dirty], and a checkout with no
# tag at all is the bare sha. A release passes its tag's own version as
# VERSION=, since describe picks one of several tags on one commit.
VERSION := $(shell (git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev) | sed 's/^v//')
# EXTRA_LDFLAGS lets a caller add to the linker flags (a baked-in default
# registry, -w -s) without taking the version stamp away from this file.
EXTRA_LDFLAGS ?=
LDFLAGS := -X github.com/blaktron/agentry-desktop/internal/version.Version=$(VERSION) $(EXTRA_LDFLAGS)
# Extra flags for `wails build`, such as `-m` (skip `go mod tidy`): a tree
# shipped with vendor/ and no GitHub credential builds in vendor mode on its
# own, but tidy walks the whole module graph past vendor/ and asks GitHub for
# the private modules. -skipbindings is
# not a caller's choice: every wails target below carries it (see `dev`), so
# a by-hand `make package-darwin` cannot regenerate the bindings either.
# Callers that still pass it are harmless.
WAILS_FLAGS ?=
# SIGN_CMD: a command that Authenticode-signs the one PE file it is given, in
# place, which a release passes. Empty means an unsigned Windows package,
# which PR and local builds are.
SIGN_CMD ?=

.PHONY: dev build check lint test copy script-tests frontend notices notices-check package package-darwin build-darwin wrap-darwin package-windows build-windows sign-windows finish-windows package-linux smoke-linux-packages smoke-darwin-dmg smoke-darwin-selfupdate app-icons version-check version clean bindings workflows-check

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
# ("Type 'Record' is not generic"). An index signature is the
# same type without the shadowed name, so every map in models.ts is
# respelled that way (the per-package .d.ts files keep Record<>: nothing
# shadows it there). A shape the sed cannot respell — a nested generic —
# fails here by name rather than later in tsc. CI regenerates and fails on
# drift, so this is the only way models.ts changes. Generating compiles
# the app, whose go:embed needs frontend/dist to hold a file: on a fresh
# tree (a release's preflight, a new clone) an empty index.html stands in
# and is removed again, so nothing downstream can mistake it for a bundle.
bindings:
	@stub=0; if [ ! -f frontend/dist/index.html ]; then mkdir -p frontend/dist && : > frontend/dist/index.html && stub=1 && echo "frontend/dist was missing: an empty index.html stands in for the embed while the bindings generate (go:embed all:frontend/dist must match) and is removed afterwards; make frontend builds the real one"; fi; \
	wails generate module; st=$$?; if [ "$$stub" = 1 ]; then rm -rf frontend/dist; fi; exit $$st
	sed -i.bak -E 's/Record<string, ([^<>]+)>/{[key: string]: \1}/g' frontend/wailsjs/go/models.ts && rm -f frontend/wailsjs/go/models.ts.bak
	@if grep -n 'Record<' frontend/wailsjs/go/models.ts; then echo "models.ts still carries a Record<> the bindings sed cannot respell (a nested generic?): widen the sed in this target"; exit 1; fi

# No `| grep -q` in the scripts: under pipefail the writer that grep -q cuts
# off reports EPIPE as the pipeline's failure, an intermittent CI failure.
# Comment lines are skipped.
lint:
	@hits=$$(grep -rnE '\|[[:space:]]*grep[[:space:]]+(-[A-Za-z]*q|--quiet)' scripts/ | grep -vE '^[^:]+:[0-9]+:[[:space:]]*#' || true); if [ -n "$$hits" ]; then echo "$$hits"; echo "a piped grep -q under pipefail can fail on the writer's EPIPE: use grep -q ... <<<\"\$$x\" or grep ... >/dev/null"; exit 1; fi
	@fmt_out=$$(gofmt -l . | grep -v -e '^frontend/' -e '^vendor/' || true); if [ -n "$$fmt_out" ]; then echo "gofmt needed on:"; echo "$$fmt_out"; exit 1; fi
	$(GO) vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; else echo "golangci-lint not installed; skipped"; fi

test:
	$(GO) test ./...

# A check over the copy this app ships: no "safe", "secure", "guaranteed" or
# "trusted" (a claim the product cannot back).
copy:
	./scripts/copy-check.sh

# The packaging scripts' own behaviour, against stand-ins for the tools they
# call, so it runs on any host.
script-tests:
	./scripts/acceptance/darwin-dmg-busy.sh

# Frontend type-check plus the dist bundle main.go embeds. `go build ./...`,
# `go vet ./...` and `go test ./...` all fail on a fresh tree without
# frontend/dist, so `check` runs this target first; wails builds its own
# bundle at packaging time.
frontend:
	cd frontend && npm install --no-audit --no-fund && npx tsc --noEmit && npx tsc --noEmit -p tsconfig.harness.json && npx vite build && npm test

check: frontend lint test copy script-tests notices-check
	$(GO) build ./...

# THIRD_PARTY_NOTICES: the licence texts of the npm packages compiled into
# the bundle, the Go modules compiled into the app, the fonts, the provider
# logos and Microsoft WebView2, generated from the lockfile, go.sum, the
# module cache and the licence files under frontend/src (see
# scripts/notices/generate.mjs). main.go embeds it for Settings -> Licences,
# and every package carries it (the targets below). `make notices` rewrites
# it after a dependency, a font or a logo changes; `make notices-check`,
# part of `make check`, fails while the committed file differs from what
# the tree generates. Needs the frontend's node_modules and the Go module
# cache, and builds the bundle once more with source maps (a minute or two).
notices:
	node scripts/notices/generate.mjs

notices-check:
	node scripts/notices/generate.mjs --check

## workflows-check: no job a fork's pull request can start runs on a
## self-hosted runner without the fork guard, and the check's own tests pass.
## Needs python3 with PyYAML; deliberately not part of `check`, which
## needs no Python. CI runs it as its own step in the `check` job.
workflows-check:
	python3 scripts/ci/workflow_guard.py
	python3 -m unittest discover -s scripts/ci

# --- packaging -------------------------------------------------------------
# Package names follow the shape the download manifest matches — product,
# version, os, arch, lowercase — so a built artifact can reach the download
# page instead of being dropped for its name.
#
# macOS: a universal .app in the two wrappers the download page offers — a
# zip for people who want the bundle, and a disk image with an Applications
# shortcut for people who want to install it the way macOS applications are
# installed.
# Both carry the same bundle: one build, two wrappers.
# hdiutil builds the image (scripts/package-darwin-dmg.sh — create-dmg is not
# on the Mac and would only add a window picture that needs a Finder session)
# and scripts/check-darwin-dmg.sh reads it back, as the Windows target does
# with its installer. This target is ad-hoc signed only. A release that signs
# runs the halves apart and signs between them: build-darwin, then the
# bundle signed, notarized and stapled, then wrap-darwin, so the zip and the
# dmg both carry the stapled bundle, then the dmg signed in turn.
# Output: build/bin/agentry-desktop-$(VERSION)-darwin-universal{.zip,.dmg}
package-darwin: build-darwin wrap-darwin

#
# The agentry CLI: when build/cli holds agentry-darwin (a universal binary,
# which a release puts there), build-darwin carries it as
# Contents/Resources/bin/agentry, re-signs it and then the bundle ad hoc
# (wails seals the bundle, and a file added after that seal fails
# verification), and wrap-darwin reads it back with --cli. The CLI keeps its
# own signing identifier, net.secureagentry.cli, so a release that signs the
# bundle signs it under the same identity as the CLI's own release, and the
# keychain treats the two copies as one program. Without it (a PR build) the
# bundle carries no CLI.
DARWIN_APP := build/bin/agentry-desktop.app

build-darwin:
	wails build $(WAILS_FLAGS) -skipbindings -platform darwin/universal -ldflags '$(LDFLAGS)'
	install -d $(DARWIN_APP)/Contents/Resources
	install -m 0644 THIRD_PARTY_NOTICES $(DARWIN_APP)/Contents/Resources/THIRD_PARTY_NOTICES.txt
ifneq ($(wildcard build/cli/agentry-darwin),)
	install -d $(DARWIN_APP)/Contents/Resources/bin
	install -m 0755 build/cli/agentry-darwin $(DARWIN_APP)/Contents/Resources/bin/agentry
	codesign --force --sign - --identifier net.secureagentry.cli $(DARWIN_APP)/Contents/Resources/bin/agentry
endif
	codesign --force --sign - $(DARWIN_APP)
	codesign --verify --deep --strict $(DARWIN_APP)

wrap-darwin:
	rm -f build/bin/agentry-desktop-$(VERSION)-darwin-universal.zip
	cd build/bin && ditto -c -k --keepParent agentry-desktop.app agentry-desktop-$(VERSION)-darwin-universal.zip
	scripts/package-darwin-dmg.sh $(DARWIN_APP) build/bin/agentry-desktop-$(VERSION)-darwin-universal.dmg
	scripts/check-darwin-dmg.sh $(if $(wildcard build/cli/agentry-darwin),--cli build/cli/agentry-darwin,) build/bin/agentry-desktop-$(VERSION)-darwin-universal.dmg $(DARWIN_APP)

# Drags the dmg's bundle onto a temporary Applications folder over a copy
# that is already there and proves the person's configuration and sign-in
# survive it. macOS only; installs nothing on
# the machine. A release runs it after package-darwin.
smoke-darwin-dmg:
	scripts/acceptance/darwin-dmg-install.sh build/bin/agentry-desktop-$(VERSION)-darwin-universal.dmg

# Applying a newer Agentry Desktop on macOS: the bundle swap, what survives
# it, and the refusal when the copy is somewhere the person cannot replace
# macOS only; installs nothing on the machine and launches nothing. A release runs it after
# package-darwin.
smoke-darwin-selfupdate:
	scripts/acceptance/darwin-selfupdate.sh build/bin/agentry-desktop.app build/bin/agentry-desktop-$(VERSION)-darwin-universal.zip

# Windows: cross-compiled from any host (Wails v2 loads WebView2
# through a pure-Go loader, so windows/amd64 builds with cgo off on Linux
# and macOS alike; the wails CLI supplies the icon, the manifest and the GUI
# subsystem a bare go build would not), then the per-user installer from
# build/windows/installer/project.nsi. -nsis needs makensis on the host
# (`apt-get install nsis` on Debian/Ubuntu, `dnf install mingw32-nsis` on
# Fedora); without it wails only warns, so the mv below is what fails, and
# an installer-less package is not a package. The installer's file name is
# the download manifest's grammar (-setup.exe is the "setup" format); wails
# names it for the architecture and the version comes from here, not from
# wails.json, for the same reason the exe's does. scripts/check-windows-installer.sh reads the result back
# (needs 7z: p7zip-full / p7zip-plugins).
#
# Authenticode is three targets so a release can hand the signing token to
# the one that signs and to nothing else: build-windows (wails: the exe, and
# an unsigned installer as a side effect of -nsis, which also writes the
# WebView2 bootstrapper and wails_tools.nsh), sign-windows (SIGN_CMD on the
# exe BEFORE the installer embeds it, so the check's byte comparison still
# holds, then makensis again with -DSIGN_CMD, under which project.nsi's
# !finalize hooks sign the uninstaller and the installer and overwrite the
# unsigned one), finish-windows (the zip and the setup name, then the check).
# package-windows runs all three; without SIGN_CMD sign-windows is a no-op,
# as in PR builds. The check says nothing about signatures: a release that
# signs reads them back itself. The seam's acceptance, with a stand-in for the
# signer: scripts/acceptance/windows-signing.sh build/bin/agentry-desktop.exe.
# Output: build/bin/agentry-desktop-$(VERSION)-windows-amd64.exe, its zip,
# and build/bin/agentry-desktop-$(VERSION)-windows-amd64-setup.exe.
#
# The agentry CLI: when build/cli holds it (agentry.exe and agentry, which a
# release puts there), the installer carries build/cli/agentry.exe into
# $INSTDIR\bin and puts it on the user Path, the check reads it back with
# --cli, and the Linux packages carry build/cli/agentry as
# /usr/bin/agentry. Without it (a PR build) neither carries a CLI.
package-windows: build-windows sign-windows finish-windows

build-windows:
	wails build $(WAILS_FLAGS) -skipbindings -platform windows/amd64 -nsis -ldflags '$(LDFLAGS)'

sign-windows:
ifneq ($(strip $(SIGN_CMD)),)
	$(SIGN_CMD) build/bin/agentry-desktop.exe
	cd build/windows/installer && makensis -DARG_WAILS_AMD64_BINARY='$(abspath build/bin/agentry-desktop.exe)' -DSIGN_CMD='$(abspath $(SIGN_CMD))' project.nsi
endif

finish-windows:
	cp THIRD_PARTY_NOTICES build/bin/THIRD_PARTY_NOTICES.txt
	cd build/bin && cp agentry-desktop.exe agentry-desktop-$(VERSION)-windows-amd64.exe && rm -f agentry-desktop-$(VERSION)-windows-amd64.zip && zip -q agentry-desktop-$(VERSION)-windows-amd64.zip agentry-desktop.exe THIRD_PARTY_NOTICES.txt
	cd build/bin && mv agentry-desktop-amd64-installer.exe agentry-desktop-$(VERSION)-windows-amd64-setup.exe
	scripts/check-windows-installer.sh $(if $(wildcard build/cli/agentry.exe),--cli build/cli/agentry.exe,) build/bin/agentry-desktop-$(VERSION)-windows-amd64-setup.exe build/bin/agentry-desktop.exe

# Linux: built inside a Debian container that carries the GTK 3 and WebKit2GTK
# 4.1 headers, from the vendored tree, so the host needs Docker and nothing
# else (scripts/build-linux.sh), then wrapped in a .deb and an .rpm from one
# description (build/nfpm/nfpm.yaml via scripts/package-linux.sh; nfpm runs
# through `go run` at a pinned version). The bare binary stays as its own
# artifact.
# Output: build/linux/agentry-desktop-$(VERSION)-linux-amd64{,.deb,.rpm}
package-linux:
	DESKTOP_VERSION='$(VERSION)' EXTRA_LDFLAGS='$(EXTRA_LDFLAGS)' scripts/build-linux.sh
	DESKTOP_VERSION='$(VERSION)' scripts/package-linux.sh

# Installs the two packages in a clean Debian and a clean Fedora container
# and proves: clean install, same binary as the bare artifact,
# declared dependencies exactly the owners of the linked libraries, a launch
# under Xvfb, and a populated home untouched by reinstall and removal. Needs
# Docker; a few minutes. A release runs it after package-linux.
smoke-linux-packages:
	scripts/acceptance/linux-packages.sh build/linux/agentry-desktop-$(VERSION)-linux-amd64.deb build/linux/agentry-desktop-$(VERSION)-linux-amd64.rpm build/linux/agentry-desktop-$(VERSION)-linux-amd64 build/linux/smoke/agentry-desktop-previous.deb build/linux/smoke/agentry-desktop-previous.rpm

# Every platform's app icon, rendered from two committed sources:
# build/appicon.svg, the mark on its tile, and build/appicon-small.svg, the
# bare mark for 16 to 32 pixels. The outputs are committed, since nothing renders them at build time: build/appicon.png
# (1024, which Wails turns into the macOS bundle's iconfile.icns),
# build/windows/icon.ico (16 to 256; Wails uses it as it is) and the Linux
# packages' hicolor sizes under build/nfpm/icons. ImageMagick with its
# librsvg delegate renders them, with -strip so a re-run is byte-identical.
# Run and commit after changing either SVG.
ICON_TMP := build/.icons
app-icons:
	rm -rf $(ICON_TMP) && mkdir -p $(ICON_TMP)
	magick -background none build/appicon.svg -depth 8 -strip build/appicon.png
	for s in 16 24 32; do magick -background none -density 1200 build/appicon-small.svg -resize $${s}x$${s} -depth 8 -strip $(ICON_TMP)/$${s}.png; done
	for s in 48 64 128 256 512; do magick build/appicon.png -resize $${s}x$${s} -depth 8 -strip $(ICON_TMP)/$${s}.png; done
	magick $(ICON_TMP)/16.png $(ICON_TMP)/24.png $(ICON_TMP)/32.png $(ICON_TMP)/48.png $(ICON_TMP)/64.png $(ICON_TMP)/128.png $(ICON_TMP)/256.png build/windows/icon.ico
	for s in 16 32 48 64 128 256 512; do cp $(ICON_TMP)/$${s}.png build/nfpm/icons/agentry-desktop-$${s}.png; done
	rm -rf $(ICON_TMP)

package: package-darwin package-windows package-linux

# A release is a tag v<productVersion>: the tag names the version wails.json
# carries into the macOS bundle and the Windows version-info block, and the
# Makefile stamps the same string into the binary. RELEASE_TAG is the tag
# being cut.
# A tag names productVersion: vX.Y.Z, or a dev prerelease of it,
# vX.Y.Z-dev.N. productVersion itself stays X.Y.Z: the
# Windows version resource and the macOS bundle version take numbers only.
version-check:
	@tag='$(RELEASE_TAG)'; pv=$$(jq -r .info.productVersion wails.json); \
	if [ -z "$$tag" ]; then echo "RELEASE_TAG is not set"; exit 2; fi; \
	v=$${tag#v}; v=$${v%-dev.*}; \
	printf '%s\n' "$$tag" | grep -Ex 'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-dev\.[1-9][0-9]*)?' >/dev/null || { echo "tag $$tag is not vX.Y.Z or vX.Y.Z-dev.N"; exit 1; }; \
	if [ "$$v" != "$$pv" ]; then echo "tag $$tag does not name wails.json productVersion $$pv: bump productVersion in a commit on dev first (a main release takes it by the promotion), then tag"; exit 1; fi; \
	echo "tag $$tag names productVersion $$pv"

clean:
	rm -rf build/bin build/linux build/.icons frontend/dist

# The version the Makefile stamps.
version:
	@echo $(VERSION)
