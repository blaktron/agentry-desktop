#!/usr/bin/env bash
# The desktop's copy check (plan hosted-runs.md §3, agentry-desktop#77).
#
#   scripts/copy-check.sh              run the check over the working tree
#   scripts/copy-check.sh --self-test  prove it fails when it should
#
# The honest posture is a rule about words: the app says what ran and what
# was enforced, and never calls a BriefAgent, a run or a registry "safe",
# "secure", "guaranteed" or "trusted". The site's prose gate holds the same
# line for the website (agentry-web scripts/prose/); this is the desktop's
# half, over the copy this app ships — the frontend's own sources and the
# Go files the bound surfaces return their sentences from.
#
# A word that is there because the rule is what the sentence is about — a
# comment quoting the rule, a protocol value named "safe" — passes by being
# listed in scripts/copy-allow.txt as `reason :: span`. Adding a line there
# is a claim that the span is honest, and is worth arguing in a PR.
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# ROOT and ALLOW are overridable so --self-test can run this same script
# against a tree it builds.
ROOT="${COPY_CHECK_ROOT:-$(cd "$HERE/.." && pwd)}"
ALLOW="${COPY_CHECK_ALLOW:-$HERE/copy-allow.txt}"

# The words, matched whole and case-insensitively. "secure" does not match
# secureagentry.net: a word boundary needs a non-word character after it.
WORDS='safe|safely|secure|securely|secured|guarantee|guarantees|guaranteed|trusted'

# The files this app's copy lives in: the frontend it ships (not the
# dev-only harness, not the generated bindings) and its Go sources (not
# their tests).
copy_files() {
  [ -d "$ROOT/frontend/src" ] && find "$ROOT/frontend/src" -type f \( -name '*.ts' -o -name '*.tsx' \) -print
  find "$ROOT" -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' -print
  [ -d "$ROOT/internal" ] && find "$ROOT/internal" -type f -name '*.go' ! -name '*_test.go' -print
  return 0
}

# allowed <line> — true when the line carries an allowlisted span.
allowed() {
  local line="$1" entry span
  [ -f "$ALLOW" ] || return 1
  while IFS= read -r entry; do
    case "$entry" in '' | \#*) continue ;; esac
    span="${entry#* :: }"
    [ "$span" = "$entry" ] && continue
    if printf '%s' "$line" | grep -qiF -- "$span"; then
      return 0
    fi
  done < "$ALLOW"
  return 1
}

check_tree() {
  local status=0 file hit
  while IFS= read -r file; do
    while IFS= read -r hit; do
      if allowed "${hit#*:}"; then
        continue
      fi
      printf '%s:%s\n' "${file#"$ROOT"/}" "$hit" >&2
      status=1
    done < <(grep -nIiE "\\b($WORDS)\\b" "$file" || true)
  done < <(copy_files)
  return $status
}

if [ "${1:-}" = "--self-test" ]; then
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  mkdir -p "$tmp/frontend/src"
  printf 'export const line = "Your run is safe here.";\n' > "$tmp/frontend/src/Bad.tsx"
  if COPY_CHECK_ROOT="$tmp" COPY_CHECK_ALLOW="$ALLOW" "$HERE/copy-check.sh" >/dev/null 2>&1; then
    echo "self-test FAILED: a line calling a run safe passed the check" >&2
    exit 1
  fi
  printf 'export const line = "This run wrote three reports.";\n' > "$tmp/frontend/src/Bad.tsx"
  if ! COPY_CHECK_ROOT="$tmp" COPY_CHECK_ALLOW="$ALLOW" "$HERE/copy-check.sh" >/dev/null 2>&1; then
    echo "self-test FAILED: honest copy was refused" >&2
    exit 1
  fi
  echo "self-test ok: the check refuses a never-word and passes honest copy"
  exit 0
fi

if check_tree; then
  echo "copy check: no never-word in the desktop's shipped copy"
  exit 0
fi
cat >&2 <<'MSG'

The lines above use a word this product's copy does not use: the app says
what ran and what was enforced, never that something is safe, secure,
guaranteed or trusted. Rewrite the sentence, or — if the word is there
because the rule is what the sentence is about — add the span to
scripts/copy-allow.txt with the reason.
MSG
exit 1
