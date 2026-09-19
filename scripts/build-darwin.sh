#!/usr/bin/env bash
# Build the universal macOS application on the Mac build host, from here.
#
#   scripts/build-darwin.sh [user@host]        (default: agentry@maxbookpro)
#
# macOS is the one target that cannot be cross-built: the Wails macOS front
# end is Objective-C behind cgo and needs clang and Apple's SDK. So this
# script runs on the workstation, prepares the tree, ships it to a Mac that
# carries the toolchain (plan agentry-notes/plans/macos-build-host.md, M1),
# builds there with the Makefile's own package-darwin target (WAILS_FLAGS=-m,
# see the Makefile for why), and brings the zip back. The Mac never needs a
# GitHub credential: the private modules are vendored here, where the
# credential helper lives, and the vendor directory travels with the tree (go
# builds in vendor mode on its own when vendor/ is present). The .git
# directory does not travel, so the version is computed here and passed in,
# and the build needs no -buildvcs=false.
#
# Environment:
#   AGENTRY_DEFAULT_REGISTRY  registry URL the built app connects to when the
#                             user configures nothing (empty: the source default,
#                             the production site). The downloads builder sets
#                             the same variable for the same reason.
#   SSHPASS                   read by sshpass -e; if unset, ssh asks (or uses a key).
#   BUILD_DARWIN_SMOKE        1 (default) to launch the built app on the Mac for a
#                             few seconds after the build; 0 to skip.
#
# Output: build/bin/agentry-desktop-<version>-darwin-universal.zip locally,
# its SHA-256 on stdout, and the lipo architectures of the binary inside.
set -euo pipefail
cd "$(dirname "$0")/.."

HOST=${1:-agentry@maxbookpro}
VERSION=$( (git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev) | sed "s/^v//")
SMOKE=${BUILD_DARWIN_SMOKE:-1}

# sshpass -e when a password is in the environment, plain ssh otherwise.
if [ -n "${SSHPASS:-}" ] && command -v sshpass >/dev/null; then
  SSH=(sshpass -e ssh -o StrictHostKeyChecking=accept-new)
  SCP=(sshpass -e scp -o StrictHostKeyChecking=accept-new)
else
  SSH=(ssh -o StrictHostKeyChecking=accept-new)
  SCP=(scp -o StrictHostKeyChecking=accept-new)
fi

echo "== vendoring modules here (GOWORK=off, matching CI)"
GOWORK=off go mod vendor

# An absolute remote path: scp in SFTP mode does not expand ~ on every server.
REMOTE_DIR="$("${SSH[@]}" "$HOST" 'echo "$HOME"')/build/agentry-desktop"
echo "== shipping the tree to $HOST:$REMOTE_DIR (no .git, no node_modules, no build/bin)"
"${SSH[@]}" "$HOST" "rm -rf $REMOTE_DIR && mkdir -p $REMOTE_DIR"
tar --exclude=.git --exclude=node_modules --exclude=build/bin --exclude=frontend/dist \
    --exclude=build/linux -cf - . | "${SSH[@]}" "$HOST" "tar -C $REMOTE_DIR -xf -"

echo "== building on $HOST"
EXTRA_LDFLAGS="-w -s"
if [ -n "${AGENTRY_DEFAULT_REGISTRY:-}" ]; then
  EXTRA_LDFLAGS="$EXTRA_LDFLAGS -X github.com/blaktron/agentry-desktop/internal/settings.DefaultRegistryURL=${AGENTRY_DEFAULT_REGISTRY}"
fi
# The remote shell is zsh; ~/.zshenv puts Go, Node and the Wails CLI on PATH
# for non-interactive sessions (plan M1). VERSION overrides the Makefile's
# git describe, which has no .git to read; WAILS_FLAGS=-m is explained there.
"${SSH[@]}" "$HOST" "cd $REMOTE_DIR && \
  make package-darwin WAILS_FLAGS='-m' VERSION='$VERSION' EXTRA_LDFLAGS='$EXTRA_LDFLAGS' && \
  cd build/bin && lipo -info agentry-desktop.app/Contents/MacOS/agentry-desktop && \
  shasum -a 256 agentry-desktop-$VERSION-darwin-universal.zip"

ZIP="agentry-desktop-$VERSION-darwin-universal.zip"
mkdir -p build/bin
echo "== copying $ZIP back"
"${SCP[@]}" -q "$HOST:$REMOTE_DIR/build/bin/$ZIP" "build/bin/$ZIP"
sha256sum "build/bin/$ZIP"

if [ "$SMOKE" = "1" ]; then
  echo "== launch smoke on $HOST (needs a logged-in GUI session for the user)"
  # The zip is unpacked beside the bundle wails wrote and the binary inside
  # is started directly, so its exit code is ours to read: it must still be
  # running after 5 s, or its exit code is reported. A user with no console
  # session on the Mac cannot open a window (WindowServer refuses), so that
  # case is reported as skipped, not as a failure of the build.
  "${SSH[@]}" "$HOST" "cd $REMOTE_DIR/build/bin && rm -rf smoke && mkdir smoke && ditto -x -k $ZIP smoke && \
    if launchctl print gui/\$(id -u) >/dev/null 2>&1; then \
      ./smoke/agentry-desktop.app/Contents/MacOS/agentry-desktop >smoke/run.log 2>&1 & pid=\$!; sleep 5; \
      if kill -0 \$pid 2>/dev/null; then echo 'SMOKE-OK: agentry-desktop running after 5 s'; kill \$pid; wait \$pid 2>/dev/null; \
      else wait \$pid; ec=\$?; echo \"SMOKE-FAIL: agentry-desktop exited with \$ec before 5 s\"; tail -5 smoke/run.log; exit 1; fi; \
    else echo 'SMOKE-SKIPPED: no GUI session for this user on the Mac (launchctl print gui/uid)'; fi"
fi
echo "BUILD-OK $ZIP"
