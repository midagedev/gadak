#!/usr/bin/env bash
# Hero phone bits 2–5, one continuous take → scratch/hero/phone-take1.mov
# (0.19, GDK-1118). Sister of record-hero-desk.sh, and deliberately the
# smaller half: the desk script owns the fixture and the serve, this one
# owns only the phone.
#
# Ownership, so the two halves are one shoot and not two:
#   the serve   — NOT started here. `record-hero-desk.sh --serve-only` seeds
#                 the board the take contract describes and holds
#                 127.0.0.1:$PORT; this script refuses to run without it.
#                 One mirror, one session, both cameras.
#   the walk    — mobile/src/lib/demo-tour.ts, armed by VITE_DEMO_TOUR=1.
#                 Absolute t≈ schedule from tour start; the beat boundaries
#                 in that table are the cut-sync contract this script's
#                 dark-flip mark is measured against.
#   the shell   — no QR dance. VITE_DEV_SHELL=1 lets loadTerminal() adopt
#                 the vite proxy the way boot() adopts it for the serve
#                 session, and the server agrees: changeOrigin makes the
#                 Host a loopback IP literal, so terminalGate's local rule
#                 admits the request with no Bearer (internal/server/
#                 terminal.go terminalLocal). Opt-in, so the unpaired
#                 three-tab default stays reachable in dev — which is where
#                 mobile/e2e/shell.spec.ts asserts it.
#   appearance  — the app follows the system, so the dark flip is `xcrun
#                 simctl ui`, scheduled here inside the tour's own window.
#
# Bit 4 types `gadak close <KEY>` through the phone's real IME path, and
# that is a REAL write on whatever origin the serve fronts. With
# --serve-only that origin is the throwaway standalone fixture (MEDIA.md:
# every asset from scripts, against a scrubbed snapshot, no real data) —
# which is exactly why this script refuses to attach to an arbitrary serve
# it did not recognise as one.
#
# Usage:
#   bash e2e/demo/record-hero-desk.sh --serve-only     # terminal 1, holds
#   bash e2e/demo/record-hero-phone.sh                 # terminal 2, records
#   bash e2e/demo/record-hero-phone.sh --frames-only [video]
#                     # re-extract the review keyframes from a finished take
#
# For the interleaved two-camera shoot — the phone bits happening inside the
# desk take's away-wait, so one board carries both halves:
#   bash e2e/demo/record-hero-phone.sh --warm-vite     # terminal 2, holds
#   bash e2e/demo/record-hero-desk.sh                  # terminal 1, records
#   bash e2e/demo/record-hero-phone.sh --reuse-vite    # when bit1_detached
#                     # lands in e2e/.tmp/hero-desk/proof-take-1.jsonl
#                     # (45s floor / 70s cap; the take needs ~25 of them)
#
# Requires a booted iOS simulator with a DEV build of dev.gadak.mobile
# installed (`cd mobile && npm run tauri ios dev` once — the installed
# binary points at the dev URL, so later takes only need the vite server).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

BUNDLE_ID="${GADAK_HERO_PHONE_BUNDLE:-dev.gadak.mobile}"
PORT="${GADAK_E2E_PORT:-7794}"
VITE_PORT=5180 # mobile/vite.config.ts pins this (strictPort)
SCRATCH="$ROOT/scratch/hero"
OUT="$ROOT/e2e/.tmp/hero-phone"

# The tour's own table (mobile/src/lib/demo-tour.ts tour()): bits 2–5 run
# 0.0s–15.4s, and the dark flip belongs in the board beat's window,
# 11.2–12.8s. LAUNCH_LEAD is the measured gap between `simctl launch`
# returning and the tour's t0 (page load → main.ts → armDemoTourInDev),
# retunable without touching the schedule.
TOUR_END_MS=15400
DARK_FLIP_MS=11800
LAUNCH_LEAD_MS="${GADAK_HERO_PHONE_LEAD_MS:-1800}"
# Offset from the video's FIRST FRAME to the tour's t0, which is a different
# number from LAUNCH_LEAD_MS: simctl's recorder drops the leading idle
# frames, so the file starts later than the recorder did (measured: a take
# scripted to ~18.1s of wall clock came back 15.97s long). Extraction reads
# this; the dark flip reads the one above. Retunable with --frames-only.
FRAME_LEAD_MS="${GADAK_HERO_PHONE_FRAME_LEAD_MS:-3000}"
TAIL_MS=900 # a beat of hold after the last frame, trimmed in the cut
# The name the shell pane's heading shows. It is the pairing label, and the
# arm supplies it so the take never puts whatever this simulator last
# scanned on camera (MEDIA.md: fictional fixture, nothing from a real home).
SHELL_LABEL="${GADAK_HERO_PHONE_SHELL_LABEL:-home}"

