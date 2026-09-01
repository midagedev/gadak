package workspace

// Tests for POST /api/v1/workspaces and DELETE /api/v1/workspaces/{name}
// (GDK-1096). The refusal contracts here mirror the CLI verb's
// (cmd/gadak/workspace_rm_test.go) — the point of the extraction is that
// both surfaces enforce the same decisions, so these tests pin the HTTP
// mapping, not the decisions themselves.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
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
	if kind == config.KindLocalOrigin {
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

func TestManageCreateLocalOriginSeedsProfile(t *testing.T) {
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
	if doc.Name != "scratch" || doc.Kind != config.KindLocalOrigin {
		t.Fatalf("doc = %+v", doc)
	}

	// The profile exists on disk exactly as the CLI door leaves it: config
	// is localOrigin, the persist the response named is real, the mirror
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
	if err != nil || !saved.HasLocalOrigin() {
		t.Fatalf("saved config not local-origin: %v %+v", err, saved)
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
	if err != nil || saved.HasLocalOrigin() {
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

	// needs_destroy_origin: local-origin persist present, yes given without
	// destroy_origin — the detail must carry the persist's absolute path.
	dir := manageSeedProfile(t, "s1", config.KindLocalOrigin, true)
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

func TestManageRemoveLocalOriginDestroys(t *testing.T) {
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
	if doc.Removed != "gone" || doc.Kind != config.KindLocalOrigin || !doc.OriginDestroyed {
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

// managePairHome is the minimal remote gadak serve a kind:"paired" create
// talks to: VerifyPaired is one GET over the passthrough path, so a server
// that answers /rest/api/3/myself is the whole fixture (the full passthrough
// contract lives in internal/server/origin_rest_pairing_test.go). status
// picks the answer: 200 proves the offer, 401 refuses it.
func managePairHome(t *testing.T, status int) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/rest/api/3/myself") {
			http.NotFound(w, r)
			return
		}
		if status == http.StatusUnauthorized {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"bad token"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"accountId":"acc-home","displayName":"Home Human"}`)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// manageOffer mints a real offer line for endpoint — EncodeOffer, the same
// constructor `gadak pairing mint` prints through, never a hand-built shape.
func manageOffer(t *testing.T, endpoint string) string {
	t.Helper()
	offer, err := pairing.EncodeOffer(pairing.Offer{
		V:        pairing.OfferV1,
		Endpoint: endpoint,
		Token:    "test-device-token",
		Label:    "web-tab",
	})
	if err != nil {
		t.Fatal(err)
	}
	return offer
}

// manageAssertNoEcho pins the absolute contract: neither the one-line offer
// nor the token inside it appears in a response body, success or failure.
func manageAssertNoEcho(t *testing.T, rec *httptest.ResponseRecorder, offer string) {
	t.Helper()
	if strings.Contains(rec.Body.String(), offer) {
		t.Errorf("response echoes the offer string: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "test-device-token") {
		t.Errorf("response echoes the pairing token: %s", rec.Body.String())
	}
}

func TestManageCreatePairedRegisters(t *testing.T) {
	setupHome(t)
	h := manageMux(New())

	ts := managePairHome(t, http.StatusOK)
	offer := manageOffer(t, ts.URL)
	rec := manageSend(t, h, http.MethodPost, "/api/v1/workspaces",
		`{"name":"laptop","kind":"paired","offer":"`+offer+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		Endpoint string `json:"endpoint"`
		Label    string `json:"label"`
		Account  string `json:"account"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// A paired workspace reports kind connected — the listing semantics
	// (WorkspaceKind: only local-origin is its own kind), pinned here so the
	// response cannot drift from what the workspaces list would say.
	if doc.Name != "laptop" || doc.Kind != config.KindConnected ||
		doc.Endpoint != ts.URL || doc.Label != "web-tab" || doc.Account != "Home Human" {
		t.Fatalf("doc = %+v", doc)
	}
	manageAssertNoEcho(t, rec, offer)

	// The profile exists on disk exactly as the CLI door leaves it: the
	// credential file next to a config stamped by the verified identity.
	dir := manageDir(t, "laptop")
	rem, err := pairing.LoadRemote(dir)
	if err != nil || rem == nil {
		t.Fatalf("remote credential missing after create: %v", err)
	}
	if rem.Endpoint != ts.URL || rem.Token != "test-device-token" || rem.Label != "web-tab" {
		t.Fatalf("remote credential = %+v", rem)
	}
	saved, err := config.LoadFor("laptop")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.HasLocalOrigin() || saved.AccountID != "acc-home" || saved.TokenOwner != "Home Human" {
		t.Fatalf("saved config = %+v", saved)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Errorf("config.json missing after create: %v", err)
	}

	// The exists refusal is shared with the local-origin branch: a name
	// already on disk is a conflict before any offer is read.
	manageSeedProfile(t, "taken", config.KindConnected, false)
	rec = manageSend(t, h, http.MethodPost, "/api/v1/workspaces",
		`{"name":"taken","kind":"paired","offer":"`+offer+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("exists: status %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestManageCreatePairedRefusals(t *testing.T) {
	setupHome(t)
	h := manageMux(New())

	// One attempt, no backoff: the unreachable case would otherwise pay the
	// production retry budget (1+2+4+8s of sleeps). The 502 mapping is
	// under test, not the patience — client.go documents this override as
	// the test hook for exactly that.
	oldRetries, oldBackoff := jira.DefaultRetries, jira.DefaultBackoff
	jira.DefaultRetries, jira.DefaultBackoff = 1, 0
	t.Cleanup(func() { jira.DefaultRetries, jira.DefaultBackoff = oldRetries, oldBackoff })

	// invalid_offer: a broken string never reaches the network.
	rec := manageSend(t, h, http.MethodPost, "/api/v1/workspaces",
		`{"name":"p1","kind":"paired","offer":"certainly-not-an-offer"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid_offer: status %d: %s", rec.Code, rec.Body.String())
	}
	if doc := manageErrDoc(t, rec); doc["error"] != "invalid_offer" {
		t.Fatalf("invalid_offer: %+v", doc)
	}
	if strings.Contains(rec.Body.String(), "certainly-not-an-offer") {
		t.Fatalf("invalid_offer detail quotes the payload: %s", rec.Body.String())
	}
	if _, err := os.Stat(manageDir(t, "p1")); !os.IsNotExist(err) {
		t.Fatalf("invalid_offer wrote a profile: %v", err)
	}

	// pairing_refused: the serve answered 401 — CLI wording, reused.
	ts := managePairHome(t, http.StatusUnauthorized)
	offer := manageOffer(t, ts.URL)
	rec = manageSend(t, h, http.MethodPost, "/api/v1/workspaces",
		`{"name":"p2","kind":"paired","offer":"`+offer+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("pairing_refused: status %d: %s", rec.Code, rec.Body.String())
	}
	doc := manageErrDoc(t, rec)
	if doc["error"] != "pairing_refused" || !strings.Contains(doc["detail"], "refused this pairing token") {
		t.Fatalf("pairing_refused: %+v", doc)
	}
	manageAssertNoEcho(t, rec, offer)
	if _, err := os.Stat(manageDir(t, "p2")); !os.IsNotExist(err) {
		t.Fatalf("pairing_refused wrote a profile: %v", err)
	}

	// serve_unreachable: a real port that is already closed (127.0.0.1
	// httptest, never a routed-away address). The endpoint itself is the
	// one thing the detail may name — the user supplied it.
	ts2 := httptest.NewServer(http.NotFoundHandler())
	endpoint := ts2.URL
	ts2.Close()
	offer2 := manageOffer(t, endpoint)
	rec = manageSend(t, h, http.MethodPost, "/api/v1/workspaces",
		`{"name":"p3","kind":"paired","offer":"`+offer2+`"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("serve_unreachable: status %d: %s", rec.Code, rec.Body.String())
	}
	doc = manageErrDoc(t, rec)
	if doc["error"] != "serve_unreachable" || !strings.Contains(doc["detail"], endpoint) {
		t.Fatalf("serve_unreachable: %+v, want detail naming %q", doc, endpoint)
	}
	manageAssertNoEcho(t, rec, offer2)
	if _, err := os.Stat(manageDir(t, "p3")); !os.IsNotExist(err) {
		t.Fatalf("serve_unreachable wrote a profile: %v", err)
	}
}
