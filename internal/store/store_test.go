package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// documentedColumns is the column list of every table in
// specs/000-product/data-model.md, in document order. The schema is a public
// contract, so a diff here is a contract break, not a test to relax.
var documentedColumns = map[string][]string{
	"sources": {"id", "kind", "base_url", "synced_at"},
	"items": {"id", "source_id", "kind", "external_id", "key", "title", "body_text",
		"author", "author_id", "url", "created_at", "updated_at", "synced_at"},
	"issues": {"item_id", "key", "project_key", "issue_type", "issue_type_id",
		"status", "status_id", "status_category", "priority", "priority_rank",
		"assignee", "assignee_id", "assignee_email", "reporter", "reporter_id", "reporter_email", "parent_key",
		"labels", "components", "fix_versions", "affects_versions", "environment_text",
		"duedate", "resolution", "created_at", "updated_at",
		"status_changed_at", "resolved_at", "reopen_count", "reopened_at",
		"assignee_changed_at", "comment_count", "description_adf", "custom", "raw",
		"reopen_reason", "cloned_from"},
	"comments": {"id", "item_id", "external_id", "author", "author_id",
		"body_adf", "body_text", "created_at", "updated_at"},
	"attachments":   {"id", "item_id", "external_id", "filename", "mime_type", "size", "author", "created_at"},
	"changelog":     {"id", "item_id", "at", "author", "field", "from_value", "from_id", "to_value", "to_id"},
	"links":         {"item_id", "type", "direction", "target_key"},
	"items_fts":     {"title", "body_text", "comments_text"},
	"deleted_items": {"key", "source_id", "deleted_at"},
	"saved_views":   {"id", "name", "config", "created_at", "updated_at"},
	"watches":       {"key", "created_at"},
	"favorites":     {"key", "created_at"},
	"sync_state": {"source_id", "watermark", "version", "last_full_sync_at", "last_error", "schema_version",
		"first_sync_at", "sync_count", "last_notified_at"},
	"enrichments": {"key", "kind", "payload", "source", "updated_at"},
	"feed_reads":  {"event_id", "read_at"},
}

func TestSchemaMatchesDataModel(t *testing.T) {
	db := openTemp(t)
	for table, want := range documentedColumns {
		rows, err := db.sql.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		var got []string
		for rows.Next() {
			var cid int
			var name, typ string
			var notnull int
			var dflt *string
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				rows.Close()
				t.Fatalf("%s: %v", table, err)
			}
			got = append(got, name)
		}
		rows.Close()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s columns\n got: %v\nwant: %v", table, got, want)
		}
	}
}

func TestMigrateForwardIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scry.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := db.SchemaVersion(); got != len(migrations) {
		t.Fatalf("schema version %d, want %d", got, len(migrations))
	}
	// A source row exists so the schema_version mirror has something to update.
	if err := db.UpsertSource(Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync("jira", SyncResult{Watermark: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Reopening an already-migrated database applies nothing and loses nothing.
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st, err := db.SyncState("jira")
	if err != nil {
		t.Fatal(err)
	}
	if st.Watermark != "2026-01-01T00:00:00Z" {
		t.Errorf("watermark %q survived migration as %q", "2026-01-01T00:00:00Z", st.Watermark)
	}
	if st.SchemaVersion != len(migrations) {
		t.Errorf("sync_state.schema_version %d, want %d", st.SchemaVersion, len(migrations))
	}
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scry.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	if _, err := Open(path); err == nil {
		t.Fatal("opened a database written by a newer scry")
	} else if !strings.Contains(err.Error(), "newer") {
		t.Fatalf("error should say the schema is newer, got: %v", err)
	}
}

func TestPragmas(t *testing.T) {
	db := openTemp(t)
	for _, c := range []struct{ pragma, want string }{
		{"journal_mode", "wal"},
		{"busy_timeout", "5000"},
		{"foreign_keys", "1"},
		{"synchronous", "1"}, // NORMAL
	} {
		var got string
		if err := db.sql.QueryRow("PRAGMA " + c.pragma).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.pragma, err)
		}
		if got != c.want {
			t.Errorf("%s = %s, want %s", c.pragma, got, c.want)
		}
	}
}

