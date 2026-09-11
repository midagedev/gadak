package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestLastSessionEndQueryRidesTheIndex is the GDK-1547 recurrence gate.
// LastSessionEnd runs on every bootstrap and every delta/304 response, so
// its query must ride the visits_viewed_at index (walk the newest entries
// and stop at the 2000-row bound), never degrade into an epoch-index lookup
// with a temp B-tree sort. The 2026-09-10 measurement that opened the
// issue: over a 300k-visit history the query cost 113–120ms per call (a
// fresh workspace answered in 0.12ms). The instrument falsified the first
// hypothesis — the ORDER BY id tiebreak alone was not the culprit; the
// origin_epoch equality (a scalar-subquery comparison the planner treats
// as a narrowing constraint) picks visits_epoch and pays the sort even with
// ORDER BY viewed_at alone — so the fix pins the index (INDEXED BY
// visits_viewed_at) in lastSessionEndSQL.
//
// Attribution: FAIL-first pinned 2026-09-10, round w11-server — against
// the unpinned query (with and without the tiebreak) this test is red (plan
// beginning "SEARCH local.visits USING INDEX visits_epoch" + "USE TEMP
// B-TREE FOR ORDER BY"); both outputs are quoted in the round report. If
// the driver's plan wording changes, this fails loudly with the plan
// printed — re-pin against the new wording rather than deleting the
// assertion.
func TestLastSessionEndQueryRidesTheIndex(t *testing.T) {
	db := openTemp(t)
	// A handful of rows so the planner has statistics to reason about;
	// EXPLAIN QUERY PLAN is a compile-time answer, not a measured one, so
	// the row count does not matter to it.
	addVisit(t, db, time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC), VisitSourceUI)

	// EXPLAIN the production query itself (lastSessionEndSQL is the shared
	// owner) — a hand-copied SQL in this test could drift from the query
	// that actually runs.
	rows, err := db.sql.QueryContext(context.Background(),
		`EXPLAIN QUERY PLAN `+lastSessionEndSQL, lastSessionVisitsBound)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan, "\n")
	t.Logf("plan:\n%s", joined)

	// The gate, both halves. ① the visits step must ride visits_viewed_at —
	// the INDEXED BY pin makes a table scan or an epoch-index plan a plan
	// SQLite refuses rather than silently degrades, but a future edit that
	// drops the pin or renames the index must fail here, not in production.
	// ② no temp sort may stand in the walk's way — that is the
	// scan-and-sort shape the 113ms measurement pinned.
	var visitsSteps int
	for _, line := range plan {
		if !strings.Contains(line, "local.visits") || strings.Contains(line, "local_meta") {
			continue
		}
		visitsSteps++
		if !strings.Contains(line, "USING INDEX visits_viewed_at") {
			t.Errorf("LastSessionEnd visits step does not ride visits_viewed_at (GDK-1547) — a per-request query left the index walk:\n%s", joined)
		}
	}
	if visitsSteps == 0 {
		t.Fatalf("plan mentions no visits step at all — the gate is no longer looking at the query it means to pin:\n%s", joined)
	}
	if strings.Contains(joined, "TEMP B-TREE") {
		t.Errorf("LastSessionEnd plan builds a temp B-tree (GDK-1547) — the scan-and-sort shape, 113–120ms/call on a 300k-visit history:\n%s", joined)
	}
}

// TestSessionBoundaryQueryRidesTheIndex is the table half of the GDK-1547
// gate (GDK-1439): once local.sessions exists, LastSessionEnd's primary
// read is sessionBoundarySQL, so the same per-request budget now depends on
// that query's shape — the boundary must be answered by the epoch-end
// composite index reading at most two rows, not a sort over the table.
// Same stance as the walk gate above: EXPLAIN the production query itself,
// fail loudly on plan-wording drift, re-pin rather than delete.
func TestSessionBoundaryQueryRidesTheIndex(t *testing.T) {
	db := openTemp(t)
	if _, err := db.sql.ExecContext(context.Background(),
		`INSERT INTO local.sessions (started_at, ended_at, first_write_at, visits) VALUES (?,?,?,1)`,
		"2026-09-10T10:00:00.000Z", "2026-09-10T10:05:00.000Z", ""); err != nil {
		t.Fatal(err)
	}

	rows, err := db.sql.QueryContext(context.Background(), `EXPLAIN QUERY PLAN `+sessionBoundarySQL)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan, "\n")
	t.Logf("plan:\n%s", joined)

	var sessionSteps int
	for _, line := range plan {
		if !strings.Contains(line, "local.sessions") || strings.Contains(line, "local_meta") {
			continue
		}
		sessionSteps++
		// COVERING or not, the step must ride the epoch-end composite — a
		// table scan or another index is the per-request cost this gate pins.
		if !strings.Contains(line, "INDEX sessions_epoch_end") {
			t.Errorf("sessionBoundarySQL sessions step does not ride sessions_epoch_end (GDK-1439/1547) — the boundary read left the index:\n%s", joined)
		}
	}
	if sessionSteps == 0 {
		t.Fatalf("plan mentions no sessions step — the gate is no longer looking at sessionBoundarySQL:\n%s", joined)
	}
	if strings.Contains(joined, "TEMP B-TREE") {
		t.Errorf("sessionBoundarySQL plan builds a temp B-tree — the boundary must be two index rows, not a sort:\n%s", joined)
	}
	// The bound itself: the LIMIT is the whole cost argument (at most two
	// rows read however many sessions the history has). A dropped or widened
	// LIMIT is a silent per-request cost change — assert the SQL text.
	if !strings.Contains(sessionBoundarySQL, "LIMIT 2") {
		t.Errorf("sessionBoundarySQL lost its LIMIT 2 bound:\n%s", sessionBoundarySQL)
	}
}
