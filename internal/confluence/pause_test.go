package confluence

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// PauseDecide is the fetch pool's seam into the politeness gap (GDK-1673):
// the pool runs PauseBetween only while its effective width is 1, so a wide
// pass must not pay the sleep. Both halves of the gate are measured here —
// nil decides "pause", false decides "skip".
func TestPagePauseDecideSkipsAndKeepsPause(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"1","type":"page","title":"p"}`))
	}))
	t.Cleanup(srv.Close)

	fetch := func(decide func() bool) time.Duration {
		c := New(srv.URL, "user@example.invalid", "secret-token")
		c.PauseBetween = 50 * time.Millisecond
		c.PauseDecide = decide
		start := time.Now()
		if _, err := c.Page(context.Background(), "1"); err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}

	// nil: the pause always runs.
	if d := fetch(nil); d < 50*time.Millisecond {
		t.Fatalf("nil PauseDecide took %s; the pause must run", d)
	}
	// false: the pause is skipped.
	if d := fetch(func() bool { return false }); d >= 50*time.Millisecond {
		t.Fatalf("PauseDecide=false took %s; the pause must be skipped", d)
	}
	// true: the pause runs.
	if d := fetch(func() bool { return true }); d < 50*time.Millisecond {
		t.Fatalf("PauseDecide=true took %s; the pause must run", d)
	}
}