// A reader must not wait on a writer: agents query this file with sqlite3 while
// `scry sync --watch` is writing to it.
func TestWALReaderNotBlockedByWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scry.db")
	writer := openTemp(t, path)
	seed(t, writer)

	tx, err := writer.sql.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// Takes the write lock and holds it for the rest of the test.
	if _, err := tx.Exec(`INSERT INTO items (id, source_id, kind, key, updated_at)
		VALUES ('jira:999', 'jira', 'issue', 'NMB-999', '2026-08-04T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	done := make(chan error, 1)
	go func() {
		_, err := reader.IssueLites()
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("read during open write transaction: %v", err)
		}
	case <-time.After(2 * time.Second):
		// busy_timeout is 5s, so a blocked reader hangs here rather than erroring.
		t.Fatal("reader blocked by an open write transaction")
	}
}

// exampleQueries are copied verbatim from the "Example queries" section of
// specs/000-product/data-model.md. They are the contract in practice.
var exampleQueries = []struct {
	name    string
	sql     string
	wantMin int
}{
	{"reopened last month", `
		SELECT key, summary, reopen_count, reopened_at
		FROM issues_full
		WHERE reopen_count > 0 AND reopened_at > datetime('now', '-1 month')
		ORDER BY reopen_count DESC, reopened_at DESC`, 1},
	{"full text across bodies and comments", `
		SELECT i.key, it.title
		FROM items_fts f
		JOIN items it ON it.rowid = f.rowid
		JOIN issues i ON i.item_id = it.id
		WHERE items_fts MATCH 'idempotency AND retry'
		LIMIT 20`, 1},
	{"open work per assignee", `
		SELECT COALESCE(assignee, '(unassigned)') AS who, COUNT(*) AS open
		FROM issues
		WHERE project_key = 'NMB' AND status_category != 'done'
		GROUP BY who ORDER BY open DESC`, 2},
	{"time in current status", `
		SELECT key, status, ROUND(julianday('now') - julianday(status_changed_at), 1) AS days
		FROM issues
		WHERE status_category = 'inprogress'
		ORDER BY days DESC LIMIT 20`, 1},
}

func TestDocumentedExampleQueries(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	for _, q := range exampleQueries {
		rows, err := db.sql.Query(q.sql)
		if err != nil {
			t.Errorf("%s: %v", q.name, err)
			continue
		}
		n := 0
		for rows.Next() {
			n++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Errorf("%s: %v", q.name, err)
			continue
		}
		if n < q.wantMin {
			t.Errorf("%s returned %d rows, want at least %d", q.name, n, q.wantMin)
		}
	}
}

func TestIssuesFullView(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	var key, summary string
	err := db.sql.QueryRow(`SELECT key, summary FROM issues_full WHERE key = 'NMB-1'`).Scan(&key, &summary)
	if err != nil {
		t.Fatal(err)
	}
	if summary == "" {
		t.Fatal("issues_full.summary empty — view must expose items.title")
	}
	if summary != "Duplicate charges on card retry" {
		t.Errorf("summary %q", summary)
	}
	// No join required: agents get the title from one table-shaped path.
	rows, err := db.sql.Query(`SELECT key, summary FROM issues_full WHERE status_category != 'done' LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if n == 0 {
		t.Fatal("issues_full returned no open issues")
	}
}

