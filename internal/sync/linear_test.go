package sync

import (
	"context"
	"encoding/json"
	"fmt"
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
	if got := db.column(t, "issues", "security_level_id", "FIX-11"); got != "" {
		t.Errorf("security_level_id = %q, want empty (Linear has none — no synthetic constants)", got)
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
// mirrored from Linear gets an explicit refusal, not a Jira lookup that would
// tombstone the row. Linear writes exist (origin.Writer); RefreshIssue routes
// them to SyncLinearIssue.
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
	if err == nil || !strings.Contains(err.Error(), "mirrored from Linear") {
		t.Fatalf("SyncIssue on a Linear key = %v, want an explicit Linear-source refusal", err)
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

func linearCommentNodes(n, start int) []map[string]any {
	out := make([]map[string]any, n)
	for i := 0; i < n; i++ {
		k := start + i
		out[i] = map[string]any{
			"id":        fmt.Sprintf("c-%d", k),
			"body":      fmt.Sprintf("comment %d", k),
			"createdAt": "2026-08-02T00:00:00.000Z",
			"updatedAt": "2026-08-02T00:00:00.000Z",
		}
	}
	return out
}

func seedLinearIssue(t *testing.T, db *mirror, key, extID string) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), store.Source{ID: LinearSourceID, Kind: "linear", BaseURL: "https://linear.app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: LinearSourceID + ":" + extID, SourceID: LinearSourceID, Kind: "issue",
				ExternalID: extID, Key: key, Title: "stale " + key, BodyText: "gone upstream",
				CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "FIX", StatusCategory: "new", Status: "Todo"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func seedJiraIssue(t *testing.T, db *mirror, key string) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:" + key, SourceID: "jira", Kind: "issue",
				ExternalID: key, Key: key, Title: "jira " + key,
				CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "NMB", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// TestRunLinearPriorityRankFromIDNotLabel: Linear priority 3 is labeled
// "Normal" (MAPPING.md). The English list linearPriorities uses "Medium" at
// that slot, so name lookup ranks 0. Rank must follow the integer id.
func TestRunLinearPriorityRankFromIDNotLabel(t *testing.T) {
	node := linearNode("FIX-20", "6", "unstarted", "Todo", 3, "Normal", nil)
	srv := linearGraphQL(t, map[string]string{"": linearIssuesResponse(t, []map[string]any{node})})
	t.Cleanup(srv.Close)
	db := newMirror(t)
	if _, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{LinearClient: testLinearClient(t, srv)}); err != nil {
		t.Fatal(err)
	}
	l := lite(t, db, "FIX-20")
	if l.PriorityRank != 3 {
		t.Fatalf("FIX-20 priority_rank = %d, want 3 (Linear id, not the label %q)", l.PriorityRank, "Normal")
	}
	if l.Priority == nil || *l.Priority != "Normal" {
		t.Errorf("priority display = %v, want the source label kept", l.Priority)
	}
	if l.PriorityID != "3" {
		t.Errorf("priority_id = %q, want the Linear integer", l.PriorityID)
	}
}

// TestRunLinearMirrorsAttachments: attachments on the issue node must land
// as store rows (same schema as the Jira path).
func TestRunLinearMirrorsAttachments(t *testing.T) {
	node := linearNode("FIX-21", "7", "unstarted", "Todo", 0, "No priority", map[string]any{
		"attachments": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
			"nodes": []map[string]any{{
				"id": "att-1", "title": "shot.png",
				"url":       "https://uploads.linear.app/example/shot.png",
				"createdAt": "2026-08-02T00:00:00.000Z",
				"metadata":  map[string]any{"size": 1234, "mimeType": "image/png"},
			}},
		},
	})
	srv := linearGraphQL(t, map[string]string{"": linearIssuesResponse(t, []map[string]any{node})})
	t.Cleanup(srv.Close)
	db := newMirror(t)
	if _, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{LinearClient: testLinearClient(t, srv)}); err != nil {
		t.Fatal(err)
	}
	detail, err := db.Detail(context.Background(), "FIX-21")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(detail.Attachments))
	}
	a := detail.Attachments[0]
	if a.Filename != "shot.png" {
		t.Errorf("filename = %q", a.Filename)
	}
	if a.MimeType != "image/png" || a.Size != 1234 {
		t.Errorf("mime/size = %q %d, want image/png 1234", a.MimeType, a.Size)
	}
	if a.ExternalID != "att-1" {
		t.Errorf("external_id = %q", a.ExternalID)
	}
	if a.URL != "https://uploads.linear.app/example/shot.png" {
		t.Errorf("url = %q, want the Linear content URL stored verbatim", a.URL)
	}
}

