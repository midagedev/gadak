#!/usr/bin/env bash
# Idempotent fixture + server for Playwright E2E.
# Builds the binary and UI, seeds e2e/.tmp/home-${PORT} from examples/demo.db, injects
# one deploy enrichment, then serves on 127.0.0.1:${PORT}.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Single owner with e2e/helpers.ts e2eServePort(): GADAK_E2E_PORT, default 7877.
# The served-artifact stamp is keyed on this value so a port change moves the
# stamp with the process. Home is port-suffixed so two listeners do not share a db.
PORT="${GADAK_E2E_PORT:-7877}"
if ! [[ "$PORT" =~ ^[1-9][0-9]*$ ]] || [ "$PORT" -gt 65535 ]; then
  echo "[e2e] GADAK_E2E_PORT must be an integer 1-65535, got ${PORT}" >&2
  exit 1
fi
# Already consumed as PORT. gadak serve/status log unknown GADAK_* names.
unset GADAK_E2E_PORT

TMP="$ROOT/e2e/.tmp"
HOME_DIR="$TMP/home-${PORT}"
BIN="$TMP/gadak"
DB="$HOME_DIR/gadak.db"
CFG="$HOME_DIR/config.json"
# Media recordings can scale the fixture up (e2e/demo/scale-demo.spec.ts
# records the 20k-issue flagship over a `gadak snapshot --scale` copy).
# The e2e suite never sets this, so its seed stays the committed fixture.
SEED_DB="${GADAK_SEED_DB:-$ROOT/examples/demo.db}"

mkdir -p "$TMP" "$HOME_DIR"

echo "[e2e] building gadak binary…"
CGO_ENABLED=0 go build -o "$BIN" ./cmd/gadak

echo "[e2e] building web UI…"
npm run build

echo "[e2e] seeding home from ${SEED_DB}…"
cp -f "$SEED_DB" "$DB"
# Drop any leftover WAL/SHM from a previous run so sqlite opens cleanly.
rm -f "${DB}-wal" "${DB}-shm"
# GDK-105 moved personal state (saved views, visits, search history) out of the
# mirror into local.db, and this reseed did not follow it: a spec that saves a
# server-side view left it in place, so the next local run started with an extra
# saved view. Measured — palette.spec.ts saw three "Saved view" rows because a
# previous run's `Shot triage <ts>` (view-delete.spec.ts) was still there. CI
# never saw it: a fresh runner has no previous run. A fixture is fresh or it is
# not, so local.db is seeded here too — by deletion, since the app creates it.
rm -f "$HOME_DIR/local.db" "$HOME_DIR/local.db-wal" "$HOME_DIR/local.db-shm"

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

# GADAK_E2E_SHELL points every pane session at a chosen shell (settings block
# terminal.shell, GDK-896). The suite never sets it, so a normal run still gets
# $SHELL. `npm run test:e2e:wide-prompt` sets it to e2e/ci-shell.sh, which is
# how the Linux CI runner's 24-column prompt — and the line wrapping that comes
# with it — is reproduced on a developer's machine.
if [ -n "${GADAK_E2E_SHELL:-}" ]; then
  SHELL_ABS="$(cd "$(dirname "$GADAK_E2E_SHELL")" && pwd)/$(basename "$GADAK_E2E_SHELL")"
  if [ ! -x "$SHELL_ABS" ]; then
    echo "[e2e] GADAK_E2E_SHELL=${GADAK_E2E_SHELL} is not an executable file" >&2
    exit 1
  fi
  echo "[e2e] terminal.shell = ${SHELL_ABS} (GADAK_E2E_SHELL)"
  CFG_PATH="$CFG" CFG_SHELL="$SHELL_ABS" node --input-type=module -e '
    import { readFileSync, writeFileSync } from "node:fs"
    const cfg = JSON.parse(readFileSync(process.env.CFG_PATH, "utf8"))
    cfg.terminal = { ...(cfg.terminal ?? {}), shell: process.env.CFG_SHELL }
    writeFileSync(process.env.CFG_PATH, JSON.stringify(cfg, null, 2) + "\n")
  '
