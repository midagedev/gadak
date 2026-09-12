#!/usr/bin/env bash
# Stale-doc guards: greps, sqlite lookups, and the delegated script gates
# (check-promises.sh, check-write-handlers.sh). No network. Runs in CI's
# "Documentation factuality" step and from the demo-fixture target.
#
# Checks (FAIL-first was recorded against the pre-fix tree in the 2026-08-15
# census fix round; see scratch/yc/progress-stale.log):
#   1. README.md wraps web-demo.gif in <details> (not an inline hero)
#   2. docs/MCP.md and contracts/agent.md do not list {text} as gadak_search's
#      primary argument (query is primary; text/q are aliases)
#   3. README.md and docs/INSTALL.md say "N issues" matching examples/demo.db
#   4. docs/project/STATE_OF_PLAY.md has no leftover "enables GitHub Pages"
#   5. docs/, specs/, AGENTS.md carry no literal 519 (demo issue count moved)
#   6. The version a reader sees first matches the latest tag: the README
#      status lines (en + ko) and STATE_OF_PLAY's "Last tagged" (2026-08-15:
#      README said 0.13 on a 0.14 tree, and the ko header claimed v0.14 while
#      its own status line said 0.13 — found by the first-impression census)
#   7. Web logic does not key status / priority / issue type on a localized
#      display name (GDK-28/29). FAIL-first: the pre-fix format.ts table
#      `/highest|긴급|가장 높음|blocker/` (and friends) is kept at
#      /tmp/gadak-priority-gdk28/format.ts and fails this grep.
#   8. docs/PROMISES.md names the same outbound destinations SECURITY.md
#      enumerates (GDK-104). FAIL-first 2026-08-15: adding "| plausible.io"
#      to the docs/PROMISES.md marker made this check fail before it was reverted.
#
# Usage: tools/doc-checks.sh
# Exit 0 = clean, 1 = a check failed.
#
# Structure note (2026-09-10 cost round): the six delegated script gates
# (backlog-scrub-check, check-promises, check-write-handlers,
# check-lockfile-platforms, ci-status-test, audit-test) start concurrently
# right after check 21 sets BACKLOG_ARCHIVE and are replayed — output
# streams and failure position unchanged — at the numbered check each held
# before. Inline checks still run in order, first-fail still exits at the
# failing check, and every assertion still runs on every invocation. The
# per-check timing this relies on prints to stderr from the EXIT trap.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# ── per-check wall timing (GDK-1488) ─────────────────────────────────────
# Every ok() line and each delegated-script call site is stamped; the trap
# prints the slowest checks to stderr at exit (pass or fail — a red run is
# exactly when you want to know what was slow). stdout and the exit code are
# unchanged: the summary is a diagnostic, not a contract.
#
# Clock: bash ≥5 (the CI runner) exposes EPOCHREALTIME, so a stamp is pure
# arithmetic — no fork. Local macOS bash 3.2 has no sub-second clock in the
# shell, so the fallback forks python3 (~25 ms × ~60 stamps ≈ 1.5 s on a
# 74 s run; measured 2026-09-10). CI pays nothing.
_now_ms() {
  if [[ -n "${EPOCHREALTIME:-}" ]]; then
    echo $(( ${EPOCHREALTIME/./} / 1000 ))
  else
    python3 -c 'import time; print(int(time.time() * 1000))'
  fi
}

_TIMING_NAMES=()
_TIMING_MS=()
_TIMING_LAST=0
_GATE_NAMES=()
_GATE_MS=()

_stamp() {
  local now
  now="$(_now_ms)"
  if [[ "$_TIMING_LAST" != "0" ]]; then
    _TIMING_NAMES+=("$1")
    _TIMING_MS+=($(( now - _TIMING_LAST )))
  fi
  _TIMING_LAST="$now"
}

# Sorted slowest-first to stderr. Never fails the run it is timing: every
# command is guarded so the trap cannot turn a green run red (or eat the
# original exit status on a red one).
_timing_summary() {
  # Length guards first: bash 3.2 (local macOS) treats an empty array under
  # set -u as "unbound variable", and this runs from the EXIT trap.
  local i total=0
  for i in ${_TIMING_MS[*]:-}; do total=$(( total + i )); done
  {
    echo "doc-checks timing: ${#_TIMING_MS[@]} stamps, ${total} ms of critical path (top 15):"
    if [[ "${#_TIMING_MS[@]}" -gt 0 ]]; then
    for i in "${!_TIMING_MS[@]}"; do
      printf '%s\t%s\n' "${_TIMING_MS[$i]}" "${_TIMING_NAMES[$i]}"
    done | sort -rn | awk 'NR<=15'
    fi
    if [[ "${#_GATE_NAMES[@]}" -gt 0 ]]; then
      echo "delegated gates (own duration; concurrent with the path above):"
      for i in "${!_GATE_NAMES[@]}"; do
        printf '  %s\t%s\n' "${_GATE_MS[$i]}" "${_GATE_NAMES[$i]}"
      done | sort -rn
    fi
  } >&2 || true
}

ok() {
  echo "ok: $*"
  _stamp "$*"
}

# ── delegated gates: started early, replayed at their check position ──────
# (GDK-1488 cost round, 2026-09-10.) The profile on this tree: the six
# delegated scripts are ~47 of the ~52 measured seconds — the scrub gate
# alone is ~39 s (1,505 detail files through a serial jq loop inside
# backlog-scrub-check.sh). They depend on no inline check (each is read-only
# against the tree; the two Go ones share the build cache, which is
# concurrency-safe), so they start here and their output is replayed at the
# exact position each occupied before — stdout to stdout, stderr to stderr,
# original order. A failure surfaces exactly where it used to: the replay
# waits for the gate and fails at that check's position. A gate the original
# ran conditionally (the scrub gate needs the archive and jq) is started
# under the same condition, so its skip branch is preserved.
#
# The exit trap kills gates still running when an inline check fails first —
# otherwise a red run would wait out a 39 s straggler nobody reads.
_PAR_PIDS=()
_PAR_IDS=()
_PAR_T0=()
_PAR_STARTED=" "
_gate_start() {  # _gate_start <id> <script> [args...]
  local id="$1"; shift
  # The wrapper records the gate's own completion time from inside the
  # background job — measuring at replay time would report the wall clock
  # at the replay position (all gates ≈ the same number), not the duration.
  # set +e first: the subshell inherits errexit, and without this a red
  # gate killed the wrapper before it could write its .rc (found by the
  # scratch engine test: exit was 1, the gate's own 3 never arrived).
  (
    set +e
    bash "$@" >"$FILE_CENSUS.$id.out" 2>"$FILE_CENSUS.$id.err"
    echo $? >"$FILE_CENSUS.$id.rc"
    echo "$(_now_ms)" >"$FILE_CENSUS.$id.t1"
  ) &
  _PAR_PIDS+=($!)
  _PAR_IDS+=("$id")
  _PAR_T0+=($(_now_ms))
  _PAR_STARTED+="$id "
}

_gate_replay() {  # _gate_replay <id> — print captured streams, rc = the gate's
  local id="$1" i pid rc t1
  if [[ "${#_PAR_IDS[@]}" -eq 0 ]]; then
    fail "internal: gate $id was never started"
  fi
  for i in "${!_PAR_IDS[@]}"; do
    [[ "${_PAR_IDS[$i]}" == "$id" ]] || continue
    pid="${_PAR_PIDS[$i]}"
    unset "_PAR_PIDS[$i]" "_PAR_IDS[$i]"
    wait "$pid"  # the wrapper's rc is the echo's, not the gate's
    rc="$(cat "$FILE_CENSUS.$id.rc" 2>/dev/null || echo 1)"
    t1="$(cat "$FILE_CENSUS.$id.t1" 2>/dev/null || echo "${_PAR_T0[$i]}")"
    _GATE_NAMES+=("$id")
    _GATE_MS+=( $(( t1 - _PAR_T0[$i] )) )
    unset "_PAR_T0[$i]"
    cat "$FILE_CENSUS.$id.out"
    cat "$FILE_CENSUS.$id.err" >&2
    rm -f "$FILE_CENSUS.$id.out" "$FILE_CENSUS.$id.err" \
          "$FILE_CENSUS.$id.rc" "$FILE_CENSUS.$id.t1"
    return "$rc"
  done
  fail "internal: gate $id was never started"
}

_kill_gates() {
  local p
  for p in ${_PAR_PIDS[*]:-}; do kill "$p" 2>/dev/null || true; done
}

_gate_require() {  # _gate_require <id> — replay; exit with the gate's rc on red
  local rc=0
  _gate_replay "$1" || rc=$?
  [[ "$rc" == 0 ]] || exit "$rc"
}



# ── Shared file census (GDK-1477) ────────────────────────────────────────
# Several checks below are whole-tree walkers: each one used to run its own
# `Path(".").rglob(...)` in a fresh interpreter and filter the result with
# `any(p in {"node_modules","dist",".git"} for p in path.parts)`. rglob yields
# the pruned paths and then throws them away, so the tree was enumerated once
# per check. This walks it once, prunes the same three directory names at the
# source, and hands the list to those heredocs on argv.
#
# The census is set-identical to the rglob-plus-parts-filter it replaces
# (measured 2026-09-07: 51914 files both ways, symmetric difference empty;
# the .go and .md subsets match too). Per-check filtering is unchanged and
# still lives in each check — this prelude owns the walk, nothing else.
#
# Measured on this tree: enumeration 1.78s per rglob walk → 0.76s once.
# mktemp portability: GNU mktemp (the CI runner) requires the X's in the
# template; macOS accepts a bare -t prefix. The bare form passed every local
# run and failed the first CI run of 2473dc62 ("too few X's in template").
FILE_CENSUS="$(mktemp "${TMPDIR:-/tmp}/gadak-doc-checks-census.XXXXXX")"
trap '_kill_gates; _timing_summary; rm -f "$FILE_CENSUS" "$FILE_CENSUS".*.out "$FILE_CENSUS".*.err' EXIT
_TIMING_LAST="$(_now_ms)"   # the origin: everything before this is shell startup
python3 - "$FILE_CENSUS" <<'CENSUSPY'
import os
import sys

PRUNE = {".git", "node_modules", "dist"}
rows = []
for dirpath, dirnames, filenames in os.walk("."):
    dirnames[:] = [d for d in dirnames if d not in PRUNE]
    for name in filenames:
        path = os.path.join(dirpath, name)[2:]
        if os.path.isfile(path):
            rows.append(path)
rows.sort()
with open(sys.argv[1], "w", encoding="utf-8") as fh:
    fh.write("\n".join(rows))
    fh.write("\n")
CENSUSPY
_stamp 'file census prelude (one os.walk)'

# ── 1. README tour GIF lives in <details> ────────────────────────────────
if ! grep -q 'web-demo.gif' README.md; then
  fail "README.md has no web-demo.gif reference"
fi
# A <details>…</details> block must contain the gif path.
if ! python3 - <<'PY'
import re, sys
from pathlib import Path
text = Path("README.md").read_text()
blocks = re.findall(r"<details\b.*?</details>", text, flags=re.S | re.I)
sys.exit(0 if any("web-demo.gif" in b for b in blocks) else 1)
PY
then
  fail "README.md: web-demo.gif is not inside a <details> block"
fi
ok "README.md web-demo.gif is inside <details>"

# ── 2. gadak_search primary arg is not {text} ────────────────────────────
# Aliases may mention text/q in prose; the brace-shape primary must not be
# {text: string} / {text, limit?}.
text_primary="$(
  grep -nE '\{text: string|\{text, limit' docs/MCP.md specs/000-product/contracts/agent.md || true
)"
if [[ -n "$text_primary" ]]; then
  fail "gadak_search still documents {text} as the primary arg:"$'\n'"$text_primary"
fi
ok "gadak_search primary arg is not {text} in MCP.md / agent.md"

# ── 3. Demo issue count matches README + INSTALL ─────────────────────────
if ! command -v sqlite3 >/dev/null; then
  fail "sqlite3 not on PATH (needed to count examples/demo.db issues)"
fi
n="$(sqlite3 examples/demo.db "select count(*) from issues")"
[[ "$n" =~ ^[0-9]+$ ]] || fail "sqlite3 did not return a count (got ${n@Q})"
for f in README.md docs/INSTALL.md site/public/llms.txt; do
  if ! grep -q "${n} issues" "$f"; then
    fail "$f does not mention ${n} issues (demo.db count)"
  fi
done
ok "README.md, docs/INSTALL.md and site/public/llms.txt say ${n} issues"

# ── 4. Hosted demo is live; no "enables GitHub Pages" leftover ───────────
if grep -n "enables GitHub Pages" docs/project/STATE_OF_PLAY.md; then
  fail "docs/project/STATE_OF_PLAY.md still says \"enables GitHub Pages\""
fi
ok "docs/project/STATE_OF_PLAY.md has no \"enables GitHub Pages\""

# ── 5. No leftover 519 inventory literal ─────────────────────────────────
# CHANGELOG is history and is not under these paths.
#
# 2026-08-23: narrowed to ignore GDK-519 and the grep prefix. The literal this
# guards is a demo inventory count; an issue key that happens to contain those
# three digits is not it, and neither is a match that lives only in the
# file:line grep prints. docs/changelog-detail.md cites GDK-519 and tripped
# the check on both counts. The bare-count case is unchanged and still fails
# (FAIL-first re-run on this edit: a line reading "519 issues" under docs/).
hits="$(grep -rn "519" docs specs AGENTS.md \
  | awk -F: '{ rest = substr($0, index($0, ":") + 1); rest = substr(rest, index(rest, ":") + 1);
               gsub(/GDK-[0-9]+/, "", rest); if (rest ~ /519/) print $0 }' || true)"
if [[ -n "$hits" ]]; then
  fail "stale 519 remnant:"$'\n'"$hits"
fi
ok "no 519 remnant in docs/ specs/ AGENTS.md"

# ── 6. The front door names the version that is actually tagged ─────────
# Skipped (not failed) in a tagless checkout: a shallow CI clone has no tags,
# and this guard is about drift between files, not about tagging policy.
tag="$(git describe --tags --abbrev=0 2>/dev/null || true)"
if [[ -z "$tag" ]]; then
  ok "no tag reachable — version guard skipped"
else
  minor="${tag#v}"; minor="${minor%.*}"   # v0.14.0 → 0.14
  for f in README.md README.ko.md README.ja.md site/public/llms.txt; do
    if ! grep -qE "(Status|상태|状態): ${minor}(,|、| )" "$f"; then
      fail "$f status line does not say ${minor} (latest tag ${tag})"
    fi
  done
  if ! grep -q "Last tagged: ${tag}" docs/project/STATE_OF_PLAY.md; then
    fail "docs/project/STATE_OF_PLAY.md does not say \"Last tagged: ${tag}\""
  fi
  ok "README (en+ko) and STATE_OF_PLAY agree with ${tag}"
fi

# ── 7. Web logic does not key status/priority/type on display names ─────
# Jira translates status, priority, and issue-type names per account and
# ignores Accept-Language. A regex/equality on the English or Korean default
# name is a silent no-op everywhere else (French "La plus haute" lost its
# color; `status = 'In Progress'` is 0 rows on a Korean account).
#
# Scope: web/src/{lib,components,stores} logic files.
#   excluded: lib/i18n/**          — catalogs ARE display names
#             *.test.ts / *.spec.ts — fixtures pass names through as labels
# Patterns: the locale-name regex/alternation tables deleted from format.ts
# (GDK-28) and the same shape for status / issue-type equality on a
# Jira-default EN/KO display name.
#
# Also: a plain string literal of a Jira-default name (GDK-248). The
# pipe-table grep cannot see `['Highest', 'High', ...]` because there is
# no `\|`. High/Medium/Low are omitted — too many other meanings.
#
# Done-status display names (GDK-272): Jira-default KO names that used to
# be a fallback set when status_category was absent. Lowercase 'done' /
# 'resolved' / 'closed' are not listed — they are also category keys or
# other-field values (RangeField 'resolved', GitHub PR 'closed').
#
# Excluded (2026-09-06, r2-detail review): web/src/lib/done-words.ts. Its
# '완료' is not a status name keyed for logic — it is the comment-text
# heuristic list `gadak retro` calls mismatch (internal/retro DoneWords, kept
# in lockstep by web/src/lib/done-words.test.ts). Matching a comment body is
# the point of that file; nothing in it compares a status/priority/type.
name_hits="$(
  grep -RInE \
    --include='*.ts' --include='*.svelte' \
    --exclude='*.test.ts' --exclude='*.spec.ts' \
    -e 'highest\|긴급|긴급\|가장 높음|가장 높음\|blocker' \
    -e 'high\|높음\|major' \
    -e 'medium\|보통\|normal' \
    -e 'lowest\|가장 낮음|매우 \?낮음\|trivial' \
    -e 'low\|낮음\|minor' \
    -e "status[[:space:]]*===[[:space:]]*['\"]In Progress['\"]" \
    -e "status[[:space:]]*===[[:space:]]*['\"]진행 중['\"]" \
    -e "issue_type[[:space:]]*===[[:space:]]*['\"]Bug['\"]" \
    -e "issue_type[[:space:]]*===[[:space:]]*['\"]버그['\"]" \
    -e "name[[:space:]]*\.[[:space:]]*toLowerCase\(\)[[:space:]]*===[[:space:]]*['\"](bug|story|task|epic|sub-task|버그|스토리|작업|에픽)['\"]" \
    -e "['\"]Highest['\"]" \
    -e "['\"]Lowest['\"]" \
    -e "['\"]In Progress['\"]" \
    -e "['\"]Sub-task['\"]" \
    -e "['\"]해결됨['\"]" \
    -e "['\"]종료['\"]" \
    -e "['\"]완료['\"]" \
    web/src/lib web/src/components web/src/stores \
    | grep -v '/i18n/' \
    | grep -v 'web/src/lib/done-words.ts' \
    || true
)"
if [[ -n "$name_hits" ]]; then
  fail "web logic keys status/priority/type on a display name — send GET /priorities/ catalog id via create.PriorityField(id); names follow the account language:"$'\n'"$name_hits"
fi
ok "web logic does not key status/priority/type on a display name"

# Go writes must send priority/status/issuetype by id, never by name.
# This is the real closure: every surface (web, CLI, future) hits Jira here.
go_name_hits="$(
  grep -RInE \
    --include='*.go' --exclude='*_test.go' \
    -e '\["(priority|status|issuetype)"\][[:space:]]*=[[:space:]]*map\[string\]string\{[[:space:]]*"name"' \
    internal cmd \
    || true
)"
if [[ -n "$go_name_hits" ]]; then
  fail "Go write sends priority/status/issuetype by name — send priority via create.PriorityField(id); names follow the account language:"$'\n'"$go_name_hits"
fi
ok "Go writes do not send priority/status/issuetype by display name"

