#!/usr/bin/env bash
# Installs the Linux packages in clean containers and proves that they
# install, upgrade and remove cleanly:
#
#   scripts/acceptance/linux-packages.sh <deb> <rpm> <binary> <previous-deb> <previous-rpm>
#
# <previous-*> are the 0.0.0 packages scripts/package-linux.sh builds from
# the same binary: installed first, so the real package is proven as the
# upgrade it will be for everyone who has a release already.
#
# The .deb in debian:bookworm-slim (the build container's family, so the
# glibc floor is the one the binary was linked against) and the .rpm in
# fedora:44 go through every step; the .deb
# also installs in ubuntu:24.04, whose *t64 library renames must satisfy the
# Debian names the package declares (every step but 4b: the owners of the
# libraries have different names there, by design).
#   1. the previous package installs from a clean image with its declared
#      dependencies and nothing else named by hand, and the real package
#      installs over it as an upgrade the package manager orders as one;
#   2. the package owns exactly the binary, the desktop entry, the icons and
#      the third-party notices (byte-identical to THIRD_PARTY_NOTICES), the
#      CLI and the transcriber when the build carries them (byte-identical,
#      and each runs),
#      nothing under /home or /root, and carries no maintainer script or
#      trigger (read from the package's control archive, not inferred);
#   3. the installed binary is byte-identical to <binary>, the one the bare
#      artifact ships;
#   4. every library the binary's ELF NEEDED entries name resolves, and the
#      transcriber's (all but the Vulkan backend's loader, which is optional)
#      when it is carried (4a), and
#      on the strict families the declared dependencies are exactly the
#      packages that own those libraries — nothing missing, nothing
#      superfluous (4b);
#   5. the app starts and stays alive under a virtual display (timeout's 124 is the
#      pass);
#   6. a home populated before the upgrade — desktop.yaml, installed.yaml, a
#      pinned key, a runner policy — is byte-identical after it and after
#      the package is removed. A container has no session keyring, so the
#      keychain half of that promise is not observed here; the package has
#      no scripts that could reach one (step 2), which is what makes it
#      hold.
# Everything runs as root inside a throwaway container; nothing is installed
# on the host. Docker is the only requirement, as for scripts/build-linux.sh.
# The staging directory lives under build/linux because the CI runner is
# itself a container driving the host's Docker daemon: only paths shared
# with the host (the work tree is) can be bind-mounted.
set -euo pipefail
cd "$(dirname "$0")/../.."

deb="${1:?usage: $0 <deb> <rpm> <binary> <previous-deb> <previous-rpm>}"
rpm="${2:?usage: $0 <deb> <rpm> <binary> <previous-deb> <previous-rpm>}"
bin="${3:?usage: $0 <deb> <rpm> <binary> <previous-deb> <previous-rpm>}"
prev_deb="${4:?usage: $0 <deb> <rpm> <binary> <previous-deb> <previous-rpm>}"
prev_rpm="${5:?usage: $0 <deb> <rpm> <binary> <previous-deb> <previous-rpm>}"
for f in "$deb" "$rpm" "$bin" "$prev_deb" "$prev_rpm"; do [ -f "$f" ] || { echo "linux-packages: $f does not exist" >&2; exit 1; }; done
[[ "$(basename "$deb")" =~ ^agentry-desktop-.+-linux-amd64\.deb$ ]] || { echo "linux-packages: $(basename "$deb") is not agentry-desktop-<version>-linux-amd64.deb, which the download manifest would drop" >&2; exit 1; }
[[ "$(basename "$rpm")" =~ ^agentry-desktop-.+-linux-amd64\.rpm$ ]] || { echo "linux-packages: $(basename "$rpm") is not agentry-desktop-<version>-linux-amd64.rpm, which the download manifest would drop" >&2; exit 1; }

stage="build/linux/smoke-stage"
rm -rf "$stage"; mkdir -p "$stage"
trap 'rm -rf "$stage"' EXIT
cp "$deb" "$stage/pkg.deb"; cp "$rpm" "$stage/pkg.rpm"; cp "$bin" "$stage/agentry-desktop.bin"
cp "$prev_deb" "$stage/previous.deb"; cp "$prev_rpm" "$stage/previous.rpm"
cp THIRD_PARTY_NOTICES "$stage/THIRD_PARTY_NOTICES"
# The agentry CLI the packages carry when the build has one (build/cli):
# /usr/bin/agentry must be it.
[ ! -f build/cli/agentry ] || cp build/cli/agentry "$stage/agentry.cli"
# The transcriber the packages carry when the build has one (build/whisper):
# /usr/lib/agentry-desktop/whisper must be it.
[ ! -f build/whisper/whisper-cli-linux-amd64/whisper-cli ] || cp -a build/whisper/whisper-cli-linux-amd64 "$stage/whisper"

