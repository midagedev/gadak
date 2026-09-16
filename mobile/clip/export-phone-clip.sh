#!/usr/bin/env bash
# Cut the phone take into the three artifacts a release ships (GDK-1955):
#
#   docs/media/phone[.<locale>].mp4          the landing's exhibit
#   docs/media/phone[.<locale>].gif          the READMEs' — GitHub strips
#                                            <video> from markdown
#   docs/media/phone-poster[.<locale>].png   the frame under the play button
#
# Source: the webm mobile/clip/phone-clip.spec.ts just recorded, at the
# phone's device-pixel size (402x874 @3x = 1206x2622). Run through
# `make media-phone-clip`, which owns the servers and the fixture.
#
# The head trim and the poster grab are the desk exporters' own helpers
# (first-ink.sh, poster.sh) rather than constants measured once on one take:
# a constant is how a locale variant shipped a blank poster and a blank flash
# at the head of an autoplaying loop (GDK-1838).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MEDIA="$ROOT/docs/media"
LOCALE="${GADAK_MEDIA_LOCALE:-en}"
case "$LOCALE" in
  en|ko|ja) ;;
  *) echo "export-phone-clip: GADAK_MEDIA_LOCALE must be en, ko or ja (got '$LOCALE')" >&2; exit 2 ;;
esac
SUFFIX=""
[[ "$LOCALE" == "en" ]] || SUFFIX=".$LOCALE"

UI_PORT="${GADAK_MOBILE_E2E_PORT:-5182}"
API_PORT="${GADAK_MOBILE_API_PORT:-7899}"
RESULTS="$ROOT/mobile/test-results/clip-${UI_PORT}-${API_PORT}"
mkdir -p "$MEDIA"

WEBM="$(find "$RESULTS" -type f -name 'video.webm' | head -n 1 || true)"
if [[ -z "$WEBM" ]]; then
  echo "export-phone-clip: no video.webm under $RESULTS" >&2
  echo "  run: make media-phone-clip" >&2
  exit 1
fi

# The take says which language it is in; this script takes the name from the
# environment. Ask the take rather than trusting the two to agree — they did
# not, once, and a Korean recording was written out as phone.mp4 with every
# gate green (2026-09-16).
TAKE_JSON="$(dirname "$WEBM")/take.json"
if [[ ! -f "$TAKE_JSON" ]]; then
  echo "export-phone-clip: no take.json beside $WEBM — the recording did not finish." >&2
  echo "  run: make media-phone-clip" >&2
  exit 1
fi
TAKE_LOCALE="$(python3 -c "
import json, sys
print(json.load(open('$TAKE_JSON'))['locale'])
")"
if [[ "$TAKE_LOCALE" != "$LOCALE" ]]; then
  echo "export-phone-clip: the recording under $RESULTS is the $TAKE_LOCALE take, but this run would write the $LOCALE files." >&2
  echo "  re-record with GADAK_MEDIA_LOCALE=$LOCALE make media-phone-clip, or export the take that is there." >&2
  exit 6
fi
echo "export-phone-clip: source $WEBM (locale $LOCALE, take.json agrees)"

# Open on the frame the take closes on. The clip loops on the landing, and
# its last beat is deliberately the screen its first beat opened on — so the
# head trim is not a clock reading, it is that contract measured: step
# through the head, compare each candidate against the final frame, and open
# on the first one that matches it.
#
# The two cheaper rules both failed here. first-ink.sh (the desk exporters'
# trim) reads black filler frames at the head of a Playwright webm as ink and
# answers 0.00s. freezedetect answers whichever threshold you pick: loose
# enough to see a hold at all, and the blank boot is itself a perfect freeze;
# tight enough to skip the boot, and it skips the opening hold too and the
# clip starts halfway down a scroll. Matching the ending has no threshold to
# tune and refuses the take that never came home.
TAIL_PNG="$(mktemp "${TMPDIR:-/tmp}/gadak-phone-tail.XXXXXX").png"
HEAD_PNG="$(mktemp "${TMPDIR:-/tmp}/gadak-phone-head.XXXXXX").png"
ffmpeg -y -v error -sseof -0.3 -i "$WEBM" -frames:v 1 "$TAIL_PNG"

