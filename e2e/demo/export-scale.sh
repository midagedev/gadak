#!/usr/bin/env bash
# Find the Playwright video from the scale take and emit
# docs/media/scale.gif + docs/media/scale.mp4.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="$ROOT/docs/media"
RESULTS="$ROOT/e2e/demo/test-results-scale"
mkdir -p "$OUT_DIR"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "${WEBM}" ]]; then
  echo "export-scale: no video.webm under $RESULTS" >&2
  echo "  run: make media-scale" >&2
  exit 1
fi

echo "export-scale: source $WEBM"

ffmpeg -y -i "$WEBM" \
  -an \
  -c:v libx264 -pix_fmt yuv420p -preset medium -crf 23 \
  -movflags +faststart \
  "$OUT_DIR/scale.mp4"

# Same budget ladder as export-groupby.sh: fps/colors before width.
# Ceiling is 4 MB (site hero slot).
FPS=9
WIDTH=960
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-scale-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT

make_gif() {
  local fps="$1" width="$2" colors="${3:-128}"
  echo "export-scale: palette 2-pass gif fps=${fps} width=${width} colors=${colors}" >&2
  ffmpeg -y -i "$WEBM" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -i "$WEBM" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$OUT_DIR/scale.gif"
}

make_gif "$FPS" "$WIDTH" 128
SIZE="$(stat -f %z "$OUT_DIR/scale.gif")"
if [[ "$SIZE" -gt 4194304 ]]; then
  make_gif 8 "$WIDTH" 96
fi
SIZE="$(stat -f %z "$OUT_DIR/scale.gif")"
if [[ "$SIZE" -gt 4194304 ]]; then
  make_gif 7 800 96
fi

ls -lh "$OUT_DIR/scale.gif" "$OUT_DIR/scale.mp4"
