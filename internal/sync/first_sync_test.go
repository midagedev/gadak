package sync

import (
	"context"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// TestFullSyncWritesLiveFirstProgressRow (GDK-1677): a full pass over an
// empty mirror leaves a live first=1 row whose fetched grows with every
// committed page, and the row is gone when the pass returns. The denominator
// is the approximate count the pass already asked for (3 fixture issues).
func TestFullSyncWritesLiveFirstProgressRow(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	ctx := context.Background()

	var pages int
	type snap struct {
		fetched int
		total   *int
		first   bool
	}
	var snaps []snap
	opts := Options{
		Full:   true,
		Client: site.start(),
		Progress: func(fetched, changed int) {
			pages++
			rows, err := db.SyncProgress(ctx)
			if err != nil {
				t.Errorf("SyncProgress mid-pass: %v", err)
				return
			}
			for _, r := range rows {
				if r.SourceID == SourceID {
					snaps = append(snaps, snap{fetched: r.Fetched, total: r.Total, first: r.First})
				}
			}
		},
	}
	if _, err := Run(ctx, testConfig(), db.DB, opts); err != nil {
		t.Fatal(err)
	}
	if pages != 2 {
		t.Fatalf("progress calls = %d, want 2 (3 issues at pageSize 2)", pages)
	}
	if len(snaps) != 2 {
		t.Fatalf("mid-pass snapshots = %d, want 2 — the row must be live while the pass runs", len(snaps))
	}
	if !snaps[0].first || !snaps[1].first {
		t.Errorf("mid-pass first flags = %v/%v, want true (mirror was empty)", snaps[0].first, snaps[1].first)
	}
	if snaps[0].fetched != 2 || snaps[1].fetched != 3 {
		t.Errorf("mid-pass fetched = %d, %d — want 2 then 3, growing per committed page", snaps[0].fetched, snaps[1].fetched)
	}
	for i, s := range snaps {
		if s.total == nil || *s.total != 3 {
			t.Errorf("snapshot %d total = %v, want 3 (approximate count of the fixture)", i, s.total)
		}
	}
	rows, err := db.SyncProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("rows after the pass = %d, want 0 — End must clear on the success path", len(rows))
	}
}

// TestFailedFullSyncStillClearsProgressRow: the End is deferred, so a pass
// that dies mid-way (page two 500s) clears its row too. A crashed process is
// the other half of that contract, covered by the store liveness test.
func TestFailedFullSyncStillClearsProgressRow(t *testing.T) {
	site := newSite(t, "en")
	site.failOffset = 2 // page one commits, page two dies
	db := newMirror(t)
	ctx := context.Background()

	if _, err := Run(ctx, testConfig(), db.DB, Options{Full: true, Client: site.start()}); err == nil {
		t.Fatal("expected the injected 500 to fail the run")
	}
	rows, err := db.SyncProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("rows after a failed pass = %d, want 0 — the defer runs on the error path", len(rows))
	}
}

