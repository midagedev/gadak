#!/usr/bin/env bash
# Stale-doc guards. Grep/sqlite only — no network, no Makefile/CI hook.
#
# Checks (FAIL-first was recorded against the pre-fix tree in the 2026-08-15
# census fix round; see scratch/yc/progress-stale.log):
#   1. README.md wraps web-demo.gif in <details> (not an inline hero)
#   2. docs/MCP.md and contracts/agent.md do not list {text} as gadak_search's
#      primary argument (query is primary; text/q are aliases)
#   3. README.md and docs/INSTALL.md say "N issues" matching examples/demo.db
#   4. docs/STATE_OF_PLAY.md has no leftover "enables GitHub Pages"
#   5. docs/, specs/, AGENTS.md carry no literal 519 (demo issue count moved)
#   6. The version a reader sees first matches the latest tag: the README
#      status lines (en + ko) and STATE_OF_PLAY's "Last tagged" (2026-08-15:
#      README said 0.13 on a 0.14 tree, and the ko header claimed v0.14 while
#      its own status line said 0.13 — found by the first-impression census)
#   7. Web logic does not key status / priority / issue type on a localized
#      display name (GDK-28/29). FAIL-first: the pre-fix format.ts table
#      `/highest|긴급|가장 높음|blocker/` (and friends) is kept at
#      /tmp/gadak-priority-gdk28/format.ts and fails this grep.
#   8. PROMISES.md names the same outbound destinations SECURITY.md
#      enumerates (GDK-104). FAIL-first 2026-08-15: adding "| plausible.io"
#      to the PROMISES.md marker made this check fail before it was reverted.
#
# Usage: tools/doc-checks.sh
# Exit 0 = clean, 1 = a check failed.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

ok() {
  echo "ok: $*"
}

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
for f in README.md docs/INSTALL.md; do
  if ! grep -q "${n} issues" "$f"; then
    fail "$f does not mention ${n} issues (demo.db count)"
  fi
done
ok "README.md and docs/INSTALL.md say ${n} issues"

# ── 4. Hosted demo is live; no "enables GitHub Pages" leftover ───────────
if grep -n "enables GitHub Pages" docs/STATE_OF_PLAY.md; then
  fail "docs/STATE_OF_PLAY.md still says \"enables GitHub Pages\""
fi
ok "docs/STATE_OF_PLAY.md has no \"enables GitHub Pages\""

# ── 5. No leftover 519 inventory literal ─────────────────────────────────
# CHANGELOG is history and is not under these paths.
hits="$(grep -rn "519" docs specs AGENTS.md || true)"
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
  for f in README.md README.ko.md; do
    if ! grep -qE "(Status|상태): ${minor}(,| )" "$f"; then
      fail "$f status line does not say ${minor} (latest tag ${tag})"
    fi
  done
  if ! grep -q "Last tagged: ${tag}" docs/STATE_OF_PLAY.md; then
    fail "docs/STATE_OF_PLAY.md does not say \"Last tagged: ${tag}\""
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

# ── 8. PROMISES.md outbound list agrees with SECURITY.md ────────────────
# SECURITY.md enumerates outbound destinations as a numbered list under
# "Outbound traffic is exactly N destinations:"; PROMISES.md repeats that set
# in an <!-- outbound: A | B --> marker beside its own outbound promise. A
# destination added to one file must not leave the other claiming fewer.
outbound_diff="$(python3 - <<'PY'
import re
from pathlib import Path

words = {"one": 1, "two": 2, "three": 3, "four": 4, "five": 5}
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

pm = re.search(r"<!--\s*outbound:(.+?)-->", Path("PROMISES.md").read_text())
if not pm:
    print("PROMISES.md: no <!-- outbound: … --> marker")
    raise SystemExit
listed = {norm(x) for x in pm.group(1).split("|")}
if listed != {norm(x) for x in declared}:
    print(f"PROMISES.md lists {sorted(listed)}; SECURITY.md declares "
          f"{sorted(norm(x) for x in declared)}")
PY
)"
if [[ -n "$outbound_diff" ]]; then
  fail "PROMISES.md outbound list disagrees with SECURITY.md:"$'\n'"$outbound_diff"
fi
ok "PROMISES.md outbound list matches SECURITY.md"

