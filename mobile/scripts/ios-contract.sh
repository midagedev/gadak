#!/usr/bin/env bash
# DESIGN.md §4.1 / §4.2 as an executable gate (GDK-868).
# A browser reports zero safe-area insets, so §4.1 cannot be measured in
# Playwright until a simulator round (GDK-838). §4.2 is the CSS-unit
# contract: no viewport-height units under src/.
#
# Usage: from mobile/, bash scripts/ios-contract.sh
set -euo pipefail
cd "$(dirname "$0")/.."

python3 - <<'PY'
from pathlib import Path
import re
import sys

src = Path("src")
if not src.is_dir():
    print("ios-contract: src/ is missing", file=sys.stderr)
    sys.exit(1)

# CSS viewport-height units (not the letters in a comment like "no vh/dvh").
UNIT = re.compile(r"(?<![-A-Za-z])\d+(?:\.\d+)?(?:vh|dvh|svh|lvh)\b", re.I)
SAFE = "env(safe-area-inset-"

suffixes = {".css", ".svelte", ".ts", ".js"}
files = sorted(
    p for p in src.rglob("*") if p.is_file() and p.suffix in suffixes
)

safe_fail = []
unit_fail = []
for path in files:
    text = path.read_text(encoding="utf-8")
    rel = path.as_posix()
    if SAFE in text and rel != "src/app.css":
        safe_fail.append(rel)
    if UNIT.search(text):
        unit_fail.append(rel)

errors = 0
if safe_fail:
    errors += 1
    print("FAIL: DESIGN.md §4.1 — only src/app.css may mention env(safe-area-inset-*):", file=sys.stderr)
    for rel in safe_fail:
        print(f"  {rel}", file=sys.stderr)
else:
    print("ok: §4.1 only src/app.css mentions env(safe-area-inset-*)")

if unit_fail:
    errors += 1
    print("FAIL: DESIGN.md §4.2 — no CSS vh/dvh/svh/lvh units under src/:", file=sys.stderr)
    for rel in unit_fail:
        print(f"  {rel}", file=sys.stderr)
else:
    print("ok: §4.2 no CSS vh/dvh/svh/lvh units under src/")

sys.exit(errors)
PY