# SSIM between a candidate head frame and that ending. The measured curve on
# the ko take (2026-09-16, 0.2s steps): 0.76 at 0.2s while the app is still
# booting, then a plateau of 0.980-0.986 from 0.4s to 2.2s — the opening hold,
# where the only difference from the ending is screencast noise — and 0.64
# from 2.4s on, once the list starts scrolling. 0.95 is the middle of that
# gap rather than a tuned number, and it matters that it is low: a floor up
# at 0.985 lands on the *last* frame of the plateau and the clip opens with
# a quarter second of stillness before the scroll, instead of the second and
# a half the beat was recorded to hold.
LOOP_SSIM_MIN=0.95
START=""
for step in $(python3 -c "
t = 0.2
while t <= 6.0001:
    print(f'{t:.1f}')
    t += 0.2
"); do
  ffmpeg -y -v error -ss "$step" -i "$WEBM" -frames:v 1 "$HEAD_PNG"
  score="$(ffmpeg -v error -i "$HEAD_PNG" -i "$TAIL_PNG" -lavfi "ssim=stats_file=-" -f null - 2>/dev/null |
    sed -n 's/.*All:\([0-9.]*\).*/\1/p' | head -1)"
  [[ -z "$score" ]] && continue
  if python3 -c "import sys; sys.exit(0 if float('$score') >= $LOOP_SSIM_MIN else 1)"; then
    START="$step"
    echo "export-phone-clip: opening at ${START}s — matches the closing frame (ssim $score)"
    break
  fi
done
rm -f "$TAIL_PNG" "$HEAD_PNG"
if [[ -z "$START" ]]; then
  echo "export-phone-clip: no frame in the first 6s matches the take's final frame (ssim floor $LOOP_SSIM_MIN)." >&2
  echo "  the take did not return to the screen it opened on, so the landing's loop would cut." >&2
  echo "  re-record: phone-clip.spec.ts's last beat is supposed to close the palette and go back to the list." >&2
  exit 4
fi

MP4="$MEDIA/phone$SUFFIX.mp4"
GIF="$MEDIA/phone$SUFFIX.gif"
POSTER="$MEDIA/phone-poster$SUFFIX.png"

# 804 wide, not the take's own 1206. The landing caps this clip at 300 css px
# (site/src/components/Landing.astro), so 804 is 2.7x of what any screen
# renders and the same file is 3.1 MB instead of 5.8 — the two desk clips it
# sits beside are 3.5 MB each. The resolution that matters was spent at
# record time, on laying the app out three times as large; this only decides
# how much of it survives the encode.
MP4_WIDTH=804
ffmpeg -y -v error -ss "$START" -i "$WEBM" \
  -an \
  -vf "scale=${MP4_WIDTH}:-2:flags=lanczos" \
  -c:v libx264 -pix_fmt yuv420p -preset medium -crf 23 \
  -movflags +faststart \
  "$MP4"

# The padded-capture gate. Playwright's video may scale a frame down but
# never up: ask for a size larger than the viewport and it pads the rest with
# flat grey, writes the file, and says nothing. On 2026-09-16 that shipped a
# take whose phone sat in the top-left ninth of every frame — the mp4, the
# gif and the poster were all produced, all valid, all 89% grey. The camera
# now carries its resolution in the layout (mobile/clip.config.ts), and this
# is the measurement that refuses the other shape: in a take that fills its
# frame, the bottom-right quadrant is app, and app has a luma range in it.
# Flat padding has none.
GUARD="$(mktemp "${TMPDIR:-/tmp}/gadak-phone-guard.XXXXXX").png"
trap 'rm -f "$PALETTE" "$GUARD"' EXIT
ffmpeg -y -v error -ss 0.3 -i "$MP4" -frames:v 1 -vf "crop=iw/2:ih/2:iw/2:ih/2" "$GUARD"
read -r GUARD_MIN GUARD_MAX <<<"$(ffprobe -v error -f lavfi -i "movie=$GUARD,signalstats" \
  -show_entries frame_tags=lavfi.signalstats.YMIN,lavfi.signalstats.YMAX -of json 2>/dev/null |
  python3 -c "
