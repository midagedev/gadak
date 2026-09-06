package store

// LastSessionEnd (spec r2-session, Part A) — the session strip's boundary:
// where the previous session of person reads ended. The rule is retro's
// (cmd/gadak/retro.go): person reads are visits with source ui or the empty
// pre-V7 source, and a gap to the previous read *exceeding* 30 minutes starts
// a new session — exactly 30m is still the same session. Clause table:
//
//	C1 ui/'' only, current epoch — TestLastSessionEndSkipsAgentReads
//	     ① a cli visit between two person reads never becomes the boundary
//	     ② a visit recorded under a retired epoch is invisible
//	     ③ visits with only cli sources → nil (no person read at all)
//	C1 excludes the current session's chain — TestLastSessionEndReturnsPreviousSessionEnd
//	     ① two sessions then a current read → the previous session's newest
//	       read (its end, not its start)
//	C1 exact gap stays in the session — TestLastSessionEndExactGapIsSameSession
//	     ① a read exactly gap before the newest is current-session → boundary
//	       is the read before it
//	C1 mid-session reload keeps the boundary — TestLastSessionEndMidSessionReload
//	     ① several reads chained to now (a tab reload's own reads included)
//	       → boundary stays the previous session's last read
//	C1 nil when none — TestLastSessionEndAbsentCases
//	     ① zero visits → nil
//	     ② only a current-session read → nil (one session is not a boundary)
//
// FAIL-first: before the Part A edit this file does not compile —
// db.LastSessionEnd is undefined (run `go test ./internal/store/ -run
// TestLastSessionEnd -count=1` and read the build output;
// failfirst evidence, session scratchpad:
// local-session-prechange.out).

