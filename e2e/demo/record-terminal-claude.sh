#!/usr/bin/env bash
# Live-Claude terminal-pane hero → docs/media/terminal-hero.mp4 (README
# and landing since 0.20; GADAK_TERMINAL_OUT redirects a scratch take).
#
# Unlike record-claude-drive.sh this is NOT a composite: there is no VHS half.
# Claude Code runs inside gadak's own terminal pane, so one Playwright
# recording of the app window holds both the agent and the board it moves.
#
# What this script owns, and why Playwright's webServer block cannot:
#   HOME       — the throwaway agent login built by prepare-claude-drive.sh.
#                The PTY inherits it, so `claude` in the pane runs as that
#                login and not as the operator (whose shell history, projects
#                and real paths would otherwise be one keystroke from frame).
#   GADAK_HOME — the frozen demo workspace the serve tab is watching. The
#                agent and the tab must be looking at the same mirror or the
#                clip is two unrelated halves.
#   cwd        — /private/tmp/…/workspace. Claude prints its cwd on boot; the
#                operator's home directory is exactly what MEDIA.md's privacy
#                rule keeps out of a public frame.
#   CLAUDE_*   — unset. This repo's own session exports them, and an inherited
#                CLAUDE_CODE_CHILD_SESSION opens the TUI with a banner.
#
# Live model. Requires ffmpeg, Playwright chromium, and a Claude Code login.
# Not part of `make media` — same reason as media-mcp and claude-drive.
#
# Usage:
#   bash e2e/demo/record-terminal-claude.sh                # prepare + record
#   bash e2e/demo/record-terminal-claude.sh --skip-prepare # reuse the home
#
# Run `bash tools/tapes/prepare-claude-drive.sh --clean` afterwards — the
# isolated HOME holds a 0600 copy of this machine's credentials.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

SKIP_PREPARE=""
if [[ "${1:-}" == "--skip-prepare" ]]; then
  SKIP_PREPARE=1
fi

# Inside serveProbePorts()' 7777-7797 sweep (cmd/gadak/views.go) on purpose,
# and off the e2e suite's 7877 on purpose. `views open` hands its hash through
# a one-shot file that the first poller to ask consumes, so a desktop window
# or a stray tab on the same serve silently eats it and the list never moves
# (measured 2026-08-26: written and gone in 240ms, three failed takes).
PORT="${GADAK_E2E_PORT:-7794}"
export GADAK_E2E_PORT="$PORT"

DRIVE_ROOT="/private/tmp/gadak-claude-drive"
AGENT_HOME="$DRIVE_ROOT/agent"
WORKSPACE="$AGENT_HOME/workspace"
GADAK_HOME_DIR="$DRIVE_ROOT/gadak-home"
BIN="$DRIVE_ROOT/bin/gadak"
# The take's workspace is the seeded mirror migrated onto the built-in
# tracker: the claim beat is a write, and the fixture's Jira credential
# cannot land one. Rebuilt per take (see the loop), served by name, and
# inherited by the pane shells through GADAK_WORKSPACE on the serve's env.
WS="nimbus"
ENV_SH="$ROOT/tools/tapes/.tmp/env-claude-drive.sh"
OUT="$ROOT/e2e/.tmp/terminal-claude"
RESULTS="$ROOT/e2e/.tmp/test-results-terminal-claude"
MAX_TAKES="${TERMINAL_CLAUDE_MAX_TAKES:-3}"

command -v ffmpeg >/dev/null || { echo "record-terminal-claude: ffmpeg required" >&2; exit 1; }
command -v ffprobe >/dev/null || { echo "record-terminal-claude: ffprobe required" >&2; exit 1; }
command -v claude >/dev/null || { echo "record-terminal-claude: claude CLI required" >&2; exit 1; }
[[ -x node_modules/.bin/playwright ]] || { echo "record-terminal-claude: playwright missing (npm ci)" >&2; exit 1; }

mkdir -p "$OUT"

if [[ -z "$SKIP_PREPARE" ]]; then
  echo "record-terminal-claude: preparing isolated HOME + frozen GADAK_HOME…"
  bash tools/tapes/prepare-claude-drive.sh
fi
[[ -f "$ENV_SH" ]] || { echo "record-terminal-claude: $ENV_SH missing — run without --skip-prepare" >&2; exit 1; }
[[ -x "$BIN" ]] || { echo "record-terminal-claude: $BIN missing — run without --skip-prepare" >&2; exit 1; }

# prepare-claude-drive.sh trusts the *tape's* cwd (its agent workspace), but
# the pane does not open there. Where it opens has moved once already —
# GADAK_HOME first, then the PTY's $HOME (the agent home; GDK-1159's
# claude-pane-env.sh already says "the fixture repo lives at $HOME (the PTY
# cwd)") — and each move cost three takes to "Is this a project you
# trust?" (measured 2026-09-02: 3/3 rejected, 90s each, the dialog in
# every frame). So trust both candidates rather than guess which one this
# build starts in. Additive — the workspace entry the tapes rely on is left
# exactly as prepare wrote it.
python3 - "$AGENT_HOME/.claude.json" "$GADAK_HOME_DIR" "$GADAK_HOME_DIR/profiles/$WS" "$AGENT_HOME" <<'PY'
import json, sys

