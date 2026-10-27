#!/usr/bin/env bash
# Reads the built Windows installer back and checks it installs per user
# and carries only what it should. This is as far as a Linux runner can
# check a Windows package without Windows: the install-over-install and
# uninstall checks (scripts/acceptance/windows-install.ps1) run by hand on a
# Windows machine.
#
#   scripts/check-windows-installer.sh [--cli FILE] [--whisper DIR] [--diarize DIR] <setup.exe> <agentry-desktop.exe> [project.nsi]
#
# What it proves:
#   1. the file name is one the download manifest admits (-setup.exe is
#      "setup");
#   2. the installer file carries an asInvoker execution-level request —
#      NSIS's spelling of RequestExecutionLevel user — and no elevated one
#      (a grep over the file: it cannot tell the stub's manifest from a
#      payload's, which is why the elevated forms are refused too);
#   3. it installs exactly three files, the application, the third-party
#      notices (THIRD_PARTY_NOTICES.txt, byte-identical to the repository's
#      THIRD_PARTY_NOTICES) and its uninstaller (everything else is NSIS
#      plumbing under $PLUGINSDIR, the WebView2 bootstrapper included) —
#      four with --cli FILE: bin/agentry.exe too, byte-identical to FILE, the
#      CLI a build put in build/cli; without --cli an installer carrying a CLI
#      is refused, since nothing then says which CLI it is; and with
#      --whisper DIR every file of DIR but its SHA256SUMS under bin/whisper/,
#      each byte-identical to DIR's, the transcriber a build put in
#      build/whisper; without --whisper an installer carrying bin/whisper is
#      refused, for the same reason; and with --diarize DIR the speaker
#      separation under bin/diarize/ the same way, the one a build put in
#      build/diarize, an installer carrying bin/diarize refused without it;
#   4. the application it carries is byte-identical to the binary the zip
#      carries: the installer is not a different build;
#   5. the script's install section reaches nothing under the person's
#      profile, the settings section is off by default, and the application
#      section cannot be unticked;
#   6. the script shows the licence page before the install pages, with the
#      box to accept, from build/windows/installer/LICENSE.txt, and that
#      file holds the MIT licence, the Microsoft Edge WebView2 Runtime terms
#      and the pointer to THIRD_PARTY_NOTICES.txt. The page text is
#      compressed inside the installer where 7z does not list it, so it is
#      checked in the script the installer is built from.
#
# It says nothing about signatures: a release that signs the installer reads
# its signatures back itself.
set -euo pipefail

cli=""
whisper=""
diarize=""
while [ $# -gt 0 ]; do
  case "$1" in
    --cli) cli="${2:?--cli needs the CLI file}"; shift 2 ;;
    --whisper) whisper="${2:?--whisper needs the sidecar folder}"; shift 2 ;;
    --diarize) diarize="${2:?--diarize needs the sidecar folder}"; shift 2 ;;
    --) shift; break ;;
    -*) echo "check-windows-installer: unknown option $1" >&2; exit 2 ;;
    *) break ;;
  esac
done
setup="${1:?usage: $0 [--cli FILE] [--whisper DIR] [--diarize DIR] <setup.exe> <agentry-desktop.exe> [project.nsi]}"
exe="${2:?usage: $0 [--cli FILE] [--whisper DIR] [--diarize DIR] <setup.exe> <agentry-desktop.exe> [project.nsi]}"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
nsi="${3:-$root_dir/build/windows/installer/project.nsi}"

fail() { echo "check-windows-installer: $*" >&2; exit 1; }

[ -f "$setup" ] || fail "$setup does not exist"
[ -f "$exe" ] || fail "$exe does not exist"
[ -f "$nsi" ] || fail "$nsi does not exist"
[ -z "$cli" ] || [ -f "$cli" ] || fail "$cli does not exist"
[ -z "$whisper" ] || [ -f "$whisper/whisper-cli.exe" ] || fail "$whisper holds no whisper-cli.exe"
# The sidecar's files, as the installer must place them.
whisper_files=""
[ -z "$whisper" ] || whisper_files=$(cd "$whisper" && find . -maxdepth 1 -type f ! -name SHA256SUMS | sed 's|^\./||' | sort)
[ -z "$diarize" ] || [ -f "$diarize/sherpa-onnx-offline-speaker-diarization.exe" ] || fail "$diarize holds no sherpa-onnx-offline-speaker-diarization.exe"
diarize_files=""
[ -z "$diarize" ] || diarize_files=$(cd "$diarize" && find . -maxdepth 1 -type f ! -name SHA256SUMS | sed 's|^\./||' | sort)
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
expected=$'THIRD_PARTY_NOTICES.txt\nagentry-desktop.exe\nuninstall.exe'
[ -z "$cli" ] || expected+=$'\nbin/agentry.exe'
[ -z "$whisper_files" ] || expected+=$'\n'"$(printf '%s\n' "$whisper_files" | sed 's|^|bin/whisper/|')"
[ -z "$diarize_files" ] || expected+=$'\n'"$(printf '%s\n' "$diarize_files" | sed 's|^|bin/diarize/|')"
if [ "$(printf '%s\n' "$listing" | sort)" != "$(printf '%s\n' "$expected" | sort)" ]; then
  printf 'installed files:\n%s\n' "$listing" >&2
  want="agentry-desktop.exe, THIRD_PARTY_NOTICES.txt, uninstall.exe"
  [ -z "$cli" ] || want+=", bin/agentry.exe"
  [ -z "$whisper" ] || want+=", the sidecar under bin/whisper"
  [ -z "$diarize" ] || want+=", the speaker separation under bin/diarize"
  fail "the installer does not install exactly $want (a CLI is checked with --cli, the sidecars with --whisper and --diarize)"
