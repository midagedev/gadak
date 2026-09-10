package store

import (
	"context"
	"os"
	"strconv"
	"testing"
)

// The GDK-1413 / GDK-307 diagnostics gates.
//
// Concentrated's threshold is FAIL-first pinned (2026-09-10, round
// w11-server): against a source with DominantSharePct = 71 the boundary
// case below ("exactly 70 fires") is red —
//
//	priority census: Concentrated exactly-70: ok=false, want true
//
// — which is the point of a 70% threshold meaning 70%, not "about 70".
// The census SQL is pinned by the fixture the same way: flipping its
// category predicate to `= 'done'` turns TestOpenPriorityDistributionCensus
// red (done rows are not open). Re-pin by editing this file only when the
// threshold or the open definition changes on purpose.

func TestPriorityDistributionConcentrated(t *testing.T) {
	cases := []struct {
		name    string
		dist    PriorityDistribution
		wantOK  bool
		wantRow PriorityCount
		wantPct int
	}{
		{
			// The boundary: 7 of 10 is exactly DominantSharePct and fires.
			name:    "exactly 70 fires",
			dist:    PriorityDistribution{Rows: []PriorityCount{{0, 1}, {1, 7}, {2, 2}}, Open: 10},
			wantOK:  true,
			wantRow: PriorityCount{Rank: 1, Count: 7},
			wantPct: 70,
		},
		{
			name:   "69 does not",
			dist:   PriorityDistribution{Rows: []PriorityCount{{1, 69}, {2, 31}}, Open: 100},
			wantOK: false,
		},
		{
			// Unset is a value like any other: "80% 미설정" is the same
			// zero-information fact as "80%가 rank 1".
			name:    "unset can dominate",
			dist:    PriorityDistribution{Rows: []PriorityCount{{0, 80}, {1, 20}}, Open: 100},
			wantOK:  true,
			wantRow: PriorityCount{Rank: 0, Count: 80},
			wantPct: 80,
		},
		{
			name:   "spread never",
			dist:   PriorityDistribution{Rows: []PriorityCount{{1, 5}, {2, 5}}, Open: 10},
			wantOK: false,
		},
		{
			name:   "empty never",
			dist:   PriorityDistribution{},
			wantOK: false,
		},
	}
	for _, tc := range cases {
		row, pct, ok := tc.dist.Concentrated()
		if ok != tc.wantOK {
			t.Errorf("%s: Concentrated ok=%v, want %v", tc.name, ok, tc.wantOK)
			continue
		}
		if !tc.wantOK {
			continue
		}
		if row != tc.wantRow {
			t.Errorf("%s: Concentrated row=%+v, want %+v", tc.name, row, tc.wantRow)
		}
		if pct != tc.wantPct {
			t.Errorf("%s: Concentrated pct=%d, want %d", tc.name, pct, tc.wantPct)
		}
	}
}

// seedCensusIssue inserts one issues_raw row with its items parent (the
// pool enforces the item_id foreign key — measured as "FOREIGN KEY
// constraint failed (787)"). The category argument takes nil (NULL
// status_category — a row that is certainly not done) or a category string.
func seedCensusIssue(t *testing.T, db *DB, ext string, category any, rank int) {
	t.Helper()
	key := "NMB-" + ext
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO sources (id, kind) VALUES ('jira', 'jira')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`
		INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at)
		VALUES (?, 'jira', 'issue', ?, ?, ?, '2026-01-01', '2026-01-02', '2026-01-02')`,
		"jira:"+ext, ext, key, key); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`
		INSERT INTO issues_raw (item_id, key, project_key, status_category, priority_rank, reopen_count, comment_count, raw)
		VALUES (?, ?, 'NMB', ?, ?, 0, 0, '{}')`,
		"jira:"+ext, key, category, rank); err != nil {
		t.Fatal(err)
	}
}

