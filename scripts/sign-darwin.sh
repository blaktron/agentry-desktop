#!/usr/bin/env bash
# Developer ID-signs, notarizes and staples the macOS bundle and its disk
# image on the Mac runner, and asks Gatekeeper for its verdict on each
# (agentry-desktop#6; plan agentry-notes/plans/os-code-signing.md §5, M-M2,
# D4, D5, D6, D8).
#
#   scripts/sign-darwin.sh app    <agentry-desktop.app>   sign, notarize, staple, assess
#   scripts/sign-darwin.sh dmg    <image.dmg>             sign, notarize, staple, assess
#   scripts/sign-darwin.sh assess <app | dmg | zip>       the read-back alone, no secret
#
# The release's macos job runs them in this order: `make build-darwin`, then
# `app` on the built bundle, then `make wrap-darwin` (the zip and the dmg are
# made from the STAPLED bundle, so both carry its ticket), then `dmg` on the
# image, then `check-darwin-dmg.sh --signed` and `assess` on the zip. Nothing
# in the app's build or tests calls this, and a PR build never does (D6): an
# unsigned build stays the ad-hoc one wails applies.
#
# `app` signs inside-out under the hardened runtime with Apple's timestamp:
# every nested Mach-O first (a Wails bundle has none today; the front end is
# embedded in the one binary), then the bundle, whose identifier is its
# CFBundleIdentifier. There are NO entitlements: WKWebView runs out of
# process, the Go binary needs no JIT, and exec of the Docker and
# coding-agent CLIs is allowed under the hardened runtime. The read-back
# refuses a signature that carries any, so one can only arrive as a reviewed
# change here, with its line in the register. A nested bundle (.framework,
# .app, .xpc, …) is refused rather than signed in a way nobody checked.
#
# Notarization submits a zip of the bundle (or the dmg itself), waits for
# Apple's decision, prints Apple's log and fails on anything but Accepted,
# then staples the ticket so Gatekeeper can judge the file offline.
#
# `assess` is the claim the release makes, read back from the file itself:
# signed by APPLE_SIGNING_IDENTITY of APPLE_TEAM_ID with a secure timestamp
# (and, for the bundle, the hardened runtime, no entitlements and a
# designated requirement naming the team), a stapled ticket that validates,
# and Gatekeeper's `spctl` accepting it as "Notarized Developer ID" —
# `--type execute` for the bundle, `--type open --context
# context:primary-signature` for the image. A zip is unpacked and its bundle
# assessed.
#
# Inputs come from the environment (the release job's secrets reach only the
# two steps that sign). Two reach a command line on their way: the .p12
# password, on `security import`'s for the moment it runs, and the notary
# Key ID and Issuer ID (identifiers, not the key), on notarytool's while it
# waits; the runner's user runs nothing else of anyone's.
#
#   APPLE_DEVELOPER_ID_P12           the .p12, base64 (certificate, key, Apple's G2 intermediate)   app, dmg
#   APPLE_DEVELOPER_ID_P12_PASSWORD  its password                                                    app, dmg
#   APPLE_NOTARY_KEY_P8              the App Store Connect API key, PEM text                          app, dmg
#   APPLE_NOTARY_KEY_ID              its Key ID                                                       app, dmg
#   APPLE_NOTARY_ISSUER_ID           its Issuer ID                                                    app, dmg
#   APPLE_SIGNING_IDENTITY           the identity the signature must carry (build/release.env)       all
#   APPLE_TEAM_ID                    its Team ID (build/release.env)                                 all
#   SIGN_DARWIN_REPORT               optional: a file to append team=, notarized= and the submission to
#   NOTARY_TIMEOUT_MINUTES           optional: how long to wait for Apple, default 120
#
# Custody (D5): the certificate lives in a keychain this script creates in its
# own 0700 directory and deletes on exit, with the .p12 (removed as soon as
# it is imported) and the .p8 (kept for the notary wait); a keychain left on
# the search list fails the run. A process killed before its EXIT trap runs
# (a cancelled job) leaves both behind: the release job's last step removes
# them and fails (release.yaml, "No signing keychain or key is left"). The keychain is on the build user's search list only while codesign
# runs (codesign finds no identity otherwise). Headless codesign also needs
# Apple's Developer ID G2 intermediate in the host's System keychain
# (installed on maxbookpro on 2026-10-01, agentry-desktop#152): without it
# codesign fails with errSecInternalComponent. The keychain handling is the
# CLI's scripts/sign-darwin.sh (agentry-cli#198), copied, as D-032 copies the
# Windows signing scripts rather than sharing them. macOS's bash is 3.2:
# nothing here needs bash 4.
set -euo pipefail

