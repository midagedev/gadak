package store

import (
	"context"
	"encoding/json"
	"testing"
)

// seedPagesWithIssues puts one issue and two pages (one Korean title) so Search
// can prove kind split, prefix match, and PageDetail in one fixture.
func seedPagesWithIssues(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://j.example"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://j.example/wiki"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{
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
	if _, err := db.UpsertPages(context.Background(), []PageRecord{
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
	res, err := db.Search(context.Background(), "billing", 10)
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
	res, err = db.Search(context.Background(), "Architecture", 10)
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
	res, err = db.Search(context.Background(), "platform", 10)
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
	res, err := db.Search(context.Background(), "로그인", 10)
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

	d, err := db.PageDetail(context.Background(), "100")
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

	missing, err := db.PageDetail(context.Background(), "no-such")
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
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	want := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`)
	if _, err := db.UpsertPages(context.Background(), []PageRecord{{
		Item: Item{
			ID: "confluence:9", SourceID: "confluence", Kind: "page", ExternalID: "9",
			Key: "9", Title: "Hi", BodyText: "hi", CreatedAt: ago(1), UpdatedAt: ago(1),
		},
		Page: Page{SpaceKey: "X", Version: 1, Status: "current", BodyADF: want},
	}}); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.sql.QueryRowContext(context.Background(), `SELECT body_adf FROM pages WHERE item_id = 'confluence:9'`).Scan(&stored); err != nil {
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
	pages, err := db.PageLites(context.Background())
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

// TestPageLabelsRoundTrip is FAIL-first for schema v13: labels JSON array on
// pages, surfaced on PageLite/PageDetail (empty array when absent, never null).
func TestPageLabelsRoundTrip(t *testing.T) {
	db := openTemp(t)
	if got := db.SchemaVersion(); got < 13 {
		t.Fatalf("schema version %d, want ≥ 13 (labels on pages)", got)
	}
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertPages(context.Background(), []PageRecord{{
		Item: Item{
			ID: "confluence:42", SourceID: "confluence", Kind: "page", ExternalID: "42",
			Key: "42", Title: "Labeled", BodyText: "body",
			CreatedAt: ago(1), UpdatedAt: ago(1),
		},
		Page: Page{
			SpaceKey: "X", Version: 1, Status: "current",
			Labels:  []string{"runbook", "ops"},
			BodyADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
		},
	}}); err != nil {
		t.Fatal(err)
	}

	var stored string
	if err := db.sql.QueryRowContext(context.Background(), `SELECT labels FROM pages WHERE item_id = 'confluence:42'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "" || stored == "null" {
		t.Fatalf("labels column = %q, want JSON array", stored)
	}
	var decoded []string
	if err := json.Unmarshal([]byte(stored), &decoded); err != nil {
		t.Fatalf("labels not JSON: %v (%s)", err, stored)
	}
	if len(decoded) != 2 || decoded[0] != "runbook" || decoded[1] != "ops" {
		t.Errorf("stored labels = %v, want [runbook ops]", decoded)
	}

	pages, err := db.PageLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("PageLites len = %d, want 1", len(pages))
	}
	if pages[0].Labels == nil {
		t.Fatal("PageLite.Labels is nil, want empty or filled array")
	}
	if len(pages[0].Labels) != 2 || pages[0].Labels[0] != "runbook" || pages[0].Labels[1] != "ops" {
		t.Errorf("PageLite.Labels = %v", pages[0].Labels)
	}

	d, err := db.PageDetail(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		t.Fatal("PageDetail(42) = nil")
	}
	if len(d.Labels) != 2 || d.Labels[0] != "runbook" {
		t.Errorf("PageDetail.Labels = %v", d.Labels)
	}

	// Absent labels → empty array, not null.
	if _, err := db.UpsertPages(context.Background(), []PageRecord{{
		Item: Item{
			ID: "confluence:43", SourceID: "confluence", Kind: "page", ExternalID: "43",
			Key: "43", Title: "No labels", BodyText: "x",
			CreatedAt: ago(1), UpdatedAt: ago(1),
		},
		Page: Page{SpaceKey: "X", Version: 1, Status: "current"},
	}}); err != nil {
		t.Fatal(err)
	}
	d2, err := db.PageDetail(context.Background(), "43")
	if err != nil {
		t.Fatal(err)
	}
	if d2 == nil {
		t.Fatal("PageDetail(43) = nil")
	}
	if d2.Labels == nil {
		t.Error("absent Labels decoded as nil, want empty slice")
	}
	if len(d2.Labels) != 0 {
		t.Errorf("absent Labels = %v, want empty", d2.Labels)
	}

	cols := documentedColumns["pages"]
	found := false
	for _, c := range cols {
		if c == "labels" {
			found = true
		}
	}
	if !found {
		t.Error("documentedColumns[pages] missing labels")
	}
}
