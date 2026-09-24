#!/usr/bin/env bash
# Agentry operator wizard.
#
# Everything above the "STAGES" marker is the library and is identical in every
# wizard: do not hand-edit it. Author your procedure below the marker, one
# `stage` per step, and set TOTAL_STAGES to the number you wrote.
#
#   bash -n <this file>     # syntax check
#   run shellcheck on this file (if available)
#   ./<this file>           # run it (a human runs this, not an agent)

set -uo pipefail

# ---------------------------------------------------------------- library ---

TOTAL_STAGES=${TOTAL_STAGES:-0}
STAGE_NO=0
ENV_FILE=${ENV_FILE:-.env}
SUMMARY=()

c_reset=$'\033[0m'; c_dim=$'\033[2m'; c_bold=$'\033[1m'
c_green=$'\033[32m'; c_yellow=$'\033[33m'; c_red=$'\033[31m'; c_blue=$'\033[36m'

say()  { printf '%s\n' "$*"; }
step() { printf '  %s->%s %s\n' "$c_blue" "$c_reset" "$*"; }
note() { printf '  %s%s%s\n' "$c_dim" "$*" "$c_reset"; }
warn() { printf '  %s! %s%s\n' "$c_yellow" "$*" "$c_reset"; }
die()  { printf '\n%serror:%s %s\n' "$c_red" "$c_reset" "$*" >&2; exit 1; }

# stage "<title>": clear the screen and start a numbered stage.
stage() {
  STAGE_NO=$((STAGE_NO + 1))
  clear 2>/dev/null || true
  printf '%s[%d/%d] %s%s\n\n' "$c_bold" "$STAGE_NO" "$TOTAL_STAGES" "$1" "$c_reset"
}

# open_url "<url>": open it in the human's browser, on Linux, macOS or WSL.
open_url() {
  local url=$1
  step "opening $url"
  if command -v xdg-open >/dev/null 2>&1;      then xdg-open "$url" >/dev/null 2>&1 &
  elif command -v open >/dev/null 2>&1;        then open "$url" >/dev/null 2>&1 &
  elif command -v wslview >/dev/null 2>&1;     then wslview "$url" >/dev/null 2>&1 &
  elif [ -x /mnt/c/Windows/System32/cmd.exe ]; then /mnt/c/Windows/System32/cmd.exe /c start "" "$url" >/dev/null 2>&1 &
  else note "open it yourself: $url"; fi
  sleep 1
}

# ask "<prompt>" VARNAME [default]: read a visible value.
ask() {
  local prompt=$1 var=$2 def=${3:-} val=""
  if [ -n "$def" ]; then read -r -p "  $prompt [$def]: " val; val=${val:-$def}
  else while [ -z "$val" ]; do read -r -p "  $prompt: " val; done; fi
  printf -v "$var" '%s' "$val"
}

# ask_secret "<prompt>" VARNAME: read a value without echoing it.
ask_secret() {
  local prompt=$1 var=$2 val=""
  while [ -z "$val" ]; do read -r -s -p "  $prompt: " val; printf '\n'; done
  printf -v "$var" '%s' "$val"
}

# confirm "<question>": yes/no gate. Anything but y/yes aborts the wizard.
confirm() {
  local ans=""
  read -r -p "  $1 [y/N]: " ans
  case "$ans" in y|Y|yes|YES) return 0 ;; *) die "stopped at stage $STAGE_NO by the operator" ;; esac
}

# pause: wait for the human to finish something in the browser.
pause() { read -r -p "  press enter when done "; }

# write_env KEY "<value>": idempotent upsert into $ENV_FILE (0600).
write_env() {
  local key=$1 value=$2
  [ -f "$ENV_FILE" ] || { : > "$ENV_FILE"; chmod 600 "$ENV_FILE"; }
  if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
    local tmp; tmp=$(mktemp)
    # grep exits 1 when it removes the only line, so never chain mv on its status
    grep -v "^${key}=" "$ENV_FILE" > "$tmp" || true
    mv "$tmp" "$ENV_FILE"
    chmod 600 "$ENV_FILE"
  fi
  printf '%s=%s\n' "$key" "$value" >> "$ENV_FILE"
  step "wrote $key to $ENV_FILE"
  SUMMARY+=("$key -> $ENV_FILE")
}

# set_secret NAME "<value>" [owner/repo]: a GitHub Actions secret. The value
# reaches gh on stdin, which gh reads when --body is NOT given (`--body -`
# stores a literal "-"), and the secret's updated_at is read back.
set_secret() {
  local name=$1 value=$2 repo=${3:-} since
  command -v gh >/dev/null 2>&1 || { warn "gh not installed; skipped secret $name"; return 0; }
  case "$value" in ""|-) warn "refusing to store an empty or '-' value in $name"; return 1 ;; esac
  since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  if [ -n "$repo" ]; then printf '%s' "$value" | gh secret set "$name" -R "$repo" >/dev/null
  else printf '%s' "$value" | gh secret set "$name" >/dev/null; fi \
    || { warn "could not set secret $name"; return 1; }
  local at; at=$(gh api "repos/${repo:-$(gh repo view --json nameWithOwner -q .nameWithOwner)}/actions/secrets/$name" -q .updated_at 2>/dev/null)
  if [ -n "$at" ] && [ ! "$at" \< "$since" ]; then
    step "set GitHub secret $name (updated $at)"; SUMMARY+=("$name -> GitHub secret ${repo:-current repo}")
  else
    warn "secret $name was not updated (updated_at ${at:-unknown}, want ${since} or later)"; return 1
  fi
}

