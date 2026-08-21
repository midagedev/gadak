package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// GDK-105: the mirror is a disposable cache, and personal state must not be
// inside it. saved_views, watches, favorites and feed_reads used to live in
// the mirror schema, so `rm gadak.db` — the documented one-line recovery —
// deleted the only copy of views the user had authored. These tests pin the
// moved home: local.db, the sibling file that survives.
//
// FAIL-first, measured 2026-08-20 on dde683c (pre-move source): opening the
// recreated mirror reported
//
//	saved views after mirror delete = [], want 1 view v1/Mine: user-authored views must live in local.db, not the disposable mirror
//	watches after mirror delete = [], want [STD-1]
//	favorites after mirror delete = [], want [STD-1]
//	feed read receipt cm:jira:c-1 lost to a mirror delete
//	feed read receipt cr:STD-1 lost to a mirror delete
//	feed read receipts after mirror delete = 0, want >= 2
//
// and the upgrade test failed on `no such table: local.saved_views`.

// seedOneFeedEvent mirrors the minimal shape of seedFeedMirror (feed_test.go):
// one issue assigned to the local user with a comment by someone else, so the
// feed computes exactly one event a read receipt can name.
func seedOneFeedEvent(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:1", SourceID: "jira", ExternalID: "1", Key: "STD-1",
				Title: "survives a mirror delete", Author: "Reporter", AuthorID: "acc-rp",
				CreatedAt: "2026-07-20T00:00:00.000Z", UpdatedAt: "2026-08-04T00:00:00.000Z",
			},
			Issue: Issue{
				ProjectKey: "STD", Status: "To Do", StatusID: "1", StatusCategory: "new",
				Assignee: "Me User", AssigneeID: "acc-me", AssigneeEmail: "me@example.com",
				Reporter: "Reporter", ReporterID: "acc-rp",
			},
			Comments: []Comment{{
				ID: "jira:c-1", ExternalID: "c-1", Author: "Other", AuthorID: "acc-ot",
				BodyText: "please look", CreatedAt: "2026-08-03T10:00:00.000Z",
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// The core gate: deleting gadak.db alone — the recovery data-model.md
// documents — must cost a re-sync and nothing else. Re-open recreates the
// mirror schema from scratch; every personal table must still answer.
func TestMirrorDeleteKeepsPersonalState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	seedOneFeedEvent(t, db)
	marked, err := db.MarkFeedRead(ctx, MarkFeedReadOpts{All: true, Me: feedMe(), Now: frozenNow})
	if err != nil {
		t.Fatal(err)
	}
	readsBefore, err := db.loadFeedReads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if marked.Updated < 1 || len(readsBefore) < 1 {
		t.Fatalf("seeding: marked %d events, %d receipts — test needs one", marked.Updated, len(readsBefore))
	}
	view := SavedView{ID: "v1", Name: "Mine", Config: json.RawMessage(`{"q":"project = STD"}`)}
	if err := db.PutSavedView(ctx, view); err != nil {
		t.Fatal(err)
	}
	if err := db.SetWatch(ctx, "STD-1", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFavorite(ctx, "STD-1", true); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// The mirror goes; local.db stays (plus any WAL sidecars, which are
	// meaningless without the main file).
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("re-opening after mirror delete: %v", err)
	}
	defer db2.Close()

	views, err := db2.SavedViews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].ID != "v1" || views[0].Name != "Mine" || string(views[0].Config) != `{"q":"project = STD"}` {
		t.Errorf("saved views after mirror delete = %+v, want 1 view v1/Mine: user-authored views must live in local.db, not the disposable mirror", views)
	}
	for name, list := range map[string]func(context.Context) ([]string, error){
		"watches": db2.Watches, "favorites": db2.Favorites,
	} {
		got, err := list(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != "STD-1" {
			t.Errorf("%s after mirror delete = %v, want [STD-1]", name, got)
		}
	}
	readsAfter, err := db2.loadFeedReads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for id := range readsBefore {
		if _, ok := readsAfter[id]; !ok {
			t.Errorf("feed read receipt %s lost to a mirror delete", id)
		}
	}
	if len(readsAfter) < len(readsBefore) {
		t.Errorf("feed read receipts after mirror delete = %d, want >= %d", len(readsAfter), len(readsBefore))
	}
}

// An upgrade from the last released schema must carry the four tables' rows
// across the file boundary, and the copy migration must be idempotent — it
// re-runs if a crash landed PRAGMA user_version without the copy (the mirror
// is WAL, so a cross-file transaction has no inter-file atomicity to lean on;
// that is also why this round does not drop the mirror-side tables).
func TestUpgradeCopiesMirrorPersonalToLocal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	prev := personalStateCopyVersion - 1 // v25: last level before the copy
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations[:prev] {
		if _, err := raw.Exec(m); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`INSERT INTO saved_views (id, name, config, created_at, updated_at) VALUES ('v1','Mine','{"q":"project = STD"}','2026-01-01T00:00:00.000Z','2026-01-02T00:00:00.000Z')`,
		`INSERT INTO watches (key, created_at) VALUES ('STD-1','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO favorites (key, created_at) VALUES ('STD-2','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO feed_reads (event_id, read_at) VALUES ('cm:jira:c-1','2026-01-01T00:00:00.000Z')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = ` + strconv.Itoa(prev)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	countLocal := func(t *testing.T, db *DB, table string) int {
		t.Helper()
		var n int
		if err := db.sql.QueryRowContext(context.Background(),
			`SELECT count(*) FROM local.`+table).Scan(&n); err != nil {
			t.Fatalf("local.%s: %v", table, err)
		}
		return n
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for table, want := range map[string]int{"saved_views": 1, "watches": 1, "favorites": 1, "feed_reads": 1} {
		if n := countLocal(t, db, table); n != want {
			t.Errorf("upgrade copied %d rows into local.%s, want %d", n, table, want)
		}
	}
	views, err := db.SavedViews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Name != "Mine" {
		t.Errorf("SavedViews after upgrade = %+v, want [Mine]", views)
	}

	// schemaV26 is INSERT OR IGNORE so a crash that landed user_version
	// without the copy can re-run. Later migrations after v26 (ALTER ADD
	// COLUMN) are not re-runnable; replaying them by rewinding
	// user_version is not this test's contract.
	if _, err := db.sql.Exec(schemaV26); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int{"saved_views": 1, "watches": 1, "favorites": 1, "feed_reads": 1} {
		if n := countLocal(t, db, table); n != want {
			t.Errorf("after a second copy pass local.%s = %d rows, want %d (INSERT OR IGNORE must keep the migration idempotent)", table, n, want)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// The copy migration must not refuse the mirror when local.db cannot be
// attached (a directory in its place is the shape local_test.go uses): Open
// succeeds, the version holds one below the copy so it re-runs later, and
// once local answers again the rows cross the file boundary.
func TestCopyMigrationWaitsForLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	prev := personalStateCopyVersion - 1
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations[:prev] {
		if _, err := raw.Exec(m); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`INSERT INTO saved_views (id, name, config, created_at) VALUES ('v1','Mine','{}','2026-01-01T00:00:00.000Z')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = ` + strconv.Itoa(prev)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	// The raw connection's attach hook created a real local.db file while the
	// v25 mirror was being built; replace it with a directory so ATTACH
	// cannot succeed.
	if err := os.Remove(LocalPath(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(LocalPath(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open with unattachable local.db must succeed: %v", err)
	}
	if got := db.SchemaVersion(); got != prev {
		t.Errorf("schema version with local unreachable = %d, want %d (hold below the copy so it re-runs)", got, prev)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(LocalPath(path)); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := db.SchemaVersion(); got != len(migrations) {
		t.Errorf("schema version after local recovered = %d, want %d (the copy must run)", got, len(migrations))
	}
	views, err := db.SavedViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Name != "Mine" {
		t.Errorf("SavedViews after local recovered = %+v, want [Mine]: the held copy must now have run", views)
	}
}
