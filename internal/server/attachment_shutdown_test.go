// GDK-2000: a detail read arms attachment warming, and warming's goroutines
// belong to the server's cancel tree — the rule the terminal socket already
// follows (GDK-915, terminal.go). The hard case is a slow origin: a warm
// worker is mid-fetch when Shutdown runs, holds no checked-out pool
// connection (waitPoolIdle cannot see it) and, before the fix, was rooted at
// context.Background() and uncounted in jobsWG — so Shutdown returned with
// the worker still in flight, and its next mirror query ran against a
// handle the caller had already closed.
//
// The origin below parks the attachment response, so a worker is
// deterministically in flight inside the origin read when Close is called.
//
// The deadline discriminates rather than just bounds. Measured on this
// machine: the fixed path unwinds the blocked fetch inside Close itself —
// jobsCancel aborts the origin read and jobsWG.Wait holds the return until
// the workers are done, ~140µs end to end, zero frames left afterwards —
// while the Background-rooted worker survives the whole window because
// nothing in Shutdown reaches it: the frame sits in the origin read (the
// client's own timeout is 2 minutes, and the frame is only ever released
// by this test's channel, after the assertion). 2s sits with wide margin
// on both sides, so this fails on a regression to Background and does not
// flake on the fix. Passing requires cancellation, not the origin waking
// up. One leaked goroutine counts as 3 mentions (worker loop frame, Fill
// closure frame, and the dump's "created by" line), so any non-zero count
// is a leak.

package server

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/attachcache"
)

func TestAttachmentWarmGoroutinesStopOnShutdown(t *testing.T) {
	db, cfg := fixture(t)

	// The origin serves the seeded attachment (10021) but holds the response
	// until the test lets it go: a warm worker that reaches it parks
	// mid-fetch, which is the state a shutdown has to unwind.
	entered := make(chan struct{})
	release := make(chan struct{})
	var enterOnce, releaseOnce sync.Once
	freeOrigin := func() { releaseOnce.Do(func() { close(release) }) }
	jira := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/attachment/content/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		enterOnce.Do(func() { close(entered) })
		<-release
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGBYTES"))
	}))
	t.Cleanup(func() {
		// The handler parks in <-release; free it first or jira.Close — which
		// waits for outstanding requests — hangs on the fatal path.
		freeOrigin()
		jira.Close()
	})

	cfg.Site = jira.URL
	cfg.Email = "dana@example.com"
	cfg.Token = "token"
	cache, err := attachcache.New(t.TempDir(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	h := NewWithCache(db, cfg, cache)

	// The detail read is what arms warming (read.go: the issue detail calls
	// warmAttachments on its way out). 10021 is an image the cache lacks, so
	// one warm job is pending the moment the response is written.
	rec := get(t, h, apiBase+"NMB-1/detail/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail → %d %s", rec.Code, rec.Body.String())
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the warm path never reached the origin — the test is not exercising the path")
	}

	liveWarmFrames := func() int {
		buf := make([]byte, 1<<20)
		dump := string(buf[:runtime.Stack(buf, true)])
		return strings.Count(dump, "warmAttachments")
	}
	if n := liveWarmFrames(); n == 0 {
		t.Fatal("no warm goroutine frames while a fetch is in flight — the test is not exercising the path")
	}

	closeStart := time.Now()
	if err := h.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	t.Logf("Close unwound the in-flight warm fetch in %v", time.Since(closeStart))

	// Close returned with the origin still blocked: the worker must already
	// be gone, freed by cancellation, not by the origin answering.
	deadline := time.Now().Add(2 * time.Second)
	for {
		n := liveWarmFrames()
		if n == 0 {
			freeOrigin()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d attachment warm goroutine frame(s) survived Shutdown (GDK-2000 leak)", n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
