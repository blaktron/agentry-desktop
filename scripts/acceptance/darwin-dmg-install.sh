#!/usr/bin/env bash
# Proves the promise the dmg makes about an upgrade: dragging the bundle onto
# Applications over a copy that is already there replaces the application and
# leaves every byte of the person's configuration, and their sign-in, where
# they were: an upgrade never touches ~/.config/agentry, ~/.agentry/runner
# or the keychain.
#
#   scripts/acceptance/darwin-dmg-install.sh <out.dmg> [previous.dmg]
#
# With one image, the same image installs twice — a valid first run followed
# by an upgrade to the same version, which is what a reinstall is; step 1
# plants a file in the first copy that the second does not have, so the
# difference a real previous release would make is modelled rather than
# skipped, and the release workflow runs it this way. With two, the first is the release already
# installed and the second the upgrade; pass a previous release's image when
# one exists to have the real thing.
#
# Nothing is installed on the machine: the "Applications" folder, the
# configuration directory and the runner home are all temporary directories,
# the app is never launched, and the keychain item goes into a keychain this
# script creates and deletes — the machine's own keychains are neither read
# nor added to the search list, so a real sign-in is never touched.
#
# The drag itself is `ditto`, which is what Finder's copy is; the existing
# bundle is removed first, because Finder's Replace replaces a bundle rather
# than merging into it. An upgrade that merged would leave a file from the
# old version behind, which step 5 plants and reads back.
#
# What this does NOT check, because the drag structurally cannot do it: that
# the image installs nothing beside the bundle. Only the .app is copied off
# the image, and nothing on it runs, so the claim belongs to
# scripts/check-darwin-dmg.sh, which asserts the image holds the bundle and
# the Applications shortcut and nothing else.
#
# Be clear about what the configuration comparison is worth. Installing from a
# dmg is a file copy with no script to run, so the person's files cannot move
# unless the copy reaches outside the Applications folder: this proves that it
# does not, which is the promise, and it is a weaker statement than a package
# manager's postinstall leaving a home alone. Its value is as a regression
# guard — the day this ships a dmg with an installer payload in it, or the day
# `drag` below stops modelling Finder's Replace (step 5), the check goes red.
# The bundle installed from the image is then launched (step 6) where a
# console session allows it, which is the one part of this that exercises the
# built application rather than the copy.
#
# The keychain half runs everywhere, including on a runner that has no
# console session and so no login keychain in its search list at all: a
# keychain this script creates and unlocks is named explicitly, which needs
# no session. What that proves is the claim being made
# — a drag of the bundle disturbs no keychain item — and it proves it without
# putting a real credential anywhere near the test. It is NOT a check of the
# login keychain, which is what internal/settings actually uses.
#
# macOS ships bash 3.2, so: no arrays, no associative arrays, no mapfile.
set -euo pipefail
cd "$(dirname "$0")/../.."

dmg="${1:?usage: $0 <out.dmg> [previous.dmg]}"
previous="${2:-$dmg}"
[ -f "$dmg" ] || { echo "darwin-dmg-install: $dmg does not exist" >&2; exit 1; }
[ -f "$previous" ] || { echo "darwin-dmg-install: $previous does not exist" >&2; exit 1; }
[ "$(uname -s)" = Darwin ] || { echo "darwin-dmg-install: needs macOS (hdiutil)" >&2; exit 1; }

fail() { echo "darwin-dmg-install: FAIL: $*" >&2; exit 1; }

# A service and a secret of this script's own: internal/settings uses the
# service "agentry" in the login keychain, and an acceptance run must not be
# able to read or delete a real sign-in. The password below unlocks a keychain
# that exists for the length of this script and holds one made-up string; it
# is not a credential.
KEYCHAIN_PASSWORD="acceptance-only-not-a-secret"
KEYCHAIN_SERVICE="agentry-dmg-acceptance"
KEYCHAIN_ACCOUNT="dev.example.net"
KEYCHAIN_SECRET="token-for-the-acceptance"

work=$(mktemp -d "${TMPDIR:-/tmp}/agentry-dmg-acc.XXXXXX")
apps="$work/Applications"
mnt="$work/mnt"
# The keychain is a FILE inside this run's own directory, not a name in
# ~/Library/Keychains: `security` takes a path, so two of these can run at
# once without one deleting the other's, and a run that is killed before its
# trap fires leaves the file in TMPDIR rather than in the user's keychain
# folder, where it would accumulate unnoticed.
KEYCHAIN="$work/acceptance.keychain"
export AGENTRY_CONFIG_DIR="$work/config/agentry"
export AGENTRY_RUNNER_HOME="$work/runner"
mkdir -p "$apps" "$mnt" "$AGENTRY_CONFIG_DIR/keys" "$AGENTRY_RUNNER_HOME/profiles"

detach() { hdiutil detach "$mnt" -quiet 2>/dev/null || hdiutil detach "$mnt" -force -quiet 2>/dev/null || true; }
cleanup() {
  detach
  security delete-keychain "$KEYCHAIN" >/dev/null 2>&1 || true
  rm -rf "$work"
}
# INT and TERM as well as EXIT: a cancelled CI job on a persistent machine must
# not leave an image attached at a path nothing will detach again.
trap cleanup EXIT INT TERM

