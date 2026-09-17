package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/skillinstall"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/workspace"
)

// seedProfile writes config.json and an empty migrated gadak.db under GADAK_HOME
// for the named profile ("" = default root), via the shared workspace helper.
func seedProfile(t *testing.T, home, name string, cfg *config.Config) {
	t.Helper()
	if err := workspace.SeedProfile(name, cfg); err != nil {
		t.Fatal(err)
	}
}

func testServeMux(t *testing.T) (http.Handler, *workspace.Registry) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	// Primary (default) + one named profile.
	seedProfile(t, home, "", &config.Config{
		Site:     "https://aaa.example.invalid",
		Email:    "user@example.invalid",
		Token:    "tok-aaa-secret-never-leak",
		Projects: []string{"AAA"},
	})
	seedProfile(t, home, "work", &config.Config{
		Site:     "https://bbb.example.invalid",
		Email:    "other@example.invalid",
		Token:    "tok-bbb-secret-never-leak",
		Projects: []string{"BBB"},
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

	api := server.New(db, primaryCfg)
	spa := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>spa</html>"))
	})
	reg := workspace.New()
	t.Cleanup(func() { reg.Close() })
	// Production wiring (cmdServe): the policy getter comes from the API
	// handler so both guard layers read one owner, and the phone bundle is
	// the embedded one.
	sites := serveSites{hosts: api.HostPolicyFn(), phoneUI: gadak.PhoneUI}
	return buildServeMux(api, spa, reg, sites), reg
}

func loopbackGet(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "127.0.0.1"
	return req
}

