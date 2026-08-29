#!/usr/bin/env bash
# The hero cut: two takes, one story → scratch/hero/hero.mp4 (0.19, GDK-1118).
#
# Inputs are the two rigs' archives, and nothing else:
#   scratch/hero/desk-take1.webm   record-hero-desk.sh   — bits 1 and 6
#   scratch/hero/phone-take1.mov   record-hero-phone.sh  — bits 2 through 5
#
# The two were shot on ONE serve and ONE terminal session, with the phone
# take running inside the desk take's away-wait (record-hero-phone.sh
# --warm-vite / --reuse-vite). That is why the desk's sidebar count at the
# end already includes the issue the phone closed: the cut is not claiming
# continuity it does not have.
#
# Composition. The desk is 1440x900 and the phone is 1206x2622, so a single
# frame cannot letterbox both without wasting most of it on one of them.
# The phone beats sit centred over the desk's own walk-away frame, dimmed
# and softened — which is also the film's argument in one image: the same
# board, in two places, at the same moment.
#
# Timings below are seconds into each take, read off that take: the desk's
# proof marks (e2e/.tmp/hero-desk/proof-take-1.jsonl, plus GADAK_HERO_LEAD)
# and, for the phone, its 1fps contact sheet — NOT the tour table, which
# describes the tour and not the recording. `record-hero.sh` prints both.
# Re-shoot and they move; this script is a cut list, not a contract.
#
# Usage:
#   bash e2e/demo/cut-hero.sh            # → scratch/hero/hero.mp4
#   bash e2e/demo/cut-hero.sh --strip    # + a review tile strip
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
SCRATCH="$ROOT/scratch/hero"

DESK="${GADAK_HERO_DESK:-$SCRATCH/desk-take1.webm}"
PHONE="${GADAK_HERO_PHONE:-$SCRATCH/phone-take1.mov}"
OUT="${GADAK_HERO_OUT:-$SCRATCH/hero.mp4}"

command -v ffmpeg >/dev/null || { echo "cut-hero: ffmpeg required" >&2; exit 1; }
[[ -f "$DESK" ]] || { echo "cut-hero: no desk take at $DESK" >&2; exit 1; }
[[ -f "$PHONE" ]] || { echo "cut-hero: no phone take at $PHONE" >&2; exit 1; }

# ── The cut list ───────────────────────────────────────────────────────────
# Desk. The proof marks are the map, but they are not frame-accurate against
# the recorder's own clock — read them, then confirm the frame. Measured on
# this take: the prompt is still going in at 14s, sent by 16, and the pane
# is gone by 18. So beat A opens mid-sentence; the film has no time for
# someone typing a paragraph and the interesting frame is the agent starting
# to move. BG_AT is the desk as it is left.
DESK_A_IN=13.0  DESK_A_OUT=17.2   # hand it over
DESK_B_IN=17.2  DESK_B_OUT=19.0   # the pane is gone; the work is not
DESK_G_IN=67.3  DESK_G_OUT=74.6   # back — the scrollback, and STD-7 in Done
BG_AT=20.5

# Phone. Read off the footage, NOT off the tour table plus FRAME_LEAD_MS.
# That arithmetic was wrong by ~2s on this take — the app's launch splash
# ran four seconds, so tour t0 landed well after the constant said, and the
# beat that opens the phone half was four seconds of blank white. The tour
# table describes the tour; only the file describes the recording.
#
# And read it off the CUT, not off phone-take1.mov: simctl's recorder writes
# a variable-frame-rate file whose 1fps contact sheet does not land on whole
# seconds (measured: a sheet tile read as take 12 was really take ~14.9).
# So the loop is — rough numbers in, rebuild, then
#
#   ffmpeg -i hero.mp4 -vf "fps=1,crop=560:1040:680:20,scale=190:-1,\
#          tile=13x1" -frames:v 1 sheet.png
#
# and move the boundaries until every second earns its place. What that
# loop removed here: four seconds of launch splash, the pane's connect
# skeleton, and the dark-flip hole below.
PH_2_IN=5.2   PH_2_OUT=8.7        # the glance: the board, on a phone
PH_3_IN=9.4   PH_3_OUT=11.6       # the terminal tab — the same machine
                                  # (9.4, not 8.9: the pane's connect skeleton)
