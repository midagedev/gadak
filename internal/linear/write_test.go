package linear

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The mutation tests. The response fixtures under testdata/issue_*.json and
// testdata/comment_create.json are hand-built to the documented mutation
// payload shape — no live mutation was allowed when the captures were taken
// (see testdata/README.md); the scrubbed vocabulary is reused so ids agree
// across files.

// ptr is the test-side address-of for the pointer-shaped optional fields.
func ptr[T any](v T) *T { return &v }

// decodeInput decodes variables.input of one GraphQL request into raw JSON,
// which is what lets a test assert the exact key set: omitted field vs
// explicit null vs value is the whole IssueUpdate contract.
func decodeInput(t *testing.T, r *http.Request, input *map[string]json.RawMessage) {
	t.Helper()
	var body struct {
		Query     string `json:"query"`
		Variables struct {
			ID    string          `json:"id"`
			Input json.RawMessage `json:"input"`
		} `json:"variables"`
	}
	decode(t, r, &body)
	if err := json.Unmarshal(body.Variables.Input, input); err != nil {
		t.Fatalf("variables.input: %v", err)
	}
}

func TestCreateIssueSendsInputAndReturnsIssue(t *testing.T) {
	var query string
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string `json:"query"`
			Variables struct {
				Input json.RawMessage `json:"input"`
			} `json:"variables"`
		}
		decode(t, r, &body)
		query = body.Query
		if err := json.Unmarshal(body.Variables.Input, &input); err != nil {
			t.Fatalf("variables.input: %v", err)
		}
		writeFixture(w, t, "issue_create.json")
	}))
	issue, err := c.CreateIssue(context.Background(), IssueCreate{
		TeamID:      "00000000-0000-4000-8000-000000000003",
		Title:       "Fixture issue 11",
		Description: "## Overview\n\nA fixture description.",
		StateID:     "00000000-0000-4000-8000-000000000008",
		Priority:    ptr(2),
		AssigneeID:  "00000000-0000-4000-8000-000000000015",
		LabelIDs:    []string{"00000000-0000-4000-8000-000000000013", "00000000-0000-4000-8000-000000000014"},
		DueDate:     "2026-09-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(query, "mutation IssueCreate") {
		t.Errorf("query = %q…, want the IssueCreate mutation document", query[:40])
	}
	if len(input) != 8 {
		t.Errorf("input has %d keys (%v), want exactly the 8 provided", len(input), input)
	}
	for field, want := range map[string]string{
		"teamId":     "00000000-0000-4000-8000-000000000003",
		"title":      "Fixture issue 11",
		"stateId":    "00000000-0000-4000-8000-000000000008",
		"assigneeId": "00000000-0000-4000-8000-000000000015",
		"dueDate":    "2026-09-30",
	} {
		var got string
		if err := json.Unmarshal(input[field], &got); err != nil || got != want {
			t.Errorf("input.%s = %s, want %q", field, input[field], want)
		}
	}
	var priority int
	if err := json.Unmarshal(input["priority"], &priority); err != nil || priority != 2 {
		t.Errorf("input.priority = %s, want the JSON number 2", input["priority"])
	}
	var labels []string
	if err := json.Unmarshal(input["labelIds"], &labels); err != nil || len(labels) != 2 || labels[0] != "00000000-0000-4000-8000-000000000013" {
		t.Errorf("input.labelIds = %s, want both label ids as a JSON array", input["labelIds"])
	}

	// The returned issue carries the full read-path field set (types.go Issue)
	// so the mirror can commit it without a refetch.
	if issue.Identifier != "FIX-11" || issue.Number != 11 {
		t.Errorf("issue = %s #%d, want FIX-11 #11", issue.Identifier, issue.Number)
	}
	if issue.State.Type != "started" || issue.State.ID != "00000000-0000-4000-8000-000000000008" {
		t.Errorf("state = %+v, want the In Progress state by id", issue.State)
	}
	if issue.Priority != 2 || issue.PriorityLabel != "High" {
		t.Errorf("priority = %d %q, want 2 High", issue.Priority, issue.PriorityLabel)
	}
	if issue.DueDate != "2026-09-30" {
		t.Errorf("dueDate = %q, want the created date", issue.DueDate)
	}
	if issue.Assignee == nil || issue.Assignee.ID != "00000000-0000-4000-8000-000000000015" {
		t.Errorf("assignee = %+v, want the fixture assignee", issue.Assignee)
	}
	if len(issue.Labels.Nodes) != 2 || issue.Labels.Nodes[0].Name != "Bug" {
		t.Errorf("labels = %+v, want both fixture labels", issue.Labels.Nodes)
	}
	if issue.Team.Key != "FIX" || issue.URL == "" {
		t.Errorf("team/url = %+v %q, want the fixture team and a URL", issue.Team, issue.URL)
	}
}

