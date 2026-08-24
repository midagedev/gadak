#!/usr/bin/env bash
# Composite VHS terminal + Playwright serve tab → docs/media/claude-drive*.
#
# Landscape (default): 1880×720 hstack — flagship-only
#   (promo-split.ts FLAGSHIP_L_WEB_W=1160, FLAGSHIP_L_WEB_H=688,
#   TERM_W=720, BAR_H=32). tokens/dashboards stay 1744×672 via FRAME_*.
# Vertical (GADAK_PROMO_LAYOUT=vertical): 1080×1350 vstack. Bar 48 +
#   flagship terminal 520 + web 782. tokens/dashboards use term 340;
#   Claude TUI fills the pane so the band is taller (must match
#   FLAGSHIP_V_TERM_H in claude-drive-web.spec.ts). The split clips
#   (GADAK_CLAUDE_DRIVE_CLIP=claude-dashboards|claude-tokens, vertical
#   only) reuse this exact geometry and the claude-drive-vertical workdir —
#   the web recorder is clip-agnostic — and write their own mp4 names.
#
# Idle waits are compressed by e2e/demo/static-cut.py, not jump-cut.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
LAYOUT="${GADAK_PROMO_LAYOUT:-landscape}"
CLIP="${GADAK_CLAUDE_DRIVE_CLIP:-claude-drive}"
case "$CLIP" in
  claude-drive|claude-dashboards|claude-tokens) ;;
  *)
    echo "export-claude-drive: unknown clip '${CLIP}' (GADAK_CLAUDE_DRIVE_CLIP)" >&2
    exit 2
    ;;
esac
if [[ "$CLIP" != "claude-drive" && "$LAYOUT" != "vertical" ]]; then
  echo "export-claude-drive: clip ${CLIP} is vertical-only (GADAK_PROMO_LAYOUT=vertical)" >&2
  exit 2
fi
if [[ "$LAYOUT" == "vertical" ]]; then
  WORK="$ROOT/e2e/.tmp/claude-drive-vertical"
  FINAL_MP4="$ROOT/docs/media/${CLIP}-vertical.mp4"
  FINAL_GIF=""
  RESULTS="$ROOT/e2e/.tmp/test-results-claude-drive-vertical"
  FRAME_W=1080
  FRAME_H=1350
  BAR_H=48
  TERM_W=1080
  TERM_H=520
  WEB_W=1080
  WEB_H=782
else
  WORK="$ROOT/e2e/.tmp/claude-drive"
  FINAL_MP4="$ROOT/docs/media/claude-drive.mp4"
  FINAL_GIF="$ROOT/docs/media/claude-drive.gif"
  RESULTS="$ROOT/e2e/.tmp/test-results-claude-drive"
  # Must match FLAGSHIP_L_* in promo-split.ts (720+1160, 32+688). Even.
  FRAME_W=1880
  FRAME_H=720
  BAR_H=32
  TERM_W=720
  TERM_H=688
  WEB_W=1160
  WEB_H=688
fi

OUT_DIR="$ROOT/docs/media"
TERM_MP4="$WORK/term.mp4"
WEB_WEBM="$WORK/web.webm"
WEB_MP4="$WORK/web.mp4"
RAW="$WORK/raw.mp4"
EDITED="$WORK/edited.mp4"
CUT="$WORK/static-cut.mp4"

mkdir -p "$OUT_DIR" "$WORK"

if [[ ! -s "$TERM_MP4" ]]; then
  echo "export-claude-drive: missing $TERM_MP4" >&2
  exit 1
fi
if [[ ! -s "$WEB_WEBM" ]]; then
  WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
  if [[ -z "${WEBM}" ]]; then
    echo "export-claude-drive: no video.webm (web half)" >&2
    exit 1
  fi
  cp -f "$WEBM" "$WEB_WEBM"
fi

echo "export-claude-drive: layout=${LAYOUT} clip=${CLIP} frame=${FRAME_W}x${FRAME_H} term=${TERM_W}x${TERM_H} web=${WEB_W}x${WEB_H}"

# Re-composite only when term/web are newer than raw. A leftover raw from a
# previous take is older than the new term.mp4, so a reshoot still composites.
# This round's premise is the on-disk raw is the original — overwriting it
# would force a reshoot.
REUSE_RAW=0
if [[ -s "$RAW" && -s "$TERM_MP4" && -s "$WEB_WEBM" \
      && "$RAW" -nt "$TERM_MP4" && "$RAW" -nt "$WEB_WEBM" ]]; then
  REUSE_RAW=1
  echo "export-claude-drive: reusing existing $RAW (newer than term.mp4 and web.webm; skip re-composite)"
fi

