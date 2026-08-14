package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/store"

	_ "modernc.org/sqlite"
)

// confFixture is an in-memory Confluence Cloud stand-in. Identifiers are
// generic (AAA/BBB, user@example.invalid) — never a real site.
type confFixture struct {
	t *testing.T

	mu     sync.Mutex
	spaces []map[string]any
	// pages keyed by id; each has title, space, version, when, bodyADF, comments.
	pages map[string]*confPage

	// rateLimitOnce makes the next Spaces call return 429 then succeed.
	rateLimitOnce atomic.Bool

	// searches records every CQL body SearchPages sent.
	searches []string
	// failIfCQLContains, when non-empty, makes serveSearch return 400 for a
	// matching CQL (not 500: atlhttp would retry).
	failIfCQLContains string
}

type confPage struct {
	ID      string
	Space   string
	Title   string
	Version int
	When    string
	BodyADF string
	// Labels are label names as returned under metadata.labels.results[].name.
	// Order here is API order; sync sorts alphabetically into Page.Labels.
	Labels []string
	// comments: each has id, text, when; replies nested one level
	Comments []confComment
}

type confComment struct {
	ID      string
	Text    string
	When    string
	Replies []confComment
}

func newConfFixture(t *testing.T) *confFixture {
	t.Helper()
	adf := func(text string) string {
		b, _ := json.Marshal(map[string]any{
			"type": "doc", "version": 1,
			"content": []any{
				map[string]any{"type": "paragraph", "content": []any{
					map[string]any{"type": "text", "text": text},
				}},
			},
		})
		return string(b)
	}
	f := &confFixture{
		t: t,
		spaces: []map[string]any{
			{"key": "AAA", "name": "Alpha", "type": "global"},
			{"key": "BBB", "name": "Beta", "type": "global"},
		},
		pages: map[string]*confPage{
			"1001": {
				ID: "1001", Space: "AAA", Title: "Login runbook",
				Version: 2, When: "2026-08-01T10:00:00.000Z",
				BodyADF: adf("로그인이 실패했다 — check the auth gateway"),
				// Unsorted API order — sync must alphabetize.
				Labels: []string{"runbook", "ops"},
				Comments: []confComment{
					{ID: "c1001", Text: "still broken on staging", When: "2026-08-01T11:00:00.000Z"},
				},
			},
			"1002": {
				ID: "1002", Space: "AAA", Title: "Release notes",
				Version: 1, When: "2026-08-02T09:00:00.000Z",
				BodyADF: adf("shipped the retry budget cut"),
			},
			"2001": {
				ID: "2001", Space: "BBB", Title: "Beta intro",
				Version: 1, When: "2026-07-15T08:00:00.000Z",
				BodyADF: adf("welcome to beta"),
			},
		},
	}
	return f
}

func (f *confFixture) start() *confluence.Client {
	srv := httptest.NewServer(f)
	f.t.Cleanup(srv.Close)
	c := confluence.New(srv.URL, "user@example.invalid", "secret-token")
	c.Retries, c.Backoff, c.PauseBetween = 4, time.Millisecond, 0
	return c
}

func (f *confFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path := r.URL.Path
	switch {
	case path == "/wiki/rest/api/space":
		if f.rateLimitOnce.CompareAndSwap(true, false) {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": f.spaces, "size": len(f.spaces), "limit": 100, "start": 0,
		})
	case strings.HasPrefix(path, "/wiki/rest/api/space/"):
		// Single-space GET (path ②): /wiki/rest/api/space/{key}?expand=homepage
		key := strings.TrimPrefix(path, "/wiki/rest/api/space/")
		if key == "" || strings.Contains(key, "/") {
			http.NotFound(w, r)
			return
		}
		for _, s := range f.spaces {
			if sk, _ := s["key"].(string); sk == key {
				_ = json.NewEncoder(w).Encode(s)
				return
			}
		}
		http.NotFound(w, r)
	case path == "/wiki/rest/api/content/search":
		f.serveSearch(w, r)
	case strings.HasSuffix(path, "/child/comment"):
		f.serveComments(w, path)
	case strings.HasPrefix(path, "/wiki/rest/api/content/"):
		id := strings.TrimPrefix(path, "/wiki/rest/api/content/")
		if strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		p := f.pages[id]
		if p == nil {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(f.fullPage(p))
	default:
		f.t.Errorf("unexpected %s %s", r.Method, path)
		http.NotFound(w, r)
	}
}

