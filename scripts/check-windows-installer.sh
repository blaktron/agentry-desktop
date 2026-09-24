#!/usr/bin/env bash
# Reads the built Windows installer back and checks it is the one the plan
# describes (agentry-desktop#83; agentry-notes/plans/desktop-installers.md
# §3, §5, §8). This is as far as a Linux runner can check a Windows package
# without Windows: the install-over-install and uninstall checks run by hand
# on a Windows machine until #55 gives CI one.
#
#   scripts/check-windows-installer.sh [--signed] [--report FILE] <setup.exe> <agentry-desktop.exe> [project.nsi]
#   scripts/check-windows-installer.sh --self-test
#
# What it proves:
#   1. the file name is one the site's manifest admits
#      (agentry-web/scripts/release/manifest.py NAME: -setup.exe is "setup");
#   2. the installer file carries an asInvoker execution-level request —
#      NSIS's spelling of RequestExecutionLevel user — and no elevated one
#      (a grep over the file: it cannot tell the stub's manifest from a
#      payload's, which is why the elevated forms are refused too);
#   3. it installs exactly two files, the application and its uninstaller
#      (everything else is NSIS plumbing under $PLUGINSDIR, the WebView2
#      bootstrapper included);
#   4. the application it carries is byte-identical to the binary the zip
#      carries: the installer is not a different build;
#   5. the script's install section reaches nothing under the person's
#      profile, the settings section is off by default, and the application
#      section cannot be unticked;
#   6. with --signed (agentry-desktop#54): the installer, the application it
#      carries and the uninstaller it embeds each carry an Authenticode
#      signature that osslsigncode verifies to the pinned Microsoft Identity
#      Verification root (build/windows/signing/), timestamped, all three by
#      the same subject — and the subject's CN is AGENTRY_WINDOWS_SIGNING_CN
#      when that is in the environment (the release exports it from
#      build/release.env). --report FILE writes what was found (cn=,
#      timestamp=, root=) for the release's BUILD-INFO. Without --signed the
#      package is reported unsigned and nothing is asserted about
#      signatures, which is what a PR build is. Needs osslsigncode 2.7 or
#      later (both spellings of its timestamp line are read).
#      Overrides for a test harness only: SIGNING_CAFILE (the root to verify
#      against; the fingerprint pin is then skipped) and SIGNING_TSA_CAFILE
#      (the timestamp chain's root, default: the same file).
set -euo pipefail

# signer_cn <osslsigncode report>: the signer's CN, in either subject form
# the tool prints — 2.8's OpenSSL slash form (/C=CA/…/CN=Name, the Ubuntu
# runner's) and 2.12's RFC 2253 comma form (CN=Name,O=…,C=CA, Fedora 44's;
# a comma inside a value is escaped with a backslash and a component
# boundary is an unescaped comma). The two forms list components in
# opposite orders (RFC 2253 reverses), so both branches pick the same
# component: the certificate's first CN — the last in the slash form, the
# first in the comma form. Empty when the subject carries no CN in the form
# it uses — never a guess from the other form (agentry-desktop#171; the
# site's manifest.py and the CLI's check-windows-binary.sh read the same).
signer_cn() {
  local subject cn
  subject=$(printf '%s\n' "$1" | awk '/Signer.s certificate/{on=1} on && /Subject:/{sub(/^[[:space:]]*Subject:[[:space:]]*/, ""); print; exit}')
  case "$subject" in
    /*) cn=$(printf '%s\n' "$subject" | sed -n 's#.*/CN=\([^/]*\).*#\1#p') ;;
    *)  # Escaped commas are hidden from the split (\x01), then the first
        # component that is a CN wins and the value is unescaped.
        cn=$(printf '%s\n' "$subject" | sed 's/\\,/\x01/g' \
          | awk -F',' '{for (i = 1; i <= NF; i++) {f = $i; sub(/^[[:space:]]+/, "", f); if (f ~ /^CN=/) {print substr(f, 4); exit}}}' \
          | sed -e 's/\x01/,/g' -e 's/\\\(.\)/\1/g') ;;
  esac
  printf '%s' "$cn" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//'
}

