package store

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// GDK-2068 (GitHub #115): CountOneSidedLinks went quadratic on a mirror without planner
// statistics (gadak never ran ANALYZE), and `gadak doctor` sat silent for
// minutes on a 7.8k-issue mirror. These tests pin the fix from three sides:
// the plan text without statistics, the count's equivalence with the query as
// it stood before the fix, and the wall-clock bound that made this a defect.

// oneSidedLinksSQLBefore2068 is the query exactly as it stood when GitHub #115
// was filed. It stays here so equivalence is tested against the real old
// text, not a paraphrase: on every fixture below, this text and
// CountOneSidedLinks must return the same number — the plan fix may not
// move a result.
const oneSidedLinksSQLBefore2068 = `
		SELECT COUNT(*) FROM links l
		JOIN items o ON o.id = l.item_id
		JOIN items f ON f.source_id = o.source_id AND f.key = l.target_key AND f.kind = 'issue'
		WHERE l.direction IN ('inward','outward')
		  AND NOT EXISTS (
			SELECT 1 FROM links b
			WHERE b.item_id = f.id AND b.type = l.type
			  AND b.target_key = o.key
			  AND b.direction = CASE l.direction WHEN 'inward' THEN 'outward' ELSE 'inward' END)`

// TestCountOneSidedLinksPlanWithoutStats pins the query's plan on the
// statistics-free mirror (GDK-2068, GitHub #115). The contract is the plan text: the join
// must not start from items_kind_updated — that plan reads every issue and
// probes every item of its source for each, which is the quadratic doctor
// hang — and f must be reached through items_source_key once per link row.
// The precondition (no sqlite_stat1) is asserted, not assumed: with
// statistics present SQLite may legitimately reorder, and the defect lived
// exactly in the state real mirrors are in.
func TestCountOneSidedLinksPlanWithoutStats(t *testing.T) {
	db := openLinksTestDB(t)
	upsertLinksFixture(t, db, []IssueRecord{
		linkRecord(1, Link{Type: "Blocks", Direction: "outward", TargetKey: "NMB-2"}),
		linkRecord(2, Link{Type: "Blocks", Direction: "inward", TargetKey: "NMB-1"}),
	})
	requireNoPlannerStats(t, db)

	rows, err := db.sql.QueryContext(context.Background(), `EXPLAIN QUERY PLAN `+oneSidedLinksSQL)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		lines = append(lines, detail)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	plan := strings.Join(lines, "\n")

	if strings.Contains(plan, "SEARCH f USING INDEX items_kind_updated") {
		t.Fatalf("plan reaches f through items_kind_updated — the quadratic shape is back:\n%s", plan)
	}
	if !strings.Contains(plan, "SEARCH f USING INDEX items_source_key") {
		t.Fatalf("plan does not reach f via items_source_key (source_id=?, key=?):\n%s", plan)
	}
	// The drive order, pinned without assuming EXPLAIN row order: links l is
	// the outer loop as a table scan, never probed per foreign row.
	if !strings.Contains(plan, "SCAN l") || strings.Contains(plan, "SEARCH l ") {
		t.Fatalf("plan does not drive from links l — the outer loop must be the link rows:\n%s", plan)
	}
}

// openLinksTestDB opens a throwaway mirror. The whole point of these tests
// is the state gadak's real mirrors are in: never ANALYZEd, no sqlite_stat1.
// Nothing here may run ANALYZE — search_scale_fixture_test.go's ANALYZE is
// why the scale suite never saw this defect.
func openLinksTestDB(tb testing.TB) *DB {
	tb.Helper()
	db, err := Open(filepath.Join(tb.TempDir(), "links-plan.db"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })
	return db
}