// TestRunLinearDetailCarriesDescriptionText: Linear bodies are markdown in
// items.body_text with empty description_adf. Detail must expose that text
// so CLI/UI can render it without stuffing markdown into the ADF column.
func TestRunLinearDetailCarriesDescriptionText(t *testing.T) {
	srv := linearGraphQL(t, map[string]string{
		"":                                     readFixture(t, "issues_page1.json"),
		"00000000-0000-4000-8000-000000000000": readFixture(t, "issues_page2.json"),
	})
	t.Cleanup(srv.Close)
	db := newMirror(t)
	if _, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{LinearClient: testLinearClient(t, srv)}); err != nil {
		t.Fatal(err)
	}
	detail, err := db.Detail(context.Background(), "FIX-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(detail.DescriptionADF); got != "" && got != "null" {
		t.Errorf("description_adf = %q, want empty — markdown must not pose as ADF", got)
	}
	if !strings.Contains(detail.DescriptionText, "## Overview") {
		t.Fatalf("description_text = %q, want the markdown body", detail.DescriptionText)
	}
}

// TestRunLinearPaginatesCommentsPastInlinePage: an issue with 60 comments
// currently keeps only the inline 50. HasNextPage must be followed.
func TestRunLinearPaginatesCommentsPastInlinePage(t *testing.T) {
	const extra = 10
	first := linearCommentNodes(linear.CommentsPageSize, 1)
	rest := linearCommentNodes(extra, linear.CommentsPageSize+1)
	issueID := "00000000-0000-4000-8000-99900000008"
	node := linearNode("FIX-22", "8", "unstarted", "Todo", 0, "No priority", map[string]any{
		"id": issueID,
		"comments": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "cmt-more"},
			"nodes":    first,
		},
	})
	issuesBody := linearIssuesResponse(t, []map[string]any{node})
	moreBody, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"issue": map[string]any{
				"comments": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
					"nodes":    rest,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad body: %v", err)
			http.Error(w, "bad body", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(req.Query, "IssueComments") {
			after, _ := req.Variables["after"].(string)
			if after != "cmt-more" {
				t.Errorf("IssueComments after = %q, want cmt-more", after)
			}
			_, _ = w.Write(moreBody)
			return
		}
		_, _ = w.Write([]byte(issuesBody))
	}))
	t.Cleanup(srv.Close)
	db := newMirror(t)
	if _, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{LinearClient: testLinearClient(t, srv)}); err != nil {
		t.Fatal(err)
	}
	detail, err := db.Detail(context.Background(), "FIX-22")
	if err != nil {
		t.Fatal(err)
	}
	want := linear.CommentsPageSize + extra
	if len(detail.Comments) != want {
		t.Fatalf("comments = %d, want %d (inline page followed to the end)", len(detail.Comments), want)
	}
}

func TestSyncLinearIssuePaginatesComments(t *testing.T) {
	const extra = 10
	first := linearCommentNodes(linear.CommentsPageSize, 1)
	rest := linearCommentNodes(extra, linear.CommentsPageSize+1)
	issueID := "00000000-0000-4000-8000-99900000009"
	iss := linearNode("FIX-23", "9", "unstarted", "Todo", 0, "No priority", map[string]any{
		"id": issueID,
		"comments": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "cmt-more"},
			"nodes":    first,
		},
	})
	oneBody, err := json.Marshal(map[string]any{"data": map[string]any{"issue": iss}})
	if err != nil {
		t.Fatal(err)
	}
	moreBody, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"issue": map[string]any{
				"comments": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
					"nodes":    rest,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(req.Query, "IssueComments") {
			_, _ = w.Write(moreBody)
			return
		}
		_, _ = w.Write(oneBody)
	}))
	t.Cleanup(srv.Close)
	db := newMirror(t)
	if err := db.UpsertSource(context.Background(), store.Source{ID: LinearSourceID, Kind: "linear"}); err != nil {
		t.Fatal(err)
	}
	if err := SyncLinearIssue(context.Background(), db.DB, testLinearClient(t, srv), "FIX-23"); err != nil {
		t.Fatal(err)
	}
	detail, err := db.Detail(context.Background(), "FIX-23")
	if err != nil {
		t.Fatal(err)
	}
	want := linear.CommentsPageSize + extra
	if len(detail.Comments) != want {
		t.Fatalf("comments = %d, want %d after SyncLinearIssue", len(detail.Comments), want)
	}
}

