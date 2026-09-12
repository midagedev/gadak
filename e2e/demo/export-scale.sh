#!/usr/bin/env bash
# Find the Playwright video from the scale take and emit
# docs/media/scale.gif + docs/media/scale.mp4.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="$ROOT/docs/media"
RESULTS="$ROOT/e2e/demo/test-results-scale"
mkdir -p "$OUT_DIR"

# The locale this take recorded in — the same variable scale-demo.spec.ts
# read to pin the UI (e2e/helpers.ts mediaLocale). en writes the bare names;
# every other locale gets the tag as the last segment before the extension,
# which is the shape site/src/i18n.ts mediaFor() asks the page for.
LOCALE="${GADAK_MEDIA_LOCALE:-en}"
case "$LOCALE" in
  en) TAG="" ;;
  ko|ja) TAG=".$LOCALE" ;;
  *) echo "export-scale: GADAK_MEDIA_LOCALE must be en|ko|ja, got '$LOCALE'" >&2; exit 2 ;;
esac
# The take has to say which language it is. Running this by hand over a
# results directory left by an earlier take of another locale produced an
# mp4 full of English pixels under a Japanese name, and nothing said so
# (measured 2026-09-07). `make media-scale` rm -rf's the directory first, so
# the failure only reaches a hand-run export -- which is what MEDIA.md
# documents. The spec writes this stamp; a mismatch stops here.
STAMP="$RESULTS/.gadak-media-locale"
if [[ ! -f "$STAMP" ]]; then
  echo "export-scale: $RESULTS carries no locale stamp -- it predates GDK-1501 or was not written by scale-demo.spec.ts." >&2
  echo "  re-record: GADAK_MEDIA_LOCALE=$LOCALE make media-scale" >&2
  exit 3
fi
TOOK="$(tr -d '[:space:]' <"$STAMP")"
if [[ "$TOOK" != "$LOCALE" ]]; then
  echo "export-scale: the take under $RESULTS was recorded in '$TOOK', not '$LOCALE' -- exporting it would name English pixels 'scale.$LOCALE.mp4'." >&2
  echo "  re-record: GADAK_MEDIA_LOCALE=$LOCALE make media-scale" >&2
  exit 3
fi
MP4="$OUT_DIR/scale${TAG}.mp4"
GIF="$OUT_DIR/scale${TAG}.gif"
POSTER="$OUT_DIR/scale-poster${TAG}.png"
echo "export-scale: locale $LOCALE → $(basename "$MP4"), $(basename "$GIF"), $(basename "$POSTER")"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "${WEBM}" ]]; then
  echo "export-scale: no video.webm under $RESULTS" >&2
  echo "  run: make media-scale" >&2
  exit 1
fi

echo "export-scale: source $WEBM"

# Trim the boot skeleton: the recording starts at page load, but the clip
# should open on the settled list (the "20,000 issues" count already up),
# not on gray placeholders.
#
# This was 2.4 — "measured on the take of 2026-08-23; re-measure if boot
# pacing changes". Boot pacing changed and nobody re-measured: on
# 2026-09-12 the take settled at 2.72s, so the encoded clip opened on 0.2s
# of empty page, and the landing hero autoplays it on a loop. It is measured
# per take now (first-ink.sh), which also re-anchors the zoompan map below —
# its times are seconds on the trimmed clip and were meant to start at
# settle. The old constant stands as the floor to search from, so a take
# that settles early cannot pull the opening back into the boot.
TRIM_HEAD="$(bash "$ROOT/e2e/demo/first-ink.sh" "$WEBM" --from 2.4 || echo 2.4)"
echo "export-scale: head trim ${TRIM_HEAD}s (first frame with ink at or after 2.4s)"

