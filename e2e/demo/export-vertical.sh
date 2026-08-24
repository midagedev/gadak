#!/usr/bin/env bash
# Find the Playwright video from a vertical promo take and emit
# docs/media/{tokens,dashboards}-vertical.mp4 (no GIF — 1080-wide GIF
# would miss the 8 MiB README budget).
#
# Usage: bash e2e/demo/export-vertical.sh tokens|dashboards
set -euo pipefail

NAME="${1:-}"
if [[ "$NAME" != "tokens" && "$NAME" != "dashboards" ]]; then
  echo "export-vertical: usage: $0 tokens|dashboards" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="$ROOT/docs/media"
RESULTS="$ROOT/e2e/.tmp/test-results-${NAME}-vertical"
OUT="$OUT_DIR/${NAME}-vertical.mp4"
mkdir -p "$OUT_DIR"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "${WEBM}" ]]; then
  echo "export-vertical: no video.webm under $RESULTS" >&2
  echo "  run: bash e2e/demo/record-vertical.sh" >&2
  exit 1
fi

TRIM="0"
if [[ -f "$ROOT/e2e/.tmp/promo-trim-${NAME}" ]]; then
  TRIM="$(tr -d '[:space:]' <"$ROOT/e2e/.tmp/promo-trim-${NAME}")"
fi

echo "export-vertical: source $WEBM trim=${TRIM}s → $OUT"

# Geometry gate before encode — Playwright letterboxes if the viewport
# diverged from 1080×1350 (same trap as the landscape FRAME_W×FRAME_H note).
probe_dim() {
  ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0:s=x "$1"
}
SRC_DIM="$(probe_dim "$WEBM")"
if [[ "$SRC_DIM" != "1080x1350" ]]; then
  echo "export-vertical: source ${SRC_DIM}, want 1080x1350" >&2
  exit 1
fi

# Same ladder as export-tokens.sh / export-dashboards.sh: libx264 yuv420p
# preset medium crf 23 + faststart. No scale, no GIF.
ffmpeg -y -i "$WEBM" -ss "$TRIM" \
  -an \
  -c:v libx264 -pix_fmt yuv420p -preset medium -crf 23 \
  -movflags +faststart \
  "$OUT"

OUT_DIM="$(probe_dim "$OUT")"
if [[ "$OUT_DIM" != "1080x1350" ]]; then
  echo "export-vertical: encoded ${OUT_DIM}, want 1080x1350" >&2
  exit 1
fi

echo "export-vertical: wrote $OUT ($(wc -c <"$OUT" | tr -d ' ') bytes)"