# ── 8. PROMISES outbound list agrees with SECURITY.md ──────────────────
# SECURITY.md enumerates outbound destinations as a numbered list under
# "Outbound traffic is exactly N destinations:"; docs/PROMISES.md repeats that set
# in an <!-- outbound: A | B --> marker beside its own outbound promise. A
# destination added to one file must not leave the other claiming fewer.
outbound_diff="$(python3 - <<'PY'
import re
from pathlib import Path

words = {"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9}
norm = lambda s: re.sub(r"\W+", " ", s).strip().lower()

sec = Path("SECURITY.md").read_text()
# Everything from the claim to the next heading: a destination appended after
# a blank line is still inside the section and must still be counted.
m = re.search(r"Outbound traffic is exactly (\w+) destinations?:\n(.*?)(?=\n## |\Z)", sec, re.S)
if not m:
    print('SECURITY.md: no "Outbound traffic is exactly N destinations:" list')
    raise SystemExit
declared = re.findall(r"^\d+\.\s+\*\*(.+?)\*\*", m.group(2), re.M)
if words.get(m.group(1)) != len(declared):
    print(f'SECURITY.md says "{m.group(1)}" destinations but lists {len(declared)}')
    raise SystemExit

pm = re.search(r"<!--\s*outbound:(.+?)-->", Path("docs/PROMISES.md").read_text())
if not pm:
    print("docs/PROMISES.md: no <!-- outbound: … --> marker")
    raise SystemExit
listed = {norm(x) for x in pm.group(1).split("|")}
if listed != {norm(x) for x in declared}:
    print(f"docs/PROMISES.md lists {sorted(listed)}; SECURITY.md declares "
          f"{sorted(norm(x) for x in declared)}")
PY
)"
if [[ -n "$outbound_diff" ]]; then
  fail "docs/PROMISES.md outbound list disagrees with SECURITY.md:"$'\n'"$outbound_diff"
fi
ok "docs/PROMISES.md outbound list matches SECURITY.md"

# ── 9. Both onboarding paths warn about the token traps before the 401 ──
# The web form and `gadak init` ask for the same token, and Atlassian's page
# offers three things that look like one: a scoped token (recommended first),
# an org key from admin.atlassian.com, and the user token that actually works.
# Each surface carries its own copy — TS and Go share no string table — so the
# invariant is pinned here instead: whoever edits one is told about the other.
token_copy_missing=""
# Web copy is one {en,ko,ja} object in messages/write.ts, not en.ts/ko.ts.
# Still require the English and Korean lines separately so a locale cannot
# drop a trap while another locale's substring keeps the grep green.
hint_ts="$(sed -n "/'onboarding.tokenHint'/,/^  },$/p" web/src/lib/i18n/messages/write.ts)"
for loc in en ko; do
  line="$(printf '%s\n' "$hint_ts" | grep -E "^    ${loc}: ")"
  if [[ -z "$line" ]]; then
    token_copy_missing+="  messages/write.ts: missing ${loc} for onboarding.tokenHint"$'\n'
    continue
  fi
  # ATATT/ATCTT are Atlassian's own prefixes and read the same in every
  # locale; the scoped-token trap is prose, so the Korean copy says 스코프.
  for trap in 'ATATT' 'ATCTT' 'scope|스코프'; do
    grep -Eqi -- "$trap" <<<"$line" || token_copy_missing+="  messages/write.ts ${loc}: token hint does not name ${trap%%|*}"$'\n'
  done
done
hint_go="$(sed -n '/tokenTrapHint/,/^$/p' cmd/gadak/init.go)"
for trap in 'ATATT' 'ATCTT' 'scope|스코프'; do
  grep -Eqi -- "$trap" <<<"$hint_go" || token_copy_missing+="  cmd/gadak/init.go: token hint does not name ${trap%%|*}"$'\n'
done
if [[ -n "$token_copy_missing" ]]; then
  fail "an onboarding surface asks for a token without naming every trap (GDK-98):"$'\n'"$token_copy_missing"
fi
ok "web + CLI token prompts both name the scoped / ATATT / ATCTT traps"

# ── 10. One owner for the Node version ──────────────────────────────────
# GDK-57: the hosted fetch adapter read a bare `navigator`, which Node has had
# since 21. Local (24) was green, CI (20) was red, and the gap hid the defect
# through several pushes. A version pinned in five workflow files and nowhere
# a developer's shell can read is a gap that reopens on its own, so .nvmrc is
# the single owner and every workflow reads it.
if [[ ! -f .nvmrc ]]; then
  fail ".nvmrc is missing — it is the one place the Node version is declared (GDK-57)"
fi
hardcoded="$(grep -rn 'node-version:' .github/workflows/ || true)"
if [[ -n "$hardcoded" ]]; then
  fail "a workflow pins Node inline instead of reading .nvmrc (GDK-57):"$'\n'"$hardcoded"
fi
ok "Node version has one owner (.nvmrc), read by every workflow"

# ── 11. A Playwright webServer command names its own interpreter ─────────
# Same class as check 10, different shell: Playwright hands `webServer.command`
# to /bin/sh, which is bash on a developer's mac and dash on the CI runner. The
# hosted config opened its inline command with `set -euo pipefail` and died on
# the runner with "Illegal option -o pipefail" — after passing every local run
# (GDK-52). An inline string does not say which shell reads it, so bash-only
# syntax there is green until the one machine that matters disagrees. Put the
# script in a file with a shebang and invoke it as `bash <path>`, the way
# e2e/serve.sh already does.
bashism_inline=""
while IFS= read -r cfg; do
  # The command value only; a bash-only construct in a comment is not a bug.
  cmd="$(sed -n "/command:/,/,$/p" "$cfg")"
  for bashism in 'pipefail' '\[\[' '<<<' '\$\{[A-Za-z_]*[/^,]'; do
    if grep -Eq -- "$bashism" <<<"$cmd" && ! grep -Eq -- "command: *'bash " <<<"$cmd"; then
      bashism_inline+="  $cfg: inline webServer.command uses bash-only syntax (${bashism})"$'\n'
    fi
  done
done < <(find e2e -name 'playwright.config.ts')
if [[ -n "$bashism_inline" ]]; then
  fail "a Playwright webServer command relies on bash but is run by /bin/sh (dash on CI, GDK-52):"$'\n'"$bashism_inline"
fi
ok "Playwright webServer commands are POSIX or name bash explicitly"

# ── 12. The derived-field rule table has one home ────────────────────────
# GDK-88: what a derived column means and how it is made lives in
# docs/DERIVE.md and only there. The rule table is recognized by its header
# row, `| Field | Rule |` — the schema column tables use different headers
# (`| Column | Type | Notes |`) and must not trip this check. A copy that
# springs up anywhere else is the next drift: two tables, one of them stale.
rule_table_hits="$(
  grep -rFn --include='*.md' '| Field | Rule |' \
    AGENTS.md skills/gadak/SKILL.md specs/000-product/data-model.md docs \
    | grep -v '^docs/DERIVE.md:' \
    || true
)"
if [[ -n "$rule_table_hits" ]]; then
  fail "the derived-field rule table exists outside docs/DERIVE.md (GDK-88):"$'\n'"$rule_table_hits"
fi
ok "derived-field rule table lives only in docs/DERIVE.md"

# ── 13. Windows install exists; never tell a user to turn SAC off ────────
# GDK-244/245: 0.16 ships an unsigned Windows desktop zip. README used to
# say "CLI only (macOS + Linux)" and "the macOS app" as if that were the
# only window. A check that only pins the version (check 6) would stay
# green while those sentences came back. The SAC rule is sharper: Microsoft
# documents no per-app override, and this project must not offer "turn
# Smart App Control off" as a workaround (once offered, someone will).
win_zip_missing=""
for f in README.md README.ko.md README.ja.md docs/INSTALL.md docs/DESKTOP.md; do
  grep -q 'windows-x64' "$f" || win_zip_missing+="  $f: no windows-x64 asset name"$'\n'
done
if [[ -n "$win_zip_missing" ]]; then
  fail "Windows desktop zip is not named in the install docs (GDK-245):"$'\n'"$win_zip_missing"
fi
if ! grep -q 'Smart App Control' docs/INSTALL.md; then
  fail "docs/INSTALL.md does not name Smart App Control (GDK-245)"
fi
if ! grep -q 'GDK-211' docs/INSTALL.md; then
  fail "docs/INSTALL.md does not name GDK-211 (signing planned, no date)"
fi
sac_off="$(
  grep -nEi 'turn(ing)?[[:space:]]+((smart[[:space:]]+app[[:space:]]+control)|SAC)[[:space:]]+off|disable[[:space:]]+((smart[[:space:]]+app[[:space:]]+control)|SAC)|스마트[[:space:]]*앱[[:space:]]*제어를[[:space:]]*끄' \
    README.md README.ko.md README.ja.md docs/INSTALL.md docs/DESKTOP.md \
    || true
)"
# The docs must mention the prohibition. A line that says "Do not turn …
# off" is the contract; a line that tells the user to turn it off is not.
# Fail only when an imperative/how-to (turn/disable) is not a prohibition.
if [[ -n "$sac_off" ]]; then
  bad_off=""
  while IFS= read -r line; do
    echo "$line" | grep -Eiq 'do[[:space:]]*(\*\*)?not|don'\''t|never|끄지 마' && continue
    bad_off+="  $line"$'\n'
  done <<< "$sac_off"
  if [[ -n "$bad_off" ]]; then
    fail "install docs tell the user to turn Smart App Control off:"$'\n'"$bad_off"
  fi
fi
ok "Windows desktop zip is documented; SAC-off is not offered"

# ── 14. matchesIdFirst id args are real issues columns (GDK-275) ────────
# The display-name grep (check 7) cannot see this class: an id-first call
# whose id argument names a column that does not exist is silently
# name-only. FAIL-first 2026-08-19: web filter passed it.priority_id while
# schema.go had no issues.priority_id (only name + rank).
id_first_missing="$(
  python3 - <<'PY'
import re
from pathlib import Path

# Columns created on issues, plus every later ADD COLUMN.
schema = Path("internal/store/schema.go").read_text()
cols = set()
m = re.search(r"CREATE TABLE issues\s*\((.*?)\)\s*;", schema, flags=re.S)
if not m:
    print("schema.go: no CREATE TABLE issues")
    raise SystemExit
for part in m.group(1).split(","):
    name = part.strip().split()
    if name:
        cols.add(name[0])
for add in re.finditer(r"ALTER TABLE issues ADD COLUMN (\w+)", schema):
    cols.add(add.group(1))

# Id argument of matchesIdFirst (and a call that wraps it): it.<field>
# in web logic. Tests/i18n are excluded the same way as check 7.
field_re = re.compile(
    r"matchesIdFirst\s*\([^,]+,\s*it\.([A-Za-z0-9_]+)",
)
missing = []
roots = [Path("web/src/lib"), Path("web/src/components"), Path("web/src/stores")]
for root in roots:
    for path in root.rglob("*"):
        if path.suffix not in {".ts", ".svelte"}:
            continue
        if path.name.endswith(".test.ts") or path.name.endswith(".spec.ts"):
            continue
        if "/i18n/" in path.as_posix():
            continue
        text = path.read_text()
        for n, line in enumerate(text.splitlines(), 1):
            for field in field_re.findall(line):
                if field not in cols:
                    missing.append(f"{path}:{n}: it.{field} is not an issues column")
if missing:
    print("\n".join(missing))
PY
)"
if [[ -n "$id_first_missing" ]]; then
  fail "matchesIdFirst id argument is not an issues column (GDK-275):"$'\n'"$id_first_missing"
fi
ok "matchesIdFirst id arguments are issues columns"

# ── 15. One owner for "what category is this status?" (GDK-279) ─────────
# The display-name grep (check 7) cannot see this class: two functions that
# both lowercase a raw status_category and return the 3-bucket set can
# disagree on the same key (list used effectiveCategory, the transition
# control used normalizeCategory; complete/todo painted differently).
# The owner is effectiveCategory. A second mapper of that shape is the
# recurrence. Color/rank adapters that consume the owner's buckets are not
# mappers. FAIL-first 2026-08-19: unmodified tree listed effectiveCategory
# and normalizeCategory.
second_category_mappers=$(
  python3 - <<'GDK279PY'
import re
from pathlib import Path

FUNC_START = re.compile(r"(?:export\s+)?function\s+([A-Za-z0-9_]+)\s*\(")
# Bare bucket return — not return "bg-status-done" or return categoryMetaOf(...).
BUCKET_RETURN = re.compile(r"return\s+['\"](?:new|inprogress|done)['\"]")
HAS_NEW = re.compile(r"['\"]new['\"]")
HAS_DONE = re.compile(r"['\"]done['\"]")
# `const c = (key || '').toLowerCase()` — the local a mapper then branches on.
LOWERED_ASSIGN = re.compile(r"\b(?:const|let|var)\s+([A-Za-z0-9_$]+)\s*=[^;\n]*\.toLowerCase\(\)")
# `key.toLowerCase() === 'done'` — the same decision without the local.
LOWERED_INLINE_CMP = re.compile(
    r"\.toLowerCase\(\)\s*[=!]==?\s*['\"](?:new|inprogress|done)['\"]"
    r"|['\"](?:new|inprogress|done)['\"]\s*[=!]==?\s*[^;\n]*\.toLowerCase\(\)"
)

def function_bodies(text):
    for m in FUNC_START.finditer(text):
        name = m.group(1)
        depth_paren = 0
        depth_brace = 0
        body_start = None
        for j in range(m.end() - 1, len(text)):
            ch = text[j]
            if ch == "(":
                depth_paren += 1
            elif ch == ")":
                depth_paren -= 1
            elif ch == "{" and depth_paren == 0:
                if body_start is None:
                    body_start = j
                    depth_brace = 1
                else:
                    depth_brace += 1
            elif ch == "}" and depth_paren == 0 and body_start is not None:
                depth_brace -= 1
                if depth_brace == 0:
                    yield name, m.start(), text[body_start : j + 1]
                    break

def is_category_mapper(body):
    # Class: lowercase a raw key yourself, then compare *that* against the
    # bucket names. What the function returns is not the tell — BulkBar's
    # catDot returned 'bg-status-done' and its jiraRank returned a number, and
    # both were still second opinions on "what category is this?" (found by the
    # lead reviewing GDK-279, after a return-shaped version of this check
    # missed them).
    #
    # Two shapes are deliberately not this class:
    #   - an adapter that calls effectiveCategory and switches on its result —
    #     it lowercases nothing of its own;
    #   - BreakdownBar's groupColor, which lowercases for 'fail'/'pass' and
    #     delegates the category branch to categoryMetaOf. Merely mentioning
    #     'new' and 'done' somewhere in the body is not enough.
    if ".toLowerCase(" not in body:
        return False
    if not (HAS_NEW.search(body) and HAS_DONE.search(body)):
        return False
    if LOWERED_INLINE_CMP.search(body):
        return True
    for var in {m.group(1) for m in LOWERED_ASSIGN.finditer(body)}:
        if re.search(r"\b%s\b\s*[=!]==?\s*['\"](?:new|inprogress|done)['\"]" % re.escape(var), body):
            return True
        if re.search(r"['\"](?:new|inprogress|done)['\"]\s*[=!]==?\s*\b%s\b" % re.escape(var), body):
            return True
    return False

hits = []
roots = [Path("web/src/lib"), Path("web/src/components"), Path("web/src/stores")]
for root in roots:
    for path in root.rglob("*"):
        if path.suffix not in {".ts", ".svelte"}:
            continue
        if path.name.endswith(".test.ts") or path.name.endswith(".spec.ts"):
            continue
        if "/i18n/" in path.as_posix():
            continue
        text = path.read_text()
        for name, start, body in function_bodies(text):
            if not is_category_mapper(body):
                continue
            line = text.count("\n", 0, start) + 1
            hits.append((path.as_posix(), line, name))

owners = [h for h in hits if h[2] == "effectiveCategory"]
others = [h for h in hits if h[2] != "effectiveCategory"]
if len(owners) != 1 or others:
    for p, n, name in hits:
        print("%s:%s: %s" % (p, n, name))
GDK279PY
)
if [[ -n "$second_category_mappers" ]]; then
  fail "web has a second status-category mapper — fold aliases in effectiveCategory only (GDK-279):"$'\n'"$second_category_mappers"
fi
ok "effectiveCategory is the only status-category mapper in web/"

# ── 16. Doc open-mode claims match the surface's store.Open* (GDK-306) ──
# Class: a sentence that says how a surface opens the mirror must name the
# function that surface actually calls. This is not "FAQ contains store.Open"
# — if ensureDB switches to OpenReadOnly, a sentence that still says
# store.Open must fail, and the reverse too.
#
# Owners are discovered from package layout, not a roster of today's files:
#   the MCP server → store.Open* in internal/mcp/*.go (non-test)
#   gadak sql      → store.Open* in cmd/gadak/sql.go
# gadak_query / "query path" / "query tool" is the SQL connection
# (sqlguard.runQuery, mode=ro), not the server open — those sentences are
# not an MCP-server claim.
#
# FAIL-first 2026-08-19: docs/FAQ.md said the MCP server opens the file
# read-only / mode=ro while internal/mcp/server.go called store.Open.
open_mode_drift=$(
  python3 - "$FILE_CENSUS" <<'GDK306PY'
import re
import sys
from pathlib import Path

# GDK-1477: the shared census (see the prelude) instead of a private walk.
CENSUS = [Path(p) for p in Path(sys.argv[1]).read_text().splitlines() if p]

OPEN_CALL = re.compile(r"\bstore\.(OpenReadOnly|Open)\s*\(")
QUERY_PATH = re.compile(r"gadak_query|query path|query tool", re.I)
MCP_SURFACE = re.compile(r"\bthe MCP server\b|\bgadak mcp\b", re.I)
SQL_SURFACE = re.compile(r"\bgadak sql\b", re.I)
NAMED_RO = re.compile(r"\bstore\.OpenReadOnly\b")
NAMED_RW = re.compile(r"\bstore\.Open\b")
MODE_RO = re.compile(r"mode=ro|\bread-only\b|읽기 전용", re.I)
# scratch/ is gitignored session notes (not live docs). Leaving it in
# the walk failed this check on scratch/yc/r3f-boris.md (2026-08-20)
# while CI, which never sees that tree, stayed green.
SKIP_DIR = {"decisions", "node_modules", "dist", "e2e", "web", "scratch"}
SKIP_FILE = {"CHANGELOG.md"}


def strip_go_comments(text):
    text = re.sub(r"/\*.*?\*/", " ", text, flags=re.S)
    out = []
    for line in text.splitlines():
        if "//" in line:
            line = line[: line.index("//")]
        out.append(line)
    return "\n".join(out)


def open_calls(paths):
    found = []
    for path in paths:
        if path.name.endswith("_test.go"):
            continue
        text = strip_go_comments(path.read_text())
        for i, line in enumerate(text.splitlines(), 1):
            for m in OPEN_CALL.finditer(line):
                found.append((path.as_posix(), i, m.group(1)))
    return found


def one_open(label, found):
    if not found:
        print("%s: no store.Open* call in production sources" % label)
        return None
    names = {n for _, _, n in found}
    if len(names) != 1:
        print("%s: mixed store.Open* calls — %s" % (
            label, "; ".join("%s:%s %s" % t for t in found)))
        return None
    return names.pop()


def sentences(text):
    tick = chr(96)
    fence = tick * 3
    text = re.sub(re.escape(fence) + ".*?" + re.escape(fence), " ", text, flags=re.S)
    out = []
    buf = []
    in_tick = False
    for ch in text:
        if ch == tick:
            in_tick = not in_tick
            buf.append(ch)
            continue
        if not in_tick and ch == ".":
            buf.append(ch)
            s = "".join(buf).strip()
            if s:
                out.append(re.sub(r"\s+", " ", s))
            buf = []
            continue
        buf.append(ch)
    tail = re.sub(r"\s+", " ", "".join(buf)).strip()
    if tail:
        out.append(tail)
    return out


def named_open(sentence):
    if NAMED_RO.search(sentence):
        return "OpenReadOnly"
    if NAMED_RW.search(sentence):
        return "Open"
    return None


def mode_open(sentence):
    if MODE_RO.search(sentence):
        return "OpenReadOnly"
    return None


mcp_open = one_open("MCP server", open_calls(Path("internal/mcp").glob("*.go")))
sql_open = one_open("gadak sql", open_calls([Path("cmd/gadak/sql.go")]))
if mcp_open is None or sql_open is None:
    raise SystemExit

docs = []
for path in CENSUS:
    if path.suffix != ".md":
        continue
    if any(p in SKIP_DIR for p in path.parts):
        continue
    if path.name in SKIP_FILE:
        continue
    docs.append(path)

hits = []
for path in sorted(docs):
    for sent in sentences(path.read_text()):
        if MCP_SURFACE.search(sent):
            claimed = named_open(sent)
            if claimed is None and not QUERY_PATH.search(sent):
                claimed = mode_open(sent)
            if claimed and claimed != mcp_open:
                hits.append("%s: MCP server claim %s but internal/mcp calls store.%s — %s" % (
                    path.as_posix(), claimed, mcp_open, sent[:160]))
        if SQL_SURFACE.search(sent):
            claimed = named_open(sent) or mode_open(sent)
            if claimed and claimed != sql_open:
                hits.append("%s: gadak sql claim %s but cmd/gadak/sql.go calls store.%s — %s" % (
                    path.as_posix(), claimed, sql_open, sent[:160]))

if hits:
    print("\n".join(hits))
GDK306PY
)
if [[ -n "$open_mode_drift" ]]; then
  fail "a doc claim about how a surface opens the mirror disagrees with that surface's store.Open* call (GDK-306):"$'\n'"$open_mode_drift"
