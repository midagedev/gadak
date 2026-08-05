package store

import (
	"encoding/json"
	"testing"
)

// seedPagesWithIssues puts one issue and two pages (one Korean title) so Search
// can prove kind split, prefix match, and PageDetail in one fixture.
func seedPagesWithIssues(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(Source{ID: "jira", Kind: "jira", BaseURL: "https://j.example"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(Source{ID: "confluence", Kind: "confluence", BaseURL: "https://j.example/wiki"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "NMB-1", Title: "billing webhook retry", BodyText: "sandbox token",
				CreatedAt: ago(2), UpdatedAt: ago(1),
			},
			Issue: Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"로그인 가이드"}]}]}`)
	cmADF := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"도움 됨"}]}]}`)
	if _, err := db.UpsertPages([]PageRecord{
		{
			Item: Item{
				ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
				Key: "100", Title: "로그인이 실패할 때", BodyText: "로그인 가이드 본문",
				Author: "Dana", URL: "https://j.example/wiki/spaces/ENG/pages/100",
				CreatedAt: ago(2), UpdatedAt: ago(1),
			},
			Page: Page{SpaceKey: "ENG", ParentID: "", Version: 3, Status: "current", BodyADF: adf},
			Comments: []Comment{{
				ID: "confluence:c1", ExternalID: "c1", Author: "Lee",
				BodyADF: cmADF, BodyText: "도움 됨",
				CreatedAt: ago(1), UpdatedAt: ago(1),
			}},
		},
		{
			Item: Item{
				ID: "confluence:200", SourceID: "confluence", Kind: "page", ExternalID: "200",
				Key: "200", Title: "Architecture", BodyText: "platform overview",
				Author: "Pat", URL: "https://j.example/wiki/spaces/ENG/pages/200",
				CreatedAt: ago(3), UpdatedAt: ago(2),
			},
			Page: Page{
				SpaceKey: "ENG", ParentID: "100", Version: 1, Status: "current",
				BodyADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSearchReturnsIssuesAndPages(t *testing.T) {
	db := openTemp(t)
	seedPagesWithIssues(t, db)

	// "billing" is in the issue title only.
	res, err := db.Search("billing", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 1 || res.Keys[0] != "NMB-1" {
		t.Errorf("keys = %v, want [NMB-1]", res.Keys)
	}
	if len(res.Pages) != 0 {
		t.Errorf("pages = %+v, want empty", res.Pages)
	}
	if res.Total != 1 {
		t.Errorf("total = %d, want 1", res.Total)
	}

	// "Architecture" hits a page only.
	res, err = db.Search("Architecture", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 0 {
		t.Errorf("keys = %v, want empty", res.Keys)
	}
	if len(res.Pages) != 1 || res.Pages[0].Key != "200" {
		t.Errorf("pages = %+v, want key 200", res.Pages)
	}
	if res.Pages[0].SpaceKey != "ENG" || res.Pages[0].Title != "Architecture" {
		t.Errorf("PageLite = %+v", res.Pages[0])
	}

	// Shared token across kinds: "sandbox" is issue body only; "platform" is page body.
	res, err = db.Search("platform", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pages) != 1 || res.Pages[0].Key != "200" {
		t.Errorf("platform search pages = %+v", res.Pages)
	}
}

func TestSearchKoreanPagePrefix(t *testing.T) {
	db := openTemp(t)
	seedPagesWithIssues(t, db)

	// Title is "로그인이 실패할 때" — bare "로그인" needs prefix rewrite.
	res, err := db.Search("로그인", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range res.Pages {
		if p.Key == "100" {
			found = true
		}
	}
	if !found {
		t.Errorf("Search(%q) pages = %+v, want page 100", "로그인", res.Pages)
	}
}

func TestPageDetailBodyADFAndComments(t *testing.T) {
	db := openTemp(t)
	seedPagesWithIssues(t, db)

	d, err := db.PageDetail("100")
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("PageDetail(100) = nil, want detail")
	}
	if d.Key != "100" || d.SpaceKey != "ENG" || d.Title != "로그인이 실패할 때" {
		t.Errorf("lite fields = %+v", d.PageLite)
	}
	var doc map[string]any
	if err := json.Unmarshal(d.BodyADF, &doc); err != nil {
		t.Fatalf("body_adf not JSON: %v (%s)", err, d.BodyADF)
	}
	if doc["type"] != "doc" {
		t.Errorf("body_adf.type = %v, want doc", doc["type"])
	}
	if len(d.Comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(d.Comments))
	}
	if d.Comments[0].Author != "Lee" || d.Comments[0].BodyText != "도움 됨" {
		t.Errorf("comment = %+v", d.Comments[0])
	}
	if len(d.Comments[0].BodyADF) == 0 {
		t.Error("comment body_adf empty")
	}

	missing, err := db.PageDetail("no-such")
	if err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Errorf("unknown key = %+v, want nil", missing)
	}
}

func TestPageBodyADFColumnRoundTrip(t *testing.T) {
	db := openTemp(t)
	// Schema must be at least v10 (body_adf on pages).
	if got := db.SchemaVersion(); got < 10 {
		t.Fatalf("schema version %d, want ≥ 10", got)
	}
	if err := db.UpsertSource(Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	want := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`)
	if _, err := db.UpsertPages([]PageRecord{{
		Item: Item{
			ID: "confluence:9", SourceID: "confluence", Kind: "page", ExternalID: "9",
			Key: "9", Title: "Hi", BodyText: "hi", CreatedAt: ago(1), UpdatedAt: ago(1),
		},
		Page: Page{SpaceKey: "X", Version: 1, Status: "current", BodyADF: want},
	}}); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.sql.QueryRow(`SELECT body_adf FROM pages WHERE item_id = 'confluence:9'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "" {
		t.Fatal("body_adf column empty after upsert")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stored), &got); err != nil {
		t.Fatalf("stored body_adf: %v", err)
	}
	if got["type"] != "doc" {
		t.Errorf("stored type = %v", got["type"])
	}

	// documentedColumns must list body_adf (TestSchemaMatchesDataModel covers it).
	cols := documentedColumns["pages"]
	found := false
	for _, c := range cols {
		if c == "body_adf" {
			found = true
		}
	}
	if !found {
		t.Error("documentedColumns[pages] missing body_adf")
	}
}

func TestPageLitesOrder(t *testing.T) {
	db := openTemp(t)
	seedPagesWithIssues(t, db)
	pages, err := db.PageLites()
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("PageLites len = %d, want 2", len(pages))
	}
	// space_key, title order: both ENG — Architecture before 로그인이…
	if pages[0].Key != "200" || pages[1].Key != "100" {
		t.Errorf("order = %s then %s, want 200 then 100", pages[0].Key, pages[1].Key)
	}
}
