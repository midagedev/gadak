package server

import (
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
func attachmentFixture(t *testing.T) (http.Handler, *attachcache.Cache, *atomic.Int64, *store.DB) {
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
	return NewWithCache(db, cfg, cache), cache, &hits, db
}

func TestAttachmentIsFetchedOnceThenServedFromDisk(t *testing.T) {
	h, cache, hits, _ := attachmentFixture(t)
	path := apiBase + "NMB-1/attachments/10001/content/"

	first := get(t, h, path, nil)
	if first.Code != http.StatusOK || first.Body.String() != "PNGBYTES" {
		t.Fatalf("first view → %d %q", first.Code, first.Body.String())
	}
	if !cache.Has("10001") {
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
	h, cache, _, db := attachmentFixture(t)
	path := apiBase + "NMB-1/attachments/10001/content/"
	if rec := get(t, h, path, nil); rec.Code != http.StatusOK {
		t.Fatalf("warm-up → %d", rec.Code)
	}

	// Same cache, no credential at all.
	offline := NewWithCache(db, &config.Config{}, cache)
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
	rec := get(t, h, apiBase+"NMB-1/attachments/10001/content/", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("→ %d, want 409 credential_required", rec.Code)
	}
}

// SVG executes script, so it must never be offered for inline rendering even
// when it is served from the cache.
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
	path := apiBase + "NMB-1/attachments/20002/content/"
	_ = get(t, h, path, nil)
	rec := get(t, h, path, nil) // cached path
	if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}
