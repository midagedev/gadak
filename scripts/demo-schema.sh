#!/usr/bin/env bash
# One-line summary of a gadak mirror (schema stamp + row counts).
# Default path is the committed fixture. Does not migrate or write.
#
#   bash scripts/demo-schema.sh
#   bash scripts/demo-schema.sh /path/to/gadak.db
#
# Lockstep against the binary is not this script: Open() would hide a lag.
# The gate is `go test ./internal/store -run TestCommittedDemoDBMatchesCurrentSchema -count=1`.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
db="${1:-$ROOT/examples/demo.db}"

if ! command -v sqlite3 >/dev/null; then
  echo "demo-schema.sh: sqlite3 not on PATH" >&2
  exit 1
fi
if [ ! -f "$db" ]; then
  echo "demo-schema.sh: no such file: $db" >&2
  exit 1
fi

sqlite3 "$db" "
SELECT printf(
  'schema=%d issues=%d comments=%d pages=%d item_refs=%d items_fts=%d excerpts=%d',
  (SELECT * FROM pragma_user_version()),
  (SELECT count(*) FROM issues),
  (SELECT count(*) FROM comments),
  (SELECT count(*) FROM pages),
  (SELECT count(*) FROM item_refs),
  (SELECT count(*) FROM items_fts),
  (SELECT count(*) FROM pages WHERE excerpt IS NOT NULL AND excerpt != '')
);
"