func TestOpenPriorityDistributionCensus(t *testing.T) {
	db := openTemp(t)
	// Open: 7×rank1 (mixed new/inprogress and one NULL category), 2×rank2,
	// 1×rank0 unset — exactly the 70% boundary. Done: 2×rank1, 1×rank3,
	// excluded wherever they carry the category.
	for i := 0; i < 6; i++ {
		cat := "new"
		if i%2 == 1 {
			cat = "inprogress"
		}
		seedCensusIssue(t, db, strconv.Itoa(i+1), cat, 1)
	}
	seedCensusIssue(t, db, "7", nil, 1) // NULL category: still open
	seedCensusIssue(t, db, "8", "new", 2)
	seedCensusIssue(t, db, "9", "inprogress", 2)
	seedCensusIssue(t, db, "10", "new", 0) // unset priority
	seedCensusIssue(t, db, "11", "done", 1)
	seedCensusIssue(t, db, "12", "done", 1)
	seedCensusIssue(t, db, "13", "done", 3)

	dist, err := db.OpenPriorityDistribution(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantRows := []PriorityCount{{0, 1}, {1, 7}, {2, 2}}
	if len(dist.Rows) != len(wantRows) {
		t.Fatalf("census rows = %+v, want %+v", dist.Rows, wantRows)
	}
	for i, w := range wantRows {
		if dist.Rows[i] != w {
			t.Fatalf("census rows = %+v, want %+v", dist.Rows, wantRows)
		}
	}
	if dist.Open != 10 {
		t.Errorf("census open = %d, want 10", dist.Open)
	}
	row, pct, ok := dist.Concentrated()
	if !ok || row != (PriorityCount{Rank: 1, Count: 7}) || pct != 70 {
		t.Errorf("Concentrated = (%+v, %d, %v), want ({1 7}, 70, true)", row, pct, ok)
	}
}

func TestOpenPriorityDistributionSpreadStaysQuiet(t *testing.T) {
	// The healthy shape: no value reaches the threshold, and doctor stays
	// quiet — the census answering false is as load-bearing as the warning.
	db := openTemp(t)
	for i := 0; i < 5; i++ {
		seedCensusIssue(t, db, strconv.Itoa(100+i), "new", 1)
		seedCensusIssue(t, db, strconv.Itoa(200+i), "inprogress", 2)
	}
	dist, err := db.OpenPriorityDistribution(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := dist.Concentrated(); ok {
		t.Errorf("spread census Concentrated ok=true, want false (rows %+v, open %d)", dist.Rows, dist.Open)
	}
}

func TestMirrorSidecarBytes(t *testing.T) {
	dir := t.TempDir()
	mirror := dir + "/gadak.db"
	// No sidecars at all: the never-opened / checkpointed-clean shape.
	if wal, shm := MirrorSidecarBytes(mirror); wal != 0 || shm != 0 {
		t.Fatalf("absent sidecars = (%d, %d), want (0, 0)", wal, shm)
	}
	if err := os.WriteFile(mirror+"-wal", []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mirror+"-shm", []byte("ab"), 0o600); err != nil {
		t.Fatal(err)
	}
	if wal, shm := MirrorSidecarBytes(mirror); wal != 10 || shm != 2 {
		t.Fatalf("sidecar bytes = (%d, %d), want (10, 2)", wal, shm)
	}
}

func TestMirrorSidecarBytesWhileOpen(t *testing.T) {
	// The GDK-307 shape the helper exists for: while a handle holds the
	// mirror, the -wal sidecar is the write ledger and exists on disk —
	// doctor must be able to see it before its own Open mints one.
	db := openTemp(t)
	seedCensusIssue(t, db, "1", "new", 1)
	if _, err := os.Stat(db.path + "-wal"); err != nil {
		t.Fatalf("open mirror write left no -wal sidecar: %v", err)
	}
	if wal, _ := MirrorSidecarBytes(db.path); wal < 0 {
		t.Fatalf("sidecar wal = %d, want >= 0", wal)
	}
}
