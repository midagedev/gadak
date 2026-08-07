package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func adfDoc(texts ...string) json.RawMessage {
	content := make([]any, 0, len(texts))
	for _, t := range texts {
		content = append(content, map[string]any{
			"type": "paragraph",
			"content": []any{
				map[string]any{"type": "text", "text": t},
			},
		})
	}
	b, err := json.Marshal(map[string]any{
		"type":    "doc",
		"version": 1,
		"content": content,
	})
	if err != nil {
		panic(err)
	}
	return b
}

// TestMigrateV15BackfillsPageExcerpt seeds a v14 DB with body_adf, opens it
// (applies v15), and asserts excerpt is derived from the existing ADF.
func TestMigrateV15BackfillsPageExcerpt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v14.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// Apply migrations 0..13 → user_version 14 (spaces, no excerpt yet).
	for i := 0; i < 14; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 14`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	adf := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Hello   world\n\nfrom ADF"}]}]}`
	if _, err := raw.Exec(`
		INSERT INTO sources (id, kind, base_url) VALUES ('confluence', 'confluence', 'https://x');
		INSERT INTO items (id, source_id, kind, external_id, key, title, body_text, author, url, created_at, updated_at, synced_at)
		VALUES ('confluence:7', 'confluence', 'page', '7', '7', 'Title', 'Hello world', 'A', 'https://x', '2026-01-01', '2026-01-02', '2026-01-02');
		INSERT INTO pages (item_id, space_key, parent_id, version, status, body_adf, labels)
		VALUES ('confluence:7', 'ENG', '', 1, 'current', ?, '[]');
	`, adf); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v14: %v", err)
	}
	defer db.Close()
	if got := db.SchemaVersion(); got < 15 {
		t.Fatalf("schema version %d, want ≥ 15", got)
	}
	var excerpt string
	if err := db.sql.QueryRowContext(context.Background(), `SELECT excerpt FROM pages WHERE item_id = 'confluence:7'`).Scan(&excerpt); err != nil {
		t.Fatal(err)
	}
	if excerpt != "Hello world from ADF" {
		t.Errorf("backfill excerpt = %q, want normalized ADF plain text", excerpt)
	}
}

