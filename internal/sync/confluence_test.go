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

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/confluence"

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
			"space": map[string]any{"key": p.Space, "name": p.Space},
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

func cqlMatch(cql string, p *confPage) bool {
	// Very small CQL interpreter for tests. Space keys arrive quoted
	// (space="AAA") since the always-quote fix for digit-leading keys.
	if strings.Contains(cql, `space="AAA"`) {
		if p.Space != "AAA" {
			return false
		}
	}
	if strings.Contains(cql, `space="BBB"`) {
		if p.Space != "BBB" {
			return false
		}
	}
	if strings.Contains(cql, "space in (") {
		// e.g. space in (AAA, BBB)
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

func (f *confFixture) fullPage(p *confPage) map[string]any {
	results := make([]map[string]any, 0, len(p.Labels))
	for i, name := range p.Labels {
		results = append(results, map[string]any{
			"name": name, "prefix": "global", "id": fmt.Sprintf("%d", i+1),
		})
	}
	return map[string]any{
		"id": p.ID, "type": "page", "status": "current", "title": p.Title,
		"space": map[string]any{"key": p.Space, "name": p.Space},
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

	st, err := db.SyncState(ConfluenceSourceID)
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
	st1, err := db.SyncState(ConfluenceSourceID)
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
	st2, err := db.SyncState(ConfluenceSourceID)
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
