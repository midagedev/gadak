package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// SyncProgressLiveWindow is how long a sync_progress row stays readable after
// its last heartbeat. It must comfortably cover the gap between committed
// pages: a single search page is ≤3s, a Confluence body batch ≤10s, so two
// minutes tolerates a slow origin and a GC pause without ever reading a live
// pass as dead. A process that crashed mid-pass leaves a row that goes stale
// after this window and reads as absent — the next full pass overwrites it.
const SyncProgressLiveWindow = 120 * time.Second

// SyncProgressCutoff is the updated_at floor below which a row is stale. The
// single owner of the comparison stamp, so the SQL reader here and the plain-
// SQL reader in cmd/gadak cannot disagree. Returned in config.ISOMilli — the
// fixed-width UTC layout Now() writes, which is what makes a lexicographic
// >= in SQL a time comparison (and a foreign-format row safely stale).
func SyncProgressCutoff(now time.Time) string {
	return now.UTC().Add(-SyncProgressLiveWindow).Format(config.ISOMilli)
}

// SyncProgressRow is one live pass's heartbeat.
type SyncProgressRow struct {
	SourceID  string
	First     bool
	StartedAt string
	UpdatedAt string
	Fetched   int
	Total     *int // nil when the origin gave no count
}

// BeginSyncProgress starts (or restarts) the heartbeat row for one source:
// fetched 0, no total. An upsert, not an insert — a stale row from a crashed
// pass or a BUSY retry must be overwritten, never stacked beside. This is
// bookkeeping in the AddAPIUsage sense: it goes through write() and never
// bumps the mirror's ETag version.
func (db *DB) BeginSyncProgress(ctx context.Context, sourceID string, first bool) error {
	f := 0
	if first {
		f = 1
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO sync_progress (source_id, first, started_at, updated_at, fetched, total)
			VALUES (?, ?, ?, ?, 0, NULL)
			ON CONFLICT(source_id) DO UPDATE SET
				first = excluded.first,
				started_at = excluded.started_at,
				updated_at = excluded.updated_at,
				fetched = 0,
				total = NULL`,
			sourceID, f, Now(), Now())
		return err
	})
}

// TouchSyncProgress advances the heartbeat after a committed page. total nil
// means this call has no denominator and the stored one, if any, is kept —
// the Jira reconcile phase resets its count to unknown mid-pass and must not
// wipe the total the search phase learned. A row that no longer exists (the
// pass already ended) matches zero rows; that is not an error.
func (db *DB) TouchSyncProgress(ctx context.Context, sourceID string, fetched int, total *int) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		var tot any
		if total != nil {
			tot = *total
		}
		_, err := tx.Exec(`
			UPDATE sync_progress
			SET fetched = ?, updated_at = ?, total = COALESCE(?, total)
			WHERE source_id = ?`,
			fetched, Now(), tot, sourceID)
		return err
	})
}

// EndSyncProgress deletes the row when a pass returns, success or error.
func (db *DB) EndSyncProgress(ctx context.Context, sourceID string) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM sync_progress WHERE source_id = ?`, sourceID)
		return err
	})
}

// SyncProgress returns the live rows only — liveness (updated_at within the
// window of now) is decided here, inside the store, so no reader re-derives
// it. Ordered by start for stable presentation.
func (db *DB) SyncProgress(ctx context.Context) ([]SyncProgressRow, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT source_id, first, started_at, updated_at, fetched, total
		FROM sync_progress
		WHERE updated_at >= ?
		ORDER BY started_at`, SyncProgressCutoff(time.Now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SyncProgressRow{}
	for rows.Next() {
		var r SyncProgressRow
		var first int
		var total sql.NullInt64
		if err := rows.Scan(&r.SourceID, &first, &r.StartedAt, &r.UpdatedAt, &r.Fetched, &total); err != nil {
			return nil, err
		}
		r.First = first != 0
		if total.Valid {
			t := int(total.Int64)
			r.Total = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
