#!/usr/bin/env bash
# Agentry operator wizard.
#
# Everything above the "STAGES" marker is the library and is identical in every
# wizard: do not hand-edit it. Author your procedure below the marker, one
# `stage` per step, and set TOTAL_STAGES to the number you wrote.
#
#   bash -n <this file>     # syntax check
#   shellcheck <this file>  # if available
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

# set_secret NAME "<value>" [owner/repo]: a GitHub Actions secret.
set_secret() {
  local name=$1 value=$2 repo=${3:-}
  command -v gh >/dev/null 2>&1 || { warn "gh not installed; skipped secret $name"; return 0; }
  if [ -n "$repo" ]; then printf '%s' "$value" | gh secret set "$name" -R "$repo" --body - >/dev/null
  else printf '%s' "$value" | gh secret set "$name" --body - >/dev/null; fi \
    && { step "set GitHub secret $name"; SUMMARY+=("$name -> GitHub secret ${repo:-current repo}"); } \
    || warn "could not set secret $name"
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


TOTAL_STAGES=9

# ---------------------------------------------------------------------------
# Agentry Desktop: connect the release workflow to Azure Artifact Signing.
#
# This is the OS code-signing identity for the Windows artifacts — Microsoft's
# HSM holds the key and the CA/Browser Forum's rules say it may live nowhere
# else (agentry-desktop#151, #54; plan os-code-signing.md D1, D2, D6). It is a
# different thing from the estate's own update-signing key (#86), which the
# app checks before it replaces itself and which this wizard does not touch.
#
# Before running it you have done, in the Azure portal, the two things only a
# person can: an Individual Public Trust identity validation that reads
# "Completed", and one Public Trust certificate profile. Everything after that
# is here, and none of it is a secret:
#
#   - an Entra app registration with ONE federated credential for this
#     repository's `release` environment, so a job in that environment exchanges
#     GitHub's OIDC token for an Azure access token at run time and no client
#     secret exists anywhere;
#   - the Certificate Profile Signer role for that identity on your account;
#   - the GitHub environment `release`, admitting only `main` and `v*` tags, and
#     six repository VARIABLES (identifiers, not secrets) the release workflow reads.
#
# It signs you in to Azure with a device code, finds the account and profile
# you made, and creates what is missing; re-running it changes nothing that
# already exists. Run it from a clone of agentry-desktop on a machine where
# `gh` is signed in as someone who administers the TARGET repository (this
# one, or the one --repo names); the wizard refuses any other.
#
#   scripts/operator/artifact-signing-setup.sh [--repo OWNER/NAME]
#
# --repo connects ANOTHER repository of the estate to the same identity
# (agentry-desktop#173; the CLI's release, agentry-cli#197, plan
# cli-release-signing.md D3): one more federated credential on the same app
# registration, named for that repository and matching its own `release`
# environment; that repository's environment, deployment policy, variables
# and probe. The app registration, the role and the profile are the same and
# are reported as existing. Without --repo it is this repository's run.
#
# Overrides, for the odd case: ENDPOINT=<host> if the account does not state
# its signing endpoint and its region is not in the table below.
# ---------------------------------------------------------------------------

DEFAULT_REPO="blaktron/agentry-desktop"
REPO="$DEFAULT_REPO"
while [ $# -gt 0 ]; do
  case "$1" in
    --repo) [ $# -ge 2 ] && [ -n "$2" ] || die "--repo needs OWNER/NAME"; REPO=$2; shift 2 ;;
    --repo=*) REPO=${1#--repo=}; shift ;;
    -h|--help) sed -n '/^# Agentry Desktop: connect/,/^# Overrides/p' "$0" | sed -e '$d' -e 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument $1 (usage: $0 [--repo OWNER/NAME])" ;;
  esac
done
case "$REPO" in
  */*/*|*/|/*|""|*[!A-Za-z0-9_./-]*) die "--repo is OWNER/NAME (letters, digits, . _ -), got '$REPO'" ;;
  */*) ;;
  *) die "--repo is OWNER/NAME, got '$REPO' (no owner)" ;;