func (f *confFixture) serveSearch(w http.ResponseWriter, r *http.Request) {
	cql := r.URL.Query().Get("cql")
	f.searches = append(f.searches, cql)
	if f.failIfCQLContains != "" && strings.Contains(cql, f.failIfCQLContains) {
		http.Error(w, "injected search failure", http.StatusBadRequest)
		return
	}
	if strings.Contains(cql, "type=comment") {
		f.serveCommentSearch(w, cql)
		return
	}
	var results []map[string]any
	// Collect and sort by When ascending.
	ids := make([]string, 0, len(f.pages))
	for id := range f.pages {
		ids = append(ids, id)
	}
	// stable-ish: sort by when then id
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if f.pages[ids[j]].When < f.pages[ids[i]].When ||
				(f.pages[ids[j]].When == f.pages[ids[i]].When && ids[j] < ids[i]) {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	for _, id := range ids {
		p := f.pages[id]
		if !cqlMatch(cql, p) {
			continue
		}
		results = append(results, map[string]any{
			"id": p.ID, "type": "page", "status": "current", "title": p.Title,
			"space": map[string]any{"key": p.Space, "name": f.spaceName(p.Space)},
			"version": map[string]any{
				"number": p.Version, "when": p.When,
				"by": map[string]any{"accountId": "acc-1", "displayName": "Ada Example"},
			},
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results": results,
		"_links":  map[string]string{},
	})
}

func (f *confFixture) serveCommentSearch(w http.ResponseWriter, cql string) {
	type hit struct {
		page *confPage
		cm   confComment
	}
	var hits []hit
	for _, p := range f.pages {
		if !commentSpaceMatch(cql, p.Space) {
			continue
		}
		var walk func(confComment)
		walk = func(cm confComment) {
			if !commentWhenMatch(cql, cm.When) {
				return
			}
			hits = append(hits, hit{page: p, cm: cm})
			for _, r := range cm.Replies {
				walk(r)
			}
		}
		for _, cm := range p.Comments {
			walk(cm)
		}
	}
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].cm.When < hits[i].cm.When ||
				(hits[j].cm.When == hits[i].cm.When && hits[j].cm.ID < hits[i].cm.ID) {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	results := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		results = append(results, map[string]any{
			"id": h.cm.ID, "type": "comment", "status": "current",
			"title": "Re: " + h.page.Title,
			"space": map[string]any{"key": h.page.Space, "name": f.spaceName(h.page.Space)},
			"version": map[string]any{
				"number": 1, "when": h.cm.When,
				"by": map[string]any{"accountId": "acc-2", "displayName": "Bob Example"},
			},
			"_links": map[string]string{
				"webui": fmt.Sprintf("/spaces/%s/pages/%s?focusedCommentId=%s",
					h.page.Space, h.page.ID, h.cm.ID),
			},
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results": results,
		"_links":  map[string]string{},
	})
}

func commentSpaceMatch(cql, space string) bool {
	if i := strings.Index(cql, `space="`); i >= 0 {
		rest := cql[i+len(`space="`):]
		end := strings.Index(rest, `"`)
		if end >= 0 && space != rest[:end] {
			return false
		}
	}
	return true
}

func commentWhenMatch(cql, when string) bool {
	i := strings.Index(cql, `lastModified >= "`)
	if i < 0 {
		return true
	}
	rest := cql[i+len(`lastModified >= "`):]
	end := strings.Index(rest, `"`)
	if end <= 0 {
		return true
	}
	return !pageWhenBeforeFloor(when, rest[:end])
}

func cqlMatch(cql string, p *confPage) bool {
	// Very small CQL interpreter for tests. Space keys arrive quoted
	// (space="AAA") since the always-quote fix for digit-leading keys.
	if i := strings.Index(cql, `space="`); i >= 0 {
		rest := cql[i+len(`space="`):]
		end := strings.Index(rest, `"`)
		if end >= 0 && p.Space != rest[:end] {
			return false
		}
	}
	if strings.Contains(cql, "space in (") {
		// e.g. space in (AAA, BBB) — legacy chunk form.
		if !strings.Contains(cql, p.Space) {
			return false
		}
	}
	if i := strings.Index(cql, `lastModified >= "`); i >= 0 {
		rest := cql[i+len(`lastModified >= "`):]
		end := strings.Index(rest, `"`)
		if end > 0 {
			floor := rest[:end] // "2006-01-02 15:04"
			// Compare page when (ISO) as date-time roughly: parse both loosely.
			if pageWhenBeforeFloor(p.When, floor) {
				return false
			}
		}
	}
	return true
}

func pageWhenBeforeFloor(when, floor string) bool {
	// floor: 2006-01-02 15:04  when: 2006-01-02T15:04:05.000Z
	wt, err1 := time.Parse("2006-01-02T15:04:05.000Z", when)
	if err1 != nil {
		wt, err1 = time.Parse(time.RFC3339, when)
	}
	ft, err2 := time.Parse("2006-01-02 15:04", floor)
	if err1 != nil || err2 != nil {
		return when < floor // string fallback
	}
	return wt.Before(ft)
}

func (f *confFixture) serveComments(w http.ResponseWriter, path string) {
	// /wiki/rest/api/content/{id}/child/comment
	trim := strings.TrimPrefix(path, "/wiki/rest/api/content/")
	id := strings.TrimSuffix(trim, "/child/comment")
	// Page-level comments
	if p := f.pages[id]; p != nil {
		var results []map[string]any
		for _, c := range p.Comments {
			results = append(results, confCommentJSON(c))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results, "size": len(results), "limit": 100})
		return
	}
	// Reply level: find comment by id
	for _, p := range f.pages {
		for _, c := range p.Comments {
			if c.ID == id {
				var results []map[string]any
				for _, r := range c.Replies {
					results = append(results, confCommentJSON(r))
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"results": results, "size": len(results), "limit": 100})
				return
			}
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "size": 0, "limit": 100})
}