# --self-test: the CN parser over the two report forms, no osslsigncode
# needed — what the seam acceptance cannot exercise on a runner that has
# only one of the two versions.
if [ "${1:-}" = "--self-test" ]; then
  fails=0
  check() {  # check <expected> <report>
    local got; got=$(signer_cn "$2")
    if [ "$got" != "$1" ]; then echo "self-test: expected [$1], got [$got] for: $2" >&2; fails=$((fails + 1)); fi
  }
  check "Chris Dupres" $'Signer\'s certificate:\n\tSigner #0:\n\t\tSubject: /C=CA/ST=on/L=Ottawa/O=Chris Dupres/CN=Chris Dupres\n'
  check "Chris Dupres" $'Signer\'s certificate:\n\tSigner #0:\n\t\tSubject: CN=Chris Dupres,O=Chris Dupres,L=Ottawa,ST=on,C=CA\n'
  check "Dupres, Chris" $'Signer\'s certificate:\n\tSigner #0:\n\t\tSubject: O=Acme,CN=Dupres\\, Chris,C=CA\n'
  check "" $'Signer\'s certificate:\n\tSigner #0:\n\t\tSubject: O=Acme\\,CN=x,C=CA\n'
  check "" $'Signer\'s certificate:\n\tSigner #0:\n\t\tSubject: OU=CN=notacn,O=x\n'
  check "" $'Signer\'s certificate:\n\tSigner #0:\n\t\tSubject: /C=CA/O=Nobody\n'
  check "" $'Number of verified signatures: 1\n'
  # Two CNs: the certificate's first, in both orders.
  check "First" $'Signer\'s certificate:\n\tSigner #0:\n\t\tSubject: /C=CA/CN=Second/O=x/CN=First\n'
  check "First" $'Signer\'s certificate:\n\tSigner #0:\n\t\tSubject: CN=First,O=x,CN=Second,C=CA\n'
  [ "$fails" -eq 0 ] || exit 1
  echo "check-windows-installer --self-test: ok (the signer's CN in both osslsigncode subject forms; no CN is no CN)"
  exit 0
fi

signed=0; report=""
while [ $# -gt 0 ]; do
  case "$1" in
    --signed) signed=1; shift ;;
    --report) report="${2:?--report needs a file}"; shift 2 ;;
    --) shift; break ;;
    -*) echo "check-windows-installer: unknown option $1" >&2; exit 2 ;;
    *) break ;;
  esac
done
setup="${1:?usage: $0 [--signed] [--report FILE] <setup.exe> <agentry-desktop.exe> [project.nsi]}"
exe="${2:?usage: $0 [--signed] [--report FILE] <setup.exe> <agentry-desktop.exe> [project.nsi]}"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
nsi="${3:-$root_dir/build/windows/installer/project.nsi}"

fail() { echo "check-windows-installer: $*" >&2; exit 1; }

[ -f "$setup" ] || fail "$setup does not exist"
[ -f "$exe" ] || fail "$exe does not exist"
[ -f "$nsi" ] || fail "$nsi does not exist"
command -v 7z >/dev/null 2>&1 || fail "7z is needed to read the installer (apt-get install p7zip-full; dnf install p7zip p7zip-plugins)"

# 1. The name.
name=$(basename "$setup")
[[ "$name" =~ ^agentry-desktop-.+-windows-amd64-setup\.exe$ ]] || fail "$name is not agentry-desktop-<version>-windows-amd64-setup.exe, which the download manifest would drop"

# 2. User-level execution, read from the built installer file, not the source.
grep -a -q 'requestedExecutionLevel level="asInvoker"' "$setup" || fail "the installer's manifest does not request asInvoker (RequestExecutionLevel user)"
if grep -a -q -E 'requestedExecutionLevel level="(requireAdministrator|highestAvailable)"' "$setup"; then
  fail "the installer's manifest asks for elevation"
fi