// TestIncrementalPassWritesNoProgressRow: only full passes own the row. An
// incremental tick must not create one (and not even take the heartbeat write).
func TestIncrementalPassWritesNoProgressRow(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	ctx := context.Background()
	client := site.start()
	cfg := testConfig()

	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	if rows, err := db.SyncProgress(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("rows after full = %v (err %v), want none", rows, err)
	}
	res, err := Run(ctx, cfg, db.DB, Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full {
		t.Fatal("second run must be incremental (watermark set)")
	}
	if rows, err := db.SyncProgress(ctx); err != nil || len(rows) != 0 {
		t.Errorf("rows after incremental = %v (err %v), want none", rows, err)
	}
}

// TestForcedFullOnFilledMirrorIsNotFirst: `gadak sync --full` over a mirror
// that already has a watermark writes first=0, which readers do not surface.
func TestForcedFullOnFilledMirrorIsNotFirst(t *testing.T) {
	site := newSite(t, "en")
	db := newMirror(t)
	ctx := context.Background()
	client := site.start()
	cfg := testConfig()

	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	var sawFirst []bool
	opts := Options{
		Full:   true,
		Client: client,
		Progress: func(fetched, changed int) {
			rows, err := db.SyncProgress(ctx)
			if err != nil {
				t.Errorf("SyncProgress mid-pass: %v", err)
				return
			}
			for _, r := range rows {
				if r.SourceID == SourceID {
					sawFirst = append(sawFirst, r.First)
				}
			}
		},
	}
	if _, err := Run(ctx, cfg, db.DB, opts); err != nil {
		t.Fatal(err)
	}
	if len(sawFirst) == 0 {
		t.Fatal("no live row observed mid-pass on a forced full")
	}
	for _, f := range sawFirst {
		if f {
			t.Errorf("forced full over a filled mirror wrote first=true — a --full resync must not read as a first sync")
		}
	}
	if rows, err := db.SyncProgress(ctx); err != nil || len(rows) != 0 {
		t.Errorf("rows after pass = %v (err %v), want none", rows, err)
	}
}

// TestFirstSyncDocBuildsFromRows is the reader contract (GDK-1677): the doc
// exists only while a first=1 row is live, phase is "documents" for the wiki
// pass, and wiki_pending is set when Confluence is configured but its row is
// not live yet.
func TestFirstSyncDocBuildsFromRows(t *testing.T) {
	db := newMirror(t)
	ctx := context.Background()

	if doc := FirstSync(ctx, db.DB, testConfig()); doc != nil {
		t.Fatalf("doc on an idle mirror = %+v, want nil", doc)
	}

	if err := db.BeginSyncProgress(ctx, SourceID, true); err != nil {
		t.Fatal(err)
	}
	total := 3514
	if err := db.TouchSyncProgress(ctx, SourceID, 1200, &total); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	cfg.Confluence = &config.ConfluenceConfig{}
	doc := FirstSync(ctx, db.DB, cfg)
	if doc == nil {
		t.Fatal("no doc while a first jira row is live")
	}
	if !doc.InProgress || doc.Phase != PhaseIssues || doc.Fetched != 1200 {
		t.Fatalf("doc = %+v", doc)
	}
	if doc.Total == nil || *doc.Total != 3514 {
		t.Errorf("doc.Total = %v, want 3514", doc.Total)
	}
	if !doc.WikiPending {
		t.Error("wiki_pending must be set while confluence is configured and its row is not live")
	}
	if doc.StartedAt == "" {
		t.Error("doc.StartedAt empty")
	}

	// Confluence live: phase flips to documents and wiki_pending drops.
	if err := db.BeginSyncProgress(ctx, ConfluenceSourceID, true); err != nil {
		t.Fatal(err)
	}
	doc = FirstSync(ctx, db.DB, cfg)
	if doc == nil || doc.Phase != PhaseDocuments {
		t.Fatalf("doc with confluence live = %+v, want phase documents", doc)
	}
	if doc.WikiPending {
		t.Error("wiki_pending must not be set once the confluence row is live")
	}

	// Without Confluence configured there is nothing pending.
	if err := db.EndSyncProgress(ctx, ConfluenceSourceID); err != nil {
		t.Fatal(err)
	}
	doc = FirstSync(ctx, db.DB, testConfig())
	if doc == nil || doc.WikiPending {
		t.Fatalf("doc without confluence config = %+v, want no wiki_pending", doc)
	}

	// Non-first rows surface nothing: a --full resync of a filled mirror.
	if err := db.EndSyncProgress(ctx, SourceID); err != nil {
		t.Fatal(err)
	}
	if err := db.BeginSyncProgress(ctx, SourceID, false); err != nil {
		t.Fatal(err)
	}
	if doc := FirstSync(ctx, db.DB, cfg); doc != nil {
		t.Fatalf("doc for a non-first row = %+v, want nil", doc)
	}
}
