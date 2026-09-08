package origin

// GDK-1617 review finding 5. serveStreaming runs the in-process origin
// handler on its own goroutine so the response body can be read while it
// is still being written. That goroutine has nothing above it: re-panicking
// there takes the whole gadak process down, where the buffering recorder it
// replaced ran the handler on the caller's stack — inside net/http's own
// recover, where a store panic cost one dropped request.
//
// This test is the statement of the bug: with the re-panic in place it does
// not fail, it crashes the test binary.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamingHandlerPanicDoesNotKillTheProcess(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/rest/api/3/myself", nil)
	res := serveStreaming(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("store: sqlite exec: disk I/O error")
	}), req)
	defer res.Body.Close()

	_, err := io.ReadAll(res.Body)
	if err == nil {
		t.Fatal("the reader saw a clean EOF after a panic — a short body that looks like a complete one")
	}
	if !strings.Contains(err.Error(), "handler panic") {
		t.Fatalf("the error does not say what happened: %v", err)
	}
}