mkdir -p "$OUT" "$SCRATCH"

# ── Keyframe extraction ────────────────────────────────────────────────────
# One frame per beat boundary, at the tour's own marks plus FRAME_LEAD_MS.
# Reads only finished files, so it re-runs freely.
extract_frames() {
  local video="$1"
  local dir="$SCRATCH/phone-frames"
  rm -rf "$dir"
  mkdir -p "$dir"
  local mark name
  for mark in "bit2-open:200" "bit2-glide:2600" "bit3-shell:4400" \
              "bit4-typed:9800" "bit5-board:11400" "bit5-dark:13000" \
              "bit5-hold:15200"; do
    name="${mark%%:*}"
    local ms="${mark##*:}"
    local at
    at="$(python3 -c "print(max(0, (${FRAME_LEAD_MS} + ${ms})) / 1000)")"
    ffmpeg -v error -y -ss "$at" -i "$video" -frames:v 1 "$dir/${name}.png" || true
  done
  echo "record-hero-phone: frames in $dir"
}

if [[ "${1:-}" == "--frames-only" ]]; then
  video="${2:-$SCRATCH/phone-take1.mov}"
  [[ -f "$video" ]] || { echo "record-hero-phone: no video at $video" >&2; exit 1; }
  extract_frames "$video"
  exit 0
fi

# --warm-vite / --reuse-vite split the script at its one slow seam. The take
# itself is ~25 seconds; starting the dev server is the minute in front of
# it. The desk take's away-wait — the window the phone bits are supposed to
# happen inside — has a 45s floor and a 70s cap (hero-desk.spec.ts), so a
# genuinely interleaved shoot has to enter that window with vite already up.
WARM_ONLY=""
REUSE_VITE=""
for arg in "$@"; do
  case "$arg" in
    --warm-vite) WARM_ONLY=1 ;;
    --reuse-vite) REUSE_VITE=1 ;;
  esac
done
# The stamp is the arming contract across the two processes: env vars live
# inside the bundle and cannot be probed from out here, so a bare listener
# on :5180 proves nothing about VITE_DEMO_TOUR. Only a server this script
# started writes one, and --reuse-vite checks the pid is still that one.
STAMP="$OUT/vite-armed.json"

# ── Preconditions, each naming its own fix ─────────────────────────────────
command -v ffmpeg >/dev/null || { echo "record-hero-phone: ffmpeg required" >&2; exit 1; }
command -v xcrun >/dev/null || { echo "record-hero-phone: Xcode command line tools required" >&2; exit 1; }

booted="$(xcrun simctl list devices booted 2>/dev/null | sed -n 's/.*(\([0-9A-F-]\{36\}\)) (Booted).*/\1/p' | head -1)"
if [[ -z "$booted" ]]; then
  echo "record-hero-phone: no booted simulator — open Simulator.app and boot a device" >&2
  exit 1
fi
echo "record-hero-phone: simulator $booted"

if ! xcrun simctl listapps booted 2>/dev/null | grep -q "$BUNDLE_ID"; then
  echo "record-hero-phone: $BUNDLE_ID not installed — run 'cd mobile && npm run tauri ios dev' once" >&2
  exit 1
fi

# The serve is the desk script's, and it must be the hero fixture: a bare
# healthz answer would also come from a leftover e2e serve on this port
# (measured 2026-08-29 — one was squatting :7794 and served the demo mirror
# instead). The board is the discriminator, so ask for it by key.
#
# --warm-vite skips all of it: warming only starts a dev server, and in the
# interleaved shoot the serve it will proxy to is the desk take's, which
# does not exist yet. The run that actually films makes every check below.
if [[ -z "$WARM_ONLY" ]]; then
  if ! curl -sf -m 3 "http://127.0.0.1:${PORT}/healthz" >/dev/null; then
    echo "record-hero-phone: no serve on 127.0.0.1:${PORT}" >&2
    echo "record-hero-phone: run 'bash e2e/demo/record-hero-desk.sh --serve-only' first" >&2
    exit 1
  fi
  board="$(curl -sf -m 5 "http://127.0.0.1:${PORT}/api/v1/issues/bootstrap/" 2>/dev/null || true)"
  if [[ "$board" != *'"STD-'* ]]; then
    echo "record-hero-phone: the serve on :${PORT} is not the hero fixture (no STD issues)" >&2
    echo "record-hero-phone: stop it, then 'bash e2e/demo/record-hero-desk.sh --serve-only'" >&2
    exit 1
  fi
  open_key="$(printf '%s' "$board" | python3 -c '