func TestCreateIssueValidatesInputBeforeSending(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	cases := []struct {
		name string
		in   IssueCreate
		want string
	}{
		{"missing teamId", IssueCreate{Title: "t"}, "teamId"},
		{"missing title", IssueCreate{TeamID: "team"}, "title"},
		{"priority above scale", IssueCreate{TeamID: "t", Title: "t", Priority: ptr(5)}, "priority"},
		{"priority negative", IssueCreate{TeamID: "t", Title: "t", Priority: ptr(-1)}, "priority"},
		{"dueDate not zero-padded", IssueCreate{TeamID: "t", Title: "t", DueDate: "2026-9-30"}, "dueDate"},
		{"dueDate datetime form", IssueCreate{TeamID: "t", Title: "t", DueDate: "2026-09-30T00:00:00Z"}, "dueDate"},
	}
	for _, tc := range cases {
		_, err := c.CreateIssue(context.Background(), tc.in)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to name %q", tc.name, err, tc.want)
		}
	}
	if calls != 0 {
		t.Errorf("validation must fail before any HTTP call, got %d calls", calls)
	}
}

func TestUpdateIssueOmitsUnsetFieldsAndClearsAssignee(t *testing.T) {
	var query string
	var idVar string
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string `json:"query"`
			Variables struct {
				ID    string          `json:"id"`
				Input json.RawMessage `json:"input"`
			} `json:"variables"`
		}
		decode(t, r, &body)
		query, idVar = body.Query, body.Variables.ID
		if err := json.Unmarshal(body.Variables.Input, &input); err != nil {
			t.Fatalf("variables.input: %v", err)
		}
		writeFixture(w, t, "issue_update.json")
	}))
	issue, err := c.UpdateIssue(context.Background(), "00000000-0000-4000-8000-000000000004", IssueUpdate{
		Title:      ptr("Renamed by update"),
		AssigneeID: ptr(""), // empty string = explicit null = unassign
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(query, "mutation IssueUpdate") {
		t.Errorf("query = %q…, want the IssueUpdate mutation document", query[:40])
	}
	if idVar != "00000000-0000-4000-8000-000000000004" {
		t.Errorf("variables.id = %q, want the issue id", idVar)
	}
	if len(input) != 2 {
		t.Errorf("input has %d keys (%v), want only the 2 set fields", len(input), input)
	}
	if raw := string(input["assigneeId"]); raw != "null" {
		t.Errorf("input.assigneeId = %s, want explicit JSON null — the documented schema clears the field on null", raw)
	}
	var title string
	if err := json.Unmarshal(input["title"], &title); err != nil || title != "Renamed by update" {
		t.Errorf("input.title = %s, want the new title", input["title"])
	}
	for _, absent := range []string{"description", "stateId", "priority", "labelIds", "dueDate"} {
		if raw, ok := input[absent]; ok {
			t.Errorf("input.%s = %s, want the key omitted (nil means unchanged)", absent, raw)
		}
	}
	if issue.Identifier != "FIX-2" || issue.Title != "Renamed by update" {
		t.Errorf("issue = %s %q, want FIX-2 with the new title", issue.Identifier, issue.Title)
	}
	if issue.Assignee != nil {
		t.Errorf("assignee = %+v, want nil — the update cleared it", issue.Assignee)
	}
}

func TestUpdateIssueSendsAllSetFields(t *testing.T) {
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeInput(t, r, &input)
		writeFixture(w, t, "issue_update.json")
	}))
	_, err := c.UpdateIssue(context.Background(), "00000000-0000-4000-8000-000000000004", IssueUpdate{
		Title:       ptr("All fields"),
		Description: ptr("body markdown"),
		StateID:     ptr("00000000-0000-4000-8000-000000000010"),
		Priority:    ptr(3),
		AssigneeID:  ptr("00000000-0000-4000-8000-000000000015"),
		LabelIDs:    ptr([]string{"00000000-0000-4000-8000-000000000013"}),
		DueDate:     ptr("2026-10-15"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(input) != 7 {
		t.Errorf("input has %d keys (%v), want all 7 set fields", len(input), input)
	}
	var priority int
	if err := json.Unmarshal(input["priority"], &priority); err != nil || priority != 3 {
		t.Errorf("input.priority = %s, want the JSON number 3", input["priority"])
	}
	var assignee string
	if err := json.Unmarshal(input["assigneeId"], &assignee); err != nil || assignee != "00000000-0000-4000-8000-000000000015" {
		t.Errorf("input.assigneeId = %s, want the user id", input["assigneeId"])
	}
	var labels []string
	if err := json.Unmarshal(input["labelIds"], &labels); err != nil || len(labels) != 1 {
		t.Errorf("input.labelIds = %s, want a one-element JSON array", input["labelIds"])
	}
	var due string
	if err := json.Unmarshal(input["dueDate"], &due); err != nil || due != "2026-10-15" {
		t.Errorf("input.dueDate = %s, want 2026-10-15", input["dueDate"])
	}
}

func TestUpdateIssueValidatesInputBeforeSending(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	cases := []struct {
		name string
		id   string
		in   IssueUpdate
		want string
	}{
		{"missing id", "", IssueUpdate{Title: ptr("t")}, "id"},
		{"priority above scale", "i", IssueUpdate{Priority: ptr(9)}, "priority"},
		{"bad dueDate", "i", IssueUpdate{DueDate: ptr("tomorrow")}, "dueDate"},
	}
	for _, tc := range cases {
		_, err := c.UpdateIssue(context.Background(), tc.id, tc.in)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to name %q", tc.name, err, tc.want)
		}
	}
	if calls != 0 {
		t.Errorf("validation must fail before any HTTP call, got %d calls", calls)
	}
}

// An all-nil update is a legal no-op on the wire (Linear answers success with
// the unchanged issue), the same way jira's EditIssue tolerates empty maps.
// The point of pinning it: an empty input must be {} on the wire, not a pile
// of nulls that would clear every optional field.
func TestUpdateIssueEmptyInputIsAnEmptyObject(t *testing.T) {
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeInput(t, r, &input)
		writeFixture(w, t, "issue_update.json")
	}))
	if _, err := c.UpdateIssue(context.Background(), "00000000-0000-4000-8000-000000000004", IssueUpdate{}); err != nil {
		t.Fatal(err)
	}
	if len(input) != 0 {
		t.Errorf("input = %v, want {} — nil fields are omitted, never nulled", input)
	}
}