# The part every container runs after installing the previous package.
# $1 is the family (deb or rpm); $2 is "strict" where the owners check runs.
cat > "$stage/common.sh" <<'EOS'
set -euo pipefail
fam="$1"; strict="${2:-}"
fail() { echo "FAIL ($fam): $*" >&2; exit 1; }
name=agentry-desktop
bin=/usr/bin/agentry-desktop
[ -x "$bin" ] || fail "$bin is not there after installing the previous package"

# 6 (setup). A populated home, before the upgrade.
export HOME=/tmp/smoke-home
mkdir -p "$HOME/.config/agentry/keys" "$HOME/.agentry/runner/profiles"
printf 'defaultRegistry: https://dev.example.net\nonboarded: true\n' > "$HOME/.config/agentry/desktop.yaml"
printf 'installs: []\n' > "$HOME/.config/agentry/installed.yaml"
printf 'dev.example.net pin\n' > "$HOME/.config/agentry/keys/dev.example.net.pub"
printf 'version: 1\nmodel:\n  provider: openai\n' > "$HOME/.agentry/runner/policy.yaml"
snapshot() { (cd "$HOME" && find .config .agentry -type f -exec sha256sum {} + | sort); }
before=$(snapshot)

# 1. The real package over the previous one, as an upgrade.
case "$fam" in
  deb) prev=$(dpkg-query -W -f '${Version}' "$name")
       apt-get install -y -qq --no-install-recommends /pkg/pkg.deb >/dev/null
       ver=$(dpkg-query -W -f '${Version}' "$name")
       dpkg --compare-versions "$ver" gt "$prev" || fail "the package version $ver does not sort above the previous $prev" ;;
  rpm) prev=$(rpm -q --qf '%{VERSION}-%{RELEASE}' "$name")
       rpm -Uvh /pkg/pkg.rpm >/dev/null
       ver=$(rpm -q --qf '%{VERSION}-%{RELEASE}' "$name")
       [ "$(rpm --eval "%{lua:print(rpm.vercmp('$ver', '$prev'))}")" = 1 ] || fail "the package version $ver does not sort above the previous $prev" ;;
esac
# A build past a tag must also sort above that tag and below the next patch.
base="${ver%%[+~]*}"; base="${base%-1}"
if [ "$base" != "${ver%-1}" ]; then
  next="${base%.*}.$(( ${base##*.} + 1 ))"
  case "$fam" in
    deb) dpkg --compare-versions "$ver" gt "$base" && dpkg --compare-versions "$ver" lt "$next" || fail "$ver does not sort between $base and $next" ;;
    rpm) [ "$(rpm --eval "%{lua:print(rpm.vercmp('$ver', '$base'))}")" = 1 ] && [ "$(rpm --eval "%{lua:print(rpm.vercmp('$ver', '$next'))}")" = -1 ] || fail "$ver does not sort between $base and $next" ;;
  esac
fi
echo "ok ($fam): upgraded $prev -> $ver, ordered as an upgrade"

# 2. What the package owns, and what it does not.
case "$fam" in
  deb) files=$(dpkg -L "$name" | grep -v '^/\.$'); scripts=$(dpkg-deb --ctrl-tarfile /pkg/pkg.deb | tar -t | sed 's#^\./##' | grep -E '^(preinst|postinst|prerm|postrm|triggers|config)$' || true) ;;
  rpm) files=$(rpm -ql "$name"); scripts=$(rpm -q --scripts --triggers "$name") ;;
esac
[ -x "$bin" ] || fail "$bin is not an executable"
[ -f /usr/share/applications/agentry-desktop.desktop ] || fail "no desktop entry"
desktop-file-validate /usr/share/applications/agentry-desktop.desktop || fail "the desktop entry does not validate"
[ -f /usr/share/icons/hicolor/512x512/apps/agentry-desktop.png ] || fail "no 512px icon"
cmp /usr/share/doc/agentry-desktop/THIRD_PARTY_NOTICES /pkg/THIRD_PARTY_NOTICES || fail "the package's third-party notices are missing or differ from THIRD_PARTY_NOTICES"
if printf '%s\n' "$files" | grep -E '^/(home|root)(/|$)'; then fail "the package owns something under a home directory"; fi
owned='^/usr$|^/usr/(bin|share)(/|$)'
[ ! -d /pkg/whisper ] || owned+='|^/usr/lib$|^/usr/lib/agentry-desktop(/|$)'
if printf '%s\n' "$files" | grep -v -E "$owned"; then fail "the package owns something outside /usr/bin, /usr/share and its transcriber's folder"; fi
[ -z "$scripts" ] || fail "the package carries maintainer scripts or triggers: $scripts"
echo "ok ($fam): owns $(printf '%s\n' "$files" | grep -c -v -E '/$') paths under /usr, no scripts, valid desktop entry"

