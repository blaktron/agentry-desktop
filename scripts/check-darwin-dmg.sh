#!/usr/bin/env bash
# Reads the built disk image back and checks it is the one the plan describes
# (agentry-desktop#85; agentry-notes/plans/desktop-installers.md §5, D2, D3).
# Runs on macOS only: it mounts the image.
#
#   scripts/check-darwin-dmg.sh <out.dmg> <agentry-desktop.app>
#
# What it proves:
#   1. the file name is one the site's manifest admits
#      (agentry-web/scripts/release/manifest.py NAME: .dmg is the "dmg"
#      format, and the download page renders a dmg card for darwin/universal);
#   2. the image's own checksum verifies (hdiutil verify): what was written
#      is what is read back;
#   3. it mounts read-only without a Finder session, and holds exactly the
#      application and an Applications symlink pointing at /Applications —
#      nothing else is handed to the person;
#   4. the bundle on the image is universal: lipo names both arm64 and
#      x86_64, the same check the release already runs on the built bundle;
#   5. its signature verifies (the ad-hoc one wails applies, D-001). The
#      authority is printed rather than asserted, so this check keeps passing
#      unchanged on the day #6 makes it a Developer ID;
#   6. the bundle on the image is byte-for-byte the bundle <agentry-desktop.app>
#      holds, which is the bundle the zip was made from: the dmg is a second
#      wrapper, not a second build. It compares the content of every file and
#      the target of every symlink, and nothing else — not modes, not owners,
#      not extended attributes. The ad-hoc signature covers the parts of those
#      that matter, and check 5 verifies it on the mounted copy.
#
# The image is detached whatever happens — on the way out, and on an
# interrupt or a cancelled CI job — because a leaked mount on a persistent
# runner is attached at a mktemp path nothing will ever detach again. A stale
# one from an earlier run is swept before this one attaches, for the same
# reason.
set -euo pipefail

dmg="${1:?usage: $0 <out.dmg> <agentry-desktop.app>}"
app="${2:?usage: $0 <out.dmg> <agentry-desktop.app>}"

fail() { echo "check-darwin-dmg: $*" >&2; exit 1; }

[ -f "$dmg" ] || fail "$dmg does not exist"
[ -d "$app" ] || fail "$app does not exist"
[ "$(uname -s)" = Darwin ] || fail "reading a dmg back needs macOS (hdiutil); this is $(uname -s)"

# 1. The name.
name=$(basename "$dmg")
[[ "$name" =~ ^agentry-desktop-.+-darwin-universal\.dmg$ ]] || fail "$name is not agentry-desktop-<version>-darwin-universal.dmg, which the download manifest would drop"

# 2. The image verifies.
hdiutil verify "$dmg" >/dev/null 2>&1 || fail "hdiutil verify failed on $name"

# 3. Mount it. -nobrowse keeps it out of any Finder that is running; an
# explicit mountpoint keeps it off /Volumes, where a parallel job's image of
# the same volume name would collide.
mnt=$(mktemp -d "${TMPDIR:-/tmp}/agentry-dmg-mnt.XXXXXX")
detach() { hdiutil detach "$mnt" -quiet 2>/dev/null || hdiutil detach "$mnt" -force -quiet 2>/dev/null || true; rmdir "$mnt" 2>/dev/null || true; }
trap detach EXIT INT TERM
# A run that was killed before its trap could fire leaves this image attached.
# Detaching by device rather than by mountpoint, because the mountpoint it was
# attached at belonged to that run.
abs_dmg=$(cd "$(dirname "$dmg")" && printf '%s/%s' "$(pwd)" "$(basename "$dmg")")
hdiutil info | awk -v img="$abs_dmg" '
  /^image-path[[:space:]]*:/ { p = $0; sub(/^image-path[[:space:]]*:[[:space:]]*/, "", p); mine = (p == img); next }
  mine && $1 ~ /^\/dev\/disk/ { print $1 }' | while read -r dev; do
  echo "check-darwin-dmg: detaching a stale attachment of $name at $dev" >&2
  hdiutil detach "$dev" -force -quiet 2>/dev/null || true
