package server

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

func TestOriginRESTConnectedIs404(t *testing.T) {
	db, cfg := fixture(t)
	if cfg.HasBuiltInOrigin() {
		t.Fatal("fixture is built-in")
	}
	h := New(db, cfg)
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("connected passthrough: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Fatalf("body %s, want not_found", rec.Body.String())
	}
}

func builtInServer(t *testing.T) (*Handler, *config.Config) {
	h, cfg, _ := builtInServerDB(t)
	return h, cfg
}

// builtInServerDB is builtInServer plus the mirror handle, for tests that
// seed rows the handler reads (GDK-1613).
func builtInServerDB(t *testing.T) (*Handler, *config.Config, *store.DB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindStandalone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "mirror.db"))
	if err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)
	t.Cleanup(func() {
		_ = h.Close()
		_ = db.Close()
	})
	return h, cfg, db
}

func TestOriginRESTBuiltInPassesThrough(t *testing.T) {
	h, _ := builtInServer(t)
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("built-in:built-in"))
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": auth,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("built-in passthrough myself: %d %s", rec.Code, rec.Body.String())
	}
}

func TestOriginRESTBuiltInPOSTWithoutOriginAllowed(t *testing.T) {
	// Missing Origin is allowed (CLI). This is the existing browser guard,
	// not new auth — loopback single-user model (decision 0003).
	h, _ := builtInServer(t)
	req := testRequest(http.MethodPost, origin.RESTPrefix+"/rest/api/3/search/jql", strings.NewReader(`{"jql":"order by created","maxResults":1}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("built-in:built-in")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("CLI POST without Origin must not be forbidden: %s", rec.Body.String())
	}
	if rec.Code == http.StatusNotFound {
		t.Fatalf("built-in POST must not 404: %s", rec.Body.String())
	}
}

// TestOriginExportServesSeedYAML is FAIL-first for GDK-768's REST half: GET
// /api/v1/issues/origin/export/ serves the same bytes `gadak workspace
// export` writes. A connected serve answers 400 export_refused — a client
// error, not a 500 — because this serve has no built-in origin to export.
func TestOriginExportServesSeedYAML(t *testing.T) {
	h, _ := builtInServer(t)
	rec := get(t, h, apiBase+"origin/export/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("built-in export: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/yaml; charset=utf-8" {
		t.Fatalf("content-type = %q, want text/yaml", ct)
	}
	if !strings.Contains(rec.Body.String(), "projects:") {
		t.Fatalf("body is not the seed YAML:\n%.300s", rec.Body.String())
	}
}

func TestOriginExportConnectedIsRefused(t *testing.T) {
	db, cfg := fixture(t)
	if cfg.HasBuiltInOrigin() {
		t.Fatal("fixture is built-in")
	}
	rec := get(t, New(db, cfg), apiBase+"origin/export/", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("connected export: %d %s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"export_refused"`) {
		t.Fatalf("body %s, want export_refused", rec.Body.String())
	}
}