# 2b. The agentry CLI, when the build carries one: /usr/bin/agentry is that
#     release byte for byte, and runs; without one, the package has none.
if [ -f /pkg/agentry.cli ]; then
  cmp /usr/bin/agentry /pkg/agentry.cli || fail "/usr/bin/agentry differs from the CLI the build carried"
  agentry --version >/tmp/cli.out 2>&1 || { cat /tmp/cli.out >&2; fail "agentry --version failed"; }
  echo "ok ($fam): /usr/bin/agentry is the carried CLI, on PATH: $(head -1 /tmp/cli.out)"
else
  [ ! -e /usr/bin/agentry ] || fail "the package carries /usr/bin/agentry but the build carried no CLI"
fi

# 2c. The transcriber, when the build carries one: every file and link of
#     /usr/lib/agentry-desktop/whisper is the build's, and whisper-cli runs;
#     without one, the package has none.
side=/usr/lib/agentry-desktop/whisper
if [ -d /pkg/whisper ]; then
  for f in /pkg/whisper/*; do
    n=$(basename "$f")
    [ "$n" != SHA256SUMS ] || continue
    if [ -L "$f" ]; then
      [ "$(readlink "$side/$n")" = "$(readlink "$f")" ] || fail "$side/$n is not the link the build carried"
    else
      cmp "$side/$n" "$f" || fail "$side/$n differs from the transcriber the build carried"
    fi
  done
  "$side/whisper-cli" --help >/tmp/whisper.out 2>&1 || { tail -5 /tmp/whisper.out >&2; fail "whisper-cli does not run"; }
  echo "ok ($fam): $side is the carried transcriber, and whisper-cli runs"
else
  [ ! -e /usr/lib/agentry-desktop ] || fail "the package carries a transcriber but the build carried none"
fi

# 3. The same binary as the bare artifact.
cmp "$bin" /pkg/agentry-desktop.bin || fail "the installed binary differs from the bare artifact"
echo "ok ($fam): installed binary is byte-identical to the bare artifact"

# 4a. Every linked library resolves. (ldd once, into a variable, read from a
#     here-string: a grep -q on a live pipe, printf's included, can close it
#     early and pipefail would read the EPIPE as the answer.)
ldd_out=$(ldd "$bin")
if grep -q 'not found' <<<"$ldd_out"; then grep 'not found' <<<"$ldd_out" >&2; fail "a linked library is missing after installing only the package"; fi
needed=$(readelf -d "$bin" | sed -n 's/.*(NEEDED).*\[\(.*\)\]/\1/p')
[ -n "$needed" ] || fail "readelf found no NEEDED entries"
paths=$(for lib in $needed; do printf '%s\n' "$ldd_out" | awk -v l="$lib" '$1==l {print $3}'; done)
[ "$(printf '%s\n' "$paths" | grep -c .)" = "$(printf '%s\n' "$needed" | grep -c .)" ] || fail "could not resolve every NEEDED library's path"
echo "ok ($fam): all $(printf '%s\n' "$needed" | grep -c .) linked libraries resolve"
# The transcriber's libraries: those it carries itself are its own, and the
# Vulkan backend is left out, since it is loaded at run time and the CPU
# runs where its loader is missing.
if [ -d /pkg/whisper ]; then
  for elf in "$side/whisper-cli" "$side"/*.so*; do
    [ -f "$elf" ] && [ ! -L "$elf" ] && [ "$(basename "$elf")" != libggml-vulkan.so ] || continue
    out=$(ldd "$elf")
    if grep -q 'not found' <<<"$out"; then grep 'not found' <<<"$out" >&2; fail "$elf links a library that is missing after installing only the package"; fi
    paths+=$'\n'$(awk -v own="$side/" '$2=="=>" && index($3, own)!=1 {print $3}' <<<"$out")
  done
  paths=$(printf '%s\n' "$paths" | grep . | sort -u)
  echo "ok ($fam): the transcriber's linked libraries resolve"
fi

# 4b. The declared dependencies are exactly the owners of what is linked.
if [ "$strict" = strict ]; then
  case "$fam" in
    # dpkg -S knows one spelling of a path (bookworm's /lib is a symlink to
    # /usr/lib and libc is registered under the former), so both are asked
    # and a miss on one is not a failure.
    deb) owners=$(for p in $paths; do { dpkg -S "$(readlink -f "$p")" 2>/dev/null || dpkg -S "$p" 2>/dev/null || true; } | cut -d: -f1; done | sort -u)
         declared=$(dpkg-query -W -f '${Depends}' "$name" | tr ',' '\n' | sed 's/([^)]*)//; s/^ *//; s/ *$//' | sort -u) ;;
    rpm) owners=$(for p in $paths; do { rpm -qf --qf '%{NAME}\n' "$p" 2>/dev/null || rpm -qf --qf '%{NAME}\n' "$(readlink -f "$p")" 2>/dev/null || true; } | grep -v 'not owned'; done | sort -u)
         declared=$(rpm -qR "$name" | grep -v -E '^rpmlib\(' | sed 's/ .*//' | sort -u) ;;
  esac
  [ -n "$owners" ] || fail "no package owns the linked libraries (the lookup found nothing)"
  if [ "$owners" != "$declared" ]; then
    echo "declared:"; echo "$declared"; echo "owners of the linked libraries:"; echo "$owners"
    fail "the declared dependencies are not exactly the packages that own the linked libraries"
  fi
  echo "ok ($fam): declared dependencies == owners of the linked libraries: $(echo "$declared" | tr '\n' ' ')"
