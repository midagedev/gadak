#!/usr/bin/env bash
# Hero desk bits 1 & 6, one continuous take → scratch/hero/desk-take1.webm
# (0.19, GDK-1037 B-loop: "자리를 비워도 일은 계속된다").
#
# Sister of record-terminal-claude.sh — same ownership list, because the pane
# still inherits everything from the serve:
#   HOME       — the throwaway agent login built by prepare-claude-drive.sh
#                (reused verbatim; that script owns the isolated credentials).
#   GADAK_HOME — NOT the frozen demo home the terminal-claude league uses.
#                That home rejects every write (origin.ErrWorkspaceFrozen,
#                internal/origin/origin.go:94-100) and this take needs the
#                agent to really transition an issue, so this script builds a
#                standalone workspace instead: `gadak init --standalone`, seed
#                issues only, issuetap in-process on loopback — writable,
#                credential-free, and fictional either way (MEDIA.md).
#   cwd        — $AGENT_HOME (the PTY opens in the serve's $HOME,
#                internal/server/terminal.go:343-347); Claude prints its cwd
#                on boot, and the operator's real home is exactly what
#                MEDIA.md keeps out of a public frame.
#   CLAUDE_*   — unset by the sourced env (prepare-claude-drive.sh), so the
#                TUI opens clean.
#
# Two env patches after prepare, both because Claude Code re-parents its
# children's environment from settings.json rather than the PTY:
#   .claude/settings.json env.GADAK_HOME → the standalone home (prepare wrote
#     the frozen demo home there; left alone, every `gadak` the agent runs
#     would read the frozen mirror and its write would fail on camera), and
#   .claude.json trust for $AGENT_HOME (the PTY's cwd — without it the take
#     burns its first ninety seconds on "Is this a project you trust?").
#
# Usage:
#   bash e2e/demo/record-hero-desk.sh                 # prepare + live rehearsal
#   bash e2e/demo/record-hero-desk.sh --dry-run       # no model call — the
#                     # tuning loop for prompt/timing/grouping choreography
#   bash e2e/demo/record-hero-desk.sh --skip-prepare  # reuse the agent HOME
#   bash e2e/demo/record-hero-desk.sh --serve-only    # seed the fixture and
#                     # hold the serve, no take — what the phone rig attaches
#                     # to, and what a two-camera shoot runs first so both
#                     # cameras watch one mirror
#   bash e2e/demo/record-hero-desk.sh --frames-only [video]
#                     # re-extract the review keyframes from a finished take
#                     # (and GADAK_HERO_LEAD to re-time them) without a new
#                     # live run — a mistimed frame never costs a live call
#
# Live model. Requires ffmpeg/ffprobe, Playwright chromium, and a Claude Code
# login. Not part of `make media` — same reason as the other live-agent tapes.
#
# Run `bash tools/tapes/prepare-claude-drive.sh --clean` afterwards — the
# isolated HOME holds a 0600 copy of this machine's credentials.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# ANTHROPIC_* must not reach the PTY. An operator shell (or an agent harness)
# that routes Claude through a proxy exports these; the PTY inherits them, and
# the "live Claude" on camera silently becomes a different login on a
# different endpoint — measured: a take recorded glm-5.3 through a proxy with
# the throwaway claude.ai login sitting unused in the HOME. ENV_SH scrubs
# CLAUDE_*; this scrubs the other half, the same way.
for v in $(env | sed -n 's/^\(ANTHROPIC[A-Z_]*\)=.*/\1/p'); do unset "$v"; done

# Node pinned to .nvmrc — the recording runs the same major the suite and CI
# do (the local-24/CI-20 gap has hidden defects before; CLAUDE.md). Best
# effort: no matching nvm install means fall through to whatever is on PATH.
if [[ -f "$ROOT/.nvmrc" && -d "$HOME/.nvm/versions/node" ]]; then
  want="$(tr -d '[:space:]' <"$ROOT/.nvmrc")"
  have="$(node --version 2>/dev/null || echo none)"
  if [[ "$have" != v"${want}".* ]]; then
    pin="$(ls -d "$HOME"/.nvm/versions/node/v"${want}".* 2>/dev/null | tail -1 || true)"
    if [[ -n "$pin" ]]; then
      export PATH="$pin/bin:$PATH"
      echo "record-hero-desk: node pinned to ${pin##*/} (.nvmrc wants ${want})"
    fi
  fi
fi

