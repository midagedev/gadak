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
# this clip exists to show together.
#
# ── Pacing ──────────────────────────────────────────────────────────────
# The take runs 204s and that is real work, not dead air: measured with
# mpdecimate over the frame minus the spinner band (crop=1440:930:0:0), only
# 471 of 6110 frames are "changed", but the still runs between them are short
# enough that capping every hold at one second still leaves 180s. There is
# nothing to delete. What there is, is one very long stretch of the agent
# working — 40s to 196s, 76% of the clip — while the two payoffs either side
# of it take seconds.
#
# So the ramp is by beat, not by frame: everything a viewer has to read runs
# at 1x, and the two working stretches are a time-lapse. This does put Claude's
# own elapsed-time counter out of step with the clip, which is why the earlier
# cut refused to speed anything up. It is the right trade at this length — a
# ticking counter inside a visibly fast-forwarded spinner reads as time-lapse,
# where a 3m24s hero clip reads as nothing at all, because it is not watched.
#
# Beats are measured per take, never guessed — a live model does not repeat
# them. Method: scene detection on the list column
# (`crop=580:1080:860:0,select='gt(scene,0.04)'`) gives the two payoffs (this
# take: 32.3s the list becomes the answer, 199.8s the wall opens), and the
# agent's own elapsed counter in-frame dates the start of each working stretch.
# Aim a boundary wrong and the cost is bounded — a second of spinner at the
# wrong rate — which is why a ramp is safe here where the zoom it replaced
# was not: that one could hide a payoff outright.
B1_END=13      # setup: list at rest, palette, pane, `claude` boots
B2_END=31      # working on prompt 1                       → sped
B3_END=40      # the list becomes the answer; prompt 2 typed
B4_END=196     # authoring the dashboard                   → sped
FAST_A=3.0
FAST_B=4.5

ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" \
  -an \
  -filter_complex "\
[0:v]fps=30,format=yuv420p,split=5[a][b][c][d][e]; \
[a]trim=0:${B1_END},setpts=PTS-STARTPTS[s1]; \
[b]trim=${B1_END}:${B2_END},setpts=(PTS-STARTPTS)/${FAST_A}[s2]; \
[c]trim=${B2_END}:${B3_END},setpts=PTS-STARTPTS[s3]; \
[d]trim=${B3_END}:${B4_END},setpts=(PTS-STARTPTS)/${FAST_B}[s4]; \
[e]trim=start=${B4_END},setpts=PTS-STARTPTS[s5]; \
[s1][s2][s3][s4][s5]concat=n=5:v=1:a=0,fps=30[v]" \
  -map "[v]" \
  -c:v libx264 -profile:v high -level 4.0 -preset slow -crf 21 \
  -movflags +faststart \
  "$OUT_DIR/terminal-hero.mp4"

ffprobe -v error -select_streams v:0 \
  -show_entries stream=width,height,r_frame_rate,pix_fmt \
  -show_entries format=duration,size \
  -of default=noprint_wrappers=1 "$OUT_DIR/terminal-hero.mp4"
ls -lh "$OUT_DIR/terminal-hero.mp4"
