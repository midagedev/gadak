#!/usr/bin/env bash
# The 0.19 round-trip take (GDK-1159) → scratch/roundtrip/take1.webm
#
#   "A command typed in the terminal moves the card on the board, right there
#    — the shell and the tracker are one screen."
#
# No live model, no simulator, no credential drive. Everything on camera is a
# `gadak` CLI write against a standalone workspace, which is the point of this
# rig rather than an economy: a retake costs seconds, so the loop that makes
# the film good (shoot → contact sheet → retime → shoot again) is affordable.
#
# Usage:
#   bash e2e/demo/record-roundtrip.sh              # light take
#   bash e2e/demo/record-roundtrip.sh --dark       # dark take, for comparison
#   bash e2e/demo/record-roundtrip.sh --frames-only [video]
#                                                  # re-extract keyframes only
#
# Ownership. This script does NOT seed its own fixture: record-hero-desk.sh is
# the single owner of seed_hero_home(), and two seeders drifting apart is how a
# rig starts filming a board nobody else has. It runs that script's
# `--serve-only` mode as a child (seed + serve + its own EXIT trap on the port)
# and then makes exactly one change on top, described below.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

PORT="${GADAK_E2E_PORT:-7794}"
export GADAK_E2E_PORT="$PORT"

HERO_HOME="/private/tmp/gadak-hero-desk/home"
GADAK_BIN="$ROOT/e2e/.tmp/gadak"
OUT="$ROOT/e2e/.tmp/roundtrip"
RESULTS="$ROOT/e2e/.tmp/test-results-roundtrip"
SCRATCH="$ROOT/scratch/roundtrip"
TARGET_KEY="${GADAK_RT_TARGET:-STD-7}"
SCHEME="light"
# Seconds between the video's first frame and the spec's 'start' mark —
# context creation to first navigation. Retunable for --frames-only.
LEAD="${GADAK_RT_LEAD:--0.2}"

mkdir -p "$OUT" "$SCRATCH"

FRAMES_ONLY=""
for arg in "$@"; do
  case "$arg" in
    --dark) SCHEME="dark" ;;
    --frames-only) FRAMES_ONLY=1 ;;
  esac
done

# ── Keyframe extraction ────────────────────────────────────────────────────
# One frame per beat, named for what it has to prove. Extraction reads only
# finished files, so it re-runs freely and a mistimed frame costs nothing.
extract_frames() {
  local video="$1" proof="$2"
  python3 - "$video" "$proof" "$SCRATCH/frames" "$LEAD" <<'PY'
import json, os, subprocess, sys

video, proof, frames_dir, lead = sys.argv[1], sys.argv[2], sys.argv[3], float(sys.argv[4])
marks = {}
with open(proof) as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        rec = json.loads(line)
        marks.setdefault(rec["mark"], rec["epoch_ms"])
if "start" not in marks:
    sys.exit(f"proof has no 'start' mark — cannot time frames: {proof}")
t0 = marks["start"]

names = [
    ("board_at_rest", "01-board-at-rest.png"),
    ("strip_two_shells", "02-two-shells.png"),
    ("claim_enter", "03-claim-typed.png"),
    ("card_moved", "04-card-moved.png"),
    ("moneyshot_hold", "05-moneyshot-hold.png"),
    ("shell_attached_seen", "06-detail-shell-attached.png"),
    ("command_placed", "07-command-placed.png"),
    ("command_output", "08-command-output.png"),
    ("linkify_opened", "09-linkify-opened.png"),
    ("close_enter", "10-close-typed.png"),
    ("card_done", "11-card-done.png"),
    ("end_frame", "12-end-frame.png"),
]
os.makedirs(frames_dir, exist_ok=True)
dur = float(subprocess.run(
    ["ffprobe", "-v", "error", "-show_entries", "format=duration",
     "-of", "default=noprint_wrappers=1:nokey=1", video],
    capture_output=True, text=True, check=True).stdout.strip())
for mark, out in names:
    if mark not in marks:
        # linkify is the one optional beat; anything else missing is a bug in
        # the take, so say which rather than dying on the first gap.
        print(f"frame {out}  SKIPPED (no '{mark}' mark)")
        continue
    t = (marks[mark] - t0) / 1000.0 + lead
    t = max(0.0, min(t, dur - 0.25))
    dest = os.path.join(frames_dir, out)
    r = subprocess.run(["ffmpeg", "-hide_banner", "-loglevel", "error",
                        "-ss", f"{t:.2f}", "-i", video,
                        "-frames:v", "1", "-y", dest])
    if r.returncode != 0 or not os.path.exists(dest):
        sys.exit(f"ffmpeg failed for {out} at t={t:.2f}s")
    print(f"frame {out}  t={t:.2f}s")

# The two numbers the cut is judged on, printed where they cannot be missed:
# how long the board took to answer each write.
for m in ("card_moved", "card_done"):
    print(f"note {m}: read ms_after_enter from {proof}")
PY
}