esac
REPO_NAME=${REPO#*/}
GH_ENV="release"
# One registration for every repository of the estate; one federated
# credential per repository, named for it, matching its own environment.
APP_NAME="agentry-desktop-release-signing"
FIC_NAME="${REPO_NAME}-release-environment"
ISSUER="https://token.actions.githubusercontent.com"
AUDIENCE="api://AzureADTokenExchange"
RESOURCE="https://codesigning.azure.net"
PROBE_WORKFLOW="signing-oidc-probe.yaml"
VENV="${XDG_DATA_HOME:-$HOME/.local/share}/agentry/azure-cli"
WORK=""

# The temp directory only ever holds the federated credential's parameters
# (issuer, subject, audience: all public), so an interrupt loses nothing.
cleanup_work() { [ -n "$WORK" ] && rm -rf "$WORK"; return 0; }
trap cleanup_work EXIT
trap 'cleanup_work; printf "\n"; die "interrupted; what was already created stays, and re-running picks it up"' INT TERM

# azj <args…>: az with JSON output and stderr silenced. The library runs
# without -e, so every call whose output feeds a test carries its own || die:
# an az failure must never read as "already exists".
azj() { az "$@" -o json 2>/dev/null; }
# count <json-array>: its length, or 0 for empty input (a failed call).
count() { printf '%s' "$1" | jq -r 'if type=="array" then length else 0 end' 2>/dev/null || echo 0; }
# flat <json>: lift ARM's `properties` up beside the top-level fields, so one
# jq path reads a field whichever shape this az version returns.
flat() { jq 'if type=="array" then map(. + (.properties // {})) else . + (.properties // {}) end'; }

stage "Where you are, and what you are about to connect"
[ -f "wails.json" ] || die "run this from the root of an agentry-desktop clone (wails.json is not here)"
for t in gh jq python3; do command -v "$t" >/dev/null 2>&1 || die "$t is needed; install it first"; done
gh auth status >/dev/null 2>&1 || die "gh is not signed in: run 'gh auth login'"
# Only a repository this sign-in administers may be joined to the signing
# identity: the credential the wizard creates lets a job of that repository's
# `release` environment sign as the operator, and the environment's policy is
# what bounds it — both need admin, and the OIDC settings below are readable
# without it, so this is the gate, before anything is created anywhere.
ADMIN=$(gh api "repos/$REPO" --jq '.permissions.admin // false' 2>/dev/null) || die "could not read $REPO (does it exist, and is gh signed in as someone who can see it?)"
[ "$ADMIN" = "true" ] || die "this gh sign-in does not administer $REPO; only a repository you administer may sign with your identity"
# The federated credential must equal the subject GitHub puts in the job's
# OIDC token, character for character. Since 2026-07-15 that subject carries
# the owner and repository ids ("immutable subject"), and a repository can
# customise it; so the prefix is read from the repository, never assumed.
OIDC=$(gh api "repos/$REPO/actions/oidc/customization/sub" 2>/dev/null) || die "could not read the OIDC subject settings of $REPO"
[ "$(printf '%s' "$OIDC" | jq -r .use_default)" = "true" ] || die "$REPO uses a customised OIDC subject template; this wizard supports the default one — reset it or edit SUBJECT here"
SUB_PREFIX=$(printf '%s' "$OIDC" | jq -r '.sub_claim_prefix // empty')
[ -n "$SUB_PREFIX" ] || SUB_PREFIX="repo:$REPO"
SUBJECT="$SUB_PREFIX:environment:$GH_ENV"
say "In the Azure portal you have already completed an Individual identity"
say "validation and created a Public Trust certificate profile. This wizard"
say "connects the release workflow of $REPO to that profile:"
say ""
if [ "$REPO" = "$DEFAULT_REPO" ]; then
  step "an Entra app registration '$APP_NAME' with ONE federated credential:"
else
  step "on the Entra app registration '$APP_NAME' (the desktop's, created if it is missing), ONE more federated credential '$FIC_NAME':"
fi
note "    issuer   $ISSUER"
note "    subject  $SUBJECT"
note "    audience $AUDIENCE"
step "the Certificate Profile Signer role for it, scoped to your signing account"
step "the GitHub environment '$GH_ENV' of $REPO, admitting only main and v* tags — created BEFORE the credential that names it — and six repository variables (not secrets)"
say ""
say "No secret is created by any stage: the app registration gets no client"
say "secret and no certificate, and nothing is written to a runner. A job in"
say "the '$GH_ENV' environment proves who it is with the OIDC token GitHub mints"
say "for it, and Azure answers with an access token that lives for the job."
say ""
confirm "continue?"

stage "The Azure CLI"
if ! command -v az >/dev/null 2>&1; then
  if [ -x "$VENV/bin/az" ]; then
    PATH="$VENV/bin:$PATH"; export PATH
    step "using the Azure CLI installed earlier in $VENV"
  else
    say "The Azure CLI is not installed. It can go into a private virtualenv,"
    say "$VENV, with no root and no system package — a few minutes."
    say ""
    confirm "install the Azure CLI there now?"
    python3 -m venv "$VENV" || die "could not create the virtualenv"
    "$VENV/bin/pip" install --quiet --upgrade pip || die "pip could not upgrade itself"
    "$VENV/bin/pip" install --quiet azure-cli || die "pip could not install azure-cli"
    PATH="$VENV/bin:$PATH"; export PATH
    step "installed the Azure CLI in $VENV"
    SUMMARY+=("Azure CLI -> $VENV (a private virtualenv; remove it when you are done)")
  fi
fi
step "az $(az version --query '"azure-cli"' -o tsv 2>/dev/null || echo '?')"
# The service was renamed from Trusted Signing to Artifact Signing in 2026;
# the CLI extension and command group follow whichever this az knows.
AZ_GROUP=""
for ext in artifact-signing trustedsigning; do
  if az extension show --name "$ext" >/dev/null 2>&1 || az extension add --name "$ext" >/dev/null 2>&1; then
    AZ_GROUP=$ext; break
  fi
done
[ -n "$AZ_GROUP" ] || die "neither the 'artifact-signing' nor the 'trustedsigning' CLI extension could be added"
step "extension '$AZ_GROUP' present (command group: az $AZ_GROUP)"

stage "Sign in to Azure"
say "A device code appears below. Open the URL in a browser signed in as the"
say "account that owns the subscription, enter the code, and come back."
say ""
az login --use-device-code -o none || die "az login failed"
mapfile -t SUBS < <(azj account list --query "[?state=='Enabled'].[id,name,tenantId]" | jq -r '.[] | @tsv')
[ ${#SUBS[@]} -gt 0 ] || die "no enabled subscription is visible to this sign-in"
if [ ${#SUBS[@]} -gt 1 ]; then
  say "Subscriptions:"
  for s in "${SUBS[@]}"; do note "  $(printf '%s' "$s" | cut -f1)  $(printf '%s' "$s" | cut -f2)"; done
  ask "subscription id that holds the Artifact Signing account" SUBSCRIPTION_ID
  az account set -s "$SUBSCRIPTION_ID" || die "could not select $SUBSCRIPTION_ID"
fi
ACCOUNT_CTX=$(azj account show) || die "az account show failed"
SUBSCRIPTION_ID=$(printf '%s' "$ACCOUNT_CTX" | jq -r .id)
TENANT_ID=$(printf '%s' "$ACCOUNT_CTX" | jq -r .tenantId)
step "subscription $SUBSCRIPTION_ID"
step "tenant       $TENANT_ID"

stage "The signing account and its certificate profile"
ACCOUNTS=$(azj "$AZ_GROUP" list | flat) || die "could not list signing accounts (is the Microsoft.CodeSigning provider registered?)"
[ "$(count "$ACCOUNTS")" -gt 0 ] || die "no Artifact Signing account in subscription $SUBSCRIPTION_ID: create one in the portal first"
if [ "$(count "$ACCOUNTS")" -gt 1 ]; then
  say "Accounts:"; printf '%s' "$ACCOUNTS" | jq -r '.[] | "    \(.name)  (\(.resourceGroup), \(.location))"'
  ask "account name" ACCOUNT_NAME
else
  ACCOUNT_NAME=$(printf '%s' "$ACCOUNTS" | jq -r '.[0].name')
fi
ACCOUNT=$(printf '%s' "$ACCOUNTS" | jq --arg n "$ACCOUNT_NAME" '.[] | select(.name==$n)')
[ -n "$ACCOUNT" ] || die "no account named '$ACCOUNT_NAME'"
ACCOUNT_ID=$(printf '%s' "$ACCOUNT" | jq -r .id)
RESOURCE_GROUP=$(printf '%s' "$ACCOUNT" | jq -r .resourceGroup)
LOCATION=$(printf '%s' "$ACCOUNT" | jq -r '.location | ascii_downcase')
# The signing endpoint: the account states it (accountUri), or it follows the
# region code (the table in Microsoft's quickstart), or the operator gave it.
# Which of the first two this az returns has not been observed yet; both are
# kept until one run says.
if [ -z "${ENDPOINT:-}" ]; then
  ENDPOINT=$(printf '%s' "$ACCOUNT" | jq -r '.accountUri // empty' | sed -e 's#^https://##' -e 's#/$##')
fi
if [ -z "$ENDPOINT" ]; then
  case "$LOCATION" in
    brazilsouth) code=brs;; centralus) code=cus;; eastus) code=eus;; japaneast) code=jpe;;
    koreacentral) code=krc;; northcentralus) code=ncus;; northeurope) code=neu;; polandcentral) code=plc;;
    southcentralus) code=scus;; switzerlandnorth) code=swn;; westcentralus) code=wcus;; westeurope) code=weu;;
    westus) code=wus;; westus2) code=wus2;; westus3) code=wus3;;
    *) die "no known signing endpoint for region '$LOCATION'; look it up in the quickstart and re-run with ENDPOINT=<host>" ;;
  esac
  ENDPOINT="$code.codesigning.azure.net"
