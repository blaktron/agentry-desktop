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
# graph past vendor/ and asks GitHub for the private modules. -skipbindings is
# not a caller's choice: every wails target below carries it (see `dev`), so
# a by-hand `make package-darwin` cannot regenerate the bindings either
# (agentry-desktop#97, #107). Callers that still pass it are harmless.
WAILS_FLAGS ?=
# SIGN_CMD: a command that Authenticode-signs the one PE file it is given, in
# place (the release workflow passes scripts/sign-windows.sh). Empty means an
# unsigned Windows package, which PR and workstation builds are.
SIGN_CMD ?=

.PHONY: dev build check lint test copy frontend package package-darwin package-windows build-windows sign-windows finish-windows package-linux smoke-linux-packages smoke-darwin-dmg smoke-darwin-selfupdate linux-icons version-check version clean bindings

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
# macOS: a universal .app in the two wrappers the download page offers — a
# zip for people who want the bundle, and a disk image with an Applications
# shortcut for people who want to install it the way macOS applications are
# installed (agentry-desktop#85; plan desktop-installers.md §5, D2).
# Both carry the same bundle: one build, two wrappers.
# hdiutil builds the image (scripts/package-darwin-dmg.sh — create-dmg is not
# on the Mac and would only add a window picture that needs a Finder session)
# and scripts/check-darwin-dmg.sh reads it back, as the Windows target does
# with its installer. Unsigned/un-notarized — see DEVIATIONS.md D-001.
# Output: build/bin/agentry-desktop-$(VERSION)-darwin-universal{.zip,.dmg}
package-darwin:
	wails build $(WAILS_FLAGS) -skipbindings -platform darwin/universal -ldflags '$(LDFLAGS)'
	cd build/bin && ditto -c -k --keepParent agentry-desktop.app agentry-desktop-$(VERSION)-darwin-universal.zip
	scripts/package-darwin-dmg.sh build/bin/agentry-desktop.app build/bin/agentry-desktop-$(VERSION)-darwin-universal.dmg
	scripts/check-darwin-dmg.sh build/bin/agentry-desktop-$(VERSION)-darwin-universal.dmg build/bin/agentry-desktop.app

# Drags the dmg's bundle onto a temporary Applications folder over a copy
# that is already there and proves the person's configuration and sign-in
# survive it (agentry-desktop#85; plan §8). macOS only; installs nothing on
# the machine. The release workflow runs it after package-darwin.
smoke-darwin-dmg:
	scripts/acceptance/darwin-dmg-install.sh build/bin/agentry-desktop-$(VERSION)-darwin-universal.dmg

# Applying a newer Agentry Desktop on macOS: the bundle swap, what survives
# it, and the refusal when the copy is somewhere the person cannot replace
# (agentry-desktop#88; plan §7, §8). macOS only; installs nothing on the
# machine and launches nothing. The release workflow runs it after
# package-darwin.
smoke-darwin-selfupdate:
	scripts/acceptance/darwin-selfupdate.sh build/bin/agentry-desktop.app build/bin/agentry-desktop-$(VERSION)-darwin-universal.zip

# Windows: cross-compiled from any host (DEV-097: Wails v2 loads WebView2
# through a pure-Go loader, so windows/amd64 builds with cgo off on Linux
# and macOS alike; the wails CLI supplies the icon, the manifest and the GUI
# subsystem a bare go build would not), then the per-user installer from
# build/windows/installer/project.nsi (agentry-desktop#83; plan
# desktop-installers.md §5, D11). -nsis needs makensis on the host
# (`apt-get install nsis` on Debian/Ubuntu, `dnf install mingw32-nsis` on
# Fedora); without it wails only warns, so the mv below is what fails, and
# an installer-less package is not a package. The installer's file name is
# the download manifest's grammar (agentry-web scripts/release/manifest.py:
# -setup.exe is the "setup" format); wails names it for the architecture and
# the version comes from here, not from wails.json, for the same reason the
# exe's does. scripts/check-windows-installer.sh reads the result back
# (needs 7z: p7zip-full / p7zip-plugins).
#
# Authenticode (agentry-desktop#54) is three targets so the release can hand
# the signing token to the one that signs and to nothing else: build-windows
# (wails: the exe, and an unsigned installer as a side effect of -nsis, which
# also writes the WebView2 bootstrapper and wails_tools.nsh), sign-windows
# (SIGN_CMD on the exe BEFORE the installer embeds it, so the check's byte
# comparison still holds, then makensis again with -DSIGN_CMD, under which
# project.nsi's !finalize hooks sign the uninstaller and the installer and
# overwrite the unsigned one), finish-windows (the zip and the setup name,
# then the check — --signed with a report when SIGN_CMD is set). package-windows
# runs all three; without SIGN_CMD sign-windows is a no-op and the check
# reports the package unsigned, as PR builds are. With SIGN_CMD also
# build/bin/windows-signing.txt, the checker's report (cn=, timestamp=,
# root=) that the release writes into BUILD-INFO. The seam's acceptance,
# with a throwaway CA in place of the real key:
# scripts/acceptance/windows-signing.sh build/bin/agentry-desktop.exe.
# Output: build/bin/agentry-desktop-$(VERSION)-windows-amd64.exe, its zip,
# and build/bin/agentry-desktop-$(VERSION)-windows-amd64-setup.exe.
package-windows: build-windows sign-windows finish-windows

