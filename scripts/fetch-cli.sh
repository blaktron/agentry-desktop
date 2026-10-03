#!/usr/bin/env bash
# The agentry CLI the desktop's packages carry (agentry-desktop#580; plan
# agentry-notes/plans/cli-on-path.md §6, D3, D5, D8; D-061).
#
#   scripts/fetch-cli.sh OUT_DIR        (release; GH_TOKEN reads agentry-cli)
#
# GitHub is read with curl and jq through the REST API (the release job has
# both; the runner image has no gh), with GH_TOKEN, which needs only read
# access to agentry-cli's contents.
#   scripts/fetch-cli.sh --self-test
#
# The CLI is never built here: the desktop carries the CLI release of the
# version its go.mod pins, the CLI's own signed binaries, so one CLI has one
# set of bytes wherever it is installed from. The release is the pin's on
# the desktop's channel, read from AGENTRY_DEFAULT_REGISTRY (the release
# job's preflight names it; build/release.env says which is which):
#   production -> v<pin>, which must be a release, not a prerelease;
#   dev        -> the newest v<pin>-dev.N, or the pin itself when go.mod
#                 pins a prerelease.
# Checked before a byte is kept:
#   - the two binaries and BUILD-INFO against the release's SHA256SUMS;
#   - BUILD-INFO: product agentry-cli, this version, a tag push, and the
#     registry this desktop bakes in (a dev desktop never carries a CLI that
#     talks to production, or the reverse: DEV-066's "one page, one
#     registry");
#   - SHA256SUMS.minisig, when the release has one, against the key set
#     this build carries and for this product and version
#     (internal/cmd/verifysums; a CLI release's sums are signed once
#     agentry-cli has the estate's key, and are unsigned before that);
#   - BUILD-INFO claims a Windows signature (signed_windows=authenticode …):
#     the signature itself is verified against the pinned Microsoft root and
#     the CN build/release.env names when the installer is read back
#     (scripts/check-windows-installer.sh --cli).
# OUT_DIR then holds agentry.exe (windows-amd64), agentry (linux-amd64) and
# CLI-BUILD-INFO.
set -euo pipefail
cd "$(dirname "$0")/.."

REPO=blaktron/agentry-cli
MODULE=github.com/blaktron/agentry-cli

die() { echo "fetch-cli: $*" >&2; exit 1; }

# pin_of <go.mod>: the version go.mod requires of the CLI module.
pin_of() {
  awk -v m="$MODULE" '$1 == m && $2 ~ /^v/ {print $2; exit} $1 == "require" && $2 == m {print $3; exit}' "$1"
}

# tag_for <pin> <channel> <tags…>: the release tag to carry.
tag_for() {
  local pin=$1 channel=$2; shift 2
  case "$channel" in
    production)
      [[ "$pin" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "a production desktop carries a released CLI, and go.mod pins $pin" >&2; return 1; }
      grep -qx -- "$pin" <<<"$(printf '%s\n' "$@")" || { echo "agentry-cli has no release $pin" >&2; return 1; }
      echo "$pin" ;;
    dev)
      if [[ "$pin" =~ -dev\.[0-9]+$ ]]; then
        grep -qx -- "$pin" <<<"$(printf '%s\n' "$@")" || { echo "agentry-cli has no release $pin" >&2; return 1; }
        echo "$pin"; return 0
      fi
      local newest
      newest=$(printf '%s\n' "$@" | grep -E "^${pin//./\\.}-dev\.[0-9]+$" | sort -t. -k4,4n | tail -1 || true)
      [ -n "$newest" ] || { echo "agentry-cli has no dev release of $pin (v…-dev.N)" >&2; return 1; }
      echo "$newest" ;;
    *) echo "unknown channel $channel" >&2; return 1 ;;
  esac
}

# info <BUILD-INFO> <key>
info() { sed -n "s/^$2=//p" "$1" | head -1; }

if [ "${1:-}" = "--self-test" ]; then
  fails=0
  t() { local want=$1; shift; local got; got=$(tag_for "$@" 2>/dev/null) || got="refused"; [ "$got" = "$want" ] || { echo "self-test: tag_for $* = $got, want $want" >&2; fails=$((fails + 1)); }; }
  t v0.6.52 v0.6.52 production v0.6.51 v0.6.52 v0.6.52-dev.2
  t refused v0.6.53 production v0.6.52
  t refused v0.6.52-dev.2 production v0.6.52-dev.2
  t v0.6.52-dev.10 v0.6.52 dev v0.6.52 v0.6.52-dev.2 v0.6.52-dev.10 v0.6.51-dev.3
  t v0.6.52-dev.2 v0.6.52-dev.2 dev v0.6.52-dev.2 v0.6.52-dev.3
  t refused v0.6.53 dev v0.6.52-dev.1
  t refused v0.6.52 staging v0.6.52
  gm=$(mktemp); trap 'rm -f "$gm"' EXIT
  printf 'module x\n\nrequire (\n\tgithub.com/blaktron/agentry-cli v0.6.52\n\tgolang.org/x/sys v0.48.0\n)\n' >"$gm"
  [ "$(pin_of "$gm")" = v0.6.52 ] || { echo "self-test: pin_of a require block" >&2; fails=$((fails + 1)); }
  printf 'module x\n\nrequire github.com/blaktron/agentry-cli v0.6.52-dev.2\n' >"$gm"
  [ "$(pin_of "$gm")" = v0.6.52-dev.2 ] || { echo "self-test: pin_of a one-line require" >&2; fails=$((fails + 1)); }
  [ "$fails" -eq 0 ] || exit 1
  echo "fetch-cli --self-test: ok (the release per channel, a production pin that is not a release refused, the newest dev.N by number, the pin read from go.mod)"
  exit 0
