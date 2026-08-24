#!/usr/bin/env bash
# Unattended regen of docs/media/{tokens,dashboards}.{mp4,gif}.
#
# Same-clock Playwright split (paper terminal | real app iframe). Commands
# are gadak's own against a frozen copy of examples/demo.db. No live site.
#
# Usage: bash e2e/demo/record-promo.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if [[ -z "${SKIP_NVM:-}" ]]; then
  export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
  if [[ -s "$NVM_DIR/nvm.sh" ]]; then
    # shellcheck disable=SC1090
    . "$NVM_DIR/nvm.sh"
    nvm use
  fi
fi

command -v ffmpeg >/dev/null || { echo "record-promo: ffmpeg required" >&2; exit 1; }
command -v ffprobe >/dev/null || { echo "record-promo: ffprobe required" >&2; exit 1; }
if [[ ! -x node_modules/.bin/playwright ]]; then
  echo "record-promo: playwright missing (npm ci)" >&2
  exit 1
fi

PORT="${GADAK_E2E_PORT:-7888}"
if ! [[ "$PORT" =~ ^[1-9][0-9]*$ ]] || [ "$PORT" -gt 65535 ]; then
  echo "record-promo: GADAK_E2E_PORT must be 1-65535, got ${PORT}" >&2
  exit 1
fi
if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "record-promo: port ${PORT} is already listening — pick another GADAK_E2E_PORT" >&2
  lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >&2 || true
  exit 1
fi
export GADAK_E2E_PORT="$PORT"
echo "record-promo: GADAK_E2E_PORT=${PORT} node=$(node -v)"

free_port() {
  local pids
  pids="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    echo "record-promo: freeing :${PORT} ($pids)"
    # shellcheck disable=SC2086
    kill $pids 2>/dev/null || true
    sleep 1
  fi
}

record_one() {
  local name="$1" config="$2" export_sh="$3" results="$4"
  rm -rf "$results"
  echo "record-promo: recording ${name}…"
  GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config "$config"
  bash "$export_sh"
  free_port
}

record_one tokens \
  e2e/demo/tokens.config.ts \
  e2e/demo/export-tokens.sh \
  e2e/.tmp/test-results-tokens

record_one dashboards \
  e2e/demo/dashboards.config.ts \
  e2e/demo/export-dashboards.sh \
  e2e/.tmp/test-results-dashboards

echo "record-promo: ffprobe"
for f in docs/media/tokens.mp4 docs/media/tokens.gif docs/media/dashboards.mp4 docs/media/dashboards.gif; do
  test -s "$f" || { echo "record-promo: missing $f" >&2; exit 1; }
  echo "── $f"
  ffprobe -v error -show_entries format=duration,size -show_entries stream=width,height,codec_name,r_frame_rate -of default=nw=1 "$f"
done
echo "record-promo: done"