fi
ok "doc claims about how MCP / gadak sql open the mirror match store.Open*"

# ── 17. Documented GADAK_* names are actually read (GDK-281) ────────────
# Class: a live document names GADAK_FOO, but no source constructs or
# Getenvs that name. Agents then export a ghost and operate on the default
# mirror. FAIL-first 2026-08-19: specs/000-product/data-model.md named
# GADAK_DB; identity.go builds names as EnvPrefix+suffix, and no call
# site passes "DB".
#
# Documented set — live prose an agent or human follows:
#   docs/ except docs/decisions/ (append-only records of past states)
#   specs/
#   AGENTS.md (repo development contract; the product cookbook is
#              docs/MIRROR.md, already covered by the docs/ walk;
#              same inclusion as checks 5 and 12)
# Deliberately not scanned:
#   CHANGELOG.md          — history; a removed var stays in old entries
#   docs/decisions/       — addendum-only; naming a since-removed var is a record
#   README.md / *.ko.md   — install front door; env names there are restated
#                           in docs/CONFIGURATION.md which is scanned
#   fenced "don't do this" blocks are still scanned: an agent copies examples,
#   and detecting prohibition-only fences is not mechanical.
#
# Read set — derived from source, not a hand-maintained suffix list:
#   config.Env("SUFFIX") / Env("SUFFIX")  → GADAK_SUFFIX
#     (identity.go concatenates EnvPrefix+suffix; a literal grep for
#     GADAK_HOME would miss the constructor)
#   os.Getenv/LookupEnv/Setenv("GADAK_X") in non-test .go (comments stripped,
#     same as check 16 — mcp.go's "GADAK_DB is not used" comment is not a read)
#   process.env.GADAK_X, os.environ["GADAK_X"] / .get("GADAK_X"),
#   $GADAK_X / ${GADAK_X} in sh/Makefile
# Go *_test.go is excluded (t.Setenv GADAK_HOME is a fixture, not a read).
# TypeScript spec files stay: GADAK_MEDIA / GADAK_PERF are consumed there
# as opt-in gates, which is what the docs claim they do.
#
# Flag half is not in this check: the only global flags are the three
# pre-subcommand tokens in cmd/gadak/main.go (--profile/-p, --help/-h,
# --version/-v). --db exists on demo and export-static, so a token-presence
# scan would stay green while a document still claimed it moved the mirror.
# That is the decorative shape GDK-288 declined. Env names are the axis
# that can be derived honestly.
env_ghosts=""
env_ghosts=$(python3 - "$FILE_CENSUS" <<'GDK281PY'
import re
import sys
from pathlib import Path

# GDK-1477: the shared census (see the prelude) instead of two private walks.
CENSUS = [Path(p) for p in Path(sys.argv[1]).read_text().splitlines() if p]

dq = chr(34)
sq = chr(39)
NAME = re.compile(r"\bGADAK_[A-Z][A-Z0-9_]*")
ENV_CALL = re.compile(r"(?:config\.)?Env\(\s*" + dq + r"([A-Z][A-Z0-9_]*)" + dq + r"\s*\)")
GO_GET = re.compile(
    r"(?:Getenv|LookupEnv|Setenv)\(\s*" + dq + r"GADAK_([A-Z][A-Z0-9_]*)" + dq
)
JS_ENV = re.compile(r"process\.env\.GADAK_([A-Z][A-Z0-9_]*)")
PY_ENV = re.compile(
    r"os\.environ(?:\.get)?\(\s*[" + sq + dq + r"]GADAK_([A-Z][A-Z0-9_]*)[" + sq + dq + r"]"
    r"|os\.environ\[\s*[" + sq + dq + r"]GADAK_([A-Z][A-Z0-9_]*)[" + sq + dq + r"]\s*\]"
)
SH_ENV = re.compile(r"[$][{]?GADAK_([A-Z][A-Z0-9_]*)")

SKIP_DOC_DIR = {"decisions", "node_modules", "dist", "media"}
SKIP_READ_DIR = {"node_modules", "dist", ".git"}
DOC_ROOTS = [Path("docs"), Path("specs"), Path("AGENTS.md")]


def strip_go_comments(text):
    text = re.sub(r"/\*.*?\*/", " ", text, flags=re.S)
    out = []
    for line in text.splitlines():
        if "//" in line:
            line = line[: line.index("//")]
        out.append(line)
    return "\n".join(out)


def documented():
    names = {}
    files = []
    for root in DOC_ROOTS:
        if root.is_file():
            files.append(root)
            continue
        for path in root.rglob("*.md"):
            if any(p in SKIP_DOC_DIR for p in path.parts):
                continue
            files.append(path)
    for path in files:
        for n, line in enumerate(path.read_text().splitlines(), 1):
            for m in NAME.finditer(line):
                names.setdefault(m.group(0), []).append("%s:%s" % (path.as_posix(), n))
    return names


# The suffix dispatch below is the whole body of the read loop: a file whose
# suffix is not one of these (and is not a Makefile) falls through it without
# touching `found`. Deciding that before read_text is what stops this check
# reading mobile/src-tauri/target — 12.4 GB of Rust build artifacts that used
# to cost 15s of the gate, decoded and discarded (GDK-1477).
READ_SUFFIXES = {".go", ".ts", ".js", ".mjs", ".py", ".sh"}


def read_names():
    found = set()
    for path in CENSUS:
        if any(p in SKIP_READ_DIR for p in path.parts):
            continue
        name = path.name
        suffix = path.suffix
        if suffix not in READ_SUFFIXES and name != "Makefile":
            continue
        try:
            text = path.read_text()
        except (UnicodeDecodeError, OSError):
            continue
        if suffix == ".go":
            if name.endswith("_test.go"):
                continue
            body = strip_go_comments(text)
            for m in ENV_CALL.finditer(body):
                found.add("GADAK_" + m.group(1))
            for m in GO_GET.finditer(body):
                found.add("GADAK_" + m.group(1))
            continue
        if suffix in {".ts", ".js", ".mjs"}:
            for m in JS_ENV.finditer(text):
                found.add("GADAK_" + m.group(1))
            continue
        if suffix == ".py":
            for m in PY_ENV.finditer(text):
                found.add("GADAK_" + (m.group(1) or m.group(2)))
            continue
        if suffix == ".sh" or name == "Makefile":
            for m in SH_ENV.finditer(text):
                found.add("GADAK_" + m.group(1))
    return found


def published_names():
    """Names gadak *sets* for a process it starts, rather than reads.

    A marker like GADAK_TERMINAL is the opposite of a ghost: the binary
    published it, and prose is right to name it. Two independent sources
    have to agree, so the check cannot be satisfied by editing either one:
    the runtime census (identity.go's envPublished, which is what stops
    warnUnknownGADAK calling the marker unrecognised) and an actual
    `"GADAK_X=..."` assignment in non-test Go.
    """
    census = set()
    ident = Path("internal/config/identity.go")
    if ident.is_file():
        body = strip_go_comments(ident.read_text())
        block = re.search(r"envPublished\s*=\s*map\[string\]struct\{\}\{(.*?)\}", body, re.S)
        if block:
            census = set(re.findall(dq + r"(GADAK_[A-Z][A-Z0-9_]*)" + dq, block.group(1)))
    assigned = set()
    for path in CENSUS:
        if path.suffix != ".go":
            continue
        if any(p in SKIP_READ_DIR for p in path.parts) or path.name.endswith("_test.go"):
            continue
        try:
            body = strip_go_comments(path.read_text())
        except (UnicodeDecodeError, OSError):
            continue
        assigned |= set(re.findall(dq + r"(GADAK_[A-Z][A-Z0-9_]*)=", body))
    return census & assigned


docs = documented()
read = read_names() | published_names()
ghosts = sorted(n for n in docs if n not in read)
for n in ghosts:
    where = ", ".join(docs[n][:4])
    print("%s documented at %s but nothing reads it" % (n, where))
GDK281PY
)
if [[ -n "$env_ghosts" ]]; then
  fail "a document names a GADAK_* variable nothing reads (GDK-281):"$'\n'"$env_ghosts"
fi
ok "documented GADAK_* names are read by the source"

# ── 18. The runtime GADAK_* allowlist matches what the Go source reads ──
# Class: internal/config/identity.go carries a hand-written census of the
# names this product reads, and warnUnknownGADAK tells the user everything
# outside it is ignored. When a new Env("X") call site lands and the census
# does not, the binary starts calling one of its own variables a ghost.
# Check 17 answers "is a documented name read"; this answers "does the
# process know what it reads". Derived the same way — Env("SUFFIX") and
# Getenv/LookupEnv/Setenv("GADAK_X") in non-test .go, comments stripped —
# so the check cannot be satisfied by editing a list.
#
# envHarness is deliberately not policed: those names (GADAK_MEDIA and
# friends) are read by scripts, not by the binary, and a stale entry there
# costs one spurious stderr line, never a ghost that stays silent.
env_census=""
env_census=$(python3 - "$FILE_CENSUS" <<'GDK281CENSUSPY'
import re
import sys
from pathlib import Path

# GDK-1477: the shared census (see the prelude) instead of a private walk.
CENSUS = [Path(p) for p in Path(sys.argv[1]).read_text().splitlines() if p]

dq = chr(34)
ENV_CALL = re.compile(r"(?:config\.)?Env\(\s*" + dq + r"([A-Z][A-Z0-9_]*)" + dq + r"\s*\)")
GO_GET = re.compile(
    r"(?:Getenv|LookupEnv|Setenv)\(\s*" + dq + r"GADAK_([A-Z][A-Z0-9_]*)" + dq
)
SKIP = {"node_modules", "dist", ".git"}


def strip_go_comments(text):
    text = re.sub(r"/\*.*?\*/", " ", text, flags=re.S)
    out = []
    for line in text.splitlines():
        if "//" in line:
            line = line[: line.index("//")]
        out.append(line)
    return "\n".join(out)


read = {}
for path in CENSUS:
    if path.suffix != ".go":
        continue
    if any(p in SKIP for p in path.parts) or path.name.endswith("_test.go"):
        continue
    try:
        body = strip_go_comments(path.read_text())
    except (UnicodeDecodeError, OSError):
        continue
    for n, line in enumerate(body.splitlines(), 1):
        for m in ENV_CALL.finditer(line):
            read.setdefault("GADAK_" + m.group(1), []).append("%s:%s" % (path.as_posix(), n))
        for m in GO_GET.finditer(line):
            read.setdefault("GADAK_" + m.group(1), []).append("%s:%s" % (path.as_posix(), n))

identity = Path("internal/config/identity.go").read_text()


def block(name):
    m = re.search(r"var " + name + r" = map\[string\]struct\{\}\{(.*?)\n\}", identity, re.S)
    return m.group(1) if m else ""


known = {"GADAK_" + s for s in re.findall(dq + r"([A-Z][A-Z0-9_]*)" + dq, block("envSuffixes"))}
known |= set(re.findall(dq + r"(GADAK_[A-Z][A-Z0-9_]*)" + dq, block("envLiterals")))
if not known:
    print("could not parse envSuffixes/envLiterals out of internal/config/identity.go")
for name in sorted(n for n in read if n not in known):
    print("%s is read at %s but is not in identity.go's allowlist — "
          "warnUnknownGADAK would call it unrecognised" % (name, read[name][0]))
GDK281CENSUSPY
)
if [[ -n "$env_census" ]]; then
  fail "the runtime GADAK_* allowlist is behind the source (GDK-281):"$'\n'"$env_census"
fi
ok "internal/config/identity.go's GADAK_* allowlist covers every name the Go source reads"

# ── 19. vnc-snap.py's --do actions are documented where they are used ───
# Class: tools/vnc-snap.py is driven from docs/runbooks/omarchy-vm.md, and the
# operator's only channel to that guest is those keystrokes. An action that
# exists in the code but nowhere in the runbook is an action nobody will reach;
# one named in the runbook but absent from the code fails mid-round on a machine
# that costs a round to retry. typeenv is the case that made this worth a gate:
# it is the only action safe for a secret, and a reader who does not know it
# exists will use type: and put a password in ps.
# Derived from the dispatch itself (raw == "x" / raw.startswith("x:")), so it
# cannot be satisfied by editing a list.
vnc_actions=""
vnc_actions=$(python3 - <<'GDKVNCPY'
import re
from pathlib import Path

src = Path("tools/vnc-snap.py")
runbook = Path("docs/runbooks/omarchy-vm.md")
if not src.is_file():
    raise SystemExit(0)  # tool removed: nothing to police
body = src.read_text()

m = re.search(r"\ndef run_actions\(.*?\n(?=\ndef |\Z)", body, re.S)
if not m:
    print("could not find run_actions in tools/vnc-snap.py")
    raise SystemExit(0)
loop = m.group(0)

dq = chr(34)
actions = set(re.findall(r"raw == " + dq + r"([a-z]+)" + dq, loop))
actions |= set(re.findall(r"raw\.startswith\(" + dq + r"([a-z]+):" + dq, loop))
if not actions:
    print("parsed no --do actions out of run_actions")
    raise SystemExit(0)

help_m = re.search(r"metavar=" + dq + r"ACTION" + dq + r"(.*?)\n\s*\)", body, re.S)
help_text = help_m.group(1) if help_m else ""
doc_text = runbook.read_text() if runbook.is_file() else ""

for name in sorted(actions):
    if name not in help_text:
        print("%s is implemented but --do help does not name it" % name)
    if doc_text and name not in doc_text:
        print("%s is implemented but docs/runbooks/omarchy-vm.md does not name it" % name)
GDKVNCPY
)
if [[ -n "$vnc_actions" ]]; then
  fail "tools/vnc-snap.py has an undocumented --do action:"$'\n'"$vnc_actions"
fi
ok "every tools/vnc-snap.py --do action is named in its own help and in the runbook"

# ── shared dependency pins agree across modules ───────────────────────────
# The desktop module resolves issuetap through the root module; bumping the
# root pin without `cd desktop && go mod tidy` builds green locally (separate
# module, not in ./...) and dies only on the CI pack step (run 32249534581).
root_pin="$(grep -o 'github.com/midagedev/issuetap v[^ ]*' go.mod | head -1)"
desktop_pin="$(grep -o 'github.com/midagedev/issuetap v[^ ]*' desktop/go.mod | head -1)"
if [[ -n "$root_pin" && -n "$desktop_pin" && "$root_pin" != "$desktop_pin" ]]; then
  fail "issuetap pin differs: go.mod has ${root_pin#*issuetap } but desktop/go.mod has ${desktop_pin#*issuetap } — run: cd desktop && go mod tidy"
fi
ok "issuetap pin agrees between go.mod and desktop/go.mod"

# ── 20. User docs do not betray standalone (GDK-271 / 373) ──────────────
# Class: v0.16's headline is a workspace with no Atlassian account, but the
# install/FAQ front door still spoke as if every workspace needed a Cloud
# token and could be deleted with `rm -rf ~/.gadak` (that one-liner wipes
# the standalone origin persist file).
# FAIL-first 2026-08-20 against the unmodified tree:
#   docs/INSTALL.md:9  "Atlassian Cloud only"
#   docs/FAQ.md:37     "Offboarding is `rm -rf ~/.gadak`." with no
#                      connected/standalone branch in the paragraph
#   README.md          0× `init --local` (INSTALL.md already had 1)
if grep -n "Atlassian Cloud only" docs/INSTALL.md; then
  fail "docs/INSTALL.md still says \"Atlassian Cloud only\" (GDK-271)"
fi
ok "docs/INSTALL.md does not say \"Atlassian Cloud only\""

faq_rm_unscoped=""
faq_rm_unscoped=$(python3 - <<'GDK373PY'
import re
from pathlib import Path

text = Path("docs/FAQ.md").read_text()
paras = re.split(r"\n\s*\n", text)
hits = []
for p in paras:
    if not re.search(r"rm\s+-rf\s+~/\.gadak", p):
        continue
    # A mention scoped to a site-backed workspace is allowed. Unqualified
    # "just delete ~/.gadak" is the class that wipes a standalone origin.
    # GDK-1278's vocabulary split retired "connected" as the category word
    # in prose; the scope is now spelled "Jira workspace" (FAIL-first
    # 2026-09-02: the reworded paragraph tripped the old regex).
    if re.search(r"\b(connected|Jira)\b", p, re.I):
        continue
    hits.append(re.sub(r"\s+", " ", p.strip())[:220])
if hits:
    print("\n".join(hits))
GDK373PY
)
if [[ -n "$faq_rm_unscoped" ]]; then
  fail "docs/FAQ.md has an unscoped \`rm -rf ~/.gadak\` (GDK-373) — scope it to a Jira workspace:"$'\n'"$faq_rm_unscoped"
