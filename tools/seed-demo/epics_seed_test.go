package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEpicTypeIDUsesHierarchyLevel(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/project/NMB") {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		// Localized name "에픽" must still resolve via hierarchyLevel == 1.
		_, _ = w.Write([]byte(`{
			"key":"NMB",
			"issueTypes":[
				{"id":"1","name":"버그","hierarchyLevel":0,"subtask":false},
				{"id":"99","name":"에픽","hierarchyLevel":1,"subtask":false},
				{"id":"3","name":"Sub-task","hierarchyLevel":-1,"subtask":true}
			]
		}`))
	}))
	got := c.epicTypeID("NMB")
	if got != "99" {
		t.Fatalf("epic type id = %q, want 99", got)
	}
}

func TestSeedEpicsCreatesAndParents(t *testing.T) {
	var posts atomic.Int32
	var puts atomic.Int32
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/project/NMB"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issueTypes": []map[string]any{
					{"id": "10", "name": "Bug", "hierarchyLevel": 0},
					{"id": "20", "name": "Epic", "hierarchyLevel": 1},
				},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/search/jql"):
			jql := r.URL.Query().Get("jql")
			switch {
			case strings.Contains(jql, "Billing reliability"):
				// no existing epic
				_ = json.NewEncoder(w).Encode(map[string]any{"issues": []any{}})
			case strings.Contains(jql, "Invoice PDF blank"):
				_ = json.NewEncoder(w).Encode(map[string]any{
					"issues": []map[string]any{
						{"key": "NMB-1", "fields": map[string]string{"summary": "Invoice PDF blank"}},
					},
				})
			case strings.Contains(jql, "Portal loop"):
				_ = json.NewEncoder(w).Encode(map[string]any{
					"issues": []map[string]any{
						{"key": "NMB-2", "fields": map[string]string{"summary": "Portal loop"}},
					},
				})
			default:
				_ = json.NewEncoder(w).Encode(map[string]any{"issues": []any{}})
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rest/api/3/issue"):
			posts.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fields, _ := body["fields"].(map[string]any)
			it, _ := fields["issuetype"].(map[string]any)
			if it["id"] != "20" {
				t.Errorf("issuetype id = %v, want 20", it["id"])
			}
			if fields["summary"] != "Billing reliability" {
				t.Errorf("summary = %v", fields["summary"])
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"key": "NMB-100", "id": "100"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/issue/") && strings.Contains(r.URL.RawQuery, "fields=parent"):
			// neither child has a parent yet
			_ = json.NewEncoder(w).Encode(map[string]any{"fields": map[string]any{"parent": nil}})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/issue/"):
			puts.Add(1)
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			fields, _ := body["fields"].(map[string]any)
			parent, _ := fields["parent"].(map[string]any)
			if parent["key"] != "NMB-100" {
				t.Errorf("parent payload = %v, want NMB-100", parent)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	c.paceDelay = 0

	data := &EpicsDataset{
		Epics: []EpicEntry{{
			Project:        "NMB",
			Summary:        "Billing reliability",
			Description:    "Make billing trustworthy.",
			ChildSummaries: []string{"Invoice PDF blank", "Portal loop"},
		}},
	}
	if code := c.seedEpicsData(data, false); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if posts.Load() != 1 {
		t.Errorf("epic creates=%d, want 1", posts.Load())
	}
	if puts.Load() != 2 {
		t.Errorf("parent puts=%d, want 2", puts.Load())
	}
}

func TestSeedEpicsIdempotentSkip(t *testing.T) {
	var posts atomic.Int32
	var puts atomic.Int32
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/project/NMB"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issueTypes": []map[string]any{
					{"id": "20", "name": "Epic", "hierarchyLevel": 1},
				},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/search/jql"):
			jql := r.URL.Query().Get("jql")
			if strings.Contains(jql, "Billing reliability") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"issues": []map[string]any{
						{"key": "NMB-100", "fields": map[string]string{"summary": "Billing reliability"}},
					},
				})
				return
			}
			if strings.Contains(jql, "Invoice PDF blank") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"issues": []map[string]any{
						{"key": "NMB-1", "fields": map[string]string{"summary": "Invoice PDF blank"}},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"issues": []any{}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rest/api/3/issue"):
			posts.Add(1)
			http.Error(w, "should not create", 500)
		case r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "fields=parent"):
			// already parented
			_ = json.NewEncoder(w).Encode(map[string]any{
				"fields": map[string]any{
					"parent": map[string]string{"key": "NMB-100"},
				},
			})
		case r.Method == http.MethodPut:
			puts.Add(1)
			http.Error(w, "should not put", 500)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	c.paceDelay = 0

	data := &EpicsDataset{
		Epics: []EpicEntry{{
			Project:        "NMB",
			Summary:        "Billing reliability",
			Description:    "x",
			ChildSummaries: []string{"Invoice PDF blank"},
		}},
	}
	if code := c.seedEpicsData(data, false); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if posts.Load() != 0 {
		t.Errorf("created %d epics, want 0 (idempotent skip)", posts.Load())
	}
	if puts.Load() != 0 {
		t.Errorf("parent puts=%d, want 0 (already parented)", puts.Load())
	}
}

func TestSeedEpicsRetries429WithRetryAfter(t *testing.T) {
	var calls atomic.Int32
	start := time.Now()
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/project/NMB") {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/project/NMB"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issueTypes": []map[string]any{
					{"id": "20", "name": "Epic", "hierarchyLevel": 1},
				},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/search/jql"):
			_ = json.NewEncoder(w).Encode(map[string]any{"issues": []any{}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rest/api/3/issue"):
			_ = json.NewEncoder(w).Encode(map[string]string{"key": "NMB-9", "id": "9"})
		default:
			http.NotFound(w, r)
		}
	}))
	c.backoff = time.Millisecond
	c.paceDelay = 0

	data := &EpicsDataset{
		Epics: []EpicEntry{{
			Project:     "NMB",
			Summary:     "Solo epic",
			Description: "x",
		}},
	}
	if code := c.seedEpicsData(data, false); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if calls.Load() < 2 {
		t.Errorf("calls=%d, want retry", calls.Load())
	}
	if time.Since(start) > 5*time.Second {
		t.Error("retry waited too long")
	}
}

func TestSeedEpicsDryRunNoNetwork(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "nope", 500)
	}))
	t.Cleanup(srv.Close)
	c := newClient(srv.URL, "a@example.com", "tok")
	c.paceDelay = 0

	data := &EpicsDataset{
		Epics: []EpicEntry{{
			Project:        "NMB",
			Summary:        "Dashboard polish and reliability",
			Description:    "Raise the bar.",
			ChildSummaries: []string{"Filter chips reset", "Chart legend overlaps"},
		}},
	}
	if code := c.seedEpicsData(data, true); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if hits != 0 {
		t.Errorf("network hits=%d, want 0 in dry-run", hits)
	}
}

func TestSetParentPayloadShape(t *testing.T) {
	var got map[string]any
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	if !c.setParent("NMB-1", "NMB-100") {
		t.Fatal("setParent failed")
	}
	fields, _ := got["fields"].(map[string]any)
	parent, _ := fields["parent"].(map[string]any)
	if parent["key"] != "NMB-100" {
		t.Fatalf("payload = %#v", got)
	}
}
