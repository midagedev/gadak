#!/usr/bin/env bash
# Record docs/media/raycast.{gif,mp4}: search the mirror from Raycast, open
# the hit in the app through a gadak:// deep link.
#
# This is the one asset `make media` cannot produce headlessly: Raycast is a
# native overlay and the deep-link handoff needs the installed Gadak.app, so
# Playwright cannot drive either half. The compromise MEDIA.md documents:
# the SETUP and the TAKE are both scripted (this file), but the take runs on
# a live screen — run it on a machine whose display you control, and do not
# touch the keyboard while it records.
#
# Prerequisites, all checked below:
#   - Raycast with the gadak search extension in dev mode (`ray develop` in
#     the extension dir; see docs/MEDIA.md for where that lives)
#   - Gadak.app installed in /Applications and owning the gadak: scheme
#   - a `demo` profile whose gadak.db is a PRISTINE copy of examples/demo.db
#     — this script reseeds it, because a dirty copy leaks whatever writes
#     you tested into the recording (fixture authors are Alex Kim / Priya
#     Sharma / Marco Reyes; anything else on screen is your data)
#   - ffmpeg
#
# The capture region is exactly the app window this script positions, so
# nothing outside it can end up in the take. The account line at the bottom
# of the sidebar is scrubbed with delogo in the encode either way.
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
out="$repo/docs/media"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

command -v ffmpeg >/dev/null || { echo "ffmpeg missing" >&2; exit 1; }
test -d /Applications/Gadak.app || { echo "/Applications/Gadak.app missing" >&2; exit 1; }

# 1. Reseed the demo profile from the scrubbed snapshot.
demo="$HOME/.gadak/profiles/demo"
mkdir -p "$demo"
osascript -e 'tell application "Gadak" to quit' 2>/dev/null || true
sleep 2
cp "$repo/examples/demo.db" "$demo/gadak.db"
rm -f "$demo/gadak.db-wal" "$demo/gadak.db-shm"

# Freeze the profile before anything opens it. Reseeding the mirror is not
# enough: this profile's config.json is the one place a real site + token can
# survive between takes, and the app syncs on open — that is how 71 real rows
# once landed on top of the scrubbed ones under the same external_id, with the
# fictional author names replaced by real ones (GDK-181). A frozen workspace
# refuses every pull, so the snapshot stays the snapshot.
python3 - "$demo/config.json" <<'PY'
import json, os, sys
path = sys.argv[1]
cfg = {}
if os.path.exists(path):
    with open(path) as f:
        cfg = json.load(f)
cfg["frozen"] = True
with open(path, "w") as f:
    json.dump(cfg, f, indent=2)
# A config file is where a token lives, so a file we may have just created
# does not get its mode from the umask.
os.chmod(path, 0o600)
PY

# 2. Cold-launch on a neutral issue, pin the window to the capture region.
open "gadak://view/w/demo?issue=NMB-2"
sleep 6
osascript <<'EOF'
tell application "System Events"
  tell process "gadak-desktop"
    set position of window 1 to {76, 80}
    set size of window 1 to {1360, 820}
  end tell
end tell
EOF
swift -e 'import CoreGraphics; CGWarpMouseCursorPosition(CGPoint(x: 1500, y: 970))' 2>/dev/null || true

# 3. Open the extension and clear its search bar.
open "raycast://extensions/midagedev/gadak-search-probe/search"
sleep 1.5
osascript <<'EOF'
tell application "Raycast" to activate
delay 0.5
tell application "System Events" to key code 51 using command down
EOF

# 4. The take: a text search (highlight + field tag), then a key search
#    (GDK-170: the key finds its issue), then Enter — the deep link raises
#    the app on the hit.
(screencapture -v -x -R76,80,1360,820 -V 16 "$tmp/take.mov" &)
sleep 1.2
osascript <<'EOF'
tell application "Raycast" to activate
delay 0.8
tell application "System Events"
  repeat with c in (characters of "overflow")
    keystroke c
    delay 0.13
  end repeat
  delay 2.4
  key code 51 using command down
  delay 0.5
  repeat with c in (characters of "nmb140")
    keystroke c
    delay 0.13
  end repeat
  delay 2.0
  keystroke return
end tell
EOF
until [ -f "$tmp/take.mov" ] && ! pgrep -x screencapture >/dev/null; do sleep 1; done

# 5. Encode. delogo covers the signed-in account line, bottom of the sidebar
#    (region in 2x source pixels). Budgets per docs/MEDIA.md.
filt="trim=start=0.3:end=13.5,setpts=PTS-STARTPTS,delogo=x=70:y=1558:w=290:h=34"
ffmpeg -v error -i "$tmp/take.mov" \
  -vf "$filt,fps=10,scale=960:-1:flags=lanczos,palettegen=max_colors=128:stats_mode=diff" -y "$tmp/pal.png"
ffmpeg -v error -i "$tmp/take.mov" -i "$tmp/pal.png" \
  -lavfi "$filt,fps=10,scale=960:-1:flags=lanczos [x]; [x][1:v] paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
  -y "$out/raycast.gif"
ffmpeg -v error -i "$tmp/take.mov" \
  -vf "$filt,fps=30,scale=1088:-2:flags=lanczos" \
  -c:v libx264 -pix_fmt yuv420p -crf 23 -movflags +faststart -y "$out/raycast.mp4"

ls -la "$out/raycast.gif" "$out/raycast.mp4"
echo "review the gif frame by frame before committing — the take ran on a live screen"
