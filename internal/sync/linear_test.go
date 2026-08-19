package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/store"
)

// linearGraphQL is an httptest GraphQL stub: it answers queryIssues with the
// canned response for the request's cursor and rejects any mutation outright
// (the connector is read-only by constitution).
func linearGraphQL(t *testing.T, pages map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad request body: %v", err)
			http.Error(w, "bad body", 400)
			return
		}
		if strings.Contains(req.Query, "mutation") {
			t.Errorf("mirror sent a mutation: %s", req.Query)
			http.Error(w, "read-only", 400)
			return
		}
		after, _ := req.Variables["after"].(string)
		body, ok := pages[after]
		if !ok {
			t.Errorf("no canned page for cursor %q", after)
			http.Error(w, "no page", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func testLinearClient(t *testing.T, srv *httptest.Server) *linear.Client {
	t.Helper()
	c := linear.New("test-key") // never lin_api_-shaped: scan-internal.sh greps the repo
	c.Endpoint = srv.URL
	c.HTTP = srv.Client()
	c.Retries = 1
	return c
}

func linearTestConfig() *config.Config {
	return &config.Config{
		Site: "https://x.atlassian.net", Email: "t@example.com", Token: "tok",
		Linear: &config.LinearConfig{APIKey: "test-key"},
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "linear", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestRunLinearMirrorsFixtures: the scrubbed live captures, replayed through a
// GraphQL stub, land in issues_full with the contract columns filled — and the
// derived columns Linear cannot honestly fill stay NULL.
func TestRunLinearMirrorsFixtures(t *testing.T) {
	srv := linearGraphQL(t, map[string]string{
		"":                                     readFixture(t, "issues_page1.json"),
		"00000000-0000-4000-8000-000000000000": readFixture(t, "issues_page2.json"),
	})
	t.Cleanup(srv.Close)
	db := newMirror(t)
	cfg := linearTestConfig()

	res, err := RunLinear(context.Background(), cfg, db.DB, Options{LinearClient: testLinearClient(t, srv)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 4 || res.Changed != 4 || !res.Full {
		t.Fatalf("result = %+v, want 4 fetched/changed on a first (full) pass", res)
	}

	one := lite(t, db, "FIX-1")
	if one.StatusCategory != "new" {
		t.Errorf("FIX-1 status_category = %q, want new (state type unstarted)", one.StatusCategory)
	}
	if one.PriorityRank != 0 {
		t.Errorf("FIX-1 priority_rank = %d, want 0 (No priority)", one.PriorityRank)
	}
	if got := db.column(t, "issues", "status_id", "FIX-1"); got == "" {
		t.Error("status_id empty — Linear state id must be stored")
	}
	if got := db.column(t, "issues_full", "status_category", "FIX-3"); got != "new" {
		t.Errorf("issues_full FIX-3 status_category = %q", got)
	}
	// NULL is honest: no history fetched this round, so no status_changed_at,
	// no resolved_at, reopen_count 0. description_adf must stay empty —
	// Linear bodies are markdown, not ADF.
	for _, col := range []string{"status_changed_at", "resolved_at"} {
		if got := db.column(t, "issues", col, "FIX-1"); got != "" {
			t.Errorf("issues.%s = %q, want NULL (Linear history not mirrored)", col, got)
		}
	}
	if got := db.column(t, "issues", "description_adf", "FIX-1"); got != "" {
		t.Errorf("description_adf = %q, want empty — markdown must not pose as ADF", got)
	}
	if got := db.column(t, "items", "body_text", "FIX-1"); !strings.Contains(got, "## Overview") {
		t.Errorf("body_text = %q, want the markdown description", got)
	}
	if got := db.column(t, "items", "url", "FIX-1"); !strings.HasPrefix(got, "https://linear.app/") {
		t.Errorf("url = %q", got)
	}

	state, err := db.SyncState(context.Background(), LinearSourceID)
	if err != nil || state.Watermark != "2026-08-18T13:18:31.131Z" {
		t.Fatalf("watermark = %q err=%v, want the fixtures' max updatedAt", state.Watermark, err)
	}
}

// linearIssuesResponse builds one single-page issues reply from nodes.
func linearIssuesResponse(t *testing.T, nodes []map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"issues": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
				"nodes":    nodes,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func linearNode(key, id, stateType, stateName string, prio int, prioLabel string, extra map[string]any) map[string]any {
	n := map[string]any{
		"id": "00000000-0000-4000-8000-9990000000" + id, "identifier": key, "number": 1,
		"title": "issue " + key, "description": "body of " + key,
		"url":       "https://linear.app/example/issue/" + key + "/x",
		"createdAt": "2026-08-01T00:00:00.000Z", "updatedAt": "2026-08-18T0" + id + ":00:00.000Z",
		"priority": prio, "priorityLabel": prioLabel,
		"state": map[string]any{"id": "state-" + stateType, "name": stateName, "type": stateType, "position": 1},
		"team":  map[string]any{"id": "team-1", "key": "FIX", "name": "Fixture Team"},
		"labels": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
			"nodes":    []map[string]any{{"id": "l1", "name": "bug"}},
		},
		"comments": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
			"nodes":    []map[string]any{},
		},
	}
	for k, v := range extra {
		n[k] = v
	}
	return n
}

// TestRunLinearMapping: the WorkflowState.type → status_category collapse and
// the fixed priority vocabulary → priority_rank mapping, over every branch.
func TestRunLinearMapping(t *testing.T) {
	nodes := []map[string]any{
		linearNode("FIX-11", "1", "started", "In Progress", 1, "Urgent", map[string]any{
			"comments": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
				"nodes": []map[string]any{{
					"id": "c1", "body": "a **markdown** comment",
					"createdAt": "2026-08-02T00:00:00.000Z", "updatedAt": "2026-08-02T00:00:00.000Z",
					"user": map[string]any{"id": "u1", "name": "Fixture User", "displayName": "Fix User"},
				}},
			},
			"assignee": map[string]any{"id": "u1", "name": "Fixture User", "displayName": "Fix User", "email": "u@example.invalid"},
			"parent":   map[string]any{"id": "p1", "identifier": "FIX-1"},
		}),
		linearNode("FIX-12", "2", "completed", "Done", 4, "Low", nil),
		linearNode("FIX-13", "3", "duplicate", "Duplicate", 2, "High", nil),
		linearNode("FIX-14", "4", "triage", "Triage", 3, "Medium", nil),
		// Open enum: a type this build has never heard of must land as new,
		// loudly, never silently something else.
		linearNode("FIX-15", "5", "sometype-from-2030", "Mystery", 0, "No priority", nil),
	}
	srv := linearGraphQL(t, map[string]string{"": linearIssuesResponse(t, nodes)})
	t.Cleanup(srv.Close)
	db := newMirror(t)

	var logs []string
	_, err := RunLinear(context.Background(), linearTestConfig(), db.DB,
		Options{LinearClient: testLinearClient(t, srv), Log: func(s string) { logs = append(logs, s) }})
	if err != nil {
		t.Fatal(err)
	}

	wantCat := map[string]string{"FIX-11": "inprogress", "FIX-12": "done", "FIX-13": "done", "FIX-14": "new", "FIX-15": "new"}
	wantRank := map[string]int{"FIX-11": 1, "FIX-12": 4, "FIX-13": 2, "FIX-14": 3, "FIX-15": 0}
	for key, cat := range wantCat {
		l := lite(t, db, key)
		if l.StatusCategory != cat {
			t.Errorf("%s status_category = %q, want %q", key, l.StatusCategory, cat)
		}
		if l.PriorityRank != wantRank[key] {
			t.Errorf("%s priority_rank = %d, want %d", key, l.PriorityRank, wantRank[key])
		}
	}
	if got := db.column(t, "issues", "priority_id", "FIX-11"); got != "1" {
		t.Errorf("priority_id = %q, want the Linear integer as a stable id", got)
	}
	if got := db.column(t, "issues", "assignee", "FIX-11"); got != "Fix User" {
		t.Errorf("assignee = %q", got)
	}
	if got := db.column(t, "issues", "parent_key", "FIX-11"); got != "FIX-1" {
		t.Errorf("parent_key = %q", got)
	}
	if got := db.column(t, "issues", "labels", "FIX-11"); !strings.Contains(got, "bug") {
		t.Errorf("labels = %q", got)
	}
	// Honest absence: Linear has no issue types.
	if got := db.column(t, "issues", "issue_type_id", "FIX-11"); got != "" {
		t.Errorf("issue_type_id = %q, want empty (Linear has none — no synthetic constants)", got)
	}
	detail, err := db.Detail(context.Background(), "FIX-11")
	if err != nil || len(detail.Comments) != 1 || !strings.Contains(detail.Comments[0].Body, "markdown") {
		t.Fatalf("detail comments = %+v err=%v", detail, err)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "sometype-from-2030") {
		t.Errorf("unknown state type was not reported in the log:\n%s", joined)
	}
}

// TestRunLinearIncrementalUsesWatermark: the second pass filters on
// updatedAt >= watermark instead of refetching the world.
func TestRunLinearIncrementalUsesWatermark(t *testing.T) {
	var gte string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables struct {
				Filter struct {
					UpdatedAt struct {
						Gte string `json:"gte"`
					} `json:"updatedAt"`
				} `json:"filter"`
			} `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gte = req.Variables.Filter.UpdatedAt.Gte
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issues":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}`))
	}))
	t.Cleanup(srv.Close)
	db := newMirror(t)
	if err := db.RecordSync(context.Background(), LinearSourceID, store.SyncResult{Watermark: "2026-08-10T00:00:00.000Z"}); err != nil {
		t.Fatal(err)
	}
	res, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{LinearClient: testLinearClient(t, srv)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full {
		t.Fatal("pass with a watermark must be incremental")
	}
	if gte != "2026-08-10T00:00:00.000Z" {
		t.Fatalf("filter.updatedAt.gte = %q, want the stored watermark", gte)
	}
	// A quiet pass must not clobber the watermark with "".
	state, err := db.SyncState(context.Background(), LinearSourceID)
	if err != nil || state.Watermark != "2026-08-10T00:00:00.000Z" {
		t.Fatalf("watermark after empty pass = %q err=%v, want unchanged", state.Watermark, err)
	}
}