fi

# 5. Launch smoke under a virtual display: alive at the timeout (124).
timeout 6 xvfb-run -a "$bin" >/tmp/run.log 2>&1 && ec=$? || ec=$?
if [ "$ec" != 124 ]; then tail -5 /tmp/run.log >&2; fail "the app exited with $ec before the 6 s timeout"; fi
echo "ok ($fam): the app ran under Xvfb until the timeout"

# 6. The home after the upgrade, and after the removal.
after=$(snapshot)
[ "$before" = "$after" ] || { diff <(echo "$before") <(echo "$after") >&2; fail "the upgrade changed the home"; }
case "$fam" in
  deb) apt-get remove -y -qq "$name" >/dev/null ;;
  rpm) rpm -e "$name" ;;
esac
[ ! -e "$bin" ] || fail "$bin survived the removal"
[ ! -e /usr/share/applications/agentry-desktop.desktop ] || fail "the desktop entry survived the removal"
[ ! -e /usr/share/doc/agentry-desktop/THIRD_PARTY_NOTICES ] || fail "the third-party notices survived the removal"
[ ! -e /usr/bin/agentry ] || fail "/usr/bin/agentry survived the removal"
[ ! -e /usr/lib/agentry-desktop ] || fail "the transcriber's folder survived the removal"
after=$(snapshot)
[ "$before" = "$after" ] || { diff <(echo "$before") <(echo "$after") >&2; fail "removing changed the home"; }
echo "ok ($fam): upgrade and removal left every configuration byte in place"
EOS

# label=disable: on a host whose Docker daemon runs with SELinux (the CI
# runner host), a bind mount is otherwise unreadable from inside.
run() { docker run --rm --security-opt label=disable -v "$PWD/$stage:/pkg:ro" "$@"; }

# The Debian family: the previous package first, then the harness — xvfb,
# xauth and dbus for the launch, binutils for readelf, desktop-file-utils
# for the entry's validator — with the package's own dependencies resolved
# by apt and nothing else named by hand.
deb_family() {
  run "$1" bash -ec '
    export DEBIAN_FRONTEND=noninteractive
    # The slim Debian and the minimal Ubuntu images tell dpkg to leave out
    # /usr/share/doc, where the package installs the third-party notices; a
    # real system keeps them.
    rm -f /etc/dpkg/dpkg.cfg.d/docker /etc/dpkg/dpkg.cfg.d/excludes
    apt-get update -qq >/dev/null
    apt-get install -y -qq --no-install-recommends /pkg/previous.deb xvfb xauth dbus binutils desktop-file-utils >/dev/null 2>/tmp/apt.err || { cat /tmp/apt.err >&2; exit 1; }
    bash /pkg/common.sh deb '"$2"'
  '
}

echo "== .deb in debian:bookworm-slim"
deb_family debian:bookworm-slim strict
echo "== .deb in ubuntu:24.04 (the t64 names must Provide the declared ones)"
deb_family ubuntu:24.04 ""

echo "== .rpm in fedora:44"
run fedora:44 bash -ec '
  # The previous package and its own dependencies; Xvfb is the smoke harness, binutils is readelf, diffutils is cmp, desktop-file-utils the validator.
  dnf install -y -q /pkg/previous.rpm xorg-x11-server-Xvfb binutils diffutils desktop-file-utils >/dev/null 2>/tmp/dnf.err || { cat /tmp/dnf.err >&2; exit 1; }
  # xvfb-run is not packaged on Fedora; the same shape in a few lines.
  cat > /usr/local/bin/xvfb-run <<'"'"'EOF'"'"'
#!/usr/bin/env bash
[ "${1:-}" = -a ] && shift
Xvfb :99 -screen 0 1280x800x24 >/dev/null 2>&1 & xp=$!
trap "kill $xp 2>/dev/null" EXIT
sleep 1; DISPLAY=:99 "$@"
EOF
  chmod +x /usr/local/bin/xvfb-run
  bash /pkg/common.sh rpm strict
'
echo "linux-packages: every family passes — upgraded from the previous package, same binary, dependencies exact where checked, launched, configuration survives upgrade and removal"