MODE_LIVE=""
MODE_DRY=""
SKIP_PREPARE=""
FRAMES_ONLY=""
SERVE_ONLY=""
# A loop rather than the old if/elif chain: --serve-only and --skip-prepare
# are the one pair a caller genuinely combines (hold the serve for the phone
# camera without rebuilding the agent HOME). --frames-only still reads $2 as
# its video path, so it stays first when it is used.
for arg in "$@"; do
  case "$arg" in
    --dry-run) MODE_DRY=1 ;;
    --skip-prepare) SKIP_PREPARE=1 ;;
    --frames-only) FRAMES_ONLY=1 ;;
    --serve-only) SERVE_ONLY=1 ;;
  esac
done

# This round's assignment (7877/7891/7892/7795 belong to other leagues).
# Inside serveProbePorts()' 7777-7797 sweep (cmd/gadak/views.go) so gadak in
# the pane still finds the serve.
PORT="${GADAK_E2E_PORT:-7794}"
export GADAK_E2E_PORT="$PORT"

DRIVE_ROOT="/private/tmp/gadak-claude-drive"
AGENT_HOME="$DRIVE_ROOT/agent"
GADAK_BIN="$DRIVE_ROOT/bin/gadak"
ENV_SH="$ROOT/tools/tapes/.tmp/env-claude-drive.sh"

HERO_ROOT="/private/tmp/gadak-hero-desk"
HERO_HOME="$HERO_ROOT/home"
OUT="$ROOT/e2e/.tmp/hero-desk"
RESULTS="$ROOT/e2e/.tmp/test-results-hero-desk"
SCRATCH="$ROOT/scratch/hero"
MAX_TAKES="${GADAK_HERO_MAX_TAKES:-2}"
TARGET_KEY="${GADAK_HERO_TARGET:-STD-7}"
# Seconds between the video's first frame and the spec's 'start' mark —
# context creation to first navigation. Retunable for --frames-only.
LEAD="${GADAK_HERO_LEAD:-2.0}"

mkdir -p "$OUT" "$SCRATCH"

# ── Keyframe extraction ────────────────────────────────────────────────────
# The proof file carries epoch marks; the video starts LEAD seconds before
# the first one. Extraction reads only finished files, so it re-runs freely.
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

# Four review keyframes: the handoff, the walked-away list, the replay, done.
# Dry-run vs live marks are told apart by what the proof actually holds, so
# --frames-only needs no mode flag and cannot mis-read a take.
names = [
    ("bit1_echo_seen" if "bit1_echo_seen" in marks else "bit1_prompt_sent", "bit1-prompt.png"),
    ("bit1_list_frame", "post-detach.png"),
    ("bit6_replay_seen", "reattach-replay.png"),
    ("bit6_end_frame" if "bit6_end_frame" in marks else "bit6_done_frame", "board-done.png"),
]
os.makedirs(frames_dir, exist_ok=True)
missing = [m for m, _ in names if m not in marks]
if missing:
    sys.exit(f"proof is missing marks: {missing}")
dur = float(subprocess.run(
    ["ffprobe", "-v", "error", "-show_entries", "format=duration",
     "-of", "default=noprint_wrappers=1:nokey=1", video],
    capture_output=True, text=True, check=True).stdout.strip())
for mark, out in names:
    t = (marks[mark] - t0) / 1000.0 + lead
    # The video ends with the test; a mark near the tail can outrun it.
    t = max(0.0, min(t, dur - 0.25))
    dest = os.path.join(frames_dir, out)
    r = subprocess.run(["ffmpeg", "-hide_banner", "-loglevel", "error",
                        "-ss", f"{t:.2f}", "-i", video,
                        "-frames:v", "1", "-y", dest])
    if r.returncode != 0 or not os.path.exists(dest):
        sys.exit(f"ffmpeg failed for {out} at t={t:.2f}s")
    print(f"frame {out}  t={t:.2f}s")
PY
}

if [[ -n "$FRAMES_ONLY" ]]; then
  video="${2:-$SCRATCH/desk-take1.webm}"
  proof="$(ls -t "$OUT"/proof-take-*.jsonl 2>/dev/null | head -1 || true)"
  [[ -f "$video" ]] || { echo "record-hero-desk: no video at $video" >&2; exit 1; }
  [[ -n "$proof" && -f "$proof" ]] || { echo "record-hero-desk: no proof file in $OUT" >&2; exit 1; }
  echo "record-hero-desk: re-extracting frames from ${video} (${proof})"
  extract_frames "$video" "$proof"
  echo "record-hero-desk: frames in $SCRATCH/frames"
  exit 0