# Mounts <dmg>, copies its bundle over whatever is in $apps, detaches, and
# leaves the destination path in $dragged. Not a command substitution: a
# failure inside one would be a failure inside a subshell, and the mount it
# left behind would outlive the trap.
dragged=""
launch=skipped
drag() {
  local image="$1" bundle
  hdiutil attach "$image" -nobrowse -readonly -noautoopen -mountpoint "$mnt" >/dev/null || fail "$(basename "$image") did not mount"
  bundle=$(ls -d "$mnt"/*.app 2>/dev/null | head -1 || true)
  [ -n "$bundle" ] || fail "$(basename "$image") holds no .app"
  dragged="$apps/$(basename "$bundle")"
  rm -rf "$dragged"
  ditto "$bundle" "$dragged"
  detach
}

echo "== 1. the release that is already installed"
drag "$previous"
installed="$dragged"
[ -x "$installed/Contents/MacOS/agentry-desktop" ] || fail "the dragged bundle has no executable"
# A file the next release does not have. Finder's Replace replaces the whole
# bundle, so this must be gone after the upgrade; a drag that merged would
# leave it, and leave every other file the old version had and the new one
# dropped. Step 5 is what reads it back.
stale="$installed/Contents/Resources/only-in-the-previous-release.txt"
printf 'a file the previous release had\n' > "$stale"
echo "   $(basename "$installed") is in the Applications folder"

echo "== 2. the person's configuration and sign-in, as they would be after using it"
printf 'defaultRegistry: https://dev.example.net\nonboarded: true\n' > "$AGENTRY_CONFIG_DIR/desktop.yaml"
printf 'installs:\n  - name: brief\n    path: /Users/someone/projects/brief\n' > "$AGENTRY_CONFIG_DIR/installed.yaml"
printf 'dev.example.net pin\n' > "$AGENTRY_CONFIG_DIR/keys/dev.example.net.pub"
printf 'defaultAdapter: docker-agent\nharness:\n  claude:\n    model: sonnet\n' > "$AGENTRY_RUNNER_HOME/policy.yaml"
printf 'name: default\n' > "$AGENTRY_RUNNER_HOME/profiles/default.yaml"

snapshot() { (cd "$work" && find config runner -type f -exec shasum -a 256 {} + | sort); }
before=$(snapshot)

# Created, not reused, and named explicitly in every call below, so the
# machine's keychain search list is never read or written and no login session
# is needed.
security create-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN" || fail "could not create the acceptance keychain"
security unlock-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN" || fail "could not unlock the acceptance keychain"
security set-keychain-settings "$KEYCHAIN" || fail "could not stop the acceptance keychain locking itself"
security add-generic-password -s "$KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" -w "$KEYCHAIN_SECRET" -U "$KEYCHAIN" >/dev/null || fail "could not write the keychain item that stands in for the sign-in"
echo "   configuration written; a keychain item in $(basename "$KEYCHAIN") stands in for the sign-in"

echo "== 3. the upgrade: the same drag, over the copy that is there"
drag "$dmg"
[ "$dragged" = "$installed" ] || fail "the upgrade landed at $dragged, not over $installed"
[ -x "$installed/Contents/MacOS/agentry-desktop" ] || fail "the upgraded bundle has no executable"
codesign --verify --deep --strict "$installed" 2>&1 || fail "the upgraded bundle's signature does not verify after the drag"

echo "== 4. what the person had is still what the person has"
after=$(snapshot)
if [ "$before" != "$after" ]; then
  diff <(printf '%s\n' "$before") <(printf '%s\n' "$after") >&2 || true
  fail "the upgrade changed the configuration"
fi
for f in "$AGENTRY_CONFIG_DIR/desktop.yaml" "$AGENTRY_CONFIG_DIR/installed.yaml" "$AGENTRY_CONFIG_DIR/keys/dev.example.net.pub" "$AGENTRY_RUNNER_HOME/policy.yaml" "$AGENTRY_RUNNER_HOME/profiles/default.yaml"; do
  [ -f "$f" ] || fail "$f did not survive the upgrade"
done
got=$(security find-generic-password -s "$KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" -w "$KEYCHAIN" 2>/dev/null) || fail "the keychain item did not survive the upgrade"
[ "$got" = "$KEYCHAIN_SECRET" ] || fail "the keychain item changed across the upgrade"

echo "== 5. the upgrade replaced the bundle rather than merging into it"
[ ! -e "$stale" ] || fail "$(basename "$stale") from the previous release survived the upgrade: the drag merged instead of replacing"

echo "== 6. the bundle installed from the image runs"
# A launch smoke on the copy that came off the dmg and through the upgrade,
# rather than on the build output the ci job launches. A window needs the
# user logged in at the console; without one WindowServer refuses and this
# is reported skipped, not failed, as the build's own smoke does. AGENTRY_CONFIG_DIR and AGENTRY_RUNNER_HOME
# are exported above so that this launch reads the temporary directories
# and not the machine's real configuration. Nothing is asserted about the configuration
# afterwards: an app that writes its own settings on launch is behaving, and
# what this script claims about configuration is claimed of the drag, above.
if launchctl print "gui/$(id -u)" >/dev/null 2>&1; then
  "$installed/Contents/MacOS/agentry-desktop" >"$work/launch.log" 2>&1 & pid=$!
  sleep 5
  if kill -0 "$pid" 2>/dev/null; then
    echo "   agentry-desktop running after 5 s"
    kill "$pid"; wait "$pid" 2>/dev/null || true
    launch=ran
  else
    wait "$pid" 2>/dev/null; ec=$?
    tail -20 "$work/launch.log" >&2 || true
    fail "the bundle installed from the image exited with $ec before 5 s"
  fi
else
  echo "   SKIPPED: no console session for $(id -un) here, so no window can open (launchctl print gui/uid)"
  launch=skipped
fi

if [ "$launch" = ran ]; then
  echo "darwin-dmg-install: ok — the upgrade replaced the bundle, left the settings, the installed agents, the pinned key, the runner policy and the keychain item untouched, and the installed bundle ran"
else
  echo "darwin-dmg-install: ok — the upgrade replaced the bundle and left the settings, the installed agents, the pinned key, the runner policy and the keychain item untouched; the installed bundle was NOT launched (no console session here)"
fi
