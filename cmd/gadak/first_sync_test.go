package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// firstSyncDemoHome is sqlDemoHome with the db handle kept, so the test can
// write a live sync_progress row into the copied fixture before running the
// command under test.
func firstSyncDemoHome(t *testing.T) (*store.DB, string) {
	t.Helper()
	sqlDemoHome(t)
	home, ok := os.LookupEnv("GADAK_HOME")
	if !ok {
		t.Fatal("sqlDemoHome left no GADAK_HOME")
	}
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open mirror rw: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, home
}

// TestStatusJSONCarriesFirstSync (GDK-1677): while a first sync is in
// progress, `gadak status --json` carries the first_sync object with phase,
// fetched and total. No confluence in the fixture config → no wiki_pending.
func TestStatusJSONCarriesFirstSync(t *testing.T) {
	db, _ := firstSyncDemoHome(t)
	ctx := context.Background()
	if err := db.BeginSyncProgress(ctx, syncer.SourceID, true); err != nil {
		t.Fatal(err)
	}
	total := 3514
	if err := db.TouchSyncProgress(ctx, syncer.SourceID, 1200, &total); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	var doc struct {
		FirstSync *struct {
			InProgress  bool   `json:"in_progress"`
			Phase       string `json:"phase"`
			Fetched     int    `json:"fetched"`
			Total       *int   `json:"total"`
			WikiPending bool   `json:"wiki_pending"`
			StartedAt   string `json:"started_at"`
		} `json:"first_sync"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.FirstSync == nil {
		t.Fatalf("first_sync missing from:\n%s", out)
	}
	if !doc.FirstSync.InProgress || doc.FirstSync.Phase != "issues" || doc.FirstSync.Fetched != 1200 {
		t.Fatalf("first_sync = %+v", doc.FirstSync)
	}
	if doc.FirstSync.Total == nil || *doc.FirstSync.Total != 3514 {
		t.Errorf("first_sync.total = %v, want 3514", doc.FirstSync.Total)
	}
	if doc.FirstSync.StartedAt == "" {
		t.Error("first_sync.started_at empty")
	}
}

// TestStatusTextCarriesFirstSyncLine: text mode prints the "first sync" row
// between the synced_at rows and the reconcile row — `issues 1,200 / 3,514`.
func TestStatusTextCarriesFirstSyncLine(t *testing.T) {
	db, _ := firstSyncDemoHome(t)
	ctx := context.Background()
	if err := db.BeginSyncProgress(ctx, syncer.SourceID, true); err != nil {
		t.Fatal(err)
	}
	total := 3514
	if err := db.TouchSyncProgress(ctx, syncer.SourceID, 1200, &total); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "first sync         issues 1,200 / 3,514\n") {
		t.Fatalf("missing first sync line:\n%s", out)
	}
}

// TestSQLWarnsFirstSyncInProgress (GDK-1677): every read verb's stale warning
// slot carries the one first-sync line while a first sync is live, and stdout
// stays byte-identical to the run without the row — the warning is stderr-only.
func TestSQLWarnsFirstSyncInProgress(t *testing.T) {
	db, _ := firstSyncDemoHome(t)
	query := []string{"select count(*) from issues"}
	out0, err0, err := captureBoth(t, func() error { return cmdSQL(query) })
	if err != nil {
		t.Fatalf("sql baseline: %v\n%s %s", err, out0, err0)
	}
	if strings.Contains(err0, "first sync") {
		t.Fatalf("baseline already warns:\n%s", err0)
	}

	ctx := context.Background()
	if err := db.BeginSyncProgress(ctx, syncer.SourceID, true); err != nil {
		t.Fatal(err)
	}
	total := 3514
	if err := db.TouchSyncProgress(ctx, syncer.SourceID, 1200, &total); err != nil {
		t.Fatal(err)
	}

	out1, err1, err := captureBoth(t, func() error { return cmdSQL(query) })
	if err != nil {
		t.Fatalf("sql with row: %v\n%s %s", err, out1, err1)
	}
	if out1 != out0 {
		t.Fatalf("stdout changed:\nbefore: %q\nafter:  %q", out0, out1)
	}
	if !strings.Contains(err1, "first sync in progress: 1,200 / 3,514 issues so far") {
		t.Fatalf("stderr missing the first-sync warning:\n%s", err1)
	}
	// Exactly one line — the ordinary stale warnings stand down while the
	// first sync explains the mirror's state.
	if n := strings.Count(err1, "first sync in progress"); n != 1 {
		t.Fatalf("first-sync lines = %d, want 1:\n%s", n, err1)
	}
}

// TestSQLWarnsFirstSyncDocumentsPhase: the wiki pass gets its own sentence,
// and a missing denominator omits the " / total" fragment rather than
// inventing one.
func TestSQLWarnsFirstSyncDocumentsPhase(t *testing.T) {
	db, _ := firstSyncDemoHome(t)
	ctx := context.Background()
	if err := db.BeginSyncProgress(ctx, syncer.ConfluenceSourceID, true); err != nil {
		t.Fatal(err)
	}
	if err := db.TouchSyncProgress(ctx, syncer.ConfluenceSourceID, 462, nil); err != nil {
		t.Fatal(err)
	}

	_, err1, err := captureBoth(t, func() error { return cmdSQL([]string{"select count(*) from issues"}) })
	if err != nil {
		t.Fatalf("sql: %v\n%s", err, err1)
	}
	if !strings.Contains(err1, "first sync in progress: issues done, 462 wiki pages so far — results are partial") {
		t.Fatalf("stderr missing the documents warning:\n%s", err1)
	}
	if strings.Contains(err1, " / ") {
		t.Errorf("no denominator was stored; the line must not carry one:\n%s", err1)
	}
}
