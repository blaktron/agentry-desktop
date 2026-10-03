#!/usr/bin/env bash
# Wraps the built universal bundle in a disk image, the way a macOS
# application is handed to a person: mount it, drag the app onto the
# Applications shortcut beside it (agentry-desktop#85; plan
# agentry-notes/plans/desktop-installers.md §5, D2, D3).
#
#   scripts/package-darwin-dmg.sh <agentry-desktop.app> <out.dmg>
#
# Built with hdiutil alone, which is part of macOS. create-dmg is not on the
# Mac (there is no Homebrew there, plan macos-build-host.md) and would buy
# only the window's background picture and icon positions, which are a
# .DS_Store written by scripting Finder — and the runner has no console
# session to run Finder in. So the image gets the two things that matter,
# the bundle and the shortcut, and nothing that needs a logged-in Mac to
# produce. A background picture is a later issue if anyone asks for one.
#
# The bundle inside is the same one the zip carries, under the same name:
# the zip and the dmg are two wrappers around one build, not two builds, and
# the self-update bundle swap (M7, #88) looks for that name.
#
# This script signs nothing. In a production release the bundle it is given
# is already Developer ID-signed, notarized and stapled, and the release
# signs, notarizes and staples the image it writes afterwards
# (scripts/sign-darwin.sh; agentry-desktop#6). Anywhere else (a PR build, a
# dev prerelease, a by-hand build) the bundle carries only the ad-hoc
# signature wails applies and the image none, and macOS refuses to open a
# downloaded copy of either until the person allows it in System Settings →
# Privacy & Security (register D-001).
set -euo pipefail

app="${1:?usage: $0 <agentry-desktop.app> <out.dmg>}"
out="${2:?usage: $0 <agentry-desktop.app> <out.dmg>}"

fail() { echo "package-darwin-dmg: $*" >&2; exit 1; }

[ -d "$app" ] || fail "$app does not exist"
[ "$(uname -s)" = Darwin ] || fail "a dmg can only be built on macOS (hdiutil); this is $(uname -s)"
command -v hdiutil >/dev/null 2>&1 || fail "hdiutil is missing"
command -v ditto >/dev/null 2>&1 || fail "ditto is missing"

stage=$(mktemp -d "${TMPDIR:-/tmp}/agentry-dmg.XXXXXX")
trap 'rm -rf "$stage"' EXIT

# ditto, not cp: it keeps the bundle's symlinks, permissions and extended
# attributes, so the copy on the image is the bundle that was built and its
# ad-hoc signature still verifies.
ditto "$app" "$stage/$(basename "$app")"
ln -s /Applications "$stage/Applications"

# UDZO is the compressed read-only format every macOS reads; HFS+ is named
# explicitly rather than left to hdiutil's default so the image mounts on
# older systems too. -ov lets hdiutil overwrite, and the rm is what handles
# the case -ov does not: a leftover at that path that is not a disk image at
# all, which hdiutil refuses rather than replaces.
rm -f "$out"
hdiutil create \
  -volname "Agentry Desktop" \
  -srcfolder "$stage" \
  -fs HFS+ \
  -format UDZO \
  -imagekey zlib-level=9 \
  -ov \
  "$out" >/dev/null

# hdiutil appends .dmg when the name lacks it; refuse rather than hand back a
# path that does not exist.
[ -f "$out" ] || fail "hdiutil did not write $out"

echo "package-darwin-dmg: $(basename "$out") ($(du -h "$out" | cut -f1)) — $(basename "$app") and an Applications shortcut, the image itself unsigned"
