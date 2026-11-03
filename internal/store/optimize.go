package store

// optimize.go owns the mirror's query-planner statistics (sqlite_stat1).
//
// gadak never ran ANALYZE on a real mirror, so every query planned against
// SQLite's default estimates. Without statistics the planner drove
// CountOneSidedLinks from the items_kind_updated index — every issue row,
// each probing every item of its source — and `gadak doctor` sat silent for
// minutes on a 7.8k-issue mirror (GDK-2068, GitHub #115). links.go pins that
// one query's plan; this file makes the whole mirror measurable.
//
// Why plain ANALYZE and not PRAGMA optimize: measured on the bundled SQLite
// 3.53.3 (modernc.org/sqlite v1.56.0), a bare `PRAGMA optimize` on a fresh
// connection did analyze never-analyzed tables — but the SQLite docs reserve
// the right to skip ("PRAGMA optimize may determine that no analysis is
// required"; the 0x10002 mask exists to force its ANALYZE bit), while
// ANALYZE itself is a deterministic statement whose effect cannot depend on
// a heuristic. The cost is one pass over the mirror's tables: 9 ms on
// 7882 items + 5648 links with 2 KB issue raws (measured 2026-11-03) —
// noise at the end of a full pass that just fetched every issue from the
// origin.
//
// The write goes through write(), not mutate(): statistics are not mirrored
// content and must not advance sync_state.version (GDK-579) — they are
// rebuilt locally, never synced.

import (
	"context"
	"database/sql"
)

// OptimizePlannerStats rebuilds sqlite_stat1 for the whole mirror. The full
// issue pass calls it once at its end (internal/sync/run.go); it is safe on
// any handle at any time and idempotent. A failure is the caller's to log,
// never a reason to fail a sync: a missed refresh costs query plans, not
// data. ANALYZE writes no row for an empty table, so a fresh mirror's empty
// links table carries no statistics until it holds rows — which is exactly
// when they would say something.
func (db *DB) OptimizePlannerStats(ctx context.Context) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `ANALYZE`)
		return err
	})
}