fi

# Recordings (not tests) freshen the sync clock: the committed snapshot ages, and
# a demo that opens with "Sync delayed" reads as a defect rather than as the
# freshness guard it is. Tests leave it unset so their timestamps stay fixed.
if [ -n "${GADAK_FRESHEN:-}" ]; then
  echo "[e2e] freshening sync clock (GADAK_FRESHEN)…"
  sqlite3 "$DB" "UPDATE sync_state SET watermark = strftime('%Y-%m-%dT%H:%M:%S.000Z','now'),
                                       last_full_sync_at = strftime('%Y-%m-%dT%H:%M:%S.000Z','now'),
                                       last_error = NULL;
                 UPDATE items SET synced_at = strftime('%Y-%m-%dT%H:%M:%S.000Z','now');
                 UPDATE sources SET synced_at = strftime('%Y-%m-%dT%H:%M:%S.000Z','now');"
fi

# Open the copy so repairItemsFTS rebuilds the portable snapshot's items_fts
# (Datasette Lite drops contentless_delete; the inject SQL below writes).
# Schema lag is a hard fail: Open on this copy used to hide a stale
# examples/demo.db and keep e2e green (GDK-671).
echo "[e2e] opening fixture copy (FTS repair for writable e2e home)…"
GADAK_HOME="$HOME_DIR" "$BIN" status >/dev/null
have="$(sqlite3 "$ROOT/examples/demo.db" "PRAGMA user_version")"
want="$(sqlite3 "$DB" "PRAGMA user_version")"
if [ "$have" != "$want" ]; then
  echo "[e2e] examples/demo.db PRAGMA user_version=${have}; this binary's mirror is ${want}." >&2
  echo "[e2e] Rebaseline the committed fixture (Open-migrate a copy, then scripts/scrub-demo-db.py). serve.sh does not migrate over a stale file." >&2
  exit 1
fi

echo "[e2e] injecting deploy enrichment on NMB-110…"
sqlite3 "$DB" <<'SQL'
INSERT INTO source_queries
  (id, source_id, external_id, name, query_text, config, favourite, owner, applied, unsupported, updated_at)
VALUES (
  'jira:e2e-open-nma',
  'jira',
  'e2e-open-nma',
  'Open in NMA',
  'project = NMA AND statusCategory = "In Progress"',
  '{"filters":{"jira_project":["NMA"],"status_category":["inprogress"]},"display":{"group_by":"status_category"}}',
  1,
  'Dana',
  '["project","statusCategory"]',
  '[]',
  datetime('now')
)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  query_text = excluded.query_text,
  config = excluded.config;

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

-- One day of call volume so the settings panel's "Jira calls" row has something
-- to show. The row hides itself at zero, so without this the spec would pass
-- against a panel that lost the row entirely.
INSERT INTO api_usage (day, requests, throttled, server_errors, retries, wait_ms, last_throttled_at)
VALUES (strftime('%Y-%m-%d','now'), 1204, 2, 0, 3, 4500, strftime('%Y-%m-%dT%H:%M:%S.000Z','now'))
ON CONFLICT(day) DO UPDATE SET
  requests = excluded.requests,
  throttled = excluded.throttled,
  retries = excluded.retries,
  wait_ms = excluded.wait_ms,
  last_throttled_at = excluded.last_throttled_at;
SQL

# GDK-590: one agent worker with real touches. The demo snapshot predates the
# bot surface entirely (no users rows, no dev_links, every comment human), so
# the badge / actor filter / linked-by / duration chip specs have nothing to
# see without this. Touches land on NMB-112 and NMB-139 — keys no other spec
# references, and both already have in-progress history so NMB-139's duration
# chip has a deterministic wait ("6m": created 14:56:24.755Z → first
# in-progress 15:03:12.577Z on 2026-07-20).
echo "[e2e] injecting agent worker (acc-e2e-bot) touching NMB-112 / NMB-139…"
sqlite3 "$DB" <<'SQL'
INSERT INTO users (source_id, account_id, name, email, account_type)
VALUES ('jira', 'acc-e2e-bot', 'Claude (build 1)', '', 'agent')
ON CONFLICT(source_id, account_id) DO UPDATE SET
  name = excluded.name,
  account_type = excluded.account_type;

