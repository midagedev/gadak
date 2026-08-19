package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// TestGDK241ItemIDCollision asserts that a standalone-created issue with
// numeric id 10001 (issuetap's first id — the same number a real site can
// hold) is not silently overwritten when the workspace is converted with
// --replace-standalone and a connected sync upserts a site issue with the
// same numeric id. The local row must either survive or be cleanly
// tombstoned by reconcile into deleted_items — never transmuted into the
// site row by ON CONFLICT(id).
func TestGDK241ItemIDCollision(t *testing.T) {
	home := seedStandaloneWithIssue(t)

	beforeItems := sqlTSV(t, "select id, key, title, external_id from items where kind = 'issue' order by id")
	t.Logf("BEFORE items: %s", beforeItems)
	if !strings.Contains(beforeItems, "standalone-jira:10001") {
		t.Fatalf("GDK-241 premise: first standalone issue id is standalone-jira:10001, got %q", beforeItems)
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

	// The conversion drops the old origin's mirror whole (disposable cache;
	// the origin persist keeps the local issue). A leftover standalone row
	// is exactly what the connected sync's ON CONFLICT(id) would transmute.
	if got := sqlTSV(t, "select id, key from items where kind = 'issue'"); got != "" {
		t.Fatalf("conversion must drop the standalone mirror, still holds: %q", got)
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

	// The site issue must land intact under its own id.
	if !strings.Contains(afterItems, "jira:10001") ||
		!strings.Contains(afterItems, "NMB-1") ||
		!strings.Contains(afterItems, "SITE ISSUE COLLISION") {
		t.Fatalf("site issue NMB-1 (jira:10001) missing after sync: %q", afterItems)
	}
	// No trace of the standalone row may leak into the connected mirror —
	// neither its key nor its title transmuted onto the site id. (Its origin
	// copy still lives in the issuetap persist file.)
	if strings.Contains(afterItems, "STD-1") || strings.Contains(afterItems, "local only issue") {
		t.Fatalf("standalone row leaked into connected mirror (GDK-241): items=%q deleted_items=%q persist=%s",
			afterItems, deleted, origin.PersistPath(home))
	}
}

// TestGDK241LegacyStandaloneMirrorUpgrades covers the upgrade path: a
// standalone mirror written before ids were namespaced holds `jira:10001` /
// STD-1. The next standalone sync inserts `standalone-jira:10001` / STD-1 —
// same UNIQUE(source_id, key), new id — which must not fail the sync: the
// pre-namespace row is purged and re-mirrored under the new namespace.
func TestGDK241LegacyStandaloneMirrorUpgrades(t *testing.T) {
	seedStandaloneWithIssue(t)

	// Rewrite the mirror row back to the pre-namespace id, simulating a
	// workspace whose mirror was written by an older build.
	dbPath := filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`UPDATE items SET id = 'jira:10001' WHERE id = 'standalone-jira:10001'`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdSync([]string{"--full"}) })
	if err != nil {
		t.Fatalf("standalone sync over a pre-namespace mirror must not fail: %v\n%s", err, out)
	}
	items := sqlTSV(t, "select id, key, title from items where kind = 'issue' order by id")
	if !strings.Contains(items, "standalone-jira:10001") || strings.Contains(items, "\tjira:10001") ||
		strings.HasPrefix(items, "jira:10001") {
		t.Fatalf("legacy row must be re-mirrored under the standalone namespace, got %q", items)
	}
	if !strings.Contains(items, "local only issue") {
		t.Fatalf("local issue lost during namespace upgrade: %q", items)
	}
}

// TestGDK241LegacyMirrorDroppedOnConversion covers the remaining seal: a
// mirror written by a pre-namespace build (legacy `jira:10001` row) is
// converted with --replace-standalone WITHOUT ever running a standalone
// sync on the new build — so the standalone-path purge never fires. The
// conversion itself must drop the old origin's mirror; otherwise the first
// connected sync's ON CONFLICT(id) silently transmutes the legacy row into
// the site issue (the original GDK-241 symptom).
func TestGDK241LegacyMirrorDroppedOnConversion(t *testing.T) {
	home := seedStandaloneWithIssue(t)

	dbPath := filepath.Join(home, "gadak.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`UPDATE items SET id = 'jira:10001' WHERE id = 'standalone-jira:10001'`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
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
	rawIssue, err := json.Marshal(siteIssue)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(&collisionSite{issueJSON: rawIssue})
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

	// The seal: conversion drops the old origin's mirror whole. A leftover
	// legacy row is exactly what the first connected sync would transmute.
	if got := sqlTSV(t, "select id, key from items where kind = 'issue'"); got != "" {
		t.Fatalf("conversion must drop the standalone mirror, still holds: %q", got)
	}

	// Plain sync (no --full): the cleared watermark must make it full, so
	// the site issue lands.
	out, err := capture(t, func() error { return cmdSync(nil) })
	if err != nil {
		t.Fatalf("first connected sync: %v\n%s", err, out)
	}
	items := sqlTSV(t, "select id, key, title from items where kind = 'issue' order by id")
	if !strings.Contains(items, "jira:10001\tNMB-1\tSITE ISSUE COLLISION") {
		t.Fatalf("site issue missing after first connected sync: %q", items)
	}
	if strings.Contains(items, "STD-1") || strings.Contains(items, "local only issue") {
		t.Fatalf("legacy standalone row leaked into connected mirror: %q", items)
	}
}