func TestCreateCommentSendsInputAndReturnsComment(t *testing.T) {
	var query string
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string `json:"query"`
			Variables struct {
				Input json.RawMessage `json:"input"`
			} `json:"variables"`
		}
		decode(t, r, &body)
		query = body.Query
		if err := json.Unmarshal(body.Variables.Input, &input); err != nil {
			t.Fatalf("variables.input: %v", err)
		}
		writeFixture(w, t, "comment_create.json")
	}))
	comment, err := c.CreateComment(context.Background(), "00000000-0000-4000-8000-000000000004", "A fixture comment in markdown shape.")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(query, "mutation CommentCreate") {
		t.Errorf("query = %q…, want the CommentCreate mutation document", query[:40])
	}
	if len(input) != 2 {
		t.Errorf("input has %d keys (%v), want exactly issueId and body", len(input), input)
	}
	for field, want := range map[string]string{
		"issueId": "00000000-0000-4000-8000-000000000004",
		"body":    "A fixture comment in markdown shape.",
	} {
		var got string
		if err := json.Unmarshal(input[field], &got); err != nil || got != want {
			t.Errorf("input.%s = %s, want %q", field, input[field], want)
		}
	}
	if comment.ID != "00000000-0000-4000-8000-000000000016" || comment.Body != "A fixture comment in markdown shape." {
		t.Errorf("comment = %+v, want the fixture comment", comment)
	}
	if comment.User == nil || comment.User.ID != "00000000-0000-4000-8000-000000000011" {
		t.Errorf("comment.User = %+v, want the viewer-shaped author (the API key owner)", comment.User)
	}
}