INSERT INTO comments (id, item_id, external_id, author, author_id, body_text, created_at)
VALUES
  ('jira:c-e2e-bot-1', 'jira:10317', 'c-e2e-bot-1', 'Claude (build 1)', 'acc-e2e-bot',
   'claimed from the triage queue; reproducing on staging now', '2026-07-16T02:10:00.000Z'),
  ('jira:c-e2e-bot-2', 'jira:10344', 'c-e2e-bot-2', 'Claude (build 1)', 'acc-e2e-bot',
   'fix up — PR linked from the dev panel', '2026-07-20T15:05:00.000Z')
-- comments is keyed (item_id, id) since schemaV39 (GDK-1179).
ON CONFLICT(item_id, id) DO UPDATE SET
  body_text = excluded.body_text;

-- comments rows ride the detail payload; the list column is stored, so the
-- row count and the badge it implies have to agree.
UPDATE issues SET comment_count = comment_count + 1 WHERE key IN ('NMB-112', 'NMB-139');

-- A dev-panel link the bot attached to a human's PR (GDK-589's two axes):
-- author is the PR author, actor is who linked it.
INSERT INTO dev_links (item_id, kind, external_id, url, title, status, author, actor, actor_name, branch, updated_at)
VALUES ('jira:10344', 'pullrequest', 'e2e-pr-9',
        'https://github.com/acme/api/pull/9', 'fix(NMB-139): retry budget for upload', 'open',
        'human-dev', 'acc-e2e-bot', 'Claude (build 1)', 'fix/nmb-139-retry', '2026-07-20T15:04:00.000Z')
ON CONFLICT(item_id, url) DO UPDATE SET
  actor = excluded.actor,
  actor_name = excluded.actor_name;

UPDATE sync_state SET version = version + 1;
SQL

export GADAK_HOME="$HOME_DIR"
WORKTREE="$(git rev-parse --show-toplevel)"
DIGEST="$(bash "$ROOT/e2e/served-digest.sh")"
# The shell the pane's sessions get is not a git fact, so served-digest.sh
# cannot see it — but it changes what the suite measures, and
# reuseExistingServer would otherwise hand a wide-prompt run the server a
# plain run left behind. Fold it in so the stamp mismatch says so out loud.
if [ -n "${GADAK_E2E_SHELL:-}" ]; then
  DIGEST="${DIGEST} shell=${GADAK_E2E_SHELL}"
fi
STAMP="${TMPDIR:-/tmp}/gadak-e2e-served-${PORT}.json"
echo "[e2e] served worktree ${WORKTREE} digest ${DIGEST}"
STAMP_PATH="$STAMP" STAMP_WORKTREE="$WORKTREE" STAMP_DIGEST="$DIGEST" node --input-type=module -e '
  import { writeFileSync } from "node:fs"
  writeFileSync(process.env.STAMP_PATH, JSON.stringify({
    worktree: process.env.STAMP_WORKTREE,
    digest: process.env.STAMP_DIGEST,
  }) + "\n")
'
echo "[e2e] serving on 127.0.0.1:${PORT} (GADAK_HOME=$GADAK_HOME)…"
# The snapshot references attachments whose bytes cannot be proxied (the fixture
# credential is fake), so seed the cache from the committed images. Without this
# the browser logs 502s and the console-hygiene spec fails.
# --no-sync: the fixture credential is fake; starting the watch loop would
# hammer a non-existent Jira and fail every tick.
exec "$BIN" serve --addr 127.0.0.1:${PORT} --static dist/app --no-open --no-sync \
  --import-attachments "$ROOT/examples/attachments"
