package snapshot

import (
	"database/sql"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/store"
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

// The demo's browsing history (GDK-1720, tools/seed-local/seed.py). local.db
// is personal state and is not committed — `make demo-fixture` seeds one
// beside the mirror when it exists and e2e/serve.sh seeds its own — so this
// test does not read a file in examples/: it makes a fresh local.db beside a
// copy of demo.db (the store's own schema), runs the seeder on it, and pins
// what `gadak retro` will then read. Measured before this shape (2026-09-09):
// a stale, empty examples/local.db on a developer tree made the assertion
// scan NULL, and on CI the absent file made the test skip — a gate that was
// red on one machine and silent on the other.
func TestDemoLocalDBCarriesBrowsingHistory(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 absent: %v", err)
	}
	src, err := filepath.Abs(filepath.Join("..", "..", "examples", "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Skipf("demo fixture absent: %v", err)
	}
	seeder, err := filepath.Abs(filepath.Join("..", "..", "tools", "seed-local", "seed.py"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	mirrorPath := filepath.Join(dir, "gadak.db")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mirrorPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureLocal(mirrorPath); err != nil {
		t.Fatal(err)
	}
	path := store.LocalPath(mirrorPath)
	cmd := exec.Command(python, seeder, mirrorPath, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed.py: %v\n%s", err, out)
	} else {
		t.Logf("%s", strings.TrimSpace(string(out)))
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

// GDK-1739: the flow the fixture showed was two products at once — 144 issues
// in progress whose average age was 51 days and climbing 7 days a week, beside
// a closed set whose cycle p50 was 1.1 days. Both came from the same place:
// applySpread placed every issue's created_at evenly across the window with no
// regard for the state it ended in, so an issue still in progress had entered
// progress long ago, while a closed one's In Progress → Done gap was a single
// exponential draw inside a five-day lifetime. And 110 of the 166 closed
// issues carried no In Progress transition at all (GDK-1730 measured 121 on an
// earlier build), so cycle time had only 56 samples to speak from.
//
// These four assertions are the contract the placement is now written to.
// They read the shipped file for the same reason the suite above does: the
// defect only exists in the round trip.

func percentileOf(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Round(p * float64(len(sorted)-1)))
	return sorted[i]
}

func scanFloats(t *testing.T, db *sql.DB, query string) []float64 {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Float64s(out)
	return out
}

// Contract 1 — work in progress is days old, not months. A couple of stale
// ones stay: the aging chart exists to show them.
func TestDemoFixtureWIPAgeIsDays(t *testing.T) {
	db := fixtureDB(t)
	ages := scanFloats(t, db, `
		SELECT (julianday('now') - julianday(started_at))
		FROM issues_raw
		WHERE status_category = 'inprogress' AND started_at IS NOT NULL AND started_at != ''`)
	if len(ages) < 50 {
		t.Fatalf("in-progress sample = %d, want ≥50", len(ages))
	}
	p50, p85, max := percentileOf(ages, 0.50), percentileOf(ages, 0.85), ages[len(ages)-1]
	t.Logf("wip age days n=%d p50=%.1f p85=%.1f max=%.1f", len(ages), p50, p85, max)
	if p50 < 3 || p50 > 10 {
		t.Errorf("wip age p50 = %.1fd, want 3–10d", p50)
	}
	if p85 > 30 {
		t.Errorf("wip age p85 = %.1fd, want ≤30d", p85)
	}
	if max > 60 {
		t.Errorf("oldest wip = %.1fd, want ≤60d", max)
	}
}

// Contract 2 — a closed issue took days of work, not an afternoon.
func TestDemoFixtureCycleBandIsDays(t *testing.T) {
	db := fixtureDB(t)
	cycles := scanFloats(t, db, `
		SELECT cycle_hours / 24.0 FROM issues_raw
		WHERE status_category = 'done' AND cycle_hours IS NOT NULL`)
	if len(cycles) < 100 {
		t.Fatalf("cycle sample = %d, want ≥100", len(cycles))
	}
	p50, p85 := percentileOf(cycles, 0.50), percentileOf(cycles, 0.85)
	t.Logf("cycle days n=%d p50=%.1f p85=%.1f max=%.1f", len(cycles), p50, p85, cycles[len(cycles)-1])
	if p50 < 3 || p50 > 8 {
		t.Errorf("cycle p50 = %.1fd, want 3–8d", p50)
	}
	if p85 < 10 || p85 > 25 {
		t.Errorf("cycle p85 = %.1fd, want 10–25d", p85)
	}
}

// Contract 3 — a closed issue was worked on before it closed. The measure is
// started_at (the first transition into an in-progress category, resolved
// through status_catalog) against resolved_at, never a display name.
func TestDemoFixtureClosedIssuesPassedThroughProgress(t *testing.T) {
	db := fixtureDB(t)
	var total, through int
	if err := db.QueryRow(`
		SELECT COUNT(*),
		       SUM(CASE WHEN started_at IS NOT NULL AND started_at != ''
		                 AND resolved_at IS NOT NULL AND started_at < resolved_at
		                THEN 1 ELSE 0 END)
		FROM issues_raw WHERE status_category = 'done'`).Scan(&total, &through); err != nil {
		t.Fatal(err)
	}
	if total == 0 {
		t.Fatal("no done issues in the fixture")
	}
	ratio := float64(through) / float64(total)
	t.Logf("closed through In Progress: %d/%d = %.0f%%", through, total, ratio*100)
	if ratio < 0.70 {
		t.Errorf("only %.0f%% of closed issues passed through In Progress, want ≥70%%", ratio*100)
	}
}

// Contract 4 — the weekly closed rate keeps the shape it had. Widening cycle
// time moves a resolution earlier or later; it must not empty a week or pile
// a quarter's worth into one.
func TestDemoFixtureWeeklyClosedRateHolds(t *testing.T) {
	db := fixtureDB(t)
	rows, err := db.Query(`
		SELECT strftime('%Y-%W', resolved_at), COUNT(*)
		FROM issues_raw WHERE status_category = 'done' AND resolved_at IS NOT NULL
		GROUP BY 1 ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var counts []int
	total := 0
	for rows.Next() {
		var wk string
		var n int
		if err := rows.Scan(&wk, &n); err != nil {
			t.Fatal(err)
		}
		counts = append(counts, n)
		total += n
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("closed per week weeks=%d total=%d counts=%v", len(counts), total, counts)
	if len(counts) < 12 {
		t.Errorf("closures land in %d weeks, want ≥12 across a 90-day window", len(counts))
	}
	sort.Ints(counts)
	mid := counts[len(counts)/2]
	hi := counts[len(counts)-1]
	if mid < 10 || mid > 30 {
		t.Errorf("median weekly closed = %d, want 10–30", mid)
	}
	if hi > 3*mid {
		t.Errorf("busiest week %d is more than 3× the median %d", hi, mid)
	}
}
