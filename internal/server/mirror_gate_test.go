package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

// The mirror REST allowlist gate (GDK-797). A DNS-named Host — the shape
// `tailscale serve` forwards — may reach a fixed allowlist of mirror reads
// plus the comment/transition writes, but only with a serve-scope Bearer.
// Everything else about DNS hosts is unchanged: unpaired stays
// forbidden_host, loopback and IP literals stay unauthenticated, and paths
// outside the allowlist never leave the rebinding guard.
//
// These tests seed the token store by writing pairing.json directly. In
// production that file is always written by another process (the
// `gadak pairing` CLI while the serve runs), so the on-disk shape — not a
// package API — is the contract the gate must honor.

// mirrorHost is a DNS-named Host shaped like the MagicDNS name a tailnet
// forwards. The example.com form keeps the secret scanner's tailnet regex
// quiet, same as TestPairedHostExemptLetsTailnetNameThrough.
const mirrorHost = "home.tailnet.example.com:8443"

type seedToken struct{ label, scope string }

// seedStore writes one active token per seedToken and returns the
// plaintexts in order. Labels are unique per call.
func seedStore(t *testing.T, dir string, toks ...seedToken) []string {
	t.Helper()
	now := time.Now().UTC()
	doc := make([]map[string]any, 0, len(toks))
	out := make([]string, 0, len(toks))
	for _, tk := range toks {
		token := "seed-token-" + tk.label + "-" + tk.scope
		sum := sha256.Sum256([]byte(token))
		doc = append(doc, map[string]any{
			"label":      tk.label,
			"scope":      tk.scope,
			"hash":       hex.EncodeToString(sum[:]),
			"created_at": now,
			"expires_at": now.Add(time.Hour),
		})
		out = append(out, token)
	}
	data, err := json.MarshalIndent(map[string]any{"tokens": doc}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, pairing.StoreRel), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return out
}

func bearer(v string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + v}
}

// ① A serve-scope Bearer on a DNS Host reaches the mirror bootstrap. Before
// the mirror exemption that Host died as forbidden_host before any token
// could speak — the phone had no path in at all.
func TestMirrorGateServeBearerOpensBootstrap(t *testing.T) {
	h, cfg := standaloneServer(t)
	token := seedStore(t, cfg.Directory(), seedToken{"phone", "serve"})[0]
	rec := getWithHost(t, h, "/api/v1/issues/bootstrap/", bearer(token), mirrorHost)
	if rec.Code != http.StatusOK {
		t.Fatalf("serve bearer, DNS host, bootstrap: %d %s; want 200", rec.Code, rec.Body.String())
	}
}

// ② credential/ stays behind the rebinding guard even with a valid serve
// token: the allowlist is the whole exemption (regression pin — 403 today
// too, back when nothing was exempt).
func TestMirrorGateKeepsCredentialForbidden(t *testing.T) {
	h, cfg := standaloneServer(t)
	token := seedStore(t, cfg.Directory(), seedToken{"phone", "serve"})[0]
	rec := getWithHost(t, h, "/api/v1/issues/credential/", bearer(token), mirrorHost)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "forbidden_host") {
		t.Fatalf("credential via DNS host: %d %s; want 403 forbidden_host", rec.Code, rec.Body.String())
	}
}

// ③ An allowlist path on a DNS Host with no Bearer is a pairing rejection,
// not a silent pass: the guard stepped aside, so the gate must speak.
func TestMirrorGateDemandsBearer(t *testing.T) {
	h, cfg := standaloneServer(t)
	seedStore(t, cfg.Directory(), seedToken{"phone", "serve"})
	rec := getWithHost(t, h, "/api/v1/issues/bootstrap/", nil, mirrorHost)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "pairing_rejected") {
		t.Fatalf("no bearer: %d %s; want 401 pairing_rejected", rec.Code, rec.Body.String())
	}
}