fi

command -v ffmpeg >/dev/null || { echo "record-hero-desk: ffmpeg required" >&2; exit 1; }
command -v ffprobe >/dev/null || { echo "record-hero-desk: ffprobe required" >&2; exit 1; }
[[ -x node_modules/.bin/playwright ]] || { echo "record-hero-desk: playwright missing (npm ci)" >&2; exit 1; }
if [[ -z "$MODE_DRY" && -z "$SERVE_ONLY" ]]; then
  command -v claude >/dev/null || { echo "record-hero-desk: claude CLI required (or --dry-run)" >&2; exit 1; }
fi

if [[ -z "$SKIP_PREPARE" ]]; then
  echo "record-hero-desk: preparing isolated agent HOME (reused from the terminal-claude league)…"
  bash tools/tapes/prepare-claude-drive.sh
fi
[[ -f "$ENV_SH" ]] || { echo "record-hero-desk: $ENV_SH missing — run without --skip-prepare" >&2; exit 1; }
[[ -x "$GADAK_BIN" ]] || { echo "record-hero-desk: $GADAK_BIN missing — run without --skip-prepare" >&2; exit 1; }

# Claude Code re-parents its Bash children onto settings.json's env block, so
# the GADAK_HOME the PTY inherited is overridden there. Point it at the
# standalone home or every write the agent attempts fails as frozen.
python3 - "$AGENT_HOME/.claude/settings.json" "$HERO_HOME" <<'PY'
import json, sys

path, home = sys.argv[1], sys.argv[2]
cfg = json.load(open(path))
cfg.setdefault("env", {})["GADAK_HOME"] = home
json.dump(cfg, open(path, "w"), indent=2)
PY

# Trust the PTY's cwd ($AGENT_HOME — the serve's $HOME; the pane starts
# there, internal/server/terminal.go:343-347). Additive, same shape the
# sibling script uses for its own extra trust entry.
python3 - "$AGENT_HOME/.claude.json" "$AGENT_HOME" <<'PY'
import json, sys

path, cwd = sys.argv[1], sys.argv[2]
cfg = json.load(open(path))
projects = cfg.setdefault("projects", {})
projects.setdefault(cwd, {}).update({
    "hasTrustDialogAccepted": True,
    "hasCompletedProjectOnboarding": True,
    "projectOnboardingSeenCount": 3,
    "allowedTools": ["Bash", "Write", "Read", "Edit", "Glob", "Grep"],
    "history": [],
    "mcpServers": {},
})
json.dump(cfg, open(path, "w"), indent=2)
PY

echo "record-hero-desk: building web UI…"
npm run build >"$OUT/build.log" 2>&1 || { tail -40 "$OUT/build.log" >&2; exit 1; }

# Nothing else may be on this port — a client on the same serve eats one-shot
# handoffs (measured 2026-08-26). Name what is there rather than killing it.
busy="$(lsof -nP -iTCP:"$PORT" 2>/dev/null | awk 'NR > 1' || true)"
if [[ -n "$busy" ]]; then
  echo "record-hero-desk: :${PORT} is in use — close these, or set GADAK_E2E_PORT" >&2
  printf '%s\n' "$busy" >&2
  exit 1
fi

# Every seed command goes through this wrapper — a VAR=value prefix cannot
# travel inside a variable or array (bash executes it as the program name).
gh() { GADAK_HOME="$HERO_HOME" "$GADAK_BIN" "$@"; }

