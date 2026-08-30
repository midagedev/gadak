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
    ("chaos", "1-chaos.png"),
    ("a_replay", "2-recover-a.png"),
    ("a_alive", "3-alive-a.png"),
    ("b_replay", "4-recover-b.png"),
    ("b_alive", "5-alive-b.png"),
    ("end_frame", "6-end-frame.png"),
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
# `-m` with a fence used to come back as paragraphs with literal ``` in them
# (GDK-1178) and this function had to PUT ADF through `gadak api`. The fix
# landed; the fence is a codeBlock now, so the rig writes the body the way any
# agent would.
#
# The command in the fence is chosen, not arbitrary. Its output is one bare
# key per row: beat 3 clicks one of those keys, and `gadak list`'s
# ~95-character table wraps three times per row in a 400px pane and reads as
# noise. One column also makes beat 3 unambiguous — every underlined thing on
# screen is an issue key.
seed_body() {
  cat >"$OUT/body.md" <<'BODY'
The weekly digest counts an issue twice when a bot closes it and a human re-opens and closes it again in the same week.

Start from what the mirror already holds:

```
gadak next
```

Decide the counting rule, write it down, and close this issue.
BODY
  if ! gh edit "$TARGET_KEY" -m - <"$OUT/body.md" >"$OUT/seed-body.log" 2>&1; then
    echo "record-roundtrip: could not set ${TARGET_KEY}'s body" >&2
    cat "$OUT/seed-body.log" >&2
    return 1
  fi
  # Neutral titles. The shared seed describes gadak's own bugs, which is fine
  # for a rig and wrong for a camera: fifteen cards all naming defects in the
  # product being advertised reads as "0.19 is this broken". Retitled here so
  # hero's seed function stays untouched — it is one `edit --summary` pass.
  while IFS='|' read -r k t; do
    [ -n "$k" ] && gh edit "$k" --summary "$t" >/dev/null 2>&1
  done <<'TITLES'
STD-1|Checkout retries the same card twice
STD-2|Ship the spring pricing page
STD-3|Onboarding copy for the free tier
STD-4|Invoice export drops the tax column
STD-5|Keyboard shortcuts help is stale
STD-6|Map pins drift after a window restore
STD-7|Weekly digest counts an order twice
STD-8|Saved reports need an empty state
STD-9|Search highlight leaks into the sidebar
STD-10|Document the session grace window
STD-11|Dashboard cards drift on narrow windows
STD-12|Weekly triage sweep checklist
STD-13|Order detail loads fields it never shows
STD-14|Comment box eats the first keystroke
STD-15|Sign-up flow on fresh browsers
TITLES

  # `gadak next` is a saved recipe, not the built-in default — the built-in
  # prints a "no saved recipe" notice above its rows, and this is the one
  # command the film asks a stranger to read. Ten characters, so it fits the
  # ~28 columns a 320px pane has once the detail panel is docked; the raw SQL
  # it replaces was 47 and folded onto three lines. Two narrow columns, and
  # STD-3 in them, which is where beat 3 lands.
  gh recipes save next "select key, priority from issues_full where status_category != 'done' order by priority_rank limit 3" >/dev/null 2>&1 || true

  # Every issue gets a body. Beat 3 lands on whichever key the terminal
  # actually printed — the spec picks from real output rather than a
  # hardcoded key, which is right — so any of them can end up under the
  # camera. Seeding only the expected one left a take landing on STD-8 and
  # its "No description", which is a poor thing to show off in a film about a
  # tracker. STD-7's own body is set separately above: it is the only one
  # that needs a runnable fence.
  while IFS='|' read -r k t; do
    [ -n "$k" ] && [ "$k" != "$TARGET_KEY" ] && gh edit "$k" -m "$t" >/dev/null 2>&1
  done <<'BODIES'
STD-1|A declined retry is charging the card a second time. Reproduce with a test card that fails once, then decide whether the retry needs an idempotency key.
STD-2|Copy and screenshots are ready; the page needs the new tier table and a link from the footer.
STD-3|New accounts see the paid-tier copy for the first minute, then it swaps. Rewrite the free-tier strings and decide where the upgrade prompt belongs.
STD-4|The tax column is present in the preview and missing in the downloaded file. Likely the export builder and the preview build their column list separately.
STD-5|The overlay still lists shortcuts that were removed two releases ago, and misses the three added since.
STD-6|Pins land in the right place, then shift by a few pixels once the window is restored from minimised. Only on restore, not on resize.
STD-8|An account with no saved reports sees an empty panel with no explanation and no way to make one.
STD-9|The highlight from a search survives into the sidebar and stays until the next query.
STD-10|The window is real and undocumented: sessions survive a closed pane for a while, and nobody outside the code knows how long.
STD-11|Below about 900px the cards overlap their own headings. The grid is fine; the card min-width is not.
STD-12|The weekly sweep is done from memory. Write down what to look at, in order, so anyone can run it.
STD-13|The detail request asks for fields the panel never renders. Trim the query and measure what it saves.
STD-14|Typing into a fresh comment box loses the first character. Focus lands a frame before the box is ready.
STD-15|Sign-up works on a browser that has been used before and fails on a clean profile. Suspect a cookie the flow expects to already exist.
BODIES

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
  # 19px rather than the 13px default, and the whole type ladder moves with it
  # — setting one rung alone earns a warning from gadak itself ("type steps
  # closer than 2px read as noise, not hierarchy"). At the defaults a text row
  # is 24.3px in the 1080 frame, under the 27px this shoot holds itself to
  # (GDK-1184 proposes revisiting them). Largest first, so the ladder is never
  # transiently inverted.
  gh config set ui.tokens.type.--text-heading 26px >/dev/null
  gh config set ui.tokens.type.--text-title 18px >/dev/null
  gh config set ui.tokens.type.--text-body 16px >/dev/null
  gh config set ui.tokens.type.--text-micro 13px >/dev/null
  gh config set ui.tokens.type.--text-terminal 19px >/dev/null

  # The sidebar shrinks to its own shipped narrow value. Arithmetic, not
  # taste: the board's three columns are 288px each (BoardColumn.svelte, not
  # tokenised) and the pane is 400, so 208 + 400 + 864 = the 1472 frame
  # exactly. At the 272px default the Done column — where the film ends — is
  # pushed off screen.
  gh config set ui.tokens.layout.sidebar 208px >/dev/null

  # Two more people on the board. A comment is a "touch" the same way a
  # transition is (read.go:849), so this is the cheapest way to get more than
  # one human chip out of a seed that runs as a single actor.
  GADAK_ACTOR='human:grace|Grace Hopper' gh comment STD-9 \
    -m "Reproduced on a narrow window." >/dev/null 2>&1 || true
  GADAK_ACTOR='human:katherine|Katherine Johnson' gh comment STD-6 \
    -m "Only after a restore, not a resize." >/dev/null 2>&1 || true

  # One wiki page, so the sidebar's Documents section says something instead
  # of "No documents in these spaces". A frame of a product should not be
  # mostly the product's empty states. (The other one, "Personal views need an
  # identity", cannot be seeded away: a standalone workspace has no account by
  # design, which is the point of standalone.)
  gh page create --space LOC --title "Weekly digest rules" \
    -m "The digest counts an issue once per week, on its first close." >/dev/null 2>&1 || true
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

# The seed runs as a PERSON. Every write records its actor (a changelog entry
# counts, internal/server/read.go:849), and the board prints those as chips —
# so with the default actor the whole board wore "Claude Code" from frame one
# and the climax's three bots were not an arrival, they were more of the same.
# The film's last beat is "the line I typed was one"; that only lands if the
# board starts human. hero's seed function is not touched — the actor is
# inherited by the child that runs it.
echo "record-roundtrip: seeding + serving via record-hero-desk.sh --serve-only…"
GADAK_ACTOR="${GADAK_RT_SEED_ACTOR:-human:ada|Ada Lovelace}" \
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
# The film no longer closes anything; it claims five issues onto five shells.
# The contract is that all five really moved (status_category, never a display
# name — CLAUDE.md).
held="$(gh sql --no-header "SELECT count(*) FROM issues_full
  WHERE key IN ('STD-4','STD-9','STD-14','STD-1')
    AND status_category='inprogress'" 2>/dev/null || true)"
if [[ "$held" != "4" ]]; then
  echo "record-roundtrip: only ${held}/4 crew issues are in progress — the take did not hold" >&2
  exit 1
fi
echo "record-roundtrip: workspace holds — 4 issues claimed onto 4 shells"

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