PH_4_IN=11.6  PH_4_OUT=14.6       # `gadak close STD-n` by thumb, and the result
# The cut skips take 14.6-16.3. The appearance flip re-themes the terminal by
# re-rendering it, and the re-render comes back with an EMPTY buffer — the
# `STD-4 Done …` line the beat exists to show disappears, and the frame is a
# black pane with a scrollbar stripe down its right edge. That is a real
# defect (GDK-1156); the cut is not hiding it, it is declining to spend a
# second of a 26s film on it. Light result → dark board, hard cut.
PH_5_IN=16.3  PH_5_OUT=19.5       # dark: the count drops; the issue holds Done

W=1920 H=1080 FPS=30
PHONE_H=1000 # the phone's height in frame; 1206x2622 → 460x1000

still="$SCRATCH/.hero-bg.png"
ffmpeg -v error -y -ss "$BG_AT" -i "$DESK" -frames:v 1 "$still"

# A desk segment: fit to height, pillarbox to frame. Never crop — the whole
# argument of those beats is what the screen says.
desk_seg() {
  echo "[0:v]trim=${1}:${2},setpts=PTS-STARTPTS,fps=${FPS},scale=-2:${H}," \
       "pad=${W}:${H}:(ow-iw)/2:0:color=0x1b1917,format=yuv420p,setsar=1[${3}];"
}

# A phone segment: the same still under every one of them, so the phone beats
# read as one continuous shot of a phone held in front of a desk. The pad is
# a bezel — the recording is the screen only, and without an edge the phone
# reads as a floating rectangle of the same paper colour as the board behind
# it rather than as a device.
phone_seg() {
  echo "[1:v]trim=${1}:${2},setpts=PTS-STARTPTS,fps=${FPS},scale=-2:${PHONE_H}," \
       "pad=iw+20:ih+20:10:10:color=0x121110,format=yuv420p[p${3}];" \
       "[bg${3}][p${3}]overlay=(W-w)/2:(H-h)/2:shortest=1,format=yuv420p,setsar=1[${4}];"
}

filter="$(
  cat <<EOF
[2:v]fps=${FPS},scale=-2:${H},pad=${W}:${H}:(ow-iw)/2:0:color=0x1b1917,
     eq=brightness=-0.34:saturation=0.35:contrast=0.9,gblur=sigma=13,format=yuv420p,setsar=1,
     split=4[bg1][bg2][bg3][bg4];
$(desk_seg $DESK_A_IN $DESK_A_OUT a)
$(desk_seg $DESK_B_IN $DESK_B_OUT b)
$(phone_seg $PH_2_IN $PH_2_OUT 1 c)
$(phone_seg $PH_3_IN $PH_3_OUT 2 d)
$(phone_seg $PH_4_IN $PH_4_OUT 3 e)
$(phone_seg $PH_5_IN $PH_5_OUT 4 f)
$(desk_seg $DESK_G_IN $DESK_G_OUT g)
[a][b][c][d][e][f][g]concat=n=7:v=1:a=0[v]
EOF
)"
# ffmpeg wants one line and no stray spacing; the layout above is for readers.
filter="$(printf '%s' "$filter" | tr -d '\n' | sed 's/  */ /g; s/; /;/g; s/ \[/[/g')"

echo "cut-hero: assembling → $OUT"
ffmpeg -v error -y \
  -i "$DESK" -i "$PHONE" -loop 1 -i "$still" \
  -filter_complex "$filter" -map '[v]' \
  -c:v libx264 -pix_fmt yuv420p -crf 19 -preset slow -movflags +faststart \
  -r "$FPS" "$OUT"

ffprobe -v error -select_streams v:0 -show_entries stream=width,height,duration \
  -of default=nw=1 "$OUT"

# The poster is the frame the clip is judged by before anyone presses play,
# so it is not frame 0 — it is the one frame that carries the whole premise:
# the phone, mid-command, over the same board on the desk behind it.
#
# 14.4 is the command COMPLETE and not yet sent (`> gadak close STD-4`).
# A second later it has run, and the frame gives away its own punchline.
# A poster promises. Confirm it on the sheet after any retime.
POSTER_AT=14.4
poster="${OUT%.mp4}-poster.png"
ffmpeg -v error -y -ss "$POSTER_AT" -i "$OUT" -frames:v 1 "$poster"
echo "cut-hero: poster at $poster"

if [[ "${1:-}" == "--strip" ]]; then
  strip="$SCRATCH/hero-strip.png"
  ffmpeg -v error -y -i "$OUT" -vf "fps=1,scale=240:-1,tile=9x3" -frames:v 1 "$strip"
  echo "cut-hero: strip at $strip"
fi
echo "cut-hero: $OUT"