# 3. The file list: two files outside $PLUGINSDIR, these two. -slt lists the
# archive's own path before the ---------- separator; the entries come after
# it. Listed once into a variable: a grep -q on a live pipe would close it
# early and pipefail would read 7z's SIGPIPE as a failure. 7-Zip builds
# differ on the separator they print, so both are read as /.
#
# No `… | grep -q` anywhere in this script (agentry-desktop#236): grep -q
# exits at its first match, and a writer still writing gets EPIPE, which
# pipefail reports as the pipeline's failure. printf of a variable is such a
# writer too: bash's stdout is line-buffered, so a multi-line value leaves in
# several write()s, and on a loaded runner grep can match on the first and
# exit before the rest ("printf: write error: Broken pipe", then the wrong
# failure message). A value is read from a here-string (grep -q … <<<"$x");
# a command's output goes to a grep that reads it all (grep … >/dev/null).
entries=$(7z l -slt "$setup" | awk '/^----------$/{on=1; next} on && /^Path = /{sub(/^Path = /, ""); print}' | tr '\\' '/')
listing=$(printf '%s\n' "$entries" | grep -v '^\$PLUGINSDIR' || true)
expected=$'agentry-desktop.exe\nuninstall.exe'
if [ "$(printf '%s\n' "$listing" | sort)" != "$(printf '%s\n' "$expected" | sort)" ]; then
  printf 'installed files:\n%s\n' "$listing" >&2
  fail "the installer does not install exactly agentry-desktop.exe and uninstall.exe"
fi
grep -q '^\$PLUGINSDIR/webview2bootstrapper/MicrosoftEdgeWebview2Setup.exe$' <<<"$entries" || fail "the WebView2 bootstrapper is not in the installer"

# 4. The same binary as the zip.
if ! 7z e -so "$setup" agentry-desktop.exe 2>/dev/null | cmp -s - "$exe"; then
  fail "the agentry-desktop.exe inside the installer differs from $exe"
fi

# 5. The script itself: what the install section may touch.
grep -q -E '^!define REQUEST_EXECUTION_LEVEL "user"' "$nsi" || fail "$nsi does not define REQUEST_EXECUTION_LEVEL user"
grep -q -E '^!define WAILS_INSTALL_SCOPE "user"' "$nsi" || fail "$nsi does not define WAILS_INSTALL_SCOPE user"
install_section=$(awk '/^Section[[:space:]]*$/{on=1} on{print} /^SectionEnd/{if(on){exit}}' "$nsi")
[ -n "$install_section" ] || fail "no install section found in $nsi"
grep -q 'wails.files' <<<"$install_section" || fail "the install section in $nsi does not copy the files (the awk found the wrong block)"
if grep -v -E '^[[:space:]]*[#;]' <<<"$install_section" | grep -E '\$PROFILE|\$APPDATA|\$LOCALAPPDATA|\.config|\.agentry|Credential|RMDir|Delete ' >/dev/null; then
  printf '%s\n' "$install_section" >&2
  fail "the install section reaches beyond \$INSTDIR, the Start Menu and the uninstall key"
fi
grep -q -E '^Section /o "un\.Also remove my Agentry settings and sign-in"' "$nsi" || fail "the settings section is not off by default (/o)"
awk '/^Section "un\./{on=1} on{print} /^SectionEnd/{if(on){exit}}' "$nsi" | grep 'SectionIn RO' >/dev/null || fail "the application section can be unticked"
grep -q -- '--remove-my-settings' "$nsi" || fail "the settings section does not hand the removal to the app"

if [ "$signed" -eq 0 ]; then
  echo "check-windows-installer: $name ok — asInvoker, installs agentry-desktop.exe + uninstall.exe, same binary as $(basename "$exe"), install section stays in \$INSTDIR; unsigned (no --signed)"
  exit 0
fi