import json, sys
doc = json.load(sys.stdin)
rows = doc.get("issues") or []
# The tour picks the first OPEN row after its own sort; this only needs to
# prove one exists, so bit 4 has a punchline to cause.
open_rows = [r for r in rows if r.get("status_category") != "done"]
print(open_rows[0]["issue_key"] if open_rows else "")
' 2>/dev/null || true)"
  if [[ -z "$open_key" ]]; then
    echo "record-hero-phone: the fixture has no open issue — bit 4 would have nothing to close" >&2
    exit 1
  fi
  echo "record-hero-phone: fixture ok (first open row ${open_key})"
fi

# ── The dev server the phone loads ─────────────────────────────────────────
# Node pinned to .nvmrc for the same reason the desk rig pins it (CLAUDE.md:
# a local-24/CI-20 gap has hidden defects before). Best effort.
if [[ -f "$ROOT/.nvmrc" && -d "$HOME/.nvm/versions/node" ]]; then
  want="$(tr -d '[:space:]' <"$ROOT/.nvmrc")"
  have="$(node --version 2>/dev/null || echo none)"
  if [[ "$have" != v"${want}".* ]]; then
    pin="$(ls -d "$HOME"/.nvm/versions/node/v"${want}".* 2>/dev/null | tail -1 || true)"
    if [[ -n "$pin" ]]; then
      export PATH="$pin/bin:$PATH"
      echo "record-hero-phone: node pinned to ${pin##*/}"
    fi
  fi
fi

VITE_PID=""
REC_PID=""
cleanup() {
  [[ -n "$REC_PID" ]] && kill -INT "$REC_PID" 2>/dev/null || true
  # Only the process that started the dev server tears it down (and clears
  # the stamp with it); a --reuse-vite run leaves VITE_PID empty on purpose,
  # so finishing a take does not pull the warm server out from under the
  # next one.
  if [[ -n "$VITE_PID" ]]; then
    kill "$VITE_PID" 2>/dev/null || true
    rm -f "$STAMP"
  fi
  # The appearance is the operator's machine state, not this take's.
  xcrun simctl ui booted appearance light >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ -n "$REUSE_VITE" ]]; then
  [[ -f "$STAMP" ]] || {
    echo "record-hero-phone: --reuse-vite but no $STAMP — run --warm-vite first" >&2
    exit 1
  }
  warm_pid="$(sed -n 's/.*"pid":\([0-9]*\).*/\1/p' "$STAMP")"
  warm_serve="$(sed -n 's/.*"serve_port":\([0-9]*\).*/\1/p' "$STAMP")"
  kill -0 "$warm_pid" 2>/dev/null || {
    echo "record-hero-phone: the warmed dev server (pid $warm_pid) is gone" >&2
    exit 1
  }
  # A warm server proxies to the port it was started with; pointing the take
  # at a different serve than the one it films is the two-unrelated-halves
  # failure this script exists to prevent.
  [[ "$warm_serve" == "$PORT" ]] || {
    echo "record-hero-phone: warmed against :${warm_serve}, asked for :${PORT}" >&2
    exit 1
  }
  echo "record-hero-phone: reusing the warmed dev server (pid $warm_pid)"
else
  existing="$(lsof -tiTCP:"$VITE_PORT" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$existing" ]]; then
    # strictPort means a squatter is a hard failure later; and an unarmed dev
    # server on this port would produce a take where nothing happens at all.
    echo "record-hero-phone: :${VITE_PORT} is in use — stop it (the tour arms at dev-server start)" >&2
    # shellcheck disable=SC2086
    ps -o pid=,command= -p $existing >&2 || true
    exit 1
  fi

  # Close the app before the server it points at appears. An instance left
  # running from an earlier take reconnects to a fresh dev server and
  # reloads — and the reload re-arms the tour, which walks and CLOSES AN
  # ISSUE with no camera on it. Measured 2026-08-29: warming for an
  # interleaved shoot silently transitioned STD-4 seventeen seconds before
  # the take that was supposed to close it began, and the film's count went
  # 12 → 10 for reasons the footage never shows.
  xcrun simctl terminate booted "$BUNDLE_ID" >/dev/null 2>&1 || true

  echo "record-hero-phone: starting armed dev server (proxy → :${PORT})…"
  (
    cd "$ROOT/mobile"
    GADAK_SERVE_PORT="$PORT" VITE_DEMO_TOUR=1 VITE_DEV_SHELL=1 \
      VITE_DEV_SHELL_LABEL="$SHELL_LABEL" exec npm run dev
  ) >"$OUT/vite.log" 2>&1 &
  VITE_PID=$!
  printf '{"pid":%s,"vite_port":%s,"serve_port":%s,"label":"%s"}\n' \
    "$VITE_PID" "$VITE_PORT" "$PORT" "$SHELL_LABEL" >"$STAMP"
