package main

import (
	"context"
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
// id issuetap assigns to the first local-origin-created issue (GDK-241).
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

// TestGDK241ItemIDCollision asserts that a local-origin-created issue with
// numeric id 10001 (issuetap's first id — the same number a real site can
// hold) is not silently overwritten when the workspace is converted with
// --replace-local and a connected sync upserts a site issue with the
// same numeric id. The local row must either survive or be cleanly
// tombstoned by reconcile into deleted_items — never transmuted into the
// site row by ON CONFLICT(id).
func TestGDK241ItemIDCollision(t *testing.T) {
	home := seedLocalOriginWithIssue(t)

	beforeItems := sqlTSV(t, "select id, key, title, external_id from items where kind = 'issue' order by id")
	t.Logf("BEFORE items: %s", beforeItems)
	if !strings.Contains(beforeItems, "local-origin-jira:10001") {
		t.Fatalf("GDK-241 premise: first local-origin issue id is local-origin-jira:10001, got %q", beforeItems)
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
				"--replace-local",
			})
		}); err != nil {
			t.Fatalf("replace-local init: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HasLocalOrigin() {
		t.Fatal("expected connected after --replace-local")
	}

	// The conversion drops the old origin's mirror whole (disposable cache;
	// the origin persist keeps the local issue). A leftover local-origin row
	// is exactly what the connected sync's ON CONFLICT(id) would transmute.
	if got := sqlTSV(t, "select id, key from items where kind = 'issue'"); got != "" {
		t.Fatalf("conversion must drop the local-origin mirror, still holds: %q", got)
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
	// No trace of the local-origin row may leak into the connected mirror —
	// neither its key nor its title transmuted onto the site id. (Its origin
	// copy still lives in the issuetap persist file.)
	if strings.Contains(afterItems, "STD-1") || strings.Contains(afterItems, "local only issue") {
		t.Fatalf("local-origin row leaked into connected mirror (GDK-241): items=%q deleted_items=%q persist=%s",
			afterItems, deleted, origin.PersistPath(home))
	}
}

// TestGDK241LegacyStandaloneMirrorUpgrades covers the upgrade path: a
// local-origin mirror written before ids were namespaced holds `jira:10001` /
// STD-1. The next local-origin sync inserts `local-origin-jira:10001` / STD-1 —
// same UNIQUE(source_id, key), new id — which must not fail the sync: the
// pre-namespace row is purged and re-mirrored under the new namespace.
func TestGDK241LegacyStandaloneMirrorUpgrades(t *testing.T) {
	seedLocalOriginWithIssue(t)

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
		t.Fatalf("local-origin sync over a pre-namespace mirror must not fail: %v\n%s", err, out)
	}
	items := sqlTSV(t, "select id, key, title from items where kind = 'issue' order by id")
	if !strings.Contains(items, "local-origin-jira:10001") || strings.Contains(items, "\tjira:10001") ||
		strings.HasPrefix(items, "jira:10001") {
		t.Fatalf("legacy row must be re-mirrored under the local-origin namespace, got %q", items)
	}
	if !strings.Contains(items, "local only issue") {
		t.Fatalf("local issue lost during namespace upgrade: %q", items)
	}
}

// TestGDK241LegacyMirrorDroppedOnConversion covers the remaining seal: a
// mirror written by a pre-namespace build (legacy `jira:10001` row) is
// converted with --replace-local WITHOUT ever running a local-origin
// sync on the new build — so the local-origin-path purge never fires. The
// conversion itself must drop the old origin's mirror; otherwise the first
// connected sync's ON CONFLICT(id) silently transmutes the legacy row into
// the site issue (the original GDK-241 symptom).
func TestGDK241LegacyMirrorDroppedOnConversion(t *testing.T) {
	home := seedLocalOriginWithIssue(t)

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
				"--replace-local",
			})
		}); err != nil {
			t.Fatalf("replace-local init: %v", err)
		}
	})

	// The seal: conversion drops the old origin's mirror whole. A leftover
	// legacy row is exactly what the first connected sync would transmute.
	if got := sqlTSV(t, "select id, key from items where kind = 'issue'"); got != "" {
		t.Fatalf("conversion must drop the local-origin mirror, still holds: %q", got)
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
		t.Fatalf("legacy local-origin row leaked into connected mirror: %q", items)
	}
}

