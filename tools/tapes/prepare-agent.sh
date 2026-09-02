#!/usr/bin/env bash
# Extra setup for tools/tapes/agent.tape — the take where a real Claude Code
# session answers a question through gadak's MCP server.
#
# Why a separate home: the recording must not show a username, a home path, or
# the operator's own MCP servers, and it must not write anything into the real
# ~/.claude.json. So the tape runs with HOME pointed at a neutral throwaway
# directory (AGENT_HOME below) that holds only what the take needs. Claude Code
# prints that path when a server is registered, which is exactly why it has to
# be anonymous.
#
# Auth: Claude Code reads credentials from $HOME, so the take needs a copy of
# this machine's login. It is copied mode 0600 and removed by
# `tools/tapes/prepare-agent.sh --clean` (run that when you are done recording).
# Without host credentials the tape cannot run; re-record agent.gif from the
# CLI-only fallback in git history instead.
set -euo pipefail

TMP="$(cd "$(dirname "$0")" && pwd)/.tmp"
# Real path, not /tmp: Claude Code keys its trust and onboarding state by the
# resolved directory, and on macOS /tmp is a symlink — registering /tmp/... puts
# the take back in front of the "do you trust this folder?" dialog.
AGENT_HOME="/private/tmp/gadak-demo"
# The session's cwd is a subdirectory, so $AGENT_HOME/.claude/settings.json is
# *user* scope. Permissions in a project-scoped settings file raise a
# "this folder pre-approves N tool permissions" prompt on the first run.
WORKSPACE="$AGENT_HOME/workspace"
REAL_HOME="${REAL_HOME:-$HOME}"

if [[ "${1:-}" == "--clean" ]]; then
  rm -rf "$AGENT_HOME"
  echo "[tapes] removed $AGENT_HOME (credential copy included)"
  exit 0
fi

if [[ ! -f "$REAL_HOME/.claude/.credentials.json" ]]; then
  echo "prepare-agent: no Claude Code credentials at $REAL_HOME/.claude/.credentials.json" >&2
  echo "  log in with \`claude\` first, or re-record the CLI-only agent tape." >&2
  exit 1
fi

rm -rf "$AGENT_HOME"
mkdir -p "$AGENT_HOME/.claude" "$WORKSPACE"
chmod 700 "$AGENT_HOME"

# Credentials, owner-readable only.
install -m 600 "$REAL_HOME/.claude/.credentials.json" "$AGENT_HOME/.claude/.credentials.json"

# Minimal account state: enough to skip onboarding, nothing about real projects.
python3 - "$REAL_HOME" "$AGENT_HOME" "$WORKSPACE" <<'PY'
import json, sys, os

real_home, agent_home, workspace = sys.argv[1], sys.argv[2], sys.argv[3]
src = json.load(open(os.path.join(real_home, ".claude.json")))

# Copy only the account fields the client needs to consider itself logged in.
# Everything human-readable is dropped on purpose: organizationName, displayName
# and emailAddress are the operator's real identity, and this file feeds a
# public recording.
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
    # Per-project state for the recording cwd: trusted and onboarded, so the
    # take opens on the prompt instead of a dialog.
    "projects": {
        workspace: {
            "hasTrustDialogAccepted": True,
            "hasCompletedProjectOnboarding": True,
            "projectOnboardingSeenCount": 3,
            "hasClaudeMdExternalIncludesApproved": True,
            "allowedTools": [],
            "history": [],
            "mcpServers": {},
        }
    },
})
json.dump(cfg, open(os.path.join(agent_home, ".claude.json"), "w"), indent=2)
PY

# Tool permissions live in settings so the recorded command line stays the one a
# reader would type — no --allowedTools noise in the frame.
#
# Task/Glob/Grep are denied, not just Bash: without them a take spawned an
# Explore subagent and searched the *codebase* for the word instead of asking
# the mirror. The only tools left are gadak's, which is the point of the clip.
# 2026-09-02: the harness grew tools the deny list never named — a take
# called Artifact(list) ("listed 1 published artifact") and then told the
# viewer "that call was unnecessary". Anything that is not a gadak tool is
# denied by name, and the list has to grow with the harness.
# MAX_THINKING_TOKENS=0 turns extended thinking off — the wait a viewer reads
# as gadak's latency is the model's, and this question needs no deliberation.
cat >"$AGENT_HOME/.claude/settings.json" <<'EOF'
{
  "model": "claude-sonnet-5",
  "permissions": {
    "allow": ["mcp__gadak__gadak_query", "mcp__gadak__gadak_search", "mcp__gadak__gadak_issue", "mcp__gadak__gadak_status"],
    "deny": ["Bash", "Read", "Write", "Edit", "WebFetch", "WebSearch", "Task", "Glob", "Grep", "Artifact", "ArtifactComments", "ArtifactData", "ArtifactCheck", "Agent", "ToolSearch", "Workflow", "Monitor", "SendMessage", "ScheduleWakeup", "advisor"],
    "defaultMode": "default"
  },
  "includeCoAuthoredBy": false,
  "env": {
    "DISABLE_AUTOUPDATER": "1",
    "DISABLE_TELEMETRY": "1",
    "MAX_THINKING_TOKENS": "0"
  }
}
EOF

# env-agent.sh is sourced (Hidden) by agent.tape.
cat >"$TMP/env-agent.sh" <<EOF
# Generated by tools/tapes/prepare-agent.sh — do not edit.
export GADAK_HOME='$TMP/home'
export PATH='$TMP':"\$PATH"
export HOME='$AGENT_HOME'
export PS1='\$ '
export PROMPT_COMMAND=
export DISABLE_AUTOUPDATER=1
unset HISTFILE
# If the recording is driven from inside a Claude Code session, the child
# inherits CLAUDE_CODE_* markers and opens with warning banners ("transcript
# saving is off", "manual mode on") that no ordinary user would see.
for v in \$(env | sed -n 's/^\(CLAUDE[A-Z_]*\)=.*/\1/p'); do unset "\$v"; done
cd '$WORKSPACE'
EOF

echo "[tapes] agent home ready at $AGENT_HOME (HOME for the take)"
echo "[tapes] remember: bash tools/tapes/prepare-agent.sh --clean when recording is done"