func TestWorkspacesListNoSecrets(t *testing.T) {
	mux, _ := testServeMux(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, loopbackGet("/api/v1/workspaces"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, banned := range []string{
		"token", "Token", "tok-aaa-secret-never-leak", "tok-bbb-secret-never-leak",
		"user@example.invalid", "other@example.invalid", "Email", "email",
	} {
		// "email" as a JSON key would be `"email"`; bare substring "email" is too
		// broad if a site URL ever contained it — check structural secrets only
		// for the token strings and the word as a JSON key.
		if banned == "email" || banned == "Email" {
			if strings.Contains(body, `"email"`) || strings.Contains(body, `"Email"`) {
				t.Fatalf("workspaces leaked key %q: %s", banned, body)
			}
			continue
		}
		if banned == "token" || banned == "Token" {
			if strings.Contains(body, `"token"`) || strings.Contains(body, `"Token"`) ||
				strings.Contains(strings.ToLower(body), `"token"`) {
				t.Fatalf("workspaces leaked key %q: %s", banned, body)
			}
			// Also reject the bare word as a field name elsewhere.
			if strings.Contains(body, "token") || strings.Contains(body, "Token") {
				t.Fatalf("workspaces response contains %q: %s", banned, body)
			}
			continue
		}
		if strings.Contains(body, banned) {
			t.Fatalf("workspaces leaked %q: %s", banned, body)
		}
	}

	var doc struct {
		Workspaces []struct {
			Name     string   `json:"name"`
			Site     string   `json:"site"`
			Projects []string `json:"projects"`
			Active   bool     `json:"active"`
			Error    string   `json:"error"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(doc.Workspaces) != 2 {
		t.Fatalf("want 2 workspaces, got %+v", doc.Workspaces)
	}
	if doc.Workspaces[0].Name != "default" || !doc.Workspaces[0].Active {
		t.Fatalf("first entry %+v", doc.Workspaces[0])
	}
	if doc.Workspaces[0].Site != "https://aaa.example.invalid" {
		t.Fatalf("default site %q", doc.Workspaces[0].Site)
	}
	if doc.Workspaces[1].Name != "work" || doc.Workspaces[1].Active {
		t.Fatalf("second entry %+v", doc.Workspaces[1])
	}
	if doc.Workspaces[1].Site != "https://bbb.example.invalid" {
		t.Fatalf("work site %q", doc.Workspaces[1].Site)
	}
}

func TestWorkspaceBootstrap(t *testing.T) {
	mux, _ := testServeMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/w/work/api/v1/issues/bootstrap/", nil)
	// httptest defaults Host to example.com, which the browser guard rejects
	// as a rebinding name; real clients arrive with a loopback Host.
	req.Host = "127.0.0.1:7777"
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var boot struct {
		Issues []struct {
			IssueKey string `json:"issue_key"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &boot); err != nil {
		t.Fatalf("decode: %v body %s", err, rec.Body.String())
	}
	if len(boot.Issues) != 1 || boot.Issues[0].IssueKey != "BBB-1" {
		t.Fatalf("want BBB-1 from work mirror, got %+v", boot.Issues)
	}

	// Primary still has AAA-1.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/issues/bootstrap/", nil)
	req2.Host = "127.0.0.1:7777"
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("primary status %d", rec2.Code)
	}
	var boot2 struct {
		Issues []struct {
			IssueKey string `json:"issue_key"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &boot2); err != nil {
		t.Fatal(err)
	}
	if len(boot2.Issues) != 1 || boot2.Issues[0].IssueKey != "AAA-1" {
		t.Fatalf("primary want AAA-1, got %+v", boot2.Issues)
	}
}

func TestWorkspaceConfigJSON(t *testing.T) {
	mux, _ := testServeMux(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, loopbackGet("/w/work/config.json"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		APIBase  string `json:"apiBase"`
		AuthBase string `json:"authBase"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.APIBase != "/w/work/api/v1/issues/" {
		t.Fatalf("apiBase %q", doc.APIBase)
	}
	if doc.AuthBase != "/w/work/api/v1/auth/" {
		t.Fatalf("authBase %q", doc.AuthBase)
	}
	// No secrets.
	body := rec.Body.String()
	for _, banned := range []string{"tok-bbb-secret-never-leak", "other@example.invalid", `"token"`, `"email"`} {
		if strings.Contains(body, banned) {
			t.Fatalf("config.json leaked %q: %s", banned, body)
		}
	}
}

// TestHealthzIdentity (GDK-1555): /healthz is the one endpoint every harness
// polls, so it is also the one place a server can prove what it is while it
// is being polled. The identity block has to answer four questions a poller
// actually asks: which binary (commit — the skillinstall owner's answer; the
// digest the harness builds stamp via -X), what it serves (home, workspace),
// and since when (startedAt, pid) — the pair that tells "we started this
// one" from "reuseExistingServer adopted it" without a side file.
func TestHealthzIdentity(t *testing.T) {
	mux, _ := testServeMux(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, loopbackGet("/healthz"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Status    string `json:"status"`
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		Digest    string `json:"digest"`
		Home      string `json:"home"`
		Workspace string `json:"workspace"`
		Clock     struct {
			Source string `json:"source"`
			Now    string `json:"now"`
		} `json:"clock"`
		StartedAt int64 `json:"startedAt"`
		Pid       int   `json:"pid"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if doc.Status != "ok" || doc.Version == "" {
		t.Fatalf("status/version: %+v", doc)
	}
	if doc.Commit != skillinstall.BuildRevision() {
		t.Fatalf("commit %q is not skillinstall.BuildRevision() = %q — healthz must not grow a second owner for the binary's revision", doc.Commit, skillinstall.BuildRevision())
	}
	// testServeMux points GADAK_HOME at a temp dir and selects the root
	// profile, so the serve's own idea of its home is exactly that dir.
	if doc.Home != os.Getenv("GADAK_HOME") {
		t.Fatalf("home %q, want the GADAK_HOME %q the serve answers for", doc.Home, os.Getenv("GADAK_HOME"))
	}
	if doc.Workspace != "default" {
		t.Fatalf("workspace %q, want the root profile's display name", doc.Workspace)
	}
	// GDK-1975: the clock member names which "now" the request path runs on.
	if (doc.Clock.Source != "wall" && doc.Clock.Source != "pinned") || doc.Clock.Now == "" {
		t.Fatalf("clock %+v, want source wall|pinned and a non-empty instant", doc.Clock)
	}
	if doc.StartedAt <= 0 || doc.StartedAt > time.Now().UnixMilli() {
		t.Fatalf("startedAt %d is not an epoch-ms process start", doc.StartedAt)
	}
	if doc.Pid != os.Getpid() {
		t.Fatalf("pid %d, want this process's %d", doc.Pid, os.Getpid())
	}

	// The digest and the commit ride the same variables the linker stamps
	// (-X main.buildDigest / -X main.buildCommit, e2e/serve.sh and
	// mobile/e2e/gate-serve.sh), so a harness build is reflected without
	// recompiling this test. The commit stamp is the one that matters in a
	// linked worktree, where Go's buildvcs writes nothing and
	// skillinstall.BuildRevision() answers "" — measured 2026-09-11.
	prevDigest, prevCommit := buildDigest, buildCommit
	buildDigest = "healthz-identity-probe"
	buildCommit = "0123456789abcdef0123456789abcdef01234567"
	t.Cleanup(func() { buildDigest, buildCommit = prevDigest, prevCommit })
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, loopbackGet("/healthz"))
	var doc2 struct {
		Digest string `json:"digest"`
		Commit string `json:"commit"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &doc2); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec2.Body.String())
	}
	if doc2.Digest != "healthz-identity-probe" {
		t.Fatalf("digest %q, want the stamped buildDigest", doc2.Digest)
	}
	if doc2.Commit != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("commit %q, want the stamped buildCommit to win over the empty ReadBuildInfo answer", doc2.Commit)
	}
}

// TestServeMuxMountsPhoneRoute (GDK-1966): /m/ is the phone bundle's own
// mount, never the web SPA's catch-all. A go build without `make phone`
// embeds no bundle, so the honest expectation is "the phone handler
// answered" — the real bundle's index, or the 503 that names the fix — and
// never the SPA shell a missing route would fall through to.
func TestServeMuxMountsPhoneRoute(t *testing.T) {
	mux, _ := testServeMux(t)

	for _, path := range []string{"/m/", "/m/issues/AAA-1"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, loopbackGet(path))
		if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status %d, want 200 (bundle) or 503 (no bundle); body %s", path, rec.Code, rec.Body.String())
		}
		if body := rec.Body.String(); body == "<html>spa</html>" {
			t.Fatalf("%s answered the web SPA shell — /m/ is the phone bundle's mount", path)
		}
		if rec.Code == http.StatusServiceUnavailable {
			var doc map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil || doc["error"] != "phone_ui_missing" {
				t.Fatalf("%s 503 body %s, want phone_ui_missing", path, rec.Body.String())
			}
		}
	}

	// The serve's own config.json carries phone_urls ([] when there is
	// nothing to dial) so the web Devices tab can bind without guards.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, loopbackGet("/config.json"))
	if rec.Code != http.StatusOK {
		t.Fatalf("/config.json status %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		PhoneURLs []string `json:"phone_urls"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("/config.json decode: %v (%s)", err, rec.Body.String())
	}
	if doc.PhoneURLs == nil {
		t.Fatalf("/config.json phone_urls absent — the key must always be present, never null")
	}
}

func TestServeMuxRejectsForeignHostOnTopLevelRoutes(t *testing.T) {
	mux, _ := testServeMux(t)

	for _, path := range []string{"/config.json", "/api/v1/workspaces"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "attacker.example"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s Host=attacker.example status %d, want 403; body %s", path, rec.Code, rec.Body.String())
			continue
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s decode: %v body %s", path, err, rec.Body.String())
			continue
		}
		if body["error"] != "forbidden_host" {
			t.Errorf("%s error %q, want forbidden_host", path, body["error"])
		}
	}

	for _, path := range []string{"/config.json", "/api/v1/workspaces"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "127.0.0.1"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s Host=127.0.0.1 status %d, want 200; body %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestWorkspaceBadName404(t *testing.T) {
	mux, _ := testServeMux(t)

	for _, path := range []string{
		"/w/bad..name/api/v1/issues/bootstrap/",
		"/w/bad..name/config.json",
		"/w/no-such-profile/api/v1/issues/bootstrap/",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, loopbackGet(path))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s → %d, want 404", path, rec.Code)
		}
	}
}

