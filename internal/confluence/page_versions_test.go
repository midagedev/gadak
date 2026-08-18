package confluence

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestPageVersionsSortsReversedPages is FAIL-first: the listing may arrive
// newest-first (or any order). After following _links.next we sort by number
// ourselves and must not inherit the server's order.
func TestPageVersionsSortsReversedPages(t *testing.T) {
	var paths []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		switch r.URL.Path {
		case "/wiki/rest/api/content/42/version":
			if r.URL.Query().Get("cursor") == "p2" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"results": []map[string]any{
						versionJSON(1, "2026-01-01T00:00:00.000Z", "acc-a", "Ada", "create", false),
					},
					"_links": map[string]string{},
				})
				return
			}
			// Deliberately reversed on page 1 (3 then 2), 1 on the next page.
			next := "/wiki/rest/api/content/42/version?limit=100&cursor=p2"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					versionJSON(3, "2026-01-03T00:00:00.000Z", "acc-c", "Cara", "rewrite", false),
					versionJSON(2, "2026-01-02T00:00:00.000Z", "acc-b", "Bob", "typo", true),
				},
				"_links": map[string]string{"next": next},
			})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))

	got, err := c.PageVersions(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (pagination dropped a page?)", len(got))
	}
	if got[0].Number != 1 || got[1].Number != 2 || got[2].Number != 3 {
		t.Errorf("order = %d,%d,%d want 1,2,3 (must not keep server order)",
			got[0].Number, got[1].Number, got[2].Number)
	}
	if got[0].Message != "create" || got[1].MinorEdit != true || got[2].By.AccountID != "acc-c" {
		t.Errorf("fields = %+v", got)
	}
	if len(paths) < 2 {
		t.Errorf("requests = %v, want both pages followed via _links.next", paths)
	}
}

func versionJSON(n int, when, acc, name, msg string, minor bool) map[string]any {
	return map[string]any{
		"number":    n,
		"when":      when,
		"message":   msg,
		"minorEdit": minor,
		"by":        map[string]any{"accountId": acc, "displayName": name},
	}
}
