package store

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// seedSearchMatchFields puts four issues that isolate title / body / comment
// FTS hits so field priority and plain snippets can be asserted without
// opening examples/demo.db.
func seedSearchMatchFields(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	b := Batch{
		Categories: fixtureCategories,
		Records: []IssueRecord{
			{
				Item: Item{
					ID: "jira:sm-title", SourceID: "jira", Kind: "issue", ExternalID: "sm-title",
					Key: "SM-TITLE", Title: "UniqueTitleNeedle appears only here",
					BodyText:  "Generic body with no special tokens for this case.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "SM", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:sm-body", SourceID: "jira", Kind: "issue", ExternalID: "sm-body",
					Key: "SM-BODY", Title: "Ordinary summary without the secret word",
					BodyText:  "The UniqueBodyNeedle lives only in the description of this issue.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "SM", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:sm-comment", SourceID: "jira", Kind: "issue", ExternalID: "sm-comment",
					Key: "SM-COMMENT", Title: "Nothing special in the title",
					BodyText:  "Nothing special in the body either.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "SM", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
				Comments: []Comment{{
					ID: "jira:sm-c1", ExternalID: "sm-c1", Author: "Ada",
					BodyText:  "Reproduced UniqueCommentNeedle on staging with a fresh workspace.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				}},
			},
			{
				// Title and body both contain MultiHitNeedle — field must be title.
				Item: Item{
					ID: "jira:sm-multi", SourceID: "jira", Kind: "issue", ExternalID: "sm-multi",
					Key: "SM-MULTI", Title: "MultiHitNeedle in the title wins",
					BodyText:  "MultiHitNeedle also in the body but lower priority.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "SM", IssueType: "Task", IssueTypeID: "10002",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
				Comments: []Comment{{
					ID: "jira:sm-c2", ExternalID: "sm-c2", Author: "Ada",
					BodyText:  "MultiHitNeedle in a comment too.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				}},
			},
		},
	}
	if _, err := db.UpsertIssues(b); err != nil {
		t.Fatal(err)
	}
}

func assertPlainSnippet(t *testing.T, snip string) {
	t.Helper()
	if strings.Contains(snip, "\x01") || strings.Contains(snip, "\x02") {
		t.Errorf("snippet still has FTS markers: %q", snip)
	}
	if strings.ContainsAny(snip, "<>") && (strings.Contains(snip, "<b>") || strings.Contains(snip, "<em>") || strings.Contains(snip, "<mark>")) {
		t.Errorf("snippet looks like HTML: %q", snip)
	}
	if utf8.RuneCountInString(snip) > searchSnippetRunes+2 { // +2 for optional … wrappers
		// Windows may add one ellipsis each side; allow a small margin.
		if utf8.RuneCountInString(snip) > searchSnippetRunes+4 {
			t.Errorf("snippet too long (%d runes): %q", utf8.RuneCountInString(snip), snip)
		}
	}
}

func TestSearchMatchFieldTitle(t *testing.T) {
	db := openTemp(t)
	seedSearchMatchFields(t, db)

	res, err := db.Search("UniqueTitleNeedle", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 1 || res.Keys[0] != "SM-TITLE" {
		t.Fatalf("keys = %v, want [SM-TITLE]", res.Keys)
	}
	m, ok := res.Matches["SM-TITLE"]
	if !ok {
		t.Fatalf("matches missing SM-TITLE: %+v", res.Matches)
	}
	if m.Field != "title" {
		t.Errorf("field = %q, want title", m.Field)
	}
	if !strings.Contains(m.Snippet, "UniqueTitleNeedle") {
		t.Errorf("snippet = %q, want title needle", m.Snippet)
	}
	assertPlainSnippet(t, m.Snippet)
}

func TestSearchMatchFieldBody(t *testing.T) {
	db := openTemp(t)
	seedSearchMatchFields(t, db)

	res, err := db.Search("UniqueBodyNeedle", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 1 || res.Keys[0] != "SM-BODY" {
		t.Fatalf("keys = %v, want [SM-BODY]", res.Keys)
	}
	m, ok := res.Matches["SM-BODY"]
	if !ok {
		t.Fatalf("matches missing SM-BODY: %+v", res.Matches)
	}
	if m.Field != "body" {
		t.Errorf("field = %q, want body", m.Field)
	}
	if !strings.Contains(m.Snippet, "UniqueBodyNeedle") {
		t.Errorf("snippet = %q, want body needle", m.Snippet)
	}
	assertPlainSnippet(t, m.Snippet)
}

func TestSearchMatchFieldComment(t *testing.T) {
	db := openTemp(t)
	seedSearchMatchFields(t, db)

	res, err := db.Search("UniqueCommentNeedle", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 1 || res.Keys[0] != "SM-COMMENT" {
		t.Fatalf("keys = %v, want [SM-COMMENT]", res.Keys)
	}
	m, ok := res.Matches["SM-COMMENT"]
	if !ok {
		t.Fatalf("matches missing SM-COMMENT: %+v", res.Matches)
	}
	if m.Field != "comment" {
		t.Errorf("field = %q, want comment", m.Field)
	}
	if !strings.Contains(m.Snippet, "UniqueCommentNeedle") {
		t.Errorf("snippet = %q, want comment needle", m.Snippet)
	}
	if !strings.Contains(strings.ToLower(m.Snippet), "staging") {
		t.Errorf("snippet = %q, want staging context", m.Snippet)
	}
	assertPlainSnippet(t, m.Snippet)
}

func TestSearchMatchFieldPriorityTitleWins(t *testing.T) {
	db := openTemp(t)
	seedSearchMatchFields(t, db)

	res, err := db.Search("MultiHitNeedle", 10)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := res.Matches["SM-MULTI"]
	if !ok {
		t.Fatalf("matches missing SM-MULTI: keys=%v matches=%+v", res.Keys, res.Matches)
	}
	if m.Field != "title" {
		t.Errorf("field = %q, want title (priority over body/comment)", m.Field)
	}
	assertPlainSnippet(t, m.Snippet)
}

func TestSearchMatchEmptyQueryAndNoHits(t *testing.T) {
	db := openTemp(t)
	seedSearchMatchFields(t, db)

	res, err := db.Search("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Matches == nil {
		t.Fatal("empty query: Matches must be non-nil {}")
	}
	if len(res.Matches) != 0 || len(res.Keys) != 0 {
		t.Errorf("empty query: got keys=%v matches=%+v", res.Keys, res.Matches)
	}

	res, err = db.Search("zzznomatchtokenzzz", 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Matches == nil {
		t.Fatal("no hits: Matches must be non-nil {}")
	}
	if len(res.Matches) != 0 || len(res.Keys) != 0 {
		t.Errorf("no hits: got keys=%v matches=%+v", res.Keys, res.Matches)
	}
}

func TestSearchMatchExistingFixtureComment(t *testing.T) {
	// Fixture comment: "Reproduced by forcing a retry against the sandbox gateway."
	db := openTemp(t)
	seed(t, db)

	res, err := db.Search("sandbox", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 1 || res.Keys[0] != "NMB-1" {
		t.Fatalf("keys = %v, want [NMB-1]", res.Keys)
	}
	m, ok := res.Matches["NMB-1"]
	if !ok {
		t.Fatalf("matches missing NMB-1: %+v", res.Matches)
	}
	if m.Field != "comment" {
		t.Errorf("field = %q, want comment", m.Field)
	}
	if !strings.Contains(strings.ToLower(m.Snippet), "sandbox") {
		t.Errorf("snippet = %q, want sandbox", m.Snippet)
	}
	assertPlainSnippet(t, m.Snippet)
}

func TestPlainFromFTSSnippetStripsMarkers(t *testing.T) {
	got := plainFromFTSSnippet("before \x01hit\x02 after")
	if got != "before hit after" {
		t.Errorf("got %q", got)
	}
	if strings.Contains(got, "\x01") || strings.Contains(got, "\x02") {
		t.Errorf("markers remain: %q", got)
	}
}
