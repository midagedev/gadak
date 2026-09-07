#!/usr/bin/env bash
# staticcheck over the GOOS matrix, for both Go modules (GDK-1463).
#
# Why a matrix and not a single run: staticcheck analyses one GOOS at a time,
# so every function whose only caller sits behind a `//go:build <other-goos>`
# tag is dead code from where it stands. On this repo a darwin-only run reports
# `parseProcStartTime` (internal/term/members_stat.go, called from
# members_linux.go) and `protocolDefaultIcon` (desktop/protocol.go, called from
# protocol_windows.go) as U1000 — both false. Running darwin, linux and windows
# and keeping only what every run agrees on removes that whole class, and the
# per-GOOS leftovers are printed separately as information rather than dropped.
#
# ST1005 (error strings should not be capitalized / end in punctuation) is
# excluded: gadak's user-facing error strings are Korean sentences that begin
# with a Latin word (cmd/gadak/raycast.go) and internal/mcp/tools.go echoes a
# multi-line protocol string on purpose. The check has no way to tell those from
# the style problem it is looking for.
#
# Some (module, GOOS) pairs cannot be analysed on a given host: desktop/ pulls
# wails, whose linux backend is cgo, so GOOS=linux fails to build from macOS
# (and GOOS=darwin fails from the linux CI runner). The script tells that apart
# from a real break by where the compile error lives — a path inside this repo
# is a failure and is reported; a path in the module cache means the host cannot
# cross-compile that pair, so the pair is skipped with a note and left out of
# the matrix. A run that does not compile never contributes findings, because
# its package set is incomplete and would demote real findings by accident.
#
# Usage:
#   tools/staticcheck.sh                 # exit 1 on cross-platform findings
#   tools/staticcheck.sh --warn-only     # always exit 0 (what CI runs today)
#   tools/staticcheck.sh --goos darwin   # one GOOS, for reproducing a finding
#   tools/staticcheck.sh --self-test     # check the classifier against fixtures
#
# Exit 0 = no cross-platform findings (or --warn-only).
#      1 = cross-platform findings, or the repo does not compile for some GOOS.
#      2 = staticcheck is not installed, or bad arguments.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

WARN_ONLY=0
SELF_TEST=0
GOOSES=(darwin linux windows)

while [[ $# -gt 0 ]]; do
  case "$1" in
    --warn-only) WARN_ONLY=1; shift ;;
    --self-test) SELF_TEST=1; shift ;;
    --goos) IFS=',' read -r -a GOOSES <<< "$2"; shift 2 ;;
    -h|--help) sed -n '2,38p' "$0"; exit 0 ;;
    *) echo "staticcheck.sh: unknown argument: $1" >&2; exit 2 ;;
  esac
done

# The report half is a variable rather than a heredoc so that --self-test can
# drive it over fixtures — the classification rules below (in-repo break vs
# host cannot cross-compile, and the all-GOOS intersection) are the part of
# this script that is worth testing, and breaking the tree to test them is not
# an option in a shared checkout.
PY_REPORT="$(cat <<'PY'
import collections
import json
import os
import re
import sys

root, manifest = sys.argv[1], sys.argv[2]
warn_only = os.environ.get("WARN_ONLY") == "1"

# See the header comment: gadak's error strings are Korean sentences and one
# deliberate multi-line protocol echo, which ST1005 cannot tell from the style
# problem it looks for.
EXCLUDED = {"ST1005"}

# `<path>:<line>:<col>:` inside a compile error's message body.
COMPILE_PATH = re.compile(r"^(\S+\.go):\d+:\d+:", re.M)

runs = []
with open(manifest) as fh:
    for line in fh:
        module, moddir, goos, stem = line.rstrip("\n").split("\t")
        runs.append((module, moddir, goos, stem))

status = {}       # (module, goos) -> "ok" | "broken" | "skipped" | "error"
detail = {}       # (module, goos) -> text explaining a non-ok status
found = {}        # (module, goos) -> {key: record}
modules = []
gooses = []

