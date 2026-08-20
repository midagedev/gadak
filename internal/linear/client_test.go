package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testKey stands where the real API key would. It is deliberately not
// key-shaped (no lin_api_ prefix) so no scanner ever has an opinion about it.
const testKey = "linear-test-key-not-a-real-secret"

func testClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(testKey)
	c.Endpoint = srv.URL
	c.Retries, c.Backoff = 4, 0
	return c
}

func decode(t *testing.T, r *http.Request, out any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatalf("bad request body: %v", err)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

func writeFixture(w http.ResponseWriter, t *testing.T, name string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.Write(fixture(t, name))
}

// The Authorization header is the bare key. Linear answers "Bearer <key>"
// with a 400 that tells you to remove the prefix (measured 2026-08-18), so a
// regression to the prefixed form breaks every call — this pins the shape.
func TestAuthHeaderIsBareKey(t *testing.T) {
	var got, contentType string
	var method, path string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		method, path = r.Method, r.URL.Path
		writeFixture(w, t, "viewer.json")
	}))
	if _, err := c.Viewer(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != testKey {
		// Print the length, not the value: if this ever fails against a real
		// credential, the failure message must not become the leak.
		t.Errorf("Authorization header is not exactly the API key (got length %d)", len(got))
	}
	if strings.HasPrefix(got, "Bearer ") {
		t.Error("Authorization must not carry a Bearer prefix — Linear rejects it with 400")
	}
	if method != http.MethodPost || path != "/" {
		// The test server has no path; production posts to Endpoint. What
		// matters here is the verb and that nothing else was contacted.
		t.Errorf("request = %s %s, want POST to the configured endpoint", method, path)
	}
	if !strings.HasSuffix(Endpoint, "/graphql") || !strings.HasPrefix(Endpoint, "https://api.linear.app/") {
		t.Errorf("Endpoint = %q, want the Linear GraphQL URL", Endpoint)
	}
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
}

// Page 1 of the real capture ends hasNextPage with a cursor; page 2 was
// captured with exactly that cursor. The client must join them in order.
func TestIssuesFollowsCursorAcrossPages(t *testing.T) {
	calls := 0
	var seen []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Variables struct {
				After string `json:"after"`
			} `json:"variables"`
		}
		decode(t, r, &body)
		switch body.Variables.After {
		case "":
			writeFixture(w, t, "issues_page1.json")
		case "00000000-0000-4000-8000-000000000000":
			writeFixture(w, t, "issues_page2.json")
		default:
			t.Errorf("unexpected cursor %q", body.Variables.After)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	err := c.Issues(context.Background(), IssueOpts{}, func(pg []Issue) error {
		for _, iss := range pg {
			seen = append(seen, iss.Identifier)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	want := []string{"FIX-1", "FIX-4", "FIX-2", "FIX-3"}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Errorf("identifiers = %v, want %v in page order", seen, want)
	}
	if u := c.Usage(); u.Requests != 2 {
		t.Errorf("Usage.Requests = %d, want 2", u.Requests)
	}
}

// The watermark and team scope must travel as filter variables — the
// incremental-sync contract (updatedAt.gte) and the never-key-on-display-name
// rule (team.id, not team key) both live here.
func TestIssuesSendsWatermarkFilterAndTeamScope(t *testing.T) {
	var vars struct {
		First           int  `json:"first"`
		IncludeArchived bool `json:"includeArchived"`
		Filter          struct {
			UpdatedAt struct {
				Gte string `json:"gte"`
			} `json:"updatedAt"`
			Team struct {
				ID struct {
					Eq string `json:"eq"`
				} `json:"id"`
			} `json:"team"`
		} `json:"filter"`
	}
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables json.RawMessage `json:"variables"`
		}
		decode(t, r, &body)
		if err := json.Unmarshal(body.Variables, &vars); err != nil {
			t.Fatalf("variables: %v", err)
		}
		writeFixture(w, t, "issues_page2.json") // single final page
	}))
	err := c.Issues(context.Background(), IssueOpts{
		TeamID:          "team-uuid-from-teams-call",
		UpdatedAfter:    "2026-08-01T00:00:00.000Z",
		IncludeArchived: true,
	}, func([]Issue) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if vars.Filter.UpdatedAt.Gte != "2026-08-01T00:00:00.000Z" {
		t.Errorf("filter.updatedAt.gte = %q, want the watermark", vars.Filter.UpdatedAt.Gte)
	}
	if vars.Filter.Team.ID.Eq != "team-uuid-from-teams-call" {
		t.Errorf("filter.team.id.eq = %q, want the team id", vars.Filter.Team.ID.Eq)
	}
	if !vars.IncludeArchived {
		t.Error("includeArchived must be passed through for reconcile passes")
	}
	if vars.First != defaultPageSize {
		t.Errorf("first = %d, want default %d", vars.First, defaultPageSize)
	}
}

