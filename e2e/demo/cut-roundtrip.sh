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

W=1920 H=1080 FPS=30
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
# A: the board at rest, and ⌘K bringing the terminal into the same window.
A_IN="$(at board_at_rest 0.9)"   A_OUT="$(at pane_open -0.25)"
# B: two shells in the strip, both anonymous. It exists so the rename in C has
#    a "before", and it earns no more than a second.
B_IN="$(at strip_two_shells -0.4)" B_OUT="$(at strip_two_shells 0.8)"
# C: THE MONEY SHOT. Opens mid-command — the film has no time to watch someone
#    type a paragraph, and the interesting frame is the one just before Enter —
#    and runs UNBROKEN through Enter, the card crossing into In progress, the
#    `bound to session` reply, and the strip row taking the issue's name.
#    Nothing may be cut out of this range: the unbroken frame IS the claim.
#    It ends after strip_renamed because the rail/strip rename trails the row
#    by 0-2s (measured 0.27-1.27s over four samples — the roster poll runs on a
#    fixed 2s cadence while the row moves the instant the write lands).
C_IN="$(at claim_enter -1.7)"    C_OUT="$(at strip_renamed 2.2)"
# D: the return leg — the issue's own body hands a command back to the shell
#    that claimed it (▶ places it, a person runs it), the keys come back, and
#    one of them is clicked open. Ends on linkify rather than on the output so
#    the third leg is in frame: shell → tracker → shell → tracker.
D_IN="$(at detail_open -0.1)"    D_OUT="$(at linkify_opened 1.2)"
# E: THE CLIMAX / payoff. `gadak close`, the card leaves In progress, and the
#    Done chip reveals the column it landed in. The reveal is not decoration:
#    without it the payoff row sits on the frame's last twelve pixels
#    (measured) and the film's final image arrives cropped.
E_IN="$(at close_enter -0.7)"    E_OUT="$(at done_revealed 1.5)"
# F: THE CLIMAX — the hand leaves and three cards cross on their own. This is
#    the segment the board (GDK-1175) exists for and the one most likely to be
#    re-shot. It opens just AFTER the starting pistol — the armed shells clear
#    themselves at that instant, so the rig's while-loop and temp path are
#    never in frame — and runs unbroken through the volley. Nothing may be cut out of the
#    volley itself — three cards flying at once IS the shot, and measured they
#    all land within 493ms of the trigger.
# G: THE BRIDGE — the climax's cause, on screen. Three shells wearing three
#    issue keys with their dots running, beside a board that is still all
#    human chips. A blind reviewer of the previous cut could only work out
#    where the climax came from by reading the end card: the crew was armed
#    off-frame and the cards simply moved. This is release-video.md's G3
#    failure ("the argument lives off-frame") and this beat is its fix.
#    Filmed from the PERSON's shell, so no armed while-loop is in frame, and
#    held 2.6s because the strip renames on a 2s roster cadence (GDK-1182) —
#    cut shorter and it can land on a row that is still a hex id.
G_IN="$(at bridge_in -0.3)"     G_OUT="$(at bridge 0.1)"
F_IN="$(at hands_off 0.45)"     F_OUT="$(at end_frame 2.2)"

# H: the issue's own shell, found by its name — click a strip row and the
#    pane swaps to that session with its scrollback replayed. The film's
#    nearest claim: you do not hunt a terminal tab, you pick the issue.
H_IN="$(at session_away -0.6)"  H_OUT="$(at session_pick 2.0)"

# G (bridge) and F (volley) stay defined above: their footage is still in the
# take and the bounds still resolve, they are simply not in the running order.
SEGMENTS="A B C D E H"

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
# it is not frame 0. It is the money shot's own hold: the command has run, the
# card is in In progress, the strip row says the issue key. Derived from the
# segment lengths so a retime cannot leave it pointing at the wrong beat.
POSTER_AT="${GADAK_RT_POSTER:-$(python3 -c "
a=$A_OUT-$A_IN; b=$B_OUT-$B_IN; c=$C_IN
print(round(a+b+($C_OUT-$C_IN)*0.72, 2))")}"
poster="${OUT%.mp4}-poster.png"
ffmpeg -v error -y -ss "$POSTER_AT" -i "$OUT" -frames:v 1 "$poster"
echo "cut-roundtrip: poster at ${poster} (t=${POSTER_AT}s)"

if [[ "${1:-}" == "--sheet" ]]; then
  sheet="$SCRATCH/sheet.png"
  ffmpeg -v error -y -i "$OUT" -vf "fps=1,scale=320:-1,tile=7x4" -frames:v 1 "$sheet"
  echo "cut-roundtrip: sheet at $sheet"
fi
echo "cut-roundtrip: $OUT"
