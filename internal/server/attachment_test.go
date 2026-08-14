package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// attachmentFixture wires a fake Jira that serves one attachment and counts hits,
// so a test can prove the second view never leaves the process.
func attachmentFixture(t *testing.T) (http.Handler, *attachcache.Cache, *atomic.Int64, *store.DB, string) {
	t.Helper()
	db, cfg := fixture(t)

	var hits atomic.Int64
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/attachment/content/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGBYTES"))
	}))
	t.Cleanup(jira.Close)

	cfg.Site = jira.URL
	cfg.Email = "dana@example.com"
	cfg.Token = "token"

	cache, err := attachcache.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	return NewWithCache(db, cfg, cache), cache, &hits, db, jira.URL
}

func TestAttachmentIsFetchedOnceThenServedFromDisk(t *testing.T) {
	h, cache, hits, _, site := attachmentFixture(t)
	path := apiBase + "NMB-1/attachments/10021/content/"

	first := get(t, h, path, nil)
	if first.Code != http.StatusOK || first.Body.String() != "PNGBYTES" {
		t.Fatalf("first view → %d %q", first.Code, first.Body.String())
	}
	if !cache.Has(attachcache.Key(site, "", "NMB-1", "10021")) {
		t.Fatal("first view did not cache the bytes")
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("upstream hit %d times on the first view", n)
	}

	second := get(t, h, path, nil)
	if second.Code != http.StatusOK || second.Body.String() != "PNGBYTES" {
		t.Fatalf("second view → %d %q", second.Code, second.Body.String())
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("upstream hit %d times — the second view should be local", n)
	}
	// The bytes behind an id never change, so the browser is told to keep them.
	if cc := second.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("Cache-Control = %q, want an immutable directive", cc)
	}
	if second.Header().Get("ETag") == "" {
		t.Fatal("no ETag on a cached response")
	}
}

// The point of caching: a mirror with cached bytes keeps showing images after the
// credential is gone, which is what makes the offline demo snapshot work.
func TestCachedAttachmentSurvivesCredentialRemoval(t *testing.T) {
	h, cache, _, db, site := attachmentFixture(t)
	path := apiBase + "NMB-1/attachments/10021/content/"
	if rec := get(t, h, path, nil); rec.Code != http.StatusOK {
		t.Fatalf("warm-up → %d", rec.Code)
	}

	// Same cache, token gone, site kept — matches DELETE credential/ (site stays).
	offline := NewWithCache(db, &config.Config{Site: site}, cache)
	rec := get(t, offline, path, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "PNGBYTES" {
		t.Fatalf("offline view → %d %q, want the cached bytes", rec.Code, rec.Body.String())
	}
}

func TestUncachedAttachmentWithoutCredentialAsksForOne(t *testing.T) {
	db, _ := fixture(t)
	cache, err := attachcache.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	h := NewWithCache(db, &config.Config{}, cache)
	rec := get(t, h, apiBase+"NMB-1/attachments/10021/content/", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("→ %d, want 409 credential_required", rec.Code)
	}
}

// SVG executes script, so it must never be offered for inline rendering even
// when it is served from the cache.
// D9: same attachment id cached on site A must not be served after the
// profile's site becomes B (cache key includes site identity).
func TestAttachmentCacheMissesAfterSiteSwitch(t *testing.T) {
	db, cfg := fixture(t)
	cache, err := attachcache.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	siteA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("SITE-A"))
	}))
	t.Cleanup(siteA.Close)
	siteB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("SITE-B"))
	}))
	t.Cleanup(siteB.Close)

	cfgA := *cfg
	cfgA.Site = siteA.URL
	hA := NewWithCache(db, &cfgA, cache)
	path := apiBase + "NMB-1/attachments/10021/content/"
	first := get(t, hA, path, nil)
	if first.Code != http.StatusOK || first.Body.String() != "SITE-A" {
		t.Fatalf("site A → %d %q", first.Code, first.Body.String())
	}

	cfgB := *cfg
	cfgB.Site = siteB.URL
	hB := NewWithCache(db, &cfgB, cache)
	second := get(t, hB, path, nil)
	if second.Code != http.StatusOK {
		t.Fatalf("site B → %d %s", second.Code, second.Body.String())
	}
	if second.Body.String() == "SITE-A" {
		t.Fatal("site B served site A's cached bytes")
	}
	if second.Body.String() != "SITE-B" {
		t.Fatalf("site B → %q, want SITE-B (miss + refetch)", second.Body.String())
	}
}

// An id the issue does not list is 404 — same as a foreign issue key.
func TestAttachmentUnknownIdIsNotFound(t *testing.T) {
	h, _, _, _, _ := attachmentFixture(t)
	rec := get(t, h, apiBase+"NMB-1/attachments/99999/content/", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id → %d %q, want 404", rec.Code, rec.Body.String())
	}
}

// The URL id is the Jira external id when that column is set. The store row id
// must not open the bytes (old Detail loop used ExternalID first).
func TestAttachmentStoreIdDoesNotBypassExternalID(t *testing.T) {
	h, _, _, _, _ := attachmentFixture(t)
	rec := get(t, h, apiBase+"NMB-1/attachments/jira:a-1/content/", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("store id while external_id is set → %d %q, want 404", rec.Code, rec.Body.String())
	}
}

// When external_id is empty, the content URL uses the store id (handleDetail).
func TestAttachmentEmptyExternalIDFallsBackToStoreID(t *testing.T) {
	db, _ := fixture(t)
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:9090", SourceID: "jira", ExternalID: "9090", Key: "NMB-90",
				Title:     "local-only attachment",
				CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "NMB", IssueType: "Bug", StatusCategory: "new"},
			Attachments: []store.Attachment{{
				ID: "jira:local-att", Filename: "note.txt", MimeType: "text/plain",
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	// No credential: belongs → 409, not listed → 404. Distinguishes membership
	// from the fetch path.
	h := New(db, &config.Config{})
	owned := get(t, h, apiBase+"NMB-90/attachments/jira:local-att/content/", nil)
	if owned.Code != http.StatusConflict {
		t.Fatalf("empty external_id via store id → %d, want 409 (belongs, no credential)", owned.Code)
	}
	foreign := get(t, h, apiBase+"NMB-90/attachments/10021/content/", nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("other id on NMB-90 → %d, want 404", foreign.Code)
	}
}

// D9: a cached attachment id must not be readable under a different issue key.
func TestAttachmentCacheRejectsForeignIssueKey(t *testing.T) {
	h, _, _, _, _ := attachmentFixture(t)
	warm := apiBase + "NMB-1/attachments/10021/content/"
	if rec := get(t, h, warm, nil); rec.Code != http.StatusOK || rec.Body.String() != "PNGBYTES" {
		t.Fatalf("warm → %d %q", rec.Code, rec.Body.String())
	}
	foreign := get(t, h, apiBase+"NMB-2/attachments/10021/content/", nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign issue key → %d %q, want 404", foreign.Code, foreign.Body.String())
	}
	if foreign.Body.String() == "PNGBYTES" {
		t.Fatal("served cached bytes for an issue that does not own the attachment")
	}
}

func TestCachedSvgIsForcedToDownload(t *testing.T) {
	db, cfg := fixture(t)
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte("<svg/>"))
	}))
	t.Cleanup(jira.Close)
	cfg.Site, cfg.Email, cfg.Token = jira.URL, "dana@example.com", "token"
	cache, err := attachcache.New(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	h := NewWithCache(db, cfg, cache)
	path := apiBase + "NMB-1/attachments/10021/content/"
	_ = get(t, h, path, nil)
	rec := get(t, h, path, nil) // cached path
	if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}
