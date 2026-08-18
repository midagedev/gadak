#!/usr/bin/env bash
# Offline checks for the gadak omarchy-shell plugin. No Omarchy required.
#
# Assertions:
#   1. manifest.json parses, schemaVersion is the number 1, every key the
#      first-party clock manifest carries is present, entry-point files exist
#   2. plugin sources have no sqlite / gadak.db / .db path; data enters only
#      via `gadak sql --json`
#   3. plugin sources do not filter on display names
#   4. the exact query.sql string runs against examples/demo.db and returns
#      NDJSON with open/stuck and open > 0
#   5. qmllint, if present; otherwise skip with a printed reason
#
# Exit 0 = one-line summary. Non-zero names the failing assertion.
set -euo pipefail

fail() {
  echo "verify.sh: FAIL: $*" >&2
  exit 1
}

ok() {
  echo "ok: $*"
}

self="${BASH_SOURCE[0]}"
if [[ "$self" != /* ]]; then
  self="$(pwd)/$self"
fi
here="${self%/*}"
repo="$(cd "${here}/../.." && pwd)"
plugin="${here}/gadak"
manifest="${plugin}/manifest.json"
query_file="${plugin}/query.sql"

[[ -f "$manifest" ]] || fail "manifest: missing $manifest"
[[ -f "$query_file" ]] || fail "query: missing $query_file"

# ── 1. manifest ──────────────────────────────────────────────────────────
GADAK_OMARCHY_MANIFEST="$manifest" GADAK_OMARCHY_PLUGIN="$plugin" python3 - <<'PY' || fail "manifest: schema or entry points"
import json, os, sys
from pathlib import Path

plugin = Path(os.environ["GADAK_OMARCHY_PLUGIN"])
raw = Path(os.environ["GADAK_OMARCHY_MANIFEST"]).read_text()
try:
    m = json.loads(raw)
except json.JSONDecodeError as e:
    print(f"manifest is not JSON: {e}", file=sys.stderr)
    raise SystemExit(1)

# Clock manifest keys (basecamp/omarchy quattro shell/plugins/panels/clock/manifest.json).
top = ["schemaVersion", "id", "name", "version", "author", "description", "kinds", "entryPoints", "barWidget"]
missing = [k for k in top if k not in m]
if missing:
    print(f"missing top-level keys (clock manifest): {missing}", file=sys.stderr)
    raise SystemExit(1)
if m.get("schemaVersion") != 1 or isinstance(m.get("schemaVersion"), bool):
    print(f"schemaVersion must be the JSON number 1, got {m.get('schemaVersion')!r}", file=sys.stderr)
    raise SystemExit(1)
if not isinstance(m["id"], str) or not m["id"]:
    print("id must be a non-empty string", file=sys.stderr)
    raise SystemExit(1)
if m["id"].startswith("omarchy."):
    print(f"id {m['id']!r} uses the reserved omarchy.* namespace", file=sys.stderr)
    raise SystemExit(1)
if not isinstance(m["kinds"], list) or not m["kinds"]:
    print("kinds must be a non-empty array", file=sys.stderr)
    raise SystemExit(1)
if "bar-widget" not in m["kinds"]:
    print("kinds must include bar-widget", file=sys.stderr)
    raise SystemExit(1)
if not isinstance(m["entryPoints"], dict):
    print("entryPoints must be an object", file=sys.stderr)
    raise SystemExit(1)
if "barWidget" not in m["entryPoints"]:
    print("kind bar-widget requires entryPoints.barWidget", file=sys.stderr)
    raise SystemExit(1)
bw = m["barWidget"]
if not isinstance(bw, dict):
    print("barWidget must be an object", file=sys.stderr)
    raise SystemExit(1)
for k in ("displayName", "description", "category", "allowMultiple"):
    if k not in bw:
        print(f"barWidget missing clock key {k!r}", file=sys.stderr)
        raise SystemExit(1)
for key, rel in m["entryPoints"].items():
    if not isinstance(rel, str) or not rel or rel.startswith("/") or ".." in rel:
        print(f"entryPoints.{key} is not a safe relative path: {rel!r}", file=sys.stderr)
        raise SystemExit(1)
    path = plugin / rel
    if not path.is_file():
        print(f"entry point file missing: {rel}", file=sys.stderr)
        raise SystemExit(1)
print("ok: manifest schemaVersion 1, clock keys, entry points on disk")
PY

# ── 2. no direct database access ─────────────────────────────────────────
# Plugin sources = everything under gadak/ (the files the shell loads).
db_hits="$(find "$plugin" -type f ! -name '.*' -print0 | xargs -0 grep -nE 'sqlite|gadak\.db|\.db\b' || true)"
if [[ -n "$db_hits" ]]; then
  echo "$db_hits" >&2
  fail "no-sqlite: plugin sources mention sqlite / gadak.db / a .db path"
fi
if ! grep -q 'gadak sql --json' "$plugin/BarWidget.qml"; then
  fail "no-sqlite: BarWidget.qml does not contain \`gadak sql --json\`"
fi
if ! grep -q 'query.sql' "$plugin/BarWidget.qml"; then
  fail "no-sqlite: BarWidget.qml does not load query.sql (single-owner query)"
fi
ok "no-sqlite: data path is gadak sql --json of query.sql"

# ── 3. no display-name filters ───────────────────────────────────────────
name_hits="$(find "$plugin" -type f ! -name '.*' -print0 | xargs -0 grep -nE "'In Progress'|'Highest'|'Lowest'|'Sub-task'|\"In Progress\"|\"Highest\"|\"Lowest\"|\"Sub-task\"" || true)"
if [[ -n "$name_hits" ]]; then
  echo "$name_hits" >&2
  fail "no-display-name: plugin sources filter on a localized display name"
fi
ok "no-display-name: no In Progress / Highest / Lowest / Sub-task literals"

# ── 4. the real query against examples/demo.db ───────────────────────────
command -v go >/dev/null 2>&1 || fail "query: go is not on PATH (needed to build gadak)"
command -v python3 >/dev/null 2>&1 || fail "query: python3 is not on PATH"
demo="${repo}/examples/demo.db"
[[ -f "$demo" ]] || fail "query: missing $demo"

work="$(mktemp -d "${TMPDIR:-/tmp}/gadak-omarchy-verify.XXXXXX")"
cleanup() { rm -rf "$work"; }
trap cleanup EXIT INT HUP TERM

if ! (cd "$repo" && go build -o "${work}/gadak" ./cmd/gadak); then
  fail "query: go build ./cmd/gadak failed"
fi
mkdir -p "${work}/home"
cp "$demo" "${work}/home/gadak.db"
query="$(cat "$query_file")"
[[ -n "$query" ]] || fail "query: query.sql is empty"

set +e
GADAK_HOME="${work}/home" "${work}/gadak" sql --json "$query" >"${work}/stdout" 2>"${work}/stderr"
sql_rc=$?
set -e
if [[ "$sql_rc" -ne 0 ]]; then
  echo "stdout:" >&2
  cat "${work}/stdout" >&2 || true
  echo "stderr:" >&2
  cat "${work}/stderr" >&2 || true
  fail "query: gadak sql --json exited $sql_rc"
fi

GADAK_OMARCHY_STDOUT="${work}/stdout" python3 - <<'PY' || fail "query: NDJSON / keys / open>0"
import json, os, sys
from pathlib import Path

text = Path(os.environ["GADAK_OMARCHY_STDOUT"]).read_text()
lines = [ln for ln in text.splitlines() if ln.strip()]
if not lines:
    print("stdout is empty (wanted one NDJSON object)", file=sys.stderr)
    raise SystemExit(1)
try:
    obj = json.loads(lines[0])
except json.JSONDecodeError as e:
    print(f"stdout is not NDJSON: {e}: {lines[0]!r}", file=sys.stderr)
    raise SystemExit(1)
if not isinstance(obj, dict):
    print(f"first row is not an object: {obj!r}", file=sys.stderr)
    raise SystemExit(1)
for key in ("open", "stuck"):
    if key not in obj:
        print(f"missing key {key!r} in {obj!r}", file=sys.stderr)
        raise SystemExit(1)
    if isinstance(obj[key], bool) or not isinstance(obj[key], (int, float)):
        print(f"{key} is not a number: {obj[key]!r}", file=sys.stderr)
        raise SystemExit(1)
if int(obj["open"]) <= 0:
    print(f"open={obj['open']} is not > 0 on the demo snapshot", file=sys.stderr)
    raise SystemExit(1)
if int(obj["stuck"]) < 0:
    print(f"stuck={obj['stuck']} is negative", file=sys.stderr)
    raise SystemExit(1)
print(f"ok: query NDJSON open={int(obj['open'])} stuck={int(obj['stuck'])}")
PY

# ── 5. qmllint if present ────────────────────────────────────────────────
if command -v qmllint >/dev/null 2>&1; then
  if ! qmllint "$plugin/BarWidget.qml"; then
    fail "qmllint: BarWidget.qml"
  fi
  ok "qmllint BarWidget.qml"
else
  echo "skip: qmllint not on PATH (tool absent; would syntax-check BarWidget.qml)"
fi

echo "verify.sh: omarchy plugin checks passed"