func TestIssuesClampsPageSize(t *testing.T) {
	var first int
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables struct {
				First int `json:"first"`
			} `json:"variables"`
		}
		decode(t, r, &body)
		first = body.Variables.First
		writeFixture(w, t, "issues_page2.json")
	}))
	err := c.Issues(context.Background(), IssueOpts{PageSize: 1000}, func([]Issue) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if first != maxPageSize {
		t.Errorf("first = %d, want clamp to %d", first, maxPageSize)
	}
}

func TestAuthFailureBecomesErrAuthWithoutRetry(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		calls := 0
		c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(status)
			w.Write([]byte(`{"errors":[{"message":"invalid api key"}]}`))
		}))
		_, err := c.Teams(context.Background())
		if !errors.Is(err, ErrAuth) {
			t.Fatalf("status %d: err = %v, want ErrAuth", status, err)
		}
		if calls != 1 {
			t.Errorf("status %d: attempts = %d, want 1 — a bad credential must not burn the rate budget", status, calls)
		}
	}
}

// TestErrAuthSatisfiesRejectedCredential pins the duck-typed marker
// IsRejectedCredential already publishes. This package must not import
// atlhttp (Linear is not an Atlassian host), so the assertion uses a local
// copy of the method set. errors.Is(err, ErrAuth) must keep working.
func TestErrAuthSatisfiesRejectedCredential(t *testing.T) {
	if ErrAuth.Error() != "linear: credential rejected" {
		t.Fatalf("ErrAuth = %q, want the existing linear: prefix so last_error names the source", ErrAuth)
	}
	wrapped := fmt.Errorf("POST /graphql: %w (401 Unauthorized)", ErrAuth)
	if !errors.Is(wrapped, ErrAuth) {
		t.Fatalf("errors.Is(%v, ErrAuth) = false", wrapped)
	}
	var rc interface{ RejectedCredential() }
	if !errors.As(ErrAuth, &rc) {
		t.Fatal("ErrAuth must implement RejectedCredential so IsRejectedCredential sees it without a sync branch")
	}
	if !errors.As(wrapped, &rc) {
		t.Fatal("wrapped ErrAuth must still implement RejectedCredential")
	}
}

// The credential-leak assertion. Article 8 (same as jira): the key travels
// only in the Authorization header; no error string may echo it. Every error
// path this package can produce is checked.
func TestErrorsNeverContainCredential(t *testing.T) {
	newErr := func(h http.Handler) error {
		c := testClient(t, h)
		_, err := c.Teams(context.Background())
		return err
	}
	cases := map[string]http.Handler{
		"auth rejected": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"errors":[{"message":"unauthorized"}]}`, http.StatusUnauthorized)
		}),
		"server error after retries": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}),
		"graphql error envelope": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"errors":[{"message":"It looks like you're trying to use an API key as a Bearer token."}]}`))
		}),
		"bad json": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`not json`))
		}),
	}
	for name, h := range cases {
		err := newErr(h)
		if err == nil {
			t.Errorf("%s: expected an error", name)
			continue
		}
		if strings.Contains(err.Error(), testKey) {
			t.Errorf("%s: error message leaked the credential", name)
		}
	}
	// Transport failure: the endpoint is unreachable. The wrapped error names
	// the method and URL context; it must still not echo the key.
	c := New(testKey)
	c.Endpoint = "http://127.0.0.1:0/graphql" // port 0: nothing listens
	c.Retries, c.Backoff = 2, 0
	if _, err := c.Viewer(context.Background()); err == nil || !strings.Contains(err.Error(), "POST /graphql") {
		t.Fatalf("transport error = %v, want a wrapped POST /graphql error", err)
	} else if strings.Contains(err.Error(), testKey) {
		t.Error("transport error leaked the credential")
	}
	// And the endpoint itself is not the key: the default is a constant.
	if strings.Contains(Endpoint, testKey) {
		t.Error("Endpoint embeds the test key")
	}
}

func TestRetriesThenSucceeds(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("X-RateLimit-Requests-Remaining", "2497")
		w.Header().Set("X-RateLimit-Requests-Limit", "2500")
		writeFixture(w, t, "teams.json")
	}))
	teams, err := c.Teams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("attempts = %d, want 3", calls)
	}
	if len(teams) != 1 || teams[0].Key != "FIX" {
		t.Errorf("teams = %+v, want the fixture team", teams)
	}
	u := c.Usage()
	if u.Requests != 3 || u.Retries != 2 || u.Throttled != 2 {
		t.Errorf("usage = %+v, want 3 requests / 2 retries / 2 throttled", u)
	}
	rl := c.LastRateLimit()
	if rl.RequestsRemaining != 2497 || rl.RequestsLimit != 2500 {
		t.Errorf("rate limit = %+v, want the parsed headers", rl)
	}
}