for module, moddir, goos, stem in runs:
    if module not in modules:
        modules.append(module)
    if goos not in gooses:
        gooses.append(goos)

    objs, junk = [], []
    with open(stem + ".json") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                objs.append(json.loads(line))
            except json.JSONDecodeError:
                junk.append(line)
    with open(stem + ".err") as fh:
        stderr = fh.read().strip()

    compile_msgs = [o.get("message", "") for o in objs if o.get("code") == "compile"]

    if compile_msgs:
        # Where the broken file lives decides what this means. Inside the repo
        # it is a real break and the gate has to say so; in the module cache it
        # means this host cannot build that pair, which is not the repo's fault.
        in_repo = []
        for msg in compile_msgs:
            for rel in COMPILE_PATH.findall(msg):
                path = os.path.normpath(os.path.join(moddir, rel))
                if path.startswith(root + os.sep):
                    in_repo.append(os.path.relpath(path, root))
        body = "\n".join("    " + ln for m in compile_msgs for ln in m.splitlines())
        if in_repo:
            status[(module, goos)] = "broken"
            detail[(module, goos)] = body
        else:
            status[(module, goos)] = "skipped"
            first = COMPILE_PATH.findall(compile_msgs[0])
            detail[(module, goos)] = (
                "does not cross-compile on this host (the failing files are "
                "dependencies, not this repo): " + (first[0] if first else "?"))
        found[(module, goos)] = {}
        continue

    if not objs and (stderr or junk):
        status[(module, goos)] = "error"
        detail[(module, goos)] = "\n".join(
            "    " + ln for ln in (stderr.splitlines() + junk))
        found[(module, goos)] = {}
        continue

    records = {}
    for o in objs:
        code = o.get("code", "")
        loc = o.get("location") or {}
        path = loc.get("file") or ""
        if not path:
            continue
        rel = os.path.relpath(path, root) if path.startswith(root + os.sep) else path
        # Key on file:line:code, as the finding's column can move between GOOS
        # runs without the finding being a different one.
        records["%s:%s:%s" % (rel, loc.get("line"), code)] = (
            rel, loc.get("line"), loc.get("column"), code, o.get("message", ""))
    status[(module, goos)] = "ok"
    found[(module, goos)] = records

fail = False
cross_total = 0
platform_total = 0
out = []

for module in modules:
    ok = [g for g in gooses if status.get((module, g)) == "ok"]
    out.append("== module %s ==" % module)
    for goos in gooses:
        st = status.get((module, goos))
        if st is None:
            continue
        if st == "ok":
            out.append("  GOOS=%-8s analysed (%d finding(s) before matrix)"
                       % (goos, len(found[(module, goos)])))
        elif st == "skipped":
            out.append("  GOOS=%-8s SKIPPED — %s" % (goos, detail[(module, goos)]))
        elif st == "broken":
            out.append("  GOOS=%-8s DOES NOT COMPILE — this repo's own files:" % goos)
            out.append(detail[(module, goos)])
            fail = True
        else:
            out.append("  GOOS=%-8s staticcheck failed to run:" % goos)
            out.append(detail[(module, goos)])
            fail = True

    if not ok:
        out.append("  no GOOS could be analysed for this module.")
        out.append("")
        continue
    if len(ok) == 1:
        out.append("  NOTE: only GOOS=%s could be analysed, so nothing can be "
                   "demoted — a U1000 here may be a build-tag false positive." % ok[0])

    counts = collections.Counter()
    record = {}
    seen_in = collections.defaultdict(list)
    for goos in ok:
        for key, rec in found[(module, goos)].items():
            counts[key] += 1
            record.setdefault(key, rec)
            seen_in[key].append(goos)

    def sort_key(k):
        rel, line, _col, code, _msg = record[k]
        return (rel, line or 0, code)

    cross, partial = [], []
    for key, n in counts.items():
        if record[key][3] in EXCLUDED:
            continue
        (cross if n == len(ok) else partial).append(key)

    out.append("")
    out.append("  cross-platform (%d) — present under every analysed GOOS:" % len(cross))
    for key in sorted(cross, key=sort_key):
        rel, line, col, code, msg = record[key]
        out.append("    %s:%s:%s: %s (%s)" % (rel, line, col, msg, code))
    if not cross:
        out.append("    (none)")

    out.append("")
    out.append("  platform-only (%d, informational) — a build-tag artefact unless "
               "the file itself is platform-specific:" % len(partial))
    for key in sorted(partial, key=sort_key):
        rel, line, col, code, msg = record[key]
        out.append("    [%s] %s:%s:%s: %s (%s)"
                   % (",".join(seen_in[key]), rel, line, col, msg, code))
    if not partial:
        out.append("    (none)")
    out.append("")

    cross_total += len(cross)
    platform_total += len(partial)
    if cross:
        fail = True

print("\n".join(out))
print("summary: %d cross-platform, %d platform-only, ST1005 excluded"
      % (cross_total, platform_total))

if fail and warn_only:
    print("--warn-only: reporting without failing.")
    sys.exit(0)
if fail:
    print("FAIL: fix the cross-platform findings above, or exclude the check in "
          "tools/staticcheck.sh with a reason.")
    sys.exit(1)
print("OK")
sys.exit(0)
PY
)"

report() {  # $1 = manifest path
  WARN_ONLY="$WARN_ONLY" python3 -c "$PY_REPORT" "$ROOT" "$1"
}

# --- self-test -------------------------------------------------------------
# Fixtures stand in for six staticcheck runs. They cover the three decisions
# this script makes that a plain "does it run" check would not: ST1005 dropped,
# a finding seen under one GOOS of two demoted to platform-only, and the two
# kinds of compile error told apart by path.
if [[ "$SELF_TEST" == 1 ]]; then
  W="$(mktemp -d)"
  trap 'rm -rf "$W"' EXIT
  : > "$W/manifest"
  fixture() {  # module goos json-lines
    local module="$1" goos="$2" dir="$ROOT"
    [[ "$module" == desktop ]] && dir="$ROOT/desktop"
    printf '%s' "$3" > "$W/$module.$goos.json"
    : > "$W/$module.$goos.err"
    printf '%s\t%s\t%s\t%s\n' "$module" "$dir" "$goos" "$W/$module.$goos" >> "$W/manifest"
  }
  fixture root darwin \