fi
ok "docs/FAQ.md does not tell a standalone user to rm -rf ~/.gadak"

standalone_cmd_missing=""
for f in docs/INSTALL.md README.md; do
  if ! grep -q 'init --local' "$f"; then
    standalone_cmd_missing+="  $f: no init --local"$'\n'
  fi
done
if [[ -n "$standalone_cmd_missing" ]]; then
  fail "install front door does not name init --local (GDK-271):"$'\n'"$standalone_cmd_missing"
fi
ok "docs/INSTALL.md and README.md name init --local"

# check 21 — every GDK key a reader-facing doc names must resolve on the public
# backlog (GDK-269, GDK-389; user decision 2026-08-20 to link past entries too).
#
# The keys are advertising: a reader who cannot open GDK-408 is reading a
# reference to a tracker they have no access to. The snapshot is the whitelist,
# so this check is the other half of tools/backlog-scrub-check.sh — that one
# asserts nothing unmarked got published, this one asserts nothing published-to
# is missing. Attribution: the failure mode it closes was measured, not
# imagined — before the whitelist existed, three keys the docs cited
# (GDK-101, GDK-23, GDK-94) sat in the private set.
#
# Skipped deliberately: CLAUDE.md, AGENTS.md, skills/**, docs/decisions/**,
# specs/**, internal/**. Agent-instruction files pay context for every link and
# have no reader to advertise to; decisions are append-only by their own rule.
BACKLOG_ARCHIVE="examples/backlog-snapshot.tar.gz"

# Fire the delegated gates here (see the parallel-engine block at the top):
# every precondition they test is known at this point, every replay position
# is below, and none of the inline checks in between writes anything they
# read. Most expensive first, so the long one overlaps the most.
if [[ -f "$BACKLOG_ARCHIVE" ]] && command -v jq >/dev/null; then
  _gate_start scrub tools/backlog-scrub-check.sh "$BACKLOG_ARCHIVE"
fi
_gate_start promises tools/check-promises.sh
_gate_start write-handlers tools/check-write-handlers.sh
_gate_start lockfile tools/check-lockfile-platforms.sh
# ci-status only when HEAD^ exists — the same guard its check position uses
# (a shallow CI checkout lacks the history its parent-walk cases need).
if git rev-parse --verify -q "HEAD^" >/dev/null 2>&1; then
  _gate_start ci-status tools/ci-status-test.sh
fi
_gate_start audit tools/audit-test.sh
READER_DOCS=(CHANGELOG.md CHANGELOG.ko.md README.md README.ko.md README.ja.md
  docs/ARCHITECTURE.md docs/DERIVE.md docs/DESKTOP.md docs/INSTALL.md
  docs/project/ROADMAP.md docs/project/STATE_OF_PLAY.md desktop/README.md)
if [[ -f "$BACKLOG_ARCHIVE" ]] && command -v jq >/dev/null; then
  published=$(tar -xOf "$BACKLOG_ARCHIVE" bootstrap.json | jq -r '.issues[].issue_key' | sort -u)
  cited=$(grep -ohE 'GDK-[0-9]+' "${READER_DOCS[@]}" 2>/dev/null | sort -u)
  dangling=$(comm -23 <(printf '%s\n' "$cited") <(printf '%s\n' "$published") | tr '\n' ' ')
  if [[ -n "${dangling// /}" ]]; then
    fail "reader-facing docs cite GDK keys that are not on the public backlog: $dangling"$'\n'"  label them public and re-run tools/backlog-snapshot.sh, or drop the citation"
  fi
  ok "every GDK key in reader-facing docs resolves on the public backlog"
else
  ok "public backlog snapshot absent or jq missing — GDK key resolution not checked"
fi

# ── 22. Pairing named on the install/agent front door (GDK-457 / GDK-458) ─
# Class: a third way to bind a workspace (home serve + mint + remote
# `init --pairing-code-stdin`) shipped in the binary, but README / INSTALL /
# the agent cookbook / SKILL said nothing, so a reader following the front
# door could not pair. Analogous to check 20 (`init --local`). The flag
# name is the pin: `cmd/gadak/init.go` registers `--pairing-code-stdin`.
# The agent cookbook moved from AGENTS.md to docs/MIRROR.md (GDK-8); this
# check follows the file, not the old name.
# FAIL-first 2026-08-21 against the unmodified f6-docs tree: all five files
# below had 0 hits for pairing-code-stdin; SKILL.md line 221 said
# `views save` kept a named view "in the mirror" and named local.db 0 times.
pairing_missing=""
for f in README.md README.ko.md README.ja.md docs/INSTALL.md docs/MIRROR.md skills/gadak/SKILL.md; do
  if ! grep -q 'pairing-code-stdin' "$f"; then
    pairing_missing+="  $f: no --pairing-code-stdin"$'\n'
  fi
done
if [[ -n "$pairing_missing" ]]; then
  fail "install/agent front door does not name --pairing-code-stdin (GDK-457):"$'\n'"$pairing_missing"
fi
ok "README, INSTALL, docs/MIRROR.md, SKILL name --pairing-code-stdin"

if grep -n 'views save' skills/gadak/SKILL.md | grep -q 'in the mirror'; then
  fail "skills/gadak/SKILL.md still says views save lives in the mirror (GDK-458) — they live in local.db"
fi
if ! grep -q 'local.db' skills/gadak/SKILL.md; then
  fail "skills/gadak/SKILL.md does not name local.db (GDK-458; saved views and visits live there)"
fi
ok "SKILL.md does not put saved views in the mirror; names local.db"

# ── 23. Public-surface GDK keys resolve on the public backlog (GDK-269) ─
# Class: a GDK-nnn cited on a tracked public surface that is neither in the
# published backlog snapshot nor in the private-key allowlist is a dangling
# reference for an external reader. Tests, e2e, the snapshot itself, and the
# allowlist are not public surfaces for this purpose.
#
# The snapshot is one-line JSON inside the archive (a line-oriented grep of
# bootstrap.json has already gone vacuous twice — tools/backlog-scrub-check.sh).
# Keys are extracted as structural tokens `"key":"GDK-N"` from the packed
# bootstrap.json, not by scanning the gzip. An absent snapshot or a snapshot
# with 0 keys is a fail: that is the published set vanishing, not a clean tree.
#
# FAIL-first 2026-08-22 against this worktree's unmodified sources + snapshot:
# 21 distinct keys cited on tracked public surfaces, absent from
# examples/backlog-snapshot/bootstrap.json:
#   GDK-461 GDK-462 GDK-463 GDK-464 GDK-465 GDK-466 GDK-467 GDK-468
#   GDK-469 GDK-470 GDK-474 GDK-476 GDK-477 GDK-478 GDK-479 GDK-481
#   GDK-482 GDK-507 GDK-579 GDK-580 GDK-600
# FAIL-first 2026-08-23 (packed medium): the same extraction against
# `tar -xOf examples/backlog-snapshot.tar.gz bootstrap.json` plus a cited
# key not in that set fails; dropping the extra key is green.
BACKLOG_SNAP="$BACKLOG_ARCHIVE"
BACKLOG_PRIVATE="tools/backlog-private-keys.txt"

if [[ ! -f "$BACKLOG_SNAP" ]]; then
  fail "public backlog snapshot is missing ($BACKLOG_SNAP) — cannot resolve GDK keys"
fi
if [[ ! -f "$BACKLOG_PRIVATE" ]]; then
  fail "$BACKLOG_PRIVATE is missing — it is the private-key allowlist for this check (GDK-269)"
fi

published=$(tar -xOf "$BACKLOG_SNAP" bootstrap.json | grep -oE '"key":"GDK-[0-9]+"' | grep -oE 'GDK-[0-9]+' | sort -u) || true
if [[ -z "$published" ]]; then
  fail "public backlog snapshot has 0 issue keys ($BACKLOG_SNAP) — refusing to treat an empty snapshot as clean"
fi