// ④ loopback is untouched, with and without tokens minted — the local web
// UI must never see a 401 from the mirror gate.
func TestMirrorGateLoopbackUnchanged(t *testing.T) {
	h, cfg := standaloneServer(t)
	rec := get(t, h, "/api/v1/issues/bootstrap/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback, no tokens: %d %s; want 200", rec.Code, rec.Body.String())
	}
	seedStore(t, cfg.Directory(), seedToken{"phone", "serve"})
	rec = get(t, h, "/api/v1/issues/bootstrap/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback, tokens minted: %d %s; want 200 (no loopback gate on the mirror)", rec.Code, rec.Body.String())
	}
}

// ⑤ No active tokens: a DNS Host gets nothing, allowlist or not. The
// exemption exists only while the gate can speak (regression pin — today's
// answer for every DNS-named Host).
func TestMirrorGateUnpairedDNSHostStaysForbidden(t *testing.T) {
	h, _ := standaloneServer(t)
	for _, path := range []string{
		"/api/v1/issues/bootstrap/",
		"/api/v1/issues/NMB-1/detail/",
		"/api/v1/auth/me/",
	} {
		rec := getWithHost(t, h, path, bearer("guessed-token"), mirrorHost)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "forbidden_host") {
			t.Fatalf("%s: %d %s; want 403 forbidden_host", path, rec.Code, rec.Body.String())
		}
	}
}

// ⑥ The scopes are one-way doors, and the rejection names itself so a
// phone developer can tell a wrong-scope token from a wrong path.
func TestMirrorGateScopeBoundaries(t *testing.T) {
	h, cfg := standaloneServer(t)
	toks := seedStore(t, cfg.Directory(), seedToken{"phone", "serve"}, seedToken{"laptop", "origin"})
	serve, laptop := toks[0], toks[1]

	// origin-scope on the mirror allowlist: authenticated, not authorized.
	rec := getWithHost(t, h, "/api/v1/issues/bootstrap/", bearer(laptop), mirrorHost)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "scope_rejected") {
		t.Fatalf("origin token on mirror allowlist: %d %s; want 403 scope_rejected", rec.Code, rec.Body.String())
	}

	// serve-scope on the origin passthrough (loopback — that gate has never
	// had a loopback bypass): same distinct answer.
	rec = get(t, h, origin.RESTPrefix+"/rest/api/3/myself", bearer(serve))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "scope_rejected") {
		t.Fatalf("serve token on origin passthrough: %d %s; want 403 scope_rejected", rec.Code, rec.Body.String())
	}

	// The origin token still rides the passthrough it was minted for.
	rec = get(t, h, origin.RESTPrefix+"/rest/api/3/myself", bearer(laptop))
	if rec.Code != http.StatusOK {
		t.Fatalf("origin token on origin passthrough: %d %s; want 200 (unchanged)", rec.Code, rec.Body.String())
	}
}

// mirrorWritableServer is the writable() write harness rebuilt on a config
// with a real profile directory: the mirror gate reads the token store
// through cfg.Directory(), and the hand-built fixture config carries none.
// Connected-shaped on purpose — the flagship phone home is a connected
// workspace whose writes go through its site.
func mirrorWritableServer(t *testing.T) (*fakeJira, *Handler, *config.Config) {
	t.Helper()
	f := newFakeJira(t)
	db, fcfg, _ := fixtureAt(t)
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
	cfg.Site = f.URL
	cfg.Email = fcfg.Email
	cfg.Token = fcfg.Token
	cfg.Projects = fcfg.Projects
	cfg.Members = fcfg.Members
	cfg.GroupRules = fcfg.GroupRules
	cfg.GroupLabels = fcfg.GroupLabels
	cfg.EditableFields = map[string]string{
		"solution": "customfield_10092", "fix_versions": "fixVersions",
		"development_test_assignee": "customfield_20000",
	}
	return f, New(db, cfg), cfg
}

