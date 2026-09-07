#!/usr/bin/env bash
# README reductions of a 4:5 Claude vertical: <stem>.mp4 → <stem>.gif (430
# wide @ 9 fps, 64 colours) + <stem>-poster.png (first settled frame).
#
# record-claude-drive.sh vertical writes only the mp4; the README pair ships
# as GIF because GitHub strips <video> from markdown (measured 2026-08-25 via
# `gh api /markdown`), and two of them sit side by side in one 900 px row —
# hence 430, not 540. Until 2026-09-07 this recipe lived only as a sentence
# in docs/project/MEDIA.md and the GIFs went stale behind a re-shot mp4.
#
# Usage: bash e2e/demo/export-vertical-gif.sh claude-dashboards-vertical [claude-tokens-vertical …]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MEDIA="$ROOT/docs/media"
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-vertical-palette.XXXXXX").png"
trap 'rm -f "$PALETTE"' EXIT
for stem in "$@"; do
  src="$MEDIA/$stem.mp4"
  [[ -f "$src" ]] || { echo "export-vertical-gif: $src missing" >&2; exit 1; }
  ffmpeg -y -v error -i "$src" \
    -vf "fps=9,scale=430:-1:flags=lanczos,palettegen=max_colors=64:stats_mode=diff" "$PALETTE"
  ffmpeg -y -v error -i "$src" -i "$PALETTE" \
    -lavfi "fps=9,scale=430:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$MEDIA/$stem.gif"
  if command -v gifsicle >/dev/null; then
    gifsicle -O3 --colors 64 "$MEDIA/$stem.gif" -o "$MEDIA/$stem.gif"
  fi
  # Same poster rule as the other exports (first settled frame, ec39ea3a).
  ffmpeg -y -v error -ss 0.2 -i "$src" -frames:v 1 "$MEDIA/$stem-poster.png"
  echo "export-vertical-gif: wrote $stem.gif ($(wc -c <"$MEDIA/$stem.gif" | tr -d ' ') bytes), $stem-poster.png"
done