OPENSSL=/usr/bin/openssl

fail() { echo "sign-darwin: $*" >&2; exit 1; }

usage="usage: $0 app <agentry-desktop.app> | dmg <image.dmg> | assess <app|dmg|zip>"
[ "$#" -eq 2 ] || fail "$usage"
cmd=$1
target=${2%/}
case "$cmd" in app|dmg|assess) ;; *) fail "$usage" ;; esac
[ -e "$target" ] || fail "$target does not exist"
[ "$(uname -s)" = Darwin ] || fail "this runs on macOS (codesign, notarytool, stapler, spctl)"

need="APPLE_SIGNING_IDENTITY APPLE_TEAM_ID"
[ "$cmd" = assess ] || need="APPLE_DEVELOPER_ID_P12 APPLE_DEVELOPER_ID_P12_PASSWORD APPLE_NOTARY_KEY_P8 APPLE_NOTARY_KEY_ID APPLE_NOTARY_ISSUER_ID $need"
for v in $need; do
  eval "val=\${$v:-}"
  [ -n "$val" ] || fail "$v is not set"
done
val=""
for t in security codesign spctl ditto plutil base64 file xcrun awk; do command -v "$t" >/dev/null 2>&1 || fail "$t is missing"; done
xcrun --find stapler >/dev/null 2>&1 || fail "xcrun cannot find stapler (the Command Line Tools)"
[ "$cmd" = assess ] || [ -x "$OPENSSL" ] || fail "$OPENSSL is missing"
[ "$cmd" = assess ] || xcrun --find notarytool >/dev/null 2>&1 || fail "xcrun cannot find notarytool (the Command Line Tools)"

W=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/sign-darwin.XXXXXX") || fail "no temporary directory"
chmod 700 "$W"
# The search list holds resolved paths (/private/tmp, not /tmp): compare
# against the same, or an entry is never recognised as this one.
W=$(cd "$W" && pwd -P)
KC="$W/signing.keychain-db"

# The user's keychain search list, one path per line.
kc_list() { security list-keychains -d user | sed -e 's/^[[:space:]]*"//' -e 's/"[[:space:]]*$//'; }
# Put KC first on the list, keeping whatever else is there.
kc_add() {
  local l; set -- "$KC"
  # An earlier run's keychain that a kill left on the list is not carried
  # forward.
  while IFS= read -r l; do
    case "$l" in ""|"$KC"|*/sign-darwin.*) ;; *) set -- "$@" "$l" ;; esac
  done <<EOF
$(kc_list)
EOF
  security list-keychains -d user -s "$@"
}
# Deleting the keychain also takes it off the list (`list-keychains -s` with
# no argument changes nothing, so an emptied list cannot be written back).
cleanup() {
  security delete-keychain "$KC" >/dev/null 2>&1 || true
  rm -rf "$W"
  # Captured, then matched: a grep -q on the live pipe could SIGPIPE
  # kc_list, and under pipefail that reads as "not on the list".
  local left
  left=$(kc_list)
  if grep -F "$(basename "$W")" <<<"$left" >/dev/null; then echo "sign-darwin: the signing keychain is still on the search list" >&2; return 1; fi
  return 0
}
# A keychain left on the list fails the run (plan os-code-signing.md M-M2 step 7).
trap 'cleanup || exit 1' EXIT

# --- the read-back ----------------------------------------------------------

