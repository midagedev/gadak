#!/usr/bin/env bash
# Isolated HOME + frozen GADAK_HOME for tools/tapes/claude-drive.tape.
#
# Sibling of prepare-agent.sh (credential copy, onboarding skip, CLAUDE_*
# unset) and prepare.sh (demo.db seed, fake credential, env.sh). The take
# runs a live Claude Code session that must drive the SAME GADAK_HOME the
# serve tab is watching — so unlike pin-demo.sh we do NOT copy the mirror
# into $HOME/.gadak.
#
# Usage:
#   bash tools/tapes/prepare-claude-drive.sh           # seed + skill install
#   bash tools/tapes/prepare-claude-drive.sh --clean   # drop the throwaway dir
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MAIN_SKILL="/Users/hckim/repo/gadak/skills/gadak/SKILL.md"
DRIVE_ROOT="/private/tmp/gadak-claude-drive"
AGENT_HOME="$DRIVE_ROOT/agent"
WORKSPACE="$AGENT_HOME/workspace"
GADAK_HOME_DIR="$DRIVE_ROOT/gadak-home"
BIN_DIR="$DRIVE_ROOT/bin"
BIN="$BIN_DIR/gadak"
TMP="$ROOT/tools/tapes/.tmp"
ENV_SH="$TMP/env-claude-drive.sh"
REAL_HOME="${REAL_HOME:-$HOME}"
PORT="${GADAK_E2E_PORT:-7889}"

if [[ "${1:-}" == "--clean" ]]; then
  rm -rf "$DRIVE_ROOT"
  echo "[claude-drive] removed $DRIVE_ROOT (credential copy included)"
  exit 0
fi

if [[ ! -f "$REAL_HOME/.claude/.credentials.json" ]]; then
  echo "prepare-claude-drive: no Claude Code credentials at $REAL_HOME/.claude/.credentials.json" >&2
  echo "  log in with \`claude\` first — this take cannot run without it." >&2
  exit 1
fi

if [[ ! -f "$MAIN_SKILL" ]]; then
  echo "prepare-claude-drive: main-tree skill missing at $MAIN_SKILL" >&2
  exit 1
fi

# Worktree SKILL.md is the 4d56754 copy and lacks Dashboards. skill install
# embeds whatever is in this tree at go-build time, so copy first.
echo "[claude-drive] copying $MAIN_SKILL → $ROOT/skills/gadak/SKILL.md"
cp -f "$MAIN_SKILL" "$ROOT/skills/gadak/SKILL.md"
if ! grep -q '## Dashboards (agent-authored walls)' "$ROOT/skills/gadak/SKILL.md"; then
  echo "prepare-claude-drive: copied skill has no Dashboards section" >&2
  exit 1
fi

rm -rf "$WORKSPACE"
mkdir -p "$TMP" "$BIN_DIR" "$GADAK_HOME_DIR" "$AGENT_HOME/.claude" "$WORKSPACE"
chmod 700 "$AGENT_HOME"

echo "[claude-drive] building gadak → $BIN"
CGO_ENABLED=0 go build -trimpath -o "$BIN" "$ROOT/cmd/gadak"
# Playwright freezePromoHome looks for e2e/.tmp/gadak; keep a copy there too
# so a leftover helper call does not pick up a stale binary.
mkdir -p "$ROOT/e2e/.tmp"
cp -f "$BIN" "$ROOT/e2e/.tmp/gadak"
chmod 755 "$ROOT/e2e/.tmp/gadak"

