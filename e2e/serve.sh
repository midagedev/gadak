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
# (me/ → email → authed), which is asserted in detail.spec.ts.
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
exec "$BIN" serve --addr 127.0.0.1:7877 --static dist/app
