#!/usr/bin/env bash
# Reads the built Windows installer back and checks it installs per user
# and carries only what it should. This is as far as a Linux runner can
# check a Windows package without Windows: the install-over-install and
# uninstall checks (scripts/acceptance/windows-install.ps1) run by hand on a
# Windows machine.
#
#   scripts/check-windows-installer.sh [--cli FILE] <setup.exe> <agentry-desktop.exe> [project.nsi]
#
# What it proves:
#   1. the file name is one the download manifest admits (-setup.exe is
#      "setup");
#   2. the installer file carries an asInvoker execution-level request —
#      NSIS's spelling of RequestExecutionLevel user — and no elevated one
#      (a grep over the file: it cannot tell the stub's manifest from a
#      payload's, which is why the elevated forms are refused too);
#   3. it installs exactly two files, the application and its uninstaller
#      (everything else is NSIS plumbing under $PLUGINSDIR, the WebView2
#      bootstrapper included) — three with --cli FILE:
#      bin/agentry.exe too, byte-identical to FILE, the CLI a build put in
#      build/cli; without --cli an installer carrying a CLI is refused, since
#      nothing then says which CLI it is;
#   4. the application it carries is byte-identical to the binary the zip
#      carries: the installer is not a different build;
#   5. the script's install section reaches nothing under the person's
#      profile, the settings section is off by default, and the application
#      section cannot be unticked.
#
# It says nothing about signatures: a release that signs the installer reads
# its signatures back itself.
set -euo pipefail

cli=""
while [ $# -gt 0 ]; do
  case "$1" in
    --cli) cli="${2:?--cli needs the CLI file}"; shift 2 ;;
    --) shift; break ;;
    -*) echo "check-windows-installer: unknown option $1" >&2; exit 2 ;;
    *) break ;;
  esac
done
setup="${1:?usage: $0 [--cli FILE] <setup.exe> <agentry-desktop.exe> [project.nsi]}"
exe="${2:?usage: $0 [--cli FILE] <setup.exe> <agentry-desktop.exe> [project.nsi]}"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
nsi="${3:-$root_dir/build/windows/installer/project.nsi}"

fail() { echo "check-windows-installer: $*" >&2; exit 1; }

[ -f "$setup" ] || fail "$setup does not exist"
[ -f "$exe" ] || fail "$exe does not exist"
[ -f "$nsi" ] || fail "$nsi does not exist"
[ -z "$cli" ] || [ -f "$cli" ] || fail "$cli does not exist"
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
# No `… | grep -q` anywhere in this script: grep -q
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
[ -z "$cli" ] || expected+=$'\nbin/agentry.exe'
if [ "$(printf '%s\n' "$listing" | sort)" != "$(printf '%s\n' "$expected" | sort)" ]; then
  printf 'installed files:\n%s\n' "$listing" >&2
  if [ -n "$cli" ]; then
    fail "the installer does not install exactly agentry-desktop.exe, bin/agentry.exe and uninstall.exe"
  fi
  fail "the installer does not install exactly agentry-desktop.exe and uninstall.exe (one that carries the CLI is checked with --cli)"
fi
grep -q '^\$PLUGINSDIR/webview2bootstrapper/MicrosoftEdgeWebview2Setup.exe$' <<<"$entries" || fail "the WebView2 bootstrapper is not in the installer"

# 4. The same binary as the zip.
if ! 7z e -so "$setup" agentry-desktop.exe 2>/dev/null | cmp -s - "$exe"; then
  fail "the agentry-desktop.exe inside the installer differs from $exe"
fi

# 4b. The CLI it carries is the one build/cli held, byte for byte, and the
# script hands the user Path to the app both ways.
if [ -n "$cli" ]; then
  if ! 7z e -so "$setup" "bin/agentry.exe" 2>/dev/null | cmp -s - "$cli"; then
    fail "the bin/agentry.exe inside the installer differs from $cli"
  fi
  grep -E '^[[:space:]]*ExecWait .*--add-cli-to-path' "$nsi" >/dev/null || fail "the install section does not have the app put the CLI on the Path (--add-cli-to-path)"
  grep -E '^[[:space:]]*ExecWait .*--remove-cli-from-path' "$nsi" >/dev/null || fail "the uninstaller does not have the app take the CLI off the Path (--remove-cli-from-path)"
fi

# 5. The script itself: what the install section may touch.
grep -q -E '^!define REQUEST_EXECUTION_LEVEL "user"' "$nsi" || fail "$nsi does not define REQUEST_EXECUTION_LEVEL user"
grep -q -E '^!define WAILS_INSTALL_SCOPE "user"' "$nsi" || fail "$nsi does not define WAILS_INSTALL_SCOPE user"
install_section=$(awk '/^Section[[:space:]]*$/{on=1} on{print} /^SectionEnd/{if(on){exit}}' "$nsi")
[ -n "$install_section" ] || fail "no install section found in $nsi"
grep -q 'wails.files' <<<"$install_section" || fail "the install section in $nsi does not copy the files (the awk found the wrong block)"
# The one Delete the section may hold is the CLI's own old copy, moved aside
# so a running agentry.exe does not stop an upgrade.
if grep -v -E '^[[:space:]]*[#;]' <<<"$install_section" | grep -v -F 'Delete "$INSTDIR\bin\agentry.exe.old"' | grep -E '\$PROFILE|\$APPDATA|\$LOCALAPPDATA|\.config|\.agentry|Credential|RMDir|Delete ' >/dev/null; then
  printf '%s\n' "$install_section" >&2
  fail "the install section reaches beyond \$INSTDIR, the Start Menu and the uninstall key"
fi
grep -q -E '^Section /o "un\.Also remove my Agentry settings and sign-in"' "$nsi" || fail "the settings section is not off by default (/o)"
awk '/^Section "un\./{on=1} on{print} /^SectionEnd/{if(on){exit}}' "$nsi" | grep 'SectionIn RO' >/dev/null || fail "the application section can be unticked"
grep -q -- '--remove-my-settings' "$nsi" || fail "the settings section does not hand the removal to the app"
# Agentry's Docker images go unless "Keep my Docker images" is ticked: the
# box is off by default, the app does the removal, and it runs before the
# settings section deletes the policy.
grep -q -E '^Section /o "un\.Keep my Docker images" SEC_UN_KEEP_IMAGES' "$nsi" || fail "the keep-images box is not off by default (/o)"
grep -q -- '--remove-docker-images' "$nsi" || fail "the images section does not hand the removal to the app"
images_at=$(grep -n -E '^[[:space:]]*ExecWait .*--remove-docker-images' "$nsi" | head -1 | cut -d: -f1)
settings_at=$(grep -n -E '^[[:space:]]*ExecWait .*--remove-my-settings' "$nsi" | head -1 | cut -d: -f1)
[ -n "$images_at" ] && [ -n "$settings_at" ] || fail "the images or the settings step does not run the app (no ExecWait)"
[ "$images_at" -lt "$settings_at" ] || fail "the images step runs after the settings step, which deletes the policy that names them"

carries="agentry-desktop.exe + uninstall.exe"
[ -z "$cli" ] || carries="agentry-desktop.exe + bin/agentry.exe ($(basename "$cli")) + uninstall.exe"
echo "check-windows-installer: $name ok — asInvoker, installs $carries, same binary as $(basename "$exe"), install section stays in \$INSTDIR"
