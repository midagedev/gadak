package confluence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/atlhttp"
)

func testClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	// New appends /wiki to the site; httptest URL is the site origin.
	c := New(srv.URL, "user@example.invalid", "secret-token")
	c.Retries, c.Backoff, c.PauseBetween = 4, time.Millisecond, 0
	return c
}

func TestSpacesPagesComments(t *testing.T) {
	adf := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"로그인이 실패했다"}]}]}`
	var hits int32
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		switch {
		case r.URL.Path == "/wiki/rest/api/space":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"key": "AAA", "name": "Alpha", "type": "global", "homepage": map[string]any{"id": "11"}},
					{"key": "BBB", "name": "Beta", "type": "global"},
				},
				"size": 2, "limit": 100, "start": 0,
			})
		case r.URL.Path == "/wiki/rest/api/content/search":
			// First page then next via _links.next.
			if r.URL.Query().Get("cursor") == "next1" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"results": []map[string]any{
						pageHit("2", "BBB", "Other page", "2026-08-02T12:00:00.000Z", 1),
					},
					"_links": map[string]string{},
				})
				return
			}
			next := "/wiki/rest/api/content/search?cql=type=page&limit=50&expand=version,space&cursor=next1"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					pageHit("1", "AAA", "Login notes", "2026-08-01T10:00:00.000Z", 2),
				},
				"_links": map[string]string{"next": next},
			})
		case r.URL.Path == "/wiki/rest/api/content/1":
			_ = json.NewEncoder(w).Encode(fullPage("1", "AAA", "Login notes", adf, 2, "2026-08-01T10:00:00.000Z"))
		case r.URL.Path == "/wiki/rest/api/content/1/child/comment":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					commentJSON("c1", "looks broken", "2026-08-01T11:00:00.000Z"),
				},
				"size": 1, "limit": 100,
			})
		case r.URL.Path == "/wiki/rest/api/content/c1/child/comment":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					commentJSON("c1r1", "reply ok", "2026-08-01T11:30:00.000Z"),
				},
				"size": 1, "limit": 100,
			})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))

	spaces, err := c.Spaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(spaces) != 2 || spaces[0].Key != "AAA" {
		t.Fatalf("spaces = %+v", spaces)
	}

	var ids []string
	err = c.SearchPages(context.Background(), "type=page", func(pages []Page) error {
		for _, p := range pages {
			ids = append(ids, p.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "1" || ids[1] != "2" {
		t.Fatalf("search ids = %v", ids)
	}

	pg, err := c.Page(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if pg.Title != "Login notes" || pg.Body.AtlasDocFormat == nil {
		t.Fatalf("page = %+v", pg)
	}
	raw := pg.Body.ADFRaw()
	if !strings.Contains(string(raw), "로그인이 실패했다") {
		t.Errorf("adf raw = %s", raw)
	}

	cms, err := c.Comments(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cms) != 2 {
		t.Fatalf("comments = %d, want 2 (top + reply)", len(cms))
	}
	if cms[0].ID != "c1" || cms[1].ID != "c1r1" {
		t.Errorf("comment ids = %s, %s", cms[0].ID, cms[1].ID)
	}
}

// TestPageParsesMetadataLabels is FAIL-first for expand=metadata.labels:
// Confluence REST v1 returns metadata.labels.results[{name,...}].
// Only the first page of labels is expanded (≤25); real pages have few labels.
func TestPageParsesMetadataLabels(t *testing.T) {
	adf := `{"type":"doc","version":1,"content":[]}`
	var expand string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wiki/rest/api/content/9" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		expand = r.URL.Query().Get("expand")
		p := fullPage("9", "ENG", "Runbook", adf, 1, "2026-08-01T10:00:00.000Z")
		// Deliberately unsorted names — client must surface names; sort is sync's job.
		p["metadata"] = map[string]any{
			"labels": map[string]any{
				"results": []map[string]any{
					{"name": "ops", "prefix": "global", "id": "1"},
					{"name": "runbook", "prefix": "global", "id": "2"},
					{"name": "alpha", "prefix": "global", "id": "3"},
				},
				"size":  3,
				"limit": 25,
				"start": 0,
			},
		}
		_ = json.NewEncoder(w).Encode(p)
	}))

	pg, err := c.Page(context.Background(), "9")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(expand, "metadata.labels") {
		t.Errorf("expand = %q, want metadata.labels", expand)
	}
	names := pg.LabelNames()
	if len(names) != 3 {
		t.Fatalf("LabelNames = %v, want 3 names", names)
	}
	// Order preserved from API (sync sorts for determinism).
	if names[0] != "ops" || names[1] != "runbook" || names[2] != "alpha" {
		t.Errorf("LabelNames = %v", names)
	}
	// Empty metadata → empty slice, not nil for callers that range.
	empty := Page{}
	if got := empty.LabelNames(); got == nil || len(got) != 0 {
		t.Errorf("empty LabelNames = %v, want empty non-nil", got)
	}
}

func TestRetryAfter429(t *testing.T) {
	var calls int32
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"key": "AAA", "name": "Alpha", "type": "global"},
			},
			"size": 1, "limit": 100, "start": 0,
		})
	}))
	spaces, err := c.Spaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("attempts = %d, want 3", calls)
	}
	if len(spaces) != 1 || spaces[0].Key != "AAA" {
		t.Errorf("spaces = %+v", spaces)
	}
}

func pageHit(id, space, title, when string, ver int) map[string]any {
	return map[string]any{
		"id": id, "type": "page", "status": "current", "title": title,
		"space": map[string]any{"key": space, "name": space},
		"version": map[string]any{"number": ver, "when": when, "by": map[string]any{
			"accountId": "acc-1", "displayName": "Ada Example",
		}},
	}
}

func fullPage(id, space, title, adf string, ver int, when string) map[string]any {
	p := pageHit(id, space, title, when, ver)
	p["body"] = map[string]any{
		"atlas_doc_format": map[string]any{
			"value":          adf,
			"representation": "atlas_doc_format",
		},
	}
	p["ancestors"] = []map[string]any{}
	return p
}

func commentJSON(id, text, when string) map[string]any {
	adf := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
	return map[string]any{
		"id": id, "type": "comment", "title": "Re:",
		"body": map[string]any{
			"atlas_doc_format": map[string]any{"value": adf, "representation": "atlas_doc_format"},
		},
		"version": map[string]any{
			"number": 1, "when": when,
			"by": map[string]any{"accountId": "acc-2", "displayName": "Bob Example"},
		},
	}
}

// TestSpacesExpandsHomepage is FAIL-first: Spaces requests expand=homepage and
// parses homepage.id from each listing row.
func TestSpacesExpandsHomepage(t *testing.T) {
	var expand string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wiki/rest/api/space" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		expand = r.URL.Query().Get("expand")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"key": "ENG", "name": "Engineering", "type": "global",
					"homepage": map[string]any{"id": "4242", "type": "page", "title": "Engineering"},
				},
			},
			"size": 1, "limit": 100, "start": 0,
		})
	}))
	spaces, err := c.Spaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if expand != "homepage" {
		t.Errorf("expand = %q, want homepage", expand)
	}
	if len(spaces) != 1 {
		t.Fatalf("spaces = %+v", spaces)
	}
	if spaces[0].Homepage == nil || spaces[0].Homepage.ID != "4242" {
		t.Errorf("Homepage = %+v, want id 4242", spaces[0].Homepage)
	}
}

// TestSpaceByKey parses a single space; RequestURI shows path-escaped key.
func TestSpaceByKey(t *testing.T) {
	key := "ENG/X"
	var sawURI string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawURI = r.RequestURI
		if r.URL.Query().Get("expand") != "homepage" {
			t.Errorf("expand = %q, want homepage", r.URL.Query().Get("expand"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key": key, "name": "Eng X", "type": "global",
			"homepage": map[string]any{"id": "99"},
		})
	}))
	sp, err := c.Space(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	escaped := url.PathEscape(key)
	if !strings.Contains(sawURI, "/wiki/rest/api/space/"+escaped) {
		t.Errorf("RequestURI = %q, want path with PathEscape(%q)=%q", sawURI, key, escaped)
	}
	if !strings.Contains(sawURI, "expand=homepage") {
		t.Errorf("RequestURI = %q, want expand=homepage", sawURI)
	}
	if sp.Key != key || sp.Name != "Eng X" {
		t.Errorf("space = %+v", sp)
	}
	if sp.Homepage == nil || sp.Homepage.ID != "99" {
		t.Errorf("Homepage = %+v, want id 99", sp.Homepage)
	}
}

func TestRawGETUnderWiki(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/wiki/api/v2/spaces" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("limit = %q", r.URL.Query().Get("limit"))
		}
		w.Write([]byte(`{"results":[]}`))
	}))
	status, body, err := c.Raw(context.Background(), http.MethodGet, "/api/v2/spaces?limit=5", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 || string(body) != `{"results":[]}` {
		t.Fatalf("status=%d body=%s", status, body)
	}
}

func TestRawRejectsAbsoluteURL(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not send: %s", r.URL)
	}))
	for _, path := range []string{"https://evil.example/x", "//evil.example/x"} {
		_, _, err := c.Raw(context.Background(), http.MethodGet, path, nil, false)
		if err == nil {
			t.Errorf("%q: want error", path)
		}
		if strings.Contains(err.Error(), "secret-token") {
			t.Errorf("token leaked in %v", err)
		}
	}
}

// TestErrAuthUnwrapsForErrorsIs pins the settings.go idiom
// (errors.Is(err, confluence.ErrAuth)) after ErrAuth became a typed sentinel.
func TestErrAuthUnwrapsForErrorsIs(t *testing.T) {
	wrapped := fmt.Errorf("GET /rest/api/space: %w (401 Unauthorized)", ErrAuth)
	if !errors.Is(wrapped, ErrAuth) {
		t.Fatalf("errors.Is(%v, ErrAuth) = false", wrapped)
	}
	if wrapped.Error() == ErrAuth.Error() {
		t.Fatal("wrapped error must keep method/path so last_error names the call")
	}
	if !strings.Contains(ErrAuth.Error(), "confluence:") {
		t.Fatalf("ErrAuth = %q, want the confluence: prefix so last_error distinguishes the source", ErrAuth)
	}
	if !errors.Is(wrapped, atlhttp.ErrAuth) {
		t.Fatalf("errors.Is(%v, atlhttp.ErrAuth) = false", wrapped)
	}
}
