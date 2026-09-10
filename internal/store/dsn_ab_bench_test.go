package store

// The GDK-978 A/B harness: what a fresh connection + first read costs under
// the two DSN shapes the issue's 2026-08-26 table compared — mirrorDSN as
// shipped (default page cache) and the same DSN with mmap_size(268435456).
// Re-run with
//
//	go test ./internal/store/ -run '^$' -bench 'BenchmarkDSN' -count=3
//
// and compare the medians. Rule of the issue, restated so the mmap row is
// never misread as a proposal: the DSN does not change without regression
// evidence at HEAD, and cache_size is not raised — the 2026-08-26 table's
// cache_size(-65536) row was 2× WORSE, which is the FAIL evidence that
// killed that direction. mmap only helps cold paths (pages not yet in the
// OS cache), and this harness runs on a warm cache like every long-lived
// serve process after its first query — so a small or absent delta here is
// the expected shape, not a falsification of the cold-path row.
//
// Each iteration opens its own *sql.DB (connection init: pragmas, ATTACH,
// identity views) and runs the census query — the GDK-1413 production SQL —
// once, then closes. 50k issues so the database is far larger than the
// default 2MB page cache and the query has real pages to choose between.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// seedCensusBench bulk-inserts n issue rows, two-thirds open, priority
// spread over ranks 0..5, plus their items parents (the pool enforces the
// foreign key — measured in diagnostics_test.go) and the sources row the
// parents reference.
func seedCensusBench(b *testing.B, db *DB, n int) {
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
		end := done + batch
		if end > n {
			end = n
		}
		var items, issues []string
		var itemArgs, issueArgs []any
		for i := done; i < end; i++ {
			id, key := fmt.Sprintf("jira:%d", i), fmt.Sprintf("NMB-%d", i)
			items = append(items, "(?, 'jira', 'issue', ?, ?, ?, '2026-01-01', '2026-01-02', '2026-01-02')")
			itemArgs = append(itemArgs, id, fmt.Sprintf("%d", i), key, fmt.Sprintf("issue %d", i))
			cat := "new"
			switch i % 3 {
			case 1:
				cat = "inprogress"
			case 2:
				cat = "done"
			}
			issues = append(issues, "(?, ?, 'NMB', ?, ?, 0, 0, '{}')")
			issueArgs = append(issueArgs, id, key, cat, i%6)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at) VALUES `+strings.Join(items, ","),
			itemArgs...); err != nil {
			b.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO issues_raw (item_id, key, project_key, status_category, priority_rank, reopen_count, comment_count, raw) VALUES `+strings.Join(issues, ","),
			issueArgs...); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
}

// dsnABSeed opens a fresh 50k-issue mirror and returns its path. Seeded
// once per benchmark; iterations open their own connections to it.
func dsnABSeed(b *testing.B) string {
	b.Helper()
	path := tempDBPath(b)
	db, err := Open(path)
	if err != nil {
		b.Fatal(err)
	}
	seedCensusBench(b, db, 50_000)
	if err := db.Close(); err != nil {
		b.Fatal(err)
	}
	return path
}

// benchDSNFirstQuery times open → census query → close on one connection
// per iteration. sql.Open here goes through the registered driver's ATTACH
// hook like every production connection.
func benchDSNFirstQuery(b *testing.B, dsn func(path string) string) {
	path := dsnABSeed(b)
	ctx := context.Background()
	const q = `SELECT priority_rank, COUNT(*) FROM issues_raw
		WHERE COALESCE(status_category, '') <> 'done'
		GROUP BY priority_rank ORDER BY priority_rank`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sqlDB, err := sql.Open("sqlite", dsn(path))
		if err != nil {
			b.Fatal(err)
		}
		rows, err := sqlDB.QueryContext(ctx, q)
		if err != nil {
			b.Fatal(err)
		}
		for rows.Next() {
		}
		if err := rows.Err(); err != nil {
			b.Fatal(err)
		}
		rows.Close()
		if err := sqlDB.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDSNDefault is mirrorDSN as shipped — the row the decision stands
// on (GDK-978).
func BenchmarkDSNDefault(b *testing.B) { benchDSNFirstQuery(b, mirrorDSN) }

// BenchmarkDSNMmap is the A/B arm: shipped pragmas + mmap_size(268435456).
// See the file comment for why a small delta here changes nothing: this is a
// warm-cache harness, and mmap's measured win was on cold paths.
func BenchmarkDSNMmap(b *testing.B) {
	benchDSNFirstQuery(b, func(path string) string {
		return mirrorDSN(path) + "&_pragma=mmap_size(268435456)"
	})
}
