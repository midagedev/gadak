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
    web/src/lib web/src/components web/src/stores \
    | grep -v '/i18n/' \
    || true
)"
if [[ -n "$name_hits" ]]; then
  fail "web logic keys status/priority/type on a display name:"$'\n'"$name_hits"
fi
ok "web logic does not key status/priority/type on a display name"

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

echo "doc-checks: all passed"
