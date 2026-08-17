package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// seedSearchRelevance builds an isolated fixture for key-lookup ranking.
// It does not open examples/demo.db (regenerated — GDK-114).
func seedSearchRelevance(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://example.invalid/wiki"}); err != nil {
		t.Fatal(err)
	}

	// REL-140 is the lookup target. Its title/body do not contain the key, so
	// FTS cannot find it — the same hole as NMB-140 in the demo snapshot.
	// KEY-1 / KEY-10 / KEY-11 / KEY-2 are the prefix family.
	// REL-T / REL-B / REL-C isolate title / body / comment for one shared term.
	// The body row repeats the term so default equal-weight bm25 prefers it;
	// weighted columns must still put the title row first.
	rankBody := strings.Repeat("RankNeedleXYZ ", 24) + "padding so the body is long enough to look like a real description."
	if _, err := db.UpsertIssues(context.Background(), Batch{
		Categories: fixtureCategories,
		Records: []IssueRecord{
			{
				Item: Item{
					ID: "jira:rel-140", SourceID: "jira", Kind: "issue", ExternalID: "rel-140",
					Key: "REL-140", Title: "The real issue whose key is not in the prose",
					BodyText:  "Description without the identifier.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "REL", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:key-1", SourceID: "jira", Kind: "issue", ExternalID: "key-1",
					Key: "KEY-1", Title: "First of the prefix family",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "KEY", IssueType: "Task", IssueTypeID: "10002",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:key-10", SourceID: "jira", Kind: "issue", ExternalID: "key-10",
					Key: "KEY-10", Title: "Tenth of the prefix family",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "KEY", IssueType: "Task", IssueTypeID: "10002",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:key-11", SourceID: "jira", Kind: "issue", ExternalID: "key-11",
					Key: "KEY-11", Title: "Eleventh of the prefix family",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "KEY", IssueType: "Task", IssueTypeID: "10002",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:key-2", SourceID: "jira", Kind: "issue", ExternalID: "key-2",
					Key: "KEY-2", Title: "Sibling outside the prefix family",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "KEY", IssueType: "Task", IssueTypeID: "10002",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:rel-t", SourceID: "jira", Kind: "issue", ExternalID: "rel-t",
					Key: "REL-T", Title: "RankNeedleXYZ lives only in this title",
					BodyText:  "Generic description with no ranking token.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "REL", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:rel-b", SourceID: "jira", Kind: "issue", ExternalID: "rel-b",
					Key: "REL-B", Title: "Ordinary summary without the ranking token",
					BodyText:  rankBody,
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "REL", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:rel-c", SourceID: "jira", Kind: "issue", ExternalID: "rel-c",
					Key: "REL-C", Title: "Nothing of the ranking token in the title",
					BodyText:  "Nothing of the ranking token in the body either.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "REL", IssueType: "Task", IssueTypeID: "10002",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
				Comments: []Comment{{
					ID: "jira:rel-c1", ExternalID: "rel-c1", Author: "Ada",
					BodyText:  "Comment carries RankNeedleXYZ once.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Several pages mention REL-140 so FTS fills a small limit with pages.
	mention := strings.Repeat("See REL-140. ", 12)
	emptyADF := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	pages := make([]PageRecord, 0, 6)
	for i, id := range []string{"9001", "9002", "9003", "9004", "9005", "9006"} {
		pages = append(pages, PageRecord{
			Item: Item{
				ID: "confluence:" + id, SourceID: "confluence", Kind: "page", ExternalID: id,
				Key: id, Title: "Mentions the issue many times " + id,
				BodyText: mention, Author: "Pat",
				URL:       "https://example.invalid/wiki/pages/" + id,
				CreatedAt: ago(2), UpdatedAt: ago(1),
			},
			Page: Page{SpaceKey: "ENG", Version: i + 1, Status: "current", BodyADF: emptyADF},
		})
	}
	if _, err := db.UpsertPages(context.Background(), pages); err != nil {
		t.Fatal(err)
	}
}

func TestSearchRelevanceExactKeyBeatsPageMentions(t *testing.T) {
	db := openTemp(t)
	seedSearchRelevance(t, db)

	res, err := db.Search(context.Background(), "REL-140", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) == 0 || res.Keys[0] != "REL-140" {
		t.Fatalf("keys = %v, want REL-140 first (above pages that mention it)", res.Keys)
	}
	if len(res.Pages) == 0 {
		t.Fatalf("pages empty; FTS should still return mention pages after the key hit")
	}
}

func TestSearchRelevanceKeyCaseAndNormalized(t *testing.T) {
	db := openTemp(t)
	seedSearchRelevance(t, db)

	for _, q := range []string{"rel-140", "REL-140", "rel140", "REL140"} {
		res, err := db.Search(context.Background(), q, 5)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if len(res.Keys) == 0 || res.Keys[0] != "REL-140" {
			t.Fatalf("Search(%q) keys = %v, want REL-140 first", q, res.Keys)
		}
	}
}

func TestSearchRelevanceKeyPrefixOrdered(t *testing.T) {
	db := openTemp(t)
	seedSearchRelevance(t, db)

	res, err := db.Search(context.Background(), "KEY-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"KEY-1", "KEY-10", "KEY-11"}
	if len(res.Keys) < 3 {
		t.Fatalf("keys = %v, want prefix family %v", res.Keys, want)
	}
	got := res.Keys[:3]
	for i, k := range want {
		if got[i] != k {
			t.Fatalf("keys[:3] = %v, want %v", got, want)
		}
	}
	for _, k := range res.Keys {
		if k == "KEY-2" {
			t.Fatalf("KEY-2 is not a prefix of KEY-1: %v", res.Keys)
		}
	}
}

func TestSearchRelevanceTitleOutranksBodyOutranksComment(t *testing.T) {
	db := openTemp(t)
	seedSearchRelevance(t, db)

	res, err := db.Search(context.Background(), "RankNeedleXYZ", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"REL-T", "REL-B", "REL-C"}
	got := make([]string, 0, 3)
	for _, k := range res.Keys {
		switch k {
		case "REL-T", "REL-B", "REL-C":
			got = append(got, k)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("field order = %v (all keys %v), want %v", got, res.Keys, want)
	}
}

func TestSearchRelevanceKeySurvivesLimit(t *testing.T) {
	db := openTemp(t)
	seedSearchRelevance(t, db)

	// Six pages mention REL-140; FTS would fill limit 2 with pages.
	res, err := db.Search(context.Background(), "REL-140", 2)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range res.Keys {
		if k == "REL-140" {
			found = true
		}
	}
	if !found {
		t.Fatalf("REL-140 dropped under limit=2: keys=%v pages=%d total=%d", res.Keys, len(res.Pages), res.Total)
	}
	if res.Total > 2 {
		t.Fatalf("total = %d, want <= 2 (limit still applies after key reservation)", res.Total)
	}
}

func TestLooksLikeKey(t *testing.T) {
	cases := []struct {
		q    string
		want bool
	}{
		{"NMB-140", true},
		{"nmb140", true},
		{"NMB-14", true},
		{"140", true},
		{"billing", false},
		{"로그인", false},
		{"flaky upload", false},
		{"NMB", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := looksLikeKey(tc.q); got != tc.want {
			t.Errorf("looksLikeKey(%q) = %v, want %v", tc.q, got, tc.want)
		}
	}
}

func TestNumericQueryMatchesKeyNumber(t *testing.T) {
	// Field find (2026-08-17, GDK-186): "4152" returned nothing for CRWN-4152
	// — the number half of a key was never a lookup form, so the one query a
	// person actually types off a list gets zero rows.
	db := openTemp(t)
	seedSearchRelevance(t, db)

	res, err := db.SearchExplain(context.Background(), "140", 10)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]string{}
	for _, e := range res.Explain {
		byKey[e.Key] = e.Reason
	}
	if byKey["REL-140"] != "key-exact" {
		t.Fatalf("numeric exact: REL-140 reason = %q (explain %+v), want key-exact", byKey["REL-140"], res.Explain)
	}

	// A shorter digit run is a number prefix, same tier as a key prefix.
	pref, err := db.SearchExplain(context.Background(), "14", 10)
	if err != nil {
		t.Fatal(err)
	}
	byKey = map[string]string{}
	for _, e := range pref.Explain {
		byKey[e.Key] = e.Reason
	}
	if byKey["REL-140"] != "key-prefix" {
		t.Fatalf("numeric prefix: REL-140 reason = %q, want key-prefix", byKey["REL-140"])
	}
}

func TestSearchExplainReasons(t *testing.T) {
	db := openTemp(t)
	seedSearchRelevance(t, db)

	res, err := db.SearchExplain(context.Background(), "REL-140", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Explain) == 0 {
		t.Fatal("SearchExplain returned no explain rows")
	}
	if res.Explain[0].Key != "REL-140" || res.Explain[0].Reason != "key-exact" {
		t.Fatalf("first explain = %+v, want REL-140 key-exact", res.Explain[0])
	}
	var sawFTS bool
	for _, e := range res.Explain[1:] {
		if e.Reason != "fts" {
			t.Errorf("mention page explain = %+v, want fts", e)
		}
		if e.Score == nil {
			t.Errorf("fts explain missing score: %+v", e)
		}
		if e.Field == "" {
			t.Errorf("fts explain missing field: %+v", e)
		}
		sawFTS = true
	}
	if !sawFTS {
		t.Fatal("expected fts explain rows for mention pages")
	}

	plain, err := db.Search(context.Background(), "REL-140", 5)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Explain != nil {
		t.Fatalf("Search (no explain) allocated Explain: %+v", plain.Explain)
	}

	pref, err := db.SearchExplain(context.Background(), "KEY-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]string{}
	for _, e := range pref.Explain {
		byKey[e.Key] = e.Reason
	}
	if byKey["KEY-1"] != "key-exact" {
		t.Errorf("KEY-1 reason = %q, want key-exact", byKey["KEY-1"])
	}
	if byKey["KEY-10"] != "key-prefix" {
		t.Errorf("KEY-10 reason = %q, want key-prefix", byKey["KEY-10"])
	}
}
