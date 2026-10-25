#!/usr/bin/env bash
# Reads the built disk image back and checks it holds the bundle and an
# Applications shortcut and nothing else. Runs on macOS only: it mounts the image.
#
#   scripts/check-darwin-dmg.sh [--cli <agentry>] [--whisper <dir>] <out.dmg> <agentry-desktop.app>
#
# What it proves:
#   1. the file name is one the download manifest admits (.dmg is the "dmg"
#      format, and the download page renders a dmg card for darwin/universal);
#   2. the image's own checksum verifies (hdiutil verify): what was written
#      is what is read back;
#   3. it mounts read-only without a Finder session, and holds exactly the
#      application and an Applications symlink pointing at /Applications —
#      nothing else is handed to the person;
#   4. the bundle on the image is universal: lipo names both arm64 and
#      x86_64, the same check the release already runs on the built bundle;
#   5. its signature verifies, whoever made it: the authority is printed
#      rather than asserted (the ad-hoc one wails applies, or the Developer
#      ID one a release signs with, which that release reads back itself);
#   6. the bundle on the image is byte-for-byte the bundle <agentry-desktop.app>
#      holds, which is the bundle the zip was made from: the dmg is a second
#      wrapper, not a second build. It compares the content of every file and
#      the target of every symlink, and nothing else — not modes, not owners,
#      not extended attributes. The bundle's signature (ad-hoc or Developer
#      ID) covers the parts of those that matter, and check 5 verifies it on
#      the mounted copy. A stapled ticket is a file in the bundle
#      (Contents/CodeResources), so it is compared too;
#   7. with --cli, the bundle on the image carries that agentry CLI as
#      Contents/Resources/bin/agentry: executable, universal, answering
#      --version as <agentry> does, signed by the same authority as the
#      bundle under the CLI's own identifier, and sealed by the bundle's
#      signature (listed in its CodeResources), so a notarized bundle's
#      ticket covers it. Without --cli the bundle must carry no CLI.
#
#   8. the bundle carries the third-party notices as
#      Contents/Resources/THIRD_PARTY_NOTICES.txt, byte-identical to the
#      repository's THIRD_PARTY_NOTICES and sealed by the bundle's signature.
#   9. with --whisper <dir>, the bundle carries Meeting notes' transcriber as
#      Contents/Resources/whisper, holding exactly <dir>'s files (whisper-cli),
#      each byte-identical to <dir>'s: the sidecar is carried as it was signed
#      and notarized, never re-signed. whisper-cli is executable, universal,
#      signed as net.secureagentry.whisper-cli under the hardened runtime with
#      no entitlements, and sealed by the bundle's signature. Without
#      --whisper the bundle must carry no Contents/Resources/whisper.
#
# The image is detached whatever happens — on the way out, and on an
# interrupt or a cancelled CI job — because a leaked mount on a persistent
# runner is attached at a mktemp path nothing will ever detach again. A stale
# one from an earlier run is swept before this one attaches, for the same
# reason.
set -euo pipefail

fail() { echo "check-darwin-dmg: $*" >&2; exit 1; }

usage="usage: $0 [--cli <agentry>] [--whisper <dir>] <out.dmg> <agentry-desktop.app>"
cli=
whisper=
while [ $# -gt 0 ]; do
  case "$1" in
    --cli) cli="${2:?$usage}"; shift 2; [ -f "$cli" ] || fail "$cli does not exist" ;;
    --whisper) whisper="${2:?$usage}"; shift 2; [ -f "$whisper/whisper-cli" ] || fail "$whisper holds no whisper-cli" ;;
    --*) fail "$usage" ;;
    *) break ;;
  esac
done
dmg="${1:?$usage}"
app="${2:?$usage}"

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
# reads all of tr's output (>/dev/null, not -q) so tr is never cut off.
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

# 7. The agentry CLI the bundle carries, or that it carries none.
carried="$mnt/$appname/Contents/Resources/bin/agentry"
clinote="no agentry CLI"
if [ -z "$cli" ]; then
  [ ! -e "$carried" ] || fail "the bundle on the image carries Contents/Resources/bin/agentry, and no --cli says which"
