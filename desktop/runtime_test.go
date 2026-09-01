package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/midagedev/gadak/internal/apprun"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/workspace"
)

func TestMainWindowOptionsOmitRuntimeJS(t *testing.T) {
	opts := mainWindowOptions()
	if opts.JS != "" {
		t.Fatalf("JS option still loads the runtime (double-load): %q", opts.JS)
	}
	if strings.Contains(opts.JS, "/wails/runtime.js") {
		t.Fatalf("JS option references /wails/runtime.js: %q", opts.JS)
	}
	if !opts.UseApplicationMenu {
		t.Fatal("UseApplicationMenu is unset; Windows creates the window with an empty HMENU")
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
	h := assetHandler(ui, fallbackHandler(http.NotFoundHandler(), ui, nil, nil, newBrowseTabs(), nil))
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

func TestShutdownOnceCancelsBeforeClosing(t *testing.T) {
	ctx, cancelContext := context.WithCancel(context.Background())
	var calls []string
	shutdown := shutdownOnce(func() {
		calls = append(calls, "cancel")
		cancelContext()
	}, func() error {
		if ctx.Err() == nil {
			t.Error("close ran before watch context cancellation")
		}
		calls = append(calls, "close")
		return nil
	})

	shutdown()
	shutdown()

	if got, want := strings.Join(calls, ","), "cancel,close"; got != want {
		t.Fatalf("shutdown calls = %q, want %q", got, want)
	}
}

func TestShutdownOnceClosesMountedStandaloneRegistry(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	t.Cleanup(func() {
		config.SetProfile("")
		_ = origin.Close()
		origin.ResetInProcess()
	})

	cfg, err := config.LoadFor("mounted")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	reg := workspace.New()
	t.Cleanup(reg.Close)
	if _, err := reg.Get("mounted"); err != nil {
		t.Fatal(err)
	}
	if !origin.IsInProcess(cfg) {
		t.Fatal("mounted local-origin did not bind its origin")
	}

	ctx, cancel := context.WithCancel(context.Background())
	shutdown := shutdownOnce(cancel, (&apprun.Runtime{Reg: reg}).Close)
	shutdown()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("shutdown did not cancel the watch context")
	}
	if origin.IsInProcess(cfg) {
		t.Fatal("in-process mark remains after desktop shutdown")
	}
}
