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

COUNT="$(sqlite3 "$OUT" 'SELECT COUNT(*) FROM issues;')"
BYTES="$(wc -c <"$OUT" | tr -d ' ')"
echo "[perf-fixture] wrote $OUT"
echo "[perf-fixture] issues=$COUNT bytes=$BYTES"

if [ "$COUNT" -lt 9000 ] || [ "$COUNT" -gt 11000 ]; then
  echo "make-fixture: expected ~10000 issues, got $COUNT" >&2
  exit 1
fi

echo "[perf-fixture] ok"
