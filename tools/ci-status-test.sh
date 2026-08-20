#!/usr/bin/env bash
# Fixture tests for tools/ci-status.sh. No live GitHub calls (GDK-432).
#
# FAIL-first: the pre-fix jq (sha-only, every run votes) turns
# tools/ci-status-fixtures/gdk-432-same-sha.json into RED. After the
# default-branch-push filter, the same fixture is green and lists the
# dispatch failure as a note.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$ROOT/tools/ci-status.sh"
FIXTURE_432="$ROOT/tools/ci-status-fixtures/gdk-432-same-sha.json"
WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/ci-status-test.XXXXXX")"
trap 'rm -rf "$WORKDIR"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok: $*"; }

# Pre-fix jq from tools/ci-status.sh before GDK-432 (commit a596667):
#   --json conclusion,status,name,databaseId
#   --jq '.[] | [.conclusion // "", .status, .name] | @tsv'
PRE_FIX_JQ='.[] | [.conclusion // "", .status, .name] | @tsv'

command -v jq >/dev/null || fail "jq is required to apply fixtures (no live gh)"
command -v python3 >/dev/null || fail "python3 is required to write per-sha fixtures"

[[ -x "$SCRIPT" || -f "$SCRIPT" ]] || fail "missing $SCRIPT"
[[ -f "$FIXTURE_432" ]] || fail "missing $FIXTURE_432"

# ── Fake gh: applies the script's --jq to a fixture. Never hits the network.
cat > "$WORKDIR/gh" << 'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "auth" ]]; then exit 0; fi
if [[ "${1:-}" == "repo" && "${2:-}" == "view" ]]; then
  echo "main"
  exit 0
fi
if [[ "${1:-}" == "run" && "${2:-}" == "list" ]]; then
  jq_expr=""
  commit=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --jq) jq_expr="$2"; shift 2 ;;
      --commit) commit="$2"; shift 2 ;;
      --json|--limit) shift 2 ;;
      *) shift ;;
    esac
  done
  [[ -n "$jq_expr" ]] || { echo "fake-gh: missing --jq" >&2; exit 1; }
  file="${GH_FAKE_RUNS_JSON:-}"
  if [[ -n "${GH_FAKE_RUNS_DIR:-}" && -n "$commit" && -f "$GH_FAKE_RUNS_DIR/${commit}.json" ]]; then
    file="$GH_FAKE_RUNS_DIR/${commit}.json"
  fi
  [[ -n "$file" && -f "$file" ]] || { echo "[]"; exit 0; }
  jq -r "$jq_expr" "$file"
  exit 0
fi
echo "fake-gh: unhandled: $*" >&2
exit 1
EOF
chmod +x "$WORKDIR/gh"

run_ci() {
  local stdout_f="$WORKDIR/stdout" stderr_f="$WORKDIR/stderr"
  set +e
  env PATH="$WORKDIR:$PATH" \
    GADAK_CI_STATUS_DEFAULT_BRANCH=main \
    GH_FAKE_RUNS_JSON="${GH_FAKE_RUNS_JSON:-}" \
    GH_FAKE_RUNS_DIR="${GH_FAKE_RUNS_DIR:-}" \
    "$SCRIPT" --no-wait "$@" >"$stdout_f" 2>"$stderr_f"
  RUN_EC=$?
  set -e
  RUN_STDOUT="$(cat "$stdout_f")"
  RUN_STDERR="$(cat "$stderr_f")"
}

# ── 1. FAIL-first lock: unfiltered 3-run fixture is a false RED ───────────
PRE_ROWS="$(jq -r "$PRE_FIX_JQ" "$FIXTURE_432")"
echo "$PRE_ROWS" | grep -qE '^(failure|timed_out)' \
  || fail "fixture no longer encodes the GDK-432 false-red (unfiltered jq must match ^failure)"
# Same stacked-red grep the pre-fix script used on every sha's rows.
echo "$PRE_ROWS" | grep -q 'failure' \
  || fail "unfiltered rows must contain 'failure' (parent stacked-red false positive)"
ok "FAIL-first lock: unfiltered gdk-432 fixture is RED (false)"

# ── 2. GDK-432: same fixture, default-branch push only → green + note ─────
GH_FAKE_RUNS_JSON="$FIXTURE_432"
GH_FAKE_RUNS_DIR=""
run_ci
[[ "$RUN_EC" -eq 0 ]] || fail "gdk-432 expected exit 0, got $RUN_EC
stdout:
$RUN_STDOUT
stderr:
$RUN_STDERR"
echo "$RUN_STDOUT" | grep -qx 'ci-status: green.' \
  || fail "gdk-432 expected 'ci-status: green.' on stdout, got:
$RUN_STDOUT"
echo "$RUN_STDERR" | grep -q 'ci-status: RED' \
  && fail "gdk-432 must not print RED
stderr:
$RUN_STDERR"
echo "$RUN_STDERR" | grep -q 'already stacked' \
  && fail "gdk-432 must not warn stacked-red (parent uses the same filter)
stderr:
$RUN_STDERR"
echo "$RUN_STDOUT" | grep -q 'not a default-branch push' \
  || fail "gdk-432 expected a note listing non-verdict runs
