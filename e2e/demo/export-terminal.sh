#!/usr/bin/env bash
# Find the Playwright video from the terminal take and emit one Twitter-ready
# mp4 in docs/media (GADAK_TERMINAL_OUT redirects a scratch take).
#
# Two deliberate differences from every other export script here:
#
#   mp4 first.    The clip is a Twitter post as much as a README block, and
#                 Twitter transcodes an uploaded gif to mp4 anyway; the gif
#                 (palette two-pass, like export-agent.sh) exists because
#                 GitHub renders no video in a README.
#   docs/media.   0.18 shipped the pane Beta and kept these bytes in scratch/;
#                 0.20 puts the pane on the README and the landing, so the
#                 clips land where site/public/media (a symlink onto
#                 docs/media) serves them. GADAK_TERMINAL_OUT still redirects
#                 a scratch take.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="${GADAK_TERMINAL_OUT:-$ROOT/docs/media}"
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
# Two takes share this exporter and the shape says which one arrived: the
# live-Claude hero is the 16:10 window (terminal-claude.config.ts), the
# scripted pipe-and-JQL take is the 4:5 social cut (terminal.config.ts).
# Each ships under its own name so one cannot overwrite the other.
case "$SRC_DIM" in
  1440x900)  OUT_NAME="terminal-hero.mp4" ;;
  1080x1350) OUT_NAME="terminal-demo.mp4" ;;
  *)
    echo "export-terminal: source ${SRC_DIM}, want 1440x900 (claude take) or 1080x1350 (scripted take)" >&2
    exit 1
    ;;
esac

# No camera work. The 4:5 cut needed a zoom because a portrait crop of a
# three-column app leaves the terminal too small to read, and the zoom was
# paying for the frame's mistake. At full width the whole window is legible at
# rest, and pushing in then only takes the board away from the shell — the one
# thing this clip exists to show together.
#
# ── Pacing ──────────────────────────────────────────────────────────────
# A long take is long because of real work, not dead air — but to a viewer
# the agent working *is* dead air: the board does not move, nobody types,
# the transcript grows a line a second under a spinner. What a clip owes
# the viewer is the payoffs either side of that at 1x.
#
# dense-cut.py measures where those stretches are on *this* take (a live
# model does not repeat its pacing, so a hand-tuned beat table is wrong the
# moment the next take lands) and prints a filter that keeps the head of
# each at 1x and time-lapses the rest. Two kinds: a still stretch (nothing
# changes anywhere) and a working stretch (only the transcript changes).
# Payoffs, typing and the answer text ship whole. Claude's own elapsed
# counter goes out of step with the clip across the compressed parts; that
# trade was taken knowingly (the beat-table ramp this replaces made it too).
#
# The regions are this config's geometry, so they live here, per shape:
# the 16:10 hero has the dock at 340px under a 560px board, a 272px roster,
# the TUI's input row two rows above the dock's floor and the spinner row
# two rows above that. The 4:5 scripted take has no agent and no spinner,
# so it gets the still rule over the whole frame — its reading holds are
# under the floor and pass through unchanged.
case "$SRC_DIM" in
  1440x900) REGIONS=(--board 272:0:1168:560 --input 272:845:1168:32 --work 272:560:1168:240) ;;
  *)        REGIONS=() ;;
esac
PLAN="$(python3 "$ROOT/e2e/demo/dense-cut.py" "$WEBM" --trim-head "$TRIM_HEAD" ${REGIONS[@]+"${REGIONS[@]}"})"
FILTER="$(printf '%s\n' "$PLAN" | sed -n 1p)"
OUT_LEN="$(printf '%s\n' "$PLAN" | sed -n 2p)"
echo "export-terminal: dense cut → ${OUT_LEN}s"

ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" \
  -an \
  -filter_complex "$FILTER" \
  -map "[v]" \
  -c:v libx264 -profile:v high -level 4.0 -preset slow -crf 21 \
  -movflags +faststart \
  "$OUT_DIR/$OUT_NAME"

ffprobe -v error -select_streams v:0 \
  -show_entries stream=width,height,r_frame_rate,pix_fmt \
  -show_entries format=duration,size \
  -of default=noprint_wrappers=1 "$OUT_DIR/$OUT_NAME"
ls -lh "$OUT_DIR/$OUT_NAME"

# Poster: the first settled frame of the cut, at the mp4's own size — the
# landing shows it before anyone presses play (same rule as every other
# poster since ec39ea3a).
STEM="${OUT_NAME%.mp4}"
ffmpeg -y -v error -ss 0.2 -i "$OUT_DIR/$OUT_NAME" -frames:v 1 "$OUT_DIR/${STEM}-poster.png"

# GIF for the README, from the *cut* mp4 so it carries the same pacing.
# Width is the README's render width at 2x: the hero sits at 900 (→ 1200 is
# plenty), the 4:5 cut at 430 (→ 860). Palette two-pass; the >8MB ladder is
# export-agent.sh's.
case "$SRC_DIM" in
  1440x900) GIF_WIDTH=1200 ;;
  *)        GIF_WIDTH=860 ;;
esac
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-terminal-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT
make_gif() {
  local fps="$1" width="$2" colors="$3"
  echo "export-terminal: gif fps=${fps} width=${width} colors=${colors}" >&2
  ffmpeg -y -v error -i "$OUT_DIR/$OUT_NAME" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -v error -i "$OUT_DIR/$OUT_NAME" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$OUT_DIR/${STEM}.gif"
}
MAX_BYTES=$((8 * 1024 * 1024))
gif_bytes() { wc -c <"$OUT_DIR/${STEM}.gif" | tr -d ' '; }
make_gif 9 "$GIF_WIDTH" 128
if (( $(gif_bytes) > MAX_BYTES )); then make_gif 8 "$GIF_WIDTH" 96; fi
if (( $(gif_bytes) > MAX_BYTES )); then make_gif 8 $(( GIF_WIDTH * 3 / 4 )) 64; fi
echo "export-terminal: wrote ${STEM}.gif ($(gif_bytes) bytes), ${STEM}-poster.png"