# The signature's claims, from codesign's own description of the file.
assess_signature() {
  local f=$1 d
  d=$(codesign -dvv "$f" 2>&1)
  grep -qxF "Authority=$APPLE_SIGNING_IDENTITY" <<<"$d" || { echo "$d" >&2; fail "$f is not signed by $APPLE_SIGNING_IDENTITY"; }
  grep -qxF "TeamIdentifier=$APPLE_TEAM_ID" <<<"$d" || { echo "$d" >&2; fail "$f does not carry Team ID $APPLE_TEAM_ID"; }
  grep -q '^Timestamp=' <<<"$d" || { echo "$d" >&2; fail "$f has no secure timestamp"; }
  echo "signed $(basename "$f"): $(grep -E '^(Identifier|Timestamp|TeamIdentifier)=' <<<"$d" | tr '\n' ' ')"
}

# Gatekeeper's verdict. Stapled, so it needs no network; retried briefly
# anyway, because a ticket Apple has just issued can take a moment to be
# served to the online lookup spctl also makes.
assess_gatekeeper() {
  local f=$1 out i
  shift
  for i in 1 2 3 4 5 6; do
    out=$(spctl --assess "$@" --verbose=2 "$f" 2>&1 || true)
    # Two judges, as the CLI's read-back has: spctl's verdict, and codesign
    # checking the ticket against the "notarized" requirement.
    if grep -q ': accepted$' <<<"$out" && grep -qxF 'source=Notarized Developer ID' <<<"$out" \
      && codesign --verify --strict --check-notarization -R="notarized" "$f" 2>/dev/null; then
      echo "$out"; return 0
    fi
    sleep 20
  done
  echo "$out" >&2
  codesign --verify --strict --check-notarization -R="notarized" -vvv "$f" >&2 || true
  fail "Gatekeeper does not accept $(basename "$f") as Notarized Developer ID"
}

assess_app() {
  local app=$1 d id
  [ -d "$app/Contents" ] || fail "$app is not an application bundle"
  codesign --verify --deep --strict --verbose=2 "$app" || fail "codesign --verify --deep --strict refused $app"
  assess_signature "$app"
  d=$(codesign -dvv "$app" 2>&1)
  id=$(plutil -extract CFBundleIdentifier raw -o - "$app/Contents/Info.plist") || fail "$app has no CFBundleIdentifier"
  grep -qxF "Identifier=$id" <<<"$d" || { echo "$d" >&2; fail "$app is not signed as $id"; }
  # flags= lists every flag ("0x12000(library-validation,runtime)"), so
  # runtime is matched as one item of the list, not as the whole of it.
  grep -qE '^CodeDirectory .*flags=0x[0-9a-f]+\(([^)]*,)?runtime[,)]' <<<"$d" || { echo "$d" >&2; fail "$app is not signed under the hardened runtime"; }
  # No entitlements: an ad-hoc entitlements blob, or one a later change
  # slipped in, is a refusal (see the header).
  # Captured before it is matched (a grep -q on the live pipe could SIGPIPE
  # codesign, and pipefail would read a match as none), and a codesign that
  # cannot answer is a refusal, not a pass.
  local ent dr
  ent=$(codesign -d --entitlements - --xml "$app" 2>/dev/null) || fail "codesign could not read the entitlements of $app"
  if grep '<key>' <<<"$ent" >/dev/null; then
    codesign -d --entitlements - "$app" >&2 2>&1 || true
    fail "$app carries entitlements; this release signs with none (add one here, deliberately, with its register line)"
  fi
  # The designated requirement is what macOS keys privacy grants and the
  # keychain's trust on: it must name our team, not a cdhash.
  # codesign quotes the value ("= \"3MU9H2V9Y9\"", seen on macOS 26.6).
  dr=$(codesign -d -r- "$app" 2>&1) || { echo "$dr" >&2; fail "codesign could not read the designated requirement of $app"; }
  grep -E "certificate leaf\[subject\.OU\] = \"?$APPLE_TEAM_ID\"?( |$)" <<<"$dr" >/dev/null \
    || { echo "$dr" >&2; fail "the designated requirement of $app does not name team $APPLE_TEAM_ID"; }
  xcrun stapler validate "$app" >/dev/null || fail "$app carries no valid stapled notarization ticket"
  echo "stapled: $(basename "$app")"
  assess_gatekeeper "$app" --type execute
}