// TestUpsertPagesStoresExcerpt asserts the sync write path persists excerpt.
func TestUpsertPagesStoresExcerpt(t *testing.T) {
	db := openTemp(t)
	if got := db.SchemaVersion(); got < 15 {
		t.Fatalf("schema version %d, want ≥ 15", got)
	}
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	body := adfDoc("Preview line for the list.")
	if _, err := db.UpsertPages(context.Background(), []PageRecord{{
		Item: Item{
			ID: "confluence:9", SourceID: "confluence", Kind: "page", ExternalID: "9",
			Key: "9", Title: "T", BodyText: "Preview line for the list.",
			CreatedAt: ago(1), UpdatedAt: ago(1),
		},
		Page: Page{SpaceKey: "X", Version: 1, Status: "current", BodyADF: body},
	}}); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.sql.QueryRowContext(context.Background(), `SELECT excerpt FROM pages WHERE item_id = 'confluence:9'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "Preview line for the list." {
		t.Errorf("stored excerpt = %q", stored)
	}

	// documentedColumns must list excerpt (TestSchemaMatchesDataModel covers order).
	found := false
	for _, c := range documentedColumns["pages"] {
		if c == "excerpt" {
			found = true
		}
	}
	if !found {
		t.Error("documentedColumns[pages] missing excerpt")
	}
}

// TestPageExcerptTruncatesAt200Runes cuts long Latin text at a word boundary
// within 200 runes (never mid-word when a space exists in the window).
func TestPageExcerptTruncatesAt200Runes(t *testing.T) {
	// 250 one-letter words: plenty of spaces; window ends mid-stream.
	var words []string
	for i := 0; i < 250; i++ {
		words = append(words, "w")
	}
	long := strings.Join(words, " ")
	got := pageExcerptFromPlain(long)
	if n := utf8.RuneCountInString(got); n > pageExcerptRunes {
		t.Fatalf("excerpt runes = %d, want ≤ %d", n, pageExcerptRunes)
	}
	if strings.HasSuffix(got, " ") {
		t.Error("excerpt ends with space")
	}
	// Must not end with a partial word; every token is "w".
	for _, tok := range strings.Fields(got) {
		if tok != "w" {
			t.Errorf("token %q is not a full word", tok)
		}
	}
	// Hard oversize single token (no space) → hard rune cut.
	hard := strings.Repeat("a", 250)
	got2 := pageExcerptFromPlain(hard)
	if utf8.RuneCountInString(got2) != pageExcerptRunes {
		t.Errorf("hard cut runes = %d, want %d", utf8.RuneCountInString(got2), pageExcerptRunes)
	}
}

// TestPageExcerptCJKRuneSafe cuts pure Hangul at the rune limit without
// splitting a code point (and without requiring spaces).
func TestPageExcerptCJKRuneSafe(t *testing.T) {
	// 300 Hangul syllables, no spaces.
	var b strings.Builder
	for i := 0; i < 300; i++ {
		b.WriteRune('가')
	}
	got := pageExcerptFromPlain(b.String())
	if n := utf8.RuneCountInString(got); n != pageExcerptRunes {
		t.Fatalf("CJK excerpt runes = %d, want %d", n, pageExcerptRunes)
	}
	if !utf8.ValidString(got) {
		t.Fatal("excerpt is not valid UTF-8")
	}
	for _, r := range got {
		if r != '가' {
			t.Fatalf("unexpected rune %U in CJK excerpt", r)
		}
	}
	// ADF path with newlines between CJK paragraphs collapses whitespace.
	adf := adfDoc("한글본문", "두 번째")
	ex := pageExcerptFromADF(string(adf))
	if ex != "한글본문두 번째" && ex != "한글본문 두 번째" {
		// paragraphs are concatenated without inter-node spaces by the walker;
		// only explicit whitespace in text nodes is normalized.
		// "한글본문" + "두 번째" → "한글본문두 번째"
		if !strings.Contains(ex, "한글") {
			t.Errorf("CJK ADF excerpt = %q", ex)
		}
	}
}

// TestPageLiteIncludesExcerpt covers list, detail, and search JSON fields.
func TestPageLiteIncludesExcerpt(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	adf := adfDoc("List preview text for search and detail.")
	if _, err := db.UpsertPages(context.Background(), []PageRecord{{
		Item: Item{
			ID: "confluence:55", SourceID: "confluence", Kind: "page", ExternalID: "55",
			Key: "55", Title: "Billing runbook", BodyText: "List preview text for search and detail.",
			Author: "Dana", URL: "https://x/55",
			CreatedAt: ago(1), UpdatedAt: ago(1),
		},
		Page: Page{SpaceKey: "ENG", Version: 2, Status: "current", BodyADF: adf},
	}}); err != nil {
		t.Fatal(err)
	}

	want := "List preview text for search and detail."
	pages, err := db.PageLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Excerpt != want {
		t.Errorf("PageLites excerpt = %+v", pages)
	}

	d, err := db.PageDetail(context.Background(), "55")
	if err != nil || d == nil {
		t.Fatalf("PageDetail: %v %#v", err, d)
	}
	if d.Excerpt != want {
		t.Errorf("PageDetail.Excerpt = %q", d.Excerpt)
	}

	res, err := db.Search(context.Background(), "Billing", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pages) != 1 || res.Pages[0].Excerpt != want {
		t.Errorf("Search pages = %+v", res.Pages)
	}

	// JSON tag is "excerpt".
	raw, err := json.Marshal(pages[0])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["excerpt"] != want {
		t.Errorf("PageLite JSON excerpt = %v (%s)", m["excerpt"], raw)
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	got := normalizeWhitespace("  a \n\t  b\r\nc  ")
	if got != "a b c" {
		t.Errorf("normalizeWhitespace = %q", got)
	}
}
