#!/usr/bin/env bash
# Wrap the Linux binary in a .deb and an .rpm from one description
# (build/nfpm/nfpm.yaml).
#
#   make package-linux            (builds the binary first, then calls this)
#   scripts/package-linux.sh      (DESKTOP_VERSION names the binary to wrap)
#
# Environment:
#   DESKTOP_VERSION   the version the binary was stamped with and is named
#                     by (default 0.0.0-dev; the Makefile passes git describe
#                     without the tag's v).
#
# nfpm is a Go program with no other dependency, run at the version pinned
# below through `go run`, so CI and a local build produce the same
# packages without installing anything; the module proxy's checksum database
# verifies what is fetched (nfpm is outside GOPRIVATE), and the first run
# compiles it into the build cache. The pin changes here, in one place.
#
# The package version is the binary's version respelled so the package
# manager orders it the way the commits are ordered: a tag is itself
# (0.1.0); a build past a tag, which git describe spells 0.1.0-9-g<sha>, is
# not a pre-release of 0.1.0 (semver would sort it below the tag it follows)
# but a build after it, so it becomes 0.1.0+9.g<sha> — build metadata, which
# dpkg and rpm both sort above 0.1.0 and below 0.1.1; -dirty and a tagless
# checkout's bare sha are spelled the same way under 0.0.0. The package file
# names keep the binary's version, the download manifest's grammar.
#
# Output: build/linux/agentry-desktop-<version>-linux-amd64.{deb,rpm}, and
# under build/linux/smoke/ a "previous" pair at version 0.0.0 from the same
# binary, which scripts/acceptance/linux-packages.sh installs first so the
# real package is proven as an upgrade, not a first install. Those two are
# never staged into a release.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${DESKTOP_VERSION:-0.0.0-dev}"
NFPM_VERSION=v2.47.0