fi
step "account   $ACCOUNT_NAME  ($RESOURCE_GROUP, $LOCATION, $(printf '%s' "$ACCOUNT" | jq -r '.sku.name // "sku ?"'), provisioning $(printf '%s' "$ACCOUNT" | jq -r '.provisioningState // "?"'))"
step "endpoint  $ENDPOINT"
PROFILES=$(azj "$AZ_GROUP" certificate-profile list --account-name "$ACCOUNT_NAME" -g "$RESOURCE_GROUP" | flat) || die "could not list certificate profiles"
PUBLIC=$(printf '%s' "$PROFILES" | jq '[.[] | select(.profileType == "PublicTrust")]')
[ "$(count "$PUBLIC")" -gt 0 ] || die "no Public Trust certificate profile on $ACCOUNT_NAME: create one in the portal first (it needs a Completed identity validation)"
if [ "$(count "$PUBLIC")" -gt 1 ]; then
  say "Public Trust profiles:"; printf '%s' "$PUBLIC" | jq -r '.[] | "    \(.name)  (\(.status // "?"))"'
  ask "profile name" PROFILE_NAME
else
  PROFILE_NAME=$(printf '%s' "$PUBLIC" | jq -r '.[0].name')
fi
PROFILE=$(printf '%s' "$PUBLIC" | jq --arg n "$PROFILE_NAME" '.[] | select(.name==$n)')
[ -n "$PROFILE" ] || die "no Public Trust profile named '$PROFILE_NAME'"
PROFILE_STATUS=$(printf '%s' "$PROFILE" | jq -r '.status // "?"')
step "profile   $PROFILE_NAME  (status $PROFILE_STATUS)"
SUBJECT_CN=$(printf '%s' "$PROFILE" | jq -r '.commonName // empty')
[ -n "$SUBJECT_CN" ] && step "subject   CN=$SUBJECT_CN — the name Windows will show as the publisher"
# The CLI does not expose identity validations; an Active Public Trust profile
# can only exist over a Completed one, which is the evidence this stage has.
if [ "$PROFILE_STATUS" = "Active" ]; then
  note "an Active Public Trust profile means its identity validation completed"