else
  [ -f "$carried" ] && [ -x "$carried" ] || fail "the bundle on the image has no executable Contents/Resources/bin/agentry"
  cli_arches=$(lipo -archs "$carried")
  for want in arm64 x86_64; do
    tr ' ' '\n' <<<"$cli_arches" | grep -x "$want" >/dev/null || fail "the carried agentry is not universal: lipo says '$cli_arches'"
  done
  want_v=$("$cli" --version 2>&1) || fail "$cli --version failed"
  got_v=$("$carried" --version 2>&1) || fail "the carried agentry's --version failed"
  want_v=${want_v%%$'\n'*} got_v=${got_v%%$'\n'*}
  [ "$got_v" = "$want_v" ] || fail "the carried agentry says '$got_v', $cli says '$want_v'"
  cli_sig=$(codesign -dvv "$carried" 2>&1) || fail "the carried agentry is not signed"
  grep -qx 'Identifier=net.secureagentry.cli' <<<"$cli_sig" || fail "the carried agentry is not signed as net.secureagentry.cli"
  app_sig=$(codesign -dvv "$mnt/$appname" 2>&1) || fail "codesign could not describe the bundle on the image"
  app_auth=$(grep -E '^(Authority|Signature)=' <<<"$app_sig" | head -1 || true)
  cli_auth=$(grep -E '^(Authority|Signature)=' <<<"$cli_sig" | head -1 || true)
  [ -n "$app_auth" ] || fail "the bundle on the image names no signing authority"
  [ "$cli_auth" = "$app_auth" ] || fail "the carried agentry is signed '$cli_auth', the bundle '$app_auth'"
  grep -q '<key>Resources/bin/agentry</key>' "$mnt/$appname/Contents/_CodeSignature/CodeResources" ||
    fail "the bundle's signature does not seal Contents/Resources/bin/agentry"
  clinote="carries ${got_v%% default registry*} ($cli_auth)"
fi

# 8. The third-party notices.
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
notices="$mnt/$appname/Contents/Resources/THIRD_PARTY_NOTICES.txt"
[ -f "$notices" ] || fail "the bundle on the image has no Contents/Resources/THIRD_PARTY_NOTICES.txt"
cmp -s "$notices" "$root_dir/THIRD_PARTY_NOTICES" || fail "the THIRD_PARTY_NOTICES.txt on the image differs from $root_dir/THIRD_PARTY_NOTICES"
grep -q '<key>Resources/THIRD_PARTY_NOTICES.txt</key>' "$mnt/$appname/Contents/_CodeSignature/CodeResources" ||
  fail "the bundle's signature does not seal Contents/Resources/THIRD_PARTY_NOTICES.txt"

# 9. Meeting notes' transcriber, or that the bundle carries none.
carried_whisper="$mnt/$appname/Contents/Resources/whisper"
whispernote="no transcriber"
if [ -z "$whisper" ]; then
  [ ! -e "$carried_whisper" ] || fail "the bundle on the image carries Contents/Resources/whisper, and no --whisper says which"
else
  [ -d "$carried_whisper" ] || fail "the bundle on the image has no Contents/Resources/whisper"
  want_files=$(cd "$whisper" && find . -mindepth 1 -print | sed 's|^\./||' | sort)
  got_files=$(cd "$carried_whisper" && find . -mindepth 1 -print | sed 's|^\./||' | sort)
  [ "$got_files" = "$want_files" ] || { printf 'carried:\n%s\nwanted:\n%s\n' "$got_files" "$want_files" >&2; fail "Contents/Resources/whisper on the image does not hold exactly the files of $whisper"; }
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    [ -f "$whisper/$f" ] && [ ! -L "$whisper/$f" ] || fail "$whisper/$f is not a regular file"
    cmp -s "$carried_whisper/$f" "$whisper/$f" || fail "Contents/Resources/whisper/$f on the image differs from $whisper/$f"
  done <<<"$want_files"
  wcli="$carried_whisper/whisper-cli"
  [ -x "$wcli" ] || fail "the carried whisper-cli is not executable"
  w_arches=$(lipo -archs "$wcli")
  for want in arm64 x86_64; do
    tr ' ' '\n' <<<"$w_arches" | grep -x "$want" >/dev/null || fail "the carried whisper-cli is not universal: lipo says '$w_arches'"
  done
  w_sig=$(codesign -dvv "$wcli" 2>&1) || fail "the carried whisper-cli is not signed"
  grep -qx 'Identifier=net.secureagentry.whisper-cli' <<<"$w_sig" || fail "the carried whisper-cli is not signed as net.secureagentry.whisper-cli"
  grep -qE '^CodeDirectory .*flags=0x[0-9a-f]+\(([^)]*,)?runtime[,)]' <<<"$w_sig" || fail "the carried whisper-cli is not signed under the hardened runtime"
  w_ent=$(codesign -d --entitlements - --xml "$wcli" 2>/dev/null) || fail "codesign could not read the entitlements of the carried whisper-cli"
  if grep '<key>' <<<"$w_ent" >/dev/null; then fail "the carried whisper-cli carries entitlements"; fi
  grep -q '<key>Resources/whisper/whisper-cli</key>' "$mnt/$appname/Contents/_CodeSignature/CodeResources" ||
    fail "the bundle's signature does not seal Contents/Resources/whisper/whisper-cli"
  w_auth=$(grep -E '^(Authority|Signature)=' <<<"$w_sig" | head -1 || true)
  whispernote="carries whisper-cli ($w_auth)"
fi

echo "check-darwin-dmg: $name ok — mounts, holds $appname + Applications -> /Applications, universal ($arches), ${authority% }, identical to $(basename "$app"), third-party notices sealed, $clinote, $whispernote"
