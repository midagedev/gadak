package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"
)

// GDK-2068 (GitHub #115): OptimizePlannerStats is what a full sync runs so the mirror
// plans against real row counts. The tests assert the observable contract
// (sqlite_stat1 holds rows for items and links), never the pragma used to
// get there.

// TestOptimizePlannerStatsCreatesStat1: a mirror that never ran ANALYZE has
// no sqlite_stat1; one call writes statistics, and a second call is a
// harmless refresh. ANALYZE writes no row for an empty table, so the fixture
// carries both items and link rows before the call.
func TestOptimizePlannerStatsCreatesStat1(t *testing.T) {
	db := openLinksTestDB(t)
	upsertLinksFixture(t, db, []IssueRecord{
		linkRecord(1, Link{Type: "Blocks", Direction: "outward", TargetKey: "NMB-2"}),
		linkRecord(2, Link{Type: "Blocks", Direction: "inward", TargetKey: "NMB-1"}),
		linkRecord(3, Link{Type: "Blocks", Direction: "outward", TargetKey: "NMB-1"}),
	})
	var n int
	if err := db.QueryRowContext(context.Background(), `SELECT count(*) FROM sqlite_stat1`).Scan(&n); err == nil {
		t.Fatalf("precondition: fresh mirror already holds sqlite_stat1 (%d rows)", n)
	}

	if err := db.OptimizePlannerStats(context.Background()); err != nil {
		t.Fatalf("OptimizePlannerStats: %v", err)
	}
	for _, tbl := range []string{"items", "links"} {
		var rows int
		if err := db.QueryRowContext(context.Background(),
			`SELECT count(*) FROM sqlite_stat1 WHERE tbl = ?`, tbl).Scan(&rows); err != nil {
			t.Fatalf("sqlite_stat1 read for %s: %v", tbl, err)
		}
		if rows == 0 {
			t.Fatalf("sqlite_stat1 has no rows for %s — the mirror still plans against default estimates", tbl)
		}
	}

	// Idempotent: a refresh over existing statistics is not an error.
	if err := db.OptimizePlannerStats(context.Background()); err != nil {
		t.Fatalf("second OptimizePlannerStats: %v", err)
	}
}

// TestOptimizePlannerStatsCostAtScale measures what a full pass pays for the
// refresh, on a synthetic mirror at the reporter's size (GitHub #115: 7882
// issues, 5648 links). The raw column carries 2 KB per issue so the ANALYZE
// cost includes scanning issue payloads, the dominant bytes of a real
// mirror. The ms is logged, not asserted — timing gates flake on loaded CI;
// the guarded contract is correctness, above.
func TestOptimizePlannerStatsCostAtScale(t *testing.T) {
	const nItems, nLinks = 8000, 6000
	db := openLinksTestDB(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	raw := strings.Repeat("x", 2048)
	err := db.write(context.Background(), func(tx *sql.Tx) error {
		stItem, err := tx.PrepareContext(context.Background(),
			`INSERT INTO items (id, source_id, kind, key, title, updated_at) VALUES (?, 'jira', 'issue', ?, ?, '2026-01-01T00:00:00.000Z')`)
		if err != nil {
			return err
		}
		defer stItem.Close()
		stRaw, err := tx.PrepareContext(context.Background(),
			`INSERT INTO issues_raw (item_id, key, raw) VALUES (?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stRaw.Close()
		for i := 1; i <= nItems; i++ {
			key := fmt.Sprintf("NMB-%d", i)
			if _, err := stItem.ExecContext(context.Background(), "jira:"+key, key, "issue "+key); err != nil {
				return err
			}
			if _, err := stRaw.ExecContext(context.Background(), "jira:"+key, key, raw); err != nil {
				return err
			}
		}
		stLink, err := tx.PrepareContext(context.Background(),
			`INSERT INTO links (item_id, type, direction, target_key) VALUES (?, 'Blocks', 'outward', ?)`)
		if err != nil {
			return err
		}
		defer stLink.Close()
		for i := 0; i < nLinks; i++ {
			from := fmt.Sprintf("jira:NMB-%d", (i%nItems)+1)
			to := fmt.Sprintf("NMB-%d", ((i*7+3)%nItems)+1)
			if _, err := stLink.ExecContext(context.Background(), from, to); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	requireNoPlannerStats(t, db)

	start := time.Now()
	if err := db.OptimizePlannerStats(context.Background()); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	var items, links int
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM sqlite_stat1 WHERE tbl = 'items'`).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM sqlite_stat1 WHERE tbl = 'links'`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if items == 0 || links == 0 {
		t.Fatalf("stat1 rows after refresh: items=%d links=%d", items, links)
	}
	t.Logf("OptimizePlannerStats on %d items (+2 KB raw each) / %d links: %s", nItems, nLinks, elapsed.Round(time.Millisecond))
}