# 6. The three signatures. osslsigncode is handed the pinned Microsoft root
# because Ubuntu's ca-certificates does not carry it; the pin is the
# fingerprint in build/windows/signing/README.md, checked before the file is
# trusted with anything.
command -v osslsigncode >/dev/null 2>&1 || fail "osslsigncode is needed to verify the signatures (apt-get install osslsigncode)"
command -v openssl >/dev/null 2>&1 || fail "openssl is needed to check the pinned root"
pinned_root="$root_dir/build/windows/signing/microsoft-identity-verification-root-2020.pem"
pinned_fp="53:67:F2:0C:7A:DE:0E:2B:CA:79:09:15:05:6D:08:6B:72:0C:33:C1:FA:2A:26:61:AC:F7:87:E3:29:2E:12:70"
ca="${SIGNING_CAFILE:-$pinned_root}"
tsa_ca="${SIGNING_TSA_CAFILE:-$ca}"
[ -f "$ca" ] || fail "$ca does not exist"
if [ -z "${SIGNING_CAFILE:-}" ]; then
  fp=$(openssl x509 -in "$ca" -noout -fingerprint -sha256 | sed 's/^.*=//')
  [ "$fp" = "$pinned_fp" ] || fail "$ca is not the pinned Microsoft Identity Verification root (fingerprint $fp)"
fi
root_subject=$(openssl x509 -in "$ca" -noout -subject | sed 's/^subject=//')

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# verify_pe <file> <label>: osslsigncode verify against the pinned root and
# the timestamp root; prints "<cn>|<timestamp>" on success.
verify_pe() {
  local f=$1 label=$2 out cn ts
  if ! out=$(osslsigncode verify -in "$f" -CAfile "$ca" -TSA-CAfile "$tsa_ca" 2>&1); then
    printf '%s\n' "$out" >&2
    fail "the signature on the $label does not verify to $root_subject"
  fi
  grep -q 'Signature verification: ok' <<<"$out" || { printf '%s\n' "$out" >&2; fail "osslsigncode did not report the $label's signature ok"; }
  cn=$(signer_cn "$out")
  [ -n "$cn" ] || { printf '%s\n' "$out" >&2; fail "could not read the signer's CN from osslsigncode's report on the $label"; }
  ts=$(printf '%s\n' "$out" | sed -n -e 's/^[[:space:]]*Timestamp time: //p' -e 's/^The signature is timestamped: //p' | head -1)
  [ -n "$ts" ] || { printf '%s\n' "$out" >&2; fail "the signature on the $label is not timestamped — an Artifact Signing certificate lives three days, so the signature would die with it"; }
  grep -q 'Timestamp Server Signature verification: ok' <<<"$out" || { printf '%s\n' "$out" >&2; fail "the timestamp on the $label does not verify"; }
  printf '%s|%s\n' "$cn" "$ts"
}

setup_sig=$(verify_pe "$setup" "installer")
exe_sig=$(verify_pe "$exe" "application binary")
7z e -so "$setup" uninstall.exe > "$work/uninstall.exe" 2>/dev/null || fail "could not extract uninstall.exe from the installer"
[ -s "$work/uninstall.exe" ] || fail "the extracted uninstall.exe is empty"
uninst_sig=$(verify_pe "$work/uninstall.exe" "embedded uninstaller")
cn=${setup_sig%%|*}
[ "${exe_sig%%|*}" = "$cn" ] || fail "the application binary is signed by '${exe_sig%%|*}', the installer by '$cn'"
[ "${uninst_sig%%|*}" = "$cn" ] || fail "the uninstaller is signed by '${uninst_sig%%|*}', the installer by '$cn'"
# The CN the release expects, when it names one (AGENTRY_WINDOWS_SIGNING_CN,
# exported from build/release.env by the release job): the certificate says
# who signed, this says who was meant to. The pinned root is shared by every
# Artifact Signing customer, so without it the check proves "an Artifact
# Signing subject, one CN across three", not "ours".
expected_cn="${AGENTRY_WINDOWS_SIGNING_CN:-}"
if [ -n "$expected_cn" ] && [ "$expected_cn" != "$cn" ]; then
  fail "signed by CN='$cn' but AGENTRY_WINDOWS_SIGNING_CN expects '$expected_cn'"
fi

if [ -n "$report" ]; then
  {
    printf 'cn=%s\n' "$cn"
    printf 'timestamp=%s\n' "${setup_sig#*|}"
    printf 'root=%s\n' "$root_subject"
  } > "$report"
fi
echo "check-windows-installer: $name ok — asInvoker, installs agentry-desktop.exe + uninstall.exe, same binary as $(basename "$exe"), install section stays in \$INSTDIR; signed by CN=$cn (installer, application and uninstaller), timestamped ${setup_sig#*|}, chain to $root_subject"
