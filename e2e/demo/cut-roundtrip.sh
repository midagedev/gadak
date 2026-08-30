#!/usr/bin/env bash
# The round-trip cut (GDK-1159) → scratch/roundtrip/roundtrip.mp4 (+ -poster.png)
#
# Input is one file — scratch/roundtrip/take-dark.webm, from
# record-roundtrip.sh --dark — plus that take's proof marks. Everything the
# film argues happened inside that single recording, so there is no continuity
# to fake and no second camera to synchronise.
#
# Why dark. Both schemes were shot (a retake is ~2 minutes here, which is the
# point of a rig with no live model in it) and compared frame to frame. Dark
# ships for one reason that survives on a phone: the clip autoplays muted in a
# feed that is usually white, and the dark frame is the only one of the two
# that reads as a *window* rather than as more page. gadak's light theme is
# warm paper — distinctive in the app, and at thumbnail size in a light feed
# its edges dissolve into the timeline. Light is one flag away.
#
# ── Boundaries are marks, not seconds ──────────────────────────────────────
# Earlier versions hardcoded seconds into the take. That broke twice, because
# the offset between the spec's wall clock and the recorder's clock MOVES
# between takes — measured -0.3s, +0.5s, +0.02s on three consecutive takes —
# and a cut list a second early ends the film before the card lands (it did).
#
# So every boundary below is `<mark> <offset>`, and the script finds the
# offset between the two clocks itself: the pane opening drops the average
# luma of the pane region off a cliff (38 → 28 on a dark take), and that
# cliff IS the `pane_open` mark. One measurable event, no eyeballing.
# GADAK_RT_LEAD overrides it when a take needs a hand.
#
# Confirm the result on the sheet anyway — it is one flag:
#   bash e2e/demo/cut-roundtrip.sh --sheet
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
SCRATCH="$ROOT/scratch/roundtrip"

TAKE="${GADAK_RT_TAKE:-$SCRATCH/take-dark.webm}"
PROOF="${GADAK_RT_PROOF_FILE:-$ROOT/e2e/.tmp/roundtrip/proof-dark.jsonl}"
OUT="${GADAK_RT_OUT:-$SCRATCH/roundtrip.mp4}"

command -v ffmpeg >/dev/null || { echo "cut-roundtrip: ffmpeg required" >&2; exit 1; }
[[ -f "$TAKE" ]] || { echo "cut-roundtrip: no take at $TAKE" >&2; exit 1; }
[[ -f "$PROOF" ]] || { echo "cut-roundtrip: no proof marks at $PROOF" >&2; exit 1; }

W=1920 H=1296 FPS=30
BG=0x0e0d0c   # the take's own panel black, so any pillarbox is invisible

# ── Clock alignment ────────────────────────────────────────────────────────
LEAD="${GADAK_RT_LEAD:-$(python3 "$ROOT/e2e/demo/rt-marks.py" lead "$TAKE" "$PROOF")}"
echo "cut-roundtrip: clock lead = ${LEAD}s (pane-open cliff vs the pane_open mark)"

# Video seconds for a mark, plus an offset.
at() { python3 "$ROOT/e2e/demo/rt-marks.py" at "$PROOF" "$1" "${2:-0}" "$LEAD"; }

# ── The cut list, as segments ──────────────────────────────────────────────
# Each segment is independent: SEGMENTS is the running order, so re-shooting
# one beat means rebuilding one pair of bounds. The climax is deliberately its
# own segment — when the kanban board lands (GDK-761) it is filmed against the
# board and swapped in here, and nothing else in this file changes.
#
# X: the opening AND the chaos, in one unbroken take — the list-to-board
#    transition only argues anything if no cut sits between the click and the
#    columns. It runs from just before the list hold, through the toggle, into
#    the chaos hold (four shells, four issue keys, four cards wearing a shell
#    edge, one scrollback visible: work was left in all of them and nobody
#    remembers which) and straight on to the first gesture, so X and Y share no
#    frames.
X_IN="$(at list_hold -0.2)"     X_OUT="$(at a_enter -0.4)"
# Y: recovery A, from the board card. In at `a_enter` — NOT at a_replay, which
#    fires after the session is already selected: the hover and the glyph are
#    the reason this cut exists, and anchoring on the replay put them off
#    camera. Cause (card) and effect (dock) are one frame apart, vertically.
Y_IN="$(at a_enter -0.4)"       Y_OUT="$(at a_alive 0)"
# Z: recovery B, from ⌘K. Same rule: in on the gesture, not on its result. Two
#    in a row by two different doors is what makes it a system rather than a
#    trick.
# a_alive and b_enter are the same instant (the read-hold ends and the next
# gesture begins), so Y and Z join with no seam and no duplicated frames.
Z_IN="$(at b_enter 0)"          Z_OUT="$(at end_frame 1.4)"

