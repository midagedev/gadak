package jira

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "someone@example.com", "secret-token")
	c.Retries, c.Backoff = 4, 0
	return c
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

func decode(t *testing.T, r *http.Request, out any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}
