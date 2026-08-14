#!/usr/bin/env bash
# Probe whether the running Gadak.app window can be moved by dragging.
#
# Reads the window frame via System Events (Accessibility). If `cliclick` is
# on PATH, drags the sidebar logo row (the traffic-light inset, x≈140 y≈24
# in window coordinates — see .desktop-titlebar-row in web/src/app.css) and
# prints the position delta. Without cliclick the drag step is skipped.
#
# Usage: tools/check-desktop-drag.sh
#        GADAK_DRAG_PID=<pid> tools/check-desktop-drag.sh
# Exit 0 when a drag moved the window, 2 when the window was found but drag
# could not be automated, 1 on any other failure.
set -euo pipefail
GADAK_DRAG_PID="${GADAK_DRAG_PID:-}"

need() {
  command -v "$1" >/dev/null || {
    echo "check-desktop-drag: $1 not found" >&2
    exit 1
  }
}

need osascript

# Process name is "Gadak" for a released .app and "gadak-desktop" for a
# `go build` / build-app.sh binary launched from Contents/MacOS. GADAK_DRAG_PID
# selects among several copies (dev + installed).
read_frame() {
  osascript - "$GADAK_DRAG_PID" <<'APPLESCRIPT'
on run argv
  set wantPid to item 1 of argv
  tell application "System Events"
    set candidates to {}
    repeat with p in (every process)
      try
        set n to name of p
        if n is "Gadak" or n is "gadak-desktop" then
          set end of candidates to p
        end if
      end try
    end repeat
    if (count of candidates) is 0 then
      error "Gadak process not running"
    end if
    set target to item 1 of candidates
    if wantPid is not "" then
      set found to false
      repeat with p in candidates
        if (unix id of p as text) is wantPid then
          set target to p
          set found to true
          exit repeat
        end if
      end repeat
      if not found then
        error "no Gadak process with pid " & wantPid
      end if
    end if
    set frontmost of target to true
    delay 0.2
    if (count of windows of target) is 0 then
      error "Gadak has no windows"
    end if
    set pos to position of window 1 of target
    set sz to size of window 1 of target
    set AppleScript's text item delimiters to " "
    return (item 1 of pos as text) & " " & (item 2 of pos as text) & " " & (item 1 of sz as text) & " " & (item 2 of sz as text)
  end tell
end run
APPLESCRIPT
}

if ! frame="$(read_frame 2>/dev/null)"; then
  echo "check-desktop-drag: Gadak.app is not running (or Accessibility is denied for this terminal)" >&2
  echo "  launch desktop/build/Gadak.app, grant Accessibility, re-run" >&2
  exit 1
fi

# shellcheck disable=SC2086
set -- $frame
x=$1 y=$2 w=$3 h=$4
echo "pid:    ${GADAK_DRAG_PID:-first-gadak}"
echo "before: x=$x y=$y w=$w h=$h"

if ! command -v cliclick >/dev/null; then
  echo "check-desktop-drag: cliclick not installed — window found, drag not automated"
  echo "  brew install cliclick   # then re-run to assert a position delta"
  echo "DONE"
  exit 2
fi

# Sidebar first row, past the traffic lights (measured 20…78) and inside
# the 48px-tall wordmark band. Screen coords = window origin + inset.
sx=$((x + 140))
sy=$((y + 24))
dx=$((sx + 80))
dy=$((sy + 40))

cliclick "dd:${sx},${sy}" "du:${dx},${dy}"
sleep 0.3

after="$(read_frame)"
# shellcheck disable=SC2086
set -- $after
ax=$1 ay=$2
echo "after:  x=$ax y=$ay w=$3 h=$4"
echo "delta:  dx=$((ax - x)) dy=$((ay - y))"

if [ "$ax" -eq "$x" ] && [ "$ay" -eq "$y" ]; then
  echo "check-desktop-drag: window did not move" >&2
  exit 1
fi
echo "DONE"
