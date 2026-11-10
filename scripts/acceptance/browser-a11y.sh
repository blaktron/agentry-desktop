#!/usr/bin/env bash
# The browser checks over the design harness (frontend/e2e): axe with colour
# contrast in light, dark and every theme, the same under forced colours, and
# the keyboard-only flows. They run in Playwright's own image, at the
# version the lockfile pins, so no browser is installed on the host; the
# harness is served from inside the container.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
FRONTEND="$ROOT/frontend"
PW_VERSION="$(node -p 'require(process.argv[1]).packages["node_modules/@playwright/test"].version' "$FRONTEND/package-lock.json")"
IMAGE="${PLAYWRIGHT_IMAGE:-mcr.microsoft.com/playwright:v$PW_VERSION-noble}"
NAME="agentry-desktop-a11y-$$"
# The harness's port: HARNESS_PORT, else one the system has free now, so
# two runs on one host (the runners share its network) do not collide.
PORT="${HARNESS_PORT:-$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')}"

[ -d "$FRONTEND/node_modules/@playwright/test" ] || { echo "run npm ci in frontend first"; exit 1; }

cleanup() { docker rm -fv "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== browser accessibility checks (Chromium, Playwright $PW_VERSION) =="
# label=disable: on a host whose Docker daemon enforces SELinux the mount is
# otherwise unreadable to the container.
docker run --rm --init --name "$NAME" --network host --shm-size=512m \
  --security-opt label=disable \
  --user "$(id -u):$(id -g)" -e HOME=/tmp -e CI="${CI:-}" -e HARNESS_PORT="$PORT" \
  -v "$FRONTEND:$FRONTEND" -w "$FRONTEND" "$IMAGE" \
  ./node_modules/.bin/playwright test -c e2e/playwright.config.ts "$@"