// Linear mutations answer success=false for application-level rejections that
// do not take the GraphQL errors array. Every verb must turn that into an
// error, or a rejected write would read as a silent no-op.
func TestMutationSuccessFalseIsAnError(t *testing.T) {
	verbs := []struct {
		name string
		call func(c *Client) error
	}{
		{"issueCreate", func(c *Client) error {
			_, err := c.CreateIssue(context.Background(), IssueCreate{TeamID: "t", Title: "x"})
			return err
		}},
		{"issueUpdate", func(c *Client) error {
			_, err := c.UpdateIssue(context.Background(), "i", IssueUpdate{Title: ptr("x")})
			return err
		}},
		{"commentCreate", func(c *Client) error {
			_, err := c.CreateComment(context.Background(), "i", "b")
			return err
		}},
	}
	for _, v := range verbs {
		c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Query string `json:"query"`
			}
			decode(t, r, &body)
			field := strings.TrimSpace(strings.Split(strings.Split(body.Query, "{")[1], "(")[0])
			w.Write([]byte(`{"data":{"` + field + `":{"success":false}}}`))
		}))
		err := v.call(c)
		if err == nil || !strings.Contains(err.Error(), "success=false") {
			t.Errorf("%s: err = %v, want a success=false error", v.name, err)
		}
	}
}

// Mutations ride the same transport as reads: a 429 is retried, and the usage
// counters and rate-limit headers are noted on the mutation path exactly as
// TestRetriesThenSucceeds pins them for reads.
func TestMutationRetries429AndRecordsUsage(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("X-RateLimit-Requests-Remaining", "2490")
		w.Header().Set("X-RateLimit-Requests-Limit", "2500")
		writeFixture(w, t, "comment_create.json")
	}))
	if _, err := c.CreateComment(context.Background(), "00000000-0000-4000-8000-000000000004", "after throttle"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("attempts = %d, want 2 (throttle then success)", calls)
	}
	u := c.Usage()
	if u.Requests != 2 || u.Retries != 1 || u.Throttled != 1 {
		t.Errorf("usage = %+v, want 2 requests / 1 retry / 1 throttled", u)
	}
	rl := c.LastRateLimit()
	if rl.RequestsRemaining != 2490 || rl.RequestsLimit != 2500 {
		t.Errorf("rate limit = %+v, want the parsed headers of the mutation response", rl)
	}
}

// The write retry policy is narrower than the read one, for the reason
// jira's write() records: a 500 may mean Linear already applied the mutation
// and the answer was lost, so a retry could post it twice. 429 and 503 — the
// server stating it did not act — are the only retried statuses.
func TestMutationDoesNotRetry500(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, `{"errors":[{"message":"internal"}]}`, http.StatusInternalServerError)
	}))
	if _, err := c.CreateComment(context.Background(), "00000000-0000-4000-8000-000000000004", "once only"); err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("attempts = %d, want 1 — a 500 on a mutation must not be retried", calls)
	}
}

func TestUploadFileReturnsTargetWithHeaders(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		decode(t, r, &body)
		if body.Variables["filename"] != "log.txt" || body.Variables["size"] != float64(42) {
			t.Fatalf("variables: %v", body.Variables)
		}
		w.Write([]byte(`{"data":{"fileUpload":{"success":true,"uploadFile":{
			"uploadUrl":"https://storage.example/put","assetUrl":"https://uploads.example/a/log.txt",
			"headers":[{"key":"x-amz-meta-a","value":"b"}]}}}}`))
	}))
	up, err := c.UploadFile(context.Background(), "log.txt", "text/plain", 42)
	if err != nil {
		t.Fatal(err)
	}
	if up.AssetURL != "https://uploads.example/a/log.txt" || up.Headers["x-amz-meta-a"] != "b" {
		t.Fatalf("target: %+v", up)
	}
	if _, err := c.UploadFile(context.Background(), "", "text/plain", 42); err == nil ||
		!strings.Contains(err.Error(), "required") {
		t.Fatalf("want validation error, got %v", err)
	}
}

func TestCreateAttachmentSendsInput(t *testing.T) {
	var input map[string]json.RawMessage
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeInput(t, r, &input)
		w.Write([]byte(`{"data":{"attachmentCreate":{"success":true,"attachment":{
			"id":"00000000-0000-4000-8000-0000000000aa","title":"log.txt","url":"https://uploads.example/a/log.txt"}}}}`))
	}))
	att, err := c.CreateAttachment(context.Background(), "iss-1", "https://uploads.example/a/log.txt", "log.txt")
	if err != nil || att.ID == "" {
		t.Fatalf("attachment: %+v err=%v", att, err)
	}
	if string(input["issueId"]) != `"iss-1"` || string(input["title"]) != `"log.txt"` {
		t.Fatalf("input: %v", input)
	}
	if _, err := c.CreateAttachment(context.Background(), "", "u", ""); err == nil {
		t.Fatal("want validation error")
	}
}