// TestServeMuxWorkspacesManageGuardBoundary pins that the workspace
// management routes (GDK-1096) inherit the outer browser guard: a DNS-named
// Host never reaches POST/DELETE, and a state-changing POST with a foreign
// Origin is refused. Neither Host exemption admits /api/v1/workspaces* —
// these are new routes on the outer mux, and this test is the proof the
// structure actually covers them.
func TestServeMuxWorkspacesManageGuardBoundary(t *testing.T) {
	mux, _ := testServeMux(t)

	// DNS Host → 403 forbidden_host on both verbs.
	for _, tc := range []struct {
		method string
		target string
	}{
		{http.MethodPost, "/api/v1/workspaces"},
		{http.MethodDelete, "/api/v1/workspaces/work"},
	} {
		req := httptest.NewRequest(tc.method, tc.target, strings.NewReader(`{"name":"x","kind":"standalone"}`))
		req.Host = "evil.example.com"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s Host=evil.example.com status %d, want 403; body %s", tc.method, tc.target, rec.Code, rec.Body.String())
			continue
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] != "forbidden_host" {
			t.Errorf("%s %s: error %v, want forbidden_host (%s)", tc.method, tc.target, body, rec.Body.String())
		}
	}

	// Foreign Origin on a state-changing POST → 403 forbidden_origin.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", strings.NewReader(`{"name":"x","kind":"standalone"}`))
	req.Host = "127.0.0.1:7777"
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign Origin POST status %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] != "forbidden_origin" {
		t.Fatalf("foreign Origin POST: error %v, want forbidden_origin (%s)", body, rec.Body.String())
	}

	// Sanity: the guard is the only thing that answered above — a loopback
	// Host with no Origin must reach the handler. An unsupported kind is
	// refused by the handler (not the guard) with its own code, proving the
	// route is registered and the guard admits it.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", strings.NewReader(`{"name":"x","kind":"connected"}`))
	req.Host = "127.0.0.1:7777"
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("loopback POST status %d, want 400 from the handler; body %s", rec.Code, rec.Body.String())
	}
	body = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] != "unsupported_kind" {
		t.Fatalf("loopback POST: error %v, want unsupported_kind (%s)", body, rec.Body.String())
	}
}

// TestServeMuxPhoneQR (GDK-1966): /phone-qr.png renders phone_urls[i] with
// the pairing QR's own encoder — one owner — and answers 404 past the list
// so the Devices tab never draws a code for an address that is not there.
func TestServeMuxPhoneQR(t *testing.T) {
	mux := buildServeMux(http.NotFoundHandler(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>spa</html>"))
	}), nil, serveSites{phoneURLs: []string{"http://192.0.2.9:7777/m/"}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, loopbackGet("/phone-qr.png"))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("/phone-qr.png status %d type %q, want 200 image/png", rec.Code, rec.Header().Get("Content-Type"))
	}
	if b := rec.Body.Bytes(); len(b) < 8 || string(b[1:4]) != "PNG" {
		t.Fatalf("/phone-qr.png body is not a PNG (%d bytes)", len(b))
	}
	for _, q := range []string{"?i=1", "?i=-1", "?i=x"} {
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, loopbackGet("/phone-qr.png"+q))
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
			t.Fatalf("/phone-qr.png%s status %d, want 404 or 400", q, rec.Code)
		}
	}
}
