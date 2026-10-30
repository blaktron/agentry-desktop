#!/usr/bin/env bash
# Build the Agentry Desktop Linux binary inside a container.
#
#   make package-linux            (sets DESKTOP_VERSION from git describe)
#   scripts/build-linux.sh        (DESKTOP_VERSION defaults to 0.0.0-dev)
#
# The GTK 4 and WebKitGTK 6.0 headers the Linux front end links against live
# in the container, not on the host, so the host needs Docker and nothing
# else: the CI runners carry no GTK, and neither does a developer's machine. The
# private modules are vendored here every time (the credential lives on the
# host, never in the container, and a stale vendor tree would link
# yesterday's modules without a word); the frontend bundle is reused when it
# exists, because it is slow and the Makefile's other targets reuse it too —
# rm -rf frontend/dist to rebuild it. The container builds in vendor mode
# with the version and the extra linker flags passed in, and hands its
# output back owned by the invoking user, not root.
#
# Environment:
#   DESKTOP_VERSION   the version stamped into the binary and its file name
#                     (default 0.0.0-dev; the Makefile passes git describe).
#   EXTRA_LDFLAGS     appended to the linker flags, as for the Makefile's
#                     other targets; nothing is stripped unless it says -w -s
#                     (a release does).
#
# Output: build/linux/agentry-desktop-<version>-linux-amd64 and a launch
# smoke under a virtual display.
set -euo pipefail
cd "$(dirname "$0")/.."   # the repo root — go.mod lives here, and it is what mounts as /src

VERSION="${DESKTOP_VERSION:-0.0.0-dev}"
OUT="agentry-desktop-${VERSION}-linux-amd64"

echo "== vendoring modules here (GOWORK=off, matching CI)"
GOWORK=off go mod vendor
# -s: a non-empty index.html; an empty stand-in is not a bundle.
if [ -s frontend/dist/index.html ]; then
  echo "== reusing frontend/dist (rm -rf frontend/dist to rebuild it)"
else
  echo "== building the frontend bundle here"
  (cd frontend && npm ci --no-audit --no-fund && npx vite build)
fi

# The build container's Go is go.mod's exact version: a floating
# golang:1.27-bookworm could be an older patch release than go.mod names,
# and with GOTOOLCHAIN=local that refuses to build.
GO_VERSION=$(awk '$1 == "go" {print $2; exit}' go.mod)
[ -n "$GO_VERSION" ] || { echo "build-linux: no go version in go.mod" >&2; exit 1; }

# label=disable: on a host whose Docker daemon runs with SELinux (the CI
# runner host), a bind mount is otherwise unwritable from inside.
# The container runs as root (apt needs it) and chowns its output to the
# invoking user on the way out, so the host can clean build/linux afterwards.
docker run --rm --security-opt label=disable -v "$PWD:/src" -w /src \
  -e DESKTOP_VERSION="$VERSION" -e EXTRA_LDFLAGS="${EXTRA_LDFLAGS:-}" -e OUT="$OUT" \
  -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
  "golang:${GO_VERSION}-trixie" bash -ec '
  trap "chown -R ${HOST_UID}:${HOST_GID} build/linux 2>/dev/null || true" EXIT
  apt-get update -qq >/dev/null 2>&1
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
    libgtk-4-dev libwebkitgtk-6.0-dev pkg-config xvfb dbus >/dev/null 2>&1
  mkdir -p build/linux
  # -buildvcs=false: the bind-mounted .git belongs to another uid, which git
  # inside the container refuses to read; the version is stamped by ldflags.
  go build -mod=vendor -trimpath -buildvcs=false -tags production \
    -ldflags "-X github.com/blaktron/agentry-desktop/internal/version.Version=${DESKTOP_VERSION} ${EXTRA_LDFLAGS}" \
    -o "build/linux/${OUT}" .
  echo BUILD-OK
  ls -la build/linux/
  # Smoke: the GUI must start and stay alive under a virtual display.
  # timeout killing it (exit 124) means it ran without crashing.
  timeout 6 xvfb-run -a "./build/linux/${OUT}" >/tmp/run.log 2>&1 && ec=$? || ec=$?
  echo "xvfb exit: ${ec}"
  tail -3 /tmp/run.log || true
  if [ "${ec}" != 124 ]; then echo "SMOKE-FAIL: exited with ${ec} before the timeout"; exit 1; fi
  echo SMOKE-OK
'
