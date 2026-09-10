package store

// The session-boundary cost benchmarks (GDK-1547). LastSessionEnd runs on
// every bootstrap and every delta/304 response (internal/server read.go
// setSessionBoundary), so its latency is per-request cost on the hot path,
// and the issue's claim is a 300k-visit history making it expensive. The
// query is bounded (newest 2000 person reads, visits_viewed_at index), so
// the honest question is what that bound costs over a 300k-row table —
// these benchmarks are the standing answer; re-run with
//
//	go test ./internal/store/ -run '^$' -bench 'BenchmarkLastSessionEnd' -count=3
//
// Two shapes over the same 300k rows:
//   - dense: one read every 30s for ~104 days. Every step is inside the
//     30-minute gap, so the walk consumes the whole 2000-row bound before
//     returning nil — the worst-case walk.
//   - gapped: the same volume with a >gap break every 50 reads, so the
//     boundary is found ~50 rows in — the shape a real person's history
//     has (sessions), and the typical case.
//
// Both seed source='ui', epoch 0 (the default the schema stamps), which is
// exactly the row population the production predicate selects.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// seedVisitsBench bulk-inserts n person reads ending just before end,
// spaced step apart (or with a >gap break every breakEvery reads when
// breakEvery > 0). Multi-row INSERTs in one transaction: 300k single
// INSERT statements would time the seeder, not the read.
func seedVisitsBench(b *testing.B, db *DB, end time.Time, n int, step time.Duration, breakEvery int) {
	b.Helper()
	ctx := context.Background()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	const batch = 500
	at := end
	for done := 0; done < n; {
		var rows []string
		var args []any
		for i := 0; i < batch && done+i < n; i++ {
			rows = append(rows, "(?,?,?,?)")
			args = append(args, VisitKindIssue, fmt.Sprintf("STD-%d", (done+i)%500+1),
				at.UTC().Format(config.ISOMilli), VisitSourceUI)
			if breakEvery > 0 && (done+i+1)%breakEvery == 0 {
				// A >gap break: the previous read ends a session.
				at = at.Add(-45 * time.Minute)
			} else {
				at = at.Add(-step)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO local.visits (kind, key, viewed_at, source) VALUES `+strings.Join(rows, ","),
			args...); err != nil {
			b.Fatal(err)
		}
		done += len(rows)
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
}

func benchLastSessionEnd(b *testing.B, n int, step time.Duration, breakEvery int) {
	db, err := Open(tempDBPath(b))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	seedVisitsBench(b, db, now.Add(-time.Minute), n, step, breakEvery)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := db.LastSessionEnd(ctxB(), now, 30*time.Minute); err != nil {
			b.Fatal(err)
		}
	}
}

// tempDBPath is openTemp's shape for benchmarks (b.TempDir needs *testing.B).
func tempDBPath(tb testing.TB) string {
	tb.Helper()
	return tb.TempDir() + "/gadak.db"
}

func ctxB() context.Context { return context.Background() }

// BenchmarkLastSessionEndDense300k: 300k reads, no session break — the walk
// consumes the full 2000-row bound.
func BenchmarkLastSessionEndDense300k(b *testing.B) {
	benchLastSessionEnd(b, 300_000, 30*time.Second, 0)
}

// BenchmarkLastSessionEndGapped300k: 300k reads, a session break every 50 —
// the typical person history; the boundary lands ~50 rows in.
func BenchmarkLastSessionEndGapped300k(b *testing.B) {
	benchLastSessionEnd(b, 300_000, 30*time.Second, 50)
}

// BenchmarkLastSessionEndEmpty: the no-history floor (fresh workspace).
func BenchmarkLastSessionEndEmpty(b *testing.B) {
	db, err := Open(tempDBPath(b))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := db.LastSessionEnd(ctxB(), now, 30*time.Minute); err != nil {
			b.Fatal(err)
		}
	}
}
