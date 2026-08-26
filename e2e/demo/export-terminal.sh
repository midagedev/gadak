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

# Twitter's player: H.264 High, yuv420p, even dimensions. 1440x900 (16:10) —
# a window shape, which is what this clip is of. It walked down to that: 4:5
# first, borrowed from the single-column tokens/dashboards clips and wrong for
# a subject that is two columns side by side; then 4:3, which fixed the width
# and left more height than the content fills.
#
# The geometry check is a gate, not a resize: Playwright letterboxes when the
# viewport and the video size disagree, and a letterboxed source scaled here
# would ship as a clip with grey bars baked in.
probe_dim() {
  ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0:s=x "$1"
}
SRC_DIM="$(probe_dim "$WEBM")"
if [[ "$SRC_DIM" != "1440x900" ]]; then
  echo "export-terminal: source ${SRC_DIM}, want 1440x900" >&2
  exit 1
fi

# No camera work. The 4:5 cut needed a zoom because a portrait crop of a
# three-column app leaves the terminal too small to read, and the zoom was
# paying for the frame's mistake. At full width the whole window is legible at
# rest, and pushing in then only takes the board away from the shell — the one
# thing this clip exists to show together.
#
# ── Pacing ──────────────────────────────────────────────────────────────
# A long take is long because of real work, not dead air. The obvious move —
# drop the static frames — was measured, and there are none to drop: with
# mpdecimate over the frame minus the spinner band (crop=in_w:in_h*0.86:0:0)
# only 471 of 6110 frames carried a change, but the still runs between them
# were short enough that capping every hold at one second still left 180s of
# 204s. The terminal is always scrolling.
#
# What a long take has instead is one very long stretch of the agent working,
# with both payoffs in seconds either side of it. So the ramp is by beat:
# everything a viewer has to read runs at 1x, and the working stretches become
# a time-lapse.
#
# That trade is real — it puts Claude's own elapsed counter out of step with
# the clip — which is why the ramp is *conditional*, not the default. Under
# RAMP_ABOVE seconds the take ships whole, at 1x, with the clock and the clip
# agreeing. Takes vary enormously: 204s while the skill was still sending the
# agent hunting for an example file that was never installed beside it, 47s
# once that example moved into the skill itself. The short take is the one to
# want; the ramp is the net under it.
RAMP_ABOVE=75

# Beats are *measured off this take*, never carried over. A live model does
# not repeat its pacing, so a hand-tuned boundary table is wrong the moment
# the next take lands. The script finds them instead.
#
# The two payoffs both change the right-hand column and nothing else does:
# the list becomes the answer, and later the wall replaces the list. Scene
# detection over that column alone reports exactly those. First one after the
# boot is payoff A, last is payoff B.
scene_times() {
  ffmpeg -nostdin -ss "$TRIM_HEAD" -i "$1" \
    -vf "crop=in_w*0.4:in_h:in_w*0.6:0,select='gt(scene,0.04)',metadata=print" \
    -an -f null - 2>&1 | sed -n 's/.*pts_time:\([0-9.]*\).*/\1/p'
}

SRC_LEN="$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$WEBM")"
USABLE="$(python3 -c "print(round($SRC_LEN - $TRIM_HEAD, 1))")"

if python3 -c "import sys; sys.exit(0 if $USABLE > $RAMP_ABOVE else 1)"; then
  # No mapfile: macOS ships bash 3.2 and this script has to run there.
  SCENES=()
  while IFS= read -r t; do SCENES+=("$t"); done < <(scene_times "$WEBM" | awk '$1 > 8')
  if (( ${#SCENES[@]} < 2 )); then
    echo "export-terminal: found ${#SCENES[@]} payoff scenes, want 2 — re-measure by hand" >&2
    exit 1
  fi
  PAYOFF_A="${SCENES[0]}"
  PAYOFF_B="${SCENES[$(( ${#SCENES[@]} - 1 ))]}"
  echo "export-terminal: ${USABLE}s of take — ramping; payoffs at ${PAYOFF_A}s (list), ${PAYOFF_B}s (wall)"

  # 1x head (list settle, palette, pane attach, `claude` boot — driven by the
  # spec's own waits, not the model), time-lapse to just before the list turns
  # over, 1x for the answer and the second prompt, time-lapse across the
  # authoring stretch, 1x for the wall to the end. Aim one of these a second
  # wrong and the cost is a second of spinner at the wrong rate — which is why
  # a ramp is safe where the zoom it replaced was not: that one could miss a
  # payoff outright.
  B1_END=13
  B2_END="$(python3 -c "print(max(14, $PAYOFF_A - 1.5))")"
  B3_END="$(python3 -c "print($PAYOFF_A + 9)")"
  B4_END="$(python3 -c "print(max($PAYOFF_A + 10, $PAYOFF_B - 3.5))")"
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
else
  echo "export-terminal: ${USABLE}s of take — shipping it whole, at 1x"
  ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" \
    -an \
    -vf "fps=30,format=yuv420p" \
    -c:v libx264 -profile:v high -level 4.0 -preset slow -crf 21 \
    -movflags +faststart \
    "$OUT_DIR/terminal-hero.mp4"
fi

ffprobe -v error -select_streams v:0 \
  -show_entries stream=width,height,r_frame_rate,pix_fmt \
  -show_entries format=duration,size \
  -of default=noprint_wrappers=1 "$OUT_DIR/terminal-hero.mp4"
ls -lh "$OUT_DIR/terminal-hero.mp4"
