#!/usr/bin/env bash
# Find the Playwright video from the last demo run and emit optimized
# docs/media/web-demo.gif + docs/media/web-demo.mp4.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="$ROOT/docs/media"
RESULTS="$ROOT/e2e/demo/test-results"
mkdir -p "$OUT_DIR"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "${WEBM}" ]]; then
  echo "export-video: no video.webm under $RESULTS" >&2
  echo "  run: SCRY_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/playwright.config.ts" >&2
  exit 1
fi

echo "export-video: source $WEBM"

# ── MP4 (Twitter / social — h264, yuv420p, faststart) ──────────────────────
ffmpeg -y -i "$WEBM" \
  -an \
  -c:v libx264 -pix_fmt yuv420p -preset medium -crf 23 \
  -movflags +faststart \
  "$OUT_DIR/web-demo.mp4"

# ── GIF (README hero) — palette 2-pass, size-budgeted ─────────────────────
# Targets: width ~1000px, 12 fps, under 8 MB (prefer ≤5 MB).
# Tradeoffs are documented in docs/MEDIA.md if we have to step down further.
# Dense UI (sidebar + list + detail) needs a conservative start size to stay
# under the 8 MB README budget. Prefer ≤5 MB when the walkthrough is short.
# Tradeoffs: see docs/MEDIA.md.
FPS=10
WIDTH=800
# macOS mktemp requires the X's at the end of the template (before any suffix).
PALETTE="$(mktemp "${TMPDIR:-/tmp}/scry-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT

make_gif() {
  local fps="$1" width="$2" colors="${3:-128}"
  echo "export-video: palette 2-pass gif fps=${fps} width=${width} colors=${colors}" >&2
  ffmpeg -y -i "$WEBM" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -i "$WEBM" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$OUT_DIR/web-demo.gif"
}

make_gif "$FPS" "$WIDTH" 128

# If still over 8 MB, step down further.
MAX_BYTES=$((8 * 1024 * 1024))
size_bytes() { wc -c <"$OUT_DIR/web-demo.gif" | tr -d ' '; }

if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-video: gif $(size_bytes) bytes > 8MB, retrying at fps=10 width=720 colors=96" >&2
  make_gif 10 720 96
fi
if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-video: gif $(size_bytes) bytes > 8MB, retrying at fps=8 width=640 colors=64" >&2
  make_gif 8 640 64
fi



echo "export-video: wrote $OUT_DIR/web-demo.gif ($(size_bytes) bytes)"
echo "export-video: wrote $OUT_DIR/web-demo.mp4 ($(wc -c <"$OUT_DIR/web-demo.mp4" | tr -d ' ') bytes)"
