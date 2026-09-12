#!/usr/bin/env bash
# The first moment a take has anything drawn on it (GDK-1838).
#
#   bash e2e/demo/first-ink.sh <video> [--from S] [--until S]
#
# Prints one number: the timestamp, in seconds, of the earliest frame at or
# after --from whose darkest luma is below the ink threshold. Exits 4 and
# prints nothing if no frame in the window has any.
#
# Why this exists. Every exporter opened its clip at a constant — 2.4s for
# scale and history, 2.2s for terminal, nothing at all for search and the
# web demo — and each constant was a measurement of one take on one day.
# Boot pacing moves. Measured 2026-09-12: the scale take settles at 2.72s in
# the source webm, so a 2.4s trim measured on 2026-08-23 left 0.2s of empty
# page at the head of the encoded clip, and the landing hero autoplays that
# clip on a loop — a blank flash every cycle, in the hero, forever. search
# and the web demo carried 0.12s and 0.16s of the same thing.
#
# A constant also silently de-anchors a camera map: export-scale.sh's
# zoompan times are "seconds on the trimmed clip" and were meant to start at
# settle. When the trim stops landing on settle, every beat is off by the
# difference, and nothing says so.
#
# `signalstats.YMIN` is the darkest luma in the frame. The app's paper
# measures 164-166 across the three locales and a settled frame measures
# 6-45, so INK_MAX at 100 sits in the middle of a gap two orders of
# magnitude wide. e2e/demo/poster.sh reads the same constant from here.
set -euo pipefail

INK_MAX=100

if [[ "${1:-}" == "--ink-max" ]]; then
  # Let poster.sh source the threshold instead of spelling it again.
  echo "$INK_MAX"
  exit 0
fi

VIDEO="${1:-}"
shift || true
FROM="0"
UNTIL="10"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --from) FROM="$2"; shift 2 ;;
    --until) UNTIL="$2"; shift 2 ;;
    *) echo "first-ink: unknown argument '$1'" >&2; exit 2 ;;
  esac
done
if [[ -z "$VIDEO" || ! -f "$VIDEO" ]]; then
  echo "first-ink: no such video: ${VIDEO:-<none>}" >&2
  exit 2
fi

ffprobe -v error -f lavfi \
  -i "movie=${VIDEO},select='between(t\,${FROM}\,${UNTIL})',signalstats" \
  -show_entries frame=pts_time -show_entries frame_tags=lavfi.signalstats.YMIN \
  -of json 2>/dev/null | python3 -c "
import sys, json
limit = $INK_MAX
try:
    frames = json.load(sys.stdin).get('frames') or []
except Exception:
    frames = []
for f in frames:
    y = f.get('tags', {}).get('lavfi.signalstats.YMIN')
    if y is None:
        continue
    if int(float(y)) < limit:
        print(f\"{float(f['pts_time']):.2f}\")
        raise SystemExit(0)
raise SystemExit(4)
"
