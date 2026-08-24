#!/usr/bin/env bash
# Unattended regen of docs/media/{tokens,dashboards}-vertical.mp4.
#
# Same-clock Playwright stack (paper terminal above the real app iframe) at
# 1080×1350. Commands are gadak's own against a frozen copy of
# examples/demo.db. No live site. No GIF.
#
# Usage: bash e2e/demo/record-vertical.sh
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

command -v ffmpeg >/dev/null || { echo "record-vertical: ffmpeg required" >&2; exit 1; }
command -v ffprobe >/dev/null || { echo "record-vertical: ffprobe required" >&2; exit 1; }
if [[ ! -x node_modules/.bin/playwright ]]; then
  echo "record-vertical: playwright missing (npm ci)" >&2
  exit 1
fi

# 7890: landscape promo is 7888, claude-drive is 7889.
PORT="${GADAK_E2E_PORT:-7890}"
if ! [[ "$PORT" =~ ^[1-9][0-9]*$ ]] || [ "$PORT" -gt 65535 ]; then
  echo "record-vertical: GADAK_E2E_PORT must be 1-65535, got ${PORT}" >&2
  exit 1
fi
if [[ "$PORT" == "7888" || "$PORT" == "7889" ]]; then
  echo "record-vertical: port ${PORT} is reserved (landscape promo 7888 / claude-drive 7889)" >&2
  exit 1
fi
if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "record-vertical: port ${PORT} is already listening — pick another GADAK_E2E_PORT" >&2
  lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >&2 || true
  exit 1
fi
export GADAK_E2E_PORT="$PORT"
export GADAK_PROMO_LAYOUT=vertical
echo "record-vertical: GADAK_E2E_PORT=${PORT} GADAK_PROMO_LAYOUT=vertical node=$(node -v)"

free_port() {
  local pids
  pids="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    echo "record-vertical: freeing :${PORT} ($pids)"
    # shellcheck disable=SC2086
    kill $pids 2>/dev/null || true
    sleep 1
  fi
}

record_one() {
  local name="$1" config="$2" results="$3"
  rm -rf "$results"
  echo "record-vertical: recording ${name}…"
  GADAK_MEDIA=1 GADAK_PROMO_LAYOUT=vertical GADAK_E2E_PORT="$PORT" \
    ./node_modules/.bin/playwright test --config "$config"
  bash e2e/demo/export-vertical.sh "$name"
  free_port
}

record_one tokens \
  e2e/demo/tokens-vertical.config.ts \
  e2e/.tmp/test-results-tokens-vertical

record_one dashboards \
  e2e/demo/dashboards-vertical.config.ts \
  e2e/.tmp/test-results-dashboards-vertical

echo "record-vertical: ffprobe"
for f in docs/media/tokens-vertical.mp4 docs/media/dashboards-vertical.mp4; do
  test -s "$f" || { echo "record-vertical: missing $f" >&2; exit 1; }
  echo "── $f"
  ffprobe -v error -show_entries format=duration,size -show_entries stream=width,height,codec_name,r_frame_rate -of default=nw=1 "$f"
done
echo "record-vertical: done"
