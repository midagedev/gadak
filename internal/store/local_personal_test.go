package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
// across the file boundary. The copy migration is idempotent — it re-runs
// if a crash landed PRAGMA user_version without the copy (the mirror is
// WAL, so a cross-file transaction has no inter-file atomicity to lean on).
// schemaV38 (GDK-824) drops the mirror-side leftovers after the copy; the
// crash-replay order is migrate()'s pending list (the v26 copy lands before
// the v38 drop), and the "local unavailable" hold is pinned by
// TestCopyMigrationWaitsForLocal.
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

	// Post-upgrade the mirror-side leftovers are gone (schemaV38, GDK-824).
	// Re-running schemaV26 by hand was the old idempotence pin; after the
	// drop it would be a local→local self-copy through SQLite's attach name
	// resolution — a dead path. Replay itself lives in migrate()'s pending
	// list: the v26 copy is applied before the v38 drop, so a crash that
	// landed user_version without the copy still re-runs it on next Open.
	var leftover int
	if err := db.sql.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('saved_views','watches','favorites','feed_reads')`).Scan(&leftover); err != nil {
		t.Fatal(err)
	}
	if leftover != 0 {
		t.Errorf("mirror-side personal tables after upgrade = %d, want 0 (dropped by schemaV38)", leftover)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// GDK-1906 (api_usage move) has its own trio below, shaped on the three above.

// TestMirrorDeleteKeepsAPIUsage is the api_usage sibling of
// TestMirrorDeleteKeepsPersonalState: the per-day outbound HTTP counters are
// operational data this process generated — the origin cannot regenerate
// them — so deleting gadak.db alone must cost a re-sync and nothing else.
func TestMirrorDeleteKeepsAPIUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := db.AddAPIUsage(ctx, "2026-09-15", APIUsageDelta{
		Requests: 120, Throttled: 3, ServerErrors: 1, Retries: 2, WaitMS: 900,
		LastThrottledAt: "2026-09-15T09:00:00.000Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

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
	days, err := db2.APIUsage(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 {
		t.Fatalf("api_usage rows after mirror delete = %d, want 1: origin-unregenerable counters must live in local.db, not the disposable mirror", len(days))
	}
	d := days[0]
	if d.Day != "2026-09-15" || d.Requests != 120 || d.Throttled != 3 || d.ServerErrors != 1 || d.Retries != 2 || d.WaitMS != 900 {
		t.Errorf("api_usage after mirror delete = %+v, want the seeded counters", d)
	}
	if d.LastThrottledAt == nil || *d.LastThrottledAt != "2026-09-15T09:00:00.000Z" {
		t.Errorf("last_throttled_at after mirror delete = %v, want the seeded value", d.LastThrottledAt)
	}
}

// TestUpgradeCopiesMirrorAPIUsageToLocal is the v53 sibling of
// TestUpgradeCopiesMirrorPersonalToLocal: an upgrade from the last pre-copy
// level carries the counters across the file boundary, and re-executing the
// copy statement is idempotent — the crash window the mirror's WAL cannot
// close across files lands user_version without the copy, so the statement
// has to be safe to run again.
func TestUpgradeCopiesMirrorAPIUsageToLocal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	prev := apiUsageCopyVersion - 1 // v52: last level before the copy
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
	if _, err := raw.Exec(`INSERT INTO api_usage (day, requests, throttled, server_errors, retries, wait_ms, last_throttled_at)
		VALUES ('2026-09-14', 40, 1, 0, 1, 200, '2026-09-14T08:00:00.000Z'),
		       ('2026-09-15', 80, 2, 1, 2, 400, '2026-09-15T09:00:00.000Z')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = ` + strconv.Itoa(prev)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v52: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	days, err := db.APIUsage(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 {
		t.Fatalf("local.api_usage rows after upgrade = %d, want 2 (copy must carry both days)", len(days))
	}
	if days[0].Day != "2026-09-15" || days[0].Requests != 80 || days[0].Throttled != 2 {
		t.Errorf("newest day after upgrade = %+v, want 2026-09-15/80/2", days[0])
	}

	// Grow a day through the production path, then re-execute the copy by
	// hand: the day-row it would collide with is a newer cumulative counter
	// than the mirror-side snapshot froze at copy time, so INSERT OR IGNORE
	// must leave it alone (the schemaV53 comment's merge rule).
	if err := db.AddAPIUsage(ctx, "2026-09-15", APIUsageDelta{Requests: 20, Throttled: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.ExecContext(ctx, schemaV53); err != nil {
		t.Fatalf("re-executing schemaV53: %v", err)
	}
	days, err = db.APIUsage(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 {
		t.Fatalf("local.api_usage rows after copy re-run = %d, want 2 (idempotent)", len(days))
	}
	if days[0].Requests != 100 || days[0].Throttled != 3 {
		t.Errorf("2026-09-15 after copy re-run = %+v, want requests=100 throttled=3 — the re-run must not reset a counter the production path already grew", days[0])
	}

	// The mirror-side table stays (frozen leftover, no code path reads it);
	// its rows must not have grown either.
	var frozen int
	if err := db.sql.QueryRowContext(ctx, `SELECT count(*) FROM api_usage`).Scan(&frozen); err != nil {
		t.Fatal(err)
	}
	if frozen != 2 {
		t.Errorf("mirror-side api_usage rows = %d, want 2 (frozen at copy time)", frozen)
	}
}

// TestAPIUsageCopyWaitsForLocal is the v53 sibling of
// TestCopyMigrationWaitsForLocal: an unattachable local.db must not refuse
// the mirror — the version holds one below the copy so it re-runs later, and
// once local answers again the counters cross the file boundary.
func TestAPIUsageCopyWaitsForLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gadak.db")
	prev := apiUsageCopyVersion - 1
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
	if _, err := raw.Exec(`INSERT INTO api_usage (day, requests, throttled, server_errors, retries, wait_ms)
		VALUES ('2026-09-15', 60, 2, 0, 1, 300)`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = ` + strconv.Itoa(prev)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	// The raw connection's attach hook created a real local.db while the
	// v52 mirror was being built; replace it with a directory so ATTACH
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
	days, err := db.APIUsage(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].Day != "2026-09-15" || days[0].Requests != 60 {
		t.Errorf("api_usage after local recovered = %+v, want the seeded 2026-09-15/60: the held copy must now have run", days)
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

// TestMemoryLocalMatchesFileLocal pins attachMemoryLocal's schema to
// migrateLocal's. The memory local artifact opens get is built by rewriting
// the same localMigrations slice onto the attached schema (qualifyLocalSQL),
// so the two must agree on tables, columns, indexes and the user_version
// stamp. A future local migration whose shape the rewrite does not recognize
// lands in main or fails the attach — this is where that turns red, before
// any snapshot ships a wrong-schema artifact (GDK-1934).
func TestMemoryLocalMatchesFileLocal(t *testing.T) {
	// File side: a real local.db through the production path.
	dir := t.TempDir()
	mirror := filepath.Join(dir, "gadak.db")
	if err := EnsureLocal(mirror); err != nil {
		t.Fatal(err)
	}
	fileDB, err := sql.Open("sqlite", "file:"+LocalPath(mirror)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer fileDB.Close()

	// Memory side: an in-memory main carrying the artifact flag, so the
	// connection hook builds the memory local on its one pooled connection.
	memDB, err := sql.Open("sqlite", "file::memory:?"+ArtifactDSNParam+"=1")
	if err != nil {
		t.Fatal(err)
	}
	defer memDB.Close()
	memDB.SetMaxOpenConns(1)
	if err := memDB.Ping(); err != nil {
		t.Fatal(err)
	}

	// The rewrite must not leak into main: an artifact's main database is
	// the file being built, and finding local tables there would corrupt it.
	var mainTables int
	if err := memDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&mainTables); err != nil {
		t.Fatal(err)
	}
	if mainTables != 0 {
		t.Fatalf("memory local leaked %d tables into main, want 0", mainTables)
	}

	schemaOf := func(db *sql.DB, prefix string) (tables map[string]string, cols map[string][]string, uv int) {
		tables = map[string]string{}
		cols = map[string][]string{}
		rows, err := db.Query(`SELECT type, name FROM ` + prefix + `sqlite_master WHERE name NOT LIKE 'sqlite_%'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var typ, name string
			if err := rows.Scan(&typ, &name); err != nil {
				t.Fatal(err)
			}
			tables[name] = typ
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		for name := range tables {
			crows, err := db.Query(`SELECT name FROM `+prefix+`pragma_table_info(?)`, name)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for crows.Next() {
				var cn string
				if err := crows.Scan(&cn); err != nil {
					t.Fatal(err)
				}
				names = append(names, cn)
			}
			crows.Close()
			sort.Strings(names)
			cols[name] = names
		}
		if err := db.QueryRow(`PRAGMA ` + prefix + `user_version`).Scan(&uv); err != nil {
			t.Fatal(err)
		}
		return tables, cols, uv
	}

	fileTables, fileCols, fileUV := schemaOf(fileDB, "")
	memTables, memCols, memUV := schemaOf(memDB, "local.")
	if !maps.Equal(fileTables, memTables) {
		t.Errorf("memory local objects = %v, want file local's %v", memTables, fileTables)
	}
	if !maps.EqualFunc(fileCols, memCols, slices.Equal) {
		t.Errorf("memory local columns = %v, want file local's %v", memCols, fileCols)
	}
	if memUV != fileUV || memUV != len(localMigrations) {
		t.Errorf("memory local user_version = %d, want %d (file side %d)", memUV, len(localMigrations), fileUV)
	}
}