func TestSyncCountAndFirstSyncAt(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	// Failed sync must not bump.
	if err := db.RecordSync("jira", SyncResult{Err: fmt.Errorf("network down")}); err != nil {
		t.Fatal(err)
	}
	st, err := db.SyncState("jira")
	if err != nil {
		t.Fatal(err)
	}
	if st.SyncCount != 0 || st.FirstSyncAt != nil {
		t.Fatalf("failed sync advanced counters: count=%d first=%v", st.SyncCount, st.FirstSyncAt)
	}
	if err := db.RecordSync("jira", SyncResult{Watermark: "2026-08-01T00:00:00Z", FullSync: true}); err != nil {
		t.Fatal(err)
	}
	st, _ = db.SyncState("jira")
	if st.SyncCount != 1 || st.FirstSyncAt == nil {
		t.Fatalf("first success: count=%d first=%v", st.SyncCount, st.FirstSyncAt)
	}
	first := *st.FirstSyncAt
	if err := db.RecordSync("jira", SyncResult{Watermark: "2026-08-02T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	st, _ = db.SyncState("jira")
	if st.SyncCount != 2 {
		t.Errorf("sync_count %d, want 2", st.SyncCount)
	}
	if st.FirstSyncAt == nil || *st.FirstSyncAt != first {
		t.Errorf("first_sync_at changed: %v → %v", first, st.FirstSyncAt)
	}
}

func TestLastNotifiedAtIndependentOfFeedReads(t *testing.T) {
	db := openTemp(t)
	if err := db.SetLastNotifiedAt("jira", "2026-08-04T12:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	st, err := db.SyncState("jira")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastNotifiedAt == nil || *st.LastNotifiedAt != "2026-08-04T12:00:00.000Z" {
		t.Errorf("last_notified_at %v", st.LastNotifiedAt)
	}
	var n int
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM feed_reads`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("SetLastNotifiedAt must not touch feed_reads, got %d rows", n)
	}
}

// The '(unassigned)' COALESCE in the documented query only works if absent text
// is NULL rather than an empty string.
func TestAbsentTextIsNull(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	var who string
	if err := db.sql.QueryRow(
		`SELECT COALESCE(assignee, '(unassigned)') FROM issues WHERE key = 'NMB-3'`).Scan(&who); err != nil {
		t.Fatal(err)
	}
	if who != "(unassigned)" {
		t.Errorf("unassigned issue reported %q", who)
	}
}

func openTemp(t *testing.T, path ...string) *DB {
	t.Helper()
	p := filepath.Join(t.TempDir(), "scry.db")
	if len(path) > 0 {
		p = path[0]
	}
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// The first migration that has to apply to an already-shipped database: v1 was
// released before the plugin boundary existed.
func TestMigrateFromV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scry.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(migrations[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sources (id, kind) VALUES ('jira', 'jira'); PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("migrating a v1 database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if db.SchemaVersion() != len(migrations) {
		t.Errorf("schema version %d, want %d", db.SchemaVersion(), len(migrations))
	}
	var kind string
	if err := db.sql.QueryRow(`SELECT kind FROM sources WHERE id = 'jira'`).Scan(&kind); err != nil {
		t.Errorf("v1 row did not survive the migration: %v", err)
	}

	// A plugin writes with raw SQL; the store only reads.
	if _, err := db.sql.Exec(`
		INSERT INTO enrichments (key, kind, payload, source, updated_at)
		VALUES ('NMB-1', 'deploy', '{"state":"prod"}', 'gh-plugin', '2026-08-04T00:00:00Z'),
		       ('NMB-1', 'prs', '[{"number":7}]', 'gh-plugin', '2026-08-04T00:00:00Z'),
		       ('NMB-2', 'deploy', '{"state":"merged"}', 'gh-plugin', '2026-08-04T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	byKind, err := db.EnrichmentsByKind("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if len(byKind) != 2 || string(byKind["NMB-1"]) != `{"state":"prod"}` {
		t.Errorf("EnrichmentsByKind = %v", byKind)
	}
	forKey, err := db.EnrichmentsFor("NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(forKey) != 2 || string(forKey["prs"]) != `[{"number":7}]` {
		t.Errorf("EnrichmentsFor = %v", forKey)
	}
	if empty, err := db.EnrichmentsByKind("qa"); err != nil || len(empty) != 0 {
		t.Errorf("unknown kind = %v (%v)", empty, err)
	}
}
