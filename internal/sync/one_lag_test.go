package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/store"
)

// The write-through refresh used to read as "search miss = deleted": Jira
// Cloud's search index lags a new or edited issue by seconds, so a create's
// refresh failed with "issue not found upstream" (4 of 6 in a batch, 2026-10)
// and an edit's refresh tombstoned a live row. A miss is now confirmed by a
// direct GET /issue/{key}; only its 404 tombstones.

func lagRows(t *testing.T, db *mirror, key string) int {
	t.Helper()
	var n int
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM items WHERE key = ?`, key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func lagTombs(t *testing.T, db *mirror, key string) int {
	t.Helper()
	var n int
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM deleted_items WHERE key = ?`, key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSyncIssueCreateShapeSurvivesSearchLag(t *testing.T) {
	site := newSite(t, "en")
	site.searchLag = map[string]int{"NMB-1": 2}
	client := site.start()
	db := newMirror(t)
	ctx := context.Background()

	err := SyncIssue(ctx, testConfig(), db.DB, "NMB-1", Options{Client: client, searchLagBackoff: []time.Duration{0, 0, 0}})
	if err != nil {
		t.Fatalf("SyncIssue after 2 lagged searches = %v, want nil", err)
	}
	if n := lagRows(t, db, "NMB-1"); n != 1 {
		t.Errorf("rows = %d, want 1", n)
	}
	if n := lagTombs(t, db, "NMB-1"); n != 0 {
		t.Errorf("tombstones = %d, want 0", n)
	}
}

func TestSyncIssueEditShapeKeepsRowOnSearchLag(t *testing.T) {
	site := newSite(t, "en")
	site.searchLag = map[string]int{"NMB-1": 99} // never indexes within the retries
	client := site.start()
	db := newMirror(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: SourceID, Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:10001", SourceID: SourceID, Kind: "issue", ExternalID: "10001",
				Key: "NMB-1", Title: "mirrored earlier",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "NMB", Status: "To Do", StatusID: "1", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	err := SyncIssue(ctx, testConfig(), db.DB, "NMB-1", Options{Client: client, searchLagBackoff: []time.Duration{0, 0, 0}})
	if !errors.Is(err, ErrIndexedLate) {
		t.Fatalf("SyncIssue = %v, want ErrIndexedLate", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("indexed-late must not read as ErrNotFound (server maps that to 404): %v", err)
	}
	if n := lagRows(t, db, "NMB-1"); n != 1 {
		t.Errorf("live row tombstoned by a search miss: rows = %d, want 1", n)
	}
	if n := lagTombs(t, db, "NMB-1"); n != 0 {
		t.Errorf("tombstones = %d, want 0", n)
	}
}

// A true 404 on the direct GET still tombstones and returns ErrNotFound:
// TestSyncIssueTombsVanishedKey (one_test.go) covers it, now through
// fakeSite.issueGet's 404 rather than a search miss alone.
