#!/usr/bin/env bash
# Deterministic ~10k-issue benchmark fixture for the perf gate suite.
# examples/demo.db → scry snapshot --scale N --now <pinned> → e2e/perf/.tmp/fixture.db
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

TMP="$ROOT/e2e/perf/.tmp"
BIN="$TMP/scry"
SRC="$ROOT/examples/demo.db"
OUT="$TMP/fixture.db"
# Pin the clock so two builds are byte-identical (snapshot contract).
NOW="2026-08-06T00:00:00Z"
SEED=1
# Target: ~10,000 issues. `scry snapshot --scale N` clones onto new keys until
# the snapshot holds exactly N when N > source count (see internal/snapshot).
TARGET=10000

mkdir -p "$TMP"

if [ ! -f "$SRC" ]; then
  echo "make-fixture: missing $SRC" >&2
  exit 1
fi

if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "make-fixture: sqlite3 is required" >&2
  exit 1
fi

SRC_COUNT="$(sqlite3 "$SRC" 'SELECT COUNT(*) FROM issues;')"
# N = TARGET: snapshot produces exactly N issues when N > source.
# (If TARGET ≤ source, scale is a no-op clone-wise and count stays at source.)
N="$TARGET"
if [ "$N" -le "$SRC_COUNT" ]; then
  echo "make-fixture: TARGET=$TARGET ≤ source count $SRC_COUNT; bump TARGET" >&2
  exit 1
fi

echo "[perf-fixture] source: $SRC ($SRC_COUNT issues)"
echo "[perf-fixture] N calculation: want ~${TARGET} issues; --scale ${N} yields exactly ${N} (N > source ${SRC_COUNT})"
echo "[perf-fixture] --now ${NOW} --seed ${SEED} (deterministic)"

echo "[perf-fixture] building scry binary…"
CGO_ENABLED=0 go build -o "$BIN" ./cmd/scry

echo "[perf-fixture] snapshot --scale ${N}…"
"$BIN" snapshot "$OUT" \
  --from "$SRC" \
  --scale "$N" \
  --seed "$SEED" \
  --now "$NOW" \
  --force

# Documents. `scry snapshot` carries the issue axis only — its copy list has no
# pages or spaces table — so a fixture built from it has zero documents, and a
# gate over that fixture cannot see the document lists at all. That blind spot
# is how an unwindowed list shipped. Cloned straight from the source mirror
# instead, deterministically: same rows, suffixed keys.
PAGE_TARGET=5000
SRC_PAGES="$(sqlite3 "$SRC" 'SELECT COUNT(*) FROM pages;')"
if [ "$SRC_PAGES" -eq 0 ]; then
  echo "make-fixture: source has no pages; the docs budget would measure nothing" >&2
  exit 1
fi
COPIES=$(( (PAGE_TARGET + SRC_PAGES - 1) / SRC_PAGES ))
echo "[perf-fixture] cloning ${SRC_PAGES} pages ×${COPIES} → trimmed to ${PAGE_TARGET}…"

sqlite3 "$OUT" <<SQL
ATTACH DATABASE '$SRC' AS src;
BEGIN;
INSERT OR IGNORE INTO sources SELECT * FROM src.sources;
INSERT OR IGNORE INTO spaces SELECT * FROM src.spaces;

CREATE TEMP TABLE seed AS
  SELECT i.* FROM src.items i JOIN src.pages p ON p.item_id = i.id;
CREATE TEMP TABLE seed_pages AS SELECT * FROM src.pages;
CREATE TEMP TABLE copies AS
  WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM c WHERE n < $COPIES)
  SELECT n FROM c;

INSERT INTO items (id, source_id, kind, external_id, key, title, body_text,
                   author, author_id, url, created_at, updated_at, synced_at)
  SELECT s.id || '-c' || c.n, s.source_id, s.kind, s.external_id || '-c' || c.n,
         s.key || '-c' || c.n, s.title || ' (' || c.n || ')', s.body_text,
         s.author, s.author_id, s.url, s.created_at, s.updated_at, s.synced_at
  FROM seed s, copies c;

-- parent_id is dropped on clones: a cloned hierarchy would nest N copies of the
-- same tree under one root, which is not a shape any mirror produces.
INSERT INTO pages (item_id, space_key, parent_id, version, status, body_adf, labels, excerpt)
  SELECT p.item_id || '-c' || c.n, p.space_key, '', p.version, p.status,
         p.body_adf, p.labels, p.excerpt
  FROM seed_pages p, copies c;

DELETE FROM pages WHERE item_id IN (
  SELECT item_id FROM pages ORDER BY item_id LIMIT -1 OFFSET $PAGE_TARGET
);
DELETE FROM items WHERE kind = 'page' AND id NOT IN (SELECT item_id FROM pages);
COMMIT;
SQL

COUNT="$(sqlite3 "$OUT" 'SELECT COUNT(*) FROM issues;')"
PAGES="$(sqlite3 "$OUT" 'SELECT COUNT(*) FROM pages;')"
BYTES="$(wc -c <"$OUT" | tr -d ' ')"
echo "[perf-fixture] wrote $OUT"
echo "[perf-fixture] issues=$COUNT pages=$PAGES bytes=$BYTES"

if [ "$PAGES" -ne "$PAGE_TARGET" ]; then
  echo "make-fixture: expected $PAGE_TARGET pages, got $PAGES" >&2
  exit 1
fi

if [ "$COUNT" -lt 9000 ] || [ "$COUNT" -gt 11000 ]; then
  echo "make-fixture: expected ~10000 issues, got $COUNT" >&2
  exit 1
fi

echo "[perf-fixture] ok"