build-windows:
	wails build $(WAILS_FLAGS) -skipbindings -platform windows/amd64 -nsis -ldflags '$(LDFLAGS)'

sign-windows:
ifneq ($(strip $(SIGN_CMD)),)
	$(SIGN_CMD) build/bin/agentry-desktop.exe
	cd build/windows/installer && makensis -DARG_WAILS_AMD64_BINARY='$(abspath build/bin/agentry-desktop.exe)' -DSIGN_CMD='$(abspath $(SIGN_CMD))' project.nsi
endif

finish-windows:
	cd build/bin && cp agentry-desktop.exe agentry-desktop-$(VERSION)-windows-amd64.exe && rm -f agentry-desktop-$(VERSION)-windows-amd64.zip && zip -q agentry-desktop-$(VERSION)-windows-amd64.zip agentry-desktop.exe
	cd build/bin && mv agentry-desktop-amd64-installer.exe agentry-desktop-$(VERSION)-windows-amd64-setup.exe
	scripts/check-windows-installer.sh $(if $(strip $(SIGN_CMD)),--signed --report build/bin/windows-signing.txt,) build/bin/agentry-desktop-$(VERSION)-windows-amd64-setup.exe build/bin/agentry-desktop.exe

# Linux: built inside a Debian container that carries the GTK 3 and WebKit2GTK
# 4.1 headers, from the vendored tree, so the host needs Docker and nothing
# else (scripts/build-linux.sh), then wrapped in a .deb and an .rpm from one
# description (build/nfpm/nfpm.yaml via scripts/package-linux.sh; nfpm runs
# through `go run` at a pinned version). The bare binary stays as its own
# artifact (agentry-desktop#84; plan desktop-installers.md §5).
# Output: build/linux/agentry-desktop-$(VERSION)-linux-amd64{,.deb,.rpm}
package-linux:
	DESKTOP_VERSION='$(VERSION)' EXTRA_LDFLAGS='$(EXTRA_LDFLAGS)' scripts/build-linux.sh
	DESKTOP_VERSION='$(VERSION)' scripts/package-linux.sh

# Installs the two packages in a clean Debian and a clean Fedora container
# and proves what #84 asks: clean install, same binary as the bare artifact,
# declared dependencies exactly the owners of the linked libraries, a launch
# under Xvfb, and a populated home untouched by reinstall and removal. Needs
# Docker; a few minutes. The release workflow runs it after package-linux.
smoke-linux-packages:
	scripts/acceptance/linux-packages.sh build/linux/agentry-desktop-$(VERSION)-linux-amd64.deb build/linux/agentry-desktop-$(VERSION)-linux-amd64.rpm build/linux/agentry-desktop-$(VERSION)-linux-amd64 build/linux/smoke/agentry-desktop-previous.deb build/linux/smoke/agentry-desktop-previous.rpm

# The Linux packages' icons: build/appicon.png at the hicolor sizes, committed
# under build/nfpm/icons (ImageMagick renders them; nothing does at build
# time). Run and commit after changing the app icon.
linux-icons:
	for s in 16 32 48 64 128 256 512; do magick build/appicon.png -resize $${s}x$${s} build/nfpm/icons/agentry-desktop-$${s}.png; done

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
