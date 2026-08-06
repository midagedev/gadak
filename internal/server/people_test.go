package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

func peopleCommentsFixture(t *testing.T) (*store.DB, *config.Config) {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/people.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(store.Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(store.Source{ID: "confluence", Kind: "confluence"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "NMB-1", Title: "mine", CreatedAt: "2026-07-01T00:00:00.000Z",
				UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
			Comments: []store.Comment{
				{
					ID: "jira:c1", ExternalID: "c1", Author: "Alex Kim", AuthorID: "acc-alex",
					BodyText: "first\n\nline", CreatedAt: "2026-08-01T10:00:00.000Z",
				},
				{
					ID: "jira:c2", ExternalID: "c2", Author: "Alex Kim", AuthorID: "acc-alex",
					BodyText: "second", CreatedAt: "2026-08-02T10:00:00.000Z",
				},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertPages([]store.PageRecord{{
		Item: store.Item{
			ID: "confluence:9", SourceID: "confluence", Kind: "page", ExternalID: "9",
			Key: "9", Title: "doc", CreatedAt: "2026-07-01T00:00:00.000Z",
			UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Page: store.Page{
			SpaceKey: "ENG", Version: 1, Status: "current",
			BodyADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
		},
		Comments: []store.Comment{{
			ID: "confluence:c1", ExternalID: "pc1", Author: "Alex Kim", AuthorID: "acc-alex",
			BodyText: "page note", CreatedAt: "2026-08-03T10:00:00.000Z",
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	return db, &config.Config{Site: "https://x.atlassian.net"}
}

func TestPeopleCommentsOK(t *testing.T) {
	db, cfg := peopleCommentsFixture(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"people/acc-alex/comments/?limit=50", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET → %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Author   string `json:"author"`
		Total    int    `json:"total"`
		Comments []struct {
			Key       string `json:"key"`
			Kind      string `json:"kind"`
			Title     string `json:"title"`
			Snippet   string `json:"snippet"`
			CreatedAt string `json:"created_at"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Author != "Alex Kim" || body.Total != 3 {
		t.Errorf("author/total = %q/%d", body.Author, body.Total)
	}
	if len(body.Comments) != 3 {
		t.Fatalf("comments = %d, want 3", len(body.Comments))
	}
	// Newest first: page, then second, then first.
	if body.Comments[0].Key != "9" || body.Comments[0].Kind != "page" {
		t.Errorf("first = %+v", body.Comments[0])
	}
	if body.Comments[1].Key != "NMB-1" || body.Comments[1].Snippet != "second" {
		t.Errorf("second = %+v", body.Comments[1])
	}
	if body.Comments[2].Snippet != "first line" {
		t.Errorf("normalized snippet = %q", body.Comments[2].Snippet)
	}
}

func TestPeopleCommentsEmptyAuthor(t *testing.T) {
	db, cfg := peopleCommentsFixture(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"people/acc-nobody/comments/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET → %d %s (want 200 empty, not 404)", rec.Code, rec.Body.String())
	}
	var body struct {
		Author   string            `json:"author"`
		Total    int               `json:"total"`
		Comments []json.RawMessage `json:"comments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 0 || body.Author != "" || len(body.Comments) != 0 {
		t.Errorf("empty body = %+v", body)
	}
	// comments must serialize as [], not null
	if !strings.Contains(rec.Body.String(), `"comments":[]`) {
		t.Errorf("body = %s, want comments:[]", rec.Body.String())
	}
}

func TestPeopleCommentsLimitAndInvalid(t *testing.T) {
	db, cfg := peopleCommentsFixture(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"people/acc-alex/comments/?limit=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("limit=1 → %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Total    int               `json:"total"`
		Comments []json.RawMessage `json:"comments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 3 || len(body.Comments) != 1 {
		t.Errorf("limit window: total=%d len=%d", body.Total, len(body.Comments))
	}

	rec = get(t, h, apiBase+"people/acc-alex/comments/?limit=nope", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid limit → %d, want 400", rec.Code)
	}
}