else
  warn "the profile is not Active; signing will fail until it is"; confirm "continue anyway?"
fi

stage "GitHub: the release environment, before the credential that names it"
say "The credential the next stage creates matches ONE subject: a job of $REPO"
say "that runs in the '$GH_ENV' environment. GitHub lets a job enter that"
say "environment only from the refs its deployment policy admits — main, for"
say "dry runs, and v* tags, for releases — so a job on any other branch, a pull"
say "request, or a fork presents no such subject and gets no token. The"
say "environment and its policy exist first, so there is no moment at which a"
say "credential names an environment any ref could enter."
say ""
gh api -X PUT "repos/$REPO/environments/$GH_ENV" --input - >/dev/null 2>&1 <<'JSON' \
  || die "could not create the '$GH_ENV' environment on $REPO with this gh sign-in (it needs repository admin); nothing has been created in Azure"
{"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}
JSON
step "environment '$GH_ENV' exists on $REPO with a custom deployment policy"
POLICIES=$(gh api "repos/$REPO/environments/$GH_ENV/deployment-branch-policies" 2>/dev/null | jq -c '[.branch_policies[] | {name, type: (.type // "branch")}]' 2>/dev/null || echo '[]')
for want in 'main branch' 'v* tag'; do
  pname=${want% *}; ptype=${want#* }
  if printf '%s' "$POLICIES" | jq -e --arg n "$pname" --arg t "$ptype" '.[] | select(.name==$n and .type==$t)' >/dev/null; then
    step "policy: $ptype $pname admitted"
  elif gh api -X POST "repos/$REPO/environments/$GH_ENV/deployment-branch-policies" -f "name=$pname" -f "type=$ptype" >/dev/null 2>&1; then
    step "policy: $ptype $pname admitted (added)"
    SUMMARY+=("environment '$GH_ENV' of $REPO admits $ptype $pname")
  else
    die "could not add the deployment policy for $ptype $pname on $REPO; nothing has been created in Azure — fix the environment (Settings → Environments → $GH_ENV) and re-run"
  fi
done

stage "The identity a release job will present"
# --display-name is a prefix match in the CLI; select the exact name.
APPS=$(azj ad app list --display-name "$APP_NAME") || die "could not list app registrations"
APP_ID=$(printf '%s' "$APPS" | jq -r --arg n "$APP_NAME" '[.[] | select(.displayName==$n)] | .[0].appId // empty')
if [ -z "$APP_ID" ]; then
  APP_ID=$(azj ad app create --display-name "$APP_NAME" --sign-in-audience AzureADMyOrg | jq -r '.appId // empty')
  [ -n "$APP_ID" ] || die "could not create the app registration (your account may not be allowed to register applications)"
  step "created app registration '$APP_NAME'"
  SUMMARY+=("Entra app registration '$APP_NAME' (client id $APP_ID)")
else
  step "app registration '$APP_NAME' exists"
fi
SP_OBJECT_ID=$(azj ad sp show --id "$APP_ID" | jq -r '.id // empty')
if [ -z "$SP_OBJECT_ID" ]; then
  SP_OBJECT_ID=$(azj ad sp create --id "$APP_ID" | jq -r '.id // empty')
  [ -n "$SP_OBJECT_ID" ] || die "could not create the service principal"
  step "created its service principal"
else
  step "its service principal exists"
fi
step "client id  $APP_ID"
FICS=$(azj ad app federated-credential list --id "$APP_ID") || die "could not list the app's federated credentials"
FIC=$(printf '%s' "$FICS" | jq --arg s "$SUBJECT" '[.[] | select(.subject==$s)]')
if [ "$(count "$FIC")" -eq 0 ]; then
  OTHER=$(printf '%s' "$FICS" | jq -r --arg n "$FIC_NAME" '[.[] | select(.name==$n)] | .[0].subject // empty')
  [ -z "$OTHER" ] || die "a federated credential named '$FIC_NAME' exists with subject '$OTHER', not '$SUBJECT' (the repository's subject prefix changed?); delete it in Entra and re-run"
  WORK=$(mktemp -d "${TMPDIR:-/tmp}/agentry-artifact-signing.XXXXXX")
  jq -n --arg n "$FIC_NAME" --arg i "$ISSUER" --arg s "$SUBJECT" --arg a "$AUDIENCE" \
    --arg d "$REPO release workflow, environment $GH_ENV (agentry-desktop#151, #173)" \
    '{name:$n, issuer:$i, subject:$s, audiences:[$a], description:$d}' \
    > "$WORK/fic.json"
  azj ad app federated-credential create --id "$APP_ID" --parameters "@$WORK/fic.json" >/dev/null \
    || die "could not create the federated credential"
  step "created federated credential '$FIC_NAME' for subject $SUBJECT"
  SUMMARY+=("federated credential '$FIC_NAME' -> subject $SUBJECT")
else
  step "a federated credential for subject $SUBJECT exists"
fi
# The whole point: no secret. Say so with evidence, on the application AND
# its service principal, each of which can carry passwords and certificates.
NSECRETS=$(count "$(azj ad app credential list --id "$APP_ID")")
NCERTS=$(count "$(azj ad app credential list --id "$APP_ID" --cert)")
NSPSECRETS=$(count "$(azj ad sp credential list --id "$SP_OBJECT_ID")")
NSPCERTS=$(count "$(azj ad sp credential list --id "$SP_OBJECT_ID" --cert)")
if [ "$((NSECRETS + NCERTS + NSPSECRETS + NSPCERTS))" -eq 0 ]; then
  step "neither the registration nor its service principal holds a client secret or a certificate"
else
  warn "found $NSECRETS/$NCERTS secret(s)/certificate(s) on the registration and $NSPSECRETS/$NSPCERTS on the service principal; the release path needs none — remove them"
fi

stage "The role on your signing account"
ROLE=""
for name in "Artifact Signing Certificate Profile Signer" "Trusted Signing Certificate Profile Signer"; do
  if [ "$(count "$(azj role definition list --name "$name")")" -gt 0 ]; then ROLE=$name; break; fi
done
[ -n "$ROLE" ] || die "the Certificate Profile Signer role definition was not found in this tenant"
HAVE=$(azj role assignment list --assignee "$SP_OBJECT_ID" --scope "$ACCOUNT_ID" --role "$ROLE") || die "could not list role assignments"
if [ "$(count "$HAVE")" -gt 0 ]; then
  step "'$ROLE' is already assigned on $ACCOUNT_NAME"
else
  azj role assignment create --role "$ROLE" --assignee-object-id "$SP_OBJECT_ID" \
    --assignee-principal-type ServicePrincipal --scope "$ACCOUNT_ID" >/dev/null \
    || die "could not assign '$ROLE' (you need Owner or User Access Administrator on the account)"
  step "assigned '$ROLE' on $ACCOUNT_NAME, and nowhere wider"
  SUMMARY+=("role '$ROLE' -> $APP_NAME on account $ACCOUNT_NAME")
fi
# "Nowhere wider" is only true if nothing else was granted earlier.
ALL=$(azj role assignment list --assignee "$SP_OBJECT_ID" --all) || die "could not list the principal's role assignments"
EXTRA=$(printf '%s' "$ALL" | jq -r --arg s "$ACCOUNT_ID" '[.[] | select((.scope | ascii_downcase) != ($s | ascii_downcase))] | .[] | "\(.roleDefinitionName) on \(.scope)"')
if [ -n "$EXTRA" ]; then
  warn "this identity also holds role assignments outside the signing account:"; printf '%s\n' "$EXTRA" | sed 's/^/      /'
  warn "the release path needs only the one above — remove the others"
else
  step "this identity holds no role assignment outside $ACCOUNT_NAME"
fi
note "Role assignments take a minute or two to propagate; the probe at the end may need one retry."

stage "GitHub: the variables"
say "Six identifiers the release workflow of $REPO reads (not secrets: a tenant,"
say "a client, a subscription, an endpoint, an account and a profile name)."
say ""
set_var AZURE_TENANT_ID            "$TENANT_ID"       "$REPO"
set_var AZURE_CLIENT_ID            "$APP_ID"          "$REPO"
set_var AZURE_SUBSCRIPTION_ID      "$SUBSCRIPTION_ID" "$REPO"
set_var ARTIFACT_SIGNING_ENDPOINT  "$ENDPOINT"        "$REPO"
set_var ARTIFACT_SIGNING_ACCOUNT   "$ACCOUNT_NAME"    "$REPO"
set_var ARTIFACT_SIGNING_PROFILE   "$PROFILE_NAME"    "$REPO"
# No Azure secret belongs in the repository either; say what the list holds.
AZSECRETS=$(gh secret list -R "$REPO" --json name --jq '[.[].name | select(test("^(AZURE|ARTIFACT_SIGNING)_"))] | join(", ")' 2>/dev/null || true)
if [ -n "$AZSECRETS" ]; then
  warn "repository secrets named for Azure exist: $AZSECRETS — the release path reads none; remove them"
else
  step "no repository secret is named for Azure"
fi

stage "Proof"
say "Two proofs. The first is yours: your own sign-in can obtain a token for"
say "the signing service, which shows the service is reachable. The second is"
say "the job's, and it is the one that counts: the '$PROBE_WORKFLOW' workflow"
say "runs a job in the '$GH_ENV' environment on the Linux runner, exchanges its"
say "OIDC token for an Azure access token, and prints nothing but the outcome."
say "(The release workflow's own signing job runs in that environment.)"
say ""
EXPIRES=$(az account get-access-token --resource "$RESOURCE" --query expiresOn -o tsv 2>/dev/null) \
  && step "your sign-in obtained a token for $RESOURCE (expires $EXPIRES)" \
  || warn "your sign-in could not obtain a token for $RESOURCE; the job's proof below is the one that matters"
say ""
# The probe is offered only where it exists: a repository whose main has no
# probe workflow is told so, never silently left unproven — and a failure to
# ask GitHub is reported as that, not as the file's absence.
PROBE_STATUS=$(gh api "repos/$REPO/contents/.github/workflows/$PROBE_WORKFLOW?ref=main" --silent --include 2>&1 | sed -n '1s/^HTTP\/[0-9.]* \([0-9]*\).*/\1/p')
case "$PROBE_STATUS" in
  200) read -r -p "  run the $PROBE_WORKFLOW workflow on $REPO now and wait for it? [Y/n]: " ans ;;
  404) warn "$REPO has no .github/workflows/$PROBE_WORKFLOW on main, so the credential cannot be proven from a job yet:"
       warn "merge that repository's release work first (it carries the probe), then run the probe by hand (below)."
       SUMMARY+=("probe NOT run: $REPO has no $PROBE_WORKFLOW on main yet")
       ans="n" ;;
  *)   warn "could not ask GitHub whether $REPO carries $PROBE_WORKFLOW (HTTP ${PROBE_STATUS:-none}); run the probe by hand (below)."
       SUMMARY+=("probe NOT run: GitHub could not be asked about $PROBE_WORKFLOW on $REPO")
       ans="n" ;;
