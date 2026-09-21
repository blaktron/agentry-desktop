#!/usr/bin/env bash
# Acceptance for the Windows signing seam (agentry-desktop#54): proves that
# the NSIS hooks sign the installer and the uninstaller, that an exe signed
# before makensis is the one the installer embeds, and that
# check-windows-installer.sh --signed accepts exactly that and refuses the
# two ways it can be wrong — with a throwaway CA and osslsigncode standing in
# for Artifact Signing and jsign, so it runs anywhere makensis does and needs
# no credential. The real key is only ever used by the release workflow.
#
#   scripts/acceptance/windows-signing.sh <agentry-desktop.exe>
#
# Needs makensis, osslsigncode, 7z, openssl, zip and network for a public
# timestamp authority (the checker insists on a timestamp, as the release
# must). On a workstation without those, run it in a container:
#
#   docker run --rm --security-opt label=disable -v "$PWD:/w" -w /w ubuntu:24.04 bash -c \
#     'apt-get update -qq && apt-get install -y -qq --no-install-recommends nsis p7zip-full osslsigncode openssl zip ca-certificates >/dev/null && scripts/acceptance/windows-signing.sh build/bin/agentry-desktop.exe'
set -euo pipefail

exe="${1:?usage: $0 <agentry-desktop.exe>}"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fail() { echo "windows-signing acceptance: $*" >&2; exit 1; }
[ -f "$exe" ] || fail "$exe does not exist"
for t in makensis osslsigncode 7z openssl zip; do command -v "$t" >/dev/null 2>&1 || fail "$t is needed"; done

T=$(mktemp -d)
installer_dir="$root_dir/build/windows/installer"
trap 'rm -rf "$T" "$installer_dir/tmp"; rm -f "$root_dir/build/bin/agentry-desktop-amd64-installer.exe"' EXIT

# A root and a leaf with the code-signing EKU, two days of life.
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$T/ca.key" -out "$T/ca.pem" -days 2 -subj "/CN=Agentry Test Root" \
  -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" 2>/dev/null
openssl req -newkey rsa:2048 -nodes -keyout "$T/leaf.key" -out "$T/leaf.csr" -subj "/C=CA/CN=Agentry Test Signer" 2>/dev/null
printf 'extendedKeyUsage=codeSigning\nkeyUsage=digitalSignature\nbasicConstraints=CA:FALSE\n' > "$T/leaf.ext"
openssl x509 -req -in "$T/leaf.csr" -CA "$T/ca.pem" -CAkey "$T/ca.key" -CAcreateserial -out "$T/leaf.pem" -days 2 -extfile "$T/leaf.ext" 2>/dev/null

# The stand-in for scripts/sign-windows.sh: the same contract (one PE file,
# signed in place), a public TSA for the timestamp, two tried in turn.
cat > "$T/sign.sh" <<SH
#!/usr/bin/env bash
set -euo pipefail
f="\$1"
for tsa in http://timestamp.digicert.com http://timestamp.sectigo.com; do
  if osslsigncode sign -certs "$T/leaf.pem" -key "$T/leaf.key" -n "Agentry Desktop" -i https://secureagentry.net -ts "\$tsa" -in "\$f" -out "\$f.signed" >/dev/null 2>&1; then
    mv "\$f.signed" "\$f"; echo "test-sign: signed \$(basename "\$f") (timestamp \$tsa)"; exit 0
  fi
done
echo "test-sign: no timestamp authority answered" >&2; exit 1
SH
chmod +x "$T/sign.sh"

# What wails' -nsis step would have left for makensis: the bootstrapper file
# (any PE does for the embedding) and the checked-in wails_tools.nsh.
mkdir -p "$installer_dir/tmp"
cp "$exe" "$installer_dir/tmp/MicrosoftEdgeWebview2Setup.exe"
cp "$exe" "$T/agentry-desktop.exe"

# 1. The exe signed before makensis; 2. makensis with the hooks.
"$T/sign.sh" "$T/agentry-desktop.exe"
( cd "$installer_dir" && makensis -V2 -DARG_WAILS_AMD64_BINARY="$T/agentry-desktop.exe" -DSIGN_CMD="$T/sign.sh" project.nsi )
setup="$T/agentry-desktop-0.0.0-windows-amd64-setup.exe"
mv "$root_dir/build/bin/agentry-desktop-amd64-installer.exe" "$setup"

# 3. The checker accepts the signed set and reports what it found.
export SIGNING_CAFILE="$T/ca.pem" SIGNING_TSA_CAFILE=/etc/ssl/certs/ca-certificates.crt
"$root_dir/scripts/check-windows-installer.sh" --signed --report "$T/report.txt" "$setup" "$T/agentry-desktop.exe"
grep -q '^cn=Agentry Test Signer$' "$T/report.txt" || { cat "$T/report.txt"; fail "the report does not name the signer"; }
grep -q '^timestamp=' "$T/report.txt" || fail "the report carries no timestamp"

# 4. Refused: an unsigned application binary beside the signed installer.
cp "$exe" "$T/unsigned.exe"
if "$root_dir/scripts/check-windows-installer.sh" --signed "$setup" "$T/unsigned.exe" >/dev/null 2>&1; then
  fail "an unsigned application binary was accepted"
fi
echo "windows-signing acceptance: unsigned application binary refused"

# 5. Refused: a chain to a root that is not the pinned one.
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$T/other.key" -out "$T/other.pem" -days 2 -subj "/CN=Other Root" 2>/dev/null
if SIGNING_CAFILE="$T/other.pem" "$root_dir/scripts/check-windows-installer.sh" --signed "$setup" "$T/agentry-desktop.exe" >/dev/null 2>&1; then
  fail "a signature chaining to another root was accepted"
fi
echo "windows-signing acceptance: a chain to another root refused"

# 6. Without --signed the same package still passes the unsigned checks.
unset SIGNING_CAFILE SIGNING_TSA_CAFILE
"$root_dir/scripts/check-windows-installer.sh" "$setup" "$T/agentry-desktop.exe" >/dev/null

# 7. The committed root is the pinned one.
fp=$(openssl x509 -in "$root_dir/build/windows/signing/microsoft-identity-verification-root-2020.pem" -noout -fingerprint -sha256 | sed 's/^.*=//')
[ "$fp" = "53:67:F2:0C:7A:DE:0E:2B:CA:79:09:15:05:6D:08:6B:72:0C:33:C1:FA:2A:26:61:AC:F7:87:E3:29:2E:12:70" ] || fail "the committed Microsoft root has fingerprint $fp, not the pinned one"

echo "windows-signing acceptance: ok — exe, installer and uninstaller signed through the hooks and verified; two refusals proven; the pinned root is the committed one"
