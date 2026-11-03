package sync

import (
	"context"
	"strings"
	"testing"
)

// GDK-2068 (GitHub #115): a full sync is the one moment the mirror's whole shape is known,
// so it is where the planner's statistics (sqlite_stat1) are rebuilt — a
// mirror that never ran ANALYZE plans doctor's one-sided link count
// quadratically. These tests go through Run, the same entry point
// `gadak sync --full` reaches, never a direct store call.

// statRowsFor returns how many sqlite_stat1 rows a table holds.
func statRowsFor(t *testing.T, db *mirror, tbl string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_stat1 WHERE tbl = ?`, tbl).Scan(&n); err != nil {
		t.Fatalf("sqlite_stat1 read after a full sync: %v", err)
	}
	return n
}

// TestFullSyncRefreshesPlannerStats: after a full issue pass through Run,
// the mirror holds statistics for items and links.
func TestFullSyncRefreshesPlannerStats(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)

	var logs []string
	opts := Options{
		Full:   true,
		Client: site.start(),
		Log:    func(line string) { logs = append(logs, line) },
	}
	if _, err := Run(context.Background(), testConfig(), db.DB, opts); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"items", "links"} {
		if n := statRowsFor(t, db, tbl); n == 0 {
			t.Fatalf("sqlite_stat1 has no rows for %s after a full sync — the mirror plans against default estimates (GDK-2068, GitHub #115)", tbl)
		}
	}
	sawLine := false
	for _, l := range logs {
		if strings.Contains(l, "planner stats") {
			sawLine = true
		}
	}
	if !sawLine {
		t.Fatalf("no planner stats line in %d sync log lines — the refresh step is invisible", len(logs))
	}
}

// TestIncrementalSyncSkipsPlannerStats: an incremental pass with nothing
// changed must not re-run the refresh. The log line is the observable: every
// run of the step logs exactly one line (success or failure), so its count
// staying at one across a quiet incremental tick is the step not running.
func TestIncrementalSyncSkipsPlannerStats(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	ctx := context.Background()

	var logs []string
	logf := func(line string) { logs = append(logs, line) }
	client := site.start()
	cfg := testConfig()

	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client, Log: logf}); err != nil {
		t.Fatal(err)
	}
	fullLines := 0
	for _, l := range logs {
		if strings.Contains(l, "planner stats") {
			fullLines++
		}
	}
	if fullLines == 0 {
		t.Fatal("full pass logged no planner stats line — precondition for observing the incremental skip")
	}

	res, err := Run(ctx, cfg, db.DB, Options{Client: client, Log: logf})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full {
		t.Fatal("second run must be incremental (watermark set by the full pass)")
	}
	total := 0
	for _, l := range logs {
		if strings.Contains(l, "planner stats") {
			total++
		}
	}
	if total != fullLines {
		t.Fatalf("planner stats lines went %d → %d across an incremental tick with nothing changed — the refresh ran", fullLines, total)
	}
}
