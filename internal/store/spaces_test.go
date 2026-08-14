package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

// TestSchemaV14SpacesTable is FAIL-first for schema v14: spaces table exists
// with (source_id, key) PK and name/kind columns.
func TestSchemaV14SpacesTable(t *testing.T) {
	db := openTemp(t)
	if got := db.SchemaVersion(); got < 14 {
		t.Fatalf("schema version %d, want ≥ 14 (spaces table)", got)
	}
	cols := documentedColumns["spaces"]
	if len(cols) == 0 {
		t.Fatal("documentedColumns[spaces] missing")
	}
	rows, err := db.sql.QueryContext(context.Background(), `PRAGMA table_info(spaces)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(spaces): %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt *string
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if len(got) == 0 {
		t.Fatal("spaces table missing or empty")
	}
	// v14 created the table; v17 added homepage_id; v19 added watermark (append-only).
	want := strings.Join(documentedColumns["spaces"], ",")
	joined := strings.Join(got, ",")
	if joined != want {
		t.Errorf("spaces columns = %q, want %q", joined, want)
	}
	if got := db.SchemaVersion(); got < 19 {
		t.Fatalf("schema version %d, want ≥ 19 (spaces.watermark)", got)
	}
}

// TestUpsertSpacesIdempotent is FAIL-first for UpsertSpaces: insert then
// re-upsert updates name/kind without error or row duplication.
func TestUpsertSpacesIdempotent(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSpaces(context.Background(), "confluence", []SpaceRow{
		{Key: "3dvBrsa61dIo", Name: "Engineering", Kind: "global"},
		{Key: "OPS", Name: "Operations", Kind: "global"},
	}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM spaces WHERE source_id = 'confluence'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("spaces count = %d, want 2", n)
	}

	// Re-upsert same keys with updated name; row count stays 2.
	if err := db.UpsertSpaces(context.Background(), "confluence", []SpaceRow{
		{Key: "3dvBrsa61dIo", Name: "Eng Team", Kind: "global"},
	}); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.sql.QueryRowContext(context.Background(), `SELECT name FROM spaces WHERE source_id = 'confluence' AND key = '3dvBrsa61dIo'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Eng Team" {
		t.Errorf("name after re-upsert = %q, want Eng Team", name)
	}
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM spaces WHERE source_id = 'confluence'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("spaces count after re-upsert = %d, want 2", n)
	}

	// Empty-kind upsert must not wipe kind.
	if err := db.UpsertSpaces(context.Background(), "confluence", []SpaceRow{
		{Key: "3dvBrsa61dIo", Name: "Eng Team", Kind: ""},
	}); err != nil {
		t.Fatal(err)
	}
	var kind string
	if err := db.sql.QueryRowContext(context.Background(), `SELECT kind FROM spaces WHERE source_id = 'confluence' AND key = '3dvBrsa61dIo'`).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "global" {
		t.Errorf("kind wiped by empty upsert = %q, want global", kind)
	}
}

