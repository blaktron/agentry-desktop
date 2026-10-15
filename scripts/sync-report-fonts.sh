#!/usr/bin/env bash
# The Report design page's web fonts.
#
#   scripts/sync-report-fonts.sh [AGENTRY_WEB_CHECKOUT] [REF]
#
# The preview draws a design's faces as web fonts: the fourteen families
# the Typst image carries, as Latin WOFF2. The site makes them from the
# image's hash-pinned files (its sync-report-fonts.py, which records each
# source and digest in PROVENANCE.json); this copies the site's, at REF
# (default origin/dev) of a checkout of it (default ~/agentry/agentry-web), into
# frontend/src/reportFonts/, with their licences and that provenance, and
# writes SOURCE naming the commit. fonts.css is the site's with each family
# renamed "Report <family>", so a face the app sets itself (IBM Plex Sans)
# is never replaced by a report's. frontend/tests/reportDesign.test.ts holds
# every file to the provenance's digest and every family of the core's list
# to a face here.
set -euo pipefail

web=${1:-$HOME/agentry/agentry-web}
ref=${2:-origin/dev}
src=app/settings/report-design/fonts
out=$(cd "$(dirname "$0")/.." && pwd)/frontend/src/reportFonts

commit=$(git -C "$web" rev-parse --verify "$ref^{commit}")
files=$(git -C "$web" ls-tree -r --name-only "$commit" -- "$src")
if [ -z "$files" ]; then
  echo "no $src at $commit in $web" >&2
  exit 1
fi

rm -rf "$out"
mkdir -p "$out/licenses"
while IFS= read -r f; do
  git -C "$web" show "$commit:$f" >"$out/${f#"$src"/}"
done <<<"$files"

sed -i -E 's/font-family: "([^"]+)";/font-family: "Report \1";/' "$out/fonts.css"
sed -i '1s|^/\*|/* Copied from agentry-web by agentry-desktop scripts/sync-report-fonts.sh,\n   each family renamed "Report <family>" (SOURCE names the commit).\n  |' "$out/fonts.css"
printf 'agentry-web %s %s\n' "$commit" "$src" >"$out/SOURCE"
echo "copied $(wc -l <<<"$files") files from agentry-web $commit to $out"
