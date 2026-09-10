package store

// The GDK-1429 standing measurement. CycleTimeP85Hours runs on every
// bootstrap and delta response (read.go flowFields → the flow block), so
// its cost is per-request like LastSessionEnd's; the issue's open question
// is whether the recompute is worth caching. This is the reproducible
// answer at whatever scale matters — 50k finished issues, the whole window
// resolved inside the last 90 days so nothing is filtered out (the worst
// case: the ORDER BY sorts every selected row).
//
//	go test ./internal/store/ -run '^$' -bench 'BenchmarkCycleTimeP85' -count=3
//
// The verdict this bench pins (2026-09-10, round w11-server): ~60ms per
// call at 50k finished issues in the window (60.4 / 61.7 / 61.3 ms, -10
// Apple M1 Pro) — NOT ms-scale. The write-up guessed "ms-scale" before this
// run and the instrument corrected it. Cost is row-linear (scan + sort +
// per-row Scan through database/sql) and every row drags its page in, so a
// real mirror's large `raw` blobs make it worse than this slim fixture.
// Against that: flowFields only reaches the query when
// StaleThresholdHours == 0, and it runs per bootstrap/delta — the same
// per-request shape GDK-1547's 113ms fix closed. The disposition stays
// "record-only" for this round (the issue's own call); this number is the
// record, and the argument for re-opening the caching question if request
// latency shows up at real scale.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// seedFinishedBench bulk-inserts n finished issues, all resolved inside the
// 90-day window before end, cycle_hours spread 1..720 deterministically.
func seedFinishedBench(b *testing.B, db *DB, n int, end time.Time) {
	b.Helper()
	ctx := context.Background()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sources (id, kind) VALUES ('jira', 'jira')`); err != nil {
		b.Fatal(err)
	}
	const batch = 500
	for done := 0; done < n; done += batch {
		endIdx := done + batch
		if endIdx > n {
			endIdx = n
		}
		var items, issues []string
		var itemArgs, issueArgs []any
		for i := done; i < endIdx; i++ {
			id, key := fmt.Sprintf("jira:%d", i), fmt.Sprintf("NMB-%d", i)
			at := end.Add(-time.Duration(i%90*24) * time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
			items = append(items, "(?, 'jira', 'issue', ?, ?, ?, '2026-01-01', ?, '2026-01-02')")
			itemArgs = append(itemArgs, id, fmt.Sprintf("%d", i), key, fmt.Sprintf("issue %d", i), at)
			issues = append(issues, "(?, ?, 'NMB', 'done', 'done', 1, 0, 0, '{}', ?, ?)")
			issueArgs = append(issueArgs, id, key, at, fmt.Sprintf("%d.5", i%720+1))
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at) VALUES `+strings.Join(items, ","),
			itemArgs...); err != nil {
			b.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO issues_raw (item_id, key, project_key, status_id, status_category, priority_rank, reopen_count, comment_count, raw, resolved_at, cycle_hours) VALUES `+strings.Join(issues, ","),
			issueArgs...); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkCycleTimeP85_50k(b *testing.B) {
	db, err := Open(tempDBPath(b))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	end := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	seedFinishedBench(b, db, 50_000, end)
	since := end.Add(-90 * 24 * time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, samples, err := db.CycleTimeP85Hours(ctxB(), since)
		if err != nil || samples != 50_000 {
			b.Fatalf("CycleTimeP85Hours samples = %d, err = %v (want 50_000, nil)", samples, err)
		}
	}
}