// TestUpsertSpacesKeepsHomepageID is FAIL-first for homepage_id: a listing
// upsert fills it, then a page-hit-style upsert with empty HomepageID must not
// wipe the stored value (same empty-preserves rule as name/kind).
func TestUpsertSpacesKeepsHomepageID(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSpaces(context.Background(), "confluence", []SpaceRow{
		{Key: "ENG", Name: "Engineering", Kind: "global", HomepageID: "4242"},
	}); err != nil {
		t.Fatal(err)
	}
	var hp string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT homepage_id FROM spaces WHERE source_id = 'confluence' AND key = 'ENG'`).Scan(&hp); err != nil {
		t.Fatal(err)
	}
	if hp != "4242" {
		t.Fatalf("homepage_id after listing upsert = %q, want 4242", hp)
	}
	// Page-hit path: name only, empty HomepageID.
	if err := db.UpsertSpaces(context.Background(), "confluence", []SpaceRow{
		{Key: "ENG", Name: "Engineering"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT homepage_id FROM spaces WHERE source_id = 'confluence' AND key = 'ENG'`).Scan(&hp); err != nil {
		t.Fatal(err)
	}
	if hp != "4242" {
		t.Errorf("homepage_id wiped by empty upsert = %q, want 4242", hp)
	}
	// Non-empty HomepageID updates.
	if err := db.UpsertSpaces(context.Background(), "confluence", []SpaceRow{
		{Key: "ENG", Name: "Engineering", Kind: "global", HomepageID: "9999"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT homepage_id FROM spaces WHERE source_id = 'confluence' AND key = 'ENG'`).Scan(&hp); err != nil {
		t.Fatal(err)
	}
	if hp != "9999" {
		t.Errorf("homepage_id after re-upsert = %q, want 9999", hp)
	}
}

// TestPageLiteSpaceNameJoin is FAIL-first for PageLite.space_name via LEFT JOIN
// spaces: named when present, empty string when absent.
func TestPageLiteSpaceNameJoin(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	if _, err := db.UpsertPages(context.Background(), []PageRecord{
		{
			Item: Item{
				ID: "confluence:1", SourceID: "confluence", Kind: "page", ExternalID: "1",
				Key: "1", Title: "Has name", BodyText: "a",
				CreatedAt: ago(1), UpdatedAt: ago(1),
			},
			Page: Page{SpaceKey: "ENG", Version: 1, Status: "current", BodyADF: adf},
		},
		{
			Item: Item{
				ID: "confluence:2", SourceID: "confluence", Kind: "page", ExternalID: "2",
				Key: "2", Title: "No space row", BodyText: "b",
				CreatedAt: ago(1), UpdatedAt: ago(1),
			},
			Page: Page{SpaceKey: "ORPHAN", Version: 1, Status: "current", BodyADF: adf},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSpaces(context.Background(), "confluence", []SpaceRow{
		{Key: "ENG", Name: "Engineering", Kind: "global", HomepageID: "100"},
	}); err != nil {
		t.Fatal(err)
	}

	pages, err := db.PageLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]PageLite{}
	for _, p := range pages {
		byKey[p.Key] = p
	}
	if byKey["1"].SpaceName != "Engineering" {
		t.Errorf("page 1 SpaceName = %q, want Engineering", byKey["1"].SpaceName)
	}
	if byKey["1"].SpaceHomepageID != "100" {
		t.Errorf("page 1 SpaceHomepageID = %q, want 100", byKey["1"].SpaceHomepageID)
	}
	if byKey["1"].SpaceKey != "ENG" {
		t.Errorf("page 1 SpaceKey = %q", byKey["1"].SpaceKey)
	}
	if byKey["2"].SpaceName != "" {
		t.Errorf("page 2 SpaceName = %q, want empty (no spaces row)", byKey["2"].SpaceName)
	}
	if byKey["2"].SpaceHomepageID != "" {
		t.Errorf("page 2 SpaceHomepageID = %q, want empty", byKey["2"].SpaceHomepageID)
	}

	d, err := db.PageDetail(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("PageDetail(1) = nil")
	}
	if d.SpaceName != "Engineering" {
		t.Errorf("PageDetail SpaceName = %q, want Engineering", d.SpaceName)
	}
	if d.SpaceHomepageID != "100" {
		t.Errorf("PageDetail SpaceHomepageID = %q, want 100", d.SpaceHomepageID)
	}
	d2, err := db.PageDetail(context.Background(), "2")
	if err != nil {
		t.Fatal(err)
	}
	if d2 == nil {
		t.Fatal("PageDetail(2) = nil")
	}
	if d2.SpaceName != "" {
		t.Errorf("PageDetail orphan SpaceName = %q, want empty", d2.SpaceName)
	}

	// Search page hits also carry space_name and space_homepage_id.
	res, err := db.Search(context.Background(), "Has", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range res.Pages {
		if p.Key == "1" {
			found = true
			if p.SpaceName != "Engineering" {
				t.Errorf("Search PageLite SpaceName = %q, want Engineering", p.SpaceName)
			}
			if p.SpaceHomepageID != "100" {
				t.Errorf("Search PageLite SpaceHomepageID = %q, want 100", p.SpaceHomepageID)
			}
		}
	}
	if !found {
		t.Error("Search did not return page 1")
	}
}

// TestSchemaV19SpacesWatermarkColumn: a new DB is at v19+ and spaces.watermark exists.
func TestSchemaV19SpacesWatermarkColumn(t *testing.T) {
	db := openTemp(t)
	if got := db.SchemaVersion(); got < 19 {
		t.Fatalf("schema version %d, want ≥ 19", got)
	}
	if got := db.SchemaVersion(); got != len(migrations) {
		t.Fatalf("schema version %d, want %d (len(migrations))", got, len(migrations))
	}
	rows, err := db.sql.QueryContext(context.Background(), `PRAGMA table_info(spaces)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	var nullable bool
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt *string
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "watermark" {
			found = true
			nullable = notnull == 0
		}
	}
	if !found {
		t.Fatal("spaces.watermark column missing")
	}
	if !nullable {
		t.Error("spaces.watermark must be nullable (NULL = not yet backfilled)")
	}
}

// TestMigrateV18ToV19PreservesSpaces: a v18 DB with a spaces row migrates to
// v19, keeps the row, and leaves watermark NULL.
func TestMigrateV18ToV19PreservesSpaces(t *testing.T) {
	path := t.TempDir() + "/gadak.db"
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 18; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO sources (id, kind, base_url) VALUES ('confluence', 'confluence', 'https://x');
		INSERT INTO spaces (source_id, key, name, kind, homepage_id)
		VALUES ('confluence', 'AAA', 'Alpha', 'global', '1000');
		PRAGMA user_version = 18`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("open v18 db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if got := db.SchemaVersion(); got < 19 {
		t.Fatalf("migrated schema version %d, want ≥ 19", got)
	}
	var name, kind, homepage string
	var wm sql.NullString
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT name, kind, homepage_id, watermark FROM spaces WHERE source_id = 'confluence' AND key = 'AAA'`).
		Scan(&name, &kind, &homepage, &wm); err != nil {
		t.Fatalf("space row after v19 migrate: %v", err)
	}
	if name != "Alpha" || kind != "global" || homepage != "1000" {
		t.Errorf("preserved row = name=%q kind=%q homepage=%q", name, kind, homepage)
	}
	if wm.Valid {
		t.Errorf("watermark after migrate = %q, want NULL", wm.String)
	}
}

