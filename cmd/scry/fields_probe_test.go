package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/scry/internal/jira"
)

// TestProbeFieldFillsOverFakeJira drives the counting path against a server that
// answers the way Jira does: `*all` returns every field on every issue, including
// the ones that are empty on that issue. The pure helpers are covered elsewhere;
// what this pins down is that the parts agree — that Extra really carries the
// custom fields, that batching does not drop or double-count an issue, and that
// a field present-but-empty does not inflate the report.
func TestProbeFieldFillsOverFakeJira(t *testing.T) {
	issue := func(key string, fields map[string]any) map[string]any {
		return map[string]any{"id": key, "key": key, "fields": fields}
	}
	// customfield_100 is filled on 2 of 3; _101 only ever empty-shaped values;
	// _102 filled once with a zero, which is a value the user chose.
	page := map[string]any{
		"issues": []any{
			issue("NMB-1", map[string]any{
				"summary":         "one",
				"customfield_100": map[string]any{"value": "Platform"},
				"customfield_101": nil,
				"customfield_102": 0,
			}),
			issue("NMB-2", map[string]any{
				"summary":         "two",
				"customfield_100": map[string]any{"value": "Platform"},
				"customfield_101": []any{},
				"customfield_102": nil,
			}),
			issue("NMB-3", map[string]any{
				"summary":         "three",
				"customfield_100": nil,
				"customfield_101": "",
				"customfield_102": nil,
			}),
		},
		"isLast": true,
	}

	var gotJQL, gotFields string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		var body struct {
			JQL    string   `json:"jql"`
			Fields []string `json:"fields"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotJQL, gotFields = body.JQL, strings.Join(body.Fields, ",")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()

	c := jira.New(srv.URL, "a@example.com", "token")
	filled, err := probeFieldFills(context.Background(), c, []string{"NMB-1", "NMB-2", "NMB-3"})
	if err != nil {
		t.Fatalf("probeFieldFills: %v", err)
	}

	if gotFields != "*all" {
		t.Errorf("asked Jira for fields %q, want *all — a narrower list cannot find unmapped fields", gotFields)
	}
	if !strings.HasPrefix(gotJQL, "key in (") || !strings.Contains(gotJQL, `"NMB-2"`) {
		t.Errorf("jql = %q, want a quoted key-in batch", gotJQL)
	}
	for id, want := range map[string]int{
		"customfield_100": 2, // object value on two issues
		"customfield_102": 1, // zero is a value someone entered
		"summary":         3,
	} {
		if filled[id] != want {
			t.Errorf("filled[%s] = %d, want %d", id, filled[id], want)
		}
	}
	if n, ok := filled["customfield_101"]; ok && n != 0 {
		t.Errorf("customfield_101 counted %d times; null, [] and \"\" are all empty", n)
	}
}
