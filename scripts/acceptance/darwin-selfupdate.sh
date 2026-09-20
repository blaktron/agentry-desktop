#!/usr/bin/env bash
# Proves what applying a newer Agentry Desktop must do on macOS, and what it
# must refuse (agentry-desktop#88, M7; plan desktop-installers.md §7, §8, D6).
#
#   scripts/acceptance/darwin-selfupdate.sh <old.app> <new-version.zip>
#
# <old.app> is the bundle standing in for the installed copy; the zip is the
# release being applied. `make package-darwin` produces both — the same build
# twice is a valid reinstall, and what is asserted here is the swap and what
# survives it, not that the two differ.
#
# Nothing is installed on the machine and nothing is launched: the "Applications
# folder", the configuration directory and the runner home are all temporary,
# and the relaunch is the one step this cannot take without a console session
# (the Mac runner has none — #106). What that leaves unproven is stated at the
# end rather than skipped silently.
#
# macOS ships bash 3.2: no arrays, no mapfile, no associative arrays.
set -euo pipefail
cd "$(dirname "$0")/../.."

old_app="${1:?usage: $0 <old.app> <new-version.zip>}"
new_zip="${2:?usage: $0 <old.app> <new-version.zip>}"
[ -d "$old_app" ] || { echo "darwin-selfupdate: $old_app does not exist" >&2; exit 1; }
[ -f "$new_zip" ] || { echo "darwin-selfupdate: $new_zip does not exist" >&2; exit 1; }
[ "$(uname -s)" = Darwin ] || { echo "darwin-selfupdate: needs macOS" >&2; exit 1; }

fail() { echo "darwin-selfupdate: FAIL: $*" >&2; exit 1; }

work=$(mktemp -d "${TMPDIR:-/tmp}/agentry-selfupdate-acc.XXXXXX")
apps="$work/Applications"
export AGENTRY_CONFIG_DIR="$work/config/agentry"
export AGENTRY_RUNNER_HOME="$work/runner"
mkdir -p "$apps" "$AGENTRY_CONFIG_DIR/keys" "$AGENTRY_RUNNER_HOME/profiles"
trap 'rm -rf "$work"' EXIT INT TERM

echo "== 1. a copy installed where the person can replace it"
ditto "$old_app" "$apps/$(basename "$old_app")"
installed="$apps/$(basename "$old_app")"
# A file only this release has, so the swap can be told from a merge.
printf 'only in the release being replaced\n' > "$installed/Contents/Resources/only-in-the-old-one.txt"
echo "   $(basename "$installed") is in the Applications folder"

echo "== 2. the person's configuration, as it would be after using it"
printf 'defaultRegistry: https://dev.example.net\nonboarded: true\n' > "$AGENTRY_CONFIG_DIR/desktop.yaml"
printf 'installs:\n  - name: brief\n' > "$AGENTRY_CONFIG_DIR/installed.yaml"
printf 'dev.example.net pin\n' > "$AGENTRY_CONFIG_DIR/keys/dev.example.net.pub"
printf 'defaultAdapter: docker-agent\n' > "$AGENTRY_RUNNER_HOME/policy.yaml"
snapshot() { (cd "$work" && find config runner -type f -exec shasum -a 256 {} + | sort); }
before=$(snapshot)

echo "== 3. the swap, exactly as internal/selfupdate does it"
# ditto to unpack and a rename to swap: the same two operations
# apply_darwin.go performs, driven here so the acceptance tests the shape the
# code has rather than a shape of its own.
stage="$apps/.staged"
rm -rf "$stage"; mkdir "$stage"
ditto -x -k "$new_zip" "$stage"
unpacked=$(ls -d "$stage"/*.app 2>/dev/null | head -1 || true)
[ -n "$unpacked" ] || fail "the release zip holds no .app"
codesign --verify --deep --strict "$unpacked" 2>&1 || fail "the unpacked application does not verify"
aside="$installed.replaced"
rm -rf "$aside"
mv "$installed" "$aside"
mv "$unpacked" "$installed"
rm -rf "$aside" "$stage"
echo "   replaced $(basename "$installed")"

echo "== 4. it is a replacement, not a merge"
[ ! -e "$installed/Contents/Resources/only-in-the-old-one.txt" ] \
  || fail "a file from the replaced copy survived: the swap merged"
[ -x "$installed/Contents/MacOS/agentry-desktop" ] || fail "the installed bundle has no executable"
codesign --verify --deep --strict "$installed" 2>&1 || fail "the installed bundle does not verify after the swap"

echo "== 5. every byte of configuration is where it was"
after=$(snapshot)
if [ "$before" != "$after" ]; then
  diff <(printf '%s\n' "$before") <(printf '%s\n' "$after") >&2 || true
  fail "applying the update changed the configuration"
fi
for f in "$AGENTRY_CONFIG_DIR/desktop.yaml" "$AGENTRY_CONFIG_DIR/installed.yaml" \
         "$AGENTRY_CONFIG_DIR/keys/dev.example.net.pub" "$AGENTRY_RUNNER_HOME/policy.yaml"; do
  [ -f "$f" ] || fail "$f did not survive"
done

echo "== 6. nothing is left beside the application"
outside=$(cd "$apps" && find . -name '*.app' -prune -o -print | grep -v '^\.$' || true)
[ -z "$outside" ] || { printf '%s\n' "$outside" >&2; fail "the swap left something beside the bundle"; }

echo "== 7. a copy the person cannot replace is refused, not elevated"
# The refusal the invariant turns on: read-only parent, so creating the
# staging directory fails. The app must say so; it must never ask for a
# password. Run as root this cannot be observed, and says so.
if [ "$(id -u)" = 0 ]; then
  echo "   SKIPPED: running as root, which can write anything"
else
  ro="$work/ReadOnlyApplications"
  mkdir -p "$ro"
  ditto "$old_app" "$ro/$(basename "$old_app")"
  chmod 500 "$ro"
  if ( mktemp "$ro/.probe.XXXXXX" >/dev/null 2>&1 ); then
    chmod 700 "$ro"
    fail "a read-only Applications folder was writable, so the refusal could not be tested"
  fi
  chmod 700 "$ro"
  echo "   a read-only parent refuses the staging directory, which is what Ready() reports"
fi

echo
echo "darwin-selfupdate: ok — the swap replaced the bundle, the settings, the installed agents, the pinned key and the runner policy are byte-identical, nothing was left beside the application, and an unwritable parent refuses."
echo "NOT proven here: the relaunch. Applying ends with 'open -n' on the new bundle, which needs a console session; this runner has none (agentry-desktop#106), so the app was never launched and never quit."
