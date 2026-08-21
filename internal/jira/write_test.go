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

// Contract: Transition is the single place that builds the POST body.
// Empty fields/comment omit those keys so the bytes match an id-only request.
func TestTransitionPOSTBody(t *testing.T) {
	var got []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, string(b))
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := context.Background()

	if err := c.Transition(ctx, "NMB-1", "31", nil, nil); err != nil {
		t.Fatalf("id-only: %v", err)
	}
	if err := c.Transition(ctx, "NMB-1", "31", map[string]any{}, nil); err != nil {
		t.Fatalf("empty fields: %v", err)
	}
	if err := c.Transition(ctx, "NMB-1", "41", map[string]any{"resolution": map[string]string{"id": "10002"}}, nil); err != nil {
		t.Fatalf("fields: %v", err)
	}
	adf := Doc("closing out", nil)
	if err := c.Transition(ctx, "NMB-1", "31", nil, adf); err != nil {
		t.Fatalf("comment: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("calls=%d bodies=%v", len(got), got)
	}
	if got[0] != `{"transition":{"id":"31"}}` {
		t.Errorf("id-only %s", got[0])
	}
	if got[1] != `{"transition":{"id":"31"}}` {
		t.Errorf("empty fields must omit the key: %s", got[1])
	}
	var withFields map[string]any
	if err := json.Unmarshal([]byte(got[2]), &withFields); err != nil {
		t.Fatalf("fields body %s: %v", got[2], err)
	}
	if _, ok := withFields["update"]; ok {
		t.Errorf("fields-only must omit update: %s", got[2])
	}
	if !strings.Contains(got[2], `"id":"10002"`) || !strings.Contains(got[2], `"id":"41"`) {
		t.Errorf("fields payload: %s", got[2])
	}
	var withComment map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got[3]), &withComment); err != nil {
		t.Fatalf("comment body %s: %v", got[3], err)
	}
	if _, ok := withComment["fields"]; ok {
		t.Errorf("comment-only must omit fields: %s", got[3])
	}
	if !strings.Contains(got[3], `"type":"doc"`) || !strings.Contains(got[3], "closing out") {
		t.Errorf("comment ADF: %s", got[3])
	}
}

// TestAddCommentPOSTBody is FAIL-first for GDK-511: unset visibility/internal
// omit those keys (byte-identical to the previous {"body":…} POST); --internal
// and visibility add the documented Jira shapes.
func TestAddCommentPOSTBody(t *testing.T) {
	var got []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, string(b))
		_, _ = w.Write([]byte(`{"id":"c-1"}`))
	}))
	ctx := context.Background()
	adf := Doc("hello", nil)

	if _, err := c.AddComment(ctx, "NMB-1", adf, nil, false); err != nil {
		t.Fatalf("plain: %v", err)
	}
	wantPlain, err := json.Marshal(map[string]any{"body": adf})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != string(wantPlain) {
		t.Errorf("plain POST not byte-identical\n got: %s\nwant: %s", got[0], wantPlain)
	}
	var plain map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got[0]), &plain); err != nil {
		t.Fatal(err)
	}
	if _, ok := plain["visibility"]; ok {
		t.Errorf("plain POST must omit visibility: %s", got[0])
	}
	if _, ok := plain["properties"]; ok {
		t.Errorf("plain POST must omit properties: %s", got[0])
	}

	if _, err := c.AddComment(ctx, "NMB-1", adf, nil, true); err != nil {
		t.Fatalf("internal: %v", err)
	}
	var internal map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got[1]), &internal); err != nil {
		t.Fatalf("internal body %s: %v", got[1], err)
	}
	var props []struct {
		Key   string `json:"key"`
		Value struct {
			Internal bool `json:"internal"`
		} `json:"value"`
	}
	if err := json.Unmarshal(internal["properties"], &props); err != nil {
		t.Fatalf("properties %s: %v", internal["properties"], err)
	}
	if len(props) == 0 || props[0].Key != "sd.public.comment" || !props[0].Value.Internal {
		t.Errorf("internal properties = %s", internal["properties"])
	}

	vis := &CommentVisibility{Type: "role", Value: "Administrators"}
	if _, err := c.AddComment(ctx, "NMB-1", adf, vis, false); err != nil {
		t.Fatalf("visibility: %v", err)
	}
	var restricted map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got[2]), &restricted); err != nil {
		t.Fatalf("visibility body %s: %v", got[2], err)
	}
	var gotVis CommentVisibility
	if err := json.Unmarshal(restricted["visibility"], &gotVis); err != nil {
		t.Fatalf("visibility %s: %v", restricted["visibility"], err)
	}
	if gotVis.Type != "role" || gotVis.Value != "Administrators" {
		t.Errorf("visibility = %+v", gotVis)
	}
	if _, ok := restricted["properties"]; ok {
		t.Errorf("visibility-only POST must omit properties: %s", got[2])
	}
}
