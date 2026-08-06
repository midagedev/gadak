#!/usr/bin/env bash
# Serve the ~10k perf fixture on 127.0.0.1:7878 for e2e/perf.
# Isolated from e2e/serve.sh (demo.db on :7877). Does not modify the main suite.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

TMP="$ROOT/e2e/perf/.tmp"
HOME_DIR="$TMP/home"
BIN="$TMP/scry"
FIXTURE="$TMP/fixture.db"
DB="$HOME_DIR/scry.db"
CFG="$HOME_DIR/config.json"
ADDR="127.0.0.1:7878"

mkdir -p "$TMP" "$HOME_DIR"

if [ ! -f "$FIXTURE" ]; then
  echo "[perf-serve] fixture missing — running make-fixture.sh…"
  bash "$ROOT/e2e/perf/make-fixture.sh"
fi

if [ ! -x "$BIN" ]; then
  echo "[perf-serve] building scry binary…"
  CGO_ENABLED=0 go build -o "$BIN" ./cmd/scry
fi

if [ ! -f "$ROOT/dist/app/index.html" ]; then
  echo "[perf-serve] building web UI…"
  npm run build
fi

echo "[perf-serve] seeding home from 10k fixture…"
cp -f "$FIXTURE" "$DB"
rm -f "${DB}-wal" "${DB}-shm"

# Fake credential unlocks identity paths; nothing talks to Jira (--no-sync).
cat >"$CFG" <<'EOF'
{
  "site": "https://nimbus.example.com",
  "email": "dana@example.com",
  "token": "e2e-fake-token",
  "projects": ["NMB", "NMA", "NMS"]
}
EOF

# Run migrations against the home copy (fixture may be an older schema shape
# after snapshot rebuild — status opens the mirror cleanly).
echo "[perf-serve] migrating fixture mirror…"
SCRY_HOME="$HOME_DIR" "$BIN" status >/dev/null

COUNT="$(sqlite3 "$DB" 'SELECT COUNT(*) FROM issues;')"
echo "[perf-serve] issues=$COUNT — serving on ${ADDR} (SCRY_HOME=$HOME_DIR)…"

export SCRY_HOME="$HOME_DIR"
exec "$BIN" serve --addr "$ADDR" --static dist/app --no-open --no-sync
