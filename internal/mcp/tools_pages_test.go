package mcp

import (
	"strings"
	"testing"
)

// GDK-816 FAIL-first: gadak_search's description taught one hydrate verb for
// every hit ("Then hydrate one key with gadak_issue"), but a pages[].key is a
// numeric wiki page id — gadak_issue answers "not in the mirror" and sync
// does not help. The tail must branch: issues hydrate with gadak_issue; a
// page id is read back with gadak_query.
func TestSearchDescriptionBranchesPageHydrate(t *testing.T) {
	for _, needle := range []string{"gadak_issue", "gadak_query", "page id"} {
		if !strings.Contains(toolSearchDescription, needle) {
			t.Errorf("gadak_search description missing %q — page hits need a hydrate path that accepts them:\n%s", needle, toolSearchDescription)
		}
	}
}

// GDK-816 FAIL-first: gadak_query's wiki example selected title and space
// with no identifier, so an agent that copied it could not act on the rows it
// got. The example must carry it.key — items.key, the numeric origin page id
// (the id `gadak page edit` takes; pages.item_id is a different, internal id).
func TestQueryWikiExampleSelectsPageKey(t *testing.T) {
	i := strings.Index(toolQueryDescription, "Wiki pages about an area")
	if i < 0 {
		t.Fatal("gadak_query description lost the wiki example")
	}
	ex := toolQueryDescription[i:]
	if !strings.Contains(ex, "it.key") {
		t.Errorf("wiki example does not select it.key (the origin page id):\n%s", ex)
	}
}

// GDK-816 FAIL-first: a numeric key is a wiki page id (items.key where
// kind='page'), never an issue key — checking the key or running `gadak sync`
// cannot make gadak_issue find it. The miss error must say so, and an
// issue-shaped miss must keep the plain sync advice.
func TestIssueNumericKeyErrorTeachesPageRead(t *testing.T) {
	db := demoDB(t)
	cr := callToolRaw(t, db, toolIssue, map[string]any{"key": "1731040459"})
	if !cr.IsError {
		t.Fatalf("numeric key must miss, got %v", cr.Content)
	}
	for _, needle := range []string{"page id", "gadak_query"} {
		if !strings.Contains(cr.Content[0].Text, needle) {
			t.Errorf("numeric miss missing %q: %s", needle, cr.Content[0].Text)
		}
	}

	cr = callToolRaw(t, db, toolIssue, map[string]any{"key": "ZZZ-9"})
	if !cr.IsError {
		t.Fatalf("issue-shaped miss must miss, got %v", cr.Content)
	}
	if strings.Contains(cr.Content[0].Text, "page id") {
		t.Errorf("issue-shaped miss must not teach the page clause: %s", cr.Content[0].Text)
	}
}