func sendWithHost(t *testing.T, h http.Handler, method, path, body string, headers map[string]string, host string) *httptest.ResponseRecorder {
	t.Helper()
	req := testRequest(method, path, strings.NewReader(body))
	req.Host = host
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ⑦ The allowlist itself, as a table: every row a paired phone needs, and
// the surfaces that must stay behind forbidden_host. On the connected
// write fixture so comment and transition complete a real origin round
// trip, not just a routing pass.
func TestMirrorAllowlistTable(t *testing.T) {
	_, h, cfg := mirrorWritableServer(t)
	token := seedStore(t, cfg.Directory(), seedToken{"phone", "serve"})[0]

	allowed := []struct{ method, path, body string }{
		{"GET", "/api/v1/auth/me/", ""},
		{"GET", "/api/v1/issues/bootstrap/", ""},
		{"GET", "/api/v1/issues/delta/", ""},
		{"GET", "/api/v1/issues/search/?q=batch", ""},
		{"GET", "/api/v1/issues/jql/?q=statusCategory%20%3D%20new", ""},
		{"POST", "/api/v1/issues/jql/", `{"input":"statusCategory = new"}`},
		{"GET", "/api/v1/issues/feed/", ""},
		{"POST", "/api/v1/issues/feed/read/", `{"all":true}`},
		{"GET", "/api/v1/issues/views/", ""},
		{"GET", "/api/v1/issues/NMB-1/detail/", ""},
		{"GET", "/api/v1/issues/NMB-1/transitions/", ""},
		{"POST", "/api/v1/issues/NMB-1/comment/", `{"text":"from the phone"}`},
		{"POST", "/api/v1/issues/NMB-1/transition/", `{"transition_id":"31"}`},
	}
	for _, row := range allowed {
		rec := sendWithHost(t, h, row.method, row.path, row.body, bearer(token), mirrorHost)
		if rec.Code != http.StatusOK {
			t.Errorf("%s %s: %d %s; want 200", row.method, row.path, rec.Code, rec.Body.String())
		}
	}

	forbidden := []struct{ method, path, body string }{
		{"GET", "/api/v1/issues/credential/", ""},
		{"PUT", "/api/v1/issues/credential/", `{"email":"x@y","token":"t"}`},
		{"GET", "/api/v1/issues/settings/", ""},
		{"PUT", "/api/v1/issues/onboarding/connect/", `{"site":"https://x","email":"x@y","token":"t"}`},
		{"POST", "/api/v1/issues/sync/", ""},
		{"POST", "/api/v1/issues/create/", `{}`},
		{"GET", "/api/v1/dashboards/", ""},
		{"POST", "/api/v1/issues/jql/emit/", `{"filters":{}}`},
		{"GET", "/api/v1/issues/NMB-1/attachments/a-1/content/", ""},
		{"GET", "/api/v1/issues/history/", ""},
		{"GET", "/api/v1/issues/NMB-1/editmeta/", ""},
		{"PUT", "/api/v1/issues/NMB-1/assignee/", `{"account_id":"acc-hc"}`},
	}
	for _, row := range forbidden {
		rec := sendWithHost(t, h, row.method, row.path, row.body, bearer(token), mirrorHost)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "forbidden_host") {
			t.Errorf("%s %s: %d %s; want 403 forbidden_host", row.method, row.path, rec.Code, rec.Body.String())
		}
	}
}

// ⑧ A write through the gate is the existing mutate path: the comment goes
// to the origin under the home machine's credential, and the response is
// the refreshed row the web UI gets from the same endpoint.
func TestMirrorGateCommentWriteGoesThroughOrigin(t *testing.T) {
	f, h, cfg := mirrorWritableServer(t)
	token := seedStore(t, cfg.Directory(), seedToken{"phone", "serve"})[0]
	rec := sendWithHost(t, h, http.MethodPost, "/api/v1/issues/NMB-1/comment/",
		`{"text":"checked from the phone"}`, bearer(token), mirrorHost)
	if rec.Code != http.StatusOK {
		t.Fatalf("phone comment: %d %s; want 200", rec.Code, rec.Body.String())
	}
	sent, ok := f.bodies["POST /issue/NMB-1/comment"]
	if !ok {
		t.Fatal("comment never reached the origin")
	}
	if !strings.Contains(string(sent), "checked from the phone") {
		t.Fatalf("origin body %s", sent)
	}
	var body struct {
		Comment struct {
			CommentID string `json:"comment_id"`
		} `json:"comment"`
		Issue map[string]any `json:"issue"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Comment.CommentID != "c-99" || body.Issue == nil {
		t.Fatalf("response %+v — not the mutate contract shape", body)
	}
}
