package store

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func seedPeopleComments(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(Source{ID: "jira", Kind: "jira", BaseURL: "https://j.example"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(Source{ID: "confluence", Kind: "confluence", BaseURL: "https://j.example/wiki"}); err != nil {
		t.Fatal(err)
	}

	// ADF-only comment (empty body_text) for fallback coverage.
	adfOnly := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"ADF   only\n\nfallback"}]}]}`)
	// Whitespace-heavy body for snippet normalization.
	messy := "hello\n\nworld   \t  and   more"

	if _, err := db.UpsertIssues(Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{
			{
				Item: Item{
					ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
					Key: "NMB-1", Title: "issue one", CreatedAt: "2026-07-01T00:00:00.000Z",
					UpdatedAt: "2026-08-01T00:00:00.000Z",
				},
				Issue: Issue{
					ProjectKey: "NMB", IssueType: "Bug", Status: "To Do", StatusID: "1",
					StatusCategory: "new",
				},
				Comments: []Comment{
					{
						ID: "jira:c-alex-1", ExternalID: "c1", Author: "Alex Kim", AuthorID: "acc-alex",
						BodyText: messy, CreatedAt: "2026-08-03T10:00:00.000Z",
					},
					{
						ID: "jira:c-alex-2", ExternalID: "c2", Author: "Alex Kim", AuthorID: "acc-alex",
						BodyADF: adfOnly, BodyText: "", CreatedAt: "2026-08-04T10:00:00.000Z",
					},
					{
						ID: "jira:c-other", ExternalID: "c3", Author: "Other", AuthorID: "acc-other",
						BodyText: "not alex", CreatedAt: "2026-08-05T10:00:00.000Z",
					},
				},
			},
			{
				Item: Item{
					ID: "jira:2", SourceID: "jira", Kind: "issue", ExternalID: "2",
					Key: "NMB-2", Title: "issue two", CreatedAt: "2026-07-01T00:00:00.000Z",
					UpdatedAt: "2026-08-01T00:00:00.000Z",
				},
				Issue: Issue{
					ProjectKey: "NMB", IssueType: "Bug", Status: "To Do", StatusID: "1",
					StatusCategory: "new",
				},
				Comments: []Comment{{
					ID: "jira:c-alex-3", ExternalID: "c4", Author: "Alex Kim", AuthorID: "acc-alex",
					BodyText: "on issue two", CreatedAt: "2026-08-02T10:00:00.000Z",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := db.UpsertPages([]PageRecord{{
		Item: Item{
			ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
			Key: "100", Title: "page title", Author: "Dana",
			CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Page: Page{
			SpaceKey: "ENG", Version: 1, Status: "current",
			BodyADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
		},
		Comments: []Comment{{
			ID: "confluence:c-alex", ExternalID: "pc1", Author: "Alex Kim", AuthorID: "acc-alex",
			BodyText: "on a page", CreatedAt: "2026-08-05T12:00:00.000Z",
		}},
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestCommentsByAuthorFilterSortLimit(t *testing.T) {
	db := openTemp(t)
	seedPeopleComments(t, db)

	res, err := db.CommentsByAuthor("acc-alex", 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 4 {
		t.Fatalf("total = %d, want 4", res.Total)
	}
	if res.Author != "Alex Kim" {
		t.Errorf("author = %q, want Alex Kim", res.Author)
	}
	if len(res.Comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2 (limit)", len(res.Comments))
	}
	// Newest first: page (08-05), then ADF issue comment (08-04).
	if res.Comments[0].Key != "100" || res.Comments[0].Kind != "page" {
		t.Errorf("first = %+v, want page 100", res.Comments[0])
	}
	if res.Comments[1].Key != "NMB-1" || res.Comments[1].Kind != "issue" {
		t.Errorf("second = %+v, want issue NMB-1", res.Comments[1])
	}
	// created_at descending across the limited window
	if res.Comments[0].CreatedAt < res.Comments[1].CreatedAt {
		t.Errorf("not sorted desc: %s then %s", res.Comments[0].CreatedAt, res.Comments[1].CreatedAt)
	}
}

func TestCommentsByAuthorMixedKinds(t *testing.T) {
	db := openTemp(t)
	seedPeopleComments(t, db)

	res, err := db.CommentsByAuthor("acc-alex", 50)
	if err != nil {
		t.Fatal(err)
	}
	var sawIssue, sawPage bool
	for _, c := range res.Comments {
		switch c.Kind {
		case "issue":
			sawIssue = true
		case "page":
			sawPage = true
		}
	}
	if !sawIssue || !sawPage {
		t.Errorf("mixed kinds: issue=%v page=%v in %+v", sawIssue, sawPage, res.Comments)
	}
}

func TestCommentsByAuthorEmpty(t *testing.T) {
	db := openTemp(t)
	seedPeopleComments(t, db)

	res, err := db.CommentsByAuthor("acc-nobody", 50)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 0 || len(res.Comments) != 0 {
		t.Errorf("empty author: %+v", res)
	}
	if res.Author != "" {
		t.Errorf("author = %q, want empty", res.Author)
	}
	if res.Comments == nil {
		t.Error("comments must be non-nil empty slice for JSON []")
	}

	// Empty author_id path param → empty result, not a full-table scan.
	res, err = db.CommentsByAuthor("", 50)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 0 || len(res.Comments) != 0 {
		t.Errorf("empty id: %+v", res)
	}
}

func TestCommentsByAuthorSnippetNormalizeAndADF(t *testing.T) {
	db := openTemp(t)
	seedPeopleComments(t, db)

	res, err := db.CommentsByAuthor("acc-alex", 50)
	if err != nil {
		t.Fatal(err)
	}
	byKeyCreated := map[string]AuthorComment{}
	for _, c := range res.Comments {
		byKeyCreated[c.Key+"|"+c.CreatedAt] = c
	}

	// Messy whitespace on NMB-1 older comment.
	messy, ok := byKeyCreated["NMB-1|2026-08-03T10:00:00.000Z"]
	if !ok {
		t.Fatalf("missing messy comment in %+v", res.Comments)
	}
	if messy.Snippet != "hello world and more" {
		t.Errorf("normalized snippet = %q", messy.Snippet)
	}
	if strings.ContainsAny(messy.Snippet, "\n\t") {
		t.Errorf("snippet still has raw whitespace: %q", messy.Snippet)
	}

	// ADF fallback when body_text empty.
	adf, ok := byKeyCreated["NMB-1|2026-08-04T10:00:00.000Z"]
	if !ok {
		t.Fatalf("missing ADF comment in %+v", res.Comments)
	}
	if adf.Snippet != "ADF only fallback" {
		t.Errorf("ADF snippet = %q, want normalized plain text", adf.Snippet)
	}
}

func TestCommentsByAuthorLimitCap(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	// 210 comments from one author so default/max caps are observable.
	comments := make([]Comment, 0, 210)
	for i := 0; i < 210; i++ {
		// Distinct ISO timestamps (millisecond ordinal) keep ORDER BY stable.
		ms := strconv.Itoa(i)
		for len(ms) < 3 {
			ms = "0" + ms
		}
		comments = append(comments, Comment{
			ID:         "jira:c-mass-" + strconv.Itoa(i),
			ExternalID: strconv.Itoa(i),
			Author:     "Mass",
			AuthorID:   "acc-mass",
			BodyText:   "c" + strconv.Itoa(i),
			CreatedAt:  "2026-08-01T00:00:00." + ms + "Z",
		})
	}
	if _, err := db.UpsertIssues(Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:9", SourceID: "jira", Kind: "issue", ExternalID: "9",
				Key: "NMB-9", Title: "mass", CreatedAt: "2026-07-01T00:00:00.000Z",
				UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue:    Issue{ProjectKey: "NMB", Status: "To Do", StatusID: "1", StatusCategory: "new"},
			Comments: comments,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	// Default limit (0 → 50).
	res, err := db.CommentsByAuthor("acc-mass", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 210 {
		t.Fatalf("total = %d, want 210", res.Total)
	}
	if len(res.Comments) != CommentsByAuthorDefaultLimit {
		t.Errorf("default limit: got %d, want %d", len(res.Comments), CommentsByAuthorDefaultLimit)
	}

	// Cap at max even when caller asks for more.
	res, err = db.CommentsByAuthor("acc-mass", 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Comments) != CommentsByAuthorMaxLimit {
		t.Errorf("max cap: got %d, want %d", len(res.Comments), CommentsByAuthorMaxLimit)
	}
}

func TestCommentSnippetRuneSafe(t *testing.T) {
	// 200 CJK runes → cut at 160.
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteRune('한')
	}
	sn := commentSnippet(b.String(), "")
	if n := utf8.RuneCountInString(sn); n != commentSnippetRunes {
		t.Fatalf("runes = %d, want %d", n, commentSnippetRunes)
	}
	if !utf8.ValidString(sn) {
		t.Fatal("snippet not valid UTF-8")
	}
}
