package workspace

// Tests for POST /api/v1/workspaces and DELETE /api/v1/workspaces/{name}
// (GDK-1096). The refusal contracts here mirror the CLI verb's
// (cmd/gadak/workspace_rm_test.go) — the point of the extraction is that
// both surfaces enforce the same decisions, so these tests pin the HTTP
// mapping, not the decisions themselves.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// manageMux wires the two handlers the way buildServeMux does (method-
// scoped routes, so PathValue("name") resolves through real routing).
func manageMux(reg *Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/workspaces", CreateHandler())
	mux.HandleFunc("POST /api/v1/workspaces/{$}", CreateHandler())
	mux.HandleFunc("DELETE /api/v1/workspaces/{name}", RemoveHandler(reg))
	return mux
}

// manageSend issues one request against the manage mux and returns the
// recorder.
func manageSend(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// manageSeedProfile plants profiles/<name> with a config of the given kind
// and optionally a persist fixture, returning the profile directory.
func manageSeedProfile(t *testing.T, name, kind string, withPersist bool) string {
	t.Helper()
	dir, err := config.DirFor(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "{}"
	if kind == config.KindStandalone {
		cfg = `{"kind":"standalone"}`
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if withPersist {
		p := origin.PersistPath(dir)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("SQLite format 3\x00fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// manageErrDoc decodes {"error":...,"detail":...}.
func manageErrDoc(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var doc map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return doc
}

func TestManageCreateStandaloneSeedsProfile(t *testing.T) {
	setupHome(t)
	h := manageMux(New())

	rec := manageSend(t, h, http.MethodPost, "/api/v1/workspaces",
		`{"name":"scratch","kind":"standalone","projects":"abc, def"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		Persist string `json:"persist"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.Name != "scratch" || doc.Kind != config.KindStandalone {
		t.Fatalf("doc = %+v", doc)
	}

	// The profile exists on disk exactly as the CLI door leaves it: config
	// is standalone, the persist the response named is real, the mirror
	// was created, and the CSV projects parsed the shared way.
	dir := manageDir(t, "scratch")
	if doc.Persist != origin.PersistPath(dir) {
		t.Fatalf("persist = %q, want %q", doc.Persist, origin.PersistPath(dir))
	}
	for _, p := range []string{doc.Persist, filepath.Join(dir, "config.json")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s missing after create: %v", p, err)
		}
	}
	saved, err := config.LoadFor("scratch")
	if err != nil || !saved.IsStandalone() {
		t.Fatalf("saved config not standalone: %v %+v", err, saved)
	}
	if strings.Join(saved.Projects, ",") != "ABC,DEF" {
		t.Fatalf("projects = %v, want [ABC DEF]", saved.Projects)
	}
	if dbPath, err := config.DBPathFor("scratch"); err == nil {
		if _, err := os.Stat(dbPath); err != nil {
			t.Errorf("mirror %s missing after create: %v", dbPath, err)
		}
	}
}

func TestManageCreateRefusals(t *testing.T) {
	setupHome(t)
	h := manageMux(New())

	cases := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"unsupported kind", `{"name":"x1","kind":"connected"}`, http.StatusBadRequest, "unsupported_kind"},
		{"separator name", `{"name":"a/b","kind":"standalone"}`, http.StatusBadRequest, "invalid_name"},
		{"root default", `{"name":"default","kind":"standalone"}`, http.StatusBadRequest, "invalid_name"},
		{"empty name", `{"name":"","kind":"standalone"}`, http.StatusBadRequest, "invalid_name"},
		{"broken body", `{"name":`, http.StatusBadRequest, "invalid_body"},
	}
	for _, tc := range cases {
		rec := manageSend(t, h, http.MethodPost, "/api/v1/workspaces", tc.body)
		if rec.Code != tc.status {
			t.Errorf("%s: status %d, want %d: %s", tc.name, rec.Code, tc.status, rec.Body.String())
			continue
		}
		if doc := manageErrDoc(t, rec); doc["error"] != tc.code {
			t.Errorf("%s: error %q, want %q", tc.name, doc["error"], tc.code)
		}
	}

	// An existing profile directory is a conflict, not an overwrite.
	manageSeedProfile(t, "taken", config.KindConnected, false)
	rec := manageSend(t, h, http.MethodPost, "/api/v1/workspaces", `{"name":"taken","kind":"standalone"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("exists: status %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if doc := manageErrDoc(t, rec); doc["error"] != "exists" {
		t.Fatalf("exists: error %q", doc["error"])
	}
	// The 409 must not have touched the existing config's kind.
	saved, err := config.LoadFor("taken")
	if err != nil || saved.IsStandalone() {
		t.Fatalf("409 reseeded an existing profile: %v %+v", err, saved)
	}

	// The trailing-slash spelling routes to the same handler (the GET pair
	// above it registers both shapes; POST matches).
	rec = manageSend(t, h, http.MethodPost, "/api/v1/workspaces/", `{"name":"x2","kind":"connected"}`)
	if rec.Code != http.StatusBadRequest || manageErrDoc(t, rec)["error"] != "unsupported_kind" {
		t.Fatalf("trailing slash POST: %d %s", rec.Code, rec.Body.String())
	}
}

func TestManageRemoveRefusals(t *testing.T) {
	setupHome(t)
	reg := New()
	h := manageMux(reg)

	// needs_yes: connected, no query.
	manageSeedProfile(t, "w1", config.KindConnected, false)
	rec := manageSend(t, h, http.MethodDelete, "/api/v1/workspaces/w1", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("needs_yes: status %d: %s", rec.Code, rec.Body.String())
	}
	doc := manageErrDoc(t, rec)
	if doc["error"] != "needs_yes" || !strings.Contains(doc["detail"], "--yes") {
		t.Fatalf("needs_yes: %+v", doc)
	}

	// needs_destroy_origin: standalone persist present, yes given without
	// destroy_origin — the detail must carry the persist's absolute path.
	dir := manageSeedProfile(t, "s1", config.KindStandalone, true)
	persist := origin.PersistPath(dir)
	rec = manageSend(t, h, http.MethodDelete, "/api/v1/workspaces/s1?yes=1", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("needs_destroy_origin: status %d: %s", rec.Code, rec.Body.String())
	}
	doc = manageErrDoc(t, rec)
	if doc["error"] != "needs_destroy_origin" || !strings.Contains(doc["detail"], persist) {
		t.Fatalf("needs_destroy_origin: %+v, want detail with %q", doc, persist)
	}
	if _, err := os.Stat(persist); err != nil {
		t.Fatalf("persist removed by a refusal: %v", err)
	}

	// not_found: 404.
	rec = manageSend(t, h, http.MethodDelete, "/api/v1/workspaces/ghost?yes=1&destroy_origin=1", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("not_found: status %d: %s", rec.Code, rec.Body.String())
	}
	if doc := manageErrDoc(t, rec); doc["error"] != "not_found" {
		t.Fatalf("not_found: %+v", doc)
	}

	// self_delete: the profile this serve resolves to. Root serve: "default".
	rec = manageSend(t, h, http.MethodDelete, "/api/v1/workspaces/default?yes=1", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("self_delete(default): status %d: %s", rec.Code, rec.Body.String())
	}
	doc = manageErrDoc(t, rec)
	if doc["error"] != "self_delete" || !strings.Contains(doc["detail"], "gadak workspaces rm") {
		t.Fatalf("self_delete(default): %+v", doc)
	}
	// Named serve profile: its own name.
	manageSeedProfile(t, "solo", config.KindConnected, false)
	config.SetProfile("solo")
	rec = manageSend(t, h, http.MethodDelete, "/api/v1/workspaces/solo?yes=1", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("self_delete(named): status %d: %s", rec.Code, rec.Body.String())
	}
	if doc := manageErrDoc(t, rec); doc["error"] != "self_delete" {
		t.Fatalf("self_delete(named): %+v", doc)
	}

	// root: the unnamed workspace from a serve on some other profile — the
	// HTTP-specific self_delete check must not swallow the root refusal.
	manageSeedProfile(t, "other", config.KindConnected, false)
	config.SetProfile("other")
	rec = manageSend(t, h, http.MethodDelete, "/api/v1/workspaces/default?yes=1&destroy_origin=1", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("root: status %d: %s", rec.Code, rec.Body.String())
	}
	doc = manageErrDoc(t, rec)
	if doc["error"] != "root_workspace" || !strings.Contains(doc["detail"], "rm -rf") {
		t.Fatalf("root: %+v", doc)
	}
}

func TestManageRemoveStandaloneDestroys(t *testing.T) {
	setupHome(t)
	h := manageMux(New())

	// Create through the POST door, remove through the DELETE door — the
	// two surfaces must compose on one workspace.
	rec := manageSend(t, h, http.MethodPost, "/api/v1/workspaces", `{"name":"gone","kind":"standalone"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	dir := manageDir(t, "gone")

	rec = manageSend(t, h, http.MethodDelete, "/api/v1/workspaces/gone?yes=1&destroy_origin=1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Removed         string   `json:"removed"`
		Kind            string   `json:"kind"`
		OriginDestroyed bool     `json:"origin_destroyed"`
		Advisories      []string `json:"advisories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.Removed != "gone" || doc.Kind != config.KindStandalone || !doc.OriginDestroyed {
		t.Fatalf("doc = %+v", doc)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("profile dir survived: %v", err)
	}
	foundRestart := false
	for _, line := range doc.Advisories {
		if strings.Contains(line, "restarted") {
			foundRestart = true
		}
	}
	if !foundRestart {
		t.Fatalf("advisories lost the serve-restart note: %v", doc.Advisories)
	}
}

func TestRegistryEvictClosesEntryAndReGetReconstructs(t *testing.T) {
	setupHome(t)
	seedProfile(t, "work", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token", Projects: []string{"BBB"},
	})
	reg := New()
	t.Cleanup(func() { reg.Close() })

	e1, err := reg.Get("work")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	reg.Evict("work")
	reg.mu.Lock()
	_, stillMapped := reg.entries["work"]
	reg.mu.Unlock()
	if stillMapped {
		t.Fatal("Evict left the entry in the registry map")
	}
	// A later Get reconstructs the entry (lazy open, like a fresh mount).
	e2, err := reg.Get("work")
	if err != nil {
		t.Fatalf("re-get: %v", err)
	}
	if e2 == e1 {
		t.Fatal("re-get returned the evicted entry instead of reconstructing")
	}

	// No-ops: absent name, nil registry.
	reg.Evict("never-opened")
	var nilReg *Registry
	nilReg.Evict("work") // must not panic (handlers may hold a nil reg)
}

func TestManageRemoveEvictsOpenMount(t *testing.T) {
	setupHome(t)
	seedProfile(t, "mounted", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token", Projects: []string{"BBB"},
	})
	reg := New()
	t.Cleanup(func() { reg.Close() })
	if _, err := reg.Get("mounted"); err != nil {
		t.Fatalf("get: %v", err)
	}

	h := manageMux(reg)
	rec := manageSend(t, h, http.MethodDelete, "/api/v1/workspaces/mounted?yes=1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	dir := manageDir(t, "mounted")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("profile dir survived: %v", err)
	}
	reg.mu.Lock()
	_, stillMapped := reg.entries["mounted"]
	reg.mu.Unlock()
	if stillMapped {
		t.Fatal("DELETE left the opened mount's entry in the registry — Evict was not called")
	}
}

// manageDir resolves a profile directory, failing the test on an invalid
// name (a test bug, not a handler behavior).
func manageDir(t *testing.T, name string) string {
	t.Helper()
	dir, err := config.DirFor(name)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