func linearLabelNodes(n, start int) []map[string]any {
	out := make([]map[string]any, n)
	for i := 0; i < n; i++ {
		k := start + i
		out[i] = map[string]any{
			"id":   fmt.Sprintf("l-%d", k),
			"name": fmt.Sprintf("label-%d", k),
		}
	}
	return out
}

func linearAttachmentNodes(n, start int) []map[string]any {
	out := make([]map[string]any, n)
	for i := 0; i < n; i++ {
		k := start + i
		out[i] = map[string]any{
			"id":        fmt.Sprintf("a-%d", k),
			"title":     fmt.Sprintf("file-%d.txt", k),
			"url":       fmt.Sprintf("https://uploads.example/%d", k),
			"createdAt": "2026-08-02T00:00:00.000Z",
			"metadata":  map[string]any{"size": float64(1), "mimeType": "text/plain"},
		}
	}
	return out
}

func TestRunLinearPaginatesLabelsPastInlinePage(t *testing.T) {
	const extra = 10
	first := linearLabelNodes(linear.LabelsPageSize, 1)
	rest := linearLabelNodes(extra, linear.LabelsPageSize+1)
	issueID := "00000000-0000-4000-8000-99900000010"
	node := linearNode("FIX-30", "g", "unstarted", "Todo", 0, "No priority", map[string]any{
		"id": issueID,
		"labels": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "lbl-more"},
			"nodes":    first,
		},
	})
	issuesBody := linearIssuesResponse(t, []map[string]any{node})
	moreBody, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"issue": map[string]any{
				"labels": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
					"nodes":    rest,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad body: %v", err)
			http.Error(w, "bad body", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(req.Query, "IssueLabels") {
			after, _ := req.Variables["after"].(string)
			if after != "lbl-more" {
				t.Errorf("IssueLabels after = %q, want lbl-more", after)
			}
			_, _ = w.Write(moreBody)
			return
		}
		_, _ = w.Write([]byte(issuesBody))
	}))
	t.Cleanup(srv.Close)
	db := newMirror(t)
	if _, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{LinearClient: testLinearClient(t, srv)}); err != nil {
		t.Fatal(err)
	}
	got := db.column(t, "issues", "labels", "FIX-30")
	if !strings.Contains(got, "label-1") || !strings.Contains(got, "label-60") {
		t.Fatalf("labels = %q, want inline page followed to label-60", got)
	}
}

func TestRunLinearPaginatesAttachmentsPastInlinePage(t *testing.T) {
	const extra = 10
	first := linearAttachmentNodes(linear.AttachmentsPageSize, 1)
	rest := linearAttachmentNodes(extra, linear.AttachmentsPageSize+1)
	issueID := "00000000-0000-4000-8000-99900000011"
	node := linearNode("FIX-31", "h", "unstarted", "Todo", 0, "No priority", map[string]any{
		"id": issueID,
		"attachments": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "att-more"},
			"nodes":    first,
		},
	})
	issuesBody := linearIssuesResponse(t, []map[string]any{node})
	moreBody, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"issue": map[string]any{
				"attachments": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
					"nodes":    rest,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad body: %v", err)
			http.Error(w, "bad body", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(req.Query, "IssueAttachments") {
			after, _ := req.Variables["after"].(string)
			if after != "att-more" {
				t.Errorf("IssueAttachments after = %q, want att-more", after)
			}
			_, _ = w.Write(moreBody)
			return
		}
		_, _ = w.Write([]byte(issuesBody))
	}))
	t.Cleanup(srv.Close)
	db := newMirror(t)
	if _, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{LinearClient: testLinearClient(t, srv)}); err != nil {
		t.Fatal(err)
	}
	detail, err := db.Detail(context.Background(), "FIX-31")
	if err != nil {
		t.Fatal(err)
	}
	want := linear.AttachmentsPageSize + extra
	if len(detail.Attachments) != want {
		t.Fatalf("attachments = %d, want %d (inline page followed to the end)", len(detail.Attachments), want)
	}
}

