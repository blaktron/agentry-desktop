#!/usr/bin/env bash
# Reads the built Windows installer back and checks it is the one the plan
# describes (agentry-desktop#83; agentry-notes/plans/desktop-installers.md
# §3, §5, §8). This is as far as a Linux runner can check a Windows package
# without Windows: the install-over-install and uninstall checks run by hand
# on a Windows machine until #55 gives CI one.
#
#   scripts/check-windows-installer.sh <setup.exe> <agentry-desktop.exe> [project.nsi]
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
#      section cannot be unticked.
set -euo pipefail

setup="${1:?usage: $0 <setup.exe> <agentry-desktop.exe> [project.nsi]}"
exe="${2:?usage: $0 <setup.exe> <agentry-desktop.exe> [project.nsi]}"
nsi="${3:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/build/windows/installer/project.nsi}"

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
entries=$(7z l -slt "$setup" | awk '/^----------$/{on=1; next} on && /^Path = /{sub(/^Path = /, ""); print}' | tr '\\' '/')
listing=$(printf '%s\n' "$entries" | grep -v '^\$PLUGINSDIR' || true)
expected=$'agentry-desktop.exe\nuninstall.exe'
if [ "$(printf '%s\n' "$listing" | sort)" != "$(printf '%s\n' "$expected" | sort)" ]; then
  printf 'installed files:\n%s\n' "$listing" >&2
  fail "the installer does not install exactly agentry-desktop.exe and uninstall.exe"
fi
printf '%s\n' "$entries" | grep -q '^\$PLUGINSDIR/webview2bootstrapper/MicrosoftEdgeWebview2Setup.exe$' || fail "the WebView2 bootstrapper is not in the installer"

# 4. The same binary as the zip.
if ! 7z e -so "$setup" agentry-desktop.exe 2>/dev/null | cmp -s - "$exe"; then
  fail "the agentry-desktop.exe inside the installer differs from $exe"
fi

# 5. The script itself: what the install section may touch.
grep -q -E '^!define REQUEST_EXECUTION_LEVEL "user"' "$nsi" || fail "$nsi does not define REQUEST_EXECUTION_LEVEL user"
grep -q -E '^!define WAILS_INSTALL_SCOPE "user"' "$nsi" || fail "$nsi does not define WAILS_INSTALL_SCOPE user"
install_section=$(awk '/^Section[[:space:]]*$/{on=1} on{print} /^SectionEnd/{if(on){exit}}' "$nsi")
[ -n "$install_section" ] || fail "no install section found in $nsi"
printf '%s\n' "$install_section" | grep -q 'wails.files' || fail "the install section in $nsi does not copy the files (the awk found the wrong block)"
if printf '%s\n' "$install_section" | grep -v -E '^[[:space:]]*[#;]' | grep -q -E '\$PROFILE|\$APPDATA|\$LOCALAPPDATA|\.config|\.agentry|Credential|RMDir|Delete '; then
  printf '%s\n' "$install_section" >&2
  fail "the install section reaches beyond \$INSTDIR, the Start Menu and the uninstall key"
fi
grep -q -E '^Section /o "un\.Also remove my Agentry settings and sign-in"' "$nsi" || fail "the settings section is not off by default (/o)"
awk '/^Section "un\./{on=1} on{print} /^SectionEnd/{if(on){exit}}' "$nsi" | grep -q 'SectionIn RO' || fail "the application section can be unticked"
grep -q -- '--remove-my-settings' "$nsi" || fail "the settings section does not hand the removal to the app"

echo "check-windows-installer: $name ok — asInvoker, installs agentry-desktop.exe + uninstall.exe, same binary as $(basename "$exe"), install section stays in \$INSTDIR"
