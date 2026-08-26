#!/usr/bin/env bash
# Find the Playwright video from the terminal take and emit one Twitter-ready
# mp4 at scratch/terminal-hero.mp4.
#
# Two deliberate differences from every other export script here:
#
#   No gif.       This clip is for a Twitter post, not a README block. Twitter
#                 transcodes an uploaded gif to mp4 anyway, and a 4 MB palette
#                 gif would only throw away the typing.
#   Not docs/media. site/public/media is a symlink onto docs/media, so anything
#                 landing there is served by the website — reachable even with
#                 no page linking it. The terminal is not announced on the site
#                 or in the README yet (0.18 ships it Beta), so the bytes stay
#                 in gitignored scratch/ and the harness stays committed. When
#                 the pane goes public, point OUT_DIR at docs/media and
#                 re-record; nothing else here changes.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="${GADAK_TERMINAL_OUT:-$ROOT/scratch}"
RESULTS="${GADAK_TERMINAL_RESULTS:-$ROOT/e2e/demo/test-results-terminal}"
mkdir -p "$OUT_DIR"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "${WEBM}" ]]; then
  echo "export-terminal: no video.webm under $RESULTS" >&2
  echo "  run: bash e2e/demo/record-terminal-claude.sh (or make media-terminal)" >&2
  exit 1
fi

echo "export-terminal: source $WEBM"

# Trim the boot skeleton: recording starts at page load, the clip should
# open on the settled list. The spec's first beat waits for the scroller,
# so the head is skeleton + settle. Re-measure if boot pacing changes.
TRIM_HEAD=2.2

# Twitter's player: H.264 High, yuv420p, even dimensions. 1440x1080 (4:3).
# The first cut was 4:5, the frame the single-column tokens/dashboards clips
# use, and it was the wrong frame for this subject: a shell and the board it
# drives sit side by side, and a portrait crop squeezes a three-column app
# until neither half reads. 4:3 still takes far more of a phone timeline than
# 16:9 and lets the terminal have its full width beside a list that is still
# a list.
#
# The geometry check is a gate, not a resize: Playwright letterboxes when the
# viewport and the video size disagree, and a letterboxed source scaled here
# would ship as a clip with grey bars baked in.
probe_dim() {
  ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0:s=x "$1"
}
SRC_DIM="$(probe_dim "$WEBM")"
if [[ "$SRC_DIM" != "1440x1080" ]]; then
  echo "export-terminal: source ${SRC_DIM}, want 1440x1080" >&2
  exit 1
fi

# No camera work. The 4:5 cut needed a zoom because a portrait crop of a
# three-column app leaves the terminal too small to read, and the zoom was
# paying for the frame's mistake. At 4:3 the whole window is legible at rest,
# and pushing in then only takes the board away from the shell — the one thing
# this clip exists to show together. A moving crop also has to be aimed at
# beats a live model does not repeat between takes, so every re-record earned
# a re-measure or shipped a zoom pointed at the wrong second.
#
# Nothing is sped up either, and that one is not a preference: Claude's own
# elapsed-time counter is in frame, so a time-lapse would put the clip and the
# clock on screen in disagreement.
ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" \
  -an \
  -vf "fps=30,format=yuv420p" \
  -c:v libx264 -profile:v high -level 4.0 -preset slow -crf 21 \
  -movflags +faststart \
  "$OUT_DIR/terminal-hero.mp4"

ffprobe -v error -select_streams v:0 \
  -show_entries stream=width,height,r_frame_rate,pix_fmt \
  -show_entries format=duration,size \
  -of default=noprint_wrappers=1 "$OUT_DIR/terminal-hero.mp4"
ls -lh "$OUT_DIR/terminal-hero.mp4"