assess_dmg() {
  local dmg=$1
  codesign --verify --strict --verbose=2 "$dmg" || fail "codesign --verify --strict refused $dmg"
  assess_signature "$dmg"
  xcrun stapler validate "$dmg" >/dev/null || fail "$dmg carries no valid stapled notarization ticket"
  echo "stapled: $(basename "$dmg")"
  assess_gatekeeper "$dmg" --type open --context context:primary-signature
}

assess_zip() {
  local zip=$1 apps n
  mkdir "$W/unzipped"
  ditto -x -k "$zip" "$W/unzipped" || fail "$zip did not unpack"
  apps=$(find "$W/unzipped" -maxdepth 1 -name '*.app' -type d)
  n=$(printf '%s' "$apps" | grep -c . || true)
  [ "$n" = 1 ] || fail "$zip holds $n application bundles at its top, not one"
  assess_app "$apps"
}

if [ "$cmd" = assess ]; then
  case "$target" in
    *.app) assess_app "$target" ;;
    *.dmg) assess_dmg "$target" ;;
    *.zip) assess_zip "$target" ;;
    *) fail "assess takes a .app, a .dmg or a .zip, not $target" ;;
  esac
  echo "sign-darwin: $(basename "$target") is Developer ID-signed by team $APPLE_TEAM_ID, stapled, and accepted by Gatekeeper as Notarized Developer ID"
  exit 0
fi

case "$cmd" in
  app) [ -d "$target/Contents/MacOS" ] || fail "$target is not an application bundle" ;;
  dmg) case "$target" in *.dmg) [ -f "$target" ] || fail "$target is not a file" ;; *) fail "$target is not a .dmg" ;; esac ;;
esac

# --- the identity, into a keychain of this run's own -------------------------

# The secrets, into the 0700 directory, never echoed.
( umask 077
  printf '%s' "$APPLE_DEVELOPER_ID_P12" | base64 --decode > "$W/id.p12"
  printf '%s\n' "$APPLE_NOTARY_KEY_P8" > "$W/notary.p8" )
grep -q 'BEGIN PRIVATE KEY' "$W/notary.p8" || fail "APPLE_NOTARY_KEY_P8 is not a PEM private key"

kcpass=$("$OPENSSL" rand -hex 24)
security create-keychain -p "$kcpass" "$KC" >/dev/null
security set-keychain-settings -lut 3600 "$KC"
security unlock-keychain -p "$kcpass" "$KC"
# -P puts the .p12 password on this one command line for the moment it runs;
# the runner's user runs nothing else of anyone's.
security import "$W/id.p12" -k "$KC" -P "$APPLE_DEVELOPER_ID_P12_PASSWORD" -f pkcs12 \
  -T /usr/bin/codesign >/dev/null || fail "security import refused the .p12 (the password?)"
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$kcpass" "$KC" >/dev/null
rm -f "$W/id.p12"
kcpass=""

kc_add
idents=$(security find-identity -v -p codesigning "$KC")
sha1=$(awk -v id="\"$APPLE_SIGNING_IDENTITY\"" 'index($0, id) && !s {s = $2} END {print s}' <<<"$idents")
[ -n "$sha1" ] || { echo "$idents" >&2; fail "no valid identity \"$APPLE_SIGNING_IDENTITY\" in the .p12"; }
echo "identity $APPLE_SIGNING_IDENTITY ($sha1)"

sign() {
  # Another job on this user may have rewritten the list; put KC back first.
  kc_add
  # Without a console session, codesign takes Apple's Developer ID G2
  # intermediate only from the System keychain; find-identity still calls
  # the identity valid without it (agentry-desktop#152).
  codesign --force --timestamp --keychain "$KC" --sign "$sha1" "$@" \
    || fail "codesign failed on ${@: -1} (errSecInternalComponent above means Apple's Developer ID G2 intermediate is missing from this host's System keychain, agentry-desktop#152)"
}

# --- sign -------------------------------------------------------------------