// collisionWikiSite is collisionSite plus the Confluence endpoints a
// connected wiki pass needs: one space and one page whose numeric id is
// 20001 — the id issuetap assigns to the first local-origin-created page
// (GDK-344 / F10).
type collisionWikiSite struct {
	collisionSite
	pageJSON []byte
}

func (s *collisionWikiSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.hits = append(s.hits, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	switch {
	case path == "/wiki/rest/api/space/ENG":
		_, _ = w.Write([]byte(`{"key":"ENG","name":"Engineering","type":"global"}`))
	case path == "/wiki/rest/api/content/search":
		_, _ = w.Write([]byte(`{"results":[` + string(s.pageJSON) + `],"_links":{}}`))
	case path == "/wiki/rest/api/content/20001":
		_, _ = w.Write(s.pageJSON)
	case strings.HasSuffix(path, "/child/comment"):
		_, _ = w.Write([]byte(`{"results":[],"size":0,"limit":100,"start":0}`))
	case path == "/wiki/rest/api/content/20001/version":
		_, _ = w.Write([]byte(`{"results":[{"number":1,"when":"2026-08-18T12:00:00.000Z"}],"_links":{}}`))
	default:
		s.collisionSite.ServeHTTP(w, r)
	}
}

func collisionPageJSON(t *testing.T) []byte {
	t.Helper()
	page := map[string]any{
		"id":     "20001",
		"type":   "page",
		"status": "current",
		"title":  "SITE PAGE COLLISION",
		"space":  map[string]any{"key": "ENG", "name": "Engineering"},
		"version": map[string]any{
			"number": 1,
			"when":   "2026-08-18T12:00:00.000Z",
			"by":     map[string]any{"accountId": "acc-stub", "displayName": "Stub User"},
		},
		"body": map[string]any{
			"atlas_doc_format": map[string]any{
				"value":          `{"type":"doc","version":1,"content":[]}`,
				"representation": "atlas_doc_format",
			},
		},
		"ancestors": []any{},
		"metadata":  map[string]any{"labels": map[string]any{"results": []any{}, "size": 0, "limit": 25, "start": 0}},
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func pageADF(text string) string {
	b, err := json.Marshal(map[string]any{
		"type": "doc", "version": 1,
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": text},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// seedLocalOriginWithPage is a local-origin workspace that already holds a
// locally originated wiki page (issuetap 20001) mirrored into gadak.db.
func seedLocalOriginWithPage(t *testing.T) (home, pageID string) {
	t.Helper()
	home = seedLocalOriginWithIssue(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	w, err := origin.Wiki(cfg)
	if err != nil {
		t.Fatal(err)
	}
	created, err := w.CreatePage(context.Background(), origin.DefaultSpaceKey, "local only page", pageADF("hello from local-origin wiki"), "")
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreatePage returned empty id")
	}
	if err := origin.Close(); err != nil {
		t.Fatalf("origin.Close: %v", err)
	}
	out, err := capture(t, func() error { return cmdSync([]string{"--full", "--source", "confluence"}) })
	if err != nil {
		t.Fatalf("local-origin wiki sync: %v\n%s", err, out)
	}
	// Production `gadak sync` exits and the kernel drops the persist lock.
	// This test stays in-process; leave the lock and CLI conversion
	// correctly refuses (GDK-415).
	if err := origin.Close(); err != nil {
		t.Fatalf("origin.Close after wiki sync: %v", err)
	}
	return home, created.ID
}

// TestGDK344PageIDCollision is the wiki sibling of TestGDK241ItemIDCollision:
// a local-origin-created page with numeric id 20001 (issuetap's first page id —
// the same number a real Confluence site can hold) is not silently overwritten
// when the workspace is converted with --replace-local and a connected
// wiki sync upserts a site page with the same numeric id.
func TestGDK344PageIDCollision(t *testing.T) {
	home, pageID := seedLocalOriginWithPage(t)
	if pageID != "20001" {
		t.Fatalf("GDK-344 premise: first local-origin page id is 20001, got %q", pageID)
	}

	before := sqlTSV(t, "select id, key, title, external_id from items where kind = 'page' order by id")
	t.Logf("BEFORE pages: %s", before)
	if !strings.Contains(before, "local-origin-confluence:20001") {
		t.Fatalf("GDK-344 premise: first local-origin page id is local-origin-confluence:20001, got %q", before)
	}
	if !strings.Contains(before, "local only page") {
		t.Fatalf("local page title missing from before-row: %q", before)
	}

	stub := &collisionWikiSite{
		collisionSite: collisionSite{issueJSON: []byte(`{"id":"10001","key":"NMB-1","fields":{"summary":"SITE ISSUE COLLISION","project":{"key":"NMB"},"issuetype":{"id":"10004","name":"Bug"},"status":{"id":"3","name":"In Progress","statusCategory":{"key":"indeterminate"}},"created":"2026-08-01T00:00:00.000Z","updated":"2026-08-18T12:00:00.000Z","comment":{"total":0,"comments":[]}},"changelog":{"total":0,"histories":[]}}`)},
		pageJSON:      collisionPageJSON(t),
	}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)

	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
				"--replace-local",
				"--spaces", "ENG",
			})
		}); err != nil {
			t.Fatalf("replace-local init: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HasLocalOrigin() {
		t.Fatal("expected connected after --replace-local")
	}
	if cfg.Confluence == nil {
		t.Fatal("expected wiki to stay on after --spaces ENG")
	}

	if got := sqlTSV(t, "select id, key from items where kind = 'page'"); got != "" {
		t.Fatalf("conversion must drop the local-origin wiki mirror, still holds: %q", got)
	}

	syncOut, syncErr := capture(t, func() error { return cmdSync([]string{"--full", "--source", "confluence"}) })
	t.Logf("wiki sync --full out=%q err=%v hits=%v", syncOut, syncErr, stub.hits)
	if syncErr != nil {
		t.Fatalf("wiki sync --full: %v\n%s", syncErr, syncOut)
	}

	after := sqlTSV(t, "select id, key, title, external_id from items where kind = 'page' order by id")
	t.Logf("AFTER pages: %s", after)
	if !strings.Contains(after, "confluence:20001") ||
		!strings.Contains(after, "SITE PAGE COLLISION") {
		t.Fatalf("site page 20001 (confluence:20001) missing after sync: %q", after)
	}
	if strings.Contains(after, "local-origin-confluence:") || strings.Contains(after, "local only page") {
		t.Fatalf("local-origin page leaked into connected mirror (GDK-344): %q persist=%s",
			after, origin.PersistPath(home))
	}
}

// TestGDK344LegacyStandalonePageMirrorUpgrades is the wiki sibling of
// TestGDK241LegacyStandaloneMirrorUpgrades: a local-origin page written before
// ids were namespaced holds `confluence:20001`. The next local-origin wiki
// sync inserts `local-origin-confluence:20001` — same UNIQUE(source_id, key),
// new id — which must not fail the sync.
func TestGDK344LegacyStandalonePageMirrorUpgrades(t *testing.T) {
	_, pageID := seedLocalOriginWithPage(t)
	if pageID != "20001" {
		t.Fatalf("GDK-344 premise: first local-origin page id is 20001, got %q", pageID)
	}

	dbPath := filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`UPDATE items SET id = 'confluence:20001' WHERE id = 'standalone-confluence:20001'`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdSync([]string{"--full", "--source", "confluence"}) })
	if err != nil {
		t.Fatalf("local-origin wiki sync over a pre-namespace page must not fail: %v\n%s", err, out)
	}
	items := sqlTSV(t, "select id, key, title from items where kind = 'page' order by id")
	if !strings.Contains(items, "local-origin-confluence:20001") || strings.Contains(items, "\tconfluence:20001") ||
		strings.HasPrefix(items, "confluence:20001") {
		t.Fatalf("legacy page must be re-mirrored under the local-origin namespace, got %q", items)
	}
	if !strings.Contains(items, "local only page") {
		t.Fatalf("local page lost during namespace upgrade: %q", items)
	}
}