stdout:
$RUN_STDOUT"
echo "$RUN_STDOUT" | grep -q 'workflow_dispatch' \
  || fail "gdk-432 note must show the dispatch event
stdout:
$RUN_STDOUT"
echo "$RUN_STDOUT" | grep -q 'gdk-432-pages' \
  || fail "gdk-432 note must show the branch ref
stdout:
$RUN_STDOUT"
# Verdict lines still list the two main push successes, without the failure
# conclusion on those lines.
echo "$RUN_STDOUT" | grep -E 'Hosted demo +success' >/dev/null \
  || fail "gdk-432 verdict should list Hosted demo success
stdout:
$RUN_STDOUT"
echo "$RUN_STDOUT" | grep -E 'CI +success' >/dev/null \
  || fail "gdk-432 verdict should list CI success
stdout:
$RUN_STDOUT"
ok "gdk-432 same-sha dispatch failure is a note, verdict is green"

# ── 3. GDK-57: a cancelled default-branch push is still no verdict ────────
cat > "$WORKDIR/cancelled-main.json" << 'JSON'
[
  {
    "conclusion": "cancelled",
    "status": "completed",
    "name": "CI",
    "databaseId": 1,
    "headBranch": "main",
    "event": "push"
  }
]
JSON
GH_FAKE_RUNS_JSON="$WORKDIR/cancelled-main.json"
run_ci
[[ "$RUN_EC" -eq 2 ]] || fail "cancelled main push expected exit 2, got $RUN_EC
stdout:
$RUN_STDOUT
stderr:
$RUN_STDERR"
echo "$RUN_STDERR" | grep -q 'no verdict' \
  || fail "cancelled main push must keep the GDK-57 no-verdict message
stderr:
$RUN_STDERR"
echo "$RUN_STDERR" | grep -q 'ci-status: RED' \
  && fail "cancelled main push must not be RED
stderr:
$RUN_STDERR"
ok "GDK-57 cancelled default-branch push is still no verdict"

# ── 4. A failed default-branch push is still RED ──────────────────────────
cat > "$WORKDIR/main-red.json" << 'JSON'
[
  {
    "conclusion": "failure",
    "status": "completed",
    "name": "CI",
    "databaseId": 2,
    "headBranch": "main",
    "event": "push"
  }
]
JSON
GH_FAKE_RUNS_JSON="$WORKDIR/main-red.json"
run_ci
[[ "$RUN_EC" -eq 1 ]] || fail "main push failure expected exit 1, got $RUN_EC
stdout:
$RUN_STDOUT
stderr:
$RUN_STDERR"
echo "$RUN_STDERR" | grep -q 'ci-status: RED' \
  || fail "main push failure must print RED
stderr:
$RUN_STDERR"
ok "default-branch push failure is still RED"

# ── 5. Cancelled dispatch next to a green push does not steal the verdict ─
cat > "$WORKDIR/cancelled-dispatch.json" << 'JSON'
[
  {
    "conclusion": "success",
    "status": "completed",
    "name": "CI",
    "databaseId": 3,
    "headBranch": "main",
    "event": "push"
  },
  {
    "conclusion": "cancelled",
    "status": "completed",
    "name": "Hosted demo",
    "databaseId": 4,
    "headBranch": "gdk-432-pages",
    "event": "workflow_dispatch"
  }
]
JSON
GH_FAKE_RUNS_JSON="$WORKDIR/cancelled-dispatch.json"
run_ci
[[ "$RUN_EC" -eq 0 ]] || fail "cancelled dispatch + green push expected exit 0, got $RUN_EC
stdout:
$RUN_STDOUT
stderr:
$RUN_STDERR"
echo "$RUN_STDOUT" | grep -qx 'ci-status: green.' \
  || fail "cancelled dispatch must not steal a green push verdict
stdout:
$RUN_STDOUT
stderr:
$RUN_STDERR"
echo "$RUN_STDERR" | grep -q 'no verdict' \
  && fail "cancelled dispatch must not trigger GDK-57 no-verdict
stderr:
$RUN_STDERR"
ok "cancelled dispatch does not steal a green default-branch push"

# ── 6. timed_out on a default-branch push is still RED ────────────────────
cat > "$WORKDIR/timed-out.json" << 'JSON'
[
  {
    "conclusion": "timed_out",
    "status": "completed",
    "name": "CI",
    "databaseId": 5,
    "headBranch": "main",
    "event": "push"
  }
]
JSON
GH_FAKE_RUNS_JSON="$WORKDIR/timed-out.json"
run_ci
[[ "$RUN_EC" -eq 1 ]] || fail "timed_out main push expected exit 1, got $RUN_EC"
echo "$RUN_STDERR" | grep -q 'ci-status: RED' \
  || fail "timed_out main push must print RED
stderr:
$RUN_STDERR"
ok "default-branch push timed_out is still RED"