# pkgversion <describe> — the package manager's spelling of the version.
pkgversion() {
  local v="$1" dirty=""
  case "$v" in *-dirty) dirty=".dirty"; v="${v%-dirty}";; esac
  if [[ "$v" =~ ^([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
    printf '%s%s\n' "$v" "${dirty:++${dirty#.}}"
  elif [[ "$v" =~ ^([0-9]+\.[0-9]+\.[0-9]+)-([0-9]+)-g([0-9a-f]+)$ ]]; then
    printf '%s+%s.g%s%s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}" "$dirty"
  elif [[ "$v" =~ ^[0-9a-f]{7,40}$ ]]; then
    printf '0.0.0+g%s%s\n' "$v" "$dirty"
  else
    printf '0.0.0+%s%s\n' "$(printf '%s' "$v" | tr -c 'A-Za-z0-9.\n' '.')" "$dirty"
  fi
}

if [ "${1:-}" = "--self-test" ]; then
  # The spellings the Makefile's VERSION can produce, and what they become.
  for pair in "0.1.0=0.1.0" "0.1.0-9-gbbade43=0.1.0+9.gbbade43" "0.1.0-9-gbbade43-dirty=0.1.0+9.gbbade43.dirty" "a5cd823=0.0.0+ga5cd823" "0.0.0-dev=0.0.0+0.0.0.dev" "0.1.0-dirty=0.1.0+dirty"; do
    got=$(pkgversion "${pair%%=*}")
    [ "$got" = "${pair#*=}" ] || { echo "self-test: pkgversion ${pair%%=*} = $got, want ${pair#*=}" >&2; exit 1; }
  done
  echo "self-test ok: every describe spelling has an ordered package version"
  exit 0
fi

bin="build/linux/agentry-desktop-${VERSION}-linux-amd64"
[ -f "$bin" ] || { echo "package-linux: $bin does not exist; build it first (scripts/build-linux.sh)" >&2; exit 1; }

PKG_VERSION=$(pkgversion "$VERSION")
echo "package-linux: binary version $VERSION, package version $PKG_VERSION"

# GOWORK=off and GOFLAGS cleared: a module run by version has nothing to do
# with this repository's workspace or its vendor directory.
nfpm() { GOWORK=off GOFLAGS= go run "github.com/goreleaser/nfpm/v2/cmd/nfpm@${NFPM_VERSION}" "$@"; }

# nfpm expands ${DESKTOP_VERSION} in the version but not in a content's src,
# so the binary is staged at the fixed path the description names.
mkdir -p build/linux/stage build/linux/smoke
cp -f "$bin" build/linux/stage/agentry-desktop
chmod 0755 build/linux/stage/agentry-desktop
trap 'rm -rf build/linux/stage' EXIT

# The agentry CLI, when the build carries it (build/cli/agentry, which a
# release puts there): /usr/bin/agentry,
# from a copy of the description with that one entry added after the app's,
# so a PR build packages exactly what it did. Still no maintainer script.
desc=build/nfpm/nfpm.yaml
if [ -f build/cli/agentry ]; then
  cp -f build/cli/agentry build/linux/stage/agentry
  chmod 0755 build/linux/stage/agentry
  desc=build/linux/stage/nfpm.yaml
  awk '{print} /^    dst: \/usr\/bin\/agentry-desktop$/{getline; print; getline; print; print "  - src: build/linux/stage/agentry\n    dst: /usr/bin/agentry\n    file_info:\n      mode: 0755"}' build/nfpm/nfpm.yaml >"$desc"
  grep -q '^    dst: /usr/bin/agentry$' "$desc" || { echo "package-linux: could not add /usr/bin/agentry to the description" >&2; exit 1; }
  echo "package-linux: carrying the agentry CLI as /usr/bin/agentry ($(sed -n 's/^version=//p' build/cli/CLI-BUILD-INFO 2>/dev/null || echo 'version unknown'))"
fi

# Meeting notes' transcriber, when the build carries it
# (build/whisper/whisper-cli-linux-amd64, which a release puts there): the
# whole folder, its libraries' links included, as
# /usr/lib/agentry-desktop/whisper, where the app looks for it beside
# /usr/bin. whisper-cli finds its libraries through its own RUNPATH
# ($ORIGIN). It links libstdc++ (GCC 12's, for GLIBCXX_3.4.30) and
# libgcc_s, which the app does not, so the description declares them too;
# the Vulkan backend's loader is not declared, since the backend is loaded
# at run time and the CPU runs without it. The two folders are the
# package's own, so a removal takes them.
sidecar=build/whisper/whisper-cli-linux-amd64
if [ -f "$sidecar/whisper-cli" ]; then
  if [ "$desc" = build/nfpm/nfpm.yaml ]; then
    cp -f build/nfpm/nfpm.yaml build/linux/stage/nfpm.yaml
    desc=build/linux/stage/nfpm.yaml
  fi
  mkdir -p build/linux/stage/whisper
  lib=/usr/lib/agentry-desktop
  entries="  - dst: $lib\n    type: dir\n  - dst: $lib/whisper\n    type: dir\n"
  for f in "$sidecar"/*; do
    n=$(basename "$f")
    [ "$n" != SHA256SUMS ] || continue
    if [ -L "$f" ]; then
      entries+="  - src: $(readlink "$f")\n    dst: $lib/whisper/$n\n    type: symlink\n"
    else
      cp -f "$f" "build/linux/stage/whisper/$n"
      entries+="  - src: build/linux/stage/whisper/$n\n    dst: $lib/whisper/$n\n    file_info:\n      mode: 0755\n"
    fi
  done
  awk -v e="$entries" '{print} /^contents:$/{printf "%s", e}' "$desc" >"$desc.new"
  awk '{print} /^      - libwebkit2gtk-4.1-0$/{print "      - libstdc++6 (>= 12)\n      - libgcc-s1"} /^      - webkit2gtk4.1$/{print "      - libstdc++ >= 12\n      - libgcc"}' "$desc.new" >"$desc"
  rm -f "$desc.new"
  grep -q "^    dst: $lib/whisper/whisper-cli$" "$desc" && grep -q '^      - libstdc++6 (>= 12)$' "$desc" && grep -q '^      - libstdc++ >= 12$' "$desc" ||
    { echo "package-linux: could not add the transcriber to the description" >&2; exit 1; }
  echo "package-linux: carrying the transcriber in $lib/whisper ($(find "$sidecar" -maxdepth 1 ! -name SHA256SUMS -type f -o -type l | grep -c .) files)"
fi

for fmt in deb rpm; do
  out="build/linux/agentry-desktop-${VERSION}-linux-amd64.${fmt}"
  DESKTOP_VERSION="$PKG_VERSION" nfpm package -f "$desc" -p "$fmt" -t "$out" >/dev/null
  echo "package-linux: $out"
  DESKTOP_VERSION=0.0.0 nfpm package -f "$desc" -p "$fmt" -t "build/linux/smoke/agentry-desktop-previous.${fmt}" >/dev/null
done
echo "package-linux: build/linux/smoke/agentry-desktop-previous.{deb,rpm} at 0.0.0, for the upgrade smoke only"
