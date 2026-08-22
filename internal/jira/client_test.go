package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/atlhttp"
)

func testClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "someone@example.com", "secret-token")
	c.Retries, c.Backoff = 4, 0
	return c
}

func TestNewProductionRetryBudget(t *testing.T) {
	if DefaultRetries != 5 || DefaultBackoff != time.Second {
		t.Fatalf("defaults Retries=%d Backoff=%s, want 5 and 1s", DefaultRetries, DefaultBackoff)
	}
	c := New("https://example.atlassian.net", "a@b.c", "tok")
	if c.Retries != 5 || c.Backoff != time.Second {
		t.Fatalf("New Retries=%d Backoff=%s, want 5 and 1s", c.Retries, c.Backoff)
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
		w.Write([]byte(`[{"id":"1","name":"Highest"},{"id":"2","name":"Low"}]`))
	}))
	got, err := c.Priorities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("attempts = %d, want 3", calls)
	}
	if len(got) != 2 || got[0] != "Highest" {
		t.Errorf("priorities = %v", got)
	}
}

func TestAuthFailureAbortsWithoutRetry(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, `{"errorMessages":["Client must be authenticated"]}`, http.StatusUnauthorized)
	}))
	_, err := c.Statuses(context.Background())
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("err = %v, want ErrAuth", err)
	}
	if calls != 1 {
		t.Errorf("attempts = %d, want 1 — a bad credential must not burn the rate budget", calls)
	}
	// Article 8: the token never reaches an error string.
	if strings.Contains(err.Error(), "secret-token") {
		t.Error("error message leaked the credential")
	}
}

func TestAuthForbiddenAbortsWithoutRetry(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, `{"errorMessages":["forbidden"]}`, http.StatusForbidden)
	}))
	_, err := c.Statuses(context.Background())
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("err = %v, want ErrAuth", err)
	}
	if calls != 1 {
		t.Errorf("attempts = %d, want 1", calls)
	}
}

// TestErrAuthMatchesCallerIdiom pins errors.Is(err, jira.ErrAuth) used by
// internal/server/onboarding.go, internal/server/write.go, and cmd/gadak/init.go.
func TestErrAuthMatchesCallerIdiom(t *testing.T) {
	wrapped := fmt.Errorf("GET /rest/api/3/myself: %w (401 Unauthorized)", ErrAuth)
	if !errors.Is(wrapped, ErrAuth) {
		t.Fatalf("errors.Is(%v, jira.ErrAuth) = false", wrapped)
	}
	if !errors.Is(wrapped, atlhttp.ErrAuth) {
		t.Fatalf("errors.Is(%v, atlhttp.ErrAuth) = false", wrapped)
	}
	if !strings.Contains(ErrAuth.Error(), "jira:") {
		t.Fatalf("ErrAuth = %q, want the jira: prefix so last_error distinguishes the source", ErrAuth)
	}
	if wrapped.Error() == ErrAuth.Error() {
		t.Fatal("wrapped error must keep method/path so last_error names the call")
	}
}

func TestCountApproximate(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/approximate-count" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			JQL string `json:"jql"`
		}
		decode(t, r, &body)
		if body.JQL != `project = "NMB"` {
			t.Errorf("jql = %q", body.JQL)
		}
		w.Write([]byte(`{"count":6543}`))
	}))
	n, err := c.Count(context.Background(), `project = "NMB"`)
	if err != nil {
		t.Fatal(err)
	}
	if n != 6543 {
		t.Errorf("count = %d, want 6543", n)
	}
	if u := c.Usage(); u.Requests != 1 {
		t.Errorf("Count must go through do() so usage counts it: Requests=%d", u.Requests)
	}
}

