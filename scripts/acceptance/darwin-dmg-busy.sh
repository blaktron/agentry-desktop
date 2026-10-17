#!/usr/bin/env bash
# Proves how scripts/package-darwin-dmg.sh handles an hdiutil that reports
# "Resource busy": it tries again a bounded number of times, gives up at once
# on any other failure, and leaves nothing attached whatever happens.
#
#   scripts/acceptance/darwin-dmg-busy.sh
#
# Runs anywhere bash does: uname, ditto and hdiutil are stand-ins on PATH, so
# no disk image is made and nothing is mounted. The stand-in hdiutil keeps its
# state in files — how many creates to refuse, which images it holds attached
# — and records every call it receives.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
script="$root/scripts/package-darwin-dmg.sh"
work=$(mktemp -d "${TMPDIR:-/tmp}/agentry-dmg-busy.XXXXXX")
trap 'rm -rf "$work"' EXIT

fail() { echo "darwin-dmg-busy: $*" >&2; exit 1; }

bin="$work/bin"
mkdir -p "$bin" "$work/app/Contents" "$work/out"
echo bundle > "$work/app/Contents/file"
app="$work/app"
out="$work/out/agentry-desktop-test-darwin-universal.dmg"

cat > "$bin/uname" <<'EOF'
#!/bin/sh
echo Darwin
EOF
cat > "$bin/ditto" <<'EOF'
#!/bin/sh
cp -R "$1" "$2"
EOF
# hdiutil create refuses while $FAKE_STATE/refuse holds a positive count, with
# the message the real one prints (or $FAKE_REFUSAL), and attaches its image
# the way the real one does: by the output path, left attached when it fails.
# info lists what is attached; detach drops it.
cat > "$bin/hdiutil" <<'EOF'
#!/bin/sh
st=$FAKE_STATE
echo "$*" >> "$st/calls"
case "$1" in
  create)
    for last; do :; done
    echo "$last" > "$st/attached"
    echo "hdiutil: WARNING: 'hdiutil create -volname -format ...' is deprecated." >&2
    n=$(cat "$st/refuse" 2>/dev/null || echo 0)
    if [ "$n" -gt 0 ]; then
      echo $((n - 1)) > "$st/refuse"
      echo "hdiutil: create failed - ${FAKE_REFUSAL:-Resource busy}" >&2
      exit 16
    fi
    : > "$last"
    rm -f "$st/attached"
    ;;
  info)
    if [ -s "$st/attached" ]; then
      echo "image-path      : $(cat "$st/attached")"
      echo "/dev/disk9s1	48465300-0000-11AA-AA11-00306543ECAC	/Volumes/Agentry Desktop"
    fi
    ;;
  detach)
    rm -f "$st/attached"
    ;;
esac
EOF
chmod +x "$bin/uname" "$bin/ditto" "$bin/hdiutil"

# run <name> <creates to refuse> [stale]: one run of the script. Leaves its
# exit status in $status, its stderr in $work/stderr and the stand-in's state
# in $work/state.
run() {
  rm -rf "$work/state" "$out"
  mkdir -p "$work/state"
  echo "$2" > "$work/state/refuse"
  [ "${3:-}" = stale ] && echo "$out" > "$work/state/attached"
  status=0
  FAKE_STATE="$work/state" PACKAGE_DMG_RETRY_DELAY=0 PATH="$bin:$PATH" \
    "$script" "$app" "$out" > "$work/stdout" 2> "$work/stderr" || status=$?
}
creates() { grep -c '^create ' "$work/state/calls"; }
no_leftover() { [ ! -s "$work/state/attached" ] || fail "$1: the image was left attached"; }

# Busy twice, then the volume lets go: the third attempt makes the image.
run busy-twice 2
[ "$status" -eq 0 ] || fail "busy twice: exit $status, want 0 ($(cat "$work/stderr"))"
[ "$(creates)" -eq 3 ] || fail "busy twice: $(creates) creates, want 3"
[ -f "$out" ] || fail "busy twice: no image at $out"
no_leftover "busy twice"

# Busy every time: it stops at the bound, says so, and leaves nothing attached.
run always-busy 99
[ "$status" -ne 0 ] || fail "always busy: exit 0, want a failure"
[ "$(creates)" -eq 4 ] || fail "always busy: $(creates) creates, want 4"
grep -q 'failed (attempt 4 of 4)' "$work/stderr" || fail "always busy: the failure does not name the attempts"
no_leftover "always busy"

# Any other failure is final on the first attempt.
FAKE_REFUSAL="No space left on device" run other-failure 1
[ "$status" -ne 0 ] || fail "other failure: exit 0, want a failure"
[ "$(creates)" -eq 1 ] || fail "other failure: $(creates) creates, want 1"
grep -q 'No space left on device' "$work/stderr" || fail "other failure: hdiutil's own message was not passed on"
no_leftover "other failure"

# An image a killed run left attached at the same path is detached before the
# first create.
run stale 0 stale
[ "$status" -eq 0 ] || fail "stale attachment: exit $status, want 0 ($(cat "$work/stderr"))"
grep -q '^detach ' "$work/state/calls" || fail "stale attachment: it was not detached"
first=$(grep -n -m1 -e '^detach ' "$work/state/calls" | cut -d: -f1)
create_line=$(grep -n -m1 -e '^create ' "$work/state/calls" | cut -d: -f1)
[ "$first" -lt "$create_line" ] || fail "stale attachment: detached after the create, not before"
no_leftover "stale attachment"

echo "darwin-dmg-busy: ok — retries a busy volume to a bound, stops at once on any other failure, leaves nothing attached"