esac
case "$ans" in
  n|N|no|NO) note "by hand: gh workflow run $PROBE_WORKFLOW -R $REPO --ref main && gh run watch -R $REPO --exit-status" ;;
  *)
    BEFORE=$(gh run list -R "$REPO" --workflow "$PROBE_WORKFLOW" -L 20 --json databaseId --jq '[.[].databaseId]' 2>/dev/null || echo '[]')
    if gh workflow run "$PROBE_WORKFLOW" -R "$REPO" --ref main 2>/dev/null; then
      RUN_ID=""; tries=0
      while [ -z "$RUN_ID" ] && [ $tries -lt 12 ]; do
        sleep 5; tries=$((tries + 1))
        RUN_ID=$(gh run list -R "$REPO" --workflow "$PROBE_WORKFLOW" -L 20 --json databaseId 2>/dev/null \
          | jq -r --argjson b "$BEFORE" '[.[].databaseId | select(. as $id | $b | index($id) | not)] | .[0] // empty' 2>/dev/null || true)
      done
      if [ -z "$RUN_ID" ]; then
        warn "the run did not appear within a minute; find it with: gh run list -R $REPO --workflow $PROBE_WORKFLOW"
      else
        note "waiting for run $RUN_ID (a self-hosted job queues until that repository's runner is free)…"
        if gh run watch "$RUN_ID" -R "$REPO" --exit-status >/dev/null 2>&1; then
          step "the probe job obtained an Artifact Signing access token: run $RUN_ID"
          SUMMARY+=("probe run $RUN_ID green: the release environment can obtain a signing token")
        else
          warn "the probe did not pass (run $RUN_ID); read its log with: gh run view $RUN_ID -R $REPO --log"
          warn "a fresh role assignment can take a couple of minutes to propagate — re-run the probe before anything else"
        fi
      fi
    else
      warn "could not start $PROBE_WORKFLOW: it exists on main only once the branch carrying it is merged"
    fi ;;
esac
say ""
read -r -p "  sign out of Azure on this machine now? [Y/n]: " ans
case "$ans" in n|N|no|NO) note "still signed in: this machine keeps a refresh token under ~/.azure until 'az logout'" ;; *) az logout >/dev/null 2>&1 && step "signed out of Azure" ;; esac

finish
