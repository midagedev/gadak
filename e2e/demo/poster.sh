#!/usr/bin/env bash
# The single owner of "which frame becomes the poster" (GDK-1836).
#
#   bash e2e/demo/poster.sh --video <mp4> --out <png> [--start S] [--tail]
#
# Every exporter used to cut its poster at a constant — `-ss 0.2` for the
# head takes, `-sseof -0.3` for the terminal's last frame. A constant is a
# measurement of one take, and the take it was measured on was English.
# Measured 2026-09-12: at 0.2s scale.ko.mp4 and scale.ja.mp4 are still the
# blank page (YMIN 166 and 164 — no dark pixel anywhere in frame), while
# scale.mp4 already has the header painted (YMIN 28). Both locales shipped a
# 22KB and a 16KB near-white poster under the landing's <video> tag, and
# nothing in the repo said so: the file existed, had the right name and the
# right dimensions.
#
# So the head poster is chosen by measurement rather than by a clock: the
# first frame at or after --start that has ink in it. `signalstats.YMIN` is
# the darkest luma in the frame; a settled page of text is under 100 and a
# blank one sits at the paper colour (~165). The tail poster keeps its
# position — the terminal clip's last frame is the payoff and no scan should
# move it — but it goes through the same refusal.
#
# The refusal is the recurrence gate: whatever frame was chosen, if it has
# no ink the export stops and names the number. A poster is the still a
# reader sees before pressing play, so a blank one is the one frame that
# must never ship.
set -euo pipefail

VIDEO="" OUT="" START="0.2" TAIL=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --video) VIDEO="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    --start) START="$2"; shift 2 ;;
    --tail) TAIL=1; shift ;;
    *) echo "poster: unknown argument '$1'" >&2; exit 2 ;;
  esac
done
if [[ -z "$VIDEO" || -z "$OUT" ]]; then
  echo "poster: --video and --out are required" >&2
  exit 2
fi
if [[ ! -f "$VIDEO" ]]; then
  echo "poster: no such video: $VIDEO" >&2
  exit 2
fi

# The darkest luma in the written PNG. signalstats' end-of-run summary is
# absent on some ffmpeg builds, so the measurement is taken from the file
# that was actually written — which every build can read back.
ymin_of_png() {
  ffprobe -v error -f lavfi -i "movie=$1,signalstats" -show_entries frame -of json 2>/dev/null |
    python3 -c "
import sys, json
d = json.load(sys.stdin)
f = d.get('frames') or []
print(f[0]['tags']['lavfi.signalstats.YMIN'] if f else 255)
"
}

# INK_MAX: a frame whose darkest pixel is lighter than this has nothing
# drawn on it. The paper the app renders on measures 164-166 across the
# three locales; settled frames measure 6-45. 100 is the middle of a gap
# that is two orders of magnitude wide, not a tuned threshold.
INK_MAX=100

CHOSEN=""
if [[ -n "$TAIL" ]]; then
  CHOSEN="$START"
  ffmpeg -y -v error -sseof "$CHOSEN" -i "$VIDEO" -frames:v 1 "$OUT"
else
  # 0.2s steps out to 4s: past that the clip is into its first beat and a
  # poster would no longer be the opening frame the exporter asked for.
  for step in $(python3 -c "
start = float('$START')
t = start
while t <= 4.0001:
    print(f'{t:.1f}')
    t += 0.2
"); do
    ffmpeg -y -v error -ss "$step" -i "$VIDEO" -frames:v 1 "$OUT"
    y="$(ymin_of_png "$OUT")"
    if [[ "$y" =~ ^[0-9]+$ ]] && (( y < INK_MAX )); then
      CHOSEN="$step"
      break
    fi
  done
fi

if [[ -z "$CHOSEN" ]]; then
  echo "poster: no frame between ${START}s and 4.0s of $(basename "$VIDEO") has ink in it (YMIN stayed at or above $INK_MAX)." >&2
  echo "  the take is blank for its whole head — re-record rather than shipping a white poster." >&2
  exit 4
fi

FINAL="$(ymin_of_png "$OUT")"
if ! [[ "$FINAL" =~ ^[0-9]+$ ]] || (( FINAL >= INK_MAX )); then
  echo "poster: $(basename "$OUT") has no ink in it (YMIN $FINAL, needs < $INK_MAX)." >&2
  echo "  a blank poster is what a reader sees before pressing play — refusing to write it." >&2
  exit 4
fi

if [[ -n "$TAIL" ]]; then
  echo "poster: $(basename "$OUT") ← ${CHOSEN}s from the end (YMIN $FINAL)"
else
  echo "poster: $(basename "$OUT") ← first frame with ink, ${CHOSEN}s (YMIN $FINAL)"
fi
