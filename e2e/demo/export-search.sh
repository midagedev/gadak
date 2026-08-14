#!/usr/bin/env bash
# Find the Playwright video from the unified-search take and emit
# docs/media/search.gif + docs/media/search.mp4.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="$ROOT/docs/media"
RESULTS="$ROOT/e2e/demo/test-results-search"
mkdir -p "$OUT_DIR"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "${WEBM}" ]]; then
  echo "export-search: no video.webm under $RESULTS" >&2
  echo "  run: GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/search.config.ts" >&2
  exit 1
fi

echo "export-search: source $WEBM"

ffmpeg -y -i "$WEBM" \
  -an \
  -c:v libx264 -pix_fmt yuv420p -preset medium -crf 23 \
  -movflags +faststart \
  "$OUT_DIR/search.mp4"

# Same budget ladder as export-video.sh (web-demo): fps/colors before width.
FPS=9
WIDTH=960
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-search-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT

make_gif() {
  local fps="$1" width="$2" colors="${3:-128}"
  echo "export-search: palette 2-pass gif fps=${fps} width=${width} colors=${colors}" >&2
  ffmpeg -y -i "$WEBM" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -i "$WEBM" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$OUT_DIR/search.gif"
}

make_gif "$FPS" "$WIDTH" 128

MAX_BYTES=$((8 * 1024 * 1024))
size_bytes() { wc -c <"$OUT_DIR/search.gif" | tr -d ' '; }

if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-search: gif $(size_bytes) bytes > 8MB, retrying at fps=8 width=960 colors=96" >&2
  make_gif 8 960 96
fi
if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-search: gif $(size_bytes) bytes > 8MB, retrying at fps=8 width=900 colors=64" >&2
  make_gif 8 900 64
fi

echo "export-search: wrote $OUT_DIR/search.gif ($(size_bytes) bytes)"
echo "export-search: wrote $OUT_DIR/search.mp4 ($(wc -c <"$OUT_DIR/search.mp4" | tr -d ' ') bytes)"
