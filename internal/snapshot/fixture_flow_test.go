package snapshot

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// The committed demo fixture is the only mirror most people ever open, and
// `gadak retro` reads flow off it: cycle percentiles, the cycle-time scatter,
// the burn-up and the scope lines. Until GDK-1720 every one of those was false
// there — `make demo-fixture` is circular, so each regeneration reproduced the
// original seed's habit of writing an issue's whole status history inside a
// couple of seconds (measured before the fix: cycle_hours max 0.00043, i.e.
// 1.6 seconds, on all 56 samples).
//
// These assertions are about the shipped file, not about Build in the
// abstract, which is why they open examples/demo.db instead of a temp source:
// the defect was only ever visible in the round trip.
func fixtureDB(t *testing.T) *sql.DB {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "examples", "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("demo fixture absent: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDemoFixtureCycleTimesAreRealistic(t *testing.T) {
	db := fixtureDB(t)

	rows, err := db.Query(`
		SELECT cycle_hours FROM issues_raw
		WHERE cycle_hours IS NOT NULL AND reopen_count = 0
		ORDER BY cycle_hours`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var cycles []float64
	for rows.Next() {
		var h float64
		if err := rows.Scan(&h); err != nil {
			t.Fatal(err)
		}
		cycles = append(cycles, h)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cycles) < 20 {
		t.Fatalf("cycle-time sample = %d, want ≥20 for a scatter worth drawing", len(cycles))
	}
	pct := func(p float64) float64 {
		i := int(math.Round(p * float64(len(cycles)-1)))
		return cycles[i]
	}
	p50, p85, max := pct(0.50), pct(0.85), cycles[len(cycles)-1]
	t.Logf("cycle_hours n=%d min=%.2f p50=%.2f p85=%.2f max=%.2f",
		len(cycles), cycles[0], p50, p85, max)
	if p50 <= 24 {
		t.Errorf("cycle p50 = %.4fh, want > 24h (a day)", p50)
	}
	if p85 <= 5*24 {
		t.Errorf("cycle p85 = %.2fh, want > %dh (five days)", p85, 5*24)
	}
	// A tail, but not one dot owning the y-axis.
	if max < 10*24 || max > 60*24 {
		t.Errorf("cycle max = %.2fh (%.1fd), want between 10d and 60d", max, max/24)
	}
}

func TestDemoFixtureStatusHistoryIsOrdered(t *testing.T) {
	db := fixtureDB(t)

	// No issue may resolve before it started, be updated before it was
	// created, or carry a changelog entry outside its own span.
	for _, probe := range []struct{ name, query string }{
		{"resolved_at before started_at",
			`SELECT COUNT(*) FROM issues_raw
			 WHERE started_at IS NOT NULL AND resolved_at IS NOT NULL AND resolved_at < started_at`},
		{"updated_at before created_at",
			`SELECT COUNT(*) FROM items
			 WHERE kind = 'issue' AND updated_at != '' AND updated_at < created_at`},
		{"changelog entry outside issue span",
			`SELECT COUNT(*) FROM changelog c JOIN items i ON i.id = c.item_id
			 WHERE i.kind = 'issue' AND c.at != '' AND c.id NOT LIKE 'sprint:%'
			   AND (c.at < i.created_at OR c.at > i.updated_at)`},
		{"comment outside issue span",
			`SELECT COUNT(*) FROM comments c JOIN items i ON i.id = c.item_id
			 WHERE i.kind = 'issue' AND c.created_at != ''
			   AND (c.created_at < i.created_at OR c.created_at > i.updated_at)`},
	} {
		var n int
		if err := db.QueryRow(probe.query).Scan(&n); err != nil {
			t.Fatalf("%s: %v", probe.name, err)
		}
		if n != 0 {
			t.Errorf("%s: %d rows, want 0", probe.name, n)
		}
	}

	// Consecutive status transitions must be separated by real time, not by
	// the sub-second gaps the pre-GDK-1720 fixture carried. Some ties are
	// legitimate (a resolution row shares its status row's instant), so this
	// measures the median gap between distinct status transitions.
	rows, err := db.Query(`
		SELECT item_id, at FROM changelog WHERE field = 'status' ORDER BY item_id, at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var gaps []float64
	prevItem, prevAt := "", time.Time{}
	for rows.Next() {
		var item, at string
		if err := rows.Scan(&item, &at); err != nil {
			t.Fatal(err)
		}
		ts, ok := parseTime(at)
		if !ok {
			t.Fatalf("unparsable changelog stamp %q", at)
		}
		if item == prevItem {
			if ts.Before(prevAt) {
				t.Errorf("%s: status transitions out of order at %s", item, at)
			}
			gaps = append(gaps, ts.Sub(prevAt).Hours())
		}
		prevItem, prevAt = item, ts
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(gaps) < 20 {
		t.Fatalf("only %d consecutive status transitions in the fixture", len(gaps))
	}
	sort.Float64s(gaps)
	median := gaps[len(gaps)/2]
	t.Logf("status-transition gaps n=%d median=%.2fh max=%.2fh", len(gaps), median, gaps[len(gaps)-1])
	if median <= 4 {
		t.Errorf("median gap between status transitions = %.4fh, want > 4h", median)
	}
}

// The weekly shape of Done entries is what the burn-up and the "closed this
// week" rows read, and other suites assert counts that ride on it. Widening
// each issue's lifetime moves those entries later, so this pins the shape
// rather than the placement: the same total, spread across the window, with no
// week owning a runaway share.
func TestDemoFixtureDoneEntriesStaySpread(t *testing.T) {
	db := fixtureDB(t)

	rows, err := db.Query(`
		SELECT strftime('%Y-%W', at) AS wk, COUNT(*)
		FROM changelog WHERE field = 'status' AND to_value = 'Done' AND at != ''
		GROUP BY wk ORDER BY wk`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	total, weeks := 0, 0
	var counts []int
	for rows.Next() {
		var wk string
		var n int
		if err := rows.Scan(&wk, &n); err != nil {
			t.Fatal(err)
		}
		total += n
		weeks++
		counts = append(counts, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("Done entries total=%d weeks=%d counts=%v", total, weeks, counts)
	if total != 305 {
		t.Errorf("Done changelog entries = %d, want 305 (the fixture's own history, unchanged by spread)", total)
	}
	if weeks < 12 {
		t.Errorf("Done entries land in %d distinct weeks, want ≥12 across a 90-day window", weeks)
	}
	sort.Ints(counts)
	mid := counts[len(counts)/2]
	hi := counts[len(counts)-1]
	if mid < 5 {
		t.Errorf("median weekly Done entries = %d, want ≥5", mid)
	}
	if hi > 3*mid {
		t.Errorf("busiest week %d is more than 3× the median %d — spread is piling up", hi, mid)
	}
}

// The committed local.db is the demo's browsing history (GDK-1720,
// tools/seed-local/seed.py, regenerated by `make demo-fixture` beside the
// mirror). `gadak retro` reads sessions, resume and seen-vs-touched off it, so
// an empty one is the difference between a surface that works and one that
// only renders. It is the same fixture as demo.db and drifts the same way, so
// it is pinned here rather than left to a manual regen.
func TestDemoLocalDBCarriesBrowsingHistory(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "examples", "local.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("local fixture absent: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var visits, uiVisits, issues, keys int
	var first, last string
	if err := db.QueryRow(`
		SELECT COUNT(*), SUM(source = 'ui'), SUM(kind = 'issue'),
		       COUNT(DISTINCT key), MIN(viewed_at), MAX(viewed_at)
		FROM visits WHERE origin_epoch = 0`).
		Scan(&visits, &uiVisits, &issues, &keys, &first, &last); err != nil {
		t.Fatal(err)
	}
	t.Logf("visits=%d ui=%d issue=%d distinct-keys=%d %s … %s", visits, uiVisits, issues, keys, first, last)
	if visits < 40 {
		t.Errorf("local.db carries %d visits, want ≥40 (a month of reading)", visits)
	}
	if uiVisits < visits/2 {
		t.Errorf("only %d of %d visits are 'ui' reads — the retro's session count would fall back to cli", uiVisits, visits)
	}
	if keys < 30 {
		t.Errorf("visits name %d distinct keys, want ≥30", keys)
	}

	// Every key must exist in the mirror the history accompanies: a read of an
	// issue the fixture does not carry is a dangling row the retro cannot join.
	mirror := fixtureDB(t)
	rows, err := db.Query(`SELECT DISTINCT kind, key FROM visits`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind, key string
		if err := rows.Scan(&kind, &key); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := mirror.QueryRow(
			`SELECT COUNT(*) FROM items WHERE kind = ? AND key = ?`, kind, key).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("visits name %s %q, which examples/demo.db does not carry", kind, key)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	// Personal state other than reading stays empty: the e2e reseed depends on
	// a fresh saved-view table (e2e/serve.sh).
	for _, table := range []string{"saved_views", "searches", "recents", "favorites", "watches", "dashboards"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s has %d rows, want 0 — the seed carries reading history only", table, n)
		}
	}
}
