package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestMainWindowOptionsOmitRuntimeJS(t *testing.T) {
	opts := mainWindowOptions()
	if opts.JS != "" {
		t.Fatalf("JS option still loads the runtime (double-load): %q", opts.JS)
	}
	if strings.Contains(opts.JS, "/wails/runtime.js") {
		t.Fatalf("JS option references /wails/runtime.js: %q", opts.JS)
	}
}

func TestInjectWailsRuntimeOnce(t *testing.T) {
	html := []byte("<!doctype html><html><head></head><body>spa</body></html>")
	got := injectWailsRuntime(html)
	if n := strings.Count(string(got), "/wails/runtime.js"); n != 1 {
		t.Fatalf("first inject: %d runtime.js refs\n%s", n, got)
	}
	// The skip guard: a page that already names the script is left alone.
	again := injectWailsRuntime(got)
	if n := strings.Count(string(again), "/wails/runtime.js"); n != 1 {
		t.Fatalf("second inject: %d runtime.js refs\n%s", n, again)
	}
}

func TestServedIndexHasOneRuntimeJS(t *testing.T) {
	ui := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><html><head></head><body>spa</body></html>")},
	}
	h := assetHandler(ui, fallbackHandler(http.NotFoundHandler(), ui, nil, nil, newBrowseTabs()))
	for _, p := range []string{"/", "/index.html", "/issues/NMA-1"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: status %d body %s", p, rec.Code, rec.Body.String())
		}
		if n := strings.Count(rec.Body.String(), "/wails/runtime.js"); n != 1 {
			t.Fatalf("%s: %d runtime.js refs\n%s", p, n, rec.Body.String())
		}
	}
}

func TestRaiseWindowNil(t *testing.T) {
	raiseWindow(nil)
}
