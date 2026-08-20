package originbind

import (
	"context"
	"database/sql"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// seedStandaloneIssue puts one issue in the standalone mirror under the key a
// real Jira site can also mint. STD is what `init --standalone` seeds, and a
// site's project can be called STD too — that collision is the whole hazard.
func seedStandaloneIssue(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "standalone-jira:10001", SourceID: "jira", Kind: "issue",
				ExternalID: "10001", Key: "STD-1", Title: "filed here",
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "STD", IssueType: "Task", IssueTypeID: "1",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// rawExec writes the tables that have no Go write path in this package's reach
// (enrichments is written by plugins by design; the rest have unexported or
// sync-internal writers). The registered driver attaches local.db, so
// `local.*` is addressable here the same way gadak sql sees it.
func rawExec(t *testing.T, path string, stmts ...string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, s := range stmts {
		if _, err := raw.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func rawCount(t *testing.T, path, query string) int {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var n int
	if err := raw.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// GDK-418. Conversion replaces which origin owns the workspace, and an issue
// key is not globally unique: a kept row that names the old origin's STD-1 does
// not go stale, it silently rebinds to the new site's STD-1 — a different issue.
//
// The tables asserted here are the ones a hand-maintained DELETE list missed.
// enrichments (schemaV2), feed_reads (schemaV4), field_usage (schemaV7) and
// sync_runs (schemaV8) were every one of them added after DropSourceMirror was
// written, and none was added to it; local.recents holds ids the old origin
// minted. That is why the fix is a classification every table must appear in
// rather than four more DELETEs.
func TestConversionLeavesNoRowBoundToTheOldOrigin(t *testing.T) {
	cfg := standaloneHome(t)
	path, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedStandaloneIssue(t, db)
	if err := db.SetWatch(ctx, "STD-1", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFavorite(ctx, "STD-1", true); err != nil {
		t.Fatal(err)
	}
	// A picker-ranking cache of ids the old origin minted: this account id does
	// not exist on the new site, so the ranking offers a stranger or nothing.
	if _, err := db.RecordRecent(ctx, "assignee", "standalone-acct-1"); err != nil {
		t.Fatal(err)
	}
	// History is protected data (data-model.md names it), so this row must
	// survive in the file — it just must stop resolving against a new mirror.
	if _, err := db.RecordVisit(ctx, store.VisitKindIssue, "STD-1"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	rawExec(t, path,
		`INSERT INTO enrichments (key, kind, payload, source, updated_at)
		   VALUES ('STD-1', 'risk', '{"score":9}', 'plugin', '2026-07-01T00:00:00.000Z')`,
		// feed event ids are "cr:" + issue key (internal/store/feed.go), so the
		// new site's STD-1 arrives already marked read.
		`INSERT INTO feed_reads (event_id, read_at) VALUES ('cr:STD-1', '2026-07-01T00:00:00.000Z')`,
		`INSERT INTO field_usage (project_key, alias, filled, total) VALUES ('STD', 'sprint', 3, 4)`,
		`INSERT INTO sync_runs (source_id, kind, started_at, finished_at, fetched, changed, deleted)
		   VALUES ('jira', 'full', '2026-07-01T00:00:00.000Z', '2026-07-01T00:00:01.000Z', 1, 1, 0)`,
	)

	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	reset, err := DropStandaloneProjection(cfg, db)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		what  string
		query string
	}{
		{"enrichments still describe the dropped STD-1", `SELECT count(*) FROM enrichments`},
		{"feed_reads still mark cr:STD-1 read", `SELECT count(*) FROM feed_reads`},
		{"field_usage still reports the dropped STD project", `SELECT count(*) FROM field_usage`},
		{"sync_runs still credit the old origin's sync", `SELECT count(*) FROM sync_runs`},
		{"recents still rank the old origin's account ids", `SELECT count(*) FROM local.recents`},
	} {
		if n := rawCount(t, path, c.query); n != 0 {
			t.Errorf("%s (%d rows): a new origin's same-key issue inherits them", c.what, n)
		}
	}

	// Visit history: hidden from the timeline (it would resolve STD-1 against
	// the new site), still on disk (exportable, per the local.db exception).
	page, err := db.History(ctx, store.HistoryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Errorf("History after conversion = %d items, want 0: the timeline resolves keys against the new mirror", len(page.Items))
	}
	db.Close()
	if n := rawCount(t, path, `SELECT count(*) FROM local.visits`); n != 1 {
		t.Errorf("local.visits = %d rows, want 1: history is protected data, hide it rather than delete it", n)
	}

	// The report is the third layer: what conversion took has to be sayable,
	// or an emptied feed reads as the new site being broken.
	for _, table := range []string{"enrichments", "feed_reads", "field_usage", "sync_runs", "recents", "watches", "favorites"} {
		if reset.Removed[table] == 0 {
			t.Errorf("OriginReset.Removed[%q] = 0, want the row it deleted counted", table)
		}
	}
	if reset.RetiredHistory != 1 {
		t.Errorf("RetiredHistory = %d, want 1", reset.RetiredHistory)
	}
	if reset.OriginEpoch != 1 {
		t.Errorf("OriginEpoch = %d, want 1 (0 is the pre-conversion generation)", reset.OriginEpoch)
	}
	if reset.String() == "" {
		t.Error("OriginReset.String() is empty after a conversion that deleted rows")
	}
}

// A saved view is authored here and no origin has a copy, so conversion must
// not delete one — but `project = STD` silently means the new site's STD after
// the swap, so it must not go unmentioned either (GDK-418, the half that was
// filed as a product judgment).
func TestConversionKeepsSavedViewsAndNamesTheRiskyOnes(t *testing.T) {
	cfg := standaloneHome(t)
	path, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedStandaloneIssue(t, db)
	for _, v := range []store.SavedView{
		{ID: "v1", Name: "std backlog", Config: []byte(`{"jql":"project = STD AND status_category = 'new'"}`)},
		{ID: "v2", Name: "everything open", Config: []byte(`{"jql":"status_category = 'inprogress'"}`)},
		{ID: "v3", Name: "stdio logs", Config: []byte(`{"jql":"summary ~ STDIO"}`)},
	} {
		if err := db.PutSavedView(ctx, v); err != nil {
			t.Fatal(err)
		}
	}

	reset, err := DropStandaloneProjection(cfg, db)
	if err != nil {
		t.Fatal(err)
	}

	views, err := db.SavedViews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 3 {
		t.Fatalf("saved views after conversion = %d, want 3: they are authored here, not mirrored", len(views))
	}
	// STDIO must not match STD: a token match, not a substring one.
	if got := reset.SavedViews; len(got) != 1 || got[0] != "std backlog" {
		t.Errorf("SavedViews naming a retired project = %v, want [std backlog]", got)
	}
	db.Close()
}
