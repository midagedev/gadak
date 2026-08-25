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

# Twitter's player: H.264 High, yuv420p, even dimensions. 1080x1350 (4:5) is
# the vertical social frame the tokens/dashboards cuts already use — the
# timeline gives a 4:5 clip the most height it will give anything.
#
# The geometry check is a gate, not a resize: Playwright letterboxes when the
# viewport and the video size disagree, and a letterboxed source scaled here
# would ship as a clip with grey bars baked in.
probe_dim() {
  ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0:s=x "$1"
}
SRC_DIM="$(probe_dim "$WEBM")"
if [[ "$SRC_DIM" != "1080x1350" ]]; then
  echo "export-terminal: source ${SRC_DIM}, want 1080x1350" >&2
  exit 1
fi

# Camera work, the same smoothstep zoompan the site hero uses
# (export-scale.sh, GDK-751): push in on the terminal column while the agent
# is working, pull back to the whole window for each payoff. The source
# pacing is untouched — only the crop moves. Nothing is sped up, deliberately:
# Claude's own elapsed-time counter is in frame, and a time-lapse would put
# the clip and the clock on screen in disagreement.
#
# Frame geometry, measured off the take rather than off the config: the
# sidebar ends at 272 and the terminal/list boundary is at 700. The pane does
# not get the width the spec asks for — the split is clamped so the list keeps
# its 390px minimum — so the terminal is ~428 wide, not the 640 in
# localStorage. At z=1.45 the visible window is 745x931 at (272,419): the
# terminal from its own left edge — no half-cropped sidebar counters — plus
# most of the list, so the board is still beside the shell while zoomed. The
# window sits on the *bottom* of the frame because that is where a terminal
# lives: newest output and the input box. The rail with the Beta mark is
# already established at full frame in beat 2. zoompan's x/y are in *input*
# coordinates (the first cut of this put them in zoomed ones and framed the
# list instead).
#
# Beat times were measured on the take, not guessed — scene detection over a
# crop of the list column (`select='gt(scene,0.03)'`) puts the list change at
# 28.8s and the dashboard at 82.8s on the trimmed clip. Each pull-back starts
# ~2s before its payoff so the frame is already wide when it lands. Re-measure
# the same way if the model's pacing changes; a live take is never identical
# twice, and a zoom aimed at the wrong second is worse than none.
Z_IN=1.45
Z_X=272
Z_Y=419
ease() { # start, duration → smoothstep in [0,1]
  local p="clip((in/30-$1)/$2,0,1)"
  printf '(%s*%s*(3-2*%s))' "$p" "$p" "$p"
}
EA="$(ease 9.0 1.0)"   # pane is open, agent starts working
EB="$(ease 26.5 1.2)"  # out, ahead of the list becoming the answer
EC="$(ease 36.0 1.2)"  # in, for the dashboard authoring stretch
ED="$(ease 80.5 1.2)"  # out, ahead of the wall opening
ENV="(${EA}-${EB}+${EC}-${ED})"

ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" \
  -an \
  -vf "fps=30,zoompan=z='1+0.45*${ENV}':x='${Z_X}*${ENV}':y='${Z_Y}*${ENV}':d=1:s=1080x1350:fps=30,format=yuv420p" \
  -c:v libx264 -profile:v high -level 4.0 -preset slow -crf 21 \
  -movflags +faststart \
  "$OUT_DIR/terminal-hero.mp4"

ffprobe -v error -select_streams v:0 \
  -show_entries stream=width,height,r_frame_rate,pix_fmt \
  -show_entries format=duration,size \
  -of default=noprint_wrappers=1 "$OUT_DIR/terminal-hero.mp4"
ls -lh "$OUT_DIR/terminal-hero.mp4"