fi

out="${1:?usage: $0 OUT_DIR | --self-test}"
registry="${AGENTRY_DEFAULT_REGISTRY:?AGENTRY_DEFAULT_REGISTRY names the registry this desktop bakes in}"
: "${GH_TOKEN:?GH_TOKEN reads the releases of $REPO}"
for t in curl jq sha256sum; do command -v "$t" >/dev/null || die "$t is needed"; done
# api <path> [curl args…]: GitHub's REST API with the token, from a config
# on stdin so the token never reaches a process listing.
api() {
  local path=$1; shift
  printf 'header = "Authorization: Bearer %s"\n' "$GH_TOKEN" |
    curl -fsSL --proto '=https' -K - -H "X-GitHub-Api-Version: 2022-11-28" "$@" "https://api.github.com/repos/$REPO/$path"
}
# shellcheck source=/dev/null
. build/release.env
case "${registry%/}" in
  "${AGENTRY_REGISTRY_PRODUCTION%/}") channel=production ;;
  "${AGENTRY_REGISTRY_DEV%/}") channel=dev ;;
  *) die "$registry is neither build/release.env's production nor its dev registry" ;;
esac

pin=$(pin_of go.mod)
[ -n "$pin" ] || die "go.mod requires no $MODULE"
# Production reads its one tag; dev needs the list, a page of 100 at a time.
if [ "$channel" = production ]; then
  tags=("$pin")
  [ "$(api "releases/tags/$pin" -H "Accept: application/vnd.github+json" | jq -r '.prerelease')" = false ] ||
    die "$REPO $pin is not a published release (or not one that is not a prerelease)"
else
  tags=()
  for page in 1 2 3 4 5; do
    mapfile -t more < <(api "releases?per_page=100&page=$page" -H "Accept: application/vnd.github+json" | jq -r '.[] | select(.draft | not) | .tag_name')
    [ "${#more[@]}" -gt 0 ] || break
    tags+=("${more[@]}")
  done
  [ "${#tags[@]}" -gt 0 ] || die "could not list the releases of $REPO"
fi
tag=$(tag_for "$pin" "$channel" "${tags[@]}") || die "no CLI release to carry"
version=${tag#v}
echo "fetch-cli: go.mod pins $pin; the $channel desktop carries $REPO $tag"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
exe="agentry-${version}-windows-amd64.exe"
lin="agentry-${version}-linux-amd64"
api "releases/tags/$tag" -H "Accept: application/vnd.github+json" >"$work/release.json" || die "could not read release $tag"
for f in "$exe" "$lin" BUILD-INFO SHA256SUMS; do
  id=$(jq -r --arg n "$f" '.assets[] | select(.name == $n) | .id' "$work/release.json")
  [ -n "$id" ] || die "$tag has no asset $f"
  api "releases/assets/$id" -H "Accept: application/octet-stream" -o "$work/$f" || die "could not download $f from $tag"
done
for f in "$exe" "$lin" BUILD-INFO; do
  [ -s "$work/$f" ] || die "$tag has no $f"
  grep -q "  $f\$" "$work/SHA256SUMS" || die "$tag's SHA256SUMS does not list $f"
done
(cd "$work" && sha256sum --quiet -c --ignore-missing SHA256SUMS) || die "$tag's files do not match its SHA256SUMS"
sig_id=$(jq -r '.assets[] | select(.name == "SHA256SUMS.minisig") | .id' "$work/release.json")
if [ -n "$sig_id" ]; then
  api "releases/assets/$sig_id" -H "Accept: application/octet-stream" -o "$work/SHA256SUMS.minisig" || die "could not download SHA256SUMS.minisig from $tag"
  GOWORK=off go run ./internal/cmd/verifysums -product agentry-cli -version "$version" "$work/SHA256SUMS" "$work/SHA256SUMS.minisig" ||
    die "$tag's SHA256SUMS.minisig does not verify against this build's key set"
fi

bi="$work/BUILD-INFO"
[ "$(info "$bi" product)" = agentry-cli ] || die "$tag's BUILD-INFO is not agentry-cli's"
[ "$(info "$bi" version)" = "$version" ] || die "$tag's BUILD-INFO names version $(info "$bi" version)"
[ "$(info "$bi" event)" = push ] || die "$tag's BUILD-INFO is not a tag push's (a dry run)"
[ "$(info "$bi" ref)" = "refs/tags/$tag" ] || die "$tag's BUILD-INFO names ref $(info "$bi" ref)"
[ "$(info "$bi" default_registry)" = "${registry%/}" ] || die "$tag bakes $(info "$bi" default_registry), not this desktop's $registry"
case "$(info "$bi" signed_windows)" in
  authenticode\ CN=*) ;;
  *) die "$tag's Windows exe is not signed (BUILD-INFO signed_windows is empty); the desktop carries only a signed CLI (plan D5)" ;;
esac

mkdir -p "$out"
cp "$work/$exe" "$out/agentry.exe"
cp "$work/$lin" "$out/agentry"
chmod 0755 "$out/agentry"
cp "$bi" "$out/CLI-BUILD-INFO"
echo "fetch-cli: $out/agentry.exe and $out/agentry are $REPO $tag ($(info "$bi" signed_windows | sed 's/ profile=.*//'))"