'{"code":"U1000","location":{"file":"'"$ROOT"'/internal/term/members_stat.go","line":45,"column":6},"message":"func parseProcStartTime is unused"}
{"code":"ST1005","location":{"file":"'"$ROOT"'/cmd/gadak/raycast.go","line":252,"column":11},"message":"error strings should not be capitalized"}
{"code":"S1016","location":{"file":"'"$ROOT"'/internal/server/jql.go","line":52,"column":62},"message":"should convert me"}
'
  fixture root linux \
'{"code":"S1016","location":{"file":"'"$ROOT"'/internal/server/jql.go","line":52,"column":62},"message":"should convert me"}
'
  fixture root windows \
'{"code":"compile","location":{"file":"","line":0,"column":0},"message":"# github.com/midagedev/gadak/internal/sync\ninternal/sync/linear.go:6:2: \"fmt\" imported and not used"}
'
  fixture desktop darwin \
'{"code":"compile","location":{"file":"","line":0,"column":0},"message":"# github.com/wailsapp/wails/v3/pkg/application\n../../../go/pkg/mod/wails/pkg/application/menu_linux.go:7:12: undefined: pointer"}
'
  fixture desktop linux ''
  fixture desktop windows ''

  rc=0
  WARN_ONLY=0 report "$W/manifest" > "$W/out" 2>&1 || rc=$?
  cat "$W/out"
  echo "--- self-test assertions (exit was $rc) ---"

  fails=0
  want() {  # description, expected-present(1)/absent(0), pattern
    if grep -qF -- "$3" "$W/out"; then got=1; else got=0; fi
    if [[ "$got" == "$2" ]]; then echo "  ok   $1"; else echo "  FAIL $1"; fails=1; fi
  }
  [[ "$rc" == 1 ]] && echo "  ok   exits 1 when something is wrong" \
                   || { echo "  FAIL expected exit 1, got $rc"; fails=1; }
  want "S1016 seen under both GOOS is cross-platform" 1 "cross-platform (1)"
  want "  ...and it is the jql.go one" 1 "internal/server/jql.go:52:62"
  want "U1000 seen under one GOOS of two is demoted" 1 "platform-only (1"
  want "  ...and it names the GOOS it came from" 1 "[darwin] internal/term/members_stat.go:45"
  want "ST1005 is excluded entirely" 0 "raycast.go"
  want "an in-repo compile error fails the run" 1 "DOES NOT COMPILE"
  want "  ...and names the file" 1 "internal/sync/linear.go"
  want "a dependency-only compile error is a skip, not a failure" 1 "SKIPPED"

  rc=0
  WARN_ONLY=1 report "$W/manifest" > "$W/warn" 2>&1 || rc=$?
  [[ "$rc" == 0 ]] && echo "  ok   --warn-only exits 0 on the same input" \
                   || { echo "  FAIL --warn-only should exit 0, got $rc"; fails=1; }

  [[ "$fails" == 0 ]] && { echo "self-test: OK"; exit 0; }
  echo "self-test: FAILED" >&2
  exit 1
fi

# --- real run --------------------------------------------------------------
if ! command -v staticcheck >/dev/null 2>&1; then
  echo "staticcheck.sh: staticcheck is not on PATH." >&2
  echo "  go install honnef.co/go/tools/cmd/staticcheck@latest" >&2
  exit 2
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
MANIFEST="$WORK/manifest"
: > "$MANIFEST"

# module name, module directory (relative to ROOT), package patterns.
# desktop/ is a separate go.mod (replace github.com/midagedev/gadak => ../), so
# it needs its own invocation from its own directory.
run_module() {
  local name="$1" dir="$2"; shift 2
  local goos out
  for goos in "${GOOSES[@]}"; do
    out="$WORK/$name.$goos"
    echo "  $name  GOOS=$goos" >&2
    # CGO_ENABLED is left at its default on purpose: the native GOOS analyses
    # what `go vet` and the desktop build see, and the cross ones are CGO-off
    # the same way `go build` makes them. No in-repo Go file is cgo-guarded, so
    # the two modes do not disagree about this repo's own code.
    ( cd "$ROOT/$dir" && GOOS="$goos" staticcheck -f json "$@" ) \
      > "$out.json" 2> "$out.err" || true
    printf '%s\t%s\t%s\t%s\n' "$name" "$ROOT/$dir" "$goos" "$out" >> "$MANIFEST"
  done
}

echo "staticcheck $(staticcheck -version | sed 's/^staticcheck //') over GOOS: ${GOOSES[*]}" >&2
run_module root . ./cmd/... ./internal/... ./tools/...
run_module desktop desktop ./...
echo >&2

report "$MANIFEST"
