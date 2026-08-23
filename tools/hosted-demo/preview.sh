#!/usr/bin/env bash
# Serve dist/hosted at http://127.0.0.1:4173/ — the same layout Pages publishes
# at the gadak.dev apex: / landing, /demo/ app, /backlog/ backlog viewer.
# Check the demo entry at 390×844 (phone / in-app) or any desktop width.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
if [[ ! -f "$ROOT/dist/hosted/index.html" ]]; then
  echo "hosted-demo: building first (dist/hosted/index.html missing)" >&2
  node "$ROOT/tools/hosted-demo/build.mjs"
fi
# Staleness gate: a build from before the overlay was removed (GDK-335)
# still carries the old first-frame markup in the demo's index.html.
if [[ -f "$ROOT/dist/hosted/demo/index.html" ]] &&
   grep -q 'data-testid="hosted-first-frame"' "$ROOT/dist/hosted/demo/index.html"; then
  echo "hosted-demo: demo/index.html still has the removed first-frame overlay — rebuild with node tools/hosted-demo/build.mjs" >&2
  exit 1
fi
# A build from before GDK-676 put the app at the root; the landing page is the
# tell that dist/hosted is the new three-door layout.
if [[ ! -f "$ROOT/dist/hosted/demo/index.html" ]]; then
  echo "hosted-demo: dist/hosted/demo/ missing — this is a pre-GDK-676 build, rebuild with node tools/hosted-demo/build.mjs" >&2
  exit 1
fi
echo "hosted-demo: http://127.0.0.1:4173/        (landing)" >&2
echo "hosted-demo: http://127.0.0.1:4173/demo/   (390×844 to check the demo entry)" >&2
echo "hosted-demo: http://127.0.0.1:4173/backlog/" >&2
exec npx --yes serve "$ROOT/dist/hosted" -l 4173 --no-port-switching
