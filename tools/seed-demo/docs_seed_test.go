package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTopoPagesOrdersParentsFirst(t *testing.T) {
	pages := []DocsPage{
		{Space: "ENG", Title: "Child A", Parent: "Root"},
		{Space: "ENG", Title: "Root"},
		{Space: "ENG", Title: "Child B", Parent: "Root"},
		{Space: "PROD", Title: "Solo"},
	}
	got, err := topoPages(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("len=%d", len(got))
	}
	// Root before its children.
	idx := map[string]int{}
	for i, p := range got {
		idx[p.Space+"/"+p.Title] = i
	}
	if idx["ENG/Root"] > idx["ENG/Child A"] || idx["ENG/Root"] > idx["ENG/Child B"] {
		t.Fatalf("parent not first: %+v", got)
	}
}

func TestTopoPagesDetectsCycle(t *testing.T) {
	pages := []DocsPage{
		{Space: "ENG", Title: "A", Parent: "B"},
		{Space: "ENG", Title: "B", Parent: "A"},
	}
	if _, err := topoPages(pages); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestTopoPagesDetectsMissingParent(t *testing.T) {
	pages := []DocsPage{
		{Space: "ENG", Title: "Orphan", Parent: "Ghost"},
	}
	if _, err := topoPages(pages); err == nil {
		t.Fatal("expected missing parent error")
	}
}

func TestSeedDocsCreatesSpacePageComment(t *testing.T) {
	var posts atomic.Int32
	var gets atomic.Int32
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/wiki/rest/api/space/"):
			gets.Add(1)
			http.NotFound(w, r) // force create
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/wiki/rest/api/space"):
			posts.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"key": "ENG", "id": "s1"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/wiki/rest/api/content"):
			gets.Add(1)
			// empty results → create
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/wiki/rest/api/content"):
			posts.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			typ, _ := body["type"].(string)
			if typ == "page" {
				if body["title"] != "Root" && body["title"] != "Child" {
					t.Errorf("unexpected title %v", body["title"])
				}
				if body["title"] == "Child" {
					anc, _ := body["ancestors"].([]any)
					if len(anc) == 0 {
						t.Error("child must have ancestors")
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"id": "p-" + body["title"].(string)})
				return
			}
			if typ == "comment" {
				_ = json.NewEncoder(w).Encode(map[string]string{"id": "c1"})
				return
			}
			t.Errorf("unexpected type %v", typ)
			http.Error(w, "bad", 400)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	c.paceDelay = 0

	data := &DocsDataset{
		Spaces: []DocsSpace{{Key: "ENG", Name: "Engineering"}},
		Pages: []DocsPage{
			{Space: "ENG", Title: "Child", Parent: "Root", BodyStorage: "<p>child</p>"},
			{Space: "ENG", Title: "Root", BodyStorage: "<p>root</p>",
				Comments: []DocsComment{{BodyStorage: "<p>note</p>"}}},
		},
	}
	if code := c.seedDocsData(data, false); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if posts.Load() < 3 { // space + 2 pages + comment
		t.Errorf("posts=%d, want ≥ 3", posts.Load())
	}
}

func TestSeedDocsSkipsExistingPage(t *testing.T) {
	var pageCreates atomic.Int32
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/wiki/rest/api/space/"):
			_ = json.NewEncoder(w).Encode(map[string]string{"key": "ENG"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/wiki/rest/api/content"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]string{{"id": "existing-1"}},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/wiki/rest/api/content"):
			pageCreates.Add(1)
			http.Error(w, "should not create", 500)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	c.paceDelay = 0

	data := &DocsDataset{
		Spaces: []DocsSpace{{Key: "ENG", Name: "Engineering"}},
		Pages: []DocsPage{
			{Space: "ENG", Title: "Already There", BodyStorage: "<p>x</p>",
				Comments: []DocsComment{{BodyStorage: "<p>c</p>"}}},
		},
	}
	if code := c.seedDocsData(data, false); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if pageCreates.Load() != 0 {
		t.Errorf("created %d pages, want 0 (idempotent skip)", pageCreates.Load())
	}
}

func TestSeedDocsRetries429WithRetryAfter(t *testing.T) {
	var calls atomic.Int32
	start := time.Now()
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 && r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/space/") {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/space/"):
			_ = json.NewEncoder(w).Encode(map[string]string{"key": "ENG"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/content"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/content"):
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "p1"})
		default:
			http.NotFound(w, r)
		}
	}))
	// Non-zero backoff so Retry-After path is exercised; 0 header → immediate.
	c.backoff = time.Millisecond
	c.paceDelay = 0

	data := &DocsDataset{
		Spaces: []DocsSpace{{Key: "ENG", Name: "Engineering"}},
		Pages:  []DocsPage{{Space: "ENG", Title: "P", BodyStorage: "<p>x</p>"}},
	}
	if code := c.seedDocsData(data, false); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if calls.Load() < 2 {
		t.Errorf("calls=%d, want retry", calls.Load())
	}
	if time.Since(start) > 5*time.Second {
		t.Error("retry waited too long")
	}
}

func TestSeedDocsDryRunNoNetwork(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "nope", 500)
	}))
	t.Cleanup(srv.Close)
	c := newClient(srv.URL, "a@example.com", "tok")
	c.paceDelay = 0
	data := &DocsDataset{
		Spaces: []DocsSpace{{Key: "ENG", Name: "E"}},
		Pages:  []DocsPage{{Space: "ENG", Title: "T", BodyStorage: "<p>x</p>"}},
	}
	if code := c.seedDocsData(data, true); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if hits != 0 {
		t.Errorf("dry-run made %d HTTP calls", hits)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter("2"); d != 2*time.Second {
		t.Errorf("seconds: got %v", d)
	}
	if d := parseRetryAfter(""); d != 0 {
		t.Errorf("empty: got %v", d)
	}
}