// TestSyncIssueRefusesLinearKeys: the write-through path is Jira-only; a key
// mirrored from Linear gets an explicit read-only refusal, not a Jira lookup
// that would tombstone the row.
func TestSyncIssueRefusesLinearKeys(t *testing.T) {
	srv := linearGraphQL(t, map[string]string{"": readFixture(t, "issues_page1.json"),
		"00000000-0000-4000-8000-000000000000": readFixture(t, "issues_page2.json")})
	t.Cleanup(srv.Close)
	db := newMirror(t)
	cfg := linearTestConfig()
	if _, err := RunLinear(context.Background(), cfg, db.DB, Options{LinearClient: testLinearClient(t, srv)}); err != nil {
		t.Fatal(err)
	}
	err := SyncIssue(context.Background(), cfg, db.DB, "FIX-1", Options{})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("SyncIssue on a Linear key = %v, want an explicit read-only refusal", err)
	}
	if _, missing := findLite(t, db, "FIX-1"); missing {
		t.Fatal("refusal must not tombstone the mirrored row")
	}
}

// findLite reports whether key is still mirrored (no Fatal on absence).
func findLite(t *testing.T, db *mirror, key string) (store.IssueLite, bool) {
	t.Helper()
	lites, err := db.IssueLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lites {
		if l.IssueKey == key {
			return l, false
		}
	}
	return store.IssueLite{}, true
}
