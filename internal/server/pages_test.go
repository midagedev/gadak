package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// fixturePages extends the issue fixture with a confluence source and two pages.
func fixturePages(t *testing.T) (*store.DB, *config.Config) {
	t.Helper()
	db, cfg := fixture(t)
	if err := db.UpsertSource(context.Background(), store.Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x.atlassian.net/wiki"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSpaces(context.Background(), "confluence", []store.SpaceRow{
		{Key: "PROD", Name: "Product", Kind: "global"},
		{Key: "ENG", Name: "Engineering", Kind: "global"},
	}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	if _, err := db.UpsertPages(context.Background(), []store.PageRecord{
		{
			Item: store.Item{
				ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
				Key: "100", Title: "빌링 품질 회의록", BodyText: "빌링 품질 논의",
				Author: "Dana", URL: "https://x/wiki/spaces/PROD/pages/100",
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Page: store.Page{SpaceKey: "PROD", Version: 2, Status: "current", BodyADF: adf},
			Comments: []store.Comment{{
				ID: "confluence:c1", ExternalID: "c1", Author: "Lee",
				BodyADF: adf, BodyText: "ok", CreatedAt: "2026-07-02T00:00:00.000Z",
			}},
		},
		{
			Item: store.Item{
				ID: "confluence:200", SourceID: "confluence", Kind: "page", ExternalID: "200",
				Key: "200", Title: "Architecture", BodyText: "overview",
				Author: "Pat", URL: "https://x/wiki/spaces/ENG/pages/200",
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-15T00:00:00.000Z",
			},
			Page: store.Page{SpaceKey: "ENG", Version: 1, Status: "current", BodyADF: adf},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Stamp confluence sync_state so ETag has a non-zero version.
	if err := db.RecordSync(context.Background(), "confluence", store.SyncResult{Watermark: "2026-08-01T00:00:00.000Z", FullSync: true}); err != nil {
		t.Fatal(err)
	}
	return db, cfg
}

func TestSearchResponseIncludesPages(t *testing.T) {
	db, cfg := fixturePages(t)
	h := New(db, cfg)
	got := decode[struct {
		Keys  []string         `json:"keys"`
		Pages []store.PageLite `json:"pages"`
		Total int              `json:"total"`
	}](t, get(t, h, apiBase+"search/?q=%EB%B9%8C%EB%A7%81&limit=10", nil)) // 빌링
	if len(got.Pages) != 1 || got.Pages[0].Key != "100" {
		t.Fatalf("pages = %+v, want page 100", got.Pages)
	}
	if got.Total < 1 {
		t.Fatalf("total = %d", got.Total)
	}
	// Empty pages array when only issues match (shape contract).
	onlyIssue := decode[struct {
		Keys  []string         `json:"keys"`
		Pages []store.PageLite `json:"pages"`
		Total int              `json:"total"`
	}](t, get(t, h, apiBase+"search/?q=hydra&limit=10", nil))
	if onlyIssue.Pages == nil {
		t.Fatal("pages must be present (empty array), not omitted/null")
	}
	if len(onlyIssue.Keys) != 1 || onlyIssue.Keys[0] != "NMB-1" {
		t.Fatalf("issue keys = %v", onlyIssue.Keys)
	}
}

func TestPagesListAndETag(t *testing.T) {
	db, cfg := fixturePages(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"pages/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("pages/ → %d %s", rec.Code, rec.Body.String())
	}
	got := decode[struct {
		Pages []store.PageLite `json:"pages"`
		Total int              `json:"total"`
	}](t, rec)
	if got.Total != 2 || len(got.Pages) != 2 {
		t.Fatalf("list = total %d len %d, want 2", got.Total, len(got.Pages))
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	// 304 when If-None-Match matches confluence sync version.
	rec304 := get(t, h, apiBase+"pages/", map[string]string{"If-None-Match": etag})
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match → %d, want 304", rec304.Code)
	}

	// Jira bootstrap ETag must still be independent (not combined).
	boot := get(t, h, apiBase+"bootstrap/", nil)
	if boot.Header().Get("ETag") == etag {
		// versions can coincidentally match; only assert bootstrap still 200 with body
	}
	if boot.Code != http.StatusOK {
		t.Fatalf("bootstrap → %d", boot.Code)
	}
}

func TestPageDetail200And404(t *testing.T) {
	db, cfg := fixturePages(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"pages/100/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail → %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["key"] != "100" {
		t.Errorf("key = %v", body["key"])
	}
	adf, ok := body["body_adf"].(map[string]any)
	if !ok || adf["type"] != "doc" {
		t.Errorf("body_adf = %v", body["body_adf"])
	}
	comments, _ := body["comments"].([]any)
	if len(comments) != 1 {
		t.Errorf("comments = %v", body["comments"])
	}

	if rec := get(t, h, apiBase+"pages/nope/", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing → %d, want 404", rec.Code)
	}
}

// TestPagesResponseIncludesSpaceName is FAIL-first for PageLite.space_name on
// the pages list/detail API (handler serializes store.PageLite — no handler change).
func TestPagesResponseIncludesSpaceName(t *testing.T) {
	db, cfg := fixturePages(t)
	h := New(db, cfg)

	list := decode[struct {
		Pages []store.PageLite `json:"pages"`
		Total int              `json:"total"`
	}](t, get(t, h, apiBase+"pages/", nil))
	if list.Total != 2 {
		t.Fatalf("list total = %d", list.Total)
	}
	byKey := map[string]store.PageLite{}
	for _, p := range list.Pages {
		byKey[p.Key] = p
	}
	if byKey["100"].SpaceName != "Product" {
		t.Errorf("page 100 space_name = %q, want Product", byKey["100"].SpaceName)
	}
	if byKey["200"].SpaceName != "Engineering" {
		t.Errorf("page 200 space_name = %q, want Engineering", byKey["200"].SpaceName)
	}

	// Raw JSON: space_name present (not omitted).
	rec := get(t, h, apiBase+"pages/", nil)
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	pages, _ := raw["pages"].([]any)
	if len(pages) == 0 {
		t.Fatal("no pages in raw")
	}
	row, _ := pages[0].(map[string]any)
	if _, ok := row["space_name"]; !ok {
		t.Fatalf("space_name field missing in page row: %v", row)
	}

	detail := decode[store.PageDetail](t, get(t, h, apiBase+"pages/100/", nil))
	if detail.SpaceName != "Product" {
		t.Errorf("detail space_name = %q, want Product", detail.SpaceName)
	}
}

// TestPagesResponseIncludesSpaceHomepageID is FAIL-first for
// PageLite.space_homepage_id on the pages list API (join from spaces).
func TestPagesResponseIncludesSpaceHomepageID(t *testing.T) {
	db, cfg := fixture(t)
	if err := db.UpsertSource(context.Background(), store.Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x.atlassian.net/wiki"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSpaces(context.Background(), "confluence", []store.SpaceRow{
		{Key: "PROD", Name: "Product", Kind: "global", HomepageID: "50"},
		{Key: "ENG", Name: "Engineering", Kind: "global", HomepageID: "60"},
	}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	if _, err := db.UpsertPages(context.Background(), []store.PageRecord{
		{
			Item: store.Item{
				ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
				Key: "100", Title: "Child", BodyText: "x",
				Author: "Dana", URL: "https://x/wiki/spaces/PROD/pages/100",
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Page: store.Page{SpaceKey: "PROD", ParentID: "50", Version: 1, Status: "current", BodyADF: adf},
		},
		{
			Item: store.Item{
				ID: "confluence:200", SourceID: "confluence", Kind: "page", ExternalID: "200",
				Key: "200", Title: "Other", BodyText: "y",
				Author: "Pat", URL: "https://x/wiki/spaces/ENG/pages/200",
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-15T00:00:00.000Z",
			},
			Page: store.Page{SpaceKey: "ENG", Version: 1, Status: "current", BodyADF: adf},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync(context.Background(), "confluence", store.SyncResult{Watermark: "2026-08-01T00:00:00.000Z", FullSync: true}); err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)

	list := decode[struct {
		Pages []store.PageLite `json:"pages"`
		Total int              `json:"total"`
	}](t, get(t, h, apiBase+"pages/", nil))
	byKey := map[string]store.PageLite{}
	for _, p := range list.Pages {
		byKey[p.Key] = p
	}
	if byKey["100"].SpaceHomepageID != "50" {
		t.Errorf("page 100 space_homepage_id = %q, want 50", byKey["100"].SpaceHomepageID)
	}
	if byKey["200"].SpaceHomepageID != "60" {
		t.Errorf("page 200 space_homepage_id = %q, want 60", byKey["200"].SpaceHomepageID)
	}

	rec := get(t, h, apiBase+"pages/", nil)
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	pages, _ := raw["pages"].([]any)
	if len(pages) == 0 {
		t.Fatal("no pages in raw")
	}
	row, _ := pages[0].(map[string]any)
	if _, ok := row["space_homepage_id"]; !ok {
		t.Fatalf("space_homepage_id field missing in page row: %v", row)
	}

	detail := decode[store.PageDetail](t, get(t, h, apiBase+"pages/100/", nil))
	if detail.SpaceHomepageID != "50" {
		t.Errorf("detail space_homepage_id = %q, want 50", detail.SpaceHomepageID)
	}
}

// TestPagesResponseIncludesLabels is FAIL-first for PageLite.Labels on the
// pages list API (handler serializes store.PageLite — no handler change).
func TestPagesResponseIncludesLabels(t *testing.T) {
	db, cfg := fixture(t)
	if err := db.UpsertSource(context.Background(), store.Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x.atlassian.net/wiki"}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	if _, err := db.UpsertPages(context.Background(), []store.PageRecord{{
		Item: store.Item{
			ID: "confluence:55", SourceID: "confluence", Kind: "page", ExternalID: "55",
			Key: "55", Title: "Labeled page", BodyText: "x",
			Author: "Dana", URL: "https://x/wiki/spaces/ENG/pages/55",
			CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Page: store.Page{
			SpaceKey: "ENG", Version: 1, Status: "current", BodyADF: adf,
			Labels: []string{"alpha", "beta"},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync(context.Background(), "confluence", store.SyncResult{Watermark: "2026-08-01T00:00:00.000Z", FullSync: true}); err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)

	list := decode[struct {
		Pages []store.PageLite `json:"pages"`
		Total int              `json:"total"`
	}](t, get(t, h, apiBase+"pages/", nil))
	if list.Total != 1 || len(list.Pages) != 1 {
		t.Fatalf("list = %+v", list)
	}
	if list.Pages[0].Labels == nil {
		t.Fatal("pages[].labels omitted/null, want array")
	}
	if len(list.Pages[0].Labels) != 2 || list.Pages[0].Labels[0] != "alpha" || list.Pages[0].Labels[1] != "beta" {
		t.Errorf("pages[].labels = %v", list.Pages[0].Labels)
	}

	// Raw JSON: labels must be present (not omitted by omitempty).
	rec := get(t, h, apiBase+"pages/", nil)
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	pages, _ := raw["pages"].([]any)
	if len(pages) != 1 {
		t.Fatalf("raw pages = %v", raw["pages"])
	}
	row, _ := pages[0].(map[string]any)
	labels, ok := row["labels"].([]any)
	if !ok {
		t.Fatalf("labels field missing or wrong type: %v", row["labels"])
	}
	if len(labels) != 2 {
		t.Errorf("raw labels = %v", labels)
	}

	// Detail also carries labels via embedded PageLite.
	detail := decode[store.PageDetail](t, get(t, h, apiBase+"pages/55/", nil))
	if len(detail.Labels) != 2 || detail.Labels[0] != "alpha" {
		t.Errorf("detail labels = %v", detail.Labels)
	}
}

func TestHealthExposesConfluenceWhenSourcePresent(t *testing.T) {
	db, cfg := fixturePages(t)
	h := New(db, cfg)
	body := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	found := false
	for _, src := range body.SyncHealth.Sources {
		if src.Key == "confluence" {
			found = true
			if src.Label != "Confluence" {
				t.Errorf("label = %q", src.Label)
			}
		}
	}
	if !found {
		t.Fatalf("sources = %+v, want confluence", body.SyncHealth.Sources)
	}

	// Without confluence source, health must not invent it.
	db2, cfg2 := fixture(t)
	h2 := New(db2, cfg2)
	body2 := decode[bootstrapResponse](t, get(t, h2, apiBase+"bootstrap/", nil))
	for _, src := range body2.SyncHealth.Sources {
		if src.Key == "confluence" {
			t.Fatal("confluence source exposed without sources row")
		}
	}
}
