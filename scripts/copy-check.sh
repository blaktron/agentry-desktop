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
# half, over the copy this app ships — the frontend's own sources, the
# Go files the bound surfaces return their sentences from, and the Windows
# installer's pages.
#
# A word that is there because the rule is what the sentence is about — a
# comment quoting the rule, a protocol value named "safe" — passes by being
# listed in scripts/copy-allow.txt as `reason :: span`. Adding a line there
# is a claim that the span is honest, and is worth arguing in a PR. The span
# exempts itself, not its line: every allowed span is cut out of the line
# and the rest is checked again, so a never-word beside an allowed span
# still fails (agentry-desktop#127).
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
  # The Windows installer's pages (desktop#83); wails_tools.nsh is generated
  # and carries no sentence of ours.
  [ -d "$ROOT/build/windows/installer" ] && find "$ROOT/build/windows/installer" -maxdepth 1 -type f -name '*.nsi' -print
  return 0
}

# The allowlist's spans, one per line (the part after ` :: `).
allowed_spans() {
  [ -f "$ALLOW" ] || return 0
  local entry span
  while IFS= read -r entry; do
    case "$entry" in '' | \#*) continue ;; esac
    span="${entry#* :: }"
    [ "$span" = "$entry" ] && continue
    printf '%s\n' "$span"
  done < "$ALLOW"
}

# strip_allowed <line> — the line with every allowlisted span cut out,
# matched case-insensitively (as the words are). awk, not sed: a span is a
# fixed string, and sed would read its punctuation as a pattern.
strip_allowed() {
  # The spans reach awk through the environment, not -v, which would
  # rewrite any backslash in them.
  printf '%s\n' "$1" | COPY_CHECK_SPANS="$SPANS" awk '
    BEGIN { n = split(ENVIRON["COPY_CHECK_SPANS"], span, "\n") }
    {
      line = $0
      for (k = 1; k <= n; k++) {
        if (span[k] == "") continue
        s = tolower(span[k])
        while ((i = index(tolower(line), s)) > 0)
          line = substr(line, 1, i - 1) " " substr(line, i + length(s))
      }
      print line
    }'
}

check_tree() {
  local status=0 file hit
  SPANS="$(allowed_spans)"
  while IFS= read -r file; do
    while IFS= read -r hit; do
      # What is left once the allowed spans are gone must carry no word.
      # (>/dev/null, not -q: grep -q would cut the writer off and pipefail
      # would read its EPIPE as the answer; agentry-desktop#236.)
      if ! strip_allowed "${hit#*:}" | grep -iE "\\b($WORDS)\\b" >/dev/null; then
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
  # An allowed span exempts itself, never the rest of its line: the first
  # span in the allowlist, then a never-word beside it, must fail; the span
  # alone must pass.
  span="$(COPY_CHECK_ALLOW="$ALLOW" ALLOW="$ALLOW" allowed_spans | head -1)"
  if [ -n "$span" ]; then
    printf '// %s\n' "$span" > "$tmp/frontend/src/Bad.tsx"
    if ! COPY_CHECK_ROOT="$tmp" COPY_CHECK_ALLOW="$ALLOW" "$HERE/copy-check.sh" >/dev/null 2>&1; then
      echo "self-test FAILED: an allowlisted span on its own was refused" >&2
      exit 1
    fi
    printf '// %s, and your run is safe here\n' "$span" > "$tmp/frontend/src/Bad.tsx"
    if COPY_CHECK_ROOT="$tmp" COPY_CHECK_ALLOW="$ALLOW" "$HERE/copy-check.sh" >/dev/null 2>&1; then
      echo "self-test FAILED: a never-word beside an allowlisted span passed the check" >&2
      exit 1
    fi
  fi
  echo "self-test ok: the check refuses a never-word, also beside an allowed span, and passes honest copy"
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