echo "[claude-drive] seeding GADAK_HOME from examples/demo.db"
cp -f "$ROOT/examples/demo.db" "$GADAK_HOME_DIR/gadak.db"
rm -f "$GADAK_HOME_DIR/gadak.db-wal" "$GADAK_HOME_DIR/gadak.db-shm"
rm -f "$GADAK_HOME_DIR/local.db" "$GADAK_HOME_DIR/local.db-wal" "$GADAK_HOME_DIR/local.db-shm"
sqlite3 "$GADAK_HOME_DIR/gadak.db" "UPDATE sync_state SET watermark = strftime('%Y-%m-%dT%H:%M:%S.000Z','now'),
                                     last_full_sync_at = strftime('%Y-%m-%dT%H:%M:%S.000Z','now'),
                                     last_error = NULL;
               UPDATE items SET synced_at = strftime('%Y-%m-%dT%H:%M:%S.000Z','now');
               UPDATE sources SET synced_at = strftime('%Y-%m-%dT%H:%M:%S.000Z','now');"

cat >"$GADAK_HOME_DIR/config.json" <<'EOF'
{
  "site": "https://nimbus.example.com",
  "email": "dana@example.com",
  "token": "demo-fake-token",
  "projects": ["NMB", "NMA", "NMS"],
  "frozen": true
}
EOF

echo "[claude-drive] gadak config set frozen true"
GADAK_HOME="$GADAK_HOME_DIR" "$BIN" config set frozen true >/dev/null
status_out="$(GADAK_HOME="$GADAK_HOME_DIR" "$BIN" status || true)"
if ! printf '%s\n' "$status_out" | grep -Ei '^frozen[[:space:]]+true\b' >/dev/null; then
  echo "prepare-claude-drive: freeze did not stick; status said:" >&2
  printf '%s\n' "$status_out" >&2
  exit 1
fi
echo "[claude-drive] frozen confirmed"

install -m 600 "$REAL_HOME/.claude/.credentials.json" "$AGENT_HOME/.claude/.credentials.json"

python3 - "$REAL_HOME" "$AGENT_HOME" "$WORKSPACE" <<'PY'
import json, sys, os

real_home, agent_home, workspace = sys.argv[1], sys.argv[2], sys.argv[3]
src = json.load(open(os.path.join(real_home, ".claude.json")))
KEEP = ("accountUuid", "organizationUuid", "billingType", "seatTier")
account = {k: v for k, v in src.get("oauthAccount", {}).items() if k in KEEP}
cfg = {"oauthAccount": account}
if "userID" in src:
    cfg["userID"] = src["userID"]
cfg.update({
    "hasCompletedOnboarding": True,
    "hasAvailableSubscription": src.get("hasAvailableSubscription", True),
    "installMethod": "native",
    "autoUpdates": False,
    "numStartups": 20,
    "theme": "dark",
    "projects": {
        workspace: {
            "hasTrustDialogAccepted": True,
            "hasCompletedProjectOnboarding": True,
            "projectOnboardingSeenCount": 3,
            "hasClaudeMdExternalIncludesApproved": True,
            "allowedTools": ["Bash", "Write", "Read", "Edit", "Glob", "Grep"],
            "history": [],
            "mcpServers": {},
        }
    },
})
json.dump(cfg, open(os.path.join(agent_home, ".claude.json"), "w"), indent=2)
PY

# Bash/Write are required: the scene is CLI config set + authoring HTML.
# Task/Web* denied so a take cannot spawn an Explore subagent or leave the
# machine (the mcp.tape deny list, minus the tools this clip actually needs).
# defaultMode is "default", not bypassPermissions: take 1 of this clip showed
# a "Bypass Permissions mode / Yes, I accept" dialog that swallowed the user
# prompt into the shell. mcp.tape auto-approves by listing tools under allow.
cat >"$AGENT_HOME/.claude/settings.json" <<EOF
{
  "model": "claude-sonnet-5",
  "permissions": {
    "allow": ["Bash", "Write", "Read", "Edit", "Glob", "Grep", "Skill"],
    "deny": ["Task", "WebFetch", "WebSearch"],
    "defaultMode": "default"
  },
  "includeCoAuthoredBy": false,
  "disableClaudeAiConnectors": true,
  "env": {
    "DISABLE_AUTOUPDATER": "1",
    "DISABLE_TELEMETRY": "1",
    "MAX_THINKING_TOKENS": "0",
    "GADAK_HOME": "$GADAK_HOME_DIR",
    "GADAK_NO_OPEN": "1"
  }
}
EOF