func TestUsageRecordsWaitMSAndLastThrottledAt(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeFixture(w, t, "teams.json")
	}))
	// Retry-After "0" is ignored (s > 0 required), so Wait uses Backoff.
	c.Backoff = time.Millisecond

	if _, err := c.Teams(context.Background()); err != nil {
		t.Fatal(err)
	}
	u := c.Usage()
	if u.Requests != 2 || u.Throttled != 1 || u.Retries != 1 {
		t.Errorf("usage = %+v, want 2 requests / 1 throttle / 1 retry", u)
	}
	if u.WaitMS <= 0 {
		t.Errorf("WaitMS = %d, want > 0 — linear must record the same wait as atlhttp", u.WaitMS)
	}
	if u.LastThrottledAt.IsZero() || u.LastThrottledAt.Location() != time.UTC {
		t.Errorf("LastThrottledAt = %v, want non-zero UTC", u.LastThrottledAt)
	}
	taken := c.TakeUsage()
	after := c.Usage()
	if after.Requests != 0 || after.Throttled != 0 || after.Retries != 0 || after.WaitMS != 0 {
		t.Errorf("counters not zeroed after TakeUsage: %+v", after)
	}
	if after.LastThrottledAt.IsZero() || !after.LastThrottledAt.Equal(taken.LastThrottledAt) {
		t.Errorf("LastThrottledAt must survive TakeUsage: taken=%v after=%v", taken.LastThrottledAt, after.LastThrottledAt)
	}
}

func TestGraphQLErrorsSurfaceAsErrors(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errors":[{"message":"Something went wrong."},{"message":"Second message."}]}`))
	}))
	_, err := c.Viewer(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Something went wrong.") || !strings.Contains(err.Error(), "Second message.") {
		t.Errorf("err = %v, want both graphql messages", err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Error("graphql error leaked the credential")
	}
}

func TestFixtureParses(t *testing.T) {
	t.Run("viewer", func(t *testing.T) {
		c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeFixture(w, t, "viewer.json")
		}))
		v, err := c.Viewer(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if v.ID == "" || v.Email == "" || v.Name == "" {
			t.Errorf("viewer = %+v, want id/name/email populated", v)
		}
	})
	t.Run("workflowstates carries the duplicate type", func(t *testing.T) {
		var teamFilter string
		c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Variables struct {
					Filter struct {
						Team struct {
							ID struct {
								Eq string `json:"eq"`
							} `json:"id"`
						} `json:"team"`
					} `json:"filter"`
				} `json:"variables"`
			}
			decode(t, r, &body)
			teamFilter = body.Variables.Filter.Team.ID.Eq
			writeFixture(w, t, "workflowstates.json")
		}))
		states, err := c.WorkflowStates(context.Background(), "00000000-0000-4000-8000-000000000003")
		if err != nil {
			t.Fatal(err)
		}
		if teamFilter != "00000000-0000-4000-8000-000000000003" {
			t.Errorf("team filter = %q, want the requested team id", teamFilter)
		}
		types := map[string]bool{}
		for _, s := range states {
			types[s.Type] = true
		}
		for _, want := range []string{"backlog", "unstarted", "started", "completed", "canceled", "duplicate"} {
			if !types[want] {
				t.Errorf("state type %q missing from catalog %v", want, types)
			}
		}
	})
	t.Run("issue fields round-trip", func(t *testing.T) {
		c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeFixture(w, t, "issues_page2.json") // final page, loop ends
		}))
		var got []Issue
		if err := c.Issues(context.Background(), IssueOpts{}, func(pg []Issue) error {
			got = append(got, pg...)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("issues = %d, want 2", len(got))
		}
		one := got[0]
		if one.Identifier != "FIX-2" || one.State.Type != "unstarted" || one.State.Name != "Todo" {
			t.Errorf("issue = %s state %+v, want FIX-2 on the Todo/unstarted state", one.Identifier, one.State)
		}
		if one.Team.Key != "FIX" || one.Team.ID != "00000000-0000-4000-8000-000000000003" {
			t.Errorf("team = %+v, want the fixture team with the id shared across files", one.Team)
		}
		if one.Priority != 0 || one.PriorityLabel != "No priority" {
			t.Errorf("priority = %d %q, want 0 No priority", one.Priority, one.PriorityLabel)
		}
		if one.CreatedAt != "2026-08-18T13:18:31.131Z" || one.UpdatedAt == "" {
			t.Errorf("timestamps = %q / %q, want verbatim ISO-8601", one.CreatedAt, one.UpdatedAt)
		}
		if !strings.Contains(one.Description, "## Overview") || !strings.Contains(one.URL, "linear.app/example/issue/FIX-") {
			t.Errorf("description/url lost the scrubbed fixture shape")
		}
	})
}

