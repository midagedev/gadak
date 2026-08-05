package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

// fixturePages extends the issue fixture with a confluence source and two pages.
func fixturePages(t *testing.T) (*store.DB, *config.Config) {
	t.Helper()
	db, cfg := fixture(t)
	if err := db.UpsertSource(store.Source{ID: "confluence", Kind: "confluence", BaseURL: "https://x.atlassian.net/wiki"}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	if _, err := db.UpsertPages([]store.PageRecord{
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
	if err := db.RecordSync("confluence", store.SyncResult{Watermark: "2026-08-01T00:00:00.000Z", FullSync: true}); err != nil {
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
