package sync

import (
	"context"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

func TestStandaloneSyncFetchesDevLinksWithFlagOff(t *testing.T) {
	// GDK-536: standalone (issuetap) always fetches, even when DevStatus is off.
	site := newSite(t, "en")
	site.devPRs = map[string][]jira.DevPR{
		"10001": {{ID: "pr-9", URL: "https://github.com/o/r/pull/9", Name: "from-origin", Status: "OPEN"}},
	}
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.Kind = config.KindStandalone
	cfg.DevStatus = false

	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	if site.devStatusHits == 0 {
		t.Fatal("standalone sync with DevStatus=false never fetched dev-status")
	}
	d, err := db.DB.Detail(context.Background(), "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.DevLinks) != 1 || d.DevLinks[0].URL != "https://github.com/o/r/pull/9" {
		t.Fatalf("dev_links = %+v, want the origin PR", d.DevLinks)
	}
}

func TestDevStatusFetchErrorPreservesRows(t *testing.T) {
	site := newSite(t, "en")
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.DevStatus = true
	ctx := context.Background()

	if err := db.UpsertSource(ctx, store.Source{ID: SourceID, Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	seed := store.IssueRecord{
		Item: store.Item{
			ID: "jira:10001", SourceID: SourceID, Kind: "issue", ExternalID: "10001",
			Key: "NMB-1", Title: "seeded", CreatedAt: "2026-07-01T00:00:00.000Z",
			UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Issue: store.Issue{
			ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
			Status: "To Do", StatusID: "1", StatusCategory: "new",
		},
		DevLinks: []store.DevLink{{
			Kind: "pullrequest", URL: "https://github.com/o/r/pull/keep", Title: "keep",
		}},
		DevLinksValid: true,
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{Categories: map[string]string{"1": "new", "3": "inprogress", "5": "done"}, Records: []store.IssueRecord{seed}}); err != nil {
		t.Fatal(err)
	}

	site.devStatusFail = true
	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	d, err := db.DB.Detail(ctx, "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range d.DevLinks {
		if l.URL == "https://github.com/o/r/pull/keep" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fetch error drained seeded link: %+v", d.DevLinks)
	}
}

func TestDevStatusSuccessfulEmptyDrains(t *testing.T) {
	site := newSite(t, "en")
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.DevStatus = true
	ctx := context.Background()

	if err := db.UpsertSource(ctx, store.Source{ID: SourceID, Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	seed := store.IssueRecord{
		Item: store.Item{
			ID: "jira:10001", SourceID: SourceID, Kind: "issue", ExternalID: "10001",
			Key: "NMB-1", Title: "seeded", CreatedAt: "2026-07-01T00:00:00.000Z",
			UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Issue: store.Issue{
			ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
			Status: "To Do", StatusID: "1", StatusCategory: "new",
		},
		DevLinks: []store.DevLink{{
			Kind: "pullrequest", URL: "https://github.com/o/r/pull/gone", Title: "gone",
		}},
		DevLinksValid: true,
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{Categories: map[string]string{"1": "new", "3": "inprogress", "5": "done"}, Records: []store.IssueRecord{seed}}); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	d, err := db.DB.Detail(ctx, "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.DevLinks) != 0 {
		t.Fatalf("successful empty must drain, still have %+v", d.DevLinks)
	}
}
