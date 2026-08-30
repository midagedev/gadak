#!/usr/bin/env bash
# What is going on in the terminal sessions, in one line each (GDK-1153).
#
# Promoted from a scratch probe written while building the session strip:
# the question "which shells are alive, which one wants a person, and which
# one is about to be reaped" was being answered with an ad-hoc curl + jq
# every few minutes. It is the server half of the answer; the client half —
# which of these rows the pane is actually holding — is
# `__gadakTermSessions()` in the browser console
# (web/src/lib/terminal/sessions.svelte.ts).
#
# Read-only. Talks to loopback only, and to nothing but the serve this
# workspace already runs.
#
# Usage:
#   tools/terminal-probe.sh                 # http://127.0.0.1:7777
#   tools/terminal-probe.sh --port 7882     # an e2e serve
#   tools/terminal-probe.sh --url http://127.0.0.1:7777
#   tools/terminal-probe.sh --json          # the raw rows, unformatted
set -euo pipefail

port=7777
url=""
raw=0
while [ $# -gt 0 ]; do
  case "$1" in
    --port) port="$2"; shift 2 ;;
    --url) url="$2"; shift 2 ;;
    --json) raw=1; shift ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[ -n "$url" ] || url="http://127.0.0.1:${port}"

body=$(curl -fsS "${url}/api/v1/terminal/sessions/") || {
  echo "no serve answered at ${url}" >&2
  exit 1
}

if [ "$raw" = 1 ]; then
  printf '%s\n' "$body"
  exit 0
fi

# python3 rather than jq: jq is not a gadak dependency, python3 is already
# assumed by the repo's other tools.
printf '%s' "$body" | python3 -c '
import json, sys, time
from datetime import datetime, timezone

doc = json.load(sys.stdin)
rows = doc.get("sessions") or []
if not rows:
    print("no live sessions")
    raise SystemExit(0)

def ago(iso):
    if not iso or iso.startswith("0001-"):
        return "never"
    try:
        t = datetime.fromisoformat(iso.replace("Z", "+00:00"))
    except ValueError:
        return "?"
    s = int((datetime.now(timezone.utc) - t).total_seconds())
    if s < 60:
        return f"{s}s"
    if s < 3600:
        return f"{s // 60}m"
    return f"{s // 3600}h"

def state(r):
    # The same four states the strip paints, decided the same way
    # (web/src/lib/terminal/strip.ts). BEL outranks everything: a process
    # on the tty answers "may I reap you", not "are you blocked".
    if r.get("needs_attention"):
        return "WANTS-YOU"
    last = r.get("last_output_at") or ""
    if last and not last.startswith("0001-"):
        t = datetime.fromisoformat(last.replace("Z", "+00:00"))
        if (datetime.now(timezone.utc) - t).total_seconds() < 6:
            return "running"
    if r.get("attached", 0) > 0 or len(r.get("pids") or []) > 1:
        return "quiet"
    return "ghost"

print(f"{len(rows)} session(s)")
for r in rows:
    sid = r["id"]
    name = r.get("issue_key") or (sid[:8] + "…")
    st = state(r)
    out = ago(r.get("last_output_at"))
    att = r.get("attached", 0)
    pids = len(r.get("pids") or [])
    size = str(r.get("cols", 0)) + "x" + str(r.get("rows", 0))
    rsz = r.get("resizes", 0)
    grace = r.get("grace_extensions", 0)
    print(f"  {name:<14} {st:<10} out {out:>6}  attached {att}  pids {pids}"
          f"  {size}  resizes {rsz}  grace+{grace}  id {sid}")
'
