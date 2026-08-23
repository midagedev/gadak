#!/usr/bin/env bash
# Find the Playwright video from the group-by take and emit
# docs/media/groupby.gif + docs/media/groupby.mp4.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="$ROOT/docs/media"
RESULTS="$ROOT/e2e/demo/test-results-groupby"
mkdir -p "$OUT_DIR"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "${WEBM}" ]]; then
  echo "export-groupby: no video.webm under $RESULTS" >&2
  echo "  run: GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/groupby.config.ts" >&2
  exit 1
fi

echo "export-groupby: source $WEBM"

ffmpeg -y -i "$WEBM" \
  -an \
  -c:v libx264 -pix_fmt yuv420p -preset medium -crf 23 \
  -movflags +faststart \
  "$OUT_DIR/groupby.mp4"

# Same budget ladder as export-search.sh: fps/colors before width.
# Ceiling is 4 MB (C1 contract); search's ladder is 8 MB for a longer cut.
FPS=9
WIDTH=960
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-groupby-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT

make_gif() {
  local fps="$1" width="$2" colors="${3:-128}"
  echo "export-groupby: palette 2-pass gif fps=${fps} width=${width} colors=${colors}" >&2
  ffmpeg -y -i "$WEBM" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -i "$WEBM" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$OUT_DIR/groupby.gif"
}

make_gif "$FPS" "$WIDTH" 128

# Decimal MB (bytes/1e6), same unit MEDIA.md uses for the sibling clips.
MAX_BYTES=$((4 * 1000 * 1000))
size_bytes() { wc -c <"$OUT_DIR/groupby.gif" | tr -d ' '; }

if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-groupby: gif $(size_bytes) bytes > 4MB, retrying at fps=8 width=960 colors=96" >&2
  make_gif 8 960 96
fi
if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-groupby: gif $(size_bytes) bytes > 4MB, retrying at fps=8 width=900 colors=64" >&2
  make_gif 8 900 64
fi
if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-groupby: gif $(size_bytes) bytes > 4MB, retrying at fps=7 width=800 colors=64" >&2
  make_gif 7 800 64
fi

echo "export-groupby: wrote $OUT_DIR/groupby.gif ($(size_bytes) bytes)"
echo "export-groupby: wrote $OUT_DIR/groupby.mp4 ($(wc -c <"$OUT_DIR/groupby.mp4" | tr -d ' ') bytes)"
