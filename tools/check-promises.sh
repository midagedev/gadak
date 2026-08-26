#!/usr/bin/env bash
# Execute the verification blocks of docs/PROMISES.md.
#
# The PROMISES preamble says: "Every command was run on this tree and
# produced the output shown; if one stops doing so, the promise is broken
# and that is a bug worth reporting." Nothing enforced that sentence —
# v0.18.1 shipped with promise #9 reading a YAML persist file the code had
# already replaced with SQLite, so the document failed its own standard on
# a released tree with every gate green.
#
# What this gate does:
#   - extracts each numbered promise's first ```bash fence from
#     docs/PROMISES.md
#   - runs it from the repo root (the blocks are written as "one command
#     you can run in a clone of this repository" and use repo-relative
#     paths), prefixed with `set -e`, each in its own TMPDIR so scratch
#     never leaks between blocks or into the tree
#   - asserts exit 0. Expected-output comments (`# → …`) are deliberately
#     NOT string-compared: brittle against formatting, and the contract is
#     "the block still runs and passes", not "the output is byte-frozen".
#     Known limit: a `grep … | wc -l` block exits 0 whatever the count is;
#     the gate keeps it because it still catches the block no longer
#     running at all (renamed flag, moved path, missing file).
#   - a block that needs a network round trip or a credential is skipped
#     ONLY by a marker line visible inside the fence:
#         # promises-skip: <reason>
#     A silent total skip is invalid: the summary lists executed and
#     skipped promise numbers, and 0 executed is a failure.
#   - a numbered promise with no ```bash fence is a failure: deleting the
#     block must not silently retire the check.
#
# Needs on PATH: go (six-plus blocks build or test Go packages), sqlite3
# (the mirror-is-ordinary-SQLite blocks). CI runs this via doc-checks in
# the build job, after `go vet` has warmed the build cache.
#
# Usage: tools/check-promises.sh  (also called at the end of tools/doc-checks.sh)
# Exit 0 = every executed block exited 0 and at least one executed.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PROMISES="docs/PROMISES.md"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

ok() {
  echo "ok: $*"
}

[[ -f "$PROMISES" ]] || fail "$PROMISES is missing"
command -v go >/dev/null || fail "go not on PATH — the PROMISES blocks need a Go toolchain"
command -v sqlite3 >/dev/null || fail "sqlite3 not on PATH — the mirror blocks read SQLite directly"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Extraction. Promise spans run from a `**N. ` header to the next one; the
# first ```bash fence inside a span is that promise's block. Writes
# $work/<N>.sh (block body only) and $work/index.tsv with one
# <number>\t<run|skip|missing>\t<reason> row per promise, in document order.
python3 - "$PROMISES" "$work" <<'PY'
import re
import sys
from pathlib import Path

src, work = Path(sys.argv[1]), Path(sys.argv[2])
text = src.read_text()

heads = [(m.start(), int(m.group(1))) for m in re.finditer(r"^\*\*(\d+)\.\s", text, re.M)]
if not heads:
    print("%s: no numbered promises found" % src)
    raise SystemExit(1)

FENCE = re.compile(r"^```bash[ \t]*\n(.*?)^```", re.S | re.M)
SKIP = re.compile(r"^\s*#\s*promises-skip:\s*(\S.*?)\s*$", re.M)

rows = []
for i, (start, num) in enumerate(heads):
    end = heads[i + 1][0] if i + 1 < len(heads) else len(text)
    chunk = text[start:end]
    m = FENCE.search(chunk)
    if not m:
        rows.append((num, "missing", "no ```bash fence in this promise"))
        continue
    body = m.group(1)
    s = SKIP.search(body)
    if s:
        rows.append((num, "skip", s.group(1)))
    else:
        rows.append((num, "run", ""))
        (work / ("%d.sh" % num)).write_text(body)

with (work / "index.tsv").open("w") as f:
    for num, kind, reason in rows:
        f.write("%d\t%s\t%s\n" % (num, kind, reason))
PY

ran_list=""
ran_count=0
skip_list=""
skip_count=0

while IFS=$'\t' read -r num kind reason; do
  case "$kind" in
    missing)
      fail "promise ${num} in ${PROMISES} has no \`\`\`bash verification block — a promise without a runnable check is just a claim"
      ;;
    skip)
      skip_count=$((skip_count + 1))
      skip_list="${skip_list}${num} "
      echo "skip: promise ${num} — ${reason}"
      ;;
    run)
      block_dir="$(mktemp -d)"
      out_file="$work/${num}.out"
      { echo 'set -e'; cat "$work/${num}.sh"; } >"$work/${num}.run.sh"
      if ( cd "$ROOT" && TMPDIR="$block_dir" bash "$work/${num}.run.sh" ) >"$out_file" 2>&1; then
        rm -rf "$block_dir"
        ran_count=$((ran_count + 1))
        ran_list="${ran_list}${num} "
        ok "promise ${num} verification block exited 0"
      else
        rc=$?
        echo "FAIL: promise ${num} verification block exited ${rc}. The block was:" >&2
        sed 's/^/    /' "$work/${num}.sh" >&2
        echo "  its output (last 25 lines):" >&2
        tail -n 25 "$out_file" | sed 's/^/    /' >&2
        echo "  A PROMISES block that no longer passes is a broken promise — fix the doc or the code." >&2
        exit 1
      fi
      ;;
  esac
done < "$work/index.tsv"

if [[ "$ran_count" -eq 0 ]]; then
  fail "every PROMISES verification block was skipped — a gate that executes nothing is not a gate"
fi

ok "PROMISES blocks: ${ran_count} executed (${ran_list% }), ${skip_count} skipped (${skip_list:-none})"
