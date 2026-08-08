package store

import (
	"context"
	"encoding/json"
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
	// v14 created the table; v17 added homepage_id (append-only).
	want := "source_id,key,name,kind,homepage_id"
	joined := ""
	for i, c := range got {
		if i > 0 {
			joined += ","
		}
		joined += c
	}
	if joined != want {
		t.Errorf("spaces columns = %q, want %q", joined, want)
	}
	if got := db.SchemaVersion(); got < 17 {
		t.Fatalf("schema version %d, want ≥ 17 (homepage_id)", got)
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
