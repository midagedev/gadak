#!/usr/bin/env bash
# Serve dist/hosted at http://127.0.0.1:4173/gadak/ so the demo entry can be
# checked at 390×844 (phone / in-app) or any desktop width.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
if [[ ! -f "$ROOT/dist/hosted/index.html" ]]; then
  echo "hosted-demo: building first (dist/hosted/index.html missing)" >&2
  node "$ROOT/tools/hosted-demo/build.mjs"
fi
# Staleness gate: a build from before the overlay was removed (GDK-335)
# still carries the old first-frame markup in index.html.
if grep -q 'data-testid="hosted-first-frame"' "$ROOT/dist/hosted/index.html"; then
  echo "hosted-demo: index.html still has the removed first-frame overlay — rebuild with node tools/hosted-demo/build.mjs" >&2
  exit 1
fi
rm -rf "$ROOT/dist/pages"
mkdir -p "$ROOT/dist/pages/gadak"
cp -R "$ROOT/dist/hosted/." "$ROOT/dist/pages/gadak/"
echo "hosted-demo: http://127.0.0.1:4173/gadak/  (390×844 to check the demo entry)" >&2
exec npx --yes serve "$ROOT/dist/pages" -l 4173 --no-port-switching
