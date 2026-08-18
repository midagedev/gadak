package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/confluence"
)

// wikiHistStub is a one-space, one-page Confluence stand-in that can serve
// version history (optionally failing it) and count those GETs.
type wikiHistStub struct {
	t *testing.T

	mu           sync.Mutex
	pageVer      int
	when         string
	body         string
	hist         []histVer
	failHist     bool
	histCalls    int
	histOrderRev bool
}

type histVer struct {
	Number  int
	When    string
	Message string
	Minor   bool
	Acc     string
	Name    string
}

func (s *wikiHistStub) start() *confluence.Client {
	srv := httptest.NewServer(s)
	s.t.Cleanup(srv.Close)
	c := confluence.New(srv.URL, "user@example.invalid", "secret-token")
	c.Retries, c.Backoff, c.PauseBetween = 2, time.Millisecond, 0
	return c
}

func (s *wikiHistStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := r.URL.Path
	switch {
	case path == "/wiki/rest/api/space/AAA":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key": "AAA", "name": "Alpha", "type": "global",
			"homepage": map[string]any{"id": "1001"},
		})
	case path == "/wiki/rest/api/content/search":
		cql := r.URL.Query().Get("cql")
		if strings.Contains(cql, "type=comment") {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "_links": map[string]string{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"id": "1001", "type": "page", "status": "current", "title": "Notes",
					"space": map[string]any{"key": "AAA", "name": "Alpha"},
					"version": map[string]any{
						"number": s.pageVer, "when": s.when,
						"by": map[string]any{"accountId": "acc-1", "displayName": "Ada"},
					},
				},
			},
			"_links": map[string]string{},
		})
	case strings.HasSuffix(path, "/child/comment"):
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "size": 0, "limit": 100})
	case path == "/wiki/rest/api/content/1001/version":
		s.histCalls++
		if s.failHist {
			http.Error(w, "history boom", http.StatusInternalServerError)
			return
		}
		rows := append([]histVer(nil), s.hist...)
		if s.histOrderRev {
			for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
		results := make([]map[string]any, 0, len(rows))
		for _, v := range rows {
			results = append(results, map[string]any{
				"number": v.Number, "when": v.When, "message": v.Message, "minorEdit": v.Minor,
				"by": map[string]any{"accountId": v.Acc, "displayName": v.Name},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results, "_links": map[string]string{}})
	case path == "/wiki/rest/api/content/1001":
		adf := s.body
		if adf == "" {
			adf = `{"type":"doc","version":1,"content":[]}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "1001", "type": "page", "status": "current", "title": "Notes",
			"space": map[string]any{"key": "AAA", "name": "Alpha"},
			"version": map[string]any{
				"number": s.pageVer, "when": s.when,
				"by": map[string]any{"accountId": "acc-1", "displayName": "Ada"},
			},
			"body": map[string]any{
				"atlas_doc_format": map[string]any{"value": adf, "representation": "atlas_doc_format"},
			},
			"ancestors": []any{},
			"metadata":  map[string]any{"labels": map[string]any{"results": []any{}}},
		})
	default:
		s.t.Errorf("unexpected %s %s", r.Method, path)
		http.NotFound(w, r)
	}
}

func (s *wikiHistStub) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.histCalls
}

// TestHistoryFetchFailureDoesNotFailSync is FAIL-first: a 500 on
// GET /content/{id}/version must leave the rest of the pass green (pages
// written). House pattern: handleWriteMeta / groupQuery — log and continue.
func TestHistoryFetchFailureDoesNotFailSync(t *testing.T) {
	s := &wikiHistStub{
		t: t, pageVer: 2, when: "2026-08-01T10:00:00.000Z", failHist: true,
		hist: []histVer{{Number: 1}, {Number: 2}},
	}
	client := s.start()
	db := newMirror(t)
	var logs []string
	res, err := RunConfluence(context.Background(), confCfg([]string{"AAA"}), db.DB, Options{
		Full: true, ConfluenceClient: client,
		Log: func(line string) { logs = append(logs, line) },
	})
	if err != nil {
		t.Fatalf("sync failed because history failed: %v", err)
	}
	if res.Fetched != 1 {
		t.Fatalf("fetched = %d, want 1 (page itself must still land)", res.Fetched)
	}
	raw := db.raw(t)
	var pages int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM pages`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if pages != 1 {
		t.Fatalf("pages = %d, want 1", pages)
	}
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM page_versions`).Scan(&n); err != nil {
		t.Fatalf("page_versions: %v", err)
	}
	if n != 0 {
		t.Errorf("page_versions rows = %d, want 0 after failed fetch", n)
	}
	logged := false
	for _, line := range logs {
		if strings.Contains(line, "page versions") || strings.Contains(line, "version") {
			logged = true
			break
		}
	}
	if !logged {
		t.Errorf("expected a degrade log for history fetch, got %v", logs)
	}
}

// TestPageVersionsCollectedSortedAndSkippedWhenUnchanged is FAIL-first for
// collect + incremental: reversed payload is stored in number order; a second
// pass over an unchanged version must not hit the history endpoint again.
func TestPageVersionsCollectedSortedAndSkippedWhenUnchanged(t *testing.T) {
	s := &wikiHistStub{
		t: t, pageVer: 2, when: "2026-08-01T10:00:00.000Z", histOrderRev: true,
		hist: []histVer{
			{Number: 1, When: "2026-08-01T09:00:00.000Z", Message: "create", Acc: "acc-a", Name: "Ada"},
			{Number: 2, When: "2026-08-01T10:00:00.000Z", Message: "edit", Acc: "acc-b", Name: "Bob", Minor: true},
		},
	}
	client := s.start()
	db := newMirror(t)
	cfg := confCfg([]string{"AAA"})
	ctx := context.Background()

	if _, err := RunConfluence(ctx, cfg, db.DB, Options{Full: true, ConfluenceClient: client}); err != nil {
		t.Fatal(err)
	}
	if s.calls() != 1 {
		t.Fatalf("history GETs after full = %d, want 1", s.calls())
	}
	got, err := db.PageVersions(ctx, "confluence:1001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Number != 1 || got[1].Number != 2 {
		t.Fatalf("stored order = %+v, want number 1 then 2", got)
	}
	if got[0].Message != "create" || got[1].Message != "edit" || !got[1].MinorEdit {
		t.Errorf("stored fields = %+v", got)
	}

	// Incremental, page version unchanged: no history refetch.
	if _, err := RunConfluence(ctx, cfg, db.DB, Options{ConfluenceClient: client}); err != nil {
		t.Fatal(err)
	}
	if s.calls() != 1 {
		t.Fatalf("history GETs after unchanged incremental = %d, want 1 (must not refetch)", s.calls())
	}

	// Version bump: history must be collected again.
	s.mu.Lock()
	s.pageVer = 3
	s.when = "2026-08-04T12:00:00.000Z"
	s.hist = append(s.hist, histVer{
		Number: 3, When: "2026-08-04T12:00:00.000Z", Message: "rewrite", Acc: "acc-c", Name: "Cara",
	})
	s.mu.Unlock()
	if _, err := RunConfluence(ctx, cfg, db.DB, Options{ConfluenceClient: client}); err != nil {
		t.Fatal(err)
	}
	if s.calls() != 2 {
		t.Fatalf("history GETs after version bump = %d, want 2", s.calls())
	}
	got, err = db.PageVersions(ctx, "confluence:1001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].Number != 3 || got[2].Message != "rewrite" {
		t.Errorf("after bump = %+v, want 3 rows ending in rewrite", got)
	}
}