func seedTwoSpacePages(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira", BaseURL: "https://j"}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	cmADF := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"beta comment"}]}]}`)
	if _, err := db.UpsertPages(ctx, []PageRecord{
		{
			Item: Item{
				ID: "confluence:a1", SourceID: "confluence", Kind: "page", ExternalID: "a1",
				Key: "a1", Title: "Keep me", BodyText: "alphakeep body",
				CreatedAt: ago(1), UpdatedAt: ago(1),
			},
			Page: Page{SpaceKey: "AAA", Version: 1, Status: "current", BodyADF: adf},
		},
		{
			Item: Item{
				ID: "confluence:b1", SourceID: "confluence", Kind: "page", ExternalID: "b1",
				Key: "b1", Title: "Drop me", BodyText: "betadrop body",
				CreatedAt: ago(1), UpdatedAt: ago(1),
			},
			Page: Page{SpaceKey: "BBB", Version: 1, Status: "current", BodyADF: adf},
			Comments: []Comment{{
				ID: "confluence:cb1", ExternalID: "cb1", Author: "Lee",
				BodyADF: cmADF, BodyText: "beta comment",
				CreatedAt: ago(1), UpdatedAt: ago(1),
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSpaces(ctx, "confluence", []SpaceRow{
		{Key: "AAA", Name: "Alpha", Kind: "global"},
		{Key: "BBB", Name: "Beta", Kind: "global"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(ctx, Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "NMB-1", Title: "jira stays", BodyText: "jiraunique body",
				CreatedAt: ago(1), UpdatedAt: ago(1),
			},
			Issue: Issue{ProjectKey: "NMB", Status: "To Do", StatusID: "1", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// TestPruneConfluenceSpacesRemovesOutOfScope: keep AAA → BBB pages/items/FTS/
// comments/spaces vanish; AAA and the Jira issue stay. keepKeys=[] is a no-op.
func TestPruneConfluenceSpacesRemovesOutOfScope(t *testing.T) {
	db := openTemp(t)
	seedTwoSpacePages(t, db)
	ctx := context.Background()

	cursor := Now()
	n, err := db.PruneConfluenceSpaces(ctx, "confluence", []string{"AAA"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pruned = %d, want 1", n)
	}

	var pages, items, comments, spaces int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM pages`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE kind = 'page'`).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM comments WHERE item_id = 'confluence:b1'`).Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM spaces WHERE source_id = 'confluence'`).Scan(&spaces); err != nil {
		t.Fatal(err)
	}
	if pages != 1 || items != 1 {
		t.Fatalf("after prune pages=%d items=%d, want 1/1", pages, items)
	}
	if comments != 0 {
		t.Errorf("BBB comments survived prune: %d", comments)
	}
	if spaces != 1 {
		t.Fatalf("spaces rows = %d, want 1", spaces)
	}
	var keepKey string
	if err := db.sql.QueryRowContext(ctx, `SELECT key FROM spaces WHERE source_id = 'confluence'`).Scan(&keepKey); err != nil {
		t.Fatal(err)
	}
	if keepKey != "AAA" {
		t.Errorf("remaining space = %q, want AAA", keepKey)
	}

	res, err := db.Search(ctx, "betadrop", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pages) != 0 {
		t.Errorf("FTS still finds pruned BBB page: %+v", res.Pages)
	}
	res, err = db.Search(ctx, "alphakeep", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pages) != 1 || res.Pages[0].Key != "a1" {
		t.Errorf("AAA page missing from FTS: %+v", res.Pages)
	}
	res, err = db.Search(ctx, "jiraunique", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 1 || res.Keys[0] != "NMB-1" {
		t.Errorf("jira issue lost in prune: keys=%v", res.Keys)
	}

	gone, err := db.DeletedKeysSince(ctx, cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(gone) != 1 || gone[0] != "b1" {
		t.Errorf("tombstones = %v, want [b1]", gone)
	}

	// keepKeys empty: refuse to delete anything (full-wipe guard).
	db2 := openTemp(t)
	seedTwoSpacePages(t, db2)
	n, err = db2.PruneConfluenceSpaces(ctx, "confluence", nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("empty keepKeys pruned %d, want 0", n)
	}
	var left int
	if err := db2.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM pages`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 2 {
		t.Errorf("empty keepKeys left %d pages, want 2", left)
	}
	if err := db2.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM spaces WHERE source_id = 'confluence'`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 2 {
		t.Errorf("empty keepKeys left %d spaces, want 2", left)
	}
}

// TestSpaceWatermarkRoundTrip: SetSpaceWatermark + ConfluenceSpaceWatermarks.
func TestSpaceWatermarkRoundTrip(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSpaces(ctx, "confluence", []SpaceRow{
		{Key: "AAA", Name: "Alpha", Kind: "global"},
		{Key: "BBB", Name: "Beta", Kind: "global"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSpaceWatermark(ctx, "confluence", "AAA", "2026-01-01T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	got, err := db.ConfluenceSpaceWatermarks(ctx, "confluence")
	if err != nil {
		t.Fatal(err)
	}
	if got["AAA"] != "2026-01-01T00:00:00.000Z" {
		t.Errorf("AAA watermark = %q", got["AAA"])
	}
	if got["BBB"] != "" {
		t.Errorf("BBB watermark = %q, want empty (NULL)", got["BBB"])
	}
	// Re-upsert name must not wipe watermark.
	if err := db.UpsertSpaces(ctx, "confluence", []SpaceRow{
		{Key: "AAA", Name: "Alpha 2", Kind: "global"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err = db.ConfluenceSpaceWatermarks(ctx, "confluence")
	if err != nil {
		t.Fatal(err)
	}
	if got["AAA"] != "2026-01-01T00:00:00.000Z" {
		t.Errorf("watermark wiped by UpsertSpaces: %q", got["AAA"])
	}
}
