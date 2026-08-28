#!/usr/bin/env bash
# Regenerates mobile/public/demo — the read-only sample workspace bundled
# into the app (GDK-1051) and served by the demo transport
# (mobile/src/lib/demo.ts) at /demo/*.
#
# The bytes come from the same exporter the hosted demo uses, so the bundle
# cannot drift from the API shape (cmd/gadak/export_static.go freezes the
# live handlers):
#   bin/gadak export-static \
#     --db examples/demo.db --attachments examples/attachments \
#     --api-base /api/v1/issues/ --auth-base /api/v1/auth/ <outdir>
#
# Attachments are dropped: the phone renders none. The staged tree is
# guarded before it lands (exit 1 on a hit) — the TestFlight .ipa contract
# (mobile/scripts/testflight-upload.sh) greps the shipped bundle for the
# same reserved string.
#
# Usage: bash mobile/scripts/demo-snapshot.sh   (repo paths are resolved
# from the script's own location, so any cwd works)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

OUT="mobile/public/demo"

echo "== building bin/gadak =="
go build -o bin/gadak ./cmd/gadak

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

echo "== export-static → staging tree =="
bin/gadak export-static \
  --db examples/demo.db \
  --attachments examples/attachments \
  --api-base /api/v1/issues/ \
  --auth-base /api/v1/auth/ \
  "$STAGE"

# The phone renders no attachments; the bundle carries data only.
rm -rf "$STAGE/attachments"

# Name discipline (GDK-1051): the reserved tour string must never appear in
# a shipped asset. Guard the staged tree before it touches the repo.
if grep -rq "demo-tour" "$STAGE"; then
  echo "FAIL: generated demo bundle contains the reserved tour string" >&2
  exit 1
fi

mkdir -p "$OUT"
rm -rf "${OUT:?}"/*
mkdir -p "$OUT/detail"
cp "$STAGE/bootstrap.json" "$OUT/bootstrap.json"
cp "$STAGE/config.json" "$OUT/config.json"
cp "$STAGE"/detail/*.json "$OUT/detail/"

ISSUES="$(python3 -c "import json; print(len(json.load(open('$OUT/bootstrap.json'))['issues']))")"
DETAILS="$(find "$OUT/detail" -name '*.json' | wc -l | tr -d ' ')"
echo "== demo bundle: $ISSUES issues, $DETAILS detail files, $(du -sh "$OUT" | cut -f1) → $OUT"