// The nested comments connection must expose its own pageInfo so an issue
// with more comments than the inline page is observable, never silently
// truncated. Synthetic vector: the capture workspace has no comment-heavy
// issue, so the envelope is hand-built to the verified shape.
func TestCommentsTruncationIsObservable(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"issues":{"pageInfo":{"hasNextPage":false},"nodes":[{` +
			`"id":"c1","identifier":"FIX-9","comments":{` +
			`"pageInfo":{"hasNextPage":true,"endCursor":"cmt-cursor"},` +
			`"nodes":[{"id":"c0","body":"body markdown","createdAt":"2026-08-18T13:18:31.131Z","updatedAt":"2026-08-18T13:18:31.131Z","user":{"id":"u0","name":"Fixture User 9","displayName":"Fix User 9"}}]}}]}}}`))
	}))
	var got []Issue
	if err := c.Issues(context.Background(), IssueOpts{}, func(pg []Issue) error {
		got = pg
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("issues = %d, want 1", len(got))
	}
	cm := got[0].Comments
	if len(cm.Nodes) != 1 || cm.Nodes[0].Body != "body markdown" {
		t.Fatalf("comments = %+v, want the parsed node", cm.Nodes)
	}
	if !cm.PageInfo.HasNextPage || cm.PageInfo.EndCursor != "cmt-cursor" {
		t.Errorf("comments pageInfo = %+v, want truncation visible", cm.PageInfo)
	}
}

func TestCompleteCommentsFollowsCursor(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Query     string `json:"query"`
			Variables struct {
				ID    string `json:"id"`
				After string `json:"after"`
			} `json:"variables"`
		}
		decode(t, r, &body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(body.Query, "IssueComments") {
			if body.Variables.ID != "iss-1" || body.Variables.After != "cmt-cursor" {
				t.Errorf("IssueComments id/after = %q %q", body.Variables.ID, body.Variables.After)
			}
			_, _ = w.Write([]byte(`{"data":{"issue":{"comments":{"pageInfo":{"hasNextPage":false},"nodes":[{"id":"c1","body":"second"}]}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"issue":{"id":"iss-1","identifier":"FIX-9","comments":{"pageInfo":{"hasNextPage":true,"endCursor":"cmt-cursor"},"nodes":[{"id":"c0","body":"first"}]}}}}`))
	}))
	iss, err := c.Issue(context.Background(), "FIX-9")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.CompleteComments(context.Background(), &iss); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if len(iss.Comments.Nodes) != 2 || iss.Comments.Nodes[1].Body != "second" {
		t.Fatalf("comments = %+v", iss.Comments.Nodes)
	}
	if iss.Comments.PageInfo.HasNextPage {
		t.Error("HasNextPage still set after following the cursor")
	}
}

func TestIssuesQueryRequestsAttachments(t *testing.T) {
	var query string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		decode(t, r, &body)
		query = body.Query
		writeFixture(w, t, "issues_page2.json")
	}))
	if err := c.Issues(context.Background(), IssueOpts{}, func([]Issue) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "attachments") {
		t.Fatal("issues query must request attachments")
	}
	if !strings.Contains(query, "metadata") {
		t.Fatal("attachments selection must ask for metadata (size/mime)")
	}
}

// GDK-406: assignee search must match email as well as display name.
// The previous filter was displayName-only, so `gadak assign KEY user@host`
// missed Linear accounts whose visible name is not their mailbox.
func TestUsersFilterMatchesDisplayNameOrEmail(t *testing.T) {
	var filter map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables struct {
				Filter map[string]any `json:"filter"`
			} `json:"variables"`
		}
		decode(t, r, &body)
		filter = body.Variables.Filter
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"users":{"nodes":[]}}}`))
	}))
	if _, err := c.Users(context.Background(), "dana@example.com"); err != nil {
		t.Fatal(err)
	}
	or, _ := filter["or"].([]any)
	if len(or) != 2 {
		t.Fatalf("filter = %#v, want or:[displayName, email]", filter)
	}
	gotName, gotEmail := false, false
	for _, raw := range or {
		clause, _ := raw.(map[string]any)
		if dn, ok := clause["displayName"].(map[string]any); ok {
			if dn["containsIgnoreCase"] == "dana@example.com" {
				gotName = true
			}
		}
		if em, ok := clause["email"].(map[string]any); ok {
			if em["containsIgnoreCase"] == "dana@example.com" {
				gotEmail = true
			}
		}
	}
	if !gotName || !gotEmail {
		t.Fatalf("filter or clauses = %#v, want displayName OR email containsIgnoreCase", or)
	}
}