func TestSyncLinearIssuePaginatesLabelsAndAttachments(t *testing.T) {
	const extra = 10
	issueID := "00000000-0000-4000-8000-99900000012"
	iss := linearNode("FIX-32", "i", "unstarted", "Todo", 0, "No priority", map[string]any{
		"id": issueID,
		"labels": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "lbl-more"},
			"nodes":    linearLabelNodes(linear.LabelsPageSize, 1),
		},
		"attachments": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "att-more"},
			"nodes":    linearAttachmentNodes(linear.AttachmentsPageSize, 1),
		},
	})
	oneBody, err := json.Marshal(map[string]any{"data": map[string]any{"issue": iss}})
	if err != nil {
		t.Fatal(err)
	}
	labelsMore, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"issue": map[string]any{
				"labels": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
					"nodes":    linearLabelNodes(extra, linear.LabelsPageSize+1),
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	attsMore, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"issue": map[string]any{
				"attachments": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
					"nodes":    linearAttachmentNodes(extra, linear.AttachmentsPageSize+1),
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(req.Query, "IssueLabels"):
			_, _ = w.Write(labelsMore)
		case strings.Contains(req.Query, "IssueAttachments"):
			_, _ = w.Write(attsMore)
		default:
			_, _ = w.Write(oneBody)
		}
	}))
	t.Cleanup(srv.Close)
	db := newMirror(t)
	if err := db.UpsertSource(context.Background(), store.Source{ID: LinearSourceID, Kind: "linear"}); err != nil {
		t.Fatal(err)
	}
	if err := SyncLinearIssue(context.Background(), db.DB, testLinearClient(t, srv), "FIX-32"); err != nil {
		t.Fatal(err)
	}
	got := db.column(t, "issues", "labels", "FIX-32")
	if !strings.Contains(got, "label-60") {
		t.Errorf("labels = %q, want followed to label-60", got)
	}
	detail, err := db.Detail(context.Background(), "FIX-32")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Attachments) != linear.AttachmentsPageSize+extra {
		t.Fatalf("attachments = %d, want %d after SyncLinearIssue", len(detail.Attachments), linear.AttachmentsPageSize+extra)
	}
}

// TestRunLinearFullSyncDeletesAbsentKeys: full sync is proof-by-absence,
// matching the Jira reconcile pass. Linear's default listing omits archived
// issues, so archived is treated as deleted too.
func TestRunLinearFullSyncDeletesAbsentKeys(t *testing.T) {
	db := newMirror(t)
	seedLinearIssue(t, db, "FIX-GONE", "gone")
	seedJiraIssue(t, db, "NMB-KEEP")
	node := linearNode("FIX-24", "a", "unstarted", "Todo", 0, "No priority", nil)
	srv := linearGraphQL(t, map[string]string{"": linearIssuesResponse(t, []map[string]any{node})})
	t.Cleanup(srv.Close)
	res, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{Full: true, LinearClient: testLinearClient(t, srv)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted < 1 {
		t.Fatalf("deleted = %d, want at least the absent linear key", res.Deleted)
	}
	if _, missing := findLite(t, db, "FIX-GONE"); !missing {
		t.Fatal("FIX-GONE still mirrored after full sync — absence must delete (archived included)")
	}
	if _, missing := findLite(t, db, "NMB-KEEP"); missing {
		t.Fatal("Jira row was deleted by the Linear reconcile")
	}
	if _, missing := findLite(t, db, "FIX-24"); missing {
		t.Fatal("listed Linear issue was not upserted")
	}
}

func TestRunLinearIncrementalDoesNotDeleteAbsentKeys(t *testing.T) {
	keep := linearNode("FIX-25", "b", "unstarted", "Todo", 0, "No priority", nil)
	drop := linearNode("FIX-26", "c", "unstarted", "Todo", 0, "No priority", nil)
	srv1 := linearGraphQL(t, map[string]string{"": linearIssuesResponse(t, []map[string]any{keep, drop})})
	t.Cleanup(srv1.Close)
	db := newMirror(t)
	if _, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{LinearClient: testLinearClient(t, srv1)}); err != nil {
		t.Fatal(err)
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issues":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}`))
	}))
	t.Cleanup(srv2.Close)
	res, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{LinearClient: testLinearClient(t, srv2)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full {
		t.Fatal("second pass must be incremental")
	}
	if res.Deleted != 0 {
		t.Fatalf("incremental deleted = %d, want 0 (Jira does not reconcile on incremental)", res.Deleted)
	}
	if _, missing := findLite(t, db, "FIX-26"); missing {
		t.Fatal("incremental pass must not treat absence from the updatedAt window as deletion")
	}
}

func TestRunLinearFullSyncRefusesToEmptyMirror(t *testing.T) {
	db := newMirror(t)
	seedLinearIssue(t, db, "FIX-GONE", "gone")
	srv := linearGraphQL(t, map[string]string{"": linearIssuesResponse(t, nil)})
	t.Cleanup(srv.Close)
	_, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{Full: true, LinearClient: testLinearClient(t, srv)})
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("err = %v, want refuse-to-empty", err)
	}
	if _, missing := findLite(t, db, "FIX-GONE"); missing {
		t.Fatal("empty upstream must not wipe the linear mirror")
	}
}