path, cwds = sys.argv[1], sys.argv[2:]
cfg = json.load(open(path))
projects = cfg.setdefault("projects", {})
for cwd in cwds:
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

echo "record-terminal-claude: building web UI…"
npm run build >"$OUT/build.log" 2>&1 || { tail -40 "$OUT/build.log" >&2; exit 1; }

# Nothing else may be on this port — not a listener, and not a *client*
# either. Any other gadak UI watching this serve would eat the one-shot focus
# the agent writes and the list would never move. Name what is there rather
# than killing it: which windows the operator has open is not this script's
# call. (awk counts, so an lsof with no matches is not a pipefail exit.)
busy="$(lsof -nP -iTCP:"$PORT" 2>/dev/null | awk 'NR > 1' || true)"
if [[ -n "$busy" ]]; then
  echo "record-terminal-claude: :${PORT} is in use — close these, or set GADAK_E2E_PORT" >&2
  printf '%s\n' "$busy" >&2
  exit 1
fi

start_serve() {
  echo "record-terminal-claude: serving $GADAK_HOME_DIR on 127.0.0.1:${PORT}"
  # The PTY the pane opens is a child of this process, so everything the
  # agent sees is set here — by sourcing the same env the tapes use, plus
  # the lines claude-pane-env.sh learned for the hero (GDK-1159): the tape
  # env sets HOME but not the shell, and the pane runs $SHELL — the
  # operator's zsh, whose ZDOTDIR rc files then print their own paths into
  # the first frame (measured 2026-09-02: "/Users/…/.zshenv: no such file"
  # above a starship prompt). /bin/sh with the e2e prompt fixture is a bare
  # `$`; NODE_EXTRA_CA_CERTS is the pair of mkcert warnings at claude boot.
  bash -c "set -a; . '$ENV_SH'; set +a; \
      export SHELL=/bin/sh ENV='$ROOT/e2e/demo/prompt.sh' GADAK_WORKSPACE='$WS'; unset ZDOTDIR NODE_EXTRA_CA_CERTS; \
      exec '$BIN' serve \
      --addr '127.0.0.1:${PORT}' --static '$ROOT/dist/app' --no-open --no-sync" \
    >"$OUT/serve.log" 2>&1 &
  echo $! >"$OUT/serve.pid"
  local i
  for i in $(seq 1 60); do
    if curl -sf "http://127.0.0.1:${PORT}/healthz" >/dev/null; then
      echo "record-terminal-claude: healthz ok"
      return 0
    fi
    sleep 0.25
  done
  echo "record-terminal-claude: serve did not become healthy" >&2
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

# The result contract, checked against the workspace rather than the video: a
# take that only *looks* right is the failure mode a live model produces. The
# dashboard is the durable half — the focus hash is consumed on delivery, so
# the spec's own assertion (the list stopped saying what it said at rest) is
# what proves beat 4, and this proves beat 5.
validate_take() {
  local dash
  dash="$(GADAK_HOME="$GADAK_HOME_DIR" "$BIN" --workspace "$WS" dashboards list --json 2>/dev/null || true)"
  if [[ -z "$dash" || "$dash" == "[]" ]]; then
    echo "record-terminal-claude: no dashboard saved in this take" >&2
    return 1
  fi
  return 0
}

take=1
while (( take <= MAX_TAKES )); do
  echo "record-terminal-claude: === take ${take}/${MAX_TAKES} ==="
  rm -rf "$RESULTS"
  # Each take starts clean: a dashboard left by take 1 would pass take 2's
  # contract without the agent doing anything. Dashboards, visits and search
  # history all live in local.db, which the app recreates — deleting it is the
  # whole reset, and it needs no parsing of anything.
  rm -f "$GADAK_HOME_DIR"/local.db "$GADAK_HOME_DIR"/local.db-wal "$GADAK_HOME_DIR"/local.db-shm
  # …and the workspace itself: the claim beat writes to the origin, so a
  # second take would find NMA-140 already taken. A fresh migrate is the
  # reset (about ten seconds), and it leaves the root mirror untouched.
  rm -rf "$GADAK_HOME_DIR/profiles/$WS"
  echo "record-terminal-claude: migrating the mirror onto the built-in tracker (workspace $WS)…"
  GADAK_HOME="$GADAK_HOME_DIR" "$BIN" --workspace "$WS" migrate --from default --skip-attachments >"$OUT/migrate-${take}.log" 2>&1

  stop_serve
  start_serve

  if GADAK_MEDIA=1 ./node_modules/.bin/playwright test \
      --config e2e/demo/terminal-claude.config.ts 2>&1 | tee "$OUT/take-${take}.log"; then
    if validate_take; then
      echo "record-terminal-claude: take ${take} holds the contract"
      break
    fi
  fi
  echo "record-terminal-claude: take ${take} rejected"
  take=$(( take + 1 ))
done

if (( take > MAX_TAKES )); then
  echo "record-terminal-claude: no take held the contract in ${MAX_TAKES} tries" >&2
  exit 1
fi

stop_serve
GADAK_TERMINAL_RESULTS="$RESULTS" bash e2e/demo/export-terminal.sh
echo "record-terminal-claude: remember — bash tools/tapes/prepare-claude-drive.sh --clean"
