#!/usr/bin/env bash
# Mobile viewport gate (GDK-868).
# Layer 1: DESIGN.md §4.1 / §4.2 (ios-contract.sh).
# Layer 2: Playwright at 402×874 against `gadak demo` on 127.0.0.1:7899
# and vite on 127.0.0.1:5182. Does not bind 7877 or use e2e/.tmp/home.
set -euo pipefail
MOBILE="$(cd "$(dirname "$0")/.." && pwd)"
ROOT="$(cd "$MOBILE/.." && pwd)"
bash "$MOBILE/scripts/ios-contract.sh"
PW="$ROOT/node_modules/.bin/playwright"
if [[ ! -x "$PW" ]]; then
  echo "viewport-gate: repo-root Playwright missing — run npm ci at ${ROOT}" >&2
  exit 1
fi
cd "$ROOT"
exec "$PW" test --config "$MOBILE/playwright.config.ts" "$@"