# ── The fixture: a standalone workspace, seeded, no real data ───────────────
# Re-built before every take, so a retry starts from the same board the take
# contract describes (STD-7 open again, no leftover agent comments).
# The standalone workflow is To Do / In Progress / Done — that is the whole
# transition graph the origin offers (probed: "In Review" exists as a name,
# not as a step), so the seed uses those three.
seed_hero_home() {
  rm -rf "$HERO_HOME"
  mkdir -p "$HERO_HOME"
  gh init --standalone --json >/dev/null

  # A lived-in backlog: fifteen issues, mixed type/priority/labels, spread
  # across the three statuses. Anything a viewer pauses on should look like
  # work, not filler.
  gh create "Filter bar loses focus after bulk edit" --project STD --type Bug --priority High --label web >/dev/null
  gh create "Ship the 0.19 hero clip storyboard" --project STD --type Task --priority Medium --label media >/dev/null
  gh create "Standalone workspace onboarding copy" --project STD --type Story --priority Low >/dev/null
  gh create "Mirror sync retries too eagerly on flaky origin" --project STD --type Bug --priority High --label sync >/dev/null
  gh create "Keyboard help overlay is two releases stale" --project STD --type Task --priority Low --label docs >/dev/null
  gh create "Terminal pane resize on window restore" --project STD --type Bug --priority Medium --label web --label terminal >/dev/null
  # The target. Self-contained on purpose: the agent must be able to close it
  # from the issue body alone, with no context the clip cannot show.
  gh create "Weekly digest counts issues closed by bots twice" --project STD --type Bug --priority High --label digest \
    -m "Digest double-counts an issue when a bot transitions it and a human re-opens and closes it again the same week. Reproduced on the standalone workspace. Decide the counting rule (count first close, or count distinct issues), write it down, and close this when the rule is agreed." >/dev/null
  gh create "Saved views list needs an empty state" --project STD --type Task --priority Low --label web >/dev/null
  gh create "Search highlight leaks into detail pane" --project STD --type Bug --priority Medium --label web >/dev/null
  gh create "Document the terminal grace window" --project STD --type Task --priority Medium --label docs --label terminal >/dev/null
  gh create "Dashboard cards drift on narrow windows" --project STD --type Bug --priority Low --label web >/dev/null
  gh create "Weekly triage sweep checklist" --project STD --type Task --priority Medium --label triage >/dev/null
  gh create "Issue detail loads metadata it never renders" --project STD --type Task --priority Low --label perf >/dev/null
  gh create "Comment editor eats the first keystroke" --project STD --type Bug --priority Medium --label web >/dev/null
  gh create "Pairing code flow on fresh browsers" --project STD --type Story --priority Low >/dev/null

  # Spread: five in progress, three done, the rest to do. The target opens in
  # To Do — it is the one issue handed over, straight from the backlog.
  local key
  local inprog=(STD-1 STD-4 STD-6 STD-9 STD-14)
  local done=(STD-2 STD-5 STD-11)
  for key in "${inprog[@]}"; do gh transition "$key" inprogress >/dev/null; done
  for key in "${done[@]}"; do gh transition "$key" done >/dev/null; done

  gh sync >/dev/null
}

start_serve() {
  echo "record-hero-desk: serving $HERO_HOME on 127.0.0.1:${PORT}"
  # The PTY the pane opens is a child of this process, so everything the
  # agent sees is set here: the tapes' env, with GADAK_HOME pointed at the
  # standalone home (the serve and the agent must watch the same mirror, or
  # the clip is two unrelated halves).
  bash -c "set -a; . '$ENV_SH'; set +a; export GADAK_HOME='$HERO_HOME'; exec '$GADAK_BIN' serve \
      --addr '127.0.0.1:${PORT}' --static '$ROOT/dist/app' --no-open --no-sync" \
    >"$OUT/serve.log" 2>&1 &
  echo $! >"$OUT/serve.pid"
  local i
  for i in $(seq 1 60); do
    if curl -sf "http://127.0.0.1:${PORT}/healthz" >/dev/null; then
      echo "record-hero-desk: healthz ok"
      return 0
    fi
    sleep 0.25
  done
  echo "record-hero-desk: serve did not become healthy" >&2
  cat "$OUT/serve.log" >&2 || true
  return 1
}

stop_serve() {
  if [[ -f "$OUT/serve.pid" ]]; then
    kill "$(cat "$OUT/serve.pid")" 2>/dev/null || true
    rm -f "$OUT/serve.pid"
  fi
  local pids
  pids="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true)"
  # shellcheck disable=SC2086
  [[ -n "$pids" ]] && kill $pids 2>/dev/null || true
}
trap stop_serve EXIT

# The result contract, checked against the workspace rather than the video —
# a take that only *looks* right is the failure mode a live model produces.
# Display names prove nothing (CLAUDE.md): status_category is the gate, the
# video is the witness.
validate_take() {
  local cat
  cat="$(GADAK_HOME="$HERO_HOME" "$GADAK_BIN" sql --no-header \
    "SELECT status_category FROM issues_full WHERE key='${TARGET_KEY}'" 2>/dev/null || true)"
  if [[ "$cat" != "done" ]]; then
    echo "record-hero-desk: ${TARGET_KEY} status_category='${cat}' — not done" >&2
    return 1
  fi
  return 0
}

