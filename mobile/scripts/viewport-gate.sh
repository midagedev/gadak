#!/usr/bin/env bash
# Mobile viewport gate (GDK-868).
# Layer 1: DESIGN.md §4.1 / §4.2 (ios-contract.sh).
# Layer 2: Playwright at 402×874 against a *built* bundle (`vite preview`)
# and `gadak demo`, both started by mobile/e2e/serve.ts — ports from
# GADAK_MOBILE_E2E_PORT / GADAK_MOBILE_API_PORT (defaults 5182 / 7899), a
# provenance stamp refuses a server another worktree built (GDK-1540).
# Does not bind 7877 or use e2e/.tmp/home.
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
