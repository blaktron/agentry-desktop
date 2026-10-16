#!/usr/bin/env bash
# Keeps a failed CI build's packages on the runner host that built them,
# instead of in GitHub's metered artifact storage.
#
#   scripts/ci-keep-build.sh <job-status> <file-or-glob>...
#
# Run at the end of a packaging job with `if: always()` and
# `${{ job.status }}`. Every run first prunes kept builds older than
# CI_KEEP_DAYS (7) — that is the clean-up, so no cron is involved — then,
# only when the job failed, copies the named files that exist into
#
#   <root>/agentry-desktop/<run id>-<attempt>-<job>/
#
# where <root> is, unless CI_KEEP_ROOT says otherwise:
#   Linux runners  /opt/runner/cache/artifacts — the host directory every
#                  runner container mounts, so the build outlives the
#                  container;
#   the Mac        ~/runner-artifacts of the runner's user, beside
#                  ~/runner-cache.
# The host and the path go to the log and to the job summary. A green build
# keeps nothing.
set -euo pipefail

status=${1:?usage: $0 <job-status> <file>...}
shift

keep_days=${CI_KEEP_DAYS:-7}
case "$(uname -s)" in
  Darwin) default_root="$HOME/runner-artifacts" ;;
  *) default_root=/opt/runner/cache/artifacts ;;
esac
root="${CI_KEEP_ROOT:-$default_root}/agentry-desktop"

host=$(hostname)

mkdir -p "$root"
# Prune first, whatever the outcome, so the store stays bounded even when
# failures are rare.
find "$root" -mindepth 1 -maxdepth 1 -type d -mtime +"$keep_days" -exec rm -rf {} + 2>/dev/null || true

if [ "$status" != failure ]; then
  echo "job $status: no build kept (kept failures older than $keep_days days pruned from $root on $host)"
  exit 0
fi

files=()
for f in "$@"; do
  [ -f "$f" ] && files+=("$f")
done
if [ ${#files[@]} -eq 0 ]; then
  echo "job failed before any package was built: nothing to keep"
  exit 0
fi

# Never fill the runner's disk for a convenience copy.
free_kb=$(df -Pk "$root" | awk 'NR == 2 { print $4 }')
need_kb=$(du -ck "${files[@]}" | awk 'END { print $1 }')
if [ "${free_kb:-0}" -lt $((need_kb + 2 * 1024 * 1024)) ]; then
  echo "::warning::not keeping the failed build: less than 2 GB would be left free on $host"
  exit 0
fi

dest="$root/${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-1}-${GITHUB_JOB:-job}"
mkdir -p "$dest"
cp "${files[@]}" "$dest/"
msg="The failed build is kept for $keep_days days on $host (runner ${RUNNER_NAME:-?}) at $dest"
echo "::notice::$msg"
ls -la "$dest"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### Failed build kept on the runner host"
    echo
    echo "$msg:"
    echo
    for f in "${files[@]}"; do echo "- \`$(basename "$f")\`"; done
  } >>"$GITHUB_STEP_SUMMARY"
fi