find_video() {
  find "$RESULTS" -type f -name '*.webm' 2>/dev/null | head -1
}

# Empty when live: an empty GADAK_HERO_DRY_RUN is falsy in the spec, and a
# conditional array expansion is the one construct this script avoids for
# /bin/bash 3.2 (empty arrays + set -u).
dry_flag="$MODE_DRY"

# --serve-only: seed the fixture, serve it, and hold. The two-camera take
# needs ONE serve — the desk browser and the phone must watch the same
# mirror and the same session, or the two halves are unrelated footage —
# and this script is the single owner of the fixture (seed_hero_home is the
# board the take contract describes). The phone rig
# (e2e/demo/record-hero-phone.sh) attaches to what this leaves listening
# and never seeds its own.
if [[ -n "$SERVE_ONLY" ]]; then
  seed_hero_home
  rm -f "$HERO_HOME"/local.db "$HERO_HOME"/local.db-wal "$HERO_HOME"/local.db-shm
  start_serve
  echo "record-hero-desk: serve-only — fixture seeded, holding 127.0.0.1:${PORT} (Ctrl-C to stop)"
  echo "record-hero-desk: GADAK_HOME=$HERO_HOME"
  # `wait` on the serve pid would return on the first trapped signal without
  # the trap having run; a sleep loop keeps EXIT (stop_serve) the only path
  # out, so Ctrl-C never leaves the port held.
  while kill -0 "$(cat "$OUT/serve.pid")" 2>/dev/null; do sleep 1; done
  exit 0
fi

take=1
while (( take <= MAX_TAKES )); do
  echo "record-hero-desk: === take ${take}/${MAX_TAKES} $([[ -n "$MODE_DRY" ]] && echo '(dry-run)') ==="
  rm -rf "$RESULTS"
  seed_hero_home
  # Each take starts clean: visits and search history live in local.db, which
  # the app recreates — deleting it is the whole reset, no parsing needed.
  rm -f "$HERO_HOME"/local.db "$HERO_HOME"/local.db-wal "$HERO_HOME"/local.db-shm
  rm -f "$OUT/proof-take-${take}.jsonl"

  stop_serve
  start_serve

  if env GADAK_MEDIA=1 GADAK_HERO_PROOF="$OUT/proof-take-${take}.jsonl" \
      GADAK_HERO_TARGET="$TARGET_KEY" GADAK_HERO_DRY_RUN="$dry_flag" \
      ./node_modules/.bin/playwright test \
      --config e2e/demo/hero-desk.config.ts 2>&1 | tee "$OUT/take-${take}.log"; then
    if [[ -n "$MODE_DRY" ]]; then
      # The dry-run contract is the choreography: the proof file must carry
      # both away-wait snapshots and the post-reattach one.
      if ! grep -q 'proof_post_detach' "$OUT/proof-take-${take}.jsonl" \
          || ! grep -q 'proof_pre_reattach' "$OUT/proof-take-${take}.jsonl" \
          || ! grep -q 'proof_post_reattach' "$OUT/proof-take-${take}.jsonl"; then
        echo "record-hero-desk: dry-run proof incomplete" >&2
      elif [[ -z "$(find_video)" ]]; then
        echo "record-hero-desk: dry-run produced no video" >&2
      else
        echo "record-hero-desk: dry-run take ${take} holds the choreography"
        break
      fi
    elif validate_take; then
      echo "record-hero-desk: take ${take} holds the contract"
      break
    fi
  fi
  echo "record-hero-desk: take ${take} rejected"
  take=$(( take + 1 ))
done

if (( take > MAX_TAKES )); then
  echo "record-hero-desk: no take held the contract in ${MAX_TAKES} tries" >&2
  exit 1
fi

stop_serve

video="$(find_video)"
if [[ -z "$video" ]]; then
  echo "record-hero-desk: take passed but no video in $RESULTS" >&2
  exit 1
fi
cp "$video" "$SCRATCH/desk-take1.webm"
video="$SCRATCH/desk-take1.webm"

echo "record-hero-desk: take archived at $video"
ffprobe -v error -show_entries format=duration \
  -show_entries stream=codec_name,width,height -of default=noprint_wrappers=1 "$video"

echo "record-hero-desk: extracting review keyframes…"
extract_frames "$video" "$OUT/proof-take-${take}.jsonl"
echo "record-hero-desk: frames in $SCRATCH/frames"

echo "record-hero-desk: remember — bash tools/tapes/prepare-claude-drive.sh --clean"
