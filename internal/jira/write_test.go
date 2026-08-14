package jira

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Contract: EditIssue sends one PUT /issue/{key} whose body is
// {"fields":…} only, {"update":…} only, or both — empty maps are omitted.
func TestEditIssuePUTBody(t *testing.T) {
	var got []string
	var paths []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		got = append(got, string(b))
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := context.Background()

	if err := c.EditIssue(ctx, "NMB-1", map[string]any{"summary": "x"}, nil); err != nil {
		t.Fatalf("fields only: %v", err)
	}
	if err := c.EditIssue(ctx, "NMB-1", nil, map[string]any{"labels": []any{map[string]string{"add": "a"}}}); err != nil {
		t.Fatalf("update only: %v", err)
	}
	if err := c.EditIssue(ctx, "NMB-1", map[string]any{"summary": "x"}, map[string]any{"labels": []any{map[string]string{"remove": "b"}}}); err != nil {
		t.Fatalf("both: %v", err)
	}

	if len(got) != 3 || len(paths) != 3 {
		t.Fatalf("calls=%d bodies=%d, want 3; paths=%v bodies=%v", len(paths), len(got), paths, got)
	}
	for i, p := range paths {
		if p != "PUT /rest/api/3/issue/NMB-1" {
			t.Errorf("call %d path %s", i, p)
		}
	}

	var fieldsOnly, updateOnly, both map[string]any
	if err := json.Unmarshal([]byte(got[0]), &fieldsOnly); err != nil {
		t.Fatalf("fields-only body %s: %v", got[0], err)
	}
	if _, ok := fieldsOnly["fields"]; !ok {
		t.Errorf("fields-only missing fields: %s", got[0])
	}
	if _, ok := fieldsOnly["update"]; ok {
		t.Errorf("fields-only must omit update: %s", got[0])
	}
	if !strings.Contains(got[0], `"summary":"x"`) {
		t.Errorf("fields-only summary: %s", got[0])
	}

	if err := json.Unmarshal([]byte(got[1]), &updateOnly); err != nil {
		t.Fatalf("update-only body %s: %v", got[1], err)
	}
	if _, ok := updateOnly["update"]; !ok {
		t.Errorf("update-only missing update: %s", got[1])
	}
	if _, ok := updateOnly["fields"]; ok {
		t.Errorf("update-only must omit fields: %s", got[1])
	}
	if !strings.Contains(got[1], `"add":"a"`) {
		t.Errorf("update-only add: %s", got[1])
	}

	if err := json.Unmarshal([]byte(got[2]), &both); err != nil {
		t.Fatalf("both body %s: %v", got[2], err)
	}
	if _, ok := both["fields"]; !ok {
		t.Errorf("both missing fields: %s", got[2])
	}
	if _, ok := both["update"]; !ok {
		t.Errorf("both missing update: %s", got[2])
	}
	if !strings.Contains(got[2], `"summary":"x"`) || !strings.Contains(got[2], `"remove":"b"`) {
		t.Errorf("both payload: %s", got[2])
	}
}
