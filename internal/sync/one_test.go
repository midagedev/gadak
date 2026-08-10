package sync

import (
	"context"
	"errors"
	"testing"

	"github.com/midagedev/scry/internal/store"
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
