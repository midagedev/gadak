package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// collisionSite is the smallest stub that can accept a connected init and
// a full sync, and that returns one issue whose numeric id is 10001 — the
// id issuetap assigns to the first standalone-created issue (GDK-241).
type collisionSite struct {
	issueJSON []byte
	hits      []string
}

func (s *collisionSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.hits = append(s.hits, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/rest/api/3/myself":
		_, _ = w.Write([]byte(`{"displayName":"Stub User","accountId":"acc-stub"}`))
	case "/rest/api/3/status":
		_, _ = w.Write([]byte(`[{"id":"3","name":"In Progress","statusCategory":{"key":"indeterminate"}}]`))
	case "/rest/api/3/priority":
		_, _ = w.Write([]byte(`[{"id":"3","name":"Medium"}]`))
	case "/rest/api/3/field":
		_, _ = w.Write([]byte(`[]`))
	case "/rest/api/3/filter/my":
		_, _ = w.Write([]byte(`[]`))
	case "/rest/api/3/search/approximate-count":
		_, _ = w.Write([]byte(`{"count":1}`))
	case "/rest/api/3/search/jql":
		_, _ = w.Write([]byte(`{"issues":[` + string(s.issueJSON) + `],"isLast":true}`))
	default:
		http.NotFound(w, r)
	}
}

func sqlTSV(t *testing.T, q string) string {
	t.Helper()
	out, err := capture(t, func() error {
		return cmdSQL([]string{"--no-header", q})
	})
	if err != nil {
		t.Fatalf("sql %s: %v\n%s", q, err, out)
	}
	return strings.TrimSpace(out)
}

// TestGDK241ItemIDCollision reproduces (or refutes) the claim that a
// standalone-created issue with numeric id 10001 is overwritten when a
// connected workspace then syncs a site issue with the same id.
//
// This test does not implement namespacing. It records what the current
// upsert (ON CONFLICT(id)) actually does.
func TestGDK241ItemIDCollision(t *testing.T) {
	home := seedStandaloneWithIssue(t)

	beforeItems := sqlTSV(t, "select id, key, title, external_id from items where kind = 'issue' order by id")
	t.Logf("BEFORE items: %s", beforeItems)
	if !strings.Contains(beforeItems, "jira:10001") {
		t.Fatalf("GDK-241 premise: first standalone issue id is jira:10001, got %q", beforeItems)
	}
	if !strings.Contains(beforeItems, "local only issue") {
		t.Fatalf("local summary missing from before-row: %q", beforeItems)
	}

	siteIssue := map[string]any{
		"id":  "10001",
		"key": "NMB-1",
		"fields": map[string]any{
			"summary":   "SITE ISSUE COLLISION",
			"project":   map[string]any{"key": "NMB"},
			"issuetype": map[string]any{"id": "10004", "name": "Bug"},
			"status":    map[string]any{"id": "3", "name": "In Progress", "statusCategory": map[string]any{"key": "indeterminate"}},
			"created":   "2026-08-01T00:00:00.000Z",
			"updated":   "2026-08-18T12:00:00.000Z",
			"comment":   map[string]any{"total": 0, "comments": []any{}},
		},
		"changelog": map[string]any{"total": 0, "histories": []any{}},
	}
	raw, err := json.Marshal(siteIssue)
	if err != nil {
		t.Fatal(err)
	}
	stub := &collisionSite{issueJSON: raw}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)

	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
				"--replace-standalone",
			})
		}); err != nil {
			t.Fatalf("replace-standalone init: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IsStandalone() {
		t.Fatal("expected connected after --replace-standalone")
	}

	syncOut, syncErr := capture(t, func() error { return cmdSync([]string{"--full"}) })
	t.Logf("sync --full out=%q err=%v hits=%v", syncOut, syncErr, stub.hits)
	if syncErr != nil {
		t.Fatalf("sync --full: %v\n%s", syncErr, syncOut)
	}

	afterItems := sqlTSV(t, "select id, key, title, external_id from items where kind = 'issue' order by id")
	afterIssues := sqlTSV(t, "select key, summary from issues_full order by key")
	deleted := sqlTSV(t, "select key, source_id from deleted_items order by key")
	t.Logf("AFTER items: %s", afterItems)
	t.Logf("AFTER issues_full: %s", afterIssues)
	t.Logf("AFTER deleted_items: %q", deleted)

	overwrote := strings.Contains(afterItems, "jira:10001") &&
		strings.Contains(afterItems, "NMB-1") &&
		strings.Contains(afterItems, "SITE ISSUE COLLISION") &&
		!strings.Contains(afterItems, "local only issue")
	keptLocal := strings.Contains(afterItems, "local only issue")
	t.Logf("GDK-241 overwrite=%v kept_local=%v persist=%s", overwrote, keptLocal, origin.PersistPath(home))
}
