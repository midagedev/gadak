package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
)

func TestPickSprintProjection(t *testing.T) {
	must := func(v any) json.RawMessage {
		t.Helper()
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	obj := func(id int, name, state string) map[string]any {
		return map[string]any{"id": id, "name": name, "state": state}
	}

	t.Run("active beats closed", func(t *testing.T) {
		id, name, state := pickSprint(must([]any{
			obj(1, "old", "closed"),
			obj(2, "now", "active"),
		}))
		if id == nil || *id != 2 || name != "now" || state != "active" {
			t.Fatalf("got id=%v name=%q state=%q", id, name, state)
		}
	})
	t.Run("future beats closed when no active", func(t *testing.T) {
		id, name, state := pickSprint(must([]any{
			obj(10, "done", "closed"),
			obj(4, "next", "future"),
		}))
		if id == nil || *id != 4 || name != "next" || state != "future" {
			t.Fatalf("got id=%v name=%q state=%q", id, name, state)
		}
	})
	t.Run("same state prefers larger id", func(t *testing.T) {
		id, _, state := pickSprint(must([]any{
			obj(8, "a", "active"),
			obj(12, "b", "active"),
		}))
		if id == nil || *id != 12 || state != "active" {
			t.Fatalf("got id=%v state=%q", id, state)
		}
	})
	t.Run("empty array is NULL", func(t *testing.T) {
		id, name, state := pickSprint(must([]any{}))
		if id != nil || name != "" || state != "" {
			t.Fatalf("got id=%v name=%q state=%q", id, name, state)
		}
	})
	t.Run("null is NULL", func(t *testing.T) {
		id, name, state := pickSprint(json.RawMessage("null"))
		if id != nil || name != "" || state != "" {
			t.Fatalf("got id=%v name=%q state=%q", id, name, state)
		}
	})
	t.Run("non-object element empties", func(t *testing.T) {
		id, name, state := pickSprint(must([]any{
			obj(2, "now", "active"),
			"com.atlassian.greenhopper.service.sprint.Sprint@abc",
		}))
		if id != nil || name != "" || state != "" {
			t.Fatalf("got id=%v name=%q state=%q", id, name, state)
		}
	})
	t.Run("string id is accepted", func(t *testing.T) {
		id, _, state := pickSprint(must([]any{
			map[string]any{"id": "15", "name": "s", "state": "active"},
		}))
		if id == nil || *id != 15 || state != "active" {
			t.Fatalf("got id=%v state=%q", id, state)
		}
	})
}

func TestFindGhSprintField(t *testing.T) {
	catalog := []jira.FieldInfo{
		{ID: "customfield_10020"},
	}
	catalog[0].Schema.Custom = "com.pyxis.greenhopper.jira:gh-sprint"
	if got := findGhSprintField(catalog); got != "customfield_10020" {
		t.Fatalf("got %q", got)
	}
	if got := findGhSprintField(nil); got != "" {
		t.Fatalf("empty catalog %q", got)
	}
}

func ghSprintField() map[string]any {
	return map[string]any{
		"id": "customfield_10020", "name": "Sprint", "custom": true,
		"schema": map[string]any{
			"type": "array", "items": "json",
			"custom": "com.pyxis.greenhopper.jira:gh-sprint",
		},
	}
}

func sprintIssue(t *testing.T, id, key string, sprints any) json.RawMessage {
	t.Helper()
	return raw(t, map[string]any{
		"id": id, "key": key,
		"fields": map[string]any{
			"summary":           key,
			"project":           map[string]any{"key": "NMB"},
			"issuetype":         map[string]any{"id": "10004", "name": "Bug"},
			"status":            statusObj("3", "en"),
			"created":           "2026-07-01T10:00:00.000+0900",
			"updated":           "2026-08-04T18:15:00.000+0900",
			"customfield_10020": sprints,
		},
		"changelog": map[string]any{"total": 0, "histories": []any{}},
	})
}

func TestSprintIngestProjection(t *testing.T) {
	obj := func(id int, name, state string) map[string]any {
		return map[string]any{"id": id, "name": name, "state": state}
	}
	site := &fakeSite{t: t, lang: "en", pageSize: 10, failOffset: -1,
		changelog: map[string]string{}, comments: map[string]string{},
		fieldCatalog: []map[string]any{ghSprintField()},
		issues: []json.RawMessage{
			sprintIssue(t, "1", "NMB-A", []any{obj(1, "old", "closed"), obj(2, "now", "active")}),
			sprintIssue(t, "2", "NMB-F", []any{obj(10, "done", "closed"), obj(4, "next", "future")}),
			sprintIssue(t, "3", "NMB-E", []any{}),
			sprintIssue(t, "4", "NMB-X", []any{obj(2, "now", "active"), "not-an-object"}),
		},
	}
	db := newMirror(t)
	if _, err := Run(context.Background(), testConfig(), db.DB, Options{Full: true, Client: site.start()}); err != nil {
		t.Fatal(err)
	}
	joined := ""
	site.mu.Lock()
	for _, f := range site.syncFields {
		joined += f + ","
	}
	site.mu.Unlock()
	if !containsCSV(joined, "customfield_10020") && joined != "*all," {
		t.Errorf("search fields %q, want customfield_10020 (or *all)", joined)
	}

	conn, err := sql.Open("sqlite", "file:"+db.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	assertSprint := func(key string, wantID *int64, wantState string) {
		t.Helper()
		var id sql.NullInt64
		var state sql.NullString
		if err := conn.QueryRow(
			`SELECT sprint_id, sprint_state FROM issues WHERE key = ?`, key).
			Scan(&id, &state); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if wantID == nil {
			if id.Valid {
				t.Errorf("%s sprint_id=%d, want NULL", key, id.Int64)
			}
		} else if !id.Valid || id.Int64 != *wantID {
			t.Errorf("%s sprint_id=%v, want %d", key, id, *wantID)
		}
		if wantState == "" {
			if state.Valid {
				t.Errorf("%s sprint_state=%q, want NULL", key, state.String)
			}
		} else if !state.Valid || state.String != wantState {
			t.Errorf("%s sprint_state=%q, want %s", key, state.String, wantState)
		}
	}
	two := int64(2)
	four := int64(4)
	assertSprint("NMB-A", &two, "active")
	assertSprint("NMB-F", &four, "future")
	assertSprint("NMB-E", nil, "")
	assertSprint("NMB-X", nil, "")
}

func TestSprintFieldAbsentLeavesNull(t *testing.T) {
	site := &fakeSite{t: t, lang: "en", pageSize: 10, failOffset: -1,
		changelog: map[string]string{}, comments: map[string]string{},
		issues: []json.RawMessage{
			sprintIssue(t, "1", "NMB-A", []any{
				map[string]any{"id": 2, "name": "now", "state": "active"},
			}),
		},
	}
	db := newMirror(t)
	if _, err := Run(context.Background(), testConfig(), db.DB, Options{Full: true, Client: site.start()}); err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite", "file:"+db.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var id sql.NullInt64
	if err := conn.QueryRow(`SELECT sprint_id FROM issues WHERE key = 'NMB-A'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id.Valid {
		t.Errorf("sprint_id=%d, want NULL when GET /field has no gh-sprint", id.Int64)
	}
}

func TestSprintFieldLookupCachedAcrossRuns(t *testing.T) {
	site := newSite(t, "en")
	site.fieldCatalog = []map[string]any{ghSprintField()}
	client := site.start()
	db := newMirror(t)
	cache := &sprintFieldCache{}
	opts := Options{Full: true, Client: client, sprintField: cache}
	if _, err := Run(context.Background(), testConfig(), db.DB, opts); err != nil {
		t.Fatal(err)
	}
	opts.Full = false
	if _, err := Run(context.Background(), testConfig(), db.DB, opts); err != nil {
		t.Fatal(err)
	}
	site.mu.Lock()
	n := site.fieldHits
	site.mu.Unlock()
	if n != 1 {
		t.Fatalf("GET /field hits = %d, want 1 (cached across Run)", n)
	}
}

func TestSprintFieldLookupRefreshesOnReconcile(t *testing.T) {
	site := newSite(t, "en")
	site.fieldCatalog = []map[string]any{ghSprintField()}
	client := site.start()
	db := newMirror(t)
	cache := &sprintFieldCache{}
	opts := Options{Full: true, Client: client, sprintField: cache}
	if _, err := Run(context.Background(), testConfig(), db.DB, opts); err != nil {
		t.Fatal(err)
	}
	opts.Full = false
	opts.Reconcile = true
	if _, err := Run(context.Background(), testConfig(), db.DB, opts); err != nil {
		t.Fatal(err)
	}
	site.mu.Lock()
	n := site.fieldHits
	site.mu.Unlock()
	if n != 2 {
		t.Fatalf("GET /field hits = %d, want 2 (reconcile refreshes)", n)
	}
}

func containsCSV(joined, want string) bool {
	for _, p := range splitCSV(joined) {
		if p == want {
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func TestAppendSprintField(t *testing.T) {
	got := appendSprintField([]string{"summary", "status"}, "customfield_10020")
	if len(got) != 3 || got[2] != "customfield_10020" {
		t.Fatalf("%v", got)
	}
	all := appendSprintField([]string{"*all"}, "customfield_10020")
	if len(all) != 1 || all[0] != "*all" {
		t.Fatalf("*all %v", all)
	}
	dup := appendSprintField([]string{"summary", "customfield_10020"}, "customfield_10020")
	if len(dup) != 2 {
		t.Fatalf("dup %v", dup)
	}
}