fi
grep -q '^\$PLUGINSDIR/webview2bootstrapper/MicrosoftEdgeWebview2Setup.exe$' <<<"$entries" || fail "the WebView2 bootstrapper is not in the installer"

# 4. The same binary as the zip.
if ! 7z e -so "$setup" agentry-desktop.exe 2>/dev/null | cmp -s - "$exe"; then
  fail "the agentry-desktop.exe inside the installer differs from $exe"
fi

# 4a. The notices it installs are the repository's file, and the zip beside
# the installer carries the same text.
if ! 7z e -so "$setup" THIRD_PARTY_NOTICES.txt 2>/dev/null | cmp -s - "$root_dir/THIRD_PARTY_NOTICES"; then
  fail "the THIRD_PARTY_NOTICES.txt inside the installer differs from $root_dir/THIRD_PARTY_NOTICES"
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

# 4c. The transcriber it carries is the one build/whisper held, file for
# file.
for f in $whisper_files; do
  if ! 7z e -so "$setup" "bin/whisper/$f" 2>/dev/null | cmp -s - "$whisper/$f"; then
    fail "the bin/whisper/$f inside the installer differs from $whisper/$f"
  fi
done

# 4d. The speaker separation it carries is the one build/diarize held.
for f in $diarize_files; do
  if ! 7z e -so "$setup" "bin/diarize/$f" 2>/dev/null | cmp -s - "$diarize/$f"; then
    fail "the bin/diarize/$f inside the installer differs from $diarize/$f"
  fi
done

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
grep -q -E '^Section /o "un\.Also remove my Agentry settings, sign-in and agents'"'"' share folders"' "$nsi" || fail "the settings section is not off by default (/o)"
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

# 6. The licence page. NSIS reads LICENSE.txt at compile time and stores it
# in the installer's compressed header, so the built file is checked through
# the script that includes it: the page must come before the directory and
# install pages, must ask for a ticked box, and must name the file below.
licence="$(dirname "$nsi")/LICENSE.txt"
[ -f "$licence" ] || fail "$licence does not exist"
grep -q -E '^!define MUI_LICENSEPAGE_CHECKBOX' "$nsi" || fail "the licence page in $nsi does not ask for a ticked box (MUI_LICENSEPAGE_CHECKBOX)"
licence_at=$(grep -n -E '^!insertmacro MUI_PAGE_LICENSE "LICENSE\.txt"' "$nsi" | head -1 | cut -d: -f1 || true)
directory_at=$(grep -n -E '^!insertmacro MUI_PAGE_DIRECTORY' "$nsi" | head -1 | cut -d: -f1 || true)
instfiles_at=$(grep -n -E '^!insertmacro MUI_PAGE_INSTFILES' "$nsi" | head -1 | cut -d: -f1 || true)
[ -n "$licence_at" ] || fail "$nsi does not show LICENSE.txt on a licence page (MUI_PAGE_LICENSE)"
[ -n "$directory_at" ] && [ -n "$instfiles_at" ] || fail "$nsi has no directory or install page"
[ "$licence_at" -lt "$directory_at" ] && [ "$licence_at" -lt "$instfiles_at" ] || fail "the licence page comes after an install page in $nsi"
licence_text=$(tr -d '\r' < "$licence")
# The phrases are matched across line breaks.
licence_flat=$(tr -d '\r' < "$licence" | tr '\n' ' ' | tr -s ' ')
for needle in \
  'Copyright (c) 2026 Chris Dupres' \
  'Permission is hereby granted, free of charge' \
  'Microsoft Edge WebView2 Runtime' \
  'Microsoft Software License Terms' \
  'https://developer.microsoft.com/microsoft-edge/webview2' \
  'Installing the Runtime means you accept Microsoft' \
  'not made, endorsed or sponsored by Microsoft' \
  'THIRD_PARTY_NOTICES.txt'; do
  grep -q -F -- "$needle" <<<"$licence_flat" || fail "$licence does not carry: $needle"
done
# The page is plain ASCII with CRLF line ends, which every NSIS licence page
# renders; a byte outside ASCII would be read in the installer's ANSI code page.
if LC_ALL=C grep -q -P '[^\x00-\x7F]' "$licence"; then
  fail "$licence has a byte outside ASCII"
fi
[ "$(grep -c -v $'\r$' "$licence" || true)" = 0 ] || fail "$licence has a line without a CRLF ending"
# The root LICENSE is the text of part 1.
root_mit=$(tr -d '\r' < "$root_dir/LICENSE")
grep -q -F -- "$root_mit" <<<"$licence_text" || fail "the MIT licence in $licence differs from $root_dir/LICENSE"

carries="agentry-desktop.exe + THIRD_PARTY_NOTICES.txt + uninstall.exe"
[ -z "$cli" ] || carries="agentry-desktop.exe + THIRD_PARTY_NOTICES.txt + bin/agentry.exe ($(basename "$cli")) + uninstall.exe"
[ -z "$whisper" ] || carries+=" + bin/whisper ($(printf '%s\n' "$whisper_files" | grep -c .) files)"
echo "check-windows-installer: $name ok — asInvoker, installs $carries, same binary as $(basename "$exe"), install section stays in \$INSTDIR, licence page before the install pages"