# ── 9. Both onboarding paths warn about the token traps before the 401 ──
# The web form and `gadak init` ask for the same token, and Atlassian's page
# offers three things that look like one: a scoped token (recommended first),
# an org key from admin.atlassian.com, and the user token that actually works.
# Each surface carries its own copy — TS and Go share no string table — so the
# invariant is pinned here instead: whoever edits one is told about the other.
token_copy_missing=""
for f in web/src/lib/i18n/en.ts web/src/lib/i18n/ko.ts cmd/gadak/init.go; do
  # The hint on the credential prompt, not the error shown after a rejection:
  # a warning that arrives after the failure it describes is the bug (GDK-98).
  case "$f" in
    *init.go) hint="$(sed -n '/tokenTrapHint/,/^$/p' "$f")" ;;
    *)        hint="$(sed -n "/'onboarding.tokenHint'/,/^  '/p" "$f")" ;;
  esac
  # ATATT/ATCTT are Atlassian's own prefixes and read the same in every
  # locale; the scoped-token trap is prose, so the Korean copy says 스코프.
  for trap in 'ATATT' 'ATCTT' 'scope|스코프'; do
    grep -Eqi -- "$trap" <<<"$hint" || token_copy_missing+="  $f: token hint does not name ${trap%%|*}"$'\n'
  done
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
for f in README.md README.ko.md docs/INSTALL.md docs/DESKTOP.md; do
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
    README.md README.ko.md docs/INSTALL.md docs/DESKTOP.md \
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
  python3 - <<'GDK306PY'
