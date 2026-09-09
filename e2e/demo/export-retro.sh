#!/usr/bin/env bash
# Find the Playwright video from the retro take and emit
# docs/media/retro.gif + docs/media/retro.mp4.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="$ROOT/docs/media"
RESULTS="$ROOT/e2e/demo/test-results-retro"
mkdir -p "$OUT_DIR"

# The locale this take recorded in — the same variable retro-demo.spec.ts
# read to pin the UI (e2e/helpers.ts mediaLocale). en writes the bare names;
# every other locale gets the tag as the last segment before the extension.
LOCALE="${GADAK_MEDIA_LOCALE:-en}"
case "$LOCALE" in
  en) TAG="" ;;
  ko|ja) TAG=".$LOCALE" ;;
  *) echo "export-retro: GADAK_MEDIA_LOCALE must be en|ko|ja, got '$LOCALE'" >&2; exit 2 ;;
esac
# The take has to say which language it is: a hand-run export over an older
# take of another locale writes English pixels under a Korean name and
# nothing says so (GDK-1501, measured on the search clip 2026-09-07).
STAMP="$RESULTS/.gadak-media-locale"
if [[ ! -f "$STAMP" ]]; then
  echo "export-retro: $RESULTS carries no locale stamp -- it was not written by retro-demo.spec.ts." >&2
  echo "  re-record: GADAK_MEDIA_LOCALE=$LOCALE make media-retro" >&2
  exit 3
fi
TOOK="$(tr -d '[:space:]' <"$STAMP")"
if [[ "$TOOK" != "$LOCALE" ]]; then
  echo "export-retro: the take under $RESULTS was recorded in '$TOOK', not '$LOCALE' -- exporting it would name '$TOOK' pixels 'retro$TAG.mp4'." >&2
  echo "  re-record: GADAK_MEDIA_LOCALE=$LOCALE make media-retro" >&2
  exit 3
fi
echo "export-retro: locale $LOCALE -> retro$TAG.mp4, retro$TAG.gif"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "${WEBM}" ]]; then
  echo "export-retro: no video.webm under $RESULTS" >&2
  echo "  run: GADAK_MEDIA=1 ./node_modules/.bin/playwright test --config e2e/demo/retro.config.ts" >&2
  exit 1
fi

echo "export-retro: source $WEBM"

ffmpeg -y -i "$WEBM" \
  -an \
  -c:v libx264 -pix_fmt yuv420p -preset medium -crf 23 \
  -movflags +faststart \
  "$OUT_DIR/retro${TAG}.mp4"

# Same budget ladder as export-search.sh: fps/colors before width.
# Ceiling is 4 MB (C1 contract); search's ladder is 8 MB for a longer cut.
FPS=9
WIDTH=960
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-retro-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT

make_gif() {
  local fps="$1" width="$2" colors="${3:-128}"
  echo "export-retro: palette 2-pass gif fps=${fps} width=${width} colors=${colors}" >&2
  ffmpeg -y -i "$WEBM" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -i "$WEBM" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$OUT_DIR/retro${TAG}.gif"
}

make_gif "$FPS" "$WIDTH" 128

# Decimal MB (bytes/1e6), same unit MEDIA.md uses for the sibling clips.
MAX_BYTES=$((4 * 1000 * 1000))
size_bytes() { wc -c <"$OUT_DIR/retro${TAG}.gif" | tr -d ' '; }

if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-retro: gif $(size_bytes) bytes > 4MB, retrying at fps=8 width=960 colors=96" >&2
  make_gif 8 960 96
fi
if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-retro: gif $(size_bytes) bytes > 4MB, retrying at fps=8 width=900 colors=64" >&2
  make_gif 8 900 64
fi
if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-retro: gif $(size_bytes) bytes > 4MB, retrying at fps=7 width=800 colors=64" >&2
  make_gif 7 800 64
fi

# The poster, cut from the mp4 this run just wrote — same reason as the
# sibling (export-search.sh): a hand-run ffmpeg line is how a locale variant
# ships with the English poster still under it.
#
# Not the sibling's first settled frame (-ss 0.2): this take opens on the
# list, so frame one is a poster that says nothing about the report.
# POSTER_AT is the table beat, after the retro column has rendered its
# weeks — the one frame that carries the claim. Re-time if the beats move.
POSTER_AT=4.5
ffmpeg -y -v error -ss "$POSTER_AT" -i "$OUT_DIR/retro${TAG}.mp4" -frames:v 1 "$OUT_DIR/retro-poster${TAG}.png"

echo "export-retro: wrote $OUT_DIR/retro${TAG}.gif ($(size_bytes) bytes)"
echo "export-retro: wrote $OUT_DIR/retro${TAG}.mp4 ($(wc -c <"$OUT_DIR/retro${TAG}.mp4" | tr -d ' ') bytes)"
echo "export-retro: wrote $OUT_DIR/retro-poster${TAG}.png"
