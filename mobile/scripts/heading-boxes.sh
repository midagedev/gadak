#!/usr/bin/env bash
# One command for "why is the list header cut off in <locale>?" (GDK-1936).
# Runs the heading spec's probe half and prints the header row's box
# measurements — .head/.scope/.name (clientWidth vs scrollWidth)/.count/
# .spacer/.new/.gear/.fresh (width and line count) — for the locale(s) you
# ask for:
#
#   bash mobile/scripts/heading-boxes.sh              # en, ko and ja
#   GADAK_HEADING_LOCALE=ja bash mobile/scripts/heading-boxes.sh
#   GADAK_HEADING_SHOT_DIR=/tmp/x bash mobile/scripts/heading-boxes.sh   # + stills
#
# The spec's ko/ja assertions run too, so the probe doubles as the gate.
# Same rig as viewport-gate.sh: built bundle + `gadak demo` via
# mobile/e2e/serve.ts, ports from GADAK_MOBILE_E2E_PORT /
# GADAK_MOBILE_API_PORT (defaults 5182 / 7899) — give parallel rounds
# their own pair.
set -euo pipefail
MOBILE="$(cd "$(dirname "$0")/.." && pwd)"
ROOT="$(cd "$MOBILE/.." && pwd)"
PW="$ROOT/node_modules/.bin/playwright"
if [[ ! -x "$PW" ]]; then
  echo "heading-boxes: repo-root Playwright missing — run npm ci at ${ROOT}" >&2
  exit 1
fi
cd "$ROOT"
exec "$PW" test --config "$MOBILE/playwright.config.ts" "$MOBILE/e2e/heading.spec.ts" "$@"