import re
from pathlib import Path

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
for path in Path(".").rglob("*.md"):
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
#   AGENTS.md (the agent cookbook; same inclusion as checks 5 and 12)
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
env_ghosts=$(python3 - <<'GDK281PY'
import re
from pathlib import Path

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


def read_names():
    found = set()
    for path in Path(".").rglob("*"):
        if not path.is_file():
            continue
        if any(p in SKIP_READ_DIR for p in path.parts):
            continue
        name = path.name
        suffix = path.suffix
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


docs = documented()
read = read_names()
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
env_census=$(python3 - <<'GDK281CENSUSPY'
import re
from pathlib import Path

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
for path in Path(".").rglob("*.go"):
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
#   README.md          0× `init --standalone` (INSTALL.md already had 1)
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
    # Connected-scoped mention is allowed. Unqualified "just delete
    # ~/.gadak" is the class that wipes a standalone origin.
    if re.search(r"\bconnected\b", p, re.I):
        continue
    hits.append(re.sub(r"\s+", " ", p.strip())[:220])
if hits:
    print("\n".join(hits))
GDK373PY
)
if [[ -n "$faq_rm_unscoped" ]]; then
  fail "docs/FAQ.md has an unscoped \`rm -rf ~/.gadak\` (GDK-373) — scope it to a connected workspace:"$'\n'"$faq_rm_unscoped"
fi
ok "docs/FAQ.md does not tell a standalone user to rm -rf ~/.gadak"

standalone_cmd_missing=""
for f in docs/INSTALL.md README.md; do
  if ! grep -q 'init --standalone' "$f"; then
    standalone_cmd_missing+="  $f: no init --standalone"$'\n'
  fi
done
if [[ -n "$standalone_cmd_missing" ]]; then
  fail "install front door does not name init --standalone (GDK-271):"$'\n'"$standalone_cmd_missing"
fi
ok "docs/INSTALL.md and README.md name init --standalone"

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
BACKLOG_SNAPSHOT="examples/backlog-snapshot/bootstrap.json"
READER_DOCS=(CHANGELOG.md CHANGELOG.ko.md README.md README.ko.md
  docs/ARCHITECTURE.md docs/DERIVE.md docs/DESKTOP.md docs/INSTALL.md
  docs/ROADMAP.md docs/STATE_OF_PLAY.md desktop/README.md)
if [[ -f "$BACKLOG_SNAPSHOT" ]] && command -v jq >/dev/null; then
  published=$(jq -r '.issues[].issue_key' "$BACKLOG_SNAPSHOT" | sort -u)
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
# AGENTS / SKILL said nothing, so a reader following the front door could
# not pair. Analogous to check 20 (`init --standalone`). The flag name is
# the pin: `cmd/gadak/init.go` registers `--pairing-code-stdin`.
# FAIL-first 2026-08-21 against the unmodified f6-docs tree: all five files
# below had 0 hits for pairing-code-stdin; SKILL.md line 221 said
# `views save` kept a named view "in the mirror" and named local.db 0 times.
pairing_missing=""
for f in README.md README.ko.md docs/INSTALL.md AGENTS.md skills/gadak/SKILL.md; do
  if ! grep -q 'pairing-code-stdin' "$f"; then
    pairing_missing+="  $f: no --pairing-code-stdin"$'\n'
  fi
done
if [[ -n "$pairing_missing" ]]; then
  fail "install/agent front door does not name --pairing-code-stdin (GDK-457):"$'\n'"$pairing_missing"
fi
ok "README, INSTALL, AGENTS, SKILL name --pairing-code-stdin"

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
# The snapshot is one-line JSON (a line-oriented grep of bootstrap.json has
# already gone vacuous twice — tools/backlog-scrub-check.sh). Keys are
# extracted as structural tokens `"key":"GDK-N"`, not by scanning the line.
# An absent snapshot or a snapshot with 0 keys is a fail: that is the
# published set vanishing, not a clean tree.
#
# FAIL-first 2026-08-22 against this worktree's unmodified sources + snapshot:
# 21 distinct keys cited on tracked public surfaces, absent from
# examples/backlog-snapshot/bootstrap.json:
#   GDK-461 GDK-462 GDK-463 GDK-464 GDK-465 GDK-466 GDK-467 GDK-468
#   GDK-469 GDK-470 GDK-474 GDK-476 GDK-477 GDK-478 GDK-479 GDK-481
#   GDK-482 GDK-507 GDK-579 GDK-580 GDK-600
BACKLOG_SNAP="examples/backlog-snapshot/bootstrap.json"
BACKLOG_PRIVATE="tools/backlog-private-keys.txt"

if [[ ! -f "$BACKLOG_SNAP" ]]; then
  fail "public backlog snapshot is missing ($BACKLOG_SNAP) — cannot resolve GDK keys"
fi
if [[ ! -f "$BACKLOG_PRIVATE" ]]; then
  fail "$BACKLOG_PRIVATE is missing — it is the private-key allowlist for this check (GDK-269)"
fi

published=$(grep -oE '"key":"GDK-[0-9]+"' "$BACKLOG_SNAP" | grep -oE 'GDK-[0-9]+' | sort -u) || true
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

cited=$(
  git ls-files \
    | grep -vE '_test\.go$|\.spec\.ts$|^e2e/|^examples/backlog-snapshot/|^tools/backlog-private-keys\.txt$' \
    | xargs grep -oEhI -E '\bGDK-[0-9]+\b' -- \
    | sort -u
) || true

resolved=$(printf '%s\n%s' "$published" "$private" | sed '/^$/d' | sort -u)
dangling=$(comm -23 <(printf '%s\n' "$cited" | sed '/^$/d') <(printf '%s\n' "$resolved") | sort -t- -k2,2n)
if [[ -n "$dangling" ]]; then
  list=$(printf '%s\n' "$dangling" | tr '\n' ' ')
  list="${list%" "}"
  fail "public surfaces cite GDK keys that are not on the public backlog: $list"$'\n'"  to publish a key: edit KEY --label +public, then tools/backlog-snapshot.sh"$'\n'"  to keep it private: add it to $BACKLOG_PRIVATE with a one-line reason"$'\n'"  the lead does both"
fi
ok "every GDK key on a public surface resolves on the public backlog or the private-key allowlist"

# ── 24. Public backlog snapshot keys match tracked detail JSON (GDK-634) ─
# Class: bootstrap.json listing a key whose detail JSON is not git-tracked
# publishes a 404 detail page; a tracked detail whose key is absent from
# bootstrap.json is an orphan the index will not link. Disk presence is not
# enough — commit 2b9cb60 shipped the index without the 21 new detail files
# that sat untracked on disk; 439bb8c closed it by eye.
#
# Existence is git-tracked state (`git ls-files -- examples/backlog-snapshot/detail/`),
# not a filesystem glob. Key extraction from bootstrap.json reuses the
# structural token `"key":"GDK-N"` and the already-non-empty `$published`
# from check 23 (a missing snapshot or 0 keys already failed there).
#
# FAIL-first 2026-08-22 (git state not mutated): drop one published key from
# the tracked-detail variable → missing; append a fake key → orphan.
# Recorded in scratch-634-failfirst.log.
backlog_snapshot_detail_consistency() {
  local published_keys="$1"
  local tracked_keys="$2"
  local missing orphans missing_list orphan_list body
  missing=$(comm -23 <(printf '%s\n' "$published_keys" | sed '/^$/d' | sort -u) \
                     <(printf '%s\n' "$tracked_keys" | sed '/^$/d' | sort -u) | sort -t- -k2,2n)
  orphans=$(comm -13 <(printf '%s\n' "$published_keys" | sed '/^$/d' | sort -u) \
                     <(printf '%s\n' "$tracked_keys" | sed '/^$/d' | sort -u) | sort -t- -k2,2n)
  if [[ -n "$missing" || -n "$orphans" ]]; then
    body="public backlog snapshot index and git-tracked detail JSON are inconsistent (GDK-634):"
    if [[ -n "$missing" ]]; then
      missing_list=$(printf '%s\n' "$missing" | tr '\n' ' ')
      missing_list="${missing_list%" "}"
      body+=$'\n'"  missing tracked detail: $missing_list"
    fi
    if [[ -n "$orphans" ]]; then
      orphan_list=$(printf '%s\n' "$orphans" | tr '\n' ' ')
      orphan_list="${orphan_list%" "}"
      body+=$'\n'"  orphan tracked detail: $orphan_list"
    fi
    body+=$'\n'"  re-run bash tools/backlog-snapshot.sh, then git add examples/backlog-snapshot/ (the lead does both)"
    fail "$body"
  fi
  ok "public backlog snapshot keys match git-tracked detail JSON (GDK-634)"
} # end backlog_snapshot_detail_consistency

tracked_detail=$(
  git ls-files -- examples/backlog-snapshot/detail/ \
    | sed -n 's|.*/\(GDK-[0-9][0-9]*\)\.json$|\1|p' \
    | sort -u
)
backlog_snapshot_detail_consistency "$published" "$tracked_detail"

echo "doc-checks: all passed"
