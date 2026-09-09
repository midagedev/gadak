package server

// GDK-1616: one view of an attachment must cost the origin exactly one
// fetch. It cost two whenever the byte cache refused the entry for its
// size: handleAttachment called fetchAttachment inside cache.Fill, and on
// TooLarge fell through to a second fetchAttachment to stream the bytes it
// had just thrown away. In-process that is only wasteful; on a paired
// serve the bytes are really remote, and a large video was pulled across
// the tailnet twice for one <video> element.
//
// Counted, not reasoned about: the fake origin increments on every content
// request, and the handler has one fetch site guarded by a counter that
// errors on a second call, so a regression cannot pass this quietly.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
	"net/http/httptest"
	"sync/atomic"
)

// tooLargeFixture is attachmentFixture with a cache that refuses the entry
// for its size, which is the branch the double fetch lived on.
func tooLargeFixture(t *testing.T) (http.Handler, *atomic.Int64, *store.DB) {
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
		w.Header().Set("Accept-Ranges", "bytes")
		_, _ = w.Write([]byte("PNGBYTES"))
	}))
	t.Cleanup(jira.Close)
	cfg.Site, cfg.Email, cfg.Token = jira.URL, "dana@example.com", "token"
	// maxEntry 4 bytes: the 8-byte body is over the per-file limit.
	cache, err := attachcache.New(t.TempDir(), 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	_ = config.Config{}
	return NewWithCache(db, cfg, cache), &hits, db
}

func TestUncacheableAttachmentIsFetchedExactlyOnce(t *testing.T) {
	h, hits, _ := tooLargeFixture(t)
	rec := get(t, h, apiBase+"NMB-1/attachments/10021/content/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("view → %d %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := rec.Body.String(); got != "PNGBYTES" {
		t.Fatalf("body = %q, want the origin's bytes", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want the mime the origin recorded", ct)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("origin fetched %d times for one view, want exactly 1", n)
	}
}
