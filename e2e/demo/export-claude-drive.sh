#!/usr/bin/env bash
# Composite VHS terminal + Playwright serve tab → docs/media/claude-drive.{mp4,gif}.
#
# Reuses the tokens/dashboards promo split geometry (promo-split.ts:
# TERM_W=720, WEB_W=1024, BAR_H=32, WEB_H=640 → 1744×672) and the same
# palette-2-pass GIF ladder as export-tokens.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="$ROOT/docs/media"
WORK="$ROOT/e2e/.tmp/claude-drive"
TERM_MP4="$WORK/term.mp4"
WEB_WEBM="$WORK/web.webm"
WEB_MP4="$WORK/web.mp4"
RAW="$WORK/raw.mp4"
EDITED="$WORK/edited.mp4"
FINAL_MP4="$OUT_DIR/claude-drive.mp4"
FINAL_GIF="$OUT_DIR/claude-drive.gif"

mkdir -p "$OUT_DIR" "$WORK"

if [[ ! -s "$TERM_MP4" ]]; then
  echo "export-claude-drive: missing $TERM_MP4" >&2
  exit 1
fi
if [[ ! -s "$WEB_WEBM" ]]; then
  WEBM="$(find "$ROOT/e2e/.tmp/test-results-claude-drive" -type f -name 'video.webm' | head -n 1 || true)"
  if [[ -z "${WEBM}" ]]; then
    echo "export-claude-drive: no video.webm (web half)" >&2
    exit 1
  fi
  cp -f "$WEBM" "$WEB_WEBM"
fi

echo "export-claude-drive: webm → mp4"
ffmpeg -y -i "$WEB_WEBM" -an -c:v libx264 -pix_fmt yuv420p -preset medium -crf 20 \
  -movflags +faststart "$WEB_MP4"

WEB_EPOCH=0
VHS_EPOCH=0
if [[ -f "$WORK/web-ready-epoch" ]]; then
  WEB_EPOCH="$(tr -d '[:space:]' <"$WORK/web-ready-epoch")"
fi
if [[ -f "$WORK/vhs-show-epoch" ]]; then
  VHS_EPOCH="$(tr -d '[:space:]' <"$WORK/vhs-show-epoch")"
fi
OFFSET="$(python3 -c "w=float('${WEB_EPOCH}' or 0); v=float('${VHS_EPOCH}' or 0); print(max(0.0, v-w) if w and v else 0.0)")"
echo "export-claude-drive: web_epoch=${WEB_EPOCH} vhs_epoch=${VHS_EPOCH} offset=${OFFSET}s"

# Playwright frame is 1744×672 (bar 32 + split 640). VHS is 720×640, no bar.
# Crop the bar and the right 1024×640 from the (offset-trimmed) web video,
# scale the terminal, hstack, vstack the bar — same chrome as tokens.mp4.
echo "export-claude-drive: hstack raw composite"
ffmpeg -y -i "$TERM_MP4" -i "$WEB_MP4" -filter_complex "\
[1:v]trim=start=${OFFSET},setpts=PTS-STARTPTS,scale=1744:672:flags=lanczos,setsar=1[full];\
[full]split=2[fulla][fullb];\
[fulla]crop=1744:32:0:0[bar];\
[fullb]crop=1024:640:720:32[web];\
[0:v]scale=720:640:flags=lanczos,setsar=1,fps=8[term];\
[term][web]hstack=inputs=2:shortest=1[mid];\
[bar][mid]vstack=inputs=2:shortest=1[out]\
" -map "[out]" -an -c:v libx264 -pix_fmt yuv420p -preset medium -crf 20 \
  -movflags +faststart "$RAW"

RAW_DUR="$(ffprobe -v error -show_entries format=duration -of default=nw=1:nk=1 "$RAW")"
echo "export-claude-drive: raw duration=${RAW_DUR}s"

python3 "$ROOT/e2e/demo/edit-claude-drive.py" "$WORK" "$RAW_DUR"
FILTER="$(tr -d '\n' <"$WORK/edit-filter.txt")"
echo "export-claude-drive: edit filter: $FILTER"

ffmpeg -y -i "$RAW" -filter_complex "$FILTER" -map "[out]" \
  -an -c:v libx264 -pix_fmt yuv420p -preset medium -crf 21 \
  -movflags +faststart "$EDITED"

# Even width/height for yuv420p; promo contract is 1744×672.
# fps=25 matches the Playwright half (VHS is 8). The concat tbr otherwise
# reports 200/1. drawbox covers the Claude TUI footer ("manual mode on")
# which is a harness leak from a parent Claude Code session — same class
# of scrub as tools/record-raycast.sh stripping the signed-in account line.
# Conversation text, including Claude's own usage-limit banner, is not painted over.
ffmpeg -y -i "$EDITED" \
  -vf "fps=25,scale=1744:672:flags=lanczos,drawbox=x=0:y=636:w=720:h=36:color=0xf4efe4:t=fill,format=yuv420p" \
  -an -c:v libx264 -preset medium -crf 21 -r 25 -movflags +faststart \
  "$FINAL_MP4"

FPS=9
WIDTH=1280
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-claude-drive-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT

make_gif() {
  local fps="$1" width="$2" colors="${3:-128}"
  echo "export-claude-drive: palette 2-pass gif fps=${fps} width=${width} colors=${colors}" >&2
  ffmpeg -y -i "$FINAL_MP4" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -i "$FINAL_MP4" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$FINAL_GIF"
}

make_gif "$FPS" "$WIDTH" 128

MAX_BYTES=$((8 * 1024 * 1024))
size_bytes() { wc -c <"$FINAL_GIF" | tr -d ' '; }

if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-claude-drive: gif $(size_bytes) bytes > 8MB, retrying at fps=8 width=1280 colors=96" >&2
  make_gif 8 1280 96
fi
if (( $(size_bytes) > MAX_BYTES )); then
  echo "export-claude-drive: gif $(size_bytes) bytes > 8MB, retrying at fps=8 width=960 colors=64" >&2
  make_gif 8 960 64
fi
if command -v gifsicle >/dev/null; then
  echo "export-claude-drive: gifsicle -O3 --colors 64"
  gifsicle -O3 --colors 64 "$FINAL_GIF" -o "$FINAL_GIF"
fi

echo "export-claude-drive: wrote $FINAL_GIF ($(size_bytes) bytes)"
echo "export-claude-drive: wrote $FINAL_MP4 ($(wc -c <"$FINAL_MP4" | tr -d ' ') bytes)"
echo "export-claude-drive: ffprobe"
for f in "$FINAL_MP4" "$FINAL_GIF"; do
  echo "── $f"
  ffprobe -v error -show_entries format=duration,size -show_entries stream=width,height,codec_name,r_frame_rate -of default=nw=1 "$f"
done
