package store

// GDK-1890: deleted_items is keyed (source_id, key). A wiki page id and an
// issue key that spell the same string used to share one row, each write
// overwriting the other's source_id, and the issue delta could not tell a page
// tombstone from an issue's.

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"sort"
	"testing"
)

func seedCollidingKey(t *testing.T, db *DB, key string) {
	t.Helper()
	ctx := context.Background()
	for _, s := range []Source{
		{ID: "confluence", Kind: "confluence", BaseURL: "https://x"},
		{ID: "jira", Kind: "jira", BaseURL: "https://j"},
	} {
		if err := db.UpsertSource(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	if _, err := db.UpsertPages(ctx, []PageRecord{{
		Item: Item{ID: "confluence:" + key, SourceID: "confluence", Kind: "page", ExternalID: key,
			Key: key, Title: "page", CreatedAt: ago(1), UpdatedAt: ago(1)},
		Page: Page{SpaceKey: "AAA", Version: 1, Status: "current", BodyADF: adf},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(ctx, Batch{Force: true, Records: []IssueRecord{{
		Item:  Item{ID: "jira:" + key, SourceID: "jira", ExternalID: key, Key: key, Title: "issue"},
		Issue: Issue{ProjectKey: "NMB", StatusID: "1", StatusCategory: "new"},
	}}}); err != nil {
		t.Fatal(err)
	}
}

func TestTombstonesWithTheSameKeyKeepTheirSources(t *testing.T) {
	db := openTemp(t)
	seedCollidingKey(t, db, "4242")
	ctx := context.Background()
	if _, err := db.DeleteItems(ctx, "confluence", []string{"4242"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DeleteItems(ctx, "jira", []string{"4242"}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT source_id FROM deleted_items WHERE key = '4242'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got, err := scanStrings(rows)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if len(got) != 2 || got[0] != "confluence" || got[1] != "jira" {
		t.Fatalf("tombstone sources for key 4242 = %v, want [confluence jira] (one row per source)", got)
	}
	for scope, want := range map[TombstoneScope]string{TombstoneIssues: "jira", TombstoneWiki: "confluence"} {
		keys, err := db.DeletedKeysSince(ctx, "", scope)
		if err != nil || len(keys) != 1 {
			t.Fatalf("scope %d keys = %v (%v), want exactly the %s tombstone", scope, keys, err, want)
		}
	}
}

// A fresh database (all migrations replayed) and a v54 file migrated up must
// end with the same deleted_items shape, and the migration keeps each row's
// source_id.
func TestSchemaV55DeletedItemsShapeAndRows(t *testing.T) {
	shape := func(raw *sql.DB) string {
		var ddl string
		if err := raw.QueryRow(`SELECT group_concat(sql, ';') FROM (SELECT sql FROM sqlite_master WHERE tbl_name = 'deleted_items' AND sql IS NOT NULL ORDER BY name)`).Scan(&ddl); err != nil {
			t.Fatal(err)
		}
		return ddl
	}
	dir := t.TempDir()
	old := filepath.Join(dir, "old.db")
	// Migrations 26 and 53 write into the attached local.db, so a raw replay
	// cannot stop at 54: take a head file and put the v54 table shape and
	// stamp back, which is exactly what a 0.24 mirror holds.
	head, err := Open(old)
	if err != nil {
		t.Fatal(err)
	}
	head.Close()
	raw, err := sql.Open("sqlite", "file:"+old)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`DROP TABLE deleted_items`,
		`CREATE TABLE deleted_items (key TEXT PRIMARY KEY, source_id TEXT NOT NULL, deleted_at TEXT NOT NULL)`,
		`CREATE INDEX deleted_items_at ON deleted_items(deleted_at)`,
		`INSERT INTO deleted_items (key, source_id, deleted_at) VALUES ('GONE-1','jira','2026-10-01T00:00:00.000Z'), ('77','confluence','2026-10-02T00:00:00.000Z')`,
		`PRAGMA user_version = 54`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	raw.Close()

	db, err := Open(old)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.SchemaVersion() < 55 {
		t.Fatalf("schema version %d after open, want >= 55", db.SchemaVersion())
	}
	var n int
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM deleted_items WHERE (key,source_id) IN (('GONE-1','jira'),('77','confluence'))`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("migrated tombstones with their source_id = %d (%v), want 2", n, err)
	}
	fresh, err := Open(filepath.Join(dir, "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if a, b := shape(db.sql), shape(fresh.sql); a != b {
		t.Fatalf("migrated deleted_items shape differs from a fresh db:\nmigrated: %s\nfresh:    %s", a, b)
	}
	var pk int
	if err := fresh.sql.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('deleted_items') WHERE pk > 0`).Scan(&pk); err != nil || pk != 2 {
		t.Fatalf("deleted_items pk columns = %d (%v), want 2 (source_id, key)", pk, err)
	}
}
