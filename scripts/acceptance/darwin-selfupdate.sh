#!/usr/bin/env bash
# Proves what applying a newer Agentry Desktop must do on macOS, and what it
# must refuse.
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
# (a CI runner has none). What that leaves unproven is stated at the
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

# bounded runs a command for at most 20 seconds. Gatekeeper evaluates a
# Developer ID bundle when anything inside it is executed, and where the
# evaluation needs a person (a prompt in the console session this runner has
# none of) the exec waits for ever; a bound turns that into a failure that
# says so.
bounded() { perl -e 'alarm 20; exec @ARGV' -- "$@"; }

echo "== 1. a copy installed where the person can replace it"
ditto "$old_app" "$apps/$(basename "$old_app")"
installed="$apps/$(basename "$old_app")"
echo "   $(basename "$installed") is in the Applications folder"
# The agentry command, when the bundle carries one: a link to it, as Install
# the agentry command makes in ~/.local/bin, which the update must keep
# working. It is run now, while the bundle's seal is intact: the marker
# written next breaks the seal, and a Developer ID bundle whose seal is broken
# makes Gatekeeper ask a person before anything inside it runs.
carried="$installed/Contents/Resources/bin/agentry"
command_link=""
if [ -e "$carried" ]; then
  mkdir -p "$work/bin"
  command_link="$work/bin/agentry"
  ln -s "$carried" "$command_link"
  bounded "$command_link" --version >/dev/null 2>&1 || fail "the agentry command does not run before the update (or did not answer within 20 s)"
  echo "   the agentry command links to the CLI the bundle carries"
fi
# A file only this release has, so the swap can be told from a merge.
printf 'only in the release being replaced\n' > "$installed/Contents/Resources/only-in-the-old-one.txt"

echo "== 2. the person's configuration, as it would be after using it"
printf 'defaultRegistry: https://dev.example.net\nonboarded: true\n' > "$AGENTRY_CONFIG_DIR/desktop.yaml"
printf 'installs:\n  - name: brief\n' > "$AGENTRY_CONFIG_DIR/installed.yaml"
printf 'dev.example.net pin\n' > "$AGENTRY_CONFIG_DIR/keys/dev.example.net.pub"
printf 'defaultAdapter: docker-agent\n' > "$AGENTRY_RUNNER_HOME/policy.yaml"
snapshot() { (cd "$work" && find config runner -type f -exec shasum -a 256 {} + | sort); }
before=$(snapshot)

echo "== 3. the swap, by the code that ships"
# go test, not a re-implementation. An earlier version of this script did its
# own ditto and its own two renames, and would have passed with
# apply_darwin.go deleted — which makes it a description of the design rather
# than a test of it. internal/selfupdate's darwin tests drive the real
# darwinApplier against a real ad-hoc-signed bundle: the swap, that it
# replaces rather than merges, that nothing is left in the Applications
# folder, that an unwritable parent is refused, that an archive holding two
# applications is refused, and that quitting does not relaunch.
go test ./internal/selfupdate/ -run TestDarwinApplier -count=1 -v 2>&1 \
  | sed -n 's/^--- /   /p' || fail "the applier's own tests did not pass"

# The swap once more, here, so that steps 5 and 6 observe a real application
# being replaced beside a real configuration. A second observation of the same
# operation, not the proof of it — step 3 is the proof.
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

if [ -n "$command_link" ]; then
  echo "== 5b. the agentry command runs the CLI the new bundle carries"
  [ -e "$installed/Contents/Resources/bin/agentry" ] || fail "the new bundle carries no agentry CLI"
  v=$(bounded "$command_link" --version 2>&1) || fail "the agentry command does not run after the update (or did not answer within 20 s): $v"
  want=$("$installed/Contents/Resources/bin/agentry" --version 2>&1)
  [ "$v" = "$want" ] || fail "the agentry command says '$v', the new bundle's CLI '$want'"
  echo "   ${v%%$'\n'*}"
fi

echo "== 6. nothing is left beside the application"
outside=$(cd "$apps" && find . -name '*.app' -prune -o -print | grep -v '^\.$' || true)
[ -z "$outside" ] || { printf '%s\n' "$outside" >&2; fail "the swap left something beside the bundle"; }

# The unwritable parent, the two-bundle archive and "do not relaunch on quit"
# are covered by the Go tests step 3 ran, against the real applier. They are
# not restated in bash here: a bash restatement would prove nothing the Go
# tests did not, and would be one more thing that could pass while the code
# was wrong.

echo
echo "darwin-selfupdate: ok — internal/selfupdate's darwin tests passed against the REAL applier (swap, replace not merge, nothing left behind, unwritable parent refused, two-bundle archive refused, no relaunch when not asked), and a swap in a real Applications folder left the settings, the installed agents, the pinned key and the runner policy byte-identical$( [ -n "$command_link" ] && printf ', and the agentry command, a link into the bundle, runs the new bundle'"'"'s CLI' )."
echo "NOT proven here: the relaunch. Applying ends with 'open -n' on the new bundle, which needs a console session; a CI runner has none. The Go test observes the exec that WOULD have run, not the window that would have opened."
