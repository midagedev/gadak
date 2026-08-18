package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// wantPageVersionCols is the schemaV21 CREATE TABLE column order. A drift here
// is a schema change, not a test to relax.
var wantPageVersionCols = []string{
	"item_id", "number", "created_at", "author_id", "author_name", "message", "minor_edit",
}

func pageVersionColumns(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(), `PRAGMA table_info(page_versions)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(page_versions): %v", err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt *string
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return cols
}

func seedWikiPage(t *testing.T, db *DB, pageID string, version int) {
	t.Helper()
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertPages(ctx, []PageRecord{{
		Item: Item{
			ID: "confluence:" + pageID, SourceID: "confluence", Kind: "page",
			ExternalID: pageID, Key: pageID, Title: "Notes",
			CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z",
		},
		Page: Page{SpaceKey: "AAA", Version: version, Status: "current"},
	}}); err != nil {
		t.Fatal(err)
	}
}

// TestFreshDatabaseMigratesToV21PageVersions is FAIL-first: a new file must
// land at the current migration level with page_versions in the decided shape.
func TestFreshDatabaseMigratesToV21PageVersions(t *testing.T) {
	db := openTemp(t)
	if got := db.SchemaVersion(); got < 21 {
		t.Fatalf("schema version %d, want ≥ 21", got)
	}
	if got := db.SchemaVersion(); got != len(migrations) {
		t.Fatalf("schema version %d, want %d (len(migrations))", got, len(migrations))
	}
	got := pageVersionColumns(t, db)
	if len(got) == 0 {
		t.Fatal("page_versions table missing")
	}
	if joinComma(got) != joinComma(wantPageVersionCols) {
		t.Errorf("page_versions columns\n got: %v\nwant: %v", got, wantPageVersionCols)
	}
	var idx int
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_page_versions_at'`).
		Scan(&idx); err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Errorf("idx_page_versions_at count = %d, want 1", idx)
	}
}

// TestMigrateV20ToV21PreservesPages is FAIL-first: an existing v20 mirror
// (author_id on changelog/attachments; no page_versions) must move forward
// without dropping wiki rows. Spec asked for "V19"; V20 is the real previous
// level — V19 is covered separately.
func TestMigrateV20ToV21PreservesPages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v20.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO sources (id, kind, base_url) VALUES ('confluence', 'confluence', 'https://x');
		INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at)
		VALUES ('confluence:9', 'confluence', 'page', '9', '9', 'Keep me', '2026-01-01', '2026-01-02', '2026-01-02');
		INSERT INTO pages (item_id, space_key, parent_id, version, status, body_adf, labels, excerpt)
		VALUES ('confluence:9', 'AAA', '', 3, 'current', '', '[]', '');
		PRAGMA user_version = 20`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v20: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if got := db.SchemaVersion(); got < 21 {
		t.Fatalf("schema version %d, want ≥ 21", got)
	}

	var title, space string
	var ver int
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT it.title, p.space_key, p.version
		 FROM items it JOIN pages p ON p.item_id = it.id
		 WHERE it.id = 'confluence:9'`).Scan(&title, &space, &ver); err != nil {
		t.Fatalf("page row after v21: %v", err)
	}
	if title != "Keep me" || space != "AAA" || ver != 3 {
		t.Errorf("preserved page = title=%q space=%q ver=%d", title, space, ver)
	}
	got := pageVersionColumns(t, db)
	if joinComma(got) != joinComma(wantPageVersionCols) {
		t.Errorf("page_versions columns after migrate\n got: %v\nwant: %v", got, wantPageVersionCols)
	}
}

// TestMigrateV19ToV21PreservesPages is the spec's "existing V19" wording:
// V19 is no longer the tip (V20 added author_id), but a V19 file must still
// walk V20 then V21 without losing the page.
func TestMigrateV19ToV21PreservesPages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v19.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 19; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO sources (id, kind, base_url) VALUES ('confluence', 'confluence', 'https://x');
		INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at)
		VALUES ('confluence:8', 'confluence', 'page', '8', '8', 'From v19', '2026-01-01', '2026-01-02', '2026-01-02');
		INSERT INTO pages (item_id, space_key, parent_id, version, status, body_adf, labels, excerpt)
		VALUES ('confluence:8', 'BBB', '1', 1, 'current', '', '[]', '');
		PRAGMA user_version = 19`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v19: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if got := db.SchemaVersion(); got < 21 {
		t.Fatalf("schema version %d, want ≥ 21", got)
	}
	var title, parent string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT it.title, p.parent_id FROM items it JOIN pages p ON p.item_id = it.id
		 WHERE it.id = 'confluence:8'`).Scan(&title, &parent); err != nil {
		t.Fatalf("page row after v19→v21: %v", err)
	}
	if title != "From v19" || parent != "1" {
		t.Errorf("preserved v19 page = title=%q parent=%q", title, parent)
	}
}

// TestReplacePageVersionsIdempotent is FAIL-first: the (item_id, number) PK
// must leave one row per version when the same page is collected twice.
func TestReplacePageVersionsIdempotent(t *testing.T) {
	db := openTemp(t)
	seedWikiPage(t, db, "42", 2)
	ctx := context.Background()
	in := []PageVersion{
		{Number: 1, CreatedAt: "2026-01-01T00:00:00.000Z", AuthorID: "acc-a", AuthorName: "Ada", Message: "create"},
		{Number: 2, CreatedAt: "2026-01-02T00:00:00.000Z", AuthorID: "acc-b", AuthorName: "Bob", Message: "typo", MinorEdit: true},
	}
	if err := db.ReplacePageVersions(ctx, "confluence:42", in); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	if err := db.ReplacePageVersions(ctx, "confluence:42", in); err != nil {
		t.Fatalf("second replace: %v", err)
	}
	got, err := db.PageVersions(ctx, "confluence:42")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("rows = %d, want 2 (idempotent)", len(got))
	}
	if got[0].Number != 1 || got[1].Number != 2 {
		t.Errorf("numbers = %d,%d want 1,2", got[0].Number, got[1].Number)
	}
	if got[1].Message != "typo" || !got[1].MinorEdit || got[1].AuthorID != "acc-b" {
		t.Errorf("v2 = %+v", got[1])
	}
	has, err := db.HasPageVersion(ctx, "confluence:42", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("HasPageVersion(42, 2) = false, want true")
	}
	has, err = db.HasPageVersion(ctx, "confluence:42", 3)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("HasPageVersion(42, 3) = true, want false")
	}
}

// TestPageVersionsSortedByNumber feeds a reversed slice and expects number order.
func TestPageVersionsSortedByNumber(t *testing.T) {
	db := openTemp(t)
	seedWikiPage(t, db, "7", 3)
	ctx := context.Background()
	if err := db.ReplacePageVersions(ctx, "confluence:7", []PageVersion{
		{Number: 3, Message: "three"},
		{Number: 1, Message: "one"},
		{Number: 2, Message: "two"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := db.PageVersions(ctx, "confluence:7")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Number != 1 || got[1].Number != 2 || got[2].Number != 3 {
		t.Fatalf("order = %+v, want number 1,2,3", got)
	}
	if got[0].Message != "one" || got[2].Message != "three" {
		t.Errorf("messages shuffled: %+v", got)
	}
}

func joinComma(v []string) string {
	out := ""
	for i, s := range v {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}