# set_var NAME "<value>" [owner/repo]: a GitHub Actions variable (not secret).
set_var() {
  local name=$1 value=$2 repo=${3:-}
  command -v gh >/dev/null 2>&1 || { warn "gh not installed; skipped variable $name"; return 0; }
  if [ -n "$repo" ]; then gh variable set "$name" -R "$repo" --body "$value" >/dev/null
  else gh variable set "$name" --body "$value" >/dev/null; fi \
    && { step "set GitHub variable $name"; SUMMARY+=("$name -> GitHub variable ${repo:-current repo}"); } \
    || warn "could not set variable $name"
}

finish() {
  clear 2>/dev/null || true
  printf '%s%sdone%s\n\n' "$c_bold" "$c_green" "$c_reset"
  if [ ${#SUMMARY[@]} -gt 0 ]; then
    say "What this wizard wrote:"
    printf '  - %s\n' "${SUMMARY[@]}"
    printf '\n'
  fi
  say "Nothing above printed a secret value. Keep $ENV_FILE out of git."
}

# ------------------------------------------------------------------ STAGES ---
# Author below this line. One `stage` per step, in dependency order.
# Set TOTAL_STAGES to the number of stages you wrote.


TOTAL_STAGES=7

# ---------------------------------------------------------------------------
# Agentry Desktop: load an update-signing key.
#
# This is the key the application checks a release against before it replaces
# itself — the estate's own key, not the operating system's (agentry-desktop#86,
# register D-026, plan desktop-installers.md §7, D5, D15). Run it twice in the
# life of the product:
#
#   dev   once, now, so pre-launch releases are signed at all;
#   prod  at launch, on hardware you hold, retiring dev in the same run.
#
# What it does: generates the pair HERE, on this machine, so no agent and no
# server ever holds the private half; loads the private half into the
# agentry-desktop Actions secret; writes the public half into
# internal/selfupdate/keys.json; and leaves you to commit that file. The two
# halves only ever meet in this script.
#
# Run it from a clone of agentry-desktop, on a machine with `gh` signed in.
# ---------------------------------------------------------------------------

KEYS_JSON="internal/selfupdate/keys.json"
REPO="blaktron/agentry-desktop"
SECRET_NAME="AGENTRY_UPDATE_SIGNING_KEY"
WORK=""

cleanup_keys() { [ -n "$WORK" ] && rm -rf "$WORK"; return 0; }
# INT and TERM must EXIT, not just clean up: a bare handler would delete the
# pair and let the script carry on to `gh secret set` with nothing to read.
trap cleanup_keys EXIT
trap 'cleanup_keys; printf "\n"; die "interrupted; nothing was changed"' INT TERM

stage "Where you are, and what you are about to make"
[ -f "$KEYS_JSON" ] || die "run this from the root of an agentry-desktop clone ($KEYS_JSON is not here)"
command -v go >/dev/null 2>&1 || die "go is needed to generate the key and write the key set"
command -v gh >/dev/null 2>&1 || die "gh is needed to load the Actions secret; install it and sign in first"
gh auth status >/dev/null 2>&1 || die "gh is not signed in: run 'gh auth login'"
say "This machine will generate an ed25519 (minisign) key pair."
say ""
step "the PRIVATE half goes into the $SECRET_NAME secret of $REPO and nowhere else"
step "the PUBLIC half goes into $KEYS_JSON, which you commit"
step "nothing is written to any server, and the private half is never printed"
say ""
note "Existing keys in $KEYS_JSON:"
go run ./internal/cmd/keyset list || die "could not read $KEYS_JSON"
say ""
confirm "continue?"

stage "Name this key"
say "A key's name is how the record and every error message refers to it, and"
say "it never changes. Use 'dev' for the pre-launch key and 'prod' for the one"
say "you generate on your own hardware at launch."
say ""
ask "name for this key" KEY_NAME "prod"
case "$KEY_NAME" in
  *[!a-z0-9-]*) die "a key name is lower-case letters, digits and hyphens: '$KEY_NAME' is not" ;;
esac

stage "Why the copy in CI carries no password"
say "The private half is about to become a GitHub Actions secret, which the"
say "release workflow reads with nobody present. A password on it would have to"
say "be a SECOND Actions secret in the same store, and anything able to read one"
say "can read the other, so it would add ceremony rather than protection. What"
say "protects that copy instead: it is piped to the signer on standard input,"
say "never written to the runner's disk, and dropped from the signer's own"
say "environment."
say ""
say "The next stage offers you a SEPARATE backup copy, and that one can carry a"
say "password. Nothing here is a choice you have to make now."
say ""
confirm "understood — generate the key?"

stage "Generate the pair"
WORK=$(mktemp -d "${TMPDIR:-/tmp}/agentry-signing.XXXXXX")
chmod 700 "$WORK"
step "generating in $WORK (removed when this wizard exits)"
PUBLIC_LINE=$(go run ./internal/cmd/keyset generate -name "$KEY_NAME" -out "$WORK") \
  || die "key generation failed"
KEY_ID=$(head -1 "$WORK/$KEY_NAME.pub" | sed 's/.*: //')
say ""
step "public key: $PUBLIC_LINE"
step "key id:     $KEY_ID"
note "the private half is in $WORK and has not been printed"

stage "Keep a backup of the private half"
say "The wizard is about to hand this key to GitHub, where it can be REPLACED"
say "but never read back, and then delete its own copy. If you keep nothing"
say "here, the private half exists nowhere you can reach."
say ""
say "For 'dev' that is an accepted loss: losing it costs a rotation and a"
say "re-release, nothing more. For 'prod' it is not — losing it means every"
say "installed copy of the app stops accepting new releases until its owner"
say "downloads a fresh one by hand."
say ""
BACKUP_PATH=""
read -r -p "  path for a password-protected backup (empty to keep none): " BACKUP_PATH
if [ -n "$BACKUP_PATH" ]; then
  [ -e "$BACKUP_PATH" ] && die "$BACKUP_PATH already exists; move it aside and re-run"
  ask_secret "password for the backup" BACKUP_PASSWORD
  BACKUP_PASSWORD2=""
  ask_secret "password again" BACKUP_PASSWORD2
  [ "$BACKUP_PASSWORD" = "$BACKUP_PASSWORD2" ] || die "the two passwords differ"
  ( umask 077; go run ./internal/cmd/keyset protect \
      -key "$WORK/$KEY_NAME.key" -out "$BACKUP_PATH" -password-stdin <<<"$BACKUP_PASSWORD" ) \
    || die "could not write the backup"
  step "wrote $BACKUP_PATH (0600, password-protected)"
  note "Keep it somewhere that survives the loss of this machine."
  note "To use it later: internal/cmd/signrelease -key $BACKUP_PATH -password-file <file>"
  SUMMARY+=("password-protected backup -> $BACKUP_PATH")
  BACKUP_PASSWORD=""; BACKUP_PASSWORD2=""
else
  if [ "$KEY_NAME" != "dev" ]; then
    warn "No backup of a key named '$KEY_NAME'. If you lose the Actions secret,"
    warn "every installed copy needs a manual download to get moving again."
    confirm "really keep no backup?"
  else
    note "no backup kept; for 'dev' that loss is accepted (D15)"
  fi
fi

stage "Load the private half into GitHub"
say "This replaces the current $SECRET_NAME secret on $REPO."
say ""
if [ "$KEY_NAME" != "dev" ]; then
  warn "Releases signed by the PREVIOUS key stay verifiable, because the next"
  warn "stage keeps its public half in the key set and marks it retired."
fi
confirm "set the $SECRET_NAME secret on $REPO now?"
gh secret set "$SECRET_NAME" -R "$REPO" < "$WORK/$KEY_NAME.key" \
  || die "could not set the secret; nothing has been committed, so re-run this wizard"
step "set $SECRET_NAME on $REPO"
SUMMARY+=("$SECRET_NAME -> GitHub secret on $REPO (key '$KEY_NAME', id $KEY_ID)")

stage "Write the public half into the key set"
say "Every key the product has ever used stays in $KEYS_JSON. Old ones are"
say "marked retired: they still prove an OLD release genuine, and the app"
say "refuses to install anything they signed. That is what makes a rotation a"
say "release rather than a reinstall."
say ""
RETIRE_ARGS=""
if go run ./internal/cmd/keyset list 2>/dev/null | grep 'current' >/dev/null; then  # not -q: pipefail (agentry-desktop#236)
  say "There is already a current key. Retiring it is almost always right:"
  say "two current keys means two keys that can authorise an install."
  read -r -p "  retire the current key(s)? [Y/n]: " ans
  case "$ans" in n|N|no|NO) warn "leaving the previous key current: two keys can now authorise an install" ;; *) RETIRE_ARGS="-retire-current" ;; esac
fi
go run ./internal/cmd/keyset add \
  -name "$KEY_NAME" \
  -public "$PUBLIC_LINE" \
  -note "private half: the $SECRET_NAME secret of $REPO" \
  $RETIRE_ARGS || die "could not write $KEYS_JSON"
say ""
go run ./internal/cmd/keyset list
SUMMARY+=("public half of '$KEY_NAME' -> $KEYS_JSON (commit it)")
say ""
warn "NOT DONE until you commit and push $KEYS_JSON:"
note "  git add $KEYS_JSON && git commit -m 'chore(release): trust the $KEY_NAME update-signing key' && git push"
note "Until that lands, the release workflow refuses every tag: it verifies its"
note "own signature against the committed key set and will not publish a release"
note "no shipped app could check."

finish