# ── 7. Parent look-back uses the same filter (false stacked-red) ──────────
# Single-file fixture is returned for every sha, including parents. After
# the filter, parents of a green main push are also green, so the stacked
# warning must stay off. Covered by test 2; assert the mechanism on a
# parent that is *only* a failed dispatch.
HEAD_SHA="$(git -C "$ROOT" rev-parse HEAD)"
PARENT_SHA="$(git -C "$ROOT" rev-parse HEAD^)"
mkdir -p "$WORKDIR/by-sha"
python3 - "$WORKDIR/by-sha" "$HEAD_SHA" "$PARENT_SHA" << 'PY'
import json, pathlib, sys
out, head, parent = pathlib.Path(sys.argv[1]), sys.argv[2], sys.argv[3]
head_runs = [
    {
        "conclusion": "success",
        "status": "completed",
        "name": "CI",
        "databaseId": 10,
        "headBranch": "main",
        "event": "push",
    }
]
parent_runs = [
    {
        "conclusion": "failure",
        "status": "completed",
        "name": "Hosted demo",
        "databaseId": 11,
        "headBranch": "gdk-432-pages",
        "event": "workflow_dispatch",
    }
]
(out / f"{head}.json").write_text(json.dumps(head_runs))
(out / f"{parent}.json").write_text(json.dumps(parent_runs))
PY
GH_FAKE_RUNS_JSON=""
GH_FAKE_RUNS_DIR="$WORKDIR/by-sha"
run_ci "$HEAD_SHA"
[[ "$RUN_EC" -eq 0 ]] || fail "parent dispatch-only failure expected HEAD green, got $RUN_EC
stdout:
$RUN_STDOUT
stderr:
$RUN_STDERR"
echo "$RUN_STDERR" | grep -q 'already stacked' \
  && fail "parent dispatch failure must not count as stacked-red
stderr:
$RUN_STDERR"
ok "parent look-back ignores a dispatch failure"

# ── 8. Parent look-back still fires for a real default-branch push red ────
python3 - "$WORKDIR/by-sha" "$HEAD_SHA" "$PARENT_SHA" << 'PY'
import json, pathlib, sys
out, head, parent = pathlib.Path(sys.argv[1]), sys.argv[2], sys.argv[3]
head_runs = [
    {
        "conclusion": "success",
        "status": "completed",
        "name": "CI",
        "databaseId": 12,
        "headBranch": "main",
        "event": "push",
    }
]
parent_runs = [
    {
        "conclusion": "failure",
        "status": "completed",
        "name": "CI",
        "databaseId": 13,
        "headBranch": "main",
        "event": "push",
    }
]
(out / f"{head}.json").write_text(json.dumps(head_runs))
(out / f"{parent}.json").write_text(json.dumps(parent_runs))
PY
run_ci "$HEAD_SHA"
[[ "$RUN_EC" -eq 0 ]] || fail "real parent red expected HEAD green exit 0, got $RUN_EC
stdout:
$RUN_STDOUT
stderr:
$RUN_STDERR"
echo "$RUN_STDOUT" | grep -qx 'ci-status: green.' \
  || fail "real parent red: HEAD should still be green
stdout:
$RUN_STDOUT"
echo "$RUN_STDERR" | grep -q 'already stacked' \
  || fail "real parent default-branch push failure must still warn stacked-red
stderr:
$RUN_STDERR"
ok "parent look-back still warns when a default-branch push was red"

# ── 9. No runs at all: same "no runs yet" contract ────────────────────────
GH_FAKE_RUNS_DIR=""
GH_FAKE_RUNS_JSON="$WORKDIR/empty.json"
echo '[]' > "$GH_FAKE_RUNS_JSON"
run_ci
[[ "$RUN_EC" -eq 2 ]] || fail "empty runs expected exit 2, got $RUN_EC"
echo "$RUN_STDERR" | grep -q 'no runs yet' \
  || fail "empty runs must print 'no runs yet'
stderr:
$RUN_STDERR"
echo "$RUN_STDOUT" | grep -q 'ci-status: green.' \
  && fail "empty runs must not be green
stdout:
$RUN_STDOUT"
ok "no runs yet is still exit 2"

# ── 10. A feature-branch push is not a default-branch verdict ─────────────
cat > "$WORKDIR/branch-push.json" << 'JSON'
[
  {
    "conclusion": "failure",
    "status": "completed",
    "name": "CI",
    "databaseId": 14,
    "headBranch": "gdk-432-pages",
    "event": "push"
  }
]
JSON
GH_FAKE_RUNS_JSON="$WORKDIR/branch-push.json"
GH_FAKE_RUNS_DIR=""
run_ci
[[ "$RUN_EC" -eq 2 ]] || fail "feature-branch push expected exit 2 (no verdict), got $RUN_EC
stdout:
$RUN_STDOUT
stderr:
$RUN_STDERR"
echo "$RUN_STDERR" | grep -q 'ci-status: RED' \
  && fail "feature-branch push must not be the HEAD verdict
stderr:
$RUN_STDERR"
echo "$RUN_STDOUT" | grep -q 'not a default-branch push' \
  || fail "feature-branch push must still be listed as a note
stdout:
$RUN_STDOUT"
ok "feature-branch push is listed, not counted"

echo "ci-status-test: all cases passed"