echo "[claude-drive] gadak skill install (HOME=$AGENT_HOME)"
# User-scope: $HOME/.claude/skills/gadak/SKILL.md (isolated HOME, not ~/.claude).
HOME="$AGENT_HOME" GADAK_HOME="$GADAK_HOME_DIR" "$BIN" skill install
# Project-scope under the recording cwd only — never --project from the
# repo root (that would write worktree .claude/skills/, outside the whitelist).
(
  cd "$WORKSPACE"
  HOME="$AGENT_HOME" GADAK_HOME="$GADAK_HOME_DIR" "$BIN" skill install --project
)

skill_user="$AGENT_HOME/.claude/skills/gadak/SKILL.md"
skill_proj="$WORKSPACE/.claude/skills/gadak/SKILL.md"
for p in "$skill_user" "$skill_proj"; do
  if [[ ! -f "$p" ]]; then
    echo "prepare-claude-drive: skill not installed at $p" >&2
    exit 1
  fi
  if ! grep -q '## Dashboards (agent-authored walls)' "$p"; then
    echo "prepare-claude-drive: $p missing Dashboards section (stale embed?)" >&2
    exit 1
  fi
  echo "[claude-drive] skill ok: $p"
done

# CLAUDE on PATH: capture the operator's claude before HOME changes.
CLAUDE_BIN="$(command -v claude || true)"
if [[ -z "$CLAUDE_BIN" ]]; then
  echo "prepare-claude-drive: claude not on PATH" >&2
  exit 1
fi
CLAUDE_DIR="$(cd "$(dirname "$CLAUDE_BIN")" && pwd)"

cat >"$ENV_SH" <<EOF
# Generated by tools/tapes/prepare-claude-drive.sh — do not edit.
export GADAK_HOME='$GADAK_HOME_DIR'
export GADAK_NO_OPEN=1
export PATH='$BIN_DIR':'$CLAUDE_DIR':"\$PATH"
export HOME='$AGENT_HOME'
export PS1='\$ '
export PROMPT_COMMAND=
export DISABLE_AUTOUPDATER=1
unset HISTFILE
unset FORCE_COLOR
unset NO_COLOR
for v in \$(env | sed -n 's/^\(CLAUDE[A-Z_]*\)=.*/\1/p'); do unset "\$v"; done
cd '$WORKSPACE'
EOF

cat >"$DRIVE_ROOT/strip-oauth.py" <<'PY'
import json, os
p = os.path.expanduser("~/.claude.json")
d = json.load(open(p))
acc = d.get("oauthAccount") or {}
keep = ("accountUuid", "organizationUuid", "billingType", "seatTier")
d["oauthAccount"] = {k: acc[k] for k in keep if k in acc}
d["hasAvailableSubscription"] = True
json.dump(d, open(p, "w"), indent=2)
PY

cat >"$DRIVE_ROOT/mark-epoch.py" <<'PY'
import time, os
path = os.environ.get("CLAUDE_DRIVE_EPOCH", "/private/tmp/gadak-claude-drive/vhs-show-epoch")
os.makedirs(os.path.dirname(path), exist_ok=True)
with open(path, "w") as f:
    f.write("%.6f\n" % time.time())
PY

cat >"$DRIVE_ROOT/port" <<EOF
$PORT
EOF

echo "[claude-drive] ready"
echo "[claude-drive] GADAK_HOME=$GADAK_HOME_DIR"
echo "[claude-drive] HOME(agent)=$AGENT_HOME"
echo "[claude-drive] workspace=$WORKSPACE"
echo "[claude-drive] bin=$BIN"
echo "[claude-drive] source $ENV_SH before the tape"
echo "[claude-drive] port=$PORT"
echo "[claude-drive] remember: bash tools/tapes/prepare-claude-drive.sh --clean when recording is done"
