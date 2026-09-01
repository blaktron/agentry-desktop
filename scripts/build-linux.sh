#!/usr/bin/env bash
# Build the Agentry Desktop Linux binary inside a container.
# Run on the docker host from the extracted source directory.
# Prep on the dev machine first: `go mod vendor` and `cd frontend && npm run build`,
# then ship the tree (minus .git, node_modules, build/bin) to the host.
set -euo pipefail
cd "$(dirname "$0")"

docker run --rm -v "$PWD:/src" -w /src golang:1.27-bookworm bash -ec '
  apt-get update -qq >/dev/null 2>&1
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
    libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config xvfb dbus >/dev/null 2>&1
  VERSION="${DESKTOP_VERSION:-0.0.0-dev}"
  go build -mod=vendor -tags "desktop production webkit2_41" \
    -ldflags "-w -s -X github.com/blaktron/agentry-desktop/internal/version.Version=${VERSION}" \
    -o build/linux/agentry-desktop .
  echo BUILD-OK
  ls -la build/linux/
  # Smoke: the GUI must start and stay alive under a virtual display.
  # timeout killing it (exit 124) means it ran without crashing.
  timeout 6 xvfb-run -a ./build/linux/agentry-desktop >/tmp/run.log 2>&1 && ec=$? || ec=$?
  echo "xvfb exit: ${ec}"
  tail -3 /tmp/run.log || true
'