if [[ -n "$FRAMES_ONLY" ]]; then
  video="${2:-$SCRATCH/take1.webm}"
  proof="$(ls -t "$OUT"/proof-*.jsonl 2>/dev/null | head -1 || true)"
  [[ -f "$video" ]] || { echo "record-roundtrip: no video at $video" >&2; exit 1; }
  [[ -n "$proof" ]] || { echo "record-roundtrip: no proof file in $OUT" >&2; exit 1; }
  extract_frames "$video" "$proof"
  echo "record-roundtrip: frames in $SCRATCH/frames"
  exit 0
fi

command -v ffmpeg >/dev/null || { echo "record-roundtrip: ffmpeg required" >&2; exit 1; }
command -v ffprobe >/dev/null || { echo "record-roundtrip: ffprobe required" >&2; exit 1; }
[[ -x node_modules/.bin/playwright ]] || { echo "record-roundtrip: playwright missing (npm ci)" >&2; exit 1; }

[[ -x "$GADAK_BIN" ]] || {
  echo "record-roundtrip: $GADAK_BIN missing — go build -o e2e/.tmp/gadak ./cmd/gadak" >&2
  exit 1
}

busy="$(lsof -nP -iTCP:"$PORT" 2>/dev/null | awk 'NR > 1' || true)"
if [[ -n "$busy" ]]; then
  echo "record-roundtrip: :${PORT} is in use — close these, or set GADAK_E2E_PORT" >&2
  printf '%s\n' "$busy" >&2
  exit 1
fi

gh() { GADAK_HOME="$HERO_HOME" "$GADAK_BIN" "$@"; }

# ── The one change on top of the shared seed ───────────────────────────────
# Beat 2 needs a runnable command in STD-7's body, and that means a real ADF
# codeBlock: the ▶ is offered by adf.ts's commandHead() for a single-line
# codeBlock (web/src/lib/issue-commands.ts:42), and by nothing else.
#
# `gadak edit STD-7 -m` cannot make one. Measured 2026-08-30: a plain-text body
# with a ``` fence comes back from the mirror as five paragraphs, the fence
# markers among them as literal text —
#   {"content":[{"text":"```","type":"text"}],"type":"paragraph"}
# — and the detail panel rendered zero [data-run-command] buttons. There is no
# `--field description=` either (no such alias; `gadak fields` lists only the
# aliases the workspace configured). So the ADF goes in through the origin's
# REST surface, which is what `gadak api` exists for, and it is still a write
# that passes through origin rather than a poke at the mirror — the product
# invariant this repo will not bend (CLAUDE.md).
#
# The command in the fence is chosen, not arbitrary: its output has to print
# several issue keys, because beat 3 clicks one of them and only keys the
# mirror knows get linkified (web/src/lib/terminal/issue-links.ts:38).
seed_body() {
  python3 - "$TARGET_KEY" <<'PY' >"$OUT/body.json"
import json, sys
key = sys.argv[1]
# Short, and narrow where it counts. Beat 2 films this line inside a pane
# clamped to TERMINAL_MIN_WIDTH_PX (320) — with the detail panel docked at
# 1440 the layout has nothing left to give it — which is ~28 columns at the
# 19px the fixture sets. So the OUTPUT is one bare key per row rather than
# `gadak list`'s ~95-character table, which measured three wrapped lines per
# issue and read as noise. One column also makes beat 3 unambiguous: every
# underlined thing on screen is an issue key.
cmd = 'gadak sql "select key from issues_full where status_category=\'done\'"'
doc = {"type": "doc", "version": 1, "content": [
    {"type": "paragraph", "content": [{"type": "text", "text":
        "The weekly digest counts an issue twice when a bot closes it and a "
        "human re-opens and closes it again in the same week."}]},
    {"type": "paragraph", "content": [{"type": "text", "text":
        "Start from what the mirror already holds — the issues that are "
        "actually closed:"}]},
    {"type": "codeBlock", "content": [{"type": "text", "text": cmd}]},
    {"type": "paragraph", "content": [{"type": "text", "text":
        "Decide the counting rule, write it down, and close this issue."}]},
]}
json.dump({"fields": {"description": doc}}, sys.stdout)
PY
  gh api PUT "/rest/api/3/issue/${TARGET_KEY}" --data "@$OUT/body.json" --write --status \
    >"$OUT/seed-body.log" 2>&1 || {
      echo "record-roundtrip: could not set ${TARGET_KEY}'s body" >&2
      cat "$OUT/seed-body.log" >&2
      return 1
    }
  gh sync >/dev/null
  # FAIL-first, in the mirror rather than in the browser: no codeBlock, no ▶,
  # and beat 2 would film a click on nothing.
  local adf
  adf="$(gh sql --no-header \
    "SELECT description_adf FROM issues_full WHERE key='${TARGET_KEY}'" 2>/dev/null || true)"
  case "$adf" in
    *codeBlock*) ;;
    *) echo "record-roundtrip: ${TARGET_KEY} body has no codeBlock — the ▶ would be absent" >&2
       return 1 ;;
  esac
  # The terminal's own text is the line this film is about, so it is filmed at
  # 19px rather than the 13px default. A shipped setting
  # (`ui.tokens.type.--text-terminal`, internal/config/settings.go), read by
  # renderer.ts:139 — not a stylesheet patched for the camera.
  gh config set ui.tokens.type.--text-terminal 19px >/dev/null
  # And the body text with it. At the shipped 13px a list row's text line is
  # 18.2px, which is 24.3px in the 1080 frame — under the 27px (2.5%) floor
  # this shoot holds itself to, and the v0.19 post-mortem's whole finding was
  # that unreadable-on-a-phone is how a film dies. 16px is the top of
  # --text-body's own tested range (internal/config/tokencheck/dim-catalog.json:
  # 12–16) and its 1.4 line-height lands at 29.9px. Both the list summaries and
  # the issue body read it.
  #
  # The whole ladder moves, not just the one rung. Setting body alone earned a
  # warning from gadak itself — "--text-title 15px breaks >= 16px + 2px (type
  # steps closer than 2px read as noise, not hierarchy)" — which is the app
  # telling the camera it is about to film a broken type scale. So: micro 13,
  # body 16, title 18, heading 26, each inside its own catalogue range.
  # Largest first: each `config set` is checked against the ladder as it
  # stands, so raising body before title prints a warning about a state that
  # exists for one command. Descending order sets the same four values with a
  # clean log.
  gh config set ui.tokens.type.--text-heading 26px >/dev/null
  gh config set ui.tokens.type.--text-title 18px >/dev/null
  gh config set ui.tokens.type.--text-body 16px >/dev/null
  gh config set ui.tokens.type.--text-micro 13px >/dev/null
}

