#!/usr/bin/env bash
# Print every enum the MCP tool schemas publish, tool by tool, straight from a
# live `gadak mcp` tools/list — and, for each value, whether the Go source
# emits it.
#
# Why this exists: internal/mcp/tools.go descriptions and schemas are a surface
# nothing verifies (CLAUDE.md). A description that teaches a value the server
# does not emit passes go build, go vet, sourcelint, doc-checks and every e2e —
# and the only reader is a shell-less agent, who has no way to discover it was
# lied to. internal/mcp/tools_uitokens_test.go is what actually enforces this
# for the ui.tokens pair; THIS SCRIPT DOES NOT ENFORCE ANYTHING. It is the
# standing aid that answers "what is the surface saying right now" in one
# command, including for tools a future round adds.
#
# Usage: bash tools/mcp-tool-enums.sh
# Exit 0 always: it prints evidence for a human and makes no judgment.

set -euo pipefail
cd "$(dirname "$0")/.."

home=$(mktemp -d)
trap 'rm -rf "$home"' EXIT

req='{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
# The reply goes to a file, not a pipe: the python below owns stdin (heredoc).
printf '%s\n' "$req" | GADAK_HOME="$home" go run ./cmd/gadak mcp 2>/dev/null >"$home/list.json"

python3 - "$PWD" "$home/list.json" <<'PY'
import json, subprocess, sys

root, listfile = sys.argv[1], sys.argv[2]
with open(listfile) as fh:
    doc = json.load(fh)
tools = doc.get("result", {}).get("tools", [])
print(f"{len(tools)} tools on the MCP surface\n")

def emitted(value):
    """Does the Go source contain this literal outside the test files?"""
    r = subprocess.run(
        ["grep", "-rl", "--include=*.go", f'"{value}"', f"{root}/internal", f"{root}/cmd"],
        capture_output=True, text=True)
    files = [f for f in r.stdout.split() if not f.endswith("_test.go")]
    return files

for t in tools:
    print(f"── {t['name']}")
    props = (t.get("inputSchema") or {}).get("properties") or {}
    found = False
    for arg, spec in sorted(props.items()):
        enum = spec.get("enum")
        if not enum:
            continue
        found = True
        for v in enum:
            files = emitted(v)
            where = files[0].replace(root + "/", "") if files else "NOT FOUND IN GO SOURCE"
            extra = f" (+{len(files)-1} more)" if len(files) > 1 else ""
            print(f"   {arg}.enum {v!r:<14} ← {where}{extra}")
    if not found:
        print("   (no enums in this tool's schema)")
    print()
PY