// requireNoPlannerStats fails unless the mirror holds no sqlite_stat1 — the
// precondition every plan assertion in this file depends on. With statistics
// present the planner may legitimately choose a different order; the defect
// and the pin are both about the statistics-free mirror every real gadak
// user has.
func requireNoPlannerStats(tb testing.TB, db *DB) {
	tb.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), `SELECT count(*) FROM sqlite_stat1`).Scan(&n); err == nil {
		tb.Fatalf("precondition: mirror holds sqlite_stat1 (%d rows) — the plan contract is the no-statistics state", n)
	}
}

// upsertLinksFixture writes issues with their link rows through the real
// upsert path, so the fixture is shaped like a synced mirror.
func upsertLinksFixture(tb testing.TB, db *DB, recs []IssueRecord) {
	tb.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira"}); err != nil {
		tb.Fatal(err)
	}
	categories := map[string]string{"1": "new"}
	if _, err := db.UpsertIssues(context.Background(), Batch{
		Categories: categories, Priorities: []string{"Medium"}, Records: recs,
	}); err != nil {
		tb.Fatal(err)
	}
}

func linkRecord(i int, links ...Link) IssueRecord {
	key := fmt.Sprintf("NMB-%d", i)
	return IssueRecord{
		Item: Item{
			ID: "jira:" + key, SourceID: "jira", Kind: "issue", ExternalID: key,
			Key: key, Title: "issue " + key,
			CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z",
		},
		Links: links,
	}
}

// countViaBefore2068 runs the pinned pre-fix text on the same mirror.
func countViaBefore2068(tb testing.TB, db *DB) int {
	tb.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), oneSidedLinksSQLBefore2068).Scan(&n); err != nil {
		tb.Fatal(err)
	}
	return n
}

// TestCountOneSidedLinksCounts pins the number's meaning on a hand-built
// fixture: a pair stored on both ends counts nothing, a link whose far end
// holds no counterpart row counts one, a link pointing outside the mirror
// counts nothing (both ends must be mirrored), and the counterpart must
// match the type, not just the keys.
func TestCountOneSidedLinksCounts(t *testing.T) {
	db := openLinksTestDB(t)
	upsertLinksFixture(t, db, []IssueRecord{
		// Mutual pair: NMB-1 outward→NMB-2, NMB-2 inward→NMB-1. Not counted.
		linkRecord(1, Link{Type: "Blocks", Direction: "outward", TargetKey: "NMB-2"}),
		linkRecord(2, Link{Type: "Blocks", Direction: "inward", TargetKey: "NMB-1"}),
		// One-sided: NMB-3 outward→NMB-4, NMB-4 holds nothing. Counted.
		linkRecord(3, Link{Type: "Blocks", Direction: "outward", TargetKey: "NMB-4"}),
		linkRecord(4),
		// Far end outside the mirror: not counted.
		linkRecord(5, Link{Type: "Blocks", Direction: "outward", TargetKey: "NMB-99"}),
		// Counterpart exists but under another type: still one-sided.
		linkRecord(6, Link{Type: "Relates", Direction: "outward", TargetKey: "NMB-1"}),
	})

	n, err := db.CountOneSidedLinks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := 2; n != want {
		t.Fatalf("CountOneSidedLinks = %d, want %d (NMB-3→4 and NMB-6→1 are the one-sided rows)", n, want)
	}
	if old := countViaBefore2068(t, db); old != n {
		t.Fatalf("pre-2068 text = %d, CountOneSidedLinks = %d — the plan fix changed a result", old, n)
	}
}