if [[ "$REUSE_RAW" -eq 0 ]]; then
  if [[ ! -s "$WEB_WEBM" ]]; then
    WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
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

  if [[ "$LAYOUT" == "vertical" ]]; then
    WEB_Y=$((BAR_H + TERM_H))
    echo "export-claude-drive: vstack raw composite web_y=${WEB_Y}"
    ffmpeg -y -i "$TERM_MP4" -i "$WEB_MP4" -filter_complex "\
[1:v]trim=start=${OFFSET},setpts=PTS-STARTPTS,scale=${FRAME_W}:${FRAME_H}:flags=lanczos,setsar=1[full];\
[full]split=2[fulla][fullb];\
[fulla]crop=${FRAME_W}:${BAR_H}:0:0[bar];\
[fullb]crop=${WEB_W}:${WEB_H}:0:${WEB_Y}[web];\
[0:v]scale=${TERM_W}:${TERM_H}:flags=lanczos,setsar=1,fps=8[term];\
[bar][term][web]vstack=inputs=3:shortest=1[out]\
" -map "[out]" -an -c:v libx264 -pix_fmt yuv420p -preset medium -crf 20 \
      -movflags +faststart "$RAW"
  else
    echo "export-claude-drive: hstack raw composite"
    ffmpeg -y -i "$TERM_MP4" -i "$WEB_MP4" -filter_complex "\
[1:v]trim=start=${OFFSET},setpts=PTS-STARTPTS,scale=${FRAME_W}:${FRAME_H}:flags=lanczos,setsar=1[full];\
[full]split=2[fulla][fullb];\
[fulla]crop=${FRAME_W}:${BAR_H}:0:0[bar];\
[fullb]crop=${WEB_W}:${WEB_H}:${TERM_W}:${BAR_H}[web];\
[0:v]scale=${TERM_W}:${TERM_H}:flags=lanczos,setsar=1,fps=8[term];\
[term][web]hstack=inputs=2:shortest=1[mid];\
[bar][mid]vstack=inputs=2:shortest=1[out]\
" -map "[out]" -an -c:v libx264 -pix_fmt yuv420p -preset medium -crf 20 \
      -movflags +faststart "$RAW"
  fi
fi

RAW_DUR="$(ffprobe -v error -show_entries format=duration -of default=nw=1:nk=1 "$RAW")"
echo "export-claude-drive: raw duration=${RAW_DUR}s"

python3 "$ROOT/e2e/demo/edit-claude-drive.py" "$WORK" "$RAW_DUR"

echo "export-claude-drive: static-cut"
# claude-tokens: the dimension repaint (ui.tokens spacing/type axes) emits
# no web-timeline beat — claude-drive-web.spec.ts watches colours and
# dashboards only — so edit-claude-drive.py's event-based trim_end can cut
# the densify payoff when it lands after the last colour change. Trim at
# the raw duration instead (CLI --trim-end overrides the protect JSON);
# static-cut still compresses the idle tail and --end-hold keeps the final
# hold. Other clips keep the event-based trim.
if [[ "$CLIP" == "claude-tokens" ]]; then
  python3 "$ROOT/e2e/demo/static-cut.py" \
    --input "$RAW" \
    --output "$CUT" \
    --protect-json "$WORK/edit-protect.json" \
    --log "$WORK/static-cut.json" \
    --fps 25 \
    --threshold 0.50 \
    --trim-end "$RAW_DUR"
else
  python3 "$ROOT/e2e/demo/static-cut.py" \
    --input "$RAW" \
    --output "$CUT" \
    --protect-json "$WORK/edit-protect.json" \
    --log "$WORK/static-cut.json" \
    --fps 25 \
    --threshold 0.50
fi

# Even width/height for yuv420p.
# fps=25 matches the Playwright half (VHS is 8).
# drawbox covers the Claude TUI footer ("manual mode on") which is a harness
# leak from a parent Claude Code session. Conversation text, including
# Claude's own usage-limit banner, is not painted over.
if [[ "$LAYOUT" == "vertical" ]]; then
  # Footer "manual mode on" sits above the last 36 px of the 520 band
  # (take-1 frames). Cover the bottom 64 px of the terminal.
  BOX_H=64
  BOX_Y=$((BAR_H + TERM_H - BOX_H))
  BOX_W=$TERM_W
  ffmpeg -y -i "$CUT" \
    -vf "fps=25,scale=${FRAME_W}:${FRAME_H}:flags=lanczos,drawbox=x=0:y=${BOX_Y}:w=${BOX_W}:h=${BOX_H}:color=0xf4efe4:t=fill,format=yuv420p" \
    -an -c:v libx264 -preset medium -crf 21 -r 25 -movflags +faststart \
    "$FINAL_MP4"
else
  # Footer "manual mode on" is the last 36 px of the 688 terminal band.
  BOX_H=36
  BOX_Y=$((BAR_H + TERM_H - BOX_H))
  BOX_W=$TERM_W
  ffmpeg -y -i "$CUT" \
    -vf "fps=25,scale=${FRAME_W}:${FRAME_H}:flags=lanczos,drawbox=x=0:y=${BOX_Y}:w=${BOX_W}:h=${BOX_H}:color=0xf4efe4:t=fill,format=yuv420p" \
    -an -c:v libx264 -preset medium -crf 21 -r 25 -movflags +faststart \
    "$FINAL_MP4"
fi
cp -f "$CUT" "$EDITED"

echo "export-claude-drive: wrote $FINAL_MP4 ($(wc -c <"$FINAL_MP4" | tr -d ' ') bytes)"

if [[ -n "$FINAL_GIF" ]]; then
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
fi

echo "export-claude-drive: ffprobe"
echo "── $FINAL_MP4"
ffprobe -v error -show_entries format=duration,size -show_entries stream=width,height,codec_name,r_frame_rate -of default=nw=1 "$FINAL_MP4"
if [[ -n "$FINAL_GIF" ]]; then
  echo "── $FINAL_GIF"
  ffprobe -v error -show_entries format=duration,size -show_entries stream=width,height,codec_name,r_frame_rate -of default=nw=1 "$FINAL_GIF"
fi
