package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

/*
GDK-965 contract for GET settings/spaces/: the picker row says what a space
will cost *before* anyone commits to mirroring it.

What must hold, and what each case here pins:
  - a space whose origin answers carries `pages`;
  - a space whose origin cannot answer carries no `pages` key at all — the
    JSON is absent, not 0 and not a string ("unknown" is the UI's word, not
    the wire's);
  - counting is best-effort: an origin that fails every count still returns
    the full list with 200, because the picker's job is to list spaces and
    the count is an extra.
*/

// spacePagesMock serves both halves the handler needs: the space list, and
// the per-space CQL count. countFor decides each space's answer — a total, or
// a shape SpacePageCount refuses (which is how "absent" is produced without
// faking the client).
type spacePagesMock struct {
	spaces   []map[string]any
	countFor map[string]any // key → totalSize (int), or nil for "origin cannot answer"
	searches atomic.Int64   // how many count requests actually reached the origin
}

func (m *spacePagesMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/wiki/rest/api/space":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": m.spaces, "size": len(m.spaces), "limit": 100, "start": 0,
		})
	case "/wiki/rest/api/content/search":
		m.searches.Add(1)
		cql := r.URL.Query().Get("cql")
		key := spaceKeyFromCQL(cql)
		total, ok := m.countFor[key]
		if !ok || total == nil {
			// The origin answers, but not with a number this client will
			// trust: 500 is the plainest form of "cannot answer".
			http.Error(w, `{"message":"nope"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"totalSize": total, "results": []any{}})
	default:
		http.NotFound(w, r)
	}
}

// spaceKeyFromCQL pulls KEY out of `space="KEY" AND type=page`.
func spaceKeyFromCQL(cql string) string {
	const pre = `space="`
	i := strings.Index(cql, pre)
	if i < 0 {
		return ""
	}
	rest := cql[i+len(pre):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func spacePagesHandler(t *testing.T, cfg *config.Config, mock *spacePagesMock) http.Handler {
	t.Helper()
	t.Setenv("GADAK_HOME", t.TempDir())
	db, base := fixture(t)
	cfg.Email = base.Email
	if cfg.Token == "" {
		cfg.Token = "space-pages-token-secret"
	}
	srv := httptest.NewServer(mock)
	t.Cleanup(srv.Close)
	cfg.Site = srv.URL
	live := *cfg
	if live.Projects == nil {
		live.Projects = []string{"NMB"}
	}
	return New(db, &live)
}

// spaceRowsWithPages decodes the response keeping `pages` optional, so an
// absent key and a zero count stay distinguishable.
type spacePagesBody struct {
	Spaces []struct {
		Key   string `json:"key"`
		Pages *int   `json:"pages"`
	} `json:"spaces"`
}

func decodeSpacePages(t *testing.T, raw string) spacePagesBody {
	t.Helper()
	var body spacePagesBody
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decode: %v body %s", err, raw)
	}
	return body
}

// TestSettingsSpacesCarriesPageCount: a space the origin can count carries
// its number, and the number is the origin's — not the length of anything
// the handler happened to fetch.
func TestSettingsSpacesCarriesPageCount(t *testing.T) {
	mock := &spacePagesMock{
		spaces: []map[string]any{
			{"key": "ENG", "name": "Engineering", "type": "global"},
			{"key": "OPS", "name": "Operations", "type": "global"},
		},
		countFor: map[string]any{"ENG": 1240, "OPS": 7},
	}
	h := spacePagesHandler(t, &config.Config{
		Confluence: &config.ConfluenceConfig{Spaces: []string{"ENG"}},
	}, mock)

	rec := get(t, h, apiBase+"settings/spaces/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeSpacePages(t, rec.Body.String())
	want := map[string]int{"ENG": 1240, "OPS": 7}
	for _, s := range body.Spaces {
		w, ok := want[s.Key]
		if !ok {
			t.Fatalf("unexpected space %q", s.Key)
		}
		if s.Pages == nil {
			t.Fatalf("%s: pages absent, want %d (body %s)", s.Key, w, rec.Body.String())
		}
		if *s.Pages != w {
			t.Fatalf("%s: pages %d, want %d", s.Key, *s.Pages, w)
		}
	}
	if n := mock.searches.Load(); n != 2 {
		t.Fatalf("count requests %d, want one per space (2)", n)
	}
}

// TestSettingsSpacesOmitsUncountableSpace: the space the origin cannot count
// has no `pages` key. Not 0 — a zero would read as "empty space", which is a
// different and wrong answer.
func TestSettingsSpacesOmitsUncountableSpace(t *testing.T) {
	mock := &spacePagesMock{
		spaces: []map[string]any{
			{"key": "ENG", "name": "Engineering", "type": "global"},
			{"key": "OPS", "name": "Operations", "type": "global"},
			{"key": "NEW", "name": "Newly made", "type": "global"},
		},
		// OPS is missing from countFor → the origin refuses that one.
		countFor: map[string]any{"ENG": 1240, "NEW": 0},
	}
	h := spacePagesHandler(t, &config.Config{
		Confluence: &config.ConfluenceConfig{},
	}, mock)

	rec := get(t, h, apiBase+"settings/spaces/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	body := decodeSpacePages(t, raw)
	got := map[string]*int{}
	for _, s := range body.Spaces {
		got[s.Key] = s.Pages
	}
	if got["OPS"] != nil {
		t.Fatalf("OPS: pages %d present, want absent (body %s)", *got["OPS"], raw)
	}
	// The key itself must be missing, not present-and-null: the web type is
	// `pages?: number` and a null would have to be handled separately.
	if strings.Contains(raw, `"pages":null`) {
		t.Fatalf("uncountable space serialized a null pages: %s", raw)
	}
	if got["ENG"] == nil || *got["ENG"] != 1240 {
		t.Fatalf("ENG pages %v, want 1240", got["ENG"])
	}
	// A real, countable zero survives the same wire that drops "unknown".
	if got["NEW"] == nil || *got["NEW"] != 0 {
		t.Fatalf("NEW pages %v, want 0 (an empty space is a known answer)", got["NEW"])
	}
}

// TestSettingsSpacesListsWhenCountingFails: counting is an extra. When every
// count fails the list is still served, in full, with 200 — the picker never
// becomes unusable because a count endpoint is unhappy.
func TestSettingsSpacesListsWhenCountingFails(t *testing.T) {
	spaces := make([]map[string]any, 0, 5)
	for i := range 5 {
		spaces = append(spaces, map[string]any{
			"key": fmt.Sprintf("S%d", i), "name": fmt.Sprintf("Space %d", i), "type": "global",
		})
	}
	mock := &spacePagesMock{spaces: spaces, countFor: nil} // every count refused
	h := spacePagesHandler(t, &config.Config{
		Confluence: &config.ConfluenceConfig{},
	}, mock)

	rec := get(t, h, apiBase+"settings/spaces/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeSpacePages(t, rec.Body.String())
	if len(body.Spaces) != 5 {
		t.Fatalf("want all 5 spaces listed, got %+v", body.Spaces)
	}
	for _, s := range body.Spaces {
		if s.Pages != nil {
			t.Fatalf("%s: pages %d, want absent", s.Key, *s.Pages)
		}
	}
}

// TestSettingsSpacesCountingIsBounded: the count fan-out is capped, so a site
// with hundreds of spaces does not turn one picker open into hundreds of
// origin requests. Rows past the cap are listed without a count.
func TestSettingsSpacesCountingIsBounded(t *testing.T) {
	n := spacePageCountRows + 12
	spaces := make([]map[string]any, 0, n)
	counts := map[string]any{}
	for i := range n {
		key := fmt.Sprintf("S%03d", i)
		spaces = append(spaces, map[string]any{
			"key": key, "name": fmt.Sprintf("Space %03d", i), "type": "global",
		})
		counts[key] = 10 + i
	}
	mock := &spacePagesMock{spaces: spaces, countFor: counts}
	h := spacePagesHandler(t, &config.Config{
		Confluence: &config.ConfluenceConfig{},
	}, mock)

	rec := get(t, h, apiBase+"settings/spaces/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeSpacePages(t, rec.Body.String())
	if len(body.Spaces) != n {
		t.Fatalf("listed %d spaces, want all %d", len(body.Spaces), n)
	}
	if got := mock.searches.Load(); got > int64(spacePageCountRows) {
		t.Fatalf("%d count requests for %d spaces, want at most %d", got, n, spacePageCountRows)
	}
	for i, s := range body.Spaces {
		if i < spacePageCountRows && s.Pages == nil {
			t.Fatalf("row %d (%s): counted rows must carry pages", i, s.Key)
		}
		if i >= spacePageCountRows && s.Pages != nil {
			t.Fatalf("row %d (%s): past the cap, pages %d must be absent", i, s.Key, *s.Pages)
		}
	}
}