fi

for i in $(seq 1 80); do
  if curl -sf -m 2 "http://localhost:${VITE_PORT}/" >/dev/null; then break; fi
  sleep 0.25
  if (( i == 80 )); then
    echo "record-hero-phone: dev server never answered" >&2
    tail -20 "$OUT/vite.log" >&2 || true
    exit 1
  fi
done
# The proxy, not just the page: a dev server that answers index.html while
# the proxy target is down produces a take of an empty board.
#
# Warming is exempt for the same reason the fixture checks above are: in an
# interleaved shoot the serve does not exist yet, and vite's proxy connects
# per request, so an unreachable target at warm time says nothing about the
# take. It is not a weakened guarantee — the run that actually films is
# --reuse-vite, and it makes this check with the serve up. Measured
# 2026-08-30: --warm-vite wrote the stamp, failed here, and its EXIT trap
# removed the stamp again; the orchestrator had already seen the stamp in
# that window and fired a phone take that had no dev server.
if [[ -z "$WARM_ONLY" ]]; then
  curl -sf -m 5 "http://localhost:${VITE_PORT}/api/v1/terminal/sessions/" >/dev/null || {
    echo "record-hero-phone: the dev proxy cannot reach the serve — no Shell tab, no bits 3-4" >&2
    exit 1
  }
  echo "record-hero-phone: dev server armed, proxy reaching the serve"
else
  echo "record-hero-phone: dev server armed (proxy target :${PORT} checked at take time)"
fi

if [[ -n "$WARM_ONLY" ]]; then
  echo "record-hero-phone: warm — holding :${VITE_PORT} armed for --reuse-vite (Ctrl-C to stop)"
  # Same shape as the desk rig's --serve-only hold: a sleep loop, so the EXIT
  # trap is the only way out and Ctrl-C never leaves the port held.
  while kill -0 "$VITE_PID" 2>/dev/null; do sleep 1; done
  exit 0
fi

# ── The take ───────────────────────────────────────────────────────────────
video="$OUT/take.mov"
rm -f "$video"
xcrun simctl ui booted appearance light >/dev/null 2>&1 || true
xcrun simctl terminate booted "$BUNDLE_ID" >/dev/null 2>&1 || true
sleep 1

xcrun simctl io booted recordVideo --codec h264 --force "$video" &
REC_PID=$!
sleep 1.2 # recorder warm-up, before anything the cut keeps

echo "record-hero-phone: launching $BUNDLE_ID"
xcrun simctl launch booted "$BUNDLE_ID" >/dev/null

# Everything below is measured from the launch, and the tour's t0 sits
# LAUNCH_LEAD_MS after it.
python3 -c "import time; time.sleep((${LAUNCH_LEAD_MS} + ${DARK_FLIP_MS}) / 1000)"
xcrun simctl ui booted appearance dark >/dev/null 2>&1 || true
python3 -c "import time; time.sleep((${TOUR_END_MS} - ${DARK_FLIP_MS} + ${TAIL_MS}) / 1000)"

kill -INT "$REC_PID" 2>/dev/null || true
wait "$REC_PID" 2>/dev/null || true
REC_PID=""
sleep 1

[[ -s "$video" ]] || { echo "record-hero-phone: no video written" >&2; exit 1; }
cp "$video" "$SCRATCH/phone-take1.mov"
echo "record-hero-phone: take archived at $SCRATCH/phone-take1.mov"
ffprobe -v error -show_entries format=duration \
  -show_entries stream=codec_name,width,height -of default=noprint_wrappers=1 \
  "$SCRATCH/phone-take1.mov"

# The result contract, checked against the workspace rather than the video —
# bit 4's write either landed or it did not, and status_category is the gate
# (display names prove nothing; CLAUDE.md).
closed="$(curl -sf -m 5 "http://127.0.0.1:${PORT}/api/v1/issues/bootstrap/" 2>/dev/null \
  | python3 -c '
import json, sys
doc = json.load(sys.stdin)
print(sum(1 for r in (doc.get("issues") or []) if r.get("status_category") == "done"))
' 2>/dev/null || echo 0)"
echo "record-hero-phone: done rows on the board after the take: ${closed}"

extract_frames "$SCRATCH/phone-take1.mov"
echo "record-hero-phone: tour log (arming + the write it announced):"
grep -i 'demo tour' "$OUT/vite.log" || echo "  (the tour logs to the app console, not here)"
