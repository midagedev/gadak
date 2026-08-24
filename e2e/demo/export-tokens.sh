#!/usr/bin/env bash
# Find the Playwright video from the tokens promo take and emit
# docs/media/tokens.gif + docs/media/tokens.mp4.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="$ROOT/docs/media"
RESULTS="$ROOT/e2e/.tmp/test-results-tokens"
mkdir -p "$OUT_DIR"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "${WEBM}" ]]; then
  echo "export-tokens: no video.webm under $RESULTS" >&2
  echo "  run: GADAK_MEDIA=1 GADAK_E2E_PORT=7888 ./node_modules/.bin/playwright test --config e2e/demo/tokens.config.ts" >&2
  exit 1
fi

TRIM="0"
if [[ -f "$ROOT/e2e/.tmp/promo-trim-tokens" ]]; then
  TRIM="$(tr -d '[:space:]' <"$ROOT/e2e/.tmp/promo-trim-tokens")"
fi

echo "export-tokens: source $WEBM trim=${TRIM}s"

ffmpeg -y -i "$WEBM" -ss "$TRIM" \
  -an \
  -c:v libx264 -pix_fmt yuv420p -preset medium -crf 23 \
  -movflags +faststart \
  "$OUT_DIR/tokens.mp4"

FPS=9
WIDTH=1280
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-tokens-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT

make_gif() {
  local fps="$1" width="$2" colors="${3:-128}"
  echo "export-tokens: palette 2-pass gif fps=${fps} width=${width} colors=${colors}" >&2
  # GIF from the already-trimmed mp4 — ffmpeg 8 + webm -ss can emit an empty palette.
  ffmpeg -y -i "$OUT_DIR/tokens.mp4" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -i "$OUT_DIR/tokens.mp4" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$OUT_DIR/tokens.gif"
}

make_gif "$FPS" "$WIDTH" 128

MAX_BYTES=$((8 * 1024 * 1024))
size_bytes() { wc -c <"$OUT_DIR/tokens.gif" | tr -d ' '; }

if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-tokens: gif $(size_bytes) bytes > 8MB, retrying at fps=8 width=1280 colors=96" >&2
  make_gif 8 1280 96
fi
if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-tokens: gif $(size_bytes) bytes > 8MB, retrying at fps=8 width=960 colors=64" >&2
  make_gif 8 960 64
fi

echo "export-tokens: wrote $OUT_DIR/tokens.gif ($(size_bytes) bytes)"
echo "export-tokens: wrote $OUT_DIR/tokens.mp4 ($(wc -c <"$OUT_DIR/tokens.mp4" | tr -d ' ') bytes)"
