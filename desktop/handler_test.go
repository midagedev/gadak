package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	scry "github.com/midagedev/scry"
	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/server"
	"github.com/midagedev/scry/internal/store"
	"github.com/midagedev/scry/internal/workspace"
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
	reg := workspace.New()
	t.Cleanup(func() { reg.Close() })
	h := fallbackHandler(server.New(db, cfg), ui, reg)

	t.Run("config.json", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/config.json", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
		}
		// The UI reserves the window-controls corner off this flag; without it
		// the hidden title bar drops the traffic lights on the wordmark.
		var doc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if doc["desktop"] != true {
			t.Fatalf("desktop flag missing: %v", doc["desktop"])
		}
		// Same document `scry serve` sends, plus the one key — a dropped field
		// would switch off a surface in the app only.
		if _, ok := doc["apiBase"]; !ok {
			t.Fatalf("apiBase lost in the rewrite: %s", rec.Body.String())
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

// seedDesktopProfile writes config.json + scry.db under SCRY_HOME for tests that
// must not touch the real ~/.scry tree.
func seedDesktopProfile(t *testing.T, name string, cfg *config.Config) {
	t.Helper()
	dir, err := config.DirFor(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.LoadFor(name)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Site = cfg.Site
	loaded.Email = cfg.Email
	loaded.Token = cfg.Token
	loaded.Projects = cfg.Projects
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "scry.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
}

func TestDesktopWorkspaceRoutes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	seedDesktopProfile(t, "", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token", Projects: []string{"AAA"},
	})
	seedDesktopProfile(t, "work", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token", Projects: []string{"BBB"},
	})

	primaryCfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ui := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><html>spa</html>")},
	}
	reg := workspace.New()
	t.Cleanup(func() { reg.Close() })
	h := fallbackHandler(server.New(db, primaryCfg), ui, reg)

	t.Run("workspace config.json is JSON", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/w/work/config.json", nil)
		req.Host = "wails.localhost"
		req.Header.Set("Origin", "wails://wails.localhost")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Fatalf("Content-Type %q", rec.Header().Get("Content-Type"))
		}
		var doc map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("not JSON: %v body %s", err, rec.Body.String())
		}
		if doc["apiBase"] != "/w/work/api/v1/issues/" {
			t.Fatalf("apiBase %v", doc["apiBase"])
		}
		// Must not be SPA HTML.
		if strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Fatalf("got SPA HTML instead of config JSON")
		}
	})

	t.Run("workspaces list is JSON", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/workspaces", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Fatalf("Content-Type %q", rec.Header().Get("Content-Type"))
		}
		var doc struct {
			Workspaces []json.RawMessage `json:"workspaces"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("not JSON: %v body %s", err, rec.Body.String())
		}
		if doc.Workspaces == nil {
			t.Fatalf("workspaces not an array: %s", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Fatalf("got SPA HTML instead of workspaces JSON")
		}
	})
}