import (
	"context"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// addVisit rows a person/agent read with an explicit stamp. RecordVisit stamps
// Now(), which cannot express "2h ago", so the test writes the row the way the
// V6-migration test does: direct SQL on the attached local.db.
func addVisit(t *testing.T, db *DB, at time.Time, source string) {
	t.Helper()
	_, err := db.sql.ExecContext(context.Background(),
		`INSERT INTO local.visits (kind, key, viewed_at, source) VALUES (?,?,?,?)`,
		VisitKindIssue, "STD-1", at.UTC().Format(config.ISOMilli), source)
	if err != nil {
		t.Fatalf("insert visit at %s: %v", at, err)
	}
}

func TestLastSessionEndReturnsPreviousSessionEnd(t *testing.T) {
	db := openTemp(t)
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	gap := 30 * time.Minute
	// Session 1: two reads 5m apart, three days back.
	addVisit(t, db, now.Add(-72*time.Hour), VisitSourceUI)
	addVisit(t, db, now.Add(-72*time.Hour+5*time.Minute), "")
	// Session 2: one read two hours back.
	prev := now.Add(-2 * time.Hour)
	addVisit(t, db, prev, VisitSourceUI)
	// Current session: one read a minute ago.
	addVisit(t, db, now.Add(-time.Minute), VisitSourceUI)

	end, err := db.LastSessionEnd(context.Background(), now, gap)
	if err != nil {
		t.Fatalf("LastSessionEnd: %v", err)
	}
	if end == nil {
		t.Fatal("LastSessionEnd = nil, want the now-2h read")
	}
	if !end.Equal(prev) {
		t.Fatalf("LastSessionEnd = %v, want %v (the previous session's newest read, not its start)", *end, prev)
	}
}

func TestLastSessionEndSkipsAgentReads(t *testing.T) {
	db := openTemp(t)
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	gap := 30 * time.Minute
	prev := now.Add(-2 * time.Hour)
	addVisit(t, db, now.Add(-72*time.Hour), VisitSourceUI)
	addVisit(t, db, prev, VisitSourceUI)
	// An agent read between the previous session and now: same epoch, same
	// key — only the source separates it. It must never extend or become the
	// boundary.
	addVisit(t, db, now.Add(-time.Hour), VisitSourceCLI)
	addVisit(t, db, now.Add(-time.Minute), VisitSourceUI)

	end, err := db.LastSessionEnd(context.Background(), now, gap)
	if err != nil {
		t.Fatalf("LastSessionEnd: %v", err)
	}
	if end == nil || !end.Equal(prev) {
		t.Fatalf("LastSessionEnd = %v, want %v (the cli read must not count)", end, prev)
	}

	// Retired-epoch person reads are invisible (the GDK-418 clause every
	// visits reader carries).
	if _, err := db.sql.ExecContext(context.Background(),
		`INSERT INTO local.visits (kind, key, viewed_at, source, origin_epoch) VALUES (?,?,?,?,99)`,
		VisitKindIssue, "STD-2", prev.Add(-time.Hour).UTC().Format(config.ISOMilli), VisitSourceUI); err != nil {
		t.Fatalf("insert retired-epoch visit: %v", err)
	}
	// With only a current-session read besides the retired rows, there is no
	// boundary: retired epochs are not the previous session.
	db2 := openTemp(t)
	addVisit(t, db2, prev.Add(-time.Hour), VisitSourceUI)
	if _, err := db2.sql.ExecContext(context.Background(),
		`UPDATE local.visits SET origin_epoch = 1`); err != nil {
		t.Fatalf("retire the only visit: %v", err)
	}
	end2, err := db2.LastSessionEnd(context.Background(), now, gap)
	if err != nil {
		t.Fatalf("LastSessionEnd (retired epoch): %v", err)
	}
	if end2 != nil {
		t.Fatalf("LastSessionEnd = %v with every visit in a retired epoch, want nil", *end2)
	}

	// Only cli reads: no person read, no session, no boundary.
	db3 := openTemp(t)
	addVisit(t, db3, now.Add(-72*time.Hour), VisitSourceCLI)
	addVisit(t, db3, now.Add(-2*time.Hour), VisitSourceCLI)
	addVisit(t, db3, now.Add(-time.Minute), VisitSourceCLI)
	end3, err := db3.LastSessionEnd(context.Background(), now, gap)
	if err != nil {
		t.Fatalf("LastSessionEnd (cli only): %v", err)
	}
	if end3 != nil {
		t.Fatalf("LastSessionEnd = %v with cli-only visits, want nil", *end3)
	}
}

func TestLastSessionEndExactGapIsSameSession(t *testing.T) {
	db := openTemp(t)
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	gap := 30 * time.Minute
	prev := now.Add(-2 * time.Hour)
	addVisit(t, db, prev, VisitSourceUI)
	// Exactly gap before now: retro's rule keeps it in the current session
	// (strictly greater splits), so the boundary is the read before it.
	addVisit(t, db, now.Add(-gap), VisitSourceUI)

	end, err := db.LastSessionEnd(context.Background(), now, gap)
	if err != nil {
		t.Fatalf("LastSessionEnd: %v", err)
	}
	if end == nil || !end.Equal(prev) {
		t.Fatalf("LastSessionEnd = %v, want %v (exact gap is the same session)", end, prev)
	}
}

func TestLastSessionEndMidSessionReload(t *testing.T) {
	db := openTemp(t)
	now := time.Date(2026, 9, 6, 11, 6, 0, 0, time.UTC)
	gap := 30 * time.Minute
	// Session A: 10:00, 10:10, 10:25. Session B (this tab): 11:00 open,
	// 11:05 reload — the reload's own reads are the current chain, and the
	// boundary must stay session A's *last* read (10:25), not move to 11:00.
	a1 := now.Add(-66 * time.Minute)   // 10:00
	a2 := now.Add(-56 * time.Minute)   // 10:10
	aEnd := now.Add(-41 * time.Minute) // 10:25
	addVisit(t, db, a1, VisitSourceUI)
	addVisit(t, db, a2, VisitSourceUI)
	addVisit(t, db, aEnd, VisitSourceUI)
	addVisit(t, db, now.Add(-6*time.Minute), VisitSourceUI) // 11:00
	addVisit(t, db, now.Add(-time.Minute), VisitSourceUI)   // 11:05

	end, err := db.LastSessionEnd(context.Background(), now, gap)
	if err != nil {
		t.Fatalf("LastSessionEnd: %v", err)
	}
	if end == nil || !end.Equal(aEnd) {
		t.Fatalf("LastSessionEnd = %v, want %v (previous session's last read survives the reload)", end, aEnd)
	}
}

func TestLastSessionEndAbsentCases(t *testing.T) {
	gap := 30 * time.Minute
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	db := openTemp(t)
	end, err := db.LastSessionEnd(context.Background(), now, gap)
	if err != nil {
		t.Fatalf("LastSessionEnd (no visits): %v", err)
	}
	if end != nil {
		t.Fatalf("LastSessionEnd = %v with zero visits, want nil", *end)
	}

	db2 := openTemp(t)
	addVisit(t, db2, now.Add(-time.Minute), VisitSourceUI)
	end2, err := db2.LastSessionEnd(context.Background(), now, gap)
	if err != nil {
		t.Fatalf("LastSessionEnd (current session only): %v", err)
	}
	if end2 != nil {
		t.Fatalf("LastSessionEnd = %v with one current-session read, want nil (one session is not a boundary)", *end2)
	}
}
