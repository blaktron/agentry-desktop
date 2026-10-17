#!/usr/bin/env bash
# Wraps the built universal bundle in a disk image, the way a macOS
# application is handed to a person: mount it, drag the app onto the
# Applications shortcut beside it.
#
#   scripts/package-darwin-dmg.sh <agentry-desktop.app> <out.dmg>
#
# Built with hdiutil alone, which is part of macOS. create-dmg is not on the
# Mac and would buy only the window's background picture and icon
# positions, which are a .DS_Store written by scripting Finder — and a CI
# runner has no console session to run Finder in. So the image gets the two
# things that matter, the bundle and the shortcut, and nothing that needs a
# logged-in Mac to produce.
#
# The bundle inside is the same one the zip carries, under the same name:
# the zip and the dmg are two wrappers around one build, not two builds, and
# the self-update bundle swap looks for that name.
#
# This script signs nothing. In a production release the bundle it is given
# is already Developer ID-signed, notarized and stapled, and the release
# signs, notarizes and staples the image it writes afterwards. Anywhere else
# (a PR build, a dev prerelease, a by-hand build) the bundle carries only the
# ad-hoc signature wails applies and the image none, and macOS refuses to open a
# downloaded copy of either until the person allows it in System Settings →
# Privacy & Security.
set -euo pipefail

app="${1:?usage: $0 <agentry-desktop.app> <out.dmg>}"
out="${2:?usage: $0 <agentry-desktop.app> <out.dmg>}"

fail() { echo "package-darwin-dmg: $*" >&2; exit 1; }

[ -d "$app" ] || fail "$app does not exist"
[ "$(uname -s)" = Darwin ] || fail "a dmg can only be built on macOS (hdiutil); this is $(uname -s)"
command -v hdiutil >/dev/null 2>&1 || fail "hdiutil is missing"
command -v ditto >/dev/null 2>&1 || fail "ditto is missing"

out_dir=$(cd "$(dirname "$out")" 2>/dev/null && pwd) || fail "the directory for $out does not exist"
out_dir_real=$(cd "$out_dir" && pwd -P)
out_base=$(basename "$out")

# While hdiutil create writes an image it attaches the image it is writing,
# by its own path, mounted at /Volumes/<volname>, and ejects it at the end.
# Whatever leaves such an attachment behind makes the next create to the
# same path find it busy, so every attachment of $out is detached before the
# first attempt, after a failed one and on every exit, by device rather than
# by mount point because the mount point belongs to whichever run made it.
attached_devices() {
  hdiutil info | awk -v a="$out_dir/$out_base" -v b="$out_dir_real/$out_base" '
    /^image-path[[:space:]]*:/ { p = $0; sub(/^image-path[[:space:]]*:[[:space:]]*/, "", p); mine = (p == a || p == b); next }
    mine && $1 ~ /^\/dev\/disk/ { print $1 }'
}
detach_out() {
  local dev
  while read -r dev; do
    [ -n "$dev" ] || continue
    echo "package-darwin-dmg: detaching $out_base from $dev" >&2
    hdiutil detach "$dev" -quiet >/dev/null 2>&1 || hdiutil detach "$dev" -force -quiet >/dev/null 2>&1 || true
  done < <(attached_devices)
}

stage=$(mktemp -d "${TMPDIR:-/tmp}/agentry-dmg.XXXXXX")
err=$(mktemp "${TMPDIR:-/tmp}/agentry-dmg-err.XXXXXX")
cleanup() { detach_out; rm -rf "$stage" "$err"; }
trap 'exit 130' INT
trap 'exit 143' TERM
trap cleanup EXIT

# ditto, not cp: it keeps the bundle's symlinks, permissions and extended
# attributes, so the copy on the image is the bundle that was built and its
# ad-hoc signature still verifies.
ditto "$app" "$stage/$(basename "$app")"
ln -s /Applications "$stage/Applications"

# UDZO is the compressed read-only format every macOS reads; HFS+ is named
# explicitly rather than left to hdiutil's default so the image mounts on
# older systems too. The rm is what handles a leftover at that path, which
# hdiutil refuses to replace when it is not a disk image at all (-ov covers
# the rest).
#
# The volume hdiutil attaches while it writes is indexed by Spotlight and
# seen by Finder like any other, and ejecting it can fail with "Resource
# busy" while either still has a file on it, although a second eject a moment
# later succeeds. That failure says nothing about the image, so create is
# attempted again, a bounded number of times, a little later each time, and
# only on that message: any other failure is final on the first attempt.
attempts=4
delay=${PACKAGE_DMG_RETRY_DELAY:-5}
detach_out
n=1
while :; do
  rm -f "$out"
  if hdiutil create \
    -volname "Agentry Desktop" \
    -srcfolder "$stage" \
    -fs HFS+ \
    -format UDZO \
    -imagekey zlib-level=9 \
    -ov \
    "$out" >/dev/null 2>"$err"; then
    cat "$err" >&2
    break
  fi
  cat "$err" >&2
  detach_out
  if [ "$n" -ge "$attempts" ] || ! grep -q 'Resource busy' "$err"; then
    fail "hdiutil create failed (attempt $n of $attempts)"
  fi
  echo "package-darwin-dmg: the volume was busy (attempt $n of $attempts); trying again in $((n * delay))s" >&2
  sleep $((n * delay))
  n=$((n + 1))
done

# hdiutil appends .dmg when the name lacks it; refuse rather than hand back a
# path that does not exist.
[ -f "$out" ] || fail "hdiutil did not write $out"

echo "package-darwin-dmg: $(basename "$out") ($(du -h "$out" | cut -f1)) — $(basename "$app") and an Applications shortcut, the image itself unsigned"
