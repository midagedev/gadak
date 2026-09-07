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

# GDK-751 (2026-08-24): the landing shows this mp4 in a narrow column, so the
# mp4 is cropped to the action region (palette + detail; the sidebar carries no
# beat in this take). The README gif below stays full-frame. Source is
# 1024×640; crop keeps x∈[224,1024).
ffmpeg -y -i "$WEBM" \
  -an \
  -vf "crop=800:640:224:0" \
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
  ffmpeg -y -i "$WEBM" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -i "$WEBM" -i "$PALETTE" \
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
ffmpeg -y -v error -ss 0.2 -i "$MP4" -frames:v 1 "$POSTER"

echo "export-search: wrote $GIF ($(size_bytes) bytes)"
echo "export-search: wrote $MP4 ($(wc -c <"$MP4" | tr -d ' ') bytes)"
echo "export-search: wrote $POSTER"
