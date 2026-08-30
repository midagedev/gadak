#!/usr/bin/env bash
# The round-trip cut: one take, five beats, one end card (GDK-1159)
#   → scratch/roundtrip/roundtrip.mp4 (+ -poster.png)
#
# Input is one file and nothing else — scratch/roundtrip/take-dark.webm, from
# record-roundtrip.sh --dark. Everything the film argues happened inside that
# single 27-second recording, so there is no continuity to fake and no second
# camera to synchronise.
#
# Why dark. Both schemes were shot (a retake is seconds here, which is the
# point of a rig with no live model in it) and the frames compared side by
# side. The take ships dark for one reason that survives on a phone: the clip
# autoplays muted inside a feed that is usually white, and the dark frame is
# the only one of the two that reads as a *window* rather than as more page.
# gadak's light theme is warm paper — distinctive in the app, and at
# thumbnail size in a light feed its edges dissolve into the timeline around
# it. Light is one `record-roundtrip.sh` away if that call is reversed.
#
# ── The cut list ───────────────────────────────────────────────────────────
# Seconds into take-dark.webm. The proof marks are the map, not the territory:
# they are wall-clock and the recorder's clock starts a beat earlier, measured
# at -0.2s on this take (record-roundtrip.sh's GADAK_RT_LEAD) — and it MOVES
# between takes, by more than a second. So read the
# marks, then CONFIRM the frame on the contact sheet:
#
#   ffmpeg -i scratch/roundtrip/roundtrip.mp4 -vf "fps=1,scale=320:-1,tile=7x3" \
#     -frames:v 1 scratch/roundtrip/sheet.png
#
# Re-shoot and these move. This is a cut list, not a contract.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
SCRATCH="$ROOT/scratch/roundtrip"

TAKE="${GADAK_RT_TAKE:-$SCRATCH/take-dark.webm}"
OUT="${GADAK_RT_OUT:-$SCRATCH/roundtrip.mp4}"

command -v ffmpeg >/dev/null || { echo "cut-roundtrip: ffmpeg required" >&2; exit 1; }
[[ -f "$TAKE" ]] || { echo "cut-roundtrip: no take at $TAKE" >&2; exit 1; }

# A: the board, and ⌘K bringing the terminal into the same window.
A_IN=0.80  A_OUT=3.35
# B: two shells in the strip, both anonymous. One second — it exists so the
#    rename in beat C has a "before", and it earns no more than that.
B_IN=5.30  B_OUT=6.40
# C: THE MONEY SHOT. Opens mid-command (the film has no time to watch someone
#    type a paragraph, and the interesting frame is the one just before Enter)
#    and runs unbroken through Enter, the card crossing into In progress, the
#    `bound to session` reply and the strip row taking the issue's name.
#    NOTHING may be cut out of this range — the unbroken frame IS the claim.
C_IN=8.30  C_OUT=13.70
# D: the return leg — the issue's own body hands a command back to the shell
#    that claimed it (▶ places, a person runs), and the keys come back.
D_IN=14.00 D_OUT=19.40
# E: `gadak close STD-7`, the card leaves In progress, and the Done chip
#    reveals the section it landed in. The reveal is not decoration: without
#    it the payoff row sits on the frame's last twelve pixels (measured), and
#    the film's final image arrives cropped.
E_IN=22.90 E_OUT=27.60

W=1920 H=1080 FPS=30
BG=0x0e0d0c   # the take's own panel black, so the pillarbox is invisible

# 1440x810 is exactly 1920x1080 ÷ 1.333, so `scale` alone fills the frame:
# no pad, no crop, nothing enlarged in post. The pad below is a no-op kept for
# the day someone re-shoots at another size and needs it not to break.
seg() {
  echo "[0:v]trim=${1}:${2},setpts=PTS-STARTPTS,fps=${FPS},scale=${W}:${H}," \
       "pad=${W}:${H}:(ow-iw)/2:(oh-ih)/2:color=${BG},format=yuv420p,setsar=1[${3}];"
}

# ── The end card ───────────────────────────────────────────────────────────
# 26 seconds of the last hero went by without the word gadak at a readable
# size and without a URL (release-video.md: "Say the name"). This is that fix,
# and it is deliberately the plainest thing in the film: the name, the
# version, the URL, on the take's own black.
# It is a PNG, rendered by Playwright, not ffmpeg text: the ffmpeg here is
# built without libfreetype, so `drawtext` is not a filter it has — measured,
# the first assembly died on "No such filter: 'drawtext'". endcard.mjs owns
# the card's content and colours.
END_SECS=2.8
END_PNG="$SCRATCH/endcard.png"
node "$ROOT/e2e/demo/endcard.mjs" "$END_PNG" >/dev/null
[[ -f "$END_PNG" ]] || { echo "cut-roundtrip: end card was not rendered" >&2; exit 1; }

filter="$(
  cat <<EOF
$(seg $A_IN $A_OUT a)
$(seg $B_IN $B_OUT b)
$(seg $C_IN $C_OUT c)
$(seg $D_IN $D_OUT d)
$(seg $E_IN $E_OUT e)
[1:v]trim=0:${END_SECS},setpts=PTS-STARTPTS,fps=${FPS},scale=${W}:${H},format=yuv420p,setsar=1[f];
[a][b][c][d][e][f]concat=n=6:v=1:a=0[v]
EOF
)"
# ffmpeg wants one line and no stray spacing; the layout above is for readers.
filter="$(printf '%s' "$filter" | tr -d '\n' | sed 's/  */ /g; s/; /;/g; s/ \[/[/g')"

echo "cut-roundtrip: assembling → $OUT"
ffmpeg -v error -y -i "$TAKE" -loop 1 -t "$END_SECS" -i "$END_PNG" \
  -filter_complex "$filter" -map '[v]' \
  -c:v libx264 -pix_fmt yuv420p -crf 19 -preset slow -movflags +faststart \
  -r "$FPS" "$OUT"

ffprobe -v error -select_streams v:0 -show_entries stream=width,height \
  -show_entries format=duration,size -of default=nw=1 "$OUT"

# The poster is the frame the clip is judged by before anyone presses play, so
# it is not frame 0. It is the money shot's own hold: the command is run, the
# card is in In progress, the strip row says STD-7. A poster promises what the
# film delivers — confirm it on the sheet after any retime.
POSTER_AT="${GADAK_RT_POSTER:-7.0}"
poster="${OUT%.mp4}-poster.png"
ffmpeg -v error -y -ss "$POSTER_AT" -i "$OUT" -frames:v 1 "$poster"
echo "cut-roundtrip: poster at $poster"

if [[ "${1:-}" == "--sheet" ]]; then
  sheet="$SCRATCH/sheet.png"
  ffmpeg -v error -y -i "$OUT" -vf "fps=1,scale=320:-1,tile=7x3" -frames:v 1 "$sheet"
  echo "cut-roundtrip: sheet at $sheet"
fi
echo "cut-roundtrip: $OUT"
