package sync

import (
	"context"
	"errors"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// TestSyncPageDoesNotAdvanceWatermark is the contract that keeps single-page
// resync from jumping the confluence incremental floor into the future.
// UpsertPages may bump version; RecordSync must not run.
func TestSyncPageDoesNotAdvanceWatermark(t *testing.T) {
	f := newConfFixture(t)
	// Seed a later lastModified than the watermark so a mistaken RecordSync
	// would be visible.
	f.pages["1001"].Title = "로그인 런북 (단건 갱신)"
	f.pages["1001"].Version = 9
	f.pages["1001"].When = "2026-12-01T00:00:00.000Z"

	client := f.start()
	db := newMirror(t)
	if err := db.UpsertSource(context.Background(), store.Source{
		ID: ConfluenceSourceID, Kind: "confluence", BaseURL: client.BaseURL(),
	}); err != nil {
		t.Fatal(err)
	}
	const fixedWM = "2026-08-01T10:00:00.000Z"
	if err := db.RecordSync(context.Background(), ConfluenceSourceID, store.SyncResult{
		Watermark: fixedWM, FullSync: true,
	}); err != nil {
		t.Fatal(err)
	}
	before, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Watermark != fixedWM {
		t.Fatalf("precondition watermark %q", before.Watermark)
	}

	cfg := confCfg([]string{"AAA"})
	cfg.Site = client.SiteURL()

	if err := SyncPage(context.Background(), cfg, db.DB, "1001"); err != nil {
		t.Fatalf("SyncPage: %v", err)
	}

	after, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Watermark != fixedWM {
		t.Fatalf("watermark moved %q → %q (RecordSync must not run on single-page resync)",
			fixedWM, after.Watermark)
	}

	// Page content did land.
	d, err := db.PageDetail(context.Background(), "1001")
	if err != nil || d == nil {
		t.Fatalf("PageDetail: %v %#v", err, d)
	}
	if d.Title != "로그인 런북 (단건 갱신)" {
		t.Errorf("title %q", d.Title)
	}
	if d.Version != 9 {
		t.Errorf("version %d, want 9", d.Version)
	}

	// Missing id → ErrNotFound (not a watermark side effect).
	err = SyncPage(context.Background(), cfg, db.DB, "no-such-page")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing page err = %v, want ErrNotFound", err)
	}
	// Watermark still untouched after the miss.
	miss, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if miss.Watermark != fixedWM {
		t.Fatalf("watermark after miss %q", miss.Watermark)
	}
}

// TestSyncPageTombsVanishedKey is FAIL-first for C9: a 404 on the single-page
// path must DeleteItems immediately, not wait for hourly prune.
func TestSyncPageTombsVanishedKey(t *testing.T) {
	f := newConfFixture(t)
	client := f.start()
	db := newMirror(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: ConfluenceSourceID, Kind: "confluence"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertPages(ctx, []store.PageRecord{{
		Item: store.Item{
			ID: "confluence:gone", SourceID: ConfluenceSourceID, Kind: "page",
			ExternalID: "gone", Key: "gone", Title: "deleted upstream",
			CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
		},
		Page: store.Page{SpaceKey: "AAA", Version: 1, Status: "current"},
	}}); err != nil {
		t.Fatal(err)
	}

	cfg := confCfg([]string{"AAA"})
	cfg.Site = client.SiteURL()
	err := SyncPage(ctx, cfg, db.DB, "gone")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("SyncPage(gone) = %v, want ErrNotFound", err)
	}
	var n int
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM items WHERE key = 'gone'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("C9: vanished page still in items (count=%d)", n)
	}
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM deleted_items WHERE key = 'gone'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("C9: tombstone rows = %d, want 1", n)
	}
}

// TestSyncIssueTombsVanishedKey is FAIL-first for C9 on the issue path.
func TestSyncIssueTombsVanishedKey(t *testing.T) {
	site := &fakeSite{t: t, lang: "en", pageSize: 2, failOffset: -1,
		changelog: map[string]string{}, comments: map[string]string{}}
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
				ID: "jira:gone", SourceID: SourceID, Kind: "issue", ExternalID: "gone",
				Key: "NMB-404", Title: "deleted upstream",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	err := SyncIssue(ctx, cfg, db.DB, "NMB-404", Options{Client: client})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("SyncIssue = %v, want ErrNotFound", err)
	}
	var n int
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM items WHERE key = 'NMB-404'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("C9: vanished issue still in items (count=%d)", n)
	}
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM deleted_items WHERE key = 'NMB-404'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("C9: tombstone rows = %d, want 1", n)
	}
}