import sys, json
f = (json.load(sys.stdin).get('frames') or [{}])[0].get('tags', {})
print(f.get('lavfi.signalstats.YMIN', 0), f.get('lavfi.signalstats.YMAX', 0))
")"
GUARD_RANGE=$(( GUARD_MAX - GUARD_MIN ))
if (( GUARD_RANGE < 8 )); then
  echo "export-phone-clip: the bottom-right quadrant of $(basename "$MP4") is one flat tone (luma $GUARD_MIN..$GUARD_MAX)." >&2
  echo "  that is a padded capture, not a recording: the app is somewhere in a corner and the rest of the frame is filler." >&2
  echo "  check the zoom in mobile/clip.config.ts — Playwright's video only ever scales a frame down." >&2
  exit 5
fi
echo "export-phone-clip: frame-fill guard ok — bottom-right quadrant luma $GUARD_MIN..$GUARD_MAX"

# The README renders this at 260 px, so 360 is 1.4x of what a reader sees and
# already 2.5 MB of GIF — a phone screen scrolling is 15 seconds of almost
# every pixel changing, which is the worst case this format has. fps and
# colours give way before width.
PALETTE="$(mktemp "${TMPDIR:-/tmp}/gadak-phone-palette.XXXXXX").png"

make_gif() {
  local fps="$1" width="$2" colors="$3"
  echo "export-phone-clip: palette 2-pass gif fps=${fps} width=${width} colors=${colors}" >&2
  ffmpeg -y -v error -i "$MP4" \
    -vf "fps=${fps},scale=${width}:-1:flags=lanczos,palettegen=max_colors=${colors}:stats_mode=diff" \
    "$PALETTE"
  ffmpeg -y -v error -i "$MP4" -i "$PALETTE" \
    -lavfi "fps=${fps},scale=${width}:-1:flags=lanczos[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$GIF"
  if command -v gifsicle >/dev/null; then
    gifsicle -O3 --colors "$colors" "$GIF" -o "$GIF"
  fi
}

gif_bytes() { wc -c <"$GIF" | tr -d ' '; }

# Decimal MB, the unit MEDIA.md uses for every sibling clip. 4 MB is the
# README budget the group-by cut set.
MAX_BYTES=$((4 * 1000 * 1000))
make_gif 9 360 96
if (( $(gif_bytes) > MAX_BYTES )); then
  echo "export-phone-clip: gif $(gif_bytes) bytes > 4MB, retrying at fps=8 colors=64" >&2
  make_gif 8 360 64
fi
if (( $(gif_bytes) > MAX_BYTES )); then
  echo "export-phone-clip: gif $(gif_bytes) bytes > 4MB, retrying at fps=8 width=320 colors=64" >&2
  make_gif 8 320 64
fi
if (( $(gif_bytes) > MAX_BYTES )); then
  echo "export-phone-clip: gif is $(gif_bytes) bytes after the whole ladder — over the 4MB budget." >&2
  exit 3
fi

# The poster is the opening frame of the cut, chosen by measurement and
# refused if it has no ink in it.
bash "$ROOT/e2e/demo/poster.sh" --video "$MP4" --out "$POSTER"

probe() { ffprobe -v error -select_streams v:0 -show_entries "$1" -of csv=p=0 "$2" | head -1; }
echo "export-phone-clip: wrote $(basename "$MP4") ($(wc -c <"$MP4" | tr -d ' ') bytes, $(probe stream=width,height "$MP4"), $(probe format=duration "$MP4")s)"
echo "export-phone-clip: wrote $(basename "$GIF") ($(gif_bytes) bytes)"
echo "export-phone-clip: wrote $(basename "$POSTER") ($(wc -c <"$POSTER" | tr -d ' ') bytes)"