func confCommentJSON(c confComment) map[string]any {
	adf, _ := json.Marshal(map[string]any{
		"type": "doc", "version": 1,
		"content": []any{
			map[string]any{"type": "paragraph", "content": []any{
				map[string]any{"type": "text", "text": c.Text},
			}},
		},
	})
	return map[string]any{
		"id": c.ID, "type": "comment", "title": "Re:",
		"body": map[string]any{
			"atlas_doc_format": map[string]any{"value": string(adf), "representation": "atlas_doc_format"},
		},
		"version": map[string]any{
			"number": 1, "when": c.When,
			"by": map[string]any{"accountId": "acc-2", "displayName": "Bob Example"},
		},
	}
}

func (f *confFixture) spaceName(key string) string {
	for _, s := range f.spaces {
		if s["key"] == key {
			if n, ok := s["name"].(string); ok {
				return n
			}
		}
	}
	return key
}

func (f *confFixture) fullPage(p *confPage) map[string]any {
	results := make([]map[string]any, 0, len(p.Labels))
	for i, name := range p.Labels {
		results = append(results, map[string]any{
			"name": name, "prefix": "global", "id": fmt.Sprintf("%d", i+1),
		})
	}
	return map[string]any{
		"id": p.ID, "type": "page", "status": "current", "title": p.Title,
		"space": map[string]any{"key": p.Space, "name": f.spaceName(p.Space)},
		"version": map[string]any{
			"number": p.Version, "when": p.When,
			"by": map[string]any{"accountId": "acc-1", "displayName": "Ada Example"},
		},
		"body": map[string]any{
			"atlas_doc_format": map[string]any{"value": p.BodyADF, "representation": "atlas_doc_format"},
		},
		"ancestors": []any{},
		"metadata": map[string]any{
			"labels": map[string]any{
				"results": results,
				"size":    len(results),
				"limit":   25,
				"start":   0,
			},
		},
	}
}

func confCfg(spaces []string) *config.Config {
	return &config.Config{
		Site:  "https://example.invalid",
		Email: "user@example.invalid",
		Token: "secret-token",
		Confluence: &config.ConfluenceConfig{
			Spaces: spaces,
		},
	}
}