// TestCountOneSidedLinksMatchesQueryBefore2068 fuzzes the equivalence the
// count tests only sample: a deterministic pseudo-random link graph over 120
// issues with mixed directions, mutual pairs, self-links and out-of-scope
// targets, compared row-for-row between the pinned pre-fix text and
// CountOneSidedLinks.
func TestCountOneSidedLinksMatchesQueryBefore2068(t *testing.T) {
	db := openLinksTestDB(t)
	rng := rand.New(rand.NewSource(2068))
	recs := make([]IssueRecord, 0, 120)
	for i := 1; i <= 120; i++ {
		var links []Link
		for _, j := range []int{rng.Intn(120) + 1, rng.Intn(120) + 1} {
			dir := "outward"
			if rng.Intn(2) == 0 {
				dir = "inward"
			}
			links = append(links, Link{Type: "Blocks", Direction: dir, TargetKey: fmt.Sprintf("NMB-%d", j)})
		}
		recs = append(recs, linkRecord(i, links...))
	}
	// Out-of-scope targets and a self-link, both of which must never count.
	recs = append(recs,
		linkRecord(121, Link{Type: "Blocks", Direction: "outward", TargetKey: "EXT-1"}),
		linkRecord(122, Link{Type: "Blocks", Direction: "outward", TargetKey: "NMB-122"}),
	)
	upsertLinksFixture(t, db, recs)

	n, err := db.CountOneSidedLinks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	old := countViaBefore2068(t, db)
	if n != old {
		t.Fatalf("CountOneSidedLinks = %d, pre-2068 text = %d on the same mirror — the plan fix changed a result", n, old)
	}
	if n == 0 {
		t.Fatal("fixture produced 0 one-sided rows — it is not exercising the count")
	}
}

// seedLinksScaleMirror fills items+links directly (one write transaction):
// 5000 issues, one source, 4000 outward links whose targets are all in the
// mirror, so every link row is one-sided and the expected count is exactly
// the link count. Direct SQL, not UpsertIssues, because this fixture
// measures a query plan over items and links — the upsert pipeline's cost
// would dominate the test without touching what it measures. No ANALYZE
// anywhere: the precondition is the statistics-free mirror real users have.
func seedLinksScaleMirror(tb testing.TB, db *DB, nItems, nLinks int) {
	tb.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira"}); err != nil {
		tb.Fatal(err)
	}
	err := db.write(context.Background(), func(tx *sql.Tx) error {
		stItem, err := tx.PrepareContext(context.Background(),
			`INSERT INTO items (id, source_id, kind, key, title, updated_at) VALUES (?, 'jira', 'issue', ?, ?, '2026-01-01T00:00:00.000Z')`)
		if err != nil {
			return err
		}
		defer stItem.Close()
		for i := 1; i <= nItems; i++ {
			key := fmt.Sprintf("NMB-%d", i)
			if _, err := stItem.ExecContext(context.Background(), "jira:"+key, key, "issue "+key); err != nil {
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
			to := fmt.Sprintf("NMB-%d", ((i*7+3)%nItems)+1) // 7 coprime with 5000: distinct targets
			if _, err := stLink.ExecContext(context.Background(), from, to); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
}

// TestCountOneSidedLinksFastWithoutPlannerStats is the wall-clock contract
// (GDK-2068, GitHub #115): on a statistics-free mirror at the reporter's scale, the count
// answers in seconds, not minutes. Before the plan fix this took ~20 s at
// this fixture size (45.6 s at 7882×5648, measured 2026-11-03 on the bundled
// SQLite 3.53.3); the bound is 2 s, two orders of magnitude above the fixed
// query's ~15 ms, so machine noise cannot flip it.
func TestCountOneSidedLinksFastWithoutPlannerStats(t *testing.T) {
	db := openLinksTestDB(t)
	const nItems, nLinks = 5000, 4000
	seedLinksScaleMirror(t, db, nItems, nLinks)
	requireNoPlannerStats(t, db)

	start := time.Now()
	n, err := db.CountOneSidedLinks(context.Background())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if n != nLinks {
		t.Fatalf("CountOneSidedLinks = %d, want %d — every seeded link row is one-sided (all targets mirrored)", n, nLinks)
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("CountOneSidedLinks took %s on %d issues / %d links without sqlite_stat1 — the quadratic plan is back (GDK-2068, GitHub #115)", elapsed, nItems, nLinks)
	}
}
