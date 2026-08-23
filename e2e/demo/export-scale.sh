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

# Trim the boot skeleton: the recording starts at page load, but the clip
# should open on the settled list (the "20,000 issues" count already up),
# not on gray placeholders. The spec's first beat waits for the count, so
# 2.4s of head is skeleton + settle; the trim lands just after settle.
# Measured on the take of 2026-08-23; re-measure if boot pacing changes.
TRIM_HEAD=2.4

# GDK-751 (2026-08-24): post-process camera work on the mp4 — smoothstep
# push-in to the palette during the typing beats, back out for the regroup,
# push-in to the counts band (chips + breakdown) for the narrowing beats,
# and back to full frame before the loop point. Times are seconds on the
# trimmed clip; the beat map was measured on the 2026-08-23 take — re-map
# if the spec's pacing changes. Source pacing is untouched (the rejected
# approach re-timed the recording; this one only moves the crop). Holds are
# constant expressions, so they are mathematically static — measured
# consecutive-frame meandiff ≈0.005 in holds, monotonic ≈13→11 across a
# transition tail (no oscillation). The gif below stays full-frame.
ZT='(in/25)'
ZEA="(clip((${ZT}-1.2)/0.8,0,1)*clip((${ZT}-1.2)/0.8,0,1)*(3-2*clip((${ZT}-1.2)/0.8,0,1)))"
ZEB="(clip((${ZT}-5.5)/0.8,0,1)*clip((${ZT}-5.5)/0.8,0,1)*(3-2*clip((${ZT}-5.5)/0.8,0,1)))"
ZEC="(clip((${ZT}-8.6)/0.8,0,1)*clip((${ZT}-8.6)/0.8,0,1)*(3-2*clip((${ZT}-8.6)/0.8,0,1)))"
ZED="(clip((${ZT}-15.9)/0.8,0,1)*clip((${ZT}-15.9)/0.8,0,1)*(3-2*clip((${ZT}-15.9)/0.8,0,1)))"
ZOOM="1+0.35021*${ZEA}-0.35021*${ZEB}+0.28*${ZEC}-0.28*${ZED}"
ZX="300*${ZEA}-300*${ZEB}+280*${ZEC}-280*${ZED}"
ZY="48*${ZEA}-48*${ZEB}"

ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" \
  -an \
  -vf "zoompan=z='${ZOOM}':x='${ZX}':y='${ZY}':d=1:s=1280x800:fps=25,format=yuv420p" \
  -c:v libx264 -preset medium -crf 20 \
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
  ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$OUT_DIR/scale.gif"
}

# Same budget ladder as export-groupby.sh: fps/colors before width.
# Ceiling is 4 MB (site hero slot).
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