# ── The serve, owned by the hero rig ───────────────────────────────────────
SERVE_PID=""
stop_serve() {
  [[ -n "$SERVE_PID" ]] && kill "$SERVE_PID" 2>/dev/null || true
  local pids
  pids="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true)"
  # shellcheck disable=SC2086
  [[ -n "$pids" ]] && kill $pids 2>/dev/null || true
  return 0
}
trap stop_serve EXIT

echo "record-roundtrip: seeding + serving via record-hero-desk.sh --serve-only…"
bash e2e/demo/record-hero-desk.sh --serve-only >"$OUT/serve.log" 2>&1 &
SERVE_PID=$!
for i in $(seq 1 240); do
  curl -sf "http://127.0.0.1:${PORT}/healthz" >/dev/null && break
  kill -0 "$SERVE_PID" 2>/dev/null || { echo "record-roundtrip: serve died" >&2; tail -30 "$OUT/serve.log" >&2; exit 1; }
  sleep 1
done
curl -sf "http://127.0.0.1:${PORT}/healthz" >/dev/null || {
  echo "record-roundtrip: serve never became healthy" >&2; tail -30 "$OUT/serve.log" >&2; exit 1; }
echo "record-roundtrip: healthz ok on :${PORT}"

seed_body

# ── The take ───────────────────────────────────────────────────────────────
PROOF="$OUT/proof-${SCHEME}.jsonl"
rm -rf "$RESULTS"; rm -f "$PROOF"

env GADAK_MEDIA=1 GADAK_RT_PROOF="$PROOF" GADAK_RT_TARGET="$TARGET_KEY" \
    GADAK_RT_SCHEME="$SCHEME" \
    ./node_modules/.bin/playwright test --config e2e/demo/roundtrip.config.ts \
    2>&1 | tee "$OUT/take-${SCHEME}.log"

# The result contract is checked against the workspace, not against the video:
# a take that only *looks* right is exactly what a rig cannot see. status_category,
# never a display name (CLAUDE.md).
cat="$(gh sql --no-header \
  "SELECT status_category FROM issues_full WHERE key='${TARGET_KEY}'" 2>/dev/null || true)"
if [[ "$cat" != "done" ]]; then
  echo "record-roundtrip: ${TARGET_KEY} status_category='${cat}' — the take did not hold" >&2
  exit 1
fi
echo "record-roundtrip: workspace holds — ${TARGET_KEY} is done"

stop_serve

video="$(find "$RESULTS" -type f -name '*.webm' 2>/dev/null | head -1)"
[[ -n "$video" ]] || { echo "record-roundtrip: take passed but no video in $RESULTS" >&2; exit 1; }
dest="$SCRATCH/take-${SCHEME}.webm"
cp "$video" "$dest"
echo "record-roundtrip: take archived at $dest"
ffprobe -v error -show_entries format=duration \
  -show_entries stream=codec_name,width,height -of default=noprint_wrappers=1 "$dest"

grep -h 'ms_after_enter' "$PROOF" || true

extract_frames "$dest" "$PROOF"
echo "record-roundtrip: frames in $SCRATCH/frames"
