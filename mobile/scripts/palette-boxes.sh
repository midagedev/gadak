#!/usr/bin/env bash
# One command for "what are these rows' edges?" (GDK-1948).
# Runs the palette-inset spec's probe half and prints every row's content-box
# left/right edge in the open scope sheet — palette rows, section headers,
# the more/recent buttons, and the desk rows — plus the board and page-edit
# desk rows outside it, so an inset regression explains itself in the run
# log. The spec's shared-edge assertions run too, so the probe doubles as
# the gate. Same rig as heading-boxes.sh: built bundle + `gadak demo` via
# mobile/e2e/serve.ts, ports from GADAK_MOBILE_E2E_PORT /
# GADAK_MOBILE_API_PORT (defaults 5182 / 7899) — give parallel rounds
# their own pair.
set -euo pipefail
MOBILE="$(cd "$(dirname "$0")/.." && pwd)"
ROOT="$(cd "$MOBILE/.." && pwd)"
PW="$ROOT/node_modules/.bin/playwright"
if [[ ! -x "$PW" ]]; then
  echo "palette-boxes: repo-root Playwright missing — run npm ci at ${ROOT}" >&2
  exit 1
fi
cd "$ROOT"
exec "$PW" test --config "$MOBILE/playwright.config.ts" "$MOBILE/e2e/palette-inset.spec.ts" "$@"