private=""
while IFS= read -r line || [[ -n "$line" ]]; do
  [[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
  read -r key rest <<< "$line"
  [[ "$key" =~ ^GDK-[0-9]+$ ]] || fail "$BACKLOG_PRIVATE: not a GDK key: ${key@Q}"
  [[ -n "$rest" ]] || fail "$BACKLOG_PRIVATE: $key has no reason (format: GDK-nnn<space-or-tab>reason)"
  private+="$key"$'\n'
done < "$BACKLOG_PRIVATE"

# GDK-683: `git ls-files` alone made this gate report green on a file it had
# never opened. A new doc citing an unpublished key sat untracked while the
# gate passed three times, and went red only after `git add` — in CI, on a
# commit already pushed. Untracked-but-not-ignored files are part of the
# working surface, so they are scanned here too: --exclude-standard keeps
# dist/, node_modules/ and the local runtime data out.
scanned_surface_files() {
  git ls-files
  git ls-files --others --exclude-standard
}
cited=$(
  scanned_surface_files \
    | grep -vE '_test\.go$|\.spec\.ts$|^e2e/|^examples/backlog-snapshot|^tools/backlog-private-keys\.txt$' \
    | xargs grep -oEhI -E '\bGDK-[0-9]+\b' -- \
    | sort -u
) || true
untracked_scanned=$(git ls-files --others --exclude-standard | wc -l | tr -d ' ')
if [[ "$untracked_scanned" != "0" ]]; then
  ok "$untracked_scanned untracked file(s) are in this check's surface — committing them cannot change its verdict"
fi

resolved=$(printf '%s\n%s' "$published" "$private" | sed '/^$/d' | sort -u)
dangling=$(comm -23 <(printf '%s\n' "$cited" | sed '/^$/d') <(printf '%s\n' "$resolved") | sort -t- -k2,2n)
if [[ -n "$dangling" ]]; then
  list=$(printf '%s\n' "$dangling" | tr '\n' ' ')
  list="${list%" "}"
  fail "public surfaces cite GDK keys that are not on the public backlog: $list"$'\n'"  to publish a key: edit KEY --label +public, then tools/backlog-snapshot.sh"$'\n'"  to keep it private: add it to $BACKLOG_PRIVATE with a one-line reason"$'\n'"  the lead does both"
fi
ok "every GDK key on a public surface resolves on the public backlog or the private-key allowlist"

# ── 24. Public backlog snapshot keys match packed detail JSON (GDK-634) ─
# Class: bootstrap.json listing a key whose detail JSON is not in the archive
# publishes a 404 detail page; a packed detail whose key is absent from
# bootstrap.json is an orphan the index will not link.
#
# Before the packed medium the tree was 613 git-tracked JSON files, and existence was
# `git ls-files -- examples/backlog-snapshot/detail/` because commit 2b9cb60
# shipped the index without 21 new detail files that sat untracked on disk.
# One archive is atomic, so that class cannot recur. The remaining hole is
# an internally inconsistent archive (bootstrap keys ≠ detail/ members).
# Members come from `tar -tzf`, not a filesystem glob of an unpacked tree.
# Key extraction from bootstrap.json reuses the structural token `"key":"GDK-N"`
# and the already-non-empty `$published` from check 23.
#
# FAIL-first 2026-08-22 (git state not mutated): drop one published key from
# the tracked-detail variable → missing; append a fake key → orphan.
# FAIL-first 2026-08-23 (packed medium): same injection against tar members.
backlog_snapshot_detail_consistency() {
  local published_keys="$1"
  local tracked_keys="$2"
  local missing orphans missing_list orphan_list body
  missing=$(comm -23 <(printf '%s\n' "$published_keys" | sed '/^$/d' | sort -u) \
                     <(printf '%s\n' "$tracked_keys" | sed '/^$/d' | sort -u) | sort -t- -k2,2n)
  orphans=$(comm -13 <(printf '%s\n' "$published_keys" | sed '/^$/d' | sort -u) \
                     <(printf '%s\n' "$tracked_keys" | sed '/^$/d' | sort -u) | sort -t- -k2,2n)
  if [[ -n "$missing" || -n "$orphans" ]]; then
    body="public backlog snapshot index and packed detail JSON are inconsistent (GDK-634):"
    if [[ -n "$missing" ]]; then
      missing_list=$(printf '%s\n' "$missing" | tr '\n' ' ')
      missing_list="${missing_list%" "}"
      body+=$'\n'"  missing packed detail: $missing_list"
    fi
    if [[ -n "$orphans" ]]; then
      orphan_list=$(printf '%s\n' "$orphans" | tr '\n' ' ')
      orphan_list="${orphan_list%" "}"
      body+=$'\n'"  orphan packed detail: $orphan_list"
    fi
    body+=$'\n'"  re-run bash tools/backlog-snapshot.sh, then git add examples/backlog-snapshot.tar.gz (the lead does both)"
    fail "$body"
  fi
  ok "public backlog snapshot keys match packed detail JSON (GDK-634)"
} # end backlog_snapshot_detail_consistency

tracked_detail=$(
  tar -tzf "$BACKLOG_ARCHIVE" \
    | sed -n 's|^detail/\(GDK-[0-9][0-9]*\)\.json$|\1|p' \
    | sort -u
)
backlog_snapshot_detail_consistency "$published" "$tracked_detail"

# ── The committed snapshot passes its own scrub gate ──
# tools/backlog-snapshot.sh runs the scrub gate at generation time and CI runs
# it on the Pages artifact, but neither guards the commit itself: on 2026-08-23
# a snapshot that failed the gate was committed and pushed anyway (the failure
# was read through a pipe), and main went red on the Pages build (GDK-675).
# Running the same gate here means "doc-checks green" implies "the tracked
# snapshot is publishable". The argument is the packed archive;
# backlog-scrub-check.sh unpacks it.
# FAIL-first 2026-08-23: unpack, set bootstrap.issues[0].assignee, repack → red;
# unmodified archive → green.
# Replayed from its concurrent start (see the parallel-engine block). The
# failure branch used to re-run the whole 39 s script a second time just to
# capture its output; the replay prints the captured streams instead.
if [[ "$_PAR_STARTED" == *" scrub "* ]]; then
  if _gate_replay scrub; then
    ok "committed backlog snapshot passes the scrub gate (GDK-675)"
  else
    fail "committed backlog snapshot fails its scrub gate (its output is above)"
  fi
fi

# ── 25. AGENTS.md is the repo development contract, not the product cookbook (GDK-8) ──
# Class: AGENTS.md is the filename coding agents look for at the repo root.
# The product cookbook (how to query the mirror) lives in docs/MIRROR.md.
# Recurrence is pasting the SQL cookbook / CLI reference back into AGENTS.md
# under those headings. Length is not the tell — a long development note is
# fine; those headings are the product-manual identity.
#
# Structural markers (not a line-count):
#   AGENTS.md must not have ## Using the mirror / ### SQL cookbook /
#   ### CLI reference (those three were the product half).
#   AGENTS.md must have ## Developing gadak (this file's remaining job).
#
# FAIL-first 2026-08-23: injecting `### SQL cookbook` into post-split
# AGENTS.md fails this check; removing it is green.
if grep -qE '^## Using the mirror$' AGENTS.md; then
  fail "AGENTS.md has heading \"## Using the mirror\" — that product section lives in docs/MIRROR.md (GDK-8)"
fi
if grep -qE '^### SQL cookbook$' AGENTS.md; then
  fail "AGENTS.md has heading \"### SQL cookbook\" — the query recipes live in docs/MIRROR.md (GDK-8)"
fi
if grep -qE '^### CLI reference$' AGENTS.md; then
  fail "AGENTS.md has heading \"### CLI reference\" — the CLI cookbook lives in docs/MIRROR.md (GDK-8)"
fi
if ! grep -qE '^## Developing gadak$' AGENTS.md; then
  fail "AGENTS.md is missing heading \"## Developing gadak\" — that is this file's remaining job (GDK-8)"
fi
ok "AGENTS.md is the development contract (no product-cookbook headings)"

# ── 26. AGENTS.md ↔ docs/MIRROR.md pointers are live (GDK-8) ──
# Same shape as check 20 (`grep -q 'init --local'`) and check 22
# (`grep -q 'pairing-code-stdin'`): a path token that must appear in the
# counterpart file. This repo's doc-checks do not have a markdown-link
# resolver; presence of the path is the existing contract.
#
# FAIL-first 2026-08-23: deleting the docs/MIRROR.md token from AGENTS.md
# fails; deleting the AGENTS.md token from docs/MIRROR.md fails; restoring
# both is green.
if [[ ! -f docs/MIRROR.md ]]; then
  fail "docs/MIRROR.md is missing — it is the product cookbook AGENTS.md must point at (GDK-8)"
fi
if ! grep -q 'docs/MIRROR.md' AGENTS.md; then
  fail "AGENTS.md does not point at docs/MIRROR.md (GDK-8)"
fi
if ! grep -q 'AGENTS.md' docs/MIRROR.md; then
  fail "docs/MIRROR.md does not point at AGENTS.md (GDK-8)"
fi
if ! grep -qE '^# Using the mirror$' docs/MIRROR.md; then
  fail "docs/MIRROR.md is missing heading \"# Using the mirror\" (GDK-8; the #using-the-mirror anchor)"
fi
ok "AGENTS.md and docs/MIRROR.md point at each other"

# ── 27. CHANGELOG en/ko key tails stay a closed, matching set ────────────
# Class: compressing a release section by memory-attaching the wrong GDK key
# (measured 2026-08-23 on an Unreleased draft: 10 keys were wrong until the
# original paragraphs were re-read). Recurrence is (a) a citation with no
# tail definition, (b) en and ko quoting different keys in the same release
# heading, (c) a tail URL that is not the public backlog form.
#
# Failure names the section and the key — "mismatch" alone is not a tool.
#
# FAIL-first 2026-08-23 against this compressed tree (each restored):
#   1. deleting [GDK-186] from CHANGELOG.md's v0.15.2 section:
#      "CHANGELOG.md:N defines GDK-186 but no section cites it"
#      "section 'v0.15.2 — 2026-08-17': en/ko key sets differ (ko-only GDK-186)"
#   2. deleting the [GDK-182] tail line from CHANGELOG.md:
#      "CHANGELOG.md section 'v0.15.1 — 2026-08-17': cites GDK-182 with no
#      tail definition"
#   3. rewriting the GDK-8 tail URL to midagedev.github.io:
#      "CHANGELOG.md:N GDK-8 URL is 'https://midagedev.github.io/…', want
#      'https://gadak.dev/backlog/#/?ks=GDK-8'"
changelog_keys=$(
  python3 - <<'CHANGELOGKEYSPY'
import re
from pathlib import Path

CITE = re.compile(r"\[(GDK-\d+)\]")
DEF = re.compile(r"^\[(GDK-\d+)\]:\s+(\S+)\s*$")
TAIL_START = re.compile(r"^\[(?:GDK-\d+|#\d+)\]:")
WANT = "https://gadak.dev/backlog/#/?ks={}"


def parse(path):
    text = Path(path).read_text()
    lines = text.splitlines()
    def_start = next(
        (i for i, line in enumerate(lines) if TAIL_START.match(line)),
        None,
    )
    if def_start is None:
        print("%s: no reference-link tail (no [GDK-nnn]: line)" % path)
        return None
    body = lines[:def_start]
    defs = {}
    dupes = []
    for i, line in enumerate(lines[def_start:], def_start + 1):
        m = DEF.match(line)
        if not m:
            continue
        key, url = m.group(1), m.group(2)
        if key in defs:
            dupes.append((key, i))
        defs[key] = (url, i)
    sections = []
    heads = [(i, line[3:]) for i, line in enumerate(body) if line.startswith("## ")]
    for idx, (i, title) in enumerate(heads):
        end = heads[idx + 1][0] if idx + 1 < len(heads) else len(body)
        chunk = "\n".join(body[i:end])
        keys = set(CITE.findall(chunk))
        sections.append((title, keys, i + 1))
    stale = [
        (i, line)
        for i, line in enumerate(lines, 1)
        if "midagedev.github.io" in line
    ]
    return {
        "path": path,
        "sections": sections,
        "defs": defs,
        "dupes": dupes,
        "stale": stale,
    }


def keynum(k):
    return int(k.split("-")[1])


fails = []
parsed = {}
for path in ("CHANGELOG.md", "CHANGELOG.ko.md", "CHANGELOG.ja.md"):
    got = parse(path)
    if got is None:
        raise SystemExit(0)
    parsed[path] = got
    cited = set()
    for title, keys, _start in got["sections"]:
        cited |= keys
        for key in sorted(keys, key=keynum):
            if key not in got["defs"]:
                fails.append(
                    "%s section %r: cites %s with no tail definition"
                    % (path, title, key)
                )
    for key, (url, line) in sorted(got["defs"].items(), key=lambda kv: keynum(kv[0])):
        if key not in cited:
            fails.append(
                "%s:%d defines %s but no section cites it" % (path, line, key)
            )
        want = WANT.format(key)
        if url != want:
            fails.append(
                "%s:%d %s URL is %r, want %r" % (path, line, key, url, want)
            )
    for key, line in got["dupes"]:
        fails.append("%s:%d duplicate tail definition for %s" % (path, line, key))
    for line, text in got["stale"]:
        fails.append(
            "%s:%d tail URL still uses midagedev.github.io — %s"
            % (path, line, text.strip())
        )

# The three files are one history in three languages (not parallel editions,
# which is what the READMEs are): same releases, in the same order, citing the
# same keys. ja joined on 2026-09-09 — before that its site page rendered the
# English file and said so.
en_titles = [t for t, _, _ in parsed["CHANGELOG.md"]["sections"]]
en_map = {t: k for t, k, _ in parsed["CHANGELOG.md"]["sections"]}
for lang, path in (("ko", "CHANGELOG.ko.md"), ("ja", "CHANGELOG.ja.md")):
    titles = [t for t, _, _ in parsed[path]["sections"]]
    if en_titles != titles:
        fails.append(
            "section headings differ between CHANGELOG.md and %s: %s vs %s"
            % (path, en_titles, titles)
        )
        continue
    other = {t: k for t, k, _ in parsed[path]["sections"]}
    for title in en_titles:
        e, k = en_map[title], other[title]
        if e == k:
            continue
        only_en = ", ".join(sorted(e - k, key=keynum))
        only_other = ", ".join(sorted(k - e, key=keynum))
        bits = []
        if only_en:
            bits.append("en-only " + only_en)
        if only_other:
            bits.append("%s-only %s" % (lang, only_other))
        fails.append(
            "section %r: en/%s key sets differ (%s)" % (title, lang, "; ".join(bits))
        )

if fails:
    print("\n".join(fails))
CHANGELOGKEYSPY
)
if [[ -n "$changelog_keys" ]]; then
  fail "CHANGELOG key/tail contract broken:"$'\n'"$changelog_keys"
fi
ok "CHANGELOG.md, .ko.md and .ja.md cite the same keys per section; every citation has a gadak.dev tail"

# ── 28. Docs do not teach leftover fieldMap / editableFields as current ──
# Class: compatibility path whose old *editing* surface no longer exists.
# LoadFor.NormalizeLegacyFields folds leftover fieldMap/editableFields into
# fields and clears them, so Settings editors and `gadak config set fieldMap`
# plant a value the next load erases. Migration prose may still name the keys.
#
# Allowed: the words fieldMap / editableFields next to legacy / migrat /
# leftover / unmarshal (LoadFor, team-import of old files, "when fields is
# empty, leftover maps are synthesized").
#
# Forbidden — method-teaching, the incident:
#   1. `gadak config set fieldMap` / `gadak config set editableFields`
#   2. a markdown table row whose first cell is fieldMap or editableFields
#      without legacy/migrat/leftover on that row (EXTENDING.md's config-key
#      table listed them as current keys)
#   3. that leftover-key row still naming Settings as where to edit it
#      (the Fields tab no longer draws those editors)
#   4. "Map it in `fieldMap`" / "`editableFields` allowlist" as a how-to
#   5. a team-export example or shared-keys cell that lists fieldMap as
#      something export writes (export copies Fields; FieldMap is
#      unmarshal-only on old team files)
#
# FAIL-first 2026-08-23 against the unmodified tree: docs/EXTENDING.md:28,30,75,78
# docs/MIRROR.md:284 docs/CONFIGURATION.md:124,126,289.
legacy_fieldmap=$(
  python3 - <<'GDK710PY'
from pathlib import Path
import re

set_re = re.compile(r"gadak config set (?:fieldMap|editableFields)\b")
map_it_re = re.compile(r"Map it in `fieldMap`")
allowlist_re = re.compile(r"`editableFields` allowlist")
export_re = re.compile(r"team export.*fieldMap")
shared_re = re.compile(r"`fields`,\s*`fieldMap`")
row_re = re.compile(r"^\|\s*`?(fieldMap|editableFields)`?\s*\|")
legacy_ok = re.compile(r"legacy|migrat|leftover|unmarshal", re.I)
settings_edit = re.compile(r"Settings\s*→")

fails = []
for path in sorted(Path("docs").rglob("*.md")):
    for i, line in enumerate(path.read_text().splitlines(), 1):
        loc = f"{path}:{i}"
        if set_re.search(line):
            fails.append(f"{loc}: teaches `gadak config set` of a leftover key")
        if map_it_re.search(line):
            fails.append(f"{loc}: recipe still says Map it in fieldMap")
        if allowlist_re.search(line):
            fails.append(f"{loc}: recipe still names editableFields allowlist as the method")
        if export_re.search(line):
            fails.append(f"{loc}: team export example names fieldMap as a shared setting (export writes Fields)")
        if shared_re.search(line) and not legacy_ok.search(line):
            fails.append(f"{loc}: shared-keys list still includes fieldMap as currently exported")
        m = row_re.match(line)
        if m:
            if not legacy_ok.search(line):
                fails.append(f"{loc}: table lists `{m.group(1)}` as a current config key")
            elif settings_edit.search(line):
                fails.append(f"{loc}: leftover key still lists Settings as where to edit it")

if fails:
    print("\n".join(fails))
GDK710PY
)
if [[ -n "$legacy_fieldmap" ]]; then
  fail "docs still teach leftover fieldMap/editableFields as the current method:"$'\n'"$legacy_fieldmap"
fi
ok "docs do not teach leftover fieldMap/editableFields as the current field-mapping method"

# ── 29. Packaging manifests pin the tagged version (GDK-744) ─────────────
# Same version owner as check 6 (`$tag` from `git describe --tags --abbrev=0`,
# already computed; do not describe again). Tagless checkout skips, same as
# check 6: this is drift between files, not tagging policy.
#
# Why this check exists at all: the on-change guards were correct and never
# ran. scoop.yml and aur.yml are `paths:`-scoped to the very files they
# guard, and the only event that can invalidate those files is a *new tag*,
# which touches neither path. So 0.16.0 and 0.16.1 both shipped with the
# manifests pinned at 0.15.2 and every gate green.
#
# Check 6 compares the truncated minor because the README status line says
# `0.16`. These manifests pin a full patch, so they must equal `0.16.1`,
# not `0.16`. Do not reuse `$minor` here.
#
# Own numbered check rather than folding into 6: fail() exits on the first
# FAIL, and putting this last is what makes a FAIL-first run still print
# every pre-existing ok line. The comparison (full patch vs minor) is also
# a different contract than the front-door docs.
#
# The on-change guards (contrib/scoop/verify.sh, contrib/aur/gadak-bin/
# check-pkgver.sh) stay; they also check hashes and packaging rules. This
# is the always-on half that a tag can actually trip.
#
# FAIL-first 2026-08-23 on this tree: both manifests still say 0.15.2
# against latest tag v0.16.1. That red is by design — the pins move at
# tag time with checksums.txt, which does not exist until the release is
# published.
if [[ -z "$tag" ]]; then
  ok "no tag reachable — packaging version guard skipped"
else
  want="${tag#v}"
  packaging_drift=""

  scoop_ver="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' contrib/scoop/gadak.json)"
  if [[ "$scoop_ver" != "$want" ]]; then
    packaging_drift+="  contrib/scoop/gadak.json version=${scoop_ver} does not match latest tag ${tag} (want ${want})"$'\n'
  fi

  pkgver_line="$(grep -E '^pkgver=' contrib/aur/gadak-bin/PKGBUILD || true)"
  if [[ -z "$pkgver_line" ]]; then
    packaging_drift+="  contrib/aur/gadak-bin/PKGBUILD has no pkgver= line (latest tag ${tag}, want ${want})"$'\n'
  else
    pkgver="${pkgver_line#pkgver=}"
    pkgver="${pkgver%%$'\n'*}"
    pkgver="${pkgver#\'}"
    pkgver="${pkgver%\'}"
    pkgver="${pkgver#\"}"
    pkgver="${pkgver%\"}"
    if [[ "$pkgver" != "$want" ]]; then
      packaging_drift+="  contrib/aur/gadak-bin/PKGBUILD pkgver=${pkgver} does not match latest tag ${tag} (want ${want})"$'\n'
    fi
  fi

  # .SRCINFO is generated from PKGBUILD by makepkg, which is not on a mac,
  # so contrib/aur/gadak-bin/update.sh prints a hint and exits 0 with it
  # stale. Measured 2026-08-23: the 0.16.1 bump landed on main with a stale
  # .SRCINFO and only the AUR workflow's own diff check caught it — after
  # the push. Comparing its pkgver here moves that catch before the commit.
  srcinfo_ver="$(grep -E '^[[:space:]]*pkgver[[:space:]]*=' contrib/aur/gadak-bin/.SRCINFO | head -1 | sed -E 's/.*=[[:space:]]*//')"
  if [[ "$srcinfo_ver" != "$want" ]]; then
    packaging_drift+="  contrib/aur/gadak-bin/.SRCINFO pkgver=${srcinfo_ver:-<missing>} does not match latest tag ${tag} (want ${want}) — regenerate with contrib/aur/gadak-bin/verify.sh"$'\n'
  fi

  if [[ -n "$packaging_drift" ]]; then
    fail "packaging manifests disagree with latest tag ${tag}:"$'\n'"${packaging_drift%$'\n'}"
  fi
  ok "scoop manifest, AUR PKGBUILD and .SRCINFO agree with ${tag}"
fi

# ── 30. README's benchmark table carries the CURRENT measurement (GDK-773) ─
# The class this closes: a re-measurement lands on one surface and the others
# keep publishing the old numbers. Measured 2026-08-24 — docs/BENCHMARKS.md
# and site/src/i18n.ts carried the 2026-08-23 re-run (7,166 issues, 23×)
# while README.md and README.ko.md still published the 2026-08-15 table
# (2,853 issues, 42×/162×) with no date on it, and every gate was green.
# One of those README rows (162× GROUP BY) could not be reproduced on the
# current corpus at all, so the front door was advertising a number the
# authority no longer supports.
#
# BENCHMARKS.md is the authority. Its LAST measurement section is current;
# the README caption must name that section's date and corpus, and every
# ms/× figure in the README table must appear in it. A future re-measurement
# therefore cannot be published to one surface only.
readme_bench=$(
  python3 - <<'BENCHPY'
import re
from pathlib import Path

bench = Path("docs/BENCHMARKS.md").read_text()
fails = []

# The authority's current section: the last "Measured"/"Re-measured" block.
heads = [m.start() for m in re.finditer(r"^#{1,2} .*?[Mm]easured", bench, re.M)]
# The opening paragraph counts as a section even without its own heading.
starts = heads or [0]
latest = bench[starts[-1]:]
nxt = re.search(r"^## (?!Re-measured)", latest[1:], re.M)
if nxt:
    latest = latest[: nxt.start() + 1]

date = re.search(r"(20\d\d-\d\d-\d\d)", latest)
corpus = re.search(r"([\d,]{3,})\s+issues", latest)
if not date or not corpus:
    fails.append("docs/BENCHMARKS.md: latest measurement section names no date or no corpus size")
else:
    figures = set(re.findall(r"\d[\d,]*(?:\.\d+)?\s*ms", latest))
    figures |= set(re.findall(r"\d[\d,]*×", latest))
    figures = {f.replace(" ", "") for f in figures}
    for path in ("README.md", "README.ko.md", "README.ja.md"):
        text = Path(path).read_text()
        rows = [
            ln
            for ln in text.splitlines()
            if ln.startswith("|") and re.search(r"\d\s*ms|\d×", ln)
        ]
        if not rows:
            fails.append(f"{path}: no benchmark table rows found (expected the REST-vs-gadak table)")
            continue
        if date.group(1) not in text:
            fails.append(
                f"{path}: does not name the current measurement date {date.group(1)} "
                "(docs/BENCHMARKS.md's latest section)"
            )
        if corpus.group(1) not in text:
            fails.append(
                f"{path}: does not name the current corpus size {corpus.group(1)} issues"
            )
        for ln in rows:
            for fig in re.findall(r"\d[\d,]*(?:\.\d+)?\s*ms|\d[\d,]*×", ln):
                if fig.replace(" ", "") not in figures:
                    fails.append(
                        f"{path}: benchmark figure {fig.strip()} is not in "
                        "docs/BENCHMARKS.md's latest measurement section"
                    )

if fails:
    print("\n".join(dict.fromkeys(fails)))
BENCHPY
)
if [[ -n "$readme_bench" ]]; then
  fail "README benchmark table disagrees with docs/BENCHMARKS.md's current measurement:"$'\n'"$readme_bench"
fi
ok "README (en+ko) benchmark table matches the current measurement in docs/BENCHMARKS.md"

# ── 31. Copy units in the entry docs (GDK-772 wave) ──────────────────────
# The class these close: a fenced block that a reader copies whole, but which
# holds two *alternatives* or an instruction hidden in a `#` comment, so the
# paste does something other than what the reader picked. Measured 2026-08-24,
# each from a real block in the tree at 2c00756:
#   a) README.md:92 was one fence with `brew install …/gadak` AND
#      `brew install …/gadak-cli` — pasting installed the app cask and the CLI
#      formula. Same shape in docs/INSTALL.md:51.
#   b) docs/MCP.md:60 stacked eight `gadak mcp install …` lines, and the first
#      of them execs `claude mcp add`, so the paste registered a server.
#      docs/AGENT_SETUP.md:162 stacked `skill install` with `--force`.
#   c) docs/INSTALL.md documents Scoop honestly but must never *instruct*
#      `scoop install`: contrib/scoop/README.md says the bucket is unpublished
#      and `scoop install` has never been run on a Windows host.
# Sequences the reader runs in order (init && sync && serve, verify then untar)
# are fine — the test is two lines the reader chooses *between*.
copy_units=$(
  python3 - <<'COPYPY'
import re
from pathlib import Path

fails = []
FENCE = re.compile(r"^```[^\n]*\n(.*?)^```", re.S | re.M)


def fences(text):
    return FENCE.findall(text)


def code_lines(body):
    return [ln for ln in body.splitlines() if ln.strip() and not ln.lstrip().startswith("#")]


# (a) one fence must not offer two installs of the same kind.
for name in ("README.md", "README.ko.md", "README.ja.md", "docs/INSTALL.md"):
    text = Path(name).read_text()
    for i, body in enumerate(fences(text), 1):
        n = sum(1 for ln in code_lines(body) if re.search(r"\bbrew\s+install\b", ln))
        if n > 1:
            fails.append(f"{name}: fence {i} offers {n} brew install lines — split the alternatives")

# (b) one fence must not stack two registration verbs. These change state:
# `mcp install claude` execs `claude mcp add`, `skill install` writes SKILL.md.
VERB = re.compile(
    r"^\s*(?:\S*/)?gadak(?:\s+--(?:workspace|profile)\s+\S+)?\s+"
    r"(?:mcp\s+install|skill\s+install|install-cli)\b"
)
targets = sorted(Path("docs").glob("*.md")) + [Path("README.md"), Path("README.ko.md"), Path("README.ja.md")]
for path in targets:
    text = path.read_text()
    for i, body in enumerate(fences(text), 1):
        hits = [ln.strip() for ln in code_lines(body) if VERB.search(ln)]
        if len(hits) > 1:
            fails.append(f"{path.as_posix()}: fence {i} stacks install verbs {hits} — one per fence")

# (c) Scoop may be described, never instructed. HTML comments are the
# publish-time draft and do not render on GitHub, so strip them first.
install = re.sub(r"<!--.*?-->", "", Path("docs/INSTALL.md").read_text(), flags=re.S)
for lineno, line in enumerate(install.splitlines(), 1):
    if re.match(r"\s*scoop\s+install\b", line):
        fails.append(
            f"docs/INSTALL.md:{lineno} instructs `scoop install` — the bucket is "
            "unpublished (contrib/scoop/README.md)"
        )

if fails:
    print("\n".join(fails))
COPYPY
)
if [[ -n "$copy_units" ]]; then
  fail "entry docs mix copy units:"$'\n'"$copy_units"
fi
ok "entry docs keep one copy unit per fence, and Scoop is described not instructed"

# ── 32. The docs index resolves and covers docs/ (GDK-777) ───────────────
# Measured 2026-08-24 against the tree at 2c00756: docs/README.md's "Start
# here" opened with the contributor reading list, INSTALL.md, DESKTOP.md,
# WINDOWS-SIGNING.md and BENCHMARKS.md were not in the index at all, and the
# entries that were there named files in backticks rather than links — so
# nothing in the documentation index was clickable and the install docs were
# not reachable from it. Both halves are asserted here: every relative link
# resolves, and every docs/*.md is actually linked (a backticked filename does
# not count, which is what made the old index look complete).
docs_index=$(
  python3 - <<'INDEXPY'
import re
from pathlib import Path

index = Path("docs/README.md")
text = index.read_text()
targets = []
for raw in re.findall(r"\]\(([^)]+)\)", text):
    t = raw.split("#", 1)[0].strip()
    if not t or re.match(r"^[a-zA-Z][a-zA-Z0-9+.-]*:", t):
        continue
    targets.append(t)

broken = [t for t in targets if not (index.parent / t).exists()]
listed = {(index.parent / t).resolve() for t in targets if (index.parent / t).exists()}
unlisted = [
    p.as_posix()
    for p in sorted(Path("docs").glob("*.md"))
    if p.name != "README.md" and p.resolve() not in listed
]

out = []
if broken:
    out.append("broken relative links in docs/README.md: " + ", ".join(broken))
if unlisted:
    out.append("docs/*.md not linked from the index: " + ", ".join(unlisted))
if out:
    print("\n".join(out))
INDEXPY
)
if [[ -n "$docs_index" ]]; then
  fail "docs/README.md is not a complete index:"$'\n'"$docs_index"
fi
ok "docs/README.md links resolve and every docs/*.md is indexed"

# ── 33. Copyable examples carry <version>, not a literal tag (GDK-778) ────
# Same axis as check 31, one step further: a fence a reader copies must not
# name a specific release archive, because the copy outlives the tag.
# Measured 2026-08-24: docs/WINDOWS-SIGNING.md hashed
# gadak_0.16.1_windows_amd64.zip in two copyable PowerShell blocks on a
# v0.17.1 tree, with the "replace 0.16.1" instruction *below* the fence — so
# the reader pasted a filename that no longer exists in the release they had.
# Prose may name a measured release (that is history); a fence may not.
version_pins=$(
  python3 - <<'PINPY'
import re
from pathlib import Path

FENCE = re.compile(r"```[^\n]*\n(.*?)```", re.S)
ARCHIVE = re.compile(r"(?:gadak_|Gadak-)\d+\.\d+\.\d+[_-]")

fails = []
targets = [Path("README.md"), Path("README.ko.md"), Path("README.ja.md")] + sorted(Path("docs").rglob("*.md"))
for path in targets:
    for block in FENCE.findall(path.read_text()):
        for m in ARCHIVE.finditer(block):
            fails.append(
                f"{path.as_posix()}: fenced example pins {m.group(0)!r} — use <version> "
                "and say what to substitute above the fence"
            )
if fails:
    print("\n".join(dict.fromkeys(fails)))
PINPY
)
if [[ -n "$version_pins" ]]; then
  fail "copyable examples pin a release version:"$'\n'"$version_pins"
fi
ok "copyable examples in the READMEs and docs/ use <version>, not a literal tag"

# ── 34. The site has one copyable-block component (GDK-779) ──────────────
# site/src/components/Snippet.astro is the copy button. A raw <pre><code>
# elsewhere is a block a reader cannot copy from — measured 2026-08-24: the
# install page was rewritten around Snippet in b1de734 while the landing kept
# four raw blocks, three of which stacked alternatives behind # comments.
raw_pre=$(grep -rln '<pre><code>' site/src --include='*.astro' | grep -v 'components/Snippet.astro' || true)
if [[ -n "$raw_pre" ]]; then
  fail "site pages hold raw <pre><code> instead of the Snippet component:"$'\n'"$raw_pre"
fi
ok "site copyable blocks all go through Snippet.astro"

# ── 35. The PROMISES verification blocks actually run ───────────────────
# docs/PROMISES.md stakes its credibility on "if one stops doing so, the
# promise is broken" — this makes that sentence executable. v0.18.1 shipped
# with promise #9 reading a YAML persist the code had replaced with SQLite.
_gate_require promises
_stamp 'check 35 check-promises.sh (replayed from its concurrent start)'

# ── 36. "Not planned" refusals match the shipped tree ────────────────────
# Class: a refusal list is a decision a reader can be pointed at. A refusal
# the tree already contradicts is worse than none — it still reads as a
# decision, only a false one. Measured twice in one month against this
# file's own tree: the locale bullet said two while the third catalog had
# shipped days earlier, and the Releases paragraph promised one window per
# week against six tags in four days. Every refusal that is tree-visible
# gets one probe here.
#
# Locale axis: the bullet must name the shipped set after "beyond", and
# that set must equal LOCALES in web/src/lib/i18n/types.ts. LOCALES is the
# single owner of what ships — the en.ts/ko.ts/ja.ts files beside it are
# re-export shims, not catalogs; the catalogs are the {en,ko,ja} objects in
# messages/*.ts, and they are keyed off LOCALES. A fourth locale added to
# LOCALES, or a doc naming one that does not ship, fails in both
# directions.
#
# Terminal axis: while the doc refuses terminal tabs/splits/profiles, the
# i18n catalog must carry no terminal.* key naming one — a tab bar, a
# split, or a profile picker needs a label, and terminal copy lives in
# messages/*.ts (every terminal.* key is in messages/shell.ts today).
# Limits: one source-level signal per word, not UI enumeration — a feature
# whose labels dodge the three words, or UI added without an i18n key,
# would not trip this. The refusal bullet itself is asserted too: dropping
# it silently would leave the first tab request with no answer to link.
#
# Release cadence: deliberately NO probe. Cadence is behavior, not tree
# state; anything grepable here could only pin the last observed wording,
# which is the fake green this file exists to remove.
refusal_drift=""
refusal_drift=$(
  python3 - <<'REFUSALPY'
import re
from pathlib import Path

fails = []
doc = Path("docs/MAINTENANCE.md").read_text()

# Locale refusal vs LOCALES.
bullet = re.search(r"^- \*\*New UI locales[^\n]*", doc, re.M)
if not bullet:
    fails.append('docs/MAINTENANCE.md: no "New UI locales" bullet in Not planned')
else:
    named = re.search(r"\bbeyond ([a-z]{2}(?:/[a-z]{2})*)\b", bullet.group(0))
    doc_locales = set(named.group(1).split("/")) if named else set()
    src = Path("web/src/lib/i18n/types.ts").read_text()
    arr = re.search(r"LOCALES\s*=\s*\[([^\]]*)\]", src)
    if not arr:
        fails.append("web/src/lib/i18n/types.ts: cannot parse the LOCALES array")
    else:
        shipped = set(re.findall(r"['\"]([a-z]{2})['\"]", arr.group(1)))
        if doc_locales != shipped:
            fails.append(
                "docs/MAINTENANCE.md names %s; the tree ships %s "
                "(web/src/lib/i18n/types.ts LOCALES) — reconcile the two"
                % (sorted(doc_locales) or ["no locale set"], sorted(shipped))
            )

# Terminal refusal vs terminal.* message keys.
if not re.search(r"^- \*\*Terminal tabs, splits, or profiles\.\*\*", doc, re.M):
    fails.append(
        'docs/MAINTENANCE.md: no "Terminal tabs, splits, or profiles" bullet '
        "in Not planned — the doc must answer the first such request"
    )
else:
    for path in sorted(Path("web/src/lib/i18n/messages").glob("*.ts")):
        text = path.read_text()
        for m in re.finditer(r"'(terminal\.[A-Za-z0-9_.]*)'\s*:", text):
            if re.search(r"tab|split|profile", m.group(1), re.I):
                line = text.count("\n", 0, m.start()) + 1
                fails.append(
                    "%s:%d: %s — MAINTENANCE.md refuses terminal tabs/splits/"
                    "profiles; reconcile doc and tree" % (path.as_posix(), line, m.group(1))
                )

if fails:
    print("\n".join(fails))
REFUSALPY
)
if [[ -n "$refusal_drift" ]]; then
  fail "a Not-planned refusal contradicts the shipped tree:"$'\n'"$refusal_drift"
fi
ok "Not-planned refusals match the shipped tree (locale set, terminal ceiling)"

# ── 37. Write handlers never mint via s.client() (GDK-681) ────────────────
# tools/check-write-handlers.sh runs TestWriteHandlersDoNotCallClient, the
# AST lock keeping issue write handlers on writerFor / keyWriter /
# createWriter: origin.Client is Jira-only, and a Linear apiKey still
# passes HasCredential, so the 409 gate does not save those handlers. The
# script had existed since GDK-681 with nothing executing it — an unrun
# guard is a comment — so doc-checks carries it the way check 35 carries
# check-promises.sh.
_gate_require write-handlers
ok "write handlers do not call s.client() (TestWriteHandlersDoNotCallClient)"

# ── 38. MCP tool descriptions name values the code actually emits ─────────
# The MCP tool text is the one surface with no gate at all: a shell-less
# agent reads it, no test asserts on it, and go / e2e / the rest of this
# script are green whatever it claims. GDK-1278's vocabulary rename walked
# straight into that — a `\bstandalone\b` sweep rewrote the enum inside
# `toolStatusDescription`, so the description advertised
# `kind: connected|builtIn` while the server kept sending `standalone`.
# Every other contract string the sweep ate was caught by a gate; this one
# was caught by a human re-reading the diff.
#
# The rule: each `(a|b|c)` enum in a description must be spelled somewhere
# in the non-test Go source as a quoted literal. That is what separates a
# real value from an invented one — `"standalone"` appears 25 times,
# `"builtIn"` zero.
mcp_enum_drift=$(python3 - <<'MCPPY'
import re, subprocess
from pathlib import Path

src = Path("internal/mcp/tools.go").read_text()
tokens = set()
for group in re.findall(r"\(([A-Za-z_]+(?:\|[A-Za-z_]+)+)\)", src):
    tokens.update(group.split("|"))

files = subprocess.run(
    ["bash", "-c", "git ls-files 'cmd/**/*.go' 'internal/**/*.go' | grep -v '_test\\.go$'"],
    capture_output=True, text=True, check=True).stdout.split()
literals = set()
for f in files:
    if f == "internal/mcp/tools.go":
        continue
    literals.update(re.findall(r'"([A-Za-z_]+)"', Path(f).read_text()))

missing = sorted(t for t in tokens if t not in literals)
if missing:
    print("\n".join(f"  {t}: named in an MCP enum, never a string literal in the Go source" for t in missing))
MCPPY
)
if [[ -n "$mcp_enum_drift" ]]; then
  fail "MCP tool descriptions teach values the code never emits:"$'\n'"$mcp_enum_drift"
fi
ok "MCP tool description enums name values the Go source emits"

# ── 39. docs/SUPPORT_MATRIX.md keeps its shape (GDK-1300) ────────────────
# Class: a support matrix is a map a reader plans around, and it is the
# single owner of the origin table — the READMEs summarize and link. This
# check pins only the structure: the file exists, the first table row is
# the four-origin header (Jira Cloud, Jira Server, Linear, Built-in — the
# Server column landed 2026-09-09 with GDK-1634), every data row carries
# exactly four cells of ✅/◐/— each with a footnote marker, every used
# marker has exactly one definition, and both READMEs link here instead of
# carrying a second table. Whether a
# cell still tells the truth is review's job — the doc's own "How this
# file is maintained" section names the files whose commits must update
# it; generating cells from refusal code instead of review is GDK-1301.
# Per-cell refusal mapping is deliberately absent.
#
# FAIL-first 2026-09-02, recorded against this branch's tree:
#   - file moved aside: the run goes red at the docs/README.md index check
#     first ("broken relative links in docs/README.md: SUPPORT_MATRIX.md",
#     exit 1), and this check's own existence branch prints
#     "docs/SUPPORT_MATRIX.md is missing — the matrix needs its single
#     owner" (run standalone with the file moved).
#   - one cell's footnote dropped (`✅[^4]` → `✅` in the full-text search
#     row): "FAIL: docs/SUPPORT_MATRIX.md drifted:
#     line 27: origin cell 2 is '✅', want ✅[^n] / ◐[^n] / —[^n]" — exit 1.
#   - as shipped: green ("docs/SUPPORT_MATRIX.md keeps its shape; the
#     READMEs link to it").
matrix_drift=""
matrix_drift=$(
  python3 - <<'MATRIXPY'
from pathlib import Path
import re

doc = Path("docs/SUPPORT_MATRIX.md")
if not doc.exists():
    print("docs/SUPPORT_MATRIX.md is missing — the matrix needs its single owner")
    raise SystemExit
text = doc.read_text()
lines = text.splitlines()

HEADER = "| Capability | Jira Cloud | Jira Server | Linear | Built-in |"
header_at = None
for i, l in enumerate(lines):
    s = l.strip()
    if s.startswith("|"):
        if s != HEADER:
            print(f"line {i+1}: first table row is {s!r}, want {HEADER!r}")
        header_at = i
        break
if header_at is None:
    print("docs/SUPPORT_MATRIX.md has no table")
    raise SystemExit

MARKER = re.compile(r"^(✅|◐|—)\[\^([0-9]+)\]$")
used = set()
rows = 0
for i in range(header_at + 1, len(lines)):
    s = lines[i].strip()
    if not s.startswith("|"):
        break  # the table ended
    if set(s) <= set("|-: "):
        continue  # separator row
    rows += 1
    cells = [c.strip() for c in s.strip("|").split("|")]
    if len(cells) != 5:
        print(f"line {i+1}: {len(cells)} cells, want 5 (capability + 4 origins)")
        continue
    for j, c in enumerate(cells[1:], 2):
        m = MARKER.match(c)
        if not m:
            print(f"line {i+1}: origin cell {j} is {c!r}, want ✅[^n] / ◐[^n] / —[^n]")
        else:
            used.add(m.group(2))
if rows == 0:
    print("docs/SUPPORT_MATRIX.md: the table has no data rows")

defined_list = re.findall(r"^\[\^([0-9]+)\]:", text, re.M)
defined = set(defined_list)
for n in sorted(used - defined, key=int):
    print(f"footnote [^{n}] is used in a cell but never defined")
# Defined twice is worse than undefined: the renderer picks one and the cell
# points at the wrong origin's sentence. FAIL-first 2026-09-09: [^106] and
# [^107] were each defined twice (Linear links / link-type catalog, then the
# sprint rows added on top of them the day before) and this check was green.
for n in sorted({x for x in defined_list if defined_list.count(x) > 1}, key=int):
    print(f"footnote [^{n}] is defined {defined_list.count(n)} times")

for f in ("README.md", "README.ko.md", "README.ja.md"):
    body = Path(f).read_text()
    if "docs/SUPPORT_MATRIX.md" not in body:
        print(f"{f}: no link to docs/SUPPORT_MATRIX.md — the matrix needs its readers")
    for i, l in enumerate(body.splitlines(), 1):
        if l.startswith("| Capability |"):
            print(f"{f}:{i}: carries a Capability table — docs/SUPPORT_MATRIX.md is the single owner")
MATRIXPY
)
if [[ -n "$matrix_drift" ]]; then
  fail "docs/SUPPORT_MATRIX.md drifted:"$'\n'"$matrix_drift"
fi
ok "docs/SUPPORT_MATRIX.md keeps its shape; the READMEs link to it"

# ── 40. Every /media/ path the site asks for resolves to a file (GDK-1501) ─
# The site serves docs/media through a symlink the build creates
# (Makefile `site:`), so a reference to a file that does not exist costs
# nothing at build time — Astro copies a directory, it does not resolve the
# strings inside a component. The page just renders a broken <video> or a
# poster-less black rectangle, and the only instrument was a person opening
# gadak.dev. This is that instrument.
#
# Two halves. First, every literal /media/<file> under site/src must exist in
# docs/media/. Second, every locale listed in MEDIA_LOCALES must have its own
# cut on disk: that map is what makes Landing.astro serve a Japanese clip, so
# an entry added before the recording lands would ship a 404, and a recording
# that lands without the entry ships the English take to everyone.
#
# FAIL-first (2026-09-07, both halves measured on this tree):
#   - a reference to a file that is not there: Landing.astro's poster changed
#     to /media/scale-poster-missing.png →
#     "FAIL: the site references media that is not in docs/media/:
#      site/src/components/Landing.astro: /media/scale-poster-missing.png" — exit 1
#   - a locale claimed before its recording exists: MEDIA_LOCALES entry
#     '/media/scale.mp4': ['ja'] with no docs/media/scale.ja.mp4 →
#     "FAIL: MEDIA_LOCALES claims locale cuts that are not in docs/media/:
#      /media/scale.mp4 [ja] -> docs/media/scale.ja.mp4" — exit 1
#   - as shipped: green.
media_missing=$(
  python3 - <<'MEDIAPY'
from pathlib import Path
import re

media = Path("docs/media")
for path in sorted(Path("site/src").rglob("*")):
    if not path.is_file() or path.suffix not in {".astro", ".ts", ".js", ".md", ".css"}:
        continue
    # A reference, not prose: the path sits in quotes and ends in a file
    # extension. Comments in these files write docs/media/og.<lang>.png
    # and similar, and a bare /media/ grep reads those as broken links.
    # (Template literals still quote the path itself -- Base.astro's
    # og:image is mediaFor(lang, '/media/og.png') inside the backticks.)
    # NOTE: every string below is single-quoted and the double quote is
    # spelled chr(34). This heredoc lives inside $( ... ), and bash scans
    # for the closing paren counting quote characters even through a quoted
    # heredoc -- an odd number of them here is a syntax error in the whole
    # file, not a python problem.
    QUOTED = '(?<=[' + chr(39) + chr(34) + '])/media/([A-Za-z0-9._-]+[.][A-Za-z0-9]{2,4})'
    refs = set(re.findall(QUOTED, path.read_text(encoding='utf-8')))
    for ref in sorted(refs):
        if not (media / ref).is_file():
            print(f"{path}: /media/{ref}")
MEDIAPY
)
if [[ -n "$media_missing" ]]; then
  fail "the site references media that is not in docs/media/:"$'\n'"$media_missing"
fi

locale_missing=$(
  python3 - <<'LOCALEPY'
from pathlib import Path
import re

src = Path("site/src/i18n.ts").read_text(encoding="utf-8")
m = re.search(r"export const MEDIA_LOCALES[^=]*=\s*\{(.*?)\n\}", src, re.S)
if not m:
    print("site/src/i18n.ts: no MEDIA_LOCALES map -- mediaFor() has no owner")
    raise SystemExit(0)

media = Path("docs/media")
entries = re.findall(r"'(/media/[^']+)':\s*\[([^\]]*)\]", m.group(1))
if not entries:
    print("site/src/i18n.ts: MEDIA_LOCALES has no entries -- did the shape change?")
for ref, locs in entries:
    stem, _, ext = ref.rpartition(".")
    for loc in re.findall(r"'([a-z]{2})'", locs):
        want = f"{stem[len('/media/'):]}.{loc}.{ext}"
        if not (media / want).is_file():
            print(f"{ref} [{loc}] -> docs/media/{want}")
LOCALEPY
)
if [[ -n "$locale_missing" ]]; then
  fail "MEDIA_LOCALES claims locale cuts that are not in docs/media/:"$'\n'"$locale_missing"
fi
ok "every /media/ path under site/src resolves, and every MEDIA_LOCALES cut exists"

# ── 41. Every locale that ships a clip has a fixture translation (GDK-1556) ─
# A localized recording is two files, not one: the cut in docs/media/ (check 40
# above) and the translation of the mirror it was recorded over. Without the
# second, `make media-search` for that locale is a hard error at record time —
# which is the right failure, but it lands on whoever tries to re-record rather
# than on whoever added the locale. This puts it on the commit.
#
# Only the locales MEDIA_LOCALES actually serves are required: listing one
# there is what turns a variant on, so it is the same both-halves rule the
# check above applies to the video files. Entries that are not recordings are
# named below rather than inferred, so a *new clip* is covered by default and
# only a new Node render has to be declared -- the safe direction.
#
# FAIL-first (2026-09-07, measured on this tree with '/media/scale.mp4': ['ko']
# in site/src/i18n.ts and no examples/demo-i18n/ko.json):
#   "FAIL: MEDIA_LOCALES serves a locale with no fixture translation:
#    /media/scale.mp4 [ko] -> examples/demo-i18n/ko.json" — exit 1.
#   With the file present, and as shipped (no non-en locale listed): green.
i18n_missing=$(
  python3 - <<'I18NPY'
from pathlib import Path
import re

# Not recordings: nothing here is shot over the demo mirror, so no translation
# of that mirror applies. og.png is a Node render (tools/brand/render.mjs) whose
# copy comes from site/src/tagline.js.
NOT_RECORDED = {"/media/og.png"}

src = Path("site/src/i18n.ts").read_text(encoding="utf-8")
m = re.search(r"export const MEDIA_LOCALES[^=]*=\s*\{(.*?)\n\}", src, re.S)
if not m:
    print("site/src/i18n.ts: no MEDIA_LOCALES map -- mediaFor() has no owner")
    raise SystemExit(0)

for ref, locs in re.findall(r"'(/media/[^']+)':\s*\[([^\]]*)\]", m.group(1)):
    if ref in NOT_RECORDED:
        continue
    for loc in re.findall(r"'([a-z]{2})'", locs):
        if loc == "en":
            continue
        want = Path(f"examples/demo-i18n/{loc}.json")
        if not want.is_file():
            print(f"{ref} [{loc}] -> {want}")
I18NPY
)
if [[ -n "$i18n_missing" ]]; then
  fail "MEDIA_LOCALES serves a locale with no fixture translation:"$'\n'"$i18n_missing"$'\n'"write it with tools/demo-i18n/extract.py + a translation, gate with tools/demo-i18n/check.py"
fi
ok "every MEDIA_LOCALES locale has its examples/demo-i18n/<locale>.json"

# ── 42. A script that calls itself a gate is wired into something that runs ─
# (v0.21 release audit: unwired-script finding). check-lockfile-platforms.sh and ci-status-test.sh both existed
# with "gate" in their own headers and nothing anywhere executing them — an
# unrun guard is a comment. The rule scans only the automation surfaces
# (Makefile, .github/workflows/, this file, other tools/*.sh); a README
# mention is documentation, not wiring. One transitive level is allowed so a
# fixture test can vouch for its subject (ci-status-test.sh runs
# ci-status.sh): deeper chains are where unwired scripts hide, so one is the
# ceiling. tools/*-test.sh is required wiring even with no "gate" wording —
# a test that never runs is the illusion of safety in its purest form.
#
# FAIL-first (2026-09-08, measured on the pre-wiring tree):
#   "FAIL: scripts that call themselves gates but are wired into nothing:
#    tools/check-lockfile-platforms.sh
#    tools/ci-status-test.sh
#    tools/ci-status.sh" — exit 1.
#   With checks 43-44 below running the first two, all three pass (the third
#   transitively, through ci-status-test.sh).
unwired=$(
  python3 - <<'GATEPY'
from pathlib import Path
import re

tools = sorted(Path("tools").glob("*.sh"))
if not tools:
    print("(no tools/*.sh found — is this the repo root?)")
    raise SystemExit(0)

def leading_comment(path: Path) -> str:
    out = []
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        if not line.startswith("#"):
            break
        out.append(line)
    return "\n".join(out)

automation = ["Makefile"]
automation += [str(p) for p in Path(".github/workflows").glob("*.yml")]
automation += [str(p) for p in Path(".github/workflows").glob("*.yaml")]
automation.append("tools/doc-checks.sh")
automation += [str(p) for p in tools]

def executable_text(path: str) -> str:
    """Corpus text with comment lines blanked — Makefile/bash/yaml all use #.
    A script named in a comment is not wired; only executable lines count
    (measured: with comments counted, this check passed while the only
    reference was the section header announcing the run)."""
    out = []
    for line in Path(path).read_text(encoding="utf-8", errors="replace").splitlines():
        out.append("" if line.lstrip().startswith("#") else line)
    return "\n".join(out)

corpus = {p: executable_text(p) for p in automation if Path(p).is_file()}

def references(name: str, corpus_path: str) -> bool:
    text = corpus.get(corpus_path, "")
    if Path(corpus_path).name == name:
        return False  # a script naming itself is not wiring
    return name in text

# The run surfaces: anything whose own execution makes a reference real.
run_surfaces = ["Makefile", "tools/doc-checks.sh"] + [
    p for p in automation if p.startswith(".github/workflows/")
]
directly_wired = {t.name for t in tools if any(references(t.name, p) for p in run_surfaces)}

for t in tools:
    name = t.name
    header = leading_comment(t)
    calls_itself_gate = re.search(r"\bgates?\b", header, re.I) is not None or "게이트" in header
    is_test_script = name.endswith("-test.sh")
    if not (calls_itself_gate or is_test_script):
        continue
    wired = name in directly_wired or any(
        references(name, p) and Path(p).name in directly_wired for p in automation
    )
    if not wired:
        why = "gate" if calls_itself_gate and not is_test_script else (
            "gate + fixture test" if calls_itself_gate else "fixture test (tools/*-test.sh)"
        )
        print(f"tools/{name}  [{why}]")
GATEPY
)
if [[ -n "$unwired" ]]; then
  fail "scripts that call themselves gates but are wired into nothing:"$'\n'"$unwired"$'\n'"wire them into the Makefile, .github/workflows/, or this file — or stop calling them gates in their own headers"
fi
ok "every gate-worded tools/*.sh (and every tools/*-test.sh) is wired into something that runs"

# ── 43. check-lockfile-platforms.sh actually runs (v0.21 release audit: unwired-script finding) ────────────────
# The script had existed since the 2026-08-26 incident with nothing executing
# it; check 42 above would have kept re-flagging it. Carried the way check 35
# carries check-promises.sh: this file runs in CI's "Documentation factuality"
# step, so the delegated run is the wiring.
_gate_require lockfile
_stamp 'check 43 check-lockfile-platforms.sh (replayed)'

# ── 44. ci-status-test.sh actually runs (v0.21 release audit: unwired-script finding) ──────────────────────────
# Same unwired-script class, plus the *-test.sh rule from check 42: a fixture
# test that never runs guards nothing. Offline by construction (fake gh on
# PATH, fixtures under tools/ci-status-fixtures/). Cases 7-8 walk the parent
# commit, which a CI shallow checkout (actions/checkout depth 1) does not
# have — say the skip out loud rather than failing there or silently passing.
if git rev-parse --verify -q "HEAD^" >/dev/null 2>&1; then
  _gate_require ci-status
  _stamp 'check 44 ci-status-test.sh (replayed)'
else
  echo "note: the ci-status fixture test is skipped — shallow checkout has no HEAD^, and its parent look-back cases (7-8) need real history. Full run locally or with fetch-depth: 0."
fi

# ── 45. llms.txt carries the front door's contract strings (GDK-1659) ────────
# site/public/llms.txt is the page an agent reads instead of the landing, and
# nothing asserted it: on 2026-09-09 it was fourteen days behind the READMEs —
# no Windows Store line, the Claude Desktop command GDK-1633 retired, and
# "source of truth" twice. Checks 3 and 6 now read it too (demo count,
# version); this one pins the install commands and the facts the review rounds
# found wrong most often. FAIL-first 2026-09-09: the pre-rewrite file had
# neither `gadak mcp install claude-desktop` nor the Store URL.
llms=site/public/llms.txt
for want in \
  'brew install --cask midagedev/tap/gadak' \
  'brew install midagedev/tap/gadak-cli' \
  'gadak init && gadak serve' \
  'http://gadak.localhost:7777' \
  'gadak mcp install claude-desktop' \
  'https://apps.microsoft.com/detail/9NZW91TXH36G' \
  'https://gadak.dev/ko/' \
  'https://gadak.dev/ja/' \
  'docs/project/FACT_LEDGER.md'; do
  if ! grep -qF -- "$want" "$llms"; then
    fail "$llms is missing the contract string: $want"
  fi
done
ok "site/public/llms.txt carries the install commands, both MCP hosts, the Store URL and the ko/ja pages"

# ── 46. a release is told in at most three themes, never as a list (user decision 2026-09-09) ──
# "이전 버전 대비 달라진 핵심이 잡혀야 하는데 자꾸 사건들의 나열이 된다": a
# release section is two or three bold-led theme paragraphs that say what
# changed since the previous version, with every GDK key cited inside one of
# them. Top-level bullets are the shape of an incident log and are refused
# outright. Continuation paragraphs (no bold lead) belong to the theme above
# them. FAIL-first 2026-09-09: Unreleased carried 26 bullets, v0.17.0 twelve
# themes, and the three files disagreed on the count. Runs on all three
# editions so the structure stays one history.
#
# GDK-1808 (2026-09-12): counting bold heads and `- ` prefixes was the wrong
# axis. The event log came back as *unbolded continuation* paragraphs — read
# their first sentences vertically and they are a bullet list ("Three sync
# corrections." / "Three things on the phone side." / "Five pieces of
# infrastructure…") — and the check stayed green on 54 paragraphs, 13,373
# words and 348 key citations in Unreleased. Three assertions were added:
#   · total paragraphs per section, continuations included (the shape rule);
#   · no paragraph appears twice inside one section (an editing accident that
#     had shipped: check 27 compares key *sets* and is blind to duplication,
#     and the reference-tail generator reads a repeated citation as normal);
#   · English section words, which is what closes the third disguise — eight
#     paragraphs of 1,670 words each satisfies a paragraph cap alone.
# They live here rather than in a sibling check because this script exits at
# the first failure: a check numbered after this one would report nothing
# until this one is green, and the duplicate is exactly the thing that has
# to be visible in the same red run.
# FAIL-first 2026-09-12 (this tree, before the rewrite): Unreleased is 54
# paragraphs in all three editions; the duplicate pair is 51/53 in en and ko
# and 35/37 in ja; the English section is 13,373 words. All shipped sections
# pass all three.
python3 - <<'PY46' || fail "changelog section shape broken (see above)"
import re, sys

# Caps are the measured shipped history, not taste.
#
# PARA_CAP: the largest release ever shipped in *any* edition is ko v0.19.0 at
# 8 paragraphs (3 theme heads + 5 continuations; en and ja tell the same
# release in 7). Counting only bold heads let 54 through. 8 also matches the
# shape the contract describes — at most three themes, each allowed to run on
# — so a section that needs a ninth paragraph is asking for a fourth theme.
PARA_CAP = 8
# WORDS_CAP: English only. `str.split()` counts spaces, and Japanese has few:
# the same Unreleased section is 13,373 "words" in en and 3,124 in ja, so one
# threshold cannot serve all three. The editions are the same history (checks
# 27 and 53 pin that), so the English number catches the bloat for all of
# them. Largest shipped English section, measured 2026-09-12, is v0.20.0 at
# 830 words (the failure message prints the live value).
#
# The cap has two terms, because a flat number punishes a large release
# rather than an uncompressed one (lead, 2026-09-12, rewriting 0.22 from
# 13,373 words to 3,741). WORDS_FLOOR is ~3x the largest shipped section, so
# every release that has ever shipped stays green on the floor alone.
# WORDS_PER_KEY scales it with the work the section actually carries: a
# release citing 322 keys legitimately needs more prose than one citing 45,
# and words-per-key is the axis that separates a compressed narrative from an
# event log. 15 is tighter than the tightest shipped precedent — v0.20.0 is
# 830/45 = 18.4 — so this is a stricter rule per unit of work, not a looser
# one. It is deliberately NOT set to whatever the 0.22 rewrite happened to
# measure: a first pass at 12 landed the rewrite at 3,972 of a 3,972 cap,
# which is a threshold fitted to one draft rather than a contract, and would
# have made every later sentence a prose-surgery exercise.
#
# FAIL-first, measured against the pre-rewrite tree: 13,373 words at 322 keys
# is 2.8x the derived cap, and the "glue the event log into eight
# mega-paragraphs" evasion (6,007 words, 129 keys) is 2.3x the floor.
WORDS_FLOOR = 2600
WORDS_PER_KEY = 15


def sections(path):
    """(head, body, paragraphs) per release, reference definitions removed.

    Any `[label]: url` line is a tail the renderer never shows, not prose —
    stripping only `[GDK-nnn]:` would let one `[issuetap]: …` definition eat a
    paragraph slot out of PARA_CAP, invisibly. Measured 2026-09-12: all 766
    definitions in each edition are GDK keys, so widening costs nothing today.
    """
    s = open(path, encoding="utf-8").read()
    for sec in re.split(r"^(?=## )", s, flags=re.M)[1:]:
        head = sec.split("\n", 1)[0].strip()
        body = re.sub(r"^\[[^\]]+\]:.*$", "", sec[len(head):], flags=re.M)
        yield head, body, [p for p in re.split(r"\n\s*\n", body) if p.strip()]


# The biggest English release actually shipped, read at run time rather than
# quoted: a message that hardcodes today's record is wrong the moment the next
# release beats it, and this is a file whose whole job is catching stale facts.
en_max = max(
    ((len(b.split()), h.lstrip("# ").split(" — ")[0]) for h, b, _ in sections("CHANGELOG.md") if h != "## Unreleased"),
    default=(0, "none"),
)

bad = []
for f in ("CHANGELOG.md", "CHANGELOG.ko.md", "CHANGELOG.ja.md"):
    for head, body, paras in sections(f):
        themes = sum(1 for p in paras if re.match(r"\s*\*\*", p))
        bullets = [l for l in body.splitlines() if l.startswith("- ")]
        if bullets:
            bad.append(f"{f} {head!r}: {len(bullets)} top-level bullet(s) — tell it as a theme paragraph")
        if themes > 3:
            bad.append(f"{f} {head!r}: {themes} themes (max 3)")
        if themes == 0 and paras and head != "## Unreleased":
            bad.append(f"{f} {head!r}: no bold-led theme paragraph")
        if len(paras) > PARA_CAP:
            bad.append(
                f"{f} {head!r}: {len(paras)} paragraphs (cap {PARA_CAP}) — "
                f"{themes} bold-led, {len(paras) - themes} continuation; "
                f"continuations count too, and the cap is the largest release "
                f"ever shipped (ko v0.19.0, 3 themes + 5 continuations)"
            )
        # Byte-identical prose twice in one section is an editing accident, not
        # a style call. Normalise whitespace so a rewrap is not a difference;
        # report 1-based indices, because that is how a section reads.
        seen = {}
        for i, p in enumerate(paras, 1):
            key = " ".join(p.split())
            if key in seen:
                bad.append(
                    f"{f} {head!r}: paragraphs {seen[key]} and {i} are the same "
                    f"text ({len(key.split())} words) — {key[:80]}…"
                )
            else:
                seen[key] = i
        if f == "CHANGELOG.md":
            words = len(body.split())
            n_keys = len(set(re.findall(r"GDK-\d+", body)))
            cap = max(WORDS_FLOOR, WORDS_PER_KEY * n_keys)
            if words > cap:
                bad.append(
                    f"{f} {head!r}: {words} words (cap {cap} = max({WORDS_FLOOR}, "
                    f"{WORDS_PER_KEY} x {n_keys} keys); the largest release "
                    f"shipped so far is {en_max[1]} at {en_max[0]} words) — a "
                    f"paragraph cap alone is satisfiable by concatenation, so "
                    f"the words are counted too. English edition only: CJK word "
                    f"counts are not comparable."
                )
for b in bad:
    print("  " + b)
sys.exit(1 if bad else 0)
PY46
ok "every changelog release is at most three themes and eight paragraphs, no bullets, no paragraph told twice, in all three editions"


# ── 47. the fact ledger's contract strings are in the files it names (GDK-1602) ──
# The ledger is prose, and prose drifts: the 2026-09-08 brand round changed
# the line in the three READMEs and missed tools/hosted-demo/build.mjs, which
# sets it as the hosted demo's title, its OG and Twitter titles, and a visible
# tagline. §1 of the ledger now says "a later change to the line must grep,
# not count" — this is the grep. §17 carries the machine-readable half as
# `file :: string` lines in a ```ledger-contract block; each is asserted
# verbatim. FAIL-first 2026-09-09: dropping the tagline line from build.mjs
# fails here, and nothing else in this file noticed.
python3 - <<'PY47' || fail "fact ledger contract strings broken (see above)"
import re, sys
led = "docs/project/FACT_LEDGER.md"
src = open(led, encoding="utf-8").read()
blocks = re.findall(r"```ledger-contract\n(.*?)```", src, re.S)
if not blocks:
    print(f"  {led}: no ```ledger-contract block — check 47 has nothing to assert")
    sys.exit(1)
bad, n = [], 0
for line in "".join(blocks).splitlines():
    line = line.strip()
    if not line or line.startswith("#"):
        continue
    if " :: " not in line:
        bad.append(f"{led}: malformed entry (want `file :: string`): {line}")
        continue
    path, want = line.split(" :: ", 1)
    path, want = path.strip(), want.strip()
    try:
        body = open(path, encoding="utf-8").read()
    except OSError as e:
        bad.append(f"{led} names {path}, which cannot be read: {e}")
        continue
    n += 1
    if want not in body:
        bad.append(f"{path} is missing the ledger's contract string: {want}")
for b in bad:
    print("  " + b)
if not bad and n == 0:
    print(f"  {led}: the contract block is empty")
    sys.exit(1)
sys.exit(1 if bad else 0)
PY47
ok "every contract string the fact ledger names is in the file it names"

# ── 48. audit-test.sh actually runs (GDK-1707: census scripts promoted to tools/audit/) ──
# The five tools/audit/*.sh census pages claim a contract (exit 0, markdown
# header, ## Sources with commands, no paths outside the repo); audit-test.sh
# asserts it, including on deliberately bad pages, and covers ci-ledger's
# gh-missing degradation offline via a fake gh. Carried the way checks 43-44
# carry their fixture tests: this file runs in CI's "Documentation factuality"
# step, so the delegated run is the wiring (check 42's *-test.sh rule).
_gate_require audit
_stamp 'check 48 audit-test.sh (replayed)'

# -- 49. every items_fts writer names all five columns (GDK-1021) --
# 0009 SS Consequences names the trap: items_fts is contentless, so a writer
# that omits a column produces an index that is empty on that axis rather than
# broken - no error, no failing test, just search silently losing a whole
# class of hit. It has now happened twice (cjk_bigram in GDK-259, labels in
# GDK-1021), and the second time the writer that lagged was
# tools/demo-i18n/apply.py, which no Go test compiles and no gate opened. So
# the census is mechanical: every INSERT INTO items_fts in the tree, in any
# language, must name the full column list, and every CREATE VIRTUAL TABLE
# items_fts must declare the same columns and the canonical tokenizer from
# internal/store/schema.go. contentless_delete is deliberately absent from the
# portable Datasette Lite snapshot (GDK-112), so only columns and tokenizer
# are asserted here.
if ! python3 tools/fts-writer-census.py; then
  fail "an items_fts writer is missing a column or the canonical tokenizer (see above)"
fi
ok "every items_fts writer names all five columns and the canonical tokenizer"

# ── 50. mirrored grammars agree across their two owners ───────────────────
# The GDK-27 census class "a regex copied to a second owner ages separately":
# the host allowlist and home-path pattern (backlog-scrub-check ↔
# scan-internal), the profile-name grammar (config ↔ deeplink), and the
# ui-token family (config/tokencheck·uitokens·settings ↔ user-tokens.ts).
# Each pair had a mirror declaration in comments and nothing executable —
# widening one side ships a gate that disagrees with its twin (a Go gate
# widened silently drops user styles after reload; a host allowed in one
# allowlist is a leak in the other). tools/mirror-pins.sh extracts both
# copies from the live source and compares, normalizing the deltas each
# pair declares. FAIL-first 2026-09-10, one diverge per pin, all red with
# both owners named: allowlist `example` dropped, PAT_HOMEPATH `._-`→`.-`,
# deeplink {0,63}→{0,31}, FONT_IDENT_RE {0,63}→{0,64}.
if ! bash tools/mirror-pins.sh; then
  fail "a mirrored grammar pair drifted (both owners are named above)"
fi
ok "mirror pins: host allowlist, home path, profile grammar, ui-token family agree"

# ── 51. README links resolve, and a path-shaped label is its target (GDK-1625) ──
# Class one: a relative link or image whose target does not exist costs
# nothing at build time — the page renders it clickable and the click is a
# 404. docs/README.md has had a resolver since check 32; the three front
# doors did not. Markdown links and the <img src>/<a href> forms both
# count: the demo recording is an <img>, so a markdown-only resolver would
# skip 13 of the front door's references.
#
# Class two: a label that looks like a filename is the path a reader types
# when a click is not available. [`CONTRIBUTING.md`](.github/CONTRIBUTING.md)
# names a file that is not at the root; README.ja.md already writes the
# full path in the label, which is the convention this check pins.
#
# en is a failure; ko prints its labels to stderr as notes, not failures —
# the ko edition is outside this round's file set and carries the same two
# labels the en fix landed (CONTRIBUTING.md, data-model.md), for the lead.
# FAIL-first 2026-09-11 (read-only, against `git show HEAD:README.md`):
# the pre-fix en file reports both label mismatches; a copy pointing at
# docs/nope.md fails the resolver half.
readme_links=$(
  python3 - <<'LINKPY'
import re
import sys
from pathlib import Path

SCHEME = re.compile(r"^[a-zA-Z][a-zA-Z0-9+.-]*:")
LABEL_PATHY = re.compile(r"\.[A-Za-z][A-Za-z0-9]{0,4}$")
# Quote parity: this heredoc lives inside $( ... ), and bash scans for the
# closing paren counting quote characters even through a quoted heredoc —
# the raw double quotes the <img src> regex needs are spelled chr(34), and
# the backtick strip below is chr(96), so neither count can go odd (the
# same constraint check 40 documents).
DQ = chr(34)
BT = chr(96)
IMG = re.compile(r"<img\s[^>]*?src=" + DQ + "([^#" + DQ + "]+)" + DQ)
MD = re.compile(r"!?\[([^\]]*)\]\(\s*<?([^)<>\s]+)>?")
fails = []
for name, enforce in (("README.md", True), ("README.ko.md", False), ("README.ja.md", False)):
    text = Path(name).read_text(encoding="utf-8")
    found = [(m.start(), "", m.group(1)) for m in IMG.finditer(text)]
    found += [(m.start(), m.group(1).strip(), m.group(2)) for m in MD.finditer(text)]
    for pos, label, target in found:
        if SCHEME.match(target):
            continue
        rel = target.split("#", 1)[0]
        line = text.count("\n", 0, pos) + 1
        if rel and not Path(rel).exists():
            fails.append("%s:%d: target %s does not exist" % (name, line, target))
        lab = label.strip(BT).strip()
        if lab and ("/" in lab or LABEL_PATHY.search(lab)) and lab != rel:
            msg = ("%s:%d: label %r is not the path it points at (%s)"
                   % (name, line, lab, rel))
            if enforce:
                fails.append(msg)
            else:
                print("note: " + msg, file=sys.stderr)
if fails:
    print("\n".join(fails))
LINKPY
)
if [[ -n "$readme_links" ]]; then
  fail "a README link is broken, or its label is not its target:"$'\n'"$readme_links"
fi
ok "README en/ko/ja: every relative target resolves; path-shaped labels are their targets"

# ── 52. A *Caption string names only its locale's own language (GDK-1625) ──
# Clips are per-language assets (MEDIA_LOCALES, checks 40/41): a locale is
# served its own take, and the caption beneath describes that take. A
# caption naming another language — "a Japanese take" under the en clip,
# 일본어 in a ko caption — describes an asset that locale never serves.
# Only *Caption keys are pinned: nameNote says the word gadak is Korean in
# every locale (fact ledger §1) and langBanner names the page's own
# language by design; neither describes a clip.
# FAIL-first 2026-09-11 on a mutated copy of i18n.ts ("a Japanese take"
# in the en videoCaption): one hit, the shipped captions are clean.
caption_lang=$(
  python3 - <<'CAPTIONPY'
import re
from pathlib import Path

src = Path("site/src/i18n.ts").read_text(encoding="utf-8")
m = re.search(r"export const strings[^=]*=\s*\{(.*?)\n\}\n", src, re.S)
if not m:
    print("site/src/i18n.ts: cannot find the strings table")
    raise SystemExit(0)
LANGS = {"English": "en", "영어": "en", "英語": "en",
         "Korean": "ko", "한국어": "ko", "韓国語": "ko",
         "Japanese": "ja", "일본어": "ja", "日本語": "ja"}
parts = re.split(r"\n  (en|ko|ja): \{", m.group(1))
if len(parts) < 6:
    print("site/src/i18n.ts: cannot split the strings table into locale blocks")
    raise SystemExit(0)
# offsets so the failure names a real line: parts join back to the body.
offs, o = [], 0
for p in parts:
    offs.append(o)
    o += len(p)
for i in range(1, len(parts), 2):
    loc, chunk, coff = parts[i], parts[i + 1], offs[i + 1] + m.start(1)
    for cm in re.finditer(r"([A-Za-z]+Caption)\s*:\s*'([^']*)'", chunk):
        for tok in (t for t in LANGS if t in cm.group(2)):
            if LANGS[tok] != loc:
                line = src.count("\n", 0, coff + cm.start()) + 1
                print("site/src/i18n.ts:%d: %s in the %s locale names %s — "
                      "this locale serves the %s cut"
                      % (line, cm.group(1), loc, tok, loc))
CAPTIONPY
)
if [[ -n "$caption_lang" ]]; then
  fail "a *Caption string names a language its locale does not serve:"$'\n'"$caption_lang"
fi
ok "every *Caption in site/src/i18n.ts names only its own locale's language"

# ── 53. Three parallel editions, not one text in three fonts (GDK-1604) ──
# The READMEs are parallel editions with their own readers (user decision
# 2026-09-08): they share facts, not structure. The drift direction is
# one-way — translations accrete structural identity — and the 2026-09-08
# review found exactly that (all three at 53 paragraphs, 9 headings). The
# predicate is the issue's own: two editions sharing BOTH the paragraph
# count and the heading sequence read as one text in three fonts. Measured
# green on this check's landing tree: 49/43/81 paragraphs, 10/11/26
# headings, no pair identical.
# FAIL-first 2026-09-11 on mutated copies in scratch: three structurally
# identical READMEs fail with the pair named.
#
# Em-dash half (en only; ko/ja punctuate with their own marks): at most 1
# per 300 words of prose, fences and code spans stripped. Threshold
# derivation (GDK-1604, 2026-09-11): the shipped en README measures 5
# dashes over 1,738 words — 1 per ~348, at the cap. The cap pins the
# frontier the 2026-09-08 rewrite set; it is not a target to grow into.
readme_style=$(
  python3 - <<'STYLEPY'
import re
from pathlib import Path

# Backtick parity (same constraint as check 51's quote note): fence and
# code-span patterns are built from chr(96) so the heredoc body carries no
# literal backtick for the $( ... ) scan to mispair.
BT = chr(96)
FENCE = BT * 3 + ".*?" + BT * 3
SPAN = BT + "[^" + BT + r"\n]+" + BT


def shape(text):
    text = re.sub(FENCE, "", text, flags=re.S)
    paras = [p for p in re.split(r"\n\s*\n", text) if p.strip()]
    heads = tuple(re.sub(r"^#+\s*", "", l) for l in text.splitlines()
                  if re.match(r"^#+\s", l))
    return len(paras), heads


editions = {n: shape(Path(n).read_text(encoding="utf-8"))
            for n in ("README.md", "README.ko.md", "README.ja.md")}
names = sorted(editions)
for i in range(len(names)):
    for j in range(i + 1, len(names)):
        a, b = editions[names[i]], editions[names[j]]
        if a == b:
            print("%s and %s share the same paragraph count (%d) and heading "
                  "sequence — parallel editions, not translations"
                  % (names[i], names[j], a[0]))

prose = Path("README.md").read_text(encoding="utf-8")
prose = re.sub(FENCE, "", prose, flags=re.S)
prose = re.sub(SPAN, "", prose)
dashes = prose.count("—")
words = len(re.findall(r"\S+", prose))
if dashes > words // 300:
    print("README.md: %d em dashes over %d words of prose — the cap is 1 per "
          "300 (%d)" % (dashes, words, words // 300))
STYLEPY
)
if [[ -n "$readme_style" ]]; then
  fail "the README editions drifted toward translation shape, or the em-dash density cap broke:"$'\n'"$readme_style"
fi
ok "README editions keep distinct shapes; en prose is within the em-dash cap"

# ── 54. CHANGELOG tails are the generated form ───────────────────────────
# (2026-09-11 docs round; the issue key is not yet on the public backlog —
# tools/changelog-tails.py's header records the incident.) A CHANGELOG
# edition's reference tail is derivation, not prose: the bracket citations
# of the body, in numeric order, each with the public-backlog URL. Hand
# maintenance drifted to 54 append-time descents per edition, all three
# the same way, because nothing compared the order. Check 27 keeps owning
# the set and the URL form; this check owns the order and keeps the
# generator executed (an unrun guard is a comment). FAIL-first 2026-09-11
# against the pre-fix tree: --check reported the 54 descents on all three
# editions, exit 1, before --write regenerated them.
if ! python3 tools/changelog-tails.py --check; then
  fail "a CHANGELOG edition's reference tail is not the generated form (see above) — python3 tools/changelog-tails.py --write <file>"
fi
ok "all three CHANGELOG tails are the generated form (numeric order, one definition per cited key)"

# ── 55. "connected"/"standalone" stay wire values, not category nouns (GDK-1288) ──
# The 2026-09-02 vocabulary split made the category words Jira / Linear /
# Built-in. `connected` and `standalone` survive as wire values
# (kind: connected, standalone-jira:) and in Atlassian's own
# atlas-run-standalone. The recurrence is prose using them as the
# workspace category — "a connected workspace", "a standalone origin" —
# the shape of both instances this round fixed (backup-restore runbook,
# SUPPORT_MATRIX footnote 139). Fenced blocks and backtick spans are
# stripped before matching so wire values stay legal; dated records
# (docs/decisions/, docs/audits/, docs/research/) are excluded because
# naming the old vocabulary is what a record is for.
# FAIL-first 2026-09-11 against `git show HEAD:docs/runbooks/backup-restore.md`
# (read-only): the pre-fix paragraph is one hit; the fixed tree is zero.
vocab_hits=$(
  python3 - "$FILE_CENSUS" <<'VOCABPY'
import re
import sys
from pathlib import Path

CENSUS = [Path(p) for p in Path(sys.argv[1]).read_text().splitlines() if p]
# Backtick parity, as in check 53: strip patterns are built, not literal.
BT = chr(96)
FENCE = BT * 3 + ".*?" + BT * 3
SPAN = BT + "[^" + BT + r"\n]+" + BT
PATTERN = re.compile(r"\b(connected|standalone)\s+(workspace|origin|tracker|account)\b", re.I)
SKIP_DIR = {"decisions", "audits", "research", "node_modules", "dist", "media"}
ROOTS = {Path("README.md"), Path("README.ko.md"), Path("README.ja.md"),
         Path("AGENTS.md"), Path("skills/gadak/SKILL.md")}


def candidates():
    yield from ROOTS
    for path in CENSUS:
        if path.suffix not in {".md", ".ts", ".astro"}:
            continue
        if any(p in SKIP_DIR for p in path.parts):
            continue
        if path.parts[0] in {"docs", "site", ".github", "skills"}:
            yield path


for path in sorted(set(candidates())):
    if not path.is_file():
        continue
    text = path.read_text(encoding="utf-8", errors="replace")
    text = re.sub(FENCE, "", text, flags=re.S)
    text = re.sub(SPAN, "", text)
    for m in PATTERN.finditer(text):
        line = text.count("\n", 0, m.start()) + 1
        print("%s:%d: %s" % (path.as_posix(), line, m.group(0)))
VOCABPY
)
if [[ -n "$vocab_hits" ]]; then
  fail "prose uses connected/standalone as the workspace category — name the tracker (Jira / Linear / Built-in) instead (GDK-1288):"$'\n'"$vocab_hits"
fi
ok "connected/standalone appear only as wire values; prose names the tracker"

# Phrase pins are wrap-tolerant: a multi-word sentence in flowing prose can
# break anywhere, and a line-based grep would call a present sentence gone
# (measured 2026-09-11, r4 of this round: AGENT_ACCESS.md carried the
# sentence with "site-side" and "subscribers" on adjacent lines and this
# check failed while the doc was correct — the instrument, not the tree,
# was red). Collapse newlines before matching.
_pin() {  # _pin <file> <phrase> — phrase presence, wrapped or not
  tr '\n' ' ' < "$1" | tr -s ' ' | grep -qF -- "$2"
}

# ── 56. The two watch meanings stay named where agents read (GDK-524) ───
# "watch" names three things in this product: the sync loop
# (`gadak sync --watch`), the local follows table (`local.db`, exported
# beside favorites and recents), and Jira's site-side watchers — which
# sync never copies and only `gadak api` reaches. An agent that mixes the
# last two asks the site about a local star, or expects a watchers column
# in the mirror. The disambiguation lives in the two agent-facing
# documents and the export paragraph; these presence pins are the same
# shape as check 20's `init --local`: a rewrite that drops the sentence
# loses the gate's token with it.
# FAIL-first 2026-09-11 against `git show HEAD:` of both agent docs: the
# pre-fix files contain none of the pin phrases.
watch_pins=""
for f in skills/gadak/SKILL.md docs/AGENT_ACCESS.md; do
  if ! _pin "$f" 'site-side subscribers'; then
    watch_pins+="  $f: no site-side-subscribers sentence (GDK-524)"$'\n'
  fi
done
if ! _pin docs/CONFIGURATION.md 'site-side watchers'; then
  watch_pins+="  docs/CONFIGURATION.md: the export list no longer separates the two watch meanings (GDK-524)"$'\n'
fi
if [[ -n "$watch_pins" ]]; then
  fail "the watch disambiguation is gone from an agent-facing doc:"$'\n'"$watch_pins"
fi
ok "SKILL.md, AGENT_ACCESS.md and CONFIGURATION.md keep the local-follows vs site-watchers split"

# ── 57. CONTRIBUTING's front stays a paragraph, not a reading wall ──────
# (2026-09-11 docs round; issue key not yet on the public backlog.) The
# must-read wall — five bulleted documents gating a first PR — was the
# contribution front door's largest object while saying nothing a
# contributor acts on: the build is `make build` and the rules get caught
# in review. The five documents stayed, as links with their one-line
# purposes. The recurrence is the wall's shape returning: a bulleted
# must-read list under a "read:" imperative. FAIL-first 2026-09-11 against
# `git show HEAD:.github/CONTRIBUTING.md`: the pre-fix block trips the
# first pin and misses both positive pins.
if grep -n -E '^Before contributing, read:' .github/CONTRIBUTING.md; then
  fail ".github/CONTRIBUTING.md has a must-read wall again — link the documents, don't gate the first PR on reading them"
fi
if ! awk 'NR<=25' .github/CONTRIBUTING.md | tr '\n' ' ' | grep -q 'make build'; then
  fail ".github/CONTRIBUTING.md no longer says the build is make build in its opening"
fi
if ! _pin .github/CONTRIBUTING.md 'Read on need'; then
  fail ".github/CONTRIBUTING.md lost the on-need pointer that replaced the must-read wall"
fi
ok "CONTRIBUTING.md front door: build named, rules caught in review, docs linked on need"
# ── 58. fenced dashboard examples never compare display names (GDK-783) ──
# docs/DASHBOARDS.md is the authoring cookbook for SQL datasources, and its
# fenced examples are the part a reader copies verbatim. A fenced
# `status = '…'` teaches the locale trap the product itself warns about at
# runtime (internal/sqlhint ZeroRowDisplayNameWarning): 0 rows on any
# account whose status names are not English. The teaching table under
# "instead of | use" is deliberately out of scope — its cells are inline
# code in prose, not a fence, and each shows the wrong spelling beside the
# right one. Word boundaries keep status_category / priority_rank /
# issue_type_id fences legal, same as the Go side. The regex body is the
# same grammar as hint.go's displayNameFilterRe; tools/mirror-pins.sh pair 5
# pins the two verbatim, so widening either side alone fails check 50.
# FAIL-first 2026-09-11: a fenced `status = 'In Progress' -- done' example
# appended under "Writing queries" failed here naming the file:line;
# removing it was green.
python3 - <<'PY51' || fail "a fenced dashboard example compares a display name (GDK-783)"
import re, sys
from pathlib import Path

# mirror-pins.sh pair 5 extracts this literal and hint.go's backtick body
# and requires them equal — edit both in one commit or neither.
DISPLAY_NAME_FILTER_RE = r"(?i)\b(status|issue_type|issuetype|priority)\s*="

pat = re.compile(DISPLAY_NAME_FILTER_RE)
bad = []
in_fence = False
for lineno, line in enumerate(Path("docs/DASHBOARDS.md").read_text().splitlines(), 1):
    if line.startswith("```"):
        in_fence = not in_fence
        continue
    if in_fence and pat.search(line):
        bad.append(f"docs/DASHBOARDS.md:{lineno}: {line.strip()}")
for b in bad:
    print("  " + b)
sys.exit(1 if bad else 0)
PY51
ok "fenced dashboard examples never compare status/priority/type display names"

echo "doc-checks: all passed"
