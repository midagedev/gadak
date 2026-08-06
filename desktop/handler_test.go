package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	scry "github.com/midagedev/scry"
	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/server"
	"github.com/midagedev/scry/internal/store"
)

// Exercises the exact seams the webview depends on: config.json, the API
// behind the browser guard (wails:// Origins, non-loopback Hosts), and the
// SPA fallback. Run with SCRY_PROFILE=demo — the profile is resolved from the
// environment at process start.
func TestFallbackHandler(t *testing.T) {
	if config.Profile() != "demo" {
		t.Skip("run with SCRY_PROFILE=demo — refusing to open the default profile's mirror")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	path, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ui, ok := scry.WebUI()
	if !ok {
		t.Fatal("no embedded web UI — run `npm run build` at the repo root first")
	}
	h := fallbackHandler(server.New(db, cfg), ui)

	t.Run("config.json", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/config.json", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
		}
	})

	t.Run("api passes the browser guard with webview identity", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/issues/sync/progress/", nil)
		req.Host = "wails.localhost" // what the webview actually sends
		req.Header.Set("Origin", "wails://wails.localhost")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("spa fallback serves index.html for client routes", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/issues/NMA-1", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Fatalf("got %d, body starts %q", rec.Code, rec.Body.String()[:min(80, rec.Body.Len())])
		}
	})
}