func (m *mirror) raw(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+m.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestConfluenceFullSyncMapsPagesAndFTS(t *testing.T) {
	f := newConfFixture(t)
	client := f.start()
	db := newMirror(t)
	// Only AAA so we get 2 pages + 1 comment.
	cfg := confCfg([]string{"AAA"})

	res, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true, ConfluenceClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 2 {
		t.Fatalf("fetched = %d, want 2", res.Fetched)
	}
	if !res.Full {
		t.Error("expected full sync")
	}

	raw := db.raw(t)
	var items, pages, comments int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM items WHERE kind = 'page'`).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM pages`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if items != 2 || pages != 2 {
		t.Fatalf("items=%d pages=%d, want 2/2", items, pages)
	}
	if comments != 1 {
		t.Fatalf("comments = %d, want 1", comments)
	}

	// Korean prefix FTS over body.
	var hits int
	if err := raw.QueryRow(`
		SELECT COUNT(*) FROM items_fts f
		JOIN items it ON it.rowid = f.rowid
		WHERE items_fts MATCH '"로그인"*' AND it.kind = 'page'`).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits < 1 {
		t.Fatalf("FTS prefix 로그인* hits = %d, want >= 1", hits)
	}

	st, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Watermark == "" {
		t.Error("watermark not set after full sync")
	}

	// Labels: API order was runbook,ops → stored alphabetically as ops,runbook.
	var labelsJSON string
	if err := raw.QueryRow(`SELECT labels FROM pages WHERE item_id = 'confluence:1001'`).Scan(&labelsJSON); err != nil {
		t.Fatal(err)
	}
	var labels []string
	if err := json.Unmarshal([]byte(labelsJSON), &labels); err != nil {
		t.Fatalf("labels JSON: %v (%s)", err, labelsJSON)
	}
	if len(labels) != 2 || labels[0] != "ops" || labels[1] != "runbook" {
		t.Errorf("page 1001 labels = %v, want [ops runbook] (sorted)", labels)
	}
	// Page without labels → empty JSON array.
	if err := raw.QueryRow(`SELECT labels FROM pages WHERE item_id = 'confluence:1002'`).Scan(&labelsJSON); err != nil {
		t.Fatal(err)
	}
	if labelsJSON != "[]" {
		t.Errorf("page 1002 labels = %q, want []", labelsJSON)
	}

	// Path ②: config listed spaces → names collected from page hits at batch commit.
	var spaceName string
	if err := raw.QueryRow(`SELECT name FROM spaces WHERE source_id = 'confluence' AND key = 'AAA'`).Scan(&spaceName); err != nil {
		t.Fatalf("spaces row for AAA: %v", err)
	}
	if spaceName != "Alpha" {
		t.Errorf("space AAA name = %q, want Alpha (from page hit Space.Name)", spaceName)
	}
	lites, err := db.PageLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range lites {
		if p.SpaceKey == "AAA" && p.SpaceName != "Alpha" {
			t.Errorf("PageLite %s SpaceName = %q, want Alpha", p.Key, p.SpaceName)
		}
	}
}

// TestConfluenceSpacesFromListing is FAIL-first for path ①: empty config.spaces
// calls Spaces() and UpsertSpaces with key/name/kind/homepage from the listing.
func TestConfluenceSpacesFromListing(t *testing.T) {
	f := newConfFixture(t)
	// Opaque Cloud-style key with a human name — the point of the feature.
	f.spaces = []map[string]any{
		{"key": "3dvBrsa61dIo", "name": "Engineering", "type": "global",
			"homepage": map[string]any{"id": "9001"}},
		{"key": "~personal", "name": "Ada personal", "type": "personal"},
	}
	f.pages = map[string]*confPage{
		"9001": {
			ID: "9001", Space: "3dvBrsa61dIo", Title: "Root",
			Version: 1, When: "2026-08-01T10:00:00.000Z",
			BodyADF: `{"type":"doc","version":1,"content":[]}`,
		},
	}
	// CQL matcher only knows AAA/BBB — extend match for this key via space in / exact.
	client := f.start()
	db := newMirror(t)
	cfg := confCfg(nil) // empty → Spaces()

	res, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true, ConfluenceClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 1 {
		t.Fatalf("fetched = %d, want 1", res.Fetched)
	}
	raw := db.raw(t)
	var name, kind, homepageID string
	if err := raw.QueryRow(`SELECT name, kind, homepage_id FROM spaces WHERE source_id = 'confluence' AND key = '3dvBrsa61dIo'`).Scan(&name, &kind, &homepageID); err != nil {
		t.Fatalf("spaces row: %v", err)
	}
	if name != "Engineering" || kind != "global" {
		t.Errorf("space = name=%q kind=%q, want Engineering/global", name, kind)
	}
	if homepageID != "9001" {
		t.Errorf("homepage_id = %q, want 9001", homepageID)
	}
	// Empty config = global page scope; personal space rows are pruned unless
	// the key is named in config.spaces (same rule as page fetch).
	var personalRows int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM spaces WHERE source_id = 'confluence' AND key = '~personal'`).Scan(&personalRows); err != nil {
		t.Fatal(err)
	}
	if personalRows != 0 {
		t.Errorf("personal space rows after empty-config prune = %d, want 0", personalRows)
	}
	pages, err := db.PageLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].SpaceName != "Engineering" {
		t.Errorf("PageLites = %+v, want SpaceName Engineering", pages)
	}
	if pages[0].SpaceHomepageID != "9001" {
		t.Errorf("PageLites SpaceHomepageID = %q, want 9001", pages[0].SpaceHomepageID)
	}
}

// TestConfluenceEmptyConfigSecondPassDoesNotBumpVersion is FAIL-first for the
// path-① insert/prune thrash: listing still contains a personal space, but a
// quiet second pass must not bump sync_state.version (that invalidates the
// client ETag every Watch cycle). The space watermark is walked past the only
// page so this assertion is not the C4 overlap rewrite.
func TestConfluenceEmptyConfigSecondPassDoesNotBumpVersion(t *testing.T) {
	f := newConfFixture(t)
	f.spaces = []map[string]any{
		{"key": "3dvBrsa61dIo", "name": "Engineering", "type": "global",
			"homepage": map[string]any{"id": "9001"}},
		{"key": "~personal", "name": "Ada personal", "type": "personal"},
	}
	f.pages = map[string]*confPage{
		"9001": {
			ID: "9001", Space: "3dvBrsa61dIo", Title: "Root",
			Version: 1, When: "2026-08-01T10:00:00.000Z",
			BodyADF: `{"type":"doc","version":1,"content":[]}`,
		},
	}
	client := f.start()
	db := newMirror(t)
	cfg := confCfg(nil)
	ctx := context.Background()

	if _, err := RunConfluence(ctx, cfg, db.DB, Options{Full: true, ConfluenceClient: client}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSpaceWatermark(ctx, ConfluenceSourceID, "3dvBrsa61dIo", "2026-08-01T12:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	st1, err := db.SyncState(ctx, ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st1.Version == 0 {
		t.Fatal("precondition: first pass must have moved version")
	}

	if _, err := RunConfluence(ctx, cfg, db.DB, Options{ConfluenceClient: client}); err != nil {
		t.Fatal(err)
	}
	st2, err := db.SyncState(ctx, ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st2.Version != st1.Version {
		t.Fatalf("second empty-config pass bumped version %d -> %d", st1.Version, st2.Version)
	}
}

// TestConfluenceSpacesFromConfigGET is FAIL-first for path ②: config lists
// spaces → per-space GET with expand=homepage stores homepage_id.
func TestConfluenceSpacesFromConfigGET(t *testing.T) {
	f := newConfFixture(t)
	f.spaces = []map[string]any{
		{"key": "AAA", "name": "Alpha", "type": "global",
			"homepage": map[string]any{"id": "1000"}},
		{"key": "BBB", "name": "Beta", "type": "global",
			"homepage": map[string]any{"id": "2000"}},
	}
	client := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA", "BBB"})

	if _, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true, ConfluenceClient: client,
	}); err != nil {
		t.Fatal(err)
	}
	raw := db.raw(t)
	for _, want := range []struct{ key, hp string }{
		{"AAA", "1000"},
		{"BBB", "2000"},
	} {
		var hp string
		if err := raw.QueryRow(`SELECT homepage_id FROM spaces WHERE source_id = 'confluence' AND key = ?`, want.key).Scan(&hp); err != nil {
			t.Fatalf("space %s: %v", want.key, err)
		}
		if hp != want.hp {
			t.Errorf("space %s homepage_id = %q, want %q", want.key, hp, want.hp)
		}
	}
}

// TestConfluenceSpaceGET404DoesNotFailSync: a missing/restricted space key
// is logged and skipped; the rest of the run still succeeds.
func TestConfluenceSpaceGET404DoesNotFailSync(t *testing.T) {
	f := newConfFixture(t)
	// Only AAA is in the mock listing; NOPE will 404 on single-space GET.
	f.spaces = []map[string]any{
		{"key": "AAA", "name": "Alpha", "type": "global",
			"homepage": map[string]any{"id": "1000"}},
	}
	// Restrict pages to AAA so BBB is not required.
	f.pages = map[string]*confPage{
		"1001": f.pages["1001"],
		"1002": f.pages["1002"],
	}
	client := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA", "NOPE"})
	var logs []string
	res, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true, ConfluenceClient: client,
		Log: func(line string) {
			logs = append(logs, line)
		},
	})
	if err != nil {
		t.Fatalf("sync failed on space 404: %v", err)
	}
	if res.Fetched < 1 {
		t.Fatalf("fetched = %d, want ≥ 1 (AAA pages still synced)", res.Fetched)
	}
	raw := db.raw(t)
	var hp string
	if err := raw.QueryRow(`SELECT homepage_id FROM spaces WHERE source_id = 'confluence' AND key = 'AAA'`).Scan(&hp); err != nil {
		t.Fatal(err)
	}
	if hp != "1000" {
		t.Errorf("AAA homepage_id = %q, want 1000", hp)
	}
	// NOPE must not have a spaces row from the failed GET.
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM spaces WHERE source_id = 'confluence' AND key = 'NOPE'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("NOPE spaces rows = %d, want 0", n)
	}
	logged := false
	for _, line := range logs {
		if strings.Contains(line, "NOPE") {
			logged = true
			break
		}
	}
	if !logged {
		t.Errorf("expected log mentioning NOPE, got %v", logs)
	}
}

func TestConfluenceIncrementalAdvancesWatermark(t *testing.T) {
	f := newConfFixture(t)
	client := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA"})

	// First full.
	if _, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true, ConfluenceClient: client,
	}); err != nil {
		t.Fatal(err)
	}
	st1, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}

	// Bump one page after the watermark.
	f.mu.Lock()
	f.pages["1001"].Version = 3
	f.pages["1001"].When = "2026-08-03T12:00:00.000Z"
	f.pages["1001"].BodyADF = `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"updated body"}]}]}`
	f.mu.Unlock()

	res, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		ConfluenceClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full {
		t.Error("second run must be incremental")
	}
	if res.Fetched < 1 {
		t.Fatalf("incremental fetched = %d, want >= 1", res.Fetched)
	}
	st2, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st2.Watermark <= st1.Watermark {
		t.Fatalf("watermark did not advance: %q -> %q", st1.Watermark, st2.Watermark)
	}

	raw := db.raw(t)
	var body string
	if err := raw.QueryRow(`SELECT body_text FROM items WHERE key = '1001'`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "updated body") {
		t.Errorf("body not updated: %q", body)
	}
}

// TestConfluenceCommentsOnlyPassWithoutPageTouch is FAIL-first for C2: a new
// comment whose page lastModified did not move must still land in the mirror.
// Decision 0006's comments-only pass; cost cap is one type=comment CQL per
// incremental space (not a full-space refetch).
func TestConfluenceCommentsOnlyPassWithoutPageTouch(t *testing.T) {
	f := newConfFixture(t)
	client := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA"})

	if _, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true, ConfluenceClient: client,
	}); err != nil {
		t.Fatal(err)
	}
	raw := db.raw(t)
	var before int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM comments WHERE item_id = 'confluence:1001'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 1 {
		t.Fatalf("comments before = %d, want 1", before)
	}

	// Page version/when stay put. Only a new comment appears.
	f.mu.Lock()
	f.pages["1001"].Comments = append(f.pages["1001"].Comments, confComment{
		ID: "c1001c", Text: "comment without page edit", When: "2026-08-10T09:00:00.000Z",
	})
	f.mu.Unlock()

	var logs []string
	if _, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		ConfluenceClient: client,
		Log:              func(line string) { logs = append(logs, line) },
	}); err != nil {
		t.Fatal(err)
	}

	var after int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM comments WHERE item_id = 'confluence:1001'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != 2 {
		t.Fatalf("C2: comments after = %d, want 2 (comment-only pass missed)", after)
	}
	var body string
	if err := raw.QueryRow(`SELECT body_text FROM comments WHERE id = 'confluence:c1001c'`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "comment without page edit") {
		t.Errorf("new comment body = %q", body)
	}

	// Cost cap: incremental AAA must issue a type=comment CQL (one extra
	// search, not one GET per page in the space).
	var commentCQL int
	for _, cql := range f.searches {
		if strings.Contains(cql, `space="AAA"`) && strings.Contains(cql, "type=comment") {
			commentCQL++
		}
	}
	if commentCQL != 1 {
		t.Errorf("C2: type=comment CQL count = %d, want 1; searches=%v", commentCQL, f.searches)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "comments-only") {
		t.Errorf("C2: missing comments-only count log, got %v", logs)
	}
}

func TestConfluenceCommentOnlyChange(t *testing.T) {
	f := newConfFixture(t)
	client := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA"})

	if _, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true, ConfluenceClient: client,
	}); err != nil {
		t.Fatal(err)
	}
	raw := db.raw(t)
	var before int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM comments WHERE item_id = 'confluence:1001'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 1 {
		t.Fatalf("comments before = %d, want 1", before)
	}

	// Same version on the page, one more comment. Nudge lastModified so the
	// incremental CQL hit includes the page; sync re-fetches comments even
	// when version is unchanged (comments do not bump page version).
	f.mu.Lock()
	f.pages["1001"].When = "2026-08-04T08:00:00.000Z"
	f.pages["1001"].Comments = append(f.pages["1001"].Comments, confComment{
		ID: "c1001b", Text: "new comment only", When: "2026-08-04T08:01:00.000Z",
	})
	f.mu.Unlock()

	res, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		ConfluenceClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched < 1 {
		t.Fatalf("fetched = %d", res.Fetched)
	}

	var after int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM comments WHERE item_id = 'confluence:1001'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != 2 {
		t.Fatalf("comments after = %d, want 2", after)
	}
	var body string
	if err := raw.QueryRow(`SELECT body_text FROM comments WHERE id = 'confluence:c1001b'`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "new comment only") {
		t.Errorf("new comment body = %q", body)
	}
}

func TestConfluence429ThenSucceeds(t *testing.T) {
	f := newConfFixture(t)
	f.rateLimitOnce.Store(true)
	client := f.start()
	db := newMirror(t)
	// Empty spaces → Spaces() which hits 429 once.
	cfg := confCfg(nil)

	res, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true, ConfluenceClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	// All three pages across AAA+BBB.
	if res.Fetched != 3 {
		t.Fatalf("fetched = %d, want 3 after 429 retry", res.Fetched)
	}
}

// TestConfluenceRunFlushesAPIUsage: RunConfluence drains the client's TakeUsage
// into store.api_usage the same way Run does for Jira (shared flushAPIUsage).
func TestConfluenceRunFlushesAPIUsage(t *testing.T) {
	f := newConfFixture(t)
	client := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA"})

	if _, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true, ConfluenceClient: client,
	}); err != nil {
		t.Fatal(err)
	}
	days, err := db.APIUsage(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 {
		t.Fatalf("api_usage rows = %d, want 1", len(days))
	}
	if days[0].Requests < 1 {
		t.Errorf("requests = %d, want at least the search/page calls", days[0].Requests)
	}
	if u := client.Usage(); u.Requests != 0 {
		t.Errorf("client still holds %d requests after flush", u.Requests)
	}
}

// TestConfluenceSyncRunKindReconcileSuffix: space prune is the Confluence
// reconcile, so SupportsReconcile=true — full stamps "full+reconcile".
func TestConfluenceSyncRunKindReconcileSuffix(t *testing.T) {
	f := newConfFixture(t)
	client := f.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA"})

	if _, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true, ConfluenceClient: client,
	}); err != nil {
		t.Fatal(err)
	}
	runs, err := db.SyncRuns(context.Background(), ConfluenceSourceID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) < 1 {
		t.Fatal("expected a SyncRun after confluence full")
	}
	if runs[0].Kind != "full+reconcile" {
		t.Fatalf("confluence full kind = %q, want full+reconcile", runs[0].Kind)
	}
}

// TestConfluenceIncrementalPerSpaceCQL: AAA has a watermark → lastModified
// floor; BBB has none → full-backfill CQL (no lastModified). No space-in chunks.
func TestConfluenceIncrementalPerSpaceCQL(t *testing.T) {
	f := newConfFixture(t)
	client := f.start()
	db := newMirror(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: ConfluenceSourceID, Kind: "confluence", BaseURL: client.BaseURL()}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{Watermark: "2026-08-01T00:00:00.000Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSpaces(ctx, ConfluenceSourceID, []store.SpaceRow{
		{Key: "AAA", Name: "Alpha", Kind: "global"},
		{Key: "BBB", Name: "Beta", Kind: "global"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSpaceWatermark(ctx, ConfluenceSourceID, "AAA", "2026-01-01T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}

	var logs []string
	if _, err := RunConfluence(ctx, confCfg([]string{"AAA", "BBB"}), db.DB, Options{
		ConfluenceClient: client,
		Log:              func(line string) { logs = append(logs, line) },
	}); err != nil {
		t.Fatal(err)
	}

	var aaaPage, bbbPage, aaaComment, bbbComment string
	pageCalls, commentCalls := 0, 0
	for _, cql := range f.searches {
		if strings.Contains(cql, "space in (") {
			t.Errorf("chunked space-in CQL still used: %s", cql)
		}
		switch {
		case strings.Contains(cql, "type=comment"):
			commentCalls++
			if strings.Contains(cql, `space="AAA"`) {
				aaaComment = cql
			}
			if strings.Contains(cql, `space="BBB"`) {
				bbbComment = cql
			}
		case strings.Contains(cql, "type=page"):
			pageCalls++
			if strings.Contains(cql, `space="AAA"`) {
				aaaPage = cql
			}
			if strings.Contains(cql, `space="BBB"`) {
				bbbPage = cql
			}
		}
	}
	if pageCalls != 2 {
		t.Errorf("type=page SearchPages = %d, want 2 (one per space); searches=%v", pageCalls, f.searches)
	}
	if aaaPage == "" || bbbPage == "" {
		t.Fatalf("missing per-space page CQL: searches=%v", f.searches)
	}
	wantFloor := cqlTime("2026-01-01T00:00:00.000Z")
	if !strings.Contains(aaaPage, `lastModified >= "`+wantFloor+`"`) {
		t.Errorf("AAA page CQL = %q, want lastModified >= %q", aaaPage, wantFloor)
	}
	if strings.Contains(bbbPage, "lastModified") {
		t.Errorf("BBB page CQL = %q, want full-backfill (no lastModified)", bbbPage)
	}
	// Incremental AAA also runs one comments-only CQL; backfill BBB must not.
	if commentCalls != 1 || aaaComment == "" {
		t.Errorf("comments-only CQL: count=%d aaa=%q searches=%v", commentCalls, aaaComment, f.searches)
	}
	if bbbComment != "" {
		t.Errorf("BBB backfill must not issue type=comment CQL: %s", bbbComment)
	}
	if aaaComment != "" && !strings.Contains(aaaComment, `lastModified >= "`+wantFloor+`"`) {
		t.Errorf("AAA comment CQL = %q, want same floor %q", aaaComment, wantFloor)
	}

	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "confluence: space AAA floor=") {
		t.Errorf("missing AAA floor log, got %v", logs)
	}
	if !strings.Contains(joined, "confluence: space BBB floor=full-backfill") {
		t.Errorf("missing BBB full-backfill log, got %v", logs)
	}
}

// TestConfluenceSpaceWatermarkIsolation: A succeeds, B's SearchPages fails.
// A's per-space watermark advances; B stays empty; the next run still
// full-backfills B (global watermark must not become B's floor).
func TestConfluenceSpaceWatermarkIsolation(t *testing.T) {
	f := newConfFixture(t)
	f.failIfCQLContains = `space="BBB"`
	client := f.start()
	db := newMirror(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: ConfluenceSourceID, Kind: "confluence", BaseURL: client.BaseURL()}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{Watermark: "2026-08-01T00:00:00.000Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSpaces(ctx, ConfluenceSourceID, []store.SpaceRow{
		{Key: "AAA", Name: "Alpha", Kind: "global"},
		{Key: "BBB", Name: "Beta", Kind: "global"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSpaceWatermark(ctx, ConfluenceSourceID, "AAA", "2026-01-01T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}

	_, err := RunConfluence(ctx, confCfg([]string{"AAA", "BBB"}), db.DB, Options{
		ConfluenceClient: client,
	})
	if err == nil {
		t.Fatal("expected BBB search to fail the pass")
	}

	wms, err := db.ConfluenceSpaceWatermarks(ctx, ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if wms["AAA"] == "" || wms["AAA"] == "2026-01-01T00:00:00.000Z" {
		t.Errorf("AAA watermark not advanced after success: %q", wms["AAA"])
	}
	if wms["BBB"] != "" {
		t.Errorf("BBB watermark = %q, want empty (failed space must stay unset)", wms["BBB"])
	}

	f.failIfCQLContains = ""
	f.searches = nil
	if _, err := RunConfluence(ctx, confCfg([]string{"AAA", "BBB"}), db.DB, Options{
		ConfluenceClient: client,
	}); err != nil {
		t.Fatal(err)
	}
	var bbb string
	for _, cql := range f.searches {
		if strings.Contains(cql, `space="BBB"`) {
			bbb = cql
		}
	}
	if bbb == "" {
		t.Fatalf("second run issued no BBB CQL: %v", f.searches)
	}
	if strings.Contains(bbb, "lastModified") {
		t.Errorf("second-run BBB CQL = %q; global/A watermark leaked into B's floor", bbb)
	}
}

// TestConfluenceSyncPrunesOutOfScopeSpaces: config AAA only, mirror already
// holds AAA+BBB+CCC pages → after a pass only AAA remains.
func TestConfluenceSyncPrunesOutOfScopeSpaces(t *testing.T) {
	f := newConfFixture(t)
	client := f.start()
	db := newMirror(t)
	ctx := context.Background()
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	if err := db.UpsertSource(ctx, store.Source{ID: ConfluenceSourceID, Kind: "confluence", BaseURL: client.BaseURL()}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertPages(ctx, []store.PageRecord{
		{
			Item: store.Item{
				ID: "confluence:a1", SourceID: ConfluenceSourceID, Kind: "page",
				ExternalID: "a1", Key: "a1", Title: "A page", BodyText: "a",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Page: store.Page{SpaceKey: "AAA", Version: 1, Status: "current", BodyADF: adf},
		},
		{
			Item: store.Item{
				ID: "confluence:b1", SourceID: ConfluenceSourceID, Kind: "page",
				ExternalID: "b1", Key: "b1", Title: "B page", BodyText: "b",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Page: store.Page{SpaceKey: "BBB", Version: 1, Status: "current", BodyADF: adf},
		},
		{
			Item: store.Item{
				ID: "confluence:c1", SourceID: ConfluenceSourceID, Kind: "page",
				ExternalID: "c1", Key: "c1", Title: "C page", BodyText: "c",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Page: store.Page{SpaceKey: "CCC", Version: 1, Status: "current", BodyADF: adf},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSpaces(ctx, ConfluenceSourceID, []store.SpaceRow{
		{Key: "AAA", Name: "Alpha", Kind: "global"},
		{Key: "BBB", Name: "Beta", Kind: "global"},
		{Key: "CCC", Name: "Gamma", Kind: "global"},
	}); err != nil {
		t.Fatal(err)
	}

	var logs []string
	res, err := RunConfluence(ctx, confCfg([]string{"AAA"}), db.DB, Options{
		Full: true, ConfluenceClient: client,
		Log: func(line string) { logs = append(logs, line) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted < 2 {
		t.Fatalf("Deleted = %d, want ≥ 2 (BBB+CCC)", res.Deleted)
	}

	raw := db.raw(t)
	rows, err := raw.Query(`SELECT space_key, COUNT(*) FROM pages GROUP BY space_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			t.Fatal(err)
		}
		got[k] = n
	}
	if len(got) != 1 || got["AAA"] < 1 {
		t.Errorf("pages by space = %v, want only AAA", got)
	}

	wrows, err := raw.Query(`SELECT key, watermark FROM spaces WHERE source_id = 'confluence' ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	defer wrows.Close()
	var keys []string
	for wrows.Next() {
		var k string
		var wm sql.NullString
		if err := wrows.Scan(&k, &wm); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
		if k == "AAA" && (!wm.Valid || wm.String == "") {
			t.Error("AAA watermark empty after full pass")
		}
	}
	if len(keys) != 1 || keys[0] != "AAA" {
		t.Errorf("spaces keys = %v, want [AAA]", keys)
	}

	prunedLog := false
	for _, line := range logs {
		if strings.Contains(line, "confluence: pruned") && strings.Contains(line, "AAA") {
			prunedLog = true
		}
	}
	if !prunedLog {
		t.Errorf("missing prune log, got %v", logs)
	}
}

// TestConfluenceNamedPersonalSpaceIsKept: a personal key named in config
// survives prune (empty-config would drop it).
func TestConfluenceNamedPersonalSpaceIsKept(t *testing.T) {
	f := newConfFixture(t)
	f.spaces = []map[string]any{
		{"key": "AAA", "name": "Alpha", "type": "global"},
		{"key": "~personal", "name": "Ada personal", "type": "personal"},
	}
	f.pages["9001"] = &confPage{
		ID: "9001", Space: "~personal", Title: "My notes",
		Version: 1, When: "2026-08-01T10:00:00.000Z",
		BodyADF: `{"type":"doc","version":1,"content":[]}`,
	}
	client := f.start()
	db := newMirror(t)
	if _, err := RunConfluence(context.Background(), confCfg([]string{"AAA", "~personal"}), db.DB, Options{
		Full: true, ConfluenceClient: client,
	}); err != nil {
		t.Fatal(err)
	}
	raw := db.raw(t)
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM spaces WHERE source_id = 'confluence' AND key = '~personal'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("named personal space rows = %d, want 1", n)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM pages WHERE space_key = '~personal'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("named personal pages = %d, want 1", n)
	}
}