done
hdiutil attach "$dmg" -nobrowse -readonly -noautoopen -mountpoint "$mnt" >/dev/null || fail "$name did not mount"

appname=$(basename "$app")
# || true on the grep: it exits 1 when it filters every line (an image with
# nothing but HFS metadata on it), and under pipefail that would kill the
# script with a bare status and no sentence saying what was wrong. The empty
# listing is then caught by the comparison below, which does say it.
listing=$(ls -A "$mnt" | { grep -v -e '^\.fseventsd$' -e '^\.Trashes$' -e '^\.DS_Store$' -e '^\.TemporaryItems$' || true; } | sort)
expected=$(printf 'Applications\n%s\n' "$appname" | sort)
if [ "$listing" != "$expected" ]; then
  printf 'on the image:\n%s\n' "$listing" >&2
  fail "the image does not hold exactly $appname and an Applications shortcut"
fi
[ -L "$mnt/Applications" ] || fail "Applications on the image is not a symlink"
[ "$(readlink "$mnt/Applications")" = /Applications ] || fail "Applications on the image points at $(readlink "$mnt/Applications"), not /Applications"

inner="$mnt/$appname/Contents/MacOS/agentry-desktop"
[ -f "$inner" ] || fail "$appname on the image has no Contents/MacOS/agentry-desktop"

# 4. Universal.
# Listed once into a variable: a grep -q on a live pipe would close it early
# and pipefail would read the SIGPIPE as "not universal" (the hazard
# check-windows-installer.sh documents at its own file listing). The grep
# reads all of tr's output (>/dev/null, not -q) so tr is never cut off
# (agentry-desktop#236).
arches=$(lipo -archs "$inner")
for want in arm64 x86_64; do
  tr ' ' '\n' <<<"$arches" | grep -x "$want" >/dev/null || fail "the bundle on the image is not universal: lipo says '$arches'"
done

# 5. The signature verifies; the authority is reported, not asserted.
if ! codesign_out=$(codesign --verify --deep --strict "$mnt/$appname" 2>&1); then
  printf '%s\n' "$codesign_out" >&2
  fail "codesign --verify failed on the bundle on the image"
fi
authority=$(codesign -dvv "$mnt/$appname" 2>&1 | grep -E '^(Authority|Signature)=' | head -2 | tr '\n' ' ' || true)
[ -n "$authority" ] || authority="Signature=(none reported)"

# 6. The same bundle as <agentry-desktop.app>, which is the bundle the zip was
# made from. Every file's content and every symlink's target, both trees,
# reduced to one digest.
#
# Each side goes into its own variable, and each is checked against the digest
# of nothing before they are compared: inside an `if [ "$(a)" != "$(b)" ]` a
# failure on BOTH sides is two empty strings, which compare equal, and the
# check would then pass by failing.
empty_digest=$(printf '' | shasum -a 256)
manifest() { (cd "$1" && find . \( -type f -o -type l \) -print0 | sort -z | xargs -0 shasum -a 256 | shasum -a 256); }
on_image=$(manifest "$mnt/$appname") || fail "could not read the bundle on the image"
on_disk=$(manifest "$app") || fail "could not read $app"
[ "$on_image" != "$empty_digest" ] || fail "the bundle on the image read as no files at all: nothing was compared"
[ "$on_disk" != "$empty_digest" ] || fail "$app read as no files at all: nothing was compared"
if [ "$on_image" != "$on_disk" ]; then
  diff <( (cd "$mnt/$appname" && find . \( -type f -o -type l \) -print0 | sort -z | xargs -0 shasum -a 256) ) \
       <( (cd "$app" && find . \( -type f -o -type l \) -print0 | sort -z | xargs -0 shasum -a 256) ) >&2 || true
  fail "the $appname on the image differs from $app: the dmg is not the build the zip carries"
fi

echo "check-darwin-dmg: $name ok — mounts, holds $appname + Applications -> /Applications, universal ($arches), ${authority% }, identical to $(basename "$app")"
