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

echo "doc-checks: all passed"
