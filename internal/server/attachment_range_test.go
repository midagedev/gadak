package server

// GDK-1616, second pass: a ranged request must still fill the byte cache
// when the object could fit it.
//
// The first shape of the one-fetch fix answered every partial or conditional
// request straight from the origin, on the reasoning that a Range cannot be
// served from a body the request is about to stream past. That is true of the
// body and false of the cache, and it broke the common case: a <video> opens
// with `Range: bytes=0-` on Safari, so the first play of a cacheable file
// cached nothing — and while warmAttachments was still filling that same
// file, the play fetched it again in parallel. In-process that is waste; on a
// paired serve the file crosses the network twice for one play.
//
// So the decision is the recorded size against the cache's per-entry cap,
// taken before a byte moves:
//
//   R1 recorded size under the cap → fill, then http.ServeContent answers
//      this range (and every later seek) off the local file, one origin fetch
//      → 'R1 a ranged view of a cacheable attachment fills the cache'
//   R2 recorded size over the cap → the browser's Range goes to the origin
//      and nothing is cached, because nothing could be
//      → 'R2 a ranged view of an oversize attachment streams past the cache'
//
// Both cases count origin hits: the invariant this round bought is one fetch
// per request, and neither branch may spend two.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/midagedev/gadak/internal/attachcache"
)

// rangeFixture is the shared server fixture with a byte cache whose per-entry
// cap is maxEntry, and an origin that honours Range. The seeded attachment
// (10021) records Size 1234, so maxEntry straddles it: 1<<20 is "cacheable",
// 4 is "oversize".
func rangeFixture(t *testing.T, maxEntry int64) (http.Handler, *atomic.Int64, *attachcache.Cache, *atomic.Value) {
	t.Helper()
	db, cfg := fixture(t)
	body := []byte("PNGBYTES")
	var hits atomic.Int64
	var sawRange atomic.Value
	sawRange.Store("")
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/attachment/content/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits.Add(1)
		sawRange.Store(r.Header.Get("Range"))
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Accept-Ranges", "bytes")
		// Honour exactly the one range shape the tests ask for, so a
		// pass-through is visible as a 206 with the origin's own header.
		if r.Header.Get("Range") == "bytes=0-3" {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-3/%d", len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[:4])
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(origin.Close)
	cfg.Site, cfg.Email, cfg.Token = origin.URL, "dana@example.com", "token"
	cache, err := attachcache.New(t.TempDir(), 0, maxEntry)
	if err != nil {
		t.Fatal(err)
	}
	return NewWithCache(db, cfg, cache), &hits, cache, &sawRange
}

func TestRangedViewOfACacheableAttachmentFillsTheCache(t *testing.T) {
	h, hits, cache, sawRange := rangeFixture(t, 1<<20)
	if files, _ := cache.Stats(); files != 0 {
		t.Fatalf("cache starts with %d files, want 0", files)
	}

	rec := get(t, h, apiBase+"NMB-1/attachments/10021/content/", map[string]string{"Range": "bytes=0-3"})
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("ranged view → %d %s, want 206", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := rec.Body.String(); got != "PNGB" {
		t.Errorf("body = %q, want the first four bytes", got)
	}
	if cr := rec.Header().Get("Content-Range"); cr != "bytes 0-3/8" {
		t.Errorf("Content-Range = %q, want bytes 0-3/8", cr)
	}
	// The range was answered locally: the origin was asked for the whole
	// object, once.
	if got := sawRange.Load().(string); got != "" {
		t.Errorf("origin saw Range %q — the fill must ask for the whole object", got)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("origin fetched %d times for one ranged view, want exactly 1", n)
	}
	files, bytes := cache.Stats()
	if files != 1 || bytes != int64(len("PNGBYTES")) {
		t.Fatalf("cache holds %d files / %d bytes after a ranged view, want 1 / 8", files, bytes)
	}

	// And the second view — the seek a player makes next — costs nothing.
	rec = get(t, h, apiBase+"NMB-1/attachments/10021/content/", map[string]string{"Range": "bytes=4-7"})
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("second ranged view → %d, want 206", rec.Code)
	}
	if got := rec.Body.String(); got != "YTES" {
		t.Errorf("second range body = %q, want the last four bytes", got)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("origin fetched %d times across two ranged views, want 1", n)
	}
}

func TestRangedViewOfAnOversizeAttachmentStreamsPastTheCache(t *testing.T) {
	// Cap 4 bytes against a recorded size of 1234: this object can never be
	// kept, so the browser's Range is the origin's to answer.
	h, hits, cache, sawRange := rangeFixture(t, 4)

	rec := get(t, h, apiBase+"NMB-1/attachments/10021/content/", map[string]string{"Range": "bytes=0-3"})
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("ranged view → %d %s, want the origin's 206", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := sawRange.Load().(string); got != "bytes=0-3" {
		t.Errorf("origin saw Range %q, want the browser's own", got)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("origin fetched %d times, want exactly 1", n)
	}
	if files, _ := cache.Stats(); files != 0 {
		t.Errorf("cache holds %d files, want 0 — the object is over the cap", files)
	}
}
