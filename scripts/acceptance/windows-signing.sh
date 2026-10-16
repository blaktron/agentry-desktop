#!/usr/bin/env bash
# Acceptance for the Windows signing seam: SIGN_CMD, a command that signs the
# one PE file it is given in place. It proves that the Makefile's
# sign-windows target hands it the application exe before makensis embeds
# it, and that project.nsi's two hooks hand it the uninstaller before it is
# embedded and the installer once it is written; that the installer then
# carries exactly what the command left in each file; and that a command
# that fails fails the installer build. A stand-in that appends a marker to
# each file plays the signer, so it needs no key and no network.
#
#   scripts/acceptance/windows-signing.sh <agentry-desktop.exe>
#
# Needs makensis and 7z. On a machine without them, run it in a container:
#
#   docker run --rm --security-opt label=disable -v "$PWD:/w" -w /w ubuntu:24.04 bash -c \
#     'apt-get update -qq && apt-get install -y -qq --no-install-recommends nsis p7zip-full make >/dev/null && scripts/acceptance/windows-signing.sh build/bin/agentry-desktop.exe'
set -euo pipefail

exe="${1:?usage: $0 <agentry-desktop.exe>}"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fail() { echo "windows-signing acceptance: $*" >&2; exit 1; }
[ -f "$exe" ] || fail "$exe does not exist"
# makensis runs from the installer directory, so the exe goes by its full path.
exe="$(cd "$(dirname "$exe")" && pwd)/$(basename "$exe")"
for t in makensis 7z; do command -v "$t" >/dev/null 2>&1 || fail "$t is needed"; done

T=$(mktemp -d)
installer_dir="$root_dir/build/windows/installer"
built="$root_dir/build/bin/agentry-desktop-amd64-installer.exe"
trap 'rm -rf "$T" "$installer_dir/tmp"; rm -f "$built"' EXIT

# The stand-in signer: the same contract (one PE file, changed in place),
# recording what it was handed and in what order.
cat > "$T/sign.sh" <<SH
#!/usr/bin/env bash
set -euo pipefail
printf 'SIGNED-BY-TEST' >> "\$1"
case "\$(basename "\$1")" in
  agentry-desktop.exe) echo exe ;;
  *-installer.exe) echo installer ;;
  *) cp "\$1" "$T/uninstall.signed"; echo uninstaller ;;
esac >> "$T/order"
SH
chmod +x "$T/sign.sh"

# What wails' -nsis step would have left for makensis: the bootstrapper file
# (any PE does for the embedding) and the checked-in wails_tools.nsh.
mkdir -p "$installer_dir/tmp" "$T/bin" "$root_dir/build/bin"
cp "$exe" "$installer_dir/tmp/MicrosoftEdgeWebview2Setup.exe"
cp "$exe" "$T/bin/agentry-desktop.exe"

# 1. The Makefile's order: the exe, then makensis, whose hooks take the
# uninstaller and then the installer.
"$T/sign.sh" "$T/bin/agentry-desktop.exe"
( cd "$installer_dir" && makensis -V2 -DARG_WAILS_AMD64_BINARY="$T/bin/agentry-desktop.exe" -DSIGN_CMD="$T/sign.sh" project.nsi )
[ "$(tr '\n' ' ' < "$T/order")" = "exe uninstaller installer " ] || fail "the signer was handed $(tr '\n' ' ' < "$T/order")instead of the exe, the uninstaller and the installer, in that order"
echo "windows-signing acceptance: the exe, the uninstaller and the installer each went to SIGN_CMD, in that order"

# 2. The installer carries what the signer left: the marked exe and the
# marked uninstaller, and is marked itself.
7z e -so "$built" agentry-desktop.exe > "$T/app-in-setup.exe" 2>/dev/null || fail "could not extract agentry-desktop.exe from the installer"
7z e -so "$built" uninstall.exe > "$T/uninstall-in-setup.exe" 2>/dev/null || fail "could not extract uninstall.exe from the installer"
cmp -s "$T/app-in-setup.exe" "$T/bin/agentry-desktop.exe" || fail "the installer does not carry the exe as the signer left it"
cmp -s "$T/uninstall-in-setup.exe" "$T/uninstall.signed" || fail "the installer does not embed the uninstaller as the signer left it"
[ "$(tail -c 14 "$built")" = SIGNED-BY-TEST ] || fail "the installer is not the file the signer was handed last"
echo "windows-signing acceptance: the installer embeds the exe and the uninstaller as the signer left them"

# 3. A signer that fails makes makensis fail (the hooks' "= 0"): no
# half-signed installer is left behind.
rm -f "$built"
printf '#!/usr/bin/env bash\nexit 1\n' > "$T/failing-sign.sh"; chmod +x "$T/failing-sign.sh"
if ( cd "$installer_dir" && makensis -V2 -DARG_WAILS_AMD64_BINARY="$T/bin/agentry-desktop.exe" -DSIGN_CMD="$T/failing-sign.sh" project.nsi >/dev/null 2>&1 ); then
  fail "makensis succeeded although the signer failed"
fi
echo "windows-signing acceptance: a failing signer fails the installer build"

# 4. Without SIGN_CMD the installer builds exactly as it does unsigned.
( cd "$installer_dir" && makensis -V2 -DARG_WAILS_AMD64_BINARY="$exe" project.nsi )
7z e -so "$built" agentry-desktop.exe 2>/dev/null | cmp -s - "$exe" || fail "an installer built without SIGN_CMD does not carry the exe as given"

echo "windows-signing acceptance: ok — SIGN_CMD signs the exe, the uninstaller and the installer, the installer carries what it signed, and a failing signer fails the build"