if [ "$cmd" = app ]; then
  nested=$(find "$target/Contents" -mindepth 1 -type d \( -name '*.app' -o -name '*.framework' -o -name '*.xpc' -o -name '*.appex' -o -name '*.bundle' -o -name '*.plugin' \) -print)
  [ -z "$nested" ] || { echo "$nested" >&2; fail "$target holds nested bundles this script does not know how to sign; teach it, with the acceptance to prove it"; }
  main="$target/Contents/MacOS/$(plutil -extract CFBundleExecutable raw -o - "$target/Contents/Info.plist")" || fail "$target has no CFBundleExecutable"
  [ -f "$main" ] || fail "$main does not exist"
  # Every other Mach-O in the bundle, deepest first.
  find "$target/Contents" -type f ! -path "$main" -print | while IFS= read -r f; do
    kind=$(file -b "$f")
    if grep -q '^Mach-O' <<<"$kind"; then printf '%s\t%s\n' "$(tr -cd / <<<"$f" | wc -c | tr -d ' ')" "$f"; fi
  done | sort -rn | cut -f2- > "$W/nested"
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    sign --options runtime "$f"
    echo "signed nested $(basename "$f")"
  done < "$W/nested"
  sign --options runtime "$target"
else
  sign "$target"
fi
security delete-keychain "$KC"
codesign --verify --strict --verbose=2 "$target" || fail "codesign --verify --strict refused $target right after signing"

# --- notarize ---------------------------------------------------------------

# A network error while polling is retried until the deadline; anything but
# Accepted prints Apple's log and fails.
notary() { xcrun notarytool "$@" --key "$W/notary.p8" --key-id "$APPLE_NOTARY_KEY_ID" --issuer "$APPLE_NOTARY_ISSUER_ID"; }
if [ "$cmd" = app ]; then
  sub="$W/$(basename "$target").zip"
  ditto -c -k --keepParent "$target" "$sub"
else
  sub=$target
fi
notary submit "$sub" --output-format json > "$W/submit.json" 2> "$W/submit.err" || { cat "$W/submit.err" >&2; fail "notarytool could not submit $(basename "$target")"; }
id=$(plutil -extract id raw -o - "$W/submit.json" 2>/dev/null || true)
[ -n "$id" ] || { cat "$W/submit.json" "$W/submit.err" >&2; fail "notarytool returned no submission id for $(basename "$target")"; }
echo "submitted $(basename "$target") for notarization: $id"
[ "$cmd" = dmg ] || rm -f "$sub"
started=$(date +%s)
deadline=$(( started + ${NOTARY_TIMEOUT_MINUTES:-120} * 60 ))
while :; do
  status=""
  if notary info "$id" --output-format json > "$W/info.json" 2> "$W/info.err"; then
    status=$(plutil -extract status raw -o - "$W/info.json" 2>/dev/null || true)
  else
    echo "notarytool info $id failed (retrying): $(head -c 200 "$W/info.err")"
  fi
  case "$status" in
    Accepted) echo "notarization $id: Accepted after $(( ($(date +%s) - started) / 60 )) minutes"; break ;;
    Invalid|Rejected) notary log "$id" >&2 || true; fail "notarization $id: $status" ;;
  esac
  [ "$(date +%s)" -lt "$deadline" ] || fail "notarization $id still ${status:-unknown} after ${NOTARY_TIMEOUT_MINUTES:-120} minutes"
  sleep 30
done
# Apple's log of an accepted submission lists any warning it chose not to
# fail on; it goes into the job output, not into a decision.
notary log "$id" 2>/dev/null | head -c 4000 || true
echo
rm -f "$W/notary.p8"

# --- staple, then the read-back ----------------------------------------------

# The ticket can take a moment to be fetchable after Accepted.
stapled=""
for i in 1 2 3 4 5 6; do
  if xcrun stapler staple "$target"; then stapled=1; break; fi
  sleep 20
done
[ -n "$stapled" ] || fail "stapler could not staple $(basename "$target")"

case "$cmd" in
  app) assess_app "$target" ;;
  dmg) assess_dmg "$target" ;;
esac

if [ -n "${SIGN_DARWIN_REPORT:-}" ]; then
  { echo "team=$APPLE_TEAM_ID"; echo "notarized=true"; echo "submission_$cmd=$id"; } >> "$SIGN_DARWIN_REPORT"
fi
echo "sign-darwin: $(basename "$target") signed, notarized, stapled and accepted by Gatekeeper"
