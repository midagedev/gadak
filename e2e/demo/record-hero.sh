#!/usr/bin/env bash
# The interleaved two-camera hero shoot, one command (0.19, GDK-1118).
#
# What it orchestrates is not new — record-hero-phone.sh's own usage block
# already describes it. What it removes is the part that was done by hand,
# and the two ways that hand slipped:
#
#   1. The phone take has to start when the desk take walks away, and that
#      moment is a mark in a JSONL file, not a wall clock. Watching for it
#      by eye means either firing early (the pane is still on camera) or
#      late (the away-wait's 150s cap expires and the take is rejected).
#      Here the trigger is the mark itself.
#   2. GADAK_HERO_MAX_TAKES defaults to 2, and a second desk take re-seeds
#      the fixture — which silently unpairs the footage, because the phone
#      take that already happened belongs to the take that was discarded.
#      Measured on the first shoot: take 1 was rejected, take 2 recorded a
#      board the phone had never touched. This script pins it to 1 and says
#      so when a take is rejected: the whole shoot re-runs, both cameras.
#
# Everything else stays where it lives. The desk script owns the fixture,
# the serve, the agent HOME and the keyframes; the phone script owns the
# simulator, the tour and the dark flip; cut-hero.sh owns the cut list.
#
# Usage:
#   bash e2e/demo/record-hero.sh            # shoot both, then cut
#   bash e2e/demo/record-hero.sh --no-cut   # shoot only (retune the cut by hand)
#
# Live model, live simulator. Preconditions are the union of the two rigs:
# a Claude Code login, ffmpeg/ffprobe, Playwright chromium, and a booted iOS
# simulator with a DEV build of dev.gadak.mobile installed.
#
# Run `bash tools/tapes/prepare-claude-drive.sh --clean` afterwards — the
# isolated HOME holds a 0600 copy of this machine's credentials.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

DESK_OUT="$ROOT/e2e/.tmp/hero-desk"
PHONE_OUT="$ROOT/e2e/.tmp/hero-phone"
SCRATCH="$ROOT/scratch/hero"
PROOF="$DESK_OUT/proof-take-1.jsonl"
STAMP="$PHONE_OUT/vite-armed.json"

DO_CUT=1
for arg in "$@"; do
  case "$arg" in
    --no-cut) DO_CUT="" ;;
    *) echo "record-hero: unknown flag $arg" >&2; exit 64 ;;
  esac
done

mkdir -p "$DESK_OUT" "$PHONE_OUT" "$SCRATCH"

WARM_PID=""
DESK_PID=""
cleanup() {
  local rc=$?
  [[ -n "$DESK_PID" ]] && kill "$DESK_PID" 2>/dev/null || true
  [[ -n "$WARM_PID" ]] && kill "$WARM_PID" 2>/dev/null || true
  return $rc
}
trap cleanup EXIT

# ── 1. Warm the phone's vite. ──────────────────────────────────────────────
# The slow seam of the phone rig (~1 min of vite startup against a ~25s
# take), and it does not need the serve — which is why it can run first,
# before the desk script has seeded anything.
#
# The signal is the hold line, NOT the stamp file. The stamp is written when
# the dev server is *spawned*, before its own arming checks run — measured
# 2026-08-30: a warm run wrote the stamp, failed a check, and its EXIT trap
# deleted the stamp, all inside one poll interval. Waiting on the stamp saw
# an armed server that no longer existed and rolled both cameras anyway.
echo "record-hero: warming vite (this is the slow minute)…"
rm -f "$STAMP"
bash e2e/demo/record-hero-phone.sh --warm-vite >"$SCRATCH/warm.log" 2>&1 &
WARM_PID=$!
armed=""
for _ in $(seq 1 180); do
  if grep -q 'holding :' "$SCRATCH/warm.log" 2>/dev/null; then armed=1; break; fi
  kill -0 "$WARM_PID" 2>/dev/null || break
  sleep 1
