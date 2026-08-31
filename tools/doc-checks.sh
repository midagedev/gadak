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
  for f in README.md README.ko.md; do
    if ! grep -qE "(Status|상태): ${minor}(,| )" "$f"; then
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
    for path in Path(".").rglob("*.go"):
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
BACKLOG_ARCHIVE="examples/backlog-snapshot.tar.gz"
READER_DOCS=(CHANGELOG.md CHANGELOG.ko.md README.md README.ko.md
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
# door could not pair. Analogous to check 20 (`init --standalone`). The flag
# name is the pin: `cmd/gadak/init.go` registers `--pairing-code-stdin`.
# The agent cookbook moved from AGENTS.md to docs/MIRROR.md (GDK-8); this
# check follows the file, not the old name.
# FAIL-first 2026-08-21 against the unmodified f6-docs tree: all five files
# below had 0 hits for pairing-code-stdin; SKILL.md line 221 said
# `views save` kept a named view "in the mirror" and named local.db 0 times.
pairing_missing=""
for f in README.md README.ko.md docs/INSTALL.md docs/MIRROR.md skills/gadak/SKILL.md; do
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
if [[ -f "$BACKLOG_ARCHIVE" ]] && command -v jq >/dev/null; then
  if bash tools/backlog-scrub-check.sh "$BACKLOG_ARCHIVE" >/dev/null 2>&1; then
    ok "committed backlog snapshot passes the scrub gate (GDK-675)"
  else
    scrub_out=$(bash tools/backlog-scrub-check.sh "$BACKLOG_ARCHIVE" 2>&1 || true)
    fail "committed backlog snapshot fails its scrub gate: $scrub_out"
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
# Same shape as check 20 (`grep -q 'init --standalone'`) and check 22
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
for path in ("CHANGELOG.md", "CHANGELOG.ko.md"):
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

en_titles = [t for t, _, _ in parsed["CHANGELOG.md"]["sections"]]
ko_titles = [t for t, _, _ in parsed["CHANGELOG.ko.md"]["sections"]]
if en_titles != ko_titles:
    fails.append(
        "section headings differ between CHANGELOG.md and CHANGELOG.ko.md: %s vs %s"
        % (en_titles, ko_titles)
    )
else:
    en_map = {t: k for t, k, _ in parsed["CHANGELOG.md"]["sections"]}
    ko_map = {t: k for t, k, _ in parsed["CHANGELOG.ko.md"]["sections"]}
    for title in en_titles:
        e, k = en_map[title], ko_map[title]
        if e == k:
            continue
        only_en = ", ".join(sorted(e - k, key=keynum))
        only_ko = ", ".join(sorted(k - e, key=keynum))
        bits = []
        if only_en:
            bits.append("en-only " + only_en)
        if only_ko:
            bits.append("ko-only " + only_ko)
        fails.append(
            "section %r: en/ko key sets differ (%s)" % (title, "; ".join(bits))
        )

if fails:
    print("\n".join(fails))
CHANGELOGKEYSPY
)
if [[ -n "$changelog_keys" ]]; then
  fail "CHANGELOG key/tail contract broken:"$'\n'"$changelog_keys"
fi
ok "CHANGELOG.md and CHANGELOG.ko.md cite the same keys per section; every citation has a gadak.dev tail"

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
    for path in ("README.md", "README.ko.md"):
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
for name in ("README.md", "README.ko.md", "docs/INSTALL.md"):
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
targets = sorted(Path("docs").glob("*.md")) + [Path("README.md"), Path("README.ko.md")]
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
targets = [Path("README.md"), Path("README.ko.md")] + sorted(Path("docs").rglob("*.md"))
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
bash tools/check-promises.sh

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
bash tools/check-write-handlers.sh
ok "write handlers do not call s.client() (TestWriteHandlersDoNotCallClient)"

echo "doc-checks: all passed"
