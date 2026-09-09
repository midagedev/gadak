package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/sync"
)

// firstSyncProgress is the GET sync/progress/ body including first_sync —
// the mirror-owned "a first full sync is running" fact (GDK-1677). Unlike
// activity it survives the process that started the sync, so a CLI-started
// first sync shows in a serve started later.
type firstSyncProgress struct {
	activityProgress
	FirstSync *sync.FirstSyncDoc `json:"first_sync"`
}

// firstSyncHarness is resetJobAndActivity with the db handle kept, because the
// tests here write the progress row directly into the mirror.
func firstSyncHarness(t *testing.T) (http.Handler, *store.DB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	db, cfg := fixture(t)
	cfg.Site, cfg.Email, cfg.Token = "", "", ""
	cfg.Projects = nil
	return New(db, cfg), db
}

func getProgressFirstSync(t *testing.T, h http.Handler) firstSyncProgress {
	t.Helper()
	rec := get(t, h, apiBase+"sync/progress/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("progress: %d %s", rec.Code, rec.Body.String())
	}
	var p firstSyncProgress
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode progress: %v\nbody: %s", err, rec.Body.String())
	}
	return p
}

// TestSyncProgressCarriesFirstSync: the polled document carries first_sync
// while a live first row exists in the mirror (read from the store, not from
// this process's activity slot) and omits it otherwise.
func TestSyncProgressCarriesFirstSync(t *testing.T) {
	h, db := firstSyncHarness(t)
	ctx := context.Background()

	if p := getProgressFirstSync(t, h); p.FirstSync != nil {
		t.Fatalf("first_sync without a row = %+v, want nil", p.FirstSync)
	}

	if err := db.BeginSyncProgress(ctx, sync.SourceID, true); err != nil {
		t.Fatal(err)
	}
	total := 3514
	if err := db.TouchSyncProgress(ctx, sync.SourceID, 1200, &total); err != nil {
		t.Fatal(err)
	}

	p := getProgressFirstSync(t, h)
	if p.FirstSync == nil {
		t.Fatal("first_sync missing while a live first row exists")
	}
	if !p.FirstSync.InProgress || p.FirstSync.Phase != "issues" || p.FirstSync.Fetched != 1200 {
		t.Fatalf("first_sync = %+v", p.FirstSync)
	}
	if p.FirstSync.Total == nil || *p.FirstSync.Total != 3514 {
		t.Errorf("first_sync.total = %v, want 3514", p.FirstSync.Total)
	}
	if p.FirstSync.WikiPending {
		t.Error("wiki_pending set although the fixture configures no confluence")
	}
	// The in-process activity slot stays what it was — no phantom run appears.
	if p.Activity.Running {
		t.Errorf("activity.running = true; the store read must not fabricate in-process activity")
	}

	if err := db.EndSyncProgress(ctx, sync.SourceID); err != nil {
		t.Fatal(err)
	}
	if p := getProgressFirstSync(t, h); p.FirstSync != nil {
		t.Fatalf("first_sync after end = %+v, want nil", p.FirstSync)
	}
}

// TestSyncProgressFirstSyncDocumentsPhase: a live confluence first row reads
// as the documents phase.
func TestSyncProgressFirstSyncDocumentsPhase(t *testing.T) {
	h, db := firstSyncHarness(t)
	ctx := context.Background()

	if err := db.BeginSyncProgress(ctx, sync.ConfluenceSourceID, true); err != nil {
		t.Fatal(err)
	}
	p := getProgressFirstSync(t, h)
	if p.FirstSync == nil || p.FirstSync.Phase != "documents" {
		t.Fatalf("first_sync = %+v, want phase documents", p.FirstSync)
	}
}
