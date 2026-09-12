#!/usr/bin/env bash
# Find the Playwright video from the unified-search take and emit
# docs/media/search.gif + docs/media/search.mp4.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="$ROOT/docs/media"
RESULTS="$ROOT/e2e/demo/test-results-search"
mkdir -p "$OUT_DIR"

# The locale this take recorded in — the same variable search-demo.spec.ts
# read to pin the UI (e2e/helpers.ts mediaLocale). en writes the bare names;
# every other locale gets the tag as the last segment before the extension,
# which is the shape site/src/i18n.ts mediaFor() asks the page for.
LOCALE="${GADAK_MEDIA_LOCALE:-en}"
case "$LOCALE" in
  en) TAG="" ;;
  ko|ja) TAG=".$LOCALE" ;;
  *) echo "export-search: GADAK_MEDIA_LOCALE must be en|ko|ja, got '$LOCALE'" >&2; exit 2 ;;
esac
# The take has to say which language it is. Running this by hand over a
# results directory left by an earlier take of another locale produced an
# mp4 full of English pixels under a Japanese name, and nothing said so
# (measured 2026-09-07). `make media-search` rm -rf's the directory first, so
# the failure only reaches a hand-run export -- which is what MEDIA.md
# documents. The spec writes this stamp; a mismatch stops here.
STAMP="$RESULTS/.gadak-media-locale"
if [[ ! -f "$STAMP" ]]; then
  echo "export-search: $RESULTS carries no locale stamp -- it predates GDK-1501 or was not written by search-demo.spec.ts." >&2
  echo "  re-record: GADAK_MEDIA_LOCALE=$LOCALE make media-search" >&2
  exit 3
fi
TOOK="$(tr -d '[:space:]' <"$STAMP")"
if [[ "$TOOK" != "$LOCALE" ]]; then
  echo "export-search: the take under $RESULTS was recorded in '$TOOK', not '$LOCALE' -- exporting it would name English pixels 'search.$LOCALE.mp4'." >&2
  echo "  re-record: GADAK_MEDIA_LOCALE=$LOCALE make media-search" >&2
  exit 3
fi
MP4="$OUT_DIR/search${TAG}.mp4"
GIF="$OUT_DIR/search${TAG}.gif"
POSTER="$OUT_DIR/search-poster${TAG}.png"
echo "export-search: locale $LOCALE → $(basename "$MP4"), $(basename "$GIF"), $(basename "$POSTER")"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "${WEBM}" ]]; then
  echo "export-search: no video.webm under $RESULTS" >&2
  echo "  run: GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/search.config.ts" >&2
  exit 1
fi

echo "export-search: source $WEBM"

# There is no crop. GDK-751 (2026-08-24) added one — the landing shows this
# mp4 in a narrow column, so it framed the action region and let the sidebar
# fall outside. The geometry does not allow it, and two rounds of trying cost
# more than the framing was worth (2026-09-12):
#
#   sidebar      x 0 … 272   (web/src/lib/viewport-regime.ts LAYOUT_SIDEBAR_PX;
#                             1024 is above the 899 step, so this is the wide one)
#   palette      x 225 … 799 (centred in the 1024 viewport, width 574)
#   detail panel x 469 … 1024
#
# The palette starts 47px BEFORE the sidebar ends, so no crop can hold the
# palette whole and leave the sidebar out — the two overlap. `crop=800:640:224:0`
# held both beats and paid 48px of half-cut sidebar, which a Korean review read
# as 가록 where the product says 기록. Moving the origin to 272 to close that
# took 47px off the palette instead and mutilated every key in the frame
# (NMB-74 → IB-74, DOCUMENTS → MENTS) — a worse defect in the clip whose whole
# subject is search results. Full frame is the only framing that mutilates
# nothing, and the README gif has always been full-frame anyway, so this is the
# legibility the clip already shipped at.
#
# If the crop is ever wanted back, the fix is the viewport, not the origin:
# the palette clears the sidebar only at (W − 574)/2 ≥ 272, i.e. W ≥ 1118.
# These takes opened straight on the recording's first frame, which is the
# page before anything is painted: 0.12s of it in search, 0.16s in the web
# demo (measured 2026-09-12). A poster hides that from a reader who never
# presses play, and the landing autoplays on a loop, so it came back every
# cycle. The head is trimmed to the first frame that has ink in it
# (first-ink.sh); a take that is painted from frame 0 trims nothing.
INK_AT="$(bash "$ROOT/e2e/demo/first-ink.sh" "$WEBM" || echo 0)"
echo "export-search: head trim ${INK_AT}s (first frame with ink)"

ffmpeg -y -ss "$INK_AT" -i "$WEBM" \
  -an \
  -c:v libx264 -pix_fmt yuv420p -preset medium -crf 21 \
  -movflags +faststart \
  "$MP4"

# Same budget ladder as export-video.sh (web-demo): fps/colors before width.
FPS=9
WIDTH=960
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-search-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT

make_gif() {
  local fps="$1" width="$2" colors="${3:-128}"
  echo "export-search: palette 2-pass gif fps=${fps} width=${width} colors=${colors}" >&2
  ffmpeg -y -ss "$INK_AT" -i "$WEBM" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -ss "$INK_AT" -i "$WEBM" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$GIF"
}

make_gif "$FPS" "$WIDTH" 128

MAX_BYTES=$((8 * 1024 * 1024))
size_bytes() { wc -c <"$GIF" | tr -d ' '; }

if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-search: gif $(size_bytes) bytes > 8MB, retrying at fps=8 width=960 colors=96" >&2
  make_gif 8 960 96
fi
if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-search: gif $(size_bytes) bytes > 8MB, retrying at fps=8 width=900 colors=64" >&2
  make_gif 8 900 64
fi

# The poster, cut from the mp4 this run just wrote. It used to be an ad-hoc
# ffmpeg line in MEDIA.md that a person ran from memory, which is how a
# locale variant would ship with no poster at all, or with the English one
# still sitting under it — nothing in the repo would have said so. First
# settled frame (-ss 0.2, ec39ea3a).
bash "$ROOT/e2e/demo/poster.sh" --video "$MP4" --out "$POSTER"

# The frame that ships is the frame that was recorded. A crop here cannot be
# checked by eye at review time -- the clip still plays, the layout still
# looks like the product, and what is missing is 47px off one edge that only
# reads as wrong if you know which key the row should have carried. So the
# geometry is asserted instead of remembered: published dimensions must equal
# the take's. FAIL-first 2026-09-12 -- with crop=752:640:272:0 restored this
# printed `752x640 from a 1024x640 take` and exited 5.
SRC_WH="$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "$WEBM")"
OUT_WH="$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "$MP4")"
if [[ "$SRC_WH" != "$OUT_WH" ]]; then
  echo "export-search: the mp4 is ${OUT_WH/,/x} from a ${SRC_WH/,/x} take -- something cropped or scaled it." >&2
  echo "  The palette spans x 225..799 and the sidebar ends at 272: they overlap, so any crop that" >&2
  echo "  keeps the sidebar out takes 47px off the palette and cuts every issue key in the frame." >&2
  echo "  See the geometry note at the top of this file before changing the filter." >&2
  exit 5
fi
echo "export-search: geometry ${OUT_WH/,/x}, uncropped (matches the take)"

echo "export-search: wrote $GIF ($(size_bytes) bytes)"
echo "export-search: wrote $MP4 ($(wc -c <"$MP4" | tr -d ' ') bytes)"
echo "export-search: wrote $POSTER"
