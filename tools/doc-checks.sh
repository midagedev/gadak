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
# Deliberately not matched (would fail the current tree; sibling issue):
#   - view-config.ts RESOLVED_STATUS_NAMES (legacy fallback when
#     status_category is absent)
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

echo "doc-checks: all passed"
