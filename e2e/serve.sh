#!/usr/bin/env bash
# Idempotent fixture + server for Playwright E2E.
# Builds the binary and UI, seeds e2e/.tmp/home from examples/demo.db, injects
# one deploy enrichment, then serves on 127.0.0.1:7877.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TMP="$ROOT/e2e/.tmp"
HOME_DIR="$TMP/home"
BIN="$TMP/scry"
DB="$HOME_DIR/scry.db"
CFG="$HOME_DIR/config.json"

mkdir -p "$TMP" "$HOME_DIR"

echo "[e2e] building scry binary…"
CGO_ENABLED=0 go build -o "$BIN" ./cmd/scry

echo "[e2e] building web UI…"
npm run build

echo "[e2e] seeding home from examples/demo.db…"
cp -f "$ROOT/examples/demo.db" "$DB"
# Drop any leftover WAL/SHM from a previous run so sqlite opens cleanly.
rm -f "${DB}-wal" "${DB}-shm"

# Demo projects + deploy/teamGroups surfaces. The credential is fake — nothing
# in the suite talks to Jira — but its presence must unlock the write UI
# (me/ → email → identified), which is asserted in detail.spec.ts.
cat >"$CFG" <<'EOF'
{
  "site": "https://nimbus.example.com",
  "email": "dana@example.com",
  "token": "e2e-fake-token",
  "projects": ["NMB", "NMA", "NMS"],
  "features": {
    "deploy": true,
    "teamGroups": true
  },
  "groupLabels": {
    "backend": "Backend"
  },
  "groupColors": {
    "backend": "#3b82f6"
  },
  "groupRules": [
    {
      "group": "backend",
      "labels": ["performance", "api"]
    }
  ],
  "staleThresholdHours": 72
}
EOF

# Recordings (not tests) freshen the sync clock: the committed snapshot ages, and
# a demo that opens with "Sync delayed" reads as a defect rather than as the
# freshness guard it is. Tests leave it unset so their timestamps stay fixed.
if [ -n "${SCRY_FRESHEN:-}" ]; then
  echo "[e2e] freshening sync clock (SCRY_FRESHEN)…"
  sqlite3 "$DB" "UPDATE sync_state SET watermark = strftime('%Y-%m-%dT%H:%M:%S.000Z','now'),
                                       last_full_sync_at = strftime('%Y-%m-%dT%H:%M:%S.000Z','now'),
                                       last_error = NULL;
                 UPDATE items SET synced_at = strftime('%Y-%m-%dT%H:%M:%S.000Z','now');
                 UPDATE sources SET synced_at = strftime('%Y-%m-%dT%H:%M:%S.000Z','now');"
fi

echo "[e2e] injecting deploy enrichment on NMB-110…"
sqlite3 "$DB" <<'SQL'
INSERT INTO enrichments (key, kind, payload, source, updated_at)
VALUES (
  'NMB-110',
  'deploy',
  '{"status":{"state":"qa","label":"QA verifiable"},"detail":{"state":"qa"}}',
  'e2e-fixture',
  datetime('now')
)
ON CONFLICT(key, kind) DO UPDATE SET
  payload = excluded.payload,
  source = excluded.source,
  updated_at = excluded.updated_at;

UPDATE sync_state SET version = version + 1;
SQL

export SCRY_HOME="$HOME_DIR"
echo "[e2e] serving on 127.0.0.1:7877 (SCRY_HOME=$SCRY_HOME)…"
# The snapshot references attachments whose bytes cannot be proxied (the fixture
# credential is fake), so seed the cache from the committed images. Without this
# the browser logs 502s and the console-hygiene spec fails.
# --no-sync: the fixture credential is fake; starting the watch loop would
# hammer a non-existent Jira and fail every tick.
exec "$BIN" serve --addr 127.0.0.1:7877 --static dist/app --no-open --no-sync \
  --import-attachments "$ROOT/examples/attachments"