done
[[ -n "$armed" && -f "$STAMP" ]] || {
  echo "record-hero: vite never armed — see $SCRATCH/warm.log" >&2
  tail -5 "$SCRATCH/warm.log" >&2 || true
  exit 1
}
echo "record-hero: vite armed ($(cat "$STAMP"))"

# ── 2. Roll the desk camera. ───────────────────────────────────────────────
# One take. See the header: a second take re-seeds the board and unpairs the
# phone footage, so a rejected take fails the whole shoot on purpose.
echo "record-hero: desk take rolling → $SCRATCH/desk-live.log"
rm -f "$PROOF"
GADAK_HERO_MAX_TAKES=1 bash e2e/demo/record-hero-desk.sh >"$SCRATCH/desk-live.log" 2>&1 &
DESK_PID=$!

# ── 3. Fire the phone when the desk walks away. ────────────────────────────
# bit1_detached is written the instant the pane is closed and the session is
# proven detached (hero-desk.spec.ts). From there the away-wait holds for a
# 45s floor and rejects at 150s; the phone take needs ~25.
echo "record-hero: waiting for bit1_detached…"
for _ in $(seq 1 900); do
  [[ -f "$PROOF" ]] && grep -q 'bit1_detached' "$PROOF" && break
  kill -0 "$DESK_PID" 2>/dev/null || { echo "record-hero: desk take died before the walk-away — see $SCRATCH/desk-live.log" >&2; exit 1; }
  sleep 1
done
grep -q 'bit1_detached' "$PROOF" 2>/dev/null || {
  echo "record-hero: no bit1_detached in $PROOF — see $SCRATCH/desk-live.log" >&2
  exit 1
}
echo "record-hero: walked away — phone take rolling → $SCRATCH/serve-phone.log"
bash e2e/demo/record-hero-phone.sh --reuse-vite >"$SCRATCH/serve-phone.log" 2>&1 || {
  echo "record-hero: phone take failed — see $SCRATCH/serve-phone.log" >&2
  tail -5 "$SCRATCH/serve-phone.log" >&2 || true
  # Both cameras stop. The desk take could physically finish, but its footage
  # would be half a shoot, and leaving it rolling leaves a live model talking
  # to a serve nobody is filming. The EXIT trap takes the desk take down with
  # this script; say so, rather than the opposite (measured 2026-08-30: this
  # line used to promise the desk take would finish, and the trap killed it
  # mid-away-wait — the take failed on ECONNREFUSED and read like a serve crash).
  echo "record-hero: stopping the desk take too — re-run the whole shoot." >&2
  exit 1
}
echo "record-hero: phone take archived"

# ── 4. Let the desk finish coming back. ────────────────────────────────────
desk_rc=0
wait "$DESK_PID" || desk_rc=$?
DESK_PID=""
if (( desk_rc != 0 )); then
  echo "record-hero: desk take rejected (rc=$desk_rc) — see $SCRATCH/desk-live.log" >&2
  echo "record-hero: re-run the WHOLE shoot; the phone footage belongs to this board." >&2
  exit "$desk_rc"
fi

kill "$WARM_PID" 2>/dev/null || true
WARM_PID=""

echo "record-hero: both takes held —"
echo "  desk  $SCRATCH/desk-take1.webm"
echo "  phone $SCRATCH/phone-take1.mov"

# ── 5. Marks, so the cut list can be retuned against this take. ────────────
echo "record-hero: proof marks (video seconds, GADAK_HERO_LEAD applied):"
python3 - "$PROOF" "${GADAK_HERO_LEAD:-2.0}" <<'PY'
import json, sys
rows = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
lead = float(sys.argv[2])
t0 = rows[0]["epoch_ms"]
for r in rows:
    print(f"  {(r['epoch_ms'] - t0) / 1000 + lead:8.2f}  {r['mark']}")
PY

if [[ -n "$DO_CUT" ]]; then
  echo "record-hero: cutting (the cut list is hand-tuned — confirm against the marks above)"
  bash e2e/demo/cut-hero.sh
fi

echo "record-hero: remember — bash tools/tapes/prepare-claude-drive.sh --clean"