SEGMENTS="X Y Z"

seg() {
  echo "[0:v]trim=${1}:${2},setpts=PTS-STARTPTS,fps=${FPS},scale=${W}:${H}," \
       "pad=${W}:${H}:(ow-iw)/2:(oh-ih)/2:color=${BG},format=yuv420p,setsar=1[${3}];"
}

# ── The end card ───────────────────────────────────────────────────────────
# 26 seconds of the last hero went by without the word gadak at a readable
# size and without a URL (release-video.md: "Say the name"). It is a PNG
# rendered by Playwright, not ffmpeg text: the ffmpeg here is built without
# libfreetype, so `drawtext` is not a filter it has — measured, the first
# assembly died on "No such filter: 'drawtext'".
END_SECS=2.8
END_PNG="$SCRATCH/endcard.png"
node "$ROOT/e2e/demo/endcard.mjs" "$END_PNG" >/dev/null
[[ -f "$END_PNG" ]] || { echo "cut-roundtrip: end card was not rendered" >&2; exit 1; }

parts=""; labels=""
for s in $SEGMENTS; do
  in_var="${s}_IN"; out_var="${s}_OUT"
  lo="${!in_var}"; hi="${!out_var}"
  awk -v a="$lo" -v b="$hi" 'BEGIN{exit !(b > a)}' || {
    echo "cut-roundtrip: segment $s is empty ($lo -> $hi) — marks moved" >&2; exit 1; }
  printf 'cut-roundtrip: %s  %6.2f -> %6.2f  (%.2fs)\n' "$s" "$lo" "$hi" \
    "$(awk -v a="$lo" -v b="$hi" 'BEGIN{print b-a}')"
  lower="$(printf '%s' "$s" | tr 'A-Z' 'a-z')"
  parts="${parts}$(seg "$lo" "$hi" "$lower")"
  labels="${labels}[${lower}]"
done

filter="${parts}[1:v]trim=0:${END_SECS},setpts=PTS-STARTPTS,fps=${FPS},scale=${W}:${H},format=yuv420p,setsar=1[end];${labels}[end]concat=n=$(( $(echo "$SEGMENTS" | wc -w) + 1 )):v=1:a=0[v]"
filter="$(printf '%s' "$filter" | tr -d '\n' | sed 's/  */ /g; s/; /;/g; s/ \[/[/g')"

echo "cut-roundtrip: assembling → $OUT"
ffmpeg -v error -y -i "$TAKE" -loop 1 -t "$END_SECS" -i "$END_PNG" \
  -filter_complex "$filter" -map '[v]' \
  -c:v libx264 -pix_fmt yuv420p -crf 19 -preset slow -movflags +faststart \
  -r "$FPS" "$OUT"

ffprobe -v error -select_streams v:0 -show_entries stream=width,height \
  -show_entries format=duration,size -of default=nw=1 "$OUT"

# The poster is the frame the clip is judged by before anyone presses play, so
# it is not frame 0, and it is not inside X either. X now opens on the list and
# holds the chaos, where every shell is mid-thought: a poster taken there caught
# a live model saying "this directory doesn't look like it contains an actual
# checkout/payment codebase" — an honest sentence three seconds into an
# investigation, and a terrible thumbnail. The frame worth being judged on is
# the END of recovery A: a card, and directly beneath it that card's shell with
# a finished reading of the bug in it. Derived from the segment lengths so a
# retime cannot leave it pointing at the wrong beat.
POSTER_AT="${GADAK_RT_POSTER:-$(awk -v x1="$X_IN" -v x2="$X_OUT" -v y1="$Y_IN" -v y2="$Y_OUT" \
  'BEGIN{print (x2-x1) + (y2-y1)*0.92}')}"
poster="${OUT%.mp4}-poster.png"
ffmpeg -v error -y -ss "$POSTER_AT" -i "$OUT" -frames:v 1 "$poster"
echo "cut-roundtrip: poster at ${poster} (t=${POSTER_AT}s)"

if [[ "${1:-}" == "--sheet" ]]; then
  sheet="$SCRATCH/sheet.png"
  ffmpeg -v error -y -i "$OUT" -vf "fps=1,scale=320:-1,tile=7x4" -frames:v 1 "$sheet"
  echo "cut-roundtrip: sheet at $sheet"
fi
echo "cut-roundtrip: $OUT"