func TestSearchFollowsPageTokens(t *testing.T) {
	var seen []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			NextPageToken string   `json:"nextPageToken"`
			Fields        []string `json:"fields"`
			Expand        string   `json:"expand"`
		}
		decode(t, r, &body)
		seen = append(seen, body.NextPageToken)
		if body.Expand != "changelog" {
			t.Errorf("expand = %q, want changelog", body.Expand)
		}
		if len(body.Fields) == 0 {
			t.Error("fields must be explicit, never *all")
		}
		switch body.NextPageToken {
		case "":
			w.Write([]byte(`{"issues":[{"id":"1","key":"A-1","fields":{"summary":"one"}}],"nextPageToken":"tok2"}`))
		case "tok2":
			w.Write([]byte(`{"issues":[{"id":"2","key":"A-2","fields":{"summary":"two"}}],"isLast":true}`))
		default:
			t.Errorf("unexpected token %q", body.NextPageToken)
		}
	}))

	var keys []string
	pages := 0
	err := c.Search(context.Background(), "project = A", []string{"summary"}, true, func(issues []Issue) error {
		pages++
		for _, i := range issues {
			keys = append(keys, i.Key)
			if i.Fields.Summary == "" {
				t.Errorf("%s: fields not decoded", i.Key)
			}
			if len(i.Raw) == 0 || i.Extra == nil {
				t.Errorf("%s: raw payload and per-field map must both survive", i.Key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || strings.Join(keys, ",") != "A-1,A-2" {
		t.Errorf("pages = %d, keys = %v", pages, keys)
	}
	if strings.Join(seen, ",") != ",tok2" {
		t.Errorf("token sequence = %v", seen)
	}
}

func TestChangelogAndCommentsPageToTotal(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/changelog"):
			if r.URL.Query().Get("startAt") == "0" {
				w.Write([]byte(`{"total":2,"values":[{"id":"h1","created":"2026-01-01T00:00:00.000+0000","items":[]}]}`))
				return
			}
			w.Write([]byte(`{"total":2,"isLast":true,"values":[{"id":"h2","created":"2026-01-02T00:00:00.000+0000","items":[]}]}`))
		case strings.HasSuffix(r.URL.Path, "/comment"):
			if r.URL.Query().Get("startAt") == "0" {
				w.Write([]byte(`{"total":2,"comments":[{"id":"c1","body":"first"}]}`))
				return
			}
			w.Write([]byte(`{"total":2,"comments":[{"id":"c2","body":"second"}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	hist, err := c.Changelog(context.Background(), "A-1")
	if err != nil || len(hist) != 2 {
		t.Fatalf("changelog = %d entries, err = %v", len(hist), err)
	}
	comments, err := c.Comments(context.Background(), "A-1")
	if err != nil || len(comments) != 2 {
		t.Fatalf("comments = %d, err = %v", len(comments), err)
	}
}

func TestFieldsParsesCatalog(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rest/api/3/field" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`[
			{"id":"summary","name":"Summary","custom":false,"schema":{"type":"string","system":"summary"}},
			{"id":"customfield_10016","name":"Story Points","custom":true,
			 "schema":{"type":"number","custom":"com.atlassian.jira.plugin.system.customfieldtypes:float","customId":10016}}
		]`))
	}))
	got, err := c.Fields(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "summary" || got[0].Custom || got[0].Schema.Type != "string" {
		t.Errorf("system field = %+v", got[0])
	}
	if got[1].ID != "customfield_10016" || !got[1].Custom || got[1].Name != "Story Points" {
		t.Errorf("custom field = %+v", got[1])
	}
	if got[1].Schema.Type != "number" || !strings.Contains(got[1].Schema.Custom, "float") {
		t.Errorf("custom schema = %+v", got[1].Schema)
	}
}

func TestUsageCountsThrottleAndRetry(t *testing.T) {
	calls := 0
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			// Positive Retry-After seconds would sleep whole seconds; use header
			// parse miss and a tiny Backoff so the test does not stall.
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`[{"id":"1","name":"Highest"}]`))
	}))
	// Backoff must be short: Retry-After "0" is ignored (s > 0 required), so
	// wait uses Backoff << attempt. One millisecond keeps WaitMS > 0 without
	// multi-second sleeps.
	c.Backoff = time.Millisecond

	got, err := c.Priorities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "Highest" {
		t.Fatalf("priorities = %v", got)
	}

	u := c.Usage()
	if u.Requests != 2 {
		t.Errorf("Requests = %d, want 2", u.Requests)
	}
	if u.Throttled != 1 {
		t.Errorf("Throttled = %d, want 1", u.Throttled)
	}
	if u.Retries != 1 {
		t.Errorf("Retries = %d, want 1", u.Retries)
	}
	if u.ServerErrors != 0 {
		t.Errorf("ServerErrors = %d, want 0", u.ServerErrors)
	}
	if u.WaitMS <= 0 {
		t.Errorf("WaitMS = %d, want > 0", u.WaitMS)
	}
	if u.LastThrottledAt.IsZero() {
		t.Error("LastThrottledAt is zero, want a UTC timestamp")
	}

	// TakeUsage zeroes counters but keeps LastThrottledAt.
	taken := c.TakeUsage()
	if taken.Requests != 2 || taken.Throttled != 1 {
		t.Errorf("TakeUsage snapshot = %+v", taken)
	}
	after := c.Usage()
	if after.Requests != 0 || after.Throttled != 0 || after.Retries != 0 || after.WaitMS != 0 {
		t.Errorf("counters not zeroed after TakeUsage: %+v", after)
	}
	if after.LastThrottledAt.IsZero() {
		t.Error("LastThrottledAt must survive TakeUsage")
	}
	if !after.LastThrottledAt.Equal(taken.LastThrottledAt) {
		t.Errorf("LastThrottledAt moved: taken=%v after=%v", taken.LastThrottledAt, after.LastThrottledAt)
	}
}

func decode(t *testing.T, r *http.Request, out any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}

func TestRawGETReturnsBody(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rest/api/3/myself" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization")
		}
		w.Write([]byte(`{"accountId":"abc"}`))
	}))
	status, body, err := c.Raw(context.Background(), http.MethodGet, "/rest/api/3/myself", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Errorf("status = %d", status)
	}
	if string(body) != `{"accountId":"abc"}` {
		t.Errorf("body = %s", body)
	}
	if u := c.Usage(); u.Requests != 1 {
		t.Errorf("Requests = %d, want 1", u.Requests)
	}
}

func TestRawRejectsAbsoluteURL(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not send request for absolute path: %s", r.URL)
	}))
	for _, path := range []string{
		"https://evil.example/steal",
		"http://evil.example/steal",
		"//evil.example/steal",
	} {
		status, body, err := c.Raw(context.Background(), http.MethodGet, path, nil, false)
		if err == nil {
			t.Errorf("%q: want error, got status=%d body=%s", path, status, body)
			continue
		}
		if !strings.Contains(err.Error(), "absolute") && !strings.Contains(err.Error(), "must start with /") {
			t.Errorf("%q: err = %v", path, err)
		}
		if strings.Contains(err.Error(), "secret-token") {
			t.Errorf("%q: error leaked token", path)
		}
	}
	if u := c.Usage(); u.Requests != 0 {
		t.Errorf("rejected paths must not leave the process: Requests=%d", u.Requests)
	}
}

func TestRawWriteRetryPolicy(t *testing.T) {
	// Write: 500 is not retried (might have applied); 429 is.
	t.Run("500 not retried", func(t *testing.T) {
		calls := 0
		c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"errorMessages":["boom"]}`))
		}))
		status, body, err := c.Raw(context.Background(), http.MethodPost, "/rest/api/3/issue/A-1/worklog",
			[]byte(`{"timeSpent":"1h"}`), true)
		if err != nil {
			t.Fatal(err)
		}
		if status != 500 {
			t.Errorf("status = %d", status)
		}
		if !strings.Contains(string(body), "boom") {
			t.Errorf("body = %s", body)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1 (write must not retry 500)", calls)
		}
	})
	t.Run("429 retried", func(t *testing.T) {
		calls := 0
		c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"wl-1"}`))
		}))
		c.Backoff = 0
		status, body, err := c.Raw(context.Background(), http.MethodPost, "/rest/api/3/issue/A-1/worklog",
			[]byte(`{"timeSpent":"1h"}`), true)
		if err != nil {
			t.Fatal(err)
		}
		if status != 201 || string(body) != `{"id":"wl-1"}` {
			t.Errorf("status=%d body=%s", status, body)
		}
		if calls != 2 {
			t.Errorf("calls = %d, want 2", calls)
		}
	})
}

func TestRawNon2xxStillReturnsBody(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["not found"]}`))
	}))
	status, body, err := c.Raw(context.Background(), http.MethodGet, "/rest/api/3/issue/NOPE", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if status != 404 {
		t.Errorf("status = %d", status)
	}
	if !strings.Contains(string(body), "not found") {
		t.Errorf("body = %s", body)
	}
}

func TestRawQueryPassthrough(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "maxResults=5&query=a+b" {
			// url.Values.Encode uses sorted keys: maxResults then query
			if r.URL.Query().Get("maxResults") != "5" || r.URL.Query().Get("query") != "a b" {
				t.Errorf("query = %q", r.URL.RawQuery)
			}
		}
		w.Write([]byte(`[]`))
	}))
	_, _, err := c.Raw(context.Background(), http.MethodGet, "/rest/api/3/user/search?maxResults=5&query=a+b", nil, false)
	if err != nil {
		t.Fatal(err)
	}
}