# GDK-751 (2026-08-24): post-process camera work on the mp4 — smoothstep
# push-in to the palette during the typing beats, back out for the regroup,
# push-in to the counts band (chips + breakdown) for the narrowing beats,
# and back to full frame before the loop point. Times are seconds on the
# trimmed clip; the beat map was measured on the 2026-08-23 take — re-map
# if the spec's pacing changes.
#
# The pull-out is the exception: it is derived, not measured. As a constant
# (15.9, from that same English take) it was wrong for two of the three
# languages, because the spec's pacing is not the same length in each —
# en ran 16.68s and settled with 0.04s to spare, ja ran 16.48s and the clip
# ended 0.22s into a 0.8s transition with the sidebar sliced down the middle,
# and ko ran 15.80s and never pulled out at all, ending zoomed. The landing
# autoplays this on a loop, so all three popped at the loop point; only en
# did not look broken doing it (measured 2026-09-12). The pull-out now ends
# where the clip does, so every language ends at rest and the loop is clean.
# Source pacing is untouched (the rejected
# approach re-timed the recording; this one only moves the crop). Holds are
# constant expressions, so they are mathematically static — measured
# consecutive-frame meandiff ≈0.005 in holds, monotonic ≈13→11 across a
# transition tail (no oscillation). The gif below stays full-frame.
ZT='(in/25)'
ZEA="(clip((${ZT}-1.2)/0.8,0,1)*clip((${ZT}-1.2)/0.8,0,1)*(3-2*clip((${ZT}-1.2)/0.8,0,1)))"
ZEB="(clip((${ZT}-5.5)/0.8,0,1)*clip((${ZT}-5.5)/0.8,0,1)*(3-2*clip((${ZT}-5.5)/0.8,0,1)))"
ZEC="(clip((${ZT}-8.6)/0.8,0,1)*clip((${ZT}-8.6)/0.8,0,1)*(3-2*clip((${ZT}-8.6)/0.8,0,1)))"
# Where the pull-out starts: 0.8s of transition plus 0.2s at rest, counted
# back from the end of the trimmed clip.
SRC_DUR="$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$WEBM")"
PULLOUT="$(python3 -c "print(f'{max(9.4, $SRC_DUR - $TRIM_HEAD - 1.0):.2f}')")"
echo "export-scale: pull-out at ${PULLOUT}s (clip runs $(python3 -c "print(f'{$SRC_DUR - $TRIM_HEAD:.2f}')")s after the head trim)"
ZED="(clip((${ZT}-${PULLOUT})/0.8,0,1)*clip((${ZT}-${PULLOUT})/0.8,0,1)*(3-2*clip((${ZT}-${PULLOUT})/0.8,0,1)))"
ZOOM="1+0.35021*${ZEA}-0.35021*${ZEB}+0.28*${ZEC}-0.28*${ZED}"
ZX="300*${ZEA}-300*${ZEB}+280*${ZEC}-280*${ZED}"
ZY="48*${ZEA}-48*${ZEB}"

ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" \
  -an \
  -vf "zoompan=z='${ZOOM}':x='${ZX}':y='${ZY}':d=1:s=1280x800:fps=25,format=yuv420p" \
  -c:v libx264 -preset medium -crf 20 \
  -movflags +faststart \
  "$MP4"

# Same budget ladder as export-groupby.sh: fps/colors before width.
# Ceiling is 4 MB (site hero slot).
FPS=9
WIDTH=960
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-scale-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT

make_gif() {
  local fps="$1" width="$2" colors="${3:-128}"
  echo "export-scale: palette 2-pass gif fps=${fps} width=${width} colors=${colors}" >&2
  ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -ss "$TRIM_HEAD" -i "$WEBM" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$GIF"
}

# Same budget ladder as export-groupby.sh: fps/colors before width.
# Ceiling is 4 MB (site hero slot).
make_gif "$FPS" "$WIDTH" 128
SIZE="$(stat -f %z "$GIF")"
if [[ "$SIZE" -gt 4194304 ]]; then
  make_gif 8 "$WIDTH" 96
fi
SIZE="$(stat -f %z "$GIF")"
if [[ "$SIZE" -gt 4194304 ]]; then
  make_gif 7 800 96
fi

# The poster, cut from the mp4 this run just wrote. It used to be an ad-hoc
# ffmpeg line in MEDIA.md that a person ran from memory, which is how a
# locale variant would ship with no poster at all, or with the English one
# still sitting under it — nothing in the repo would have said so. The frame
# is now chosen by measurement rather than by the clock (poster.sh): -ss 0.2
# was measured on an English take, and the ko and ja takes are still blank
# there — both shipped a near-white poster on 2026-09-12.
bash "$ROOT/e2e/demo/poster.sh" --video "$MP4" --out "$POSTER"

# The clip has to end at rest. A clip cut mid-transition still plays, still
# ends on the right content, and still passes every check that reads the last
# frame's pixels -- what is wrong with it only appears at the loop point, when
# a half-panned frame snaps back to the settled one. So the camera is measured
# rather than trusted: the last two frames of the mp4 must be the same frame.
# FAIL-first 2026-09-12 -- with the 15.9 constant restored, the ja take (which
# runs 16.48s) exited 6 at a mean frame difference of 12.7157 against the 1.0
# threshold; derived, the same take settles well inside it.
END_T="$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$MP4")"
CMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gadak-scale-tail.XXXXXX")"
ffmpeg -v error -y -ss "$(python3 -c "print(f'{$END_T - 0.20:.3f}')")" -i "$MP4" -frames:v 1 "$CMP_DIR/a.png"
ffmpeg -v error -y -ss "$(python3 -c "print(f'{$END_T - 0.05:.3f}')")" -i "$MP4" -frames:v 1 "$CMP_DIR/b.png"
TAIL_DIFF="$(ffmpeg -v error -i "$CMP_DIR/a.png" -i "$CMP_DIR/b.png" \
  -filter_complex "blend=all_mode=difference,signalstats,metadata=print:file=-" -f null - 2>/dev/null \
  | awk -F= '/YAVG/{print $2; exit}')"
rm -rf "$CMP_DIR"
if [[ -z "$TAIL_DIFF" ]] || python3 -c "import sys; sys.exit(0 if float('$TAIL_DIFF') > 1.0 else 1)"; then
  echo "export-scale: last 0.15s still moving (mean frame difference $TAIL_DIFF, want <= 1.0)." >&2
  echo "  The camera pull-out has not finished when the clip ends, so the landing's autoplay loop" >&2
  echo "  snaps from a half-panned frame back to the settled one. See the beat-map note above." >&2
  exit 6
fi
echo "export-scale: tail at rest (mean frame difference $TAIL_DIFF)"

ls -lh "$GIF" "$MP4" "$POSTER"
