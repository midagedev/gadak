package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/serveaddr"
	"github.com/midagedev/gadak/internal/store"
)

// GDK-433 CLI tests: the mint→list→revoke round trip (⑤), the
// verify-before-save contract of `init --pairing-code` (⑥), and the
// explicit refusal of unknown offer versions (⑦).

// pairingHome stands up a temp GADAK_HOME with a local-origin workspace —
// the shape `gadak pairing` operates on — and returns its profile dir.
func pairingHome(t *testing.T) string {
	t.Helper()
	clearCredentialEnv(t)
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return cfg.Directory()
}

// emptyHome stands up a temp GADAK_HOME with no origin at all — the shape
// `init --pairing-code` requires.
func emptyHome(t *testing.T) string {
	t.Helper()
	// Pairing verify and paired create go through origin.Connected →
	// jira.New. A closed serve is a refused dial; production retries
	// sleep 15s. Shrink the budget for this fixture (GDK-608).
	shrinkJiraRetryBudget(t)
	clearCredentialEnv(t)
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// shrinkJiraRetryBudget is the GDK-608 test seam: one attempt, no sleep.
func shrinkJiraRetryBudget(t *testing.T) {
	t.Helper()
	prevRetries, prevBackoff := jira.DefaultRetries, jira.DefaultBackoff
	jira.DefaultRetries, jira.DefaultBackoff = 1, 0
	t.Cleanup(func() {
		jira.DefaultRetries, jira.DefaultBackoff = prevRetries, prevBackoff
	})
}

// pairingOriginServe answers the passthrough myself path the paired
// transport hits, recording the Authorization it saw. status is what a
// 401-refusing variant returns instead.
func pairingOriginServe(t *testing.T, status int) (*httptest.Server, *string) {
	t.Helper()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != origin.RESTPrefix+"/rest/api/3/myself" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"errorMessages":["bad token"]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"displayName":"Home User","accountId":"acc-pair"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotAuth
}

func mustOffer(t *testing.T, o pairing.Offer) string {
	t.Helper()
	line, err := pairing.EncodeOffer(o)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func TestPairingMintListRevokeRoundTrip(t *testing.T) {
	dir := pairingHome(t)

	out, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop", "--ttl", "1h", "--endpoint", "http://127.0.0.1:9"})
	})
	if err != nil {
		t.Fatal(err)
	}
	// stdout is exactly the offer, one line (the pipe target contract).
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 || strings.TrimSpace(lines[0]) == "" {
		t.Fatalf("mint stdout must be exactly the offer line, got %q", out)
	}
	offer, err := pairing.DecodeOffer(lines[0])
	if err != nil {
		t.Fatal(err)
	}
	if offer.Label != "laptop" || offer.Endpoint != "http://127.0.0.1:9" || offer.Token == "" || offer.V != pairing.OfferV1 {
		t.Fatalf("offer = %+v", offer)
	}

	out, _, err = captureErr(t, func() error {
		return cmdPairing([]string{"list"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "laptop") {
		t.Fatalf("list lacks label: %q", out)
	}
	if strings.Contains(out, offer.Token) {
		t.Fatal("list leaked the plaintext token")
	}

	out, _, err = captureErr(t, func() error {
		return cmdPairing([]string{"revoke", "laptop"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "revoked") {
		t.Fatalf("revoke stdout = %q", out)
	}
	// Two tokens live here: the revoked "laptop" and the "_home" routing
	// token the first mint creates so this machine's own writes keep passing
	// the gate (ensureHomeRoutingToken) — that one must stay active.
	toks, err := pairing.List(dir)
	if err != nil || len(toks) != 2 {
		t.Fatalf("store after revoke: %+v (%v)", toks, err)
	}
	for _, tok := range toks {
		switch tok.Label {
		case "laptop":
			if tok.RevokedAt == nil {
				t.Fatalf("laptop not revoked: %+v", tok)
			}
		case "_home":
			if tok.RevokedAt != nil {
				t.Fatalf("_home must survive an unrelated revoke: %+v", tok)
			}
		default:
			t.Fatalf("unexpected token %+v", tok)
		}
	}
	rem, err := pairing.LoadRemote(dir)
	if err != nil || rem == nil || rem.Label != "_home" || rem.Token == "" {
		t.Fatalf("first mint must store the _home routing credential: %+v (%v)", rem, err)
	}
}

func TestPairingMintNeedsEndpointOrLiveServe(t *testing.T) {
	pairingHome(t)
	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop"})
	})
	if err == nil || !strings.Contains(err.Error(), "--endpoint") {
		t.Fatalf("mint without serve/endpoint must point at --endpoint, got %v", err)
	}
	// The failed mint must not have left a token behind.
	toks, err := pairing.List(configDirOrFatal(t))
	if err != nil || len(toks) != 0 {
		t.Fatalf("failed mint left tokens: %+v (%v)", toks, err)
	}
}

// An unconfigured home still refuses — pairing needs an origin to protect.
// (Was TestPairingRefusesConnectedWorkspace; since GDK-798 a *configured*
// connected workspace may mint, so the refusal that remains is
// "not configured", not "not local-origin".)
func TestPairingMintNeedsConfiguredWorkspace(t *testing.T) {
	clearCredentialEnv(t)
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "x", "--endpoint", "http://127.0.0.1:9"})
	})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("mint on an unconfigured workspace must refuse with not-configured, got %v", err)
	}
}

// connectedHome stands up a temp GADAK_HOME with a site-credential
// workspace — the connected home a phone wants to read (GDK-798).
func connectedHome(t *testing.T) *config.Config {
	t.Helper()
	clearCredentialEnv(t)
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindConnected
	cfg.Site = "https://example.atlassian.net"
	cfg.Email = "hc@example.com"
	cfg.Token = "site-token"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// GDK-798: a connected workspace can mint. The phone is not a second
// gadak workspace — it is a REST client of this home's mirror — so the
// local-origin-only refusal is gone. What stays closed is the origin
// passthrough itself (404 on connected, origin_rest.go).
func TestPairingMintServeScopeOnConnectedWorkspace(t *testing.T) {
	cfg := connectedHome(t)
	out, errout, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "phone", "--scope", "serve", "--endpoint", "http://127.0.0.1:9"})
	})
	if err != nil {
		t.Fatalf("connected serve mint must succeed: %v; stderr: %s", err, errout)
	}
	// stdout is exactly the offer line (the pipe target contract).
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("mint stdout must be exactly the offer line, got %q", out)
	}
	offer, err := pairing.DecodeOffer(lines[0])
	if err != nil || offer.Label != "phone" || offer.Token == "" {
		t.Fatalf("offer = %+v (%v)", offer, err)
	}
	toks, err := pairing.List(configDirOrFatal(t))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range toks {
		if m.Label == "phone" {
			found = true
			// literal "serve": this test must compile and FAIL before the
			// ScopeServe constant exists (FAIL-first for GDK-797/798).
			if m.Scope != "serve" {
				t.Fatalf("scope %q, want serve", m.Scope)
			}
		}
	}
	if !found {
		t.Fatalf("phone token missing from store: %+v", toks)
	}
	// Structural guard: a connected workspace's writes go straight to its
	// site, so minting must not leave a routing credential behind —
	// pairedRemote would read remote-origin.json as "this workspace is
	// paired" and rewire the origin to this machine's own serve.
	if rem, err := pairing.LoadRemote(configDirOrFatal(t)); err != nil || rem != nil {
		t.Fatalf("connected mint must not write remote-origin.json: %+v (%v)", rem, err)
	}
	if st, err := origin.PairedStatus(cfg); err != nil || st != nil {
		t.Fatalf("connected workspace misread as paired: %+v (%v)", st, err)
	}
}

// The default scope is origin, minted exactly as before GDK-797 (a paired
// laptop's token still rides the passthrough).
func TestPairingMintDefaultsToOriginScope(t *testing.T) {
	pairingHome(t)
	if _, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop", "--ttl", "1h", "--endpoint", "http://127.0.0.1:9"})
	}); err != nil {
		t.Fatal(err)
	}
	toks, err := pairing.List(configDirOrFatal(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range toks {
		if m.Label == "laptop" && m.Scope != "origin" {
			t.Fatalf("default scope = %q, want origin", m.Scope)
		}
	}
}

// A serve-scope offer is consumed by the phone companion, not by
// `init --pairing-code` — that consumes origin-scope offers, and a serve
// token cannot ride the passthrough. The stderr hint must not send the
// user to init.
func TestPairingMintServeHintNamesItsSurface(t *testing.T) {
	pairingHome(t)
	_, errout, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "phone", "--scope", "serve", "--endpoint", "http://127.0.0.1:9"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(errout, "--pairing-code") {
		t.Fatalf("serve mint must not point at init --pairing-code: %q", errout)
	}
	if !strings.Contains(errout, "mirror") {
		t.Fatalf("serve mint stderr should name what the token opens: %q", errout)
	}
}

// Unknown scopes are refused up front — minting a token whose scope no
// gate reads would be a silent lie.
func TestPairingMintScopeFlagValidates(t *testing.T) {
	pairingHome(t)
	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "x", "--scope", "wiki", "--endpoint", "http://127.0.0.1:9"})
	})
	if err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("unknown scope must be refused naming scope, got %v", err)
	}
	toks, err := pairing.List(configDirOrFatal(t))
	if err != nil || len(toks) != 0 {
		t.Fatalf("refused mint left tokens: %+v (%v)", toks, err)
	}
}

// `--label _home` rotates the routing token a local-origin home's own writes
// present. A connected workspace has no such token — its writes go
// straight to the site — and minting one would write remote-origin.json
// and poison PairedStatus, so it is refused, naming why.
func TestPairingMintHomeRefusedOnConnected(t *testing.T) {
	connectedHome(t)
	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "_home", "--endpoint", "http://127.0.0.1:9"})
	})
	if err == nil || !strings.Contains(err.Error(), "local-origin") {
		t.Fatalf("connected _home mint must be refused naming local-origin routing, got %v", err)
	}
	if rem, rerr := pairing.LoadRemote(configDirOrFatal(t)); rerr != nil || rem != nil {
		t.Fatalf("refused _home mint still wrote a routing credential: %+v (%v)", rem, rerr)
	}
}

func configDirOrFatal(t *testing.T) string {
	t.Helper()
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInitPairingCodeVerifiesBeforeSave(t *testing.T) {
	srv, gotAuth := pairingOriginServe(t, http.StatusOK)
	offer := mustOffer(t, pairing.Offer{
		V: pairing.OfferV1, Endpoint: srv.URL, Token: "pair-token-1", Label: "laptop",
	})
	dir := emptyHome(t)

	out, _, err := captureErr(t, func() error {
		return cmdInit([]string{"--pairing-code", offer})
	})
	if err != nil {
		t.Fatal(err)
	}
	if *gotAuth != "Bearer pair-token-1" {
		t.Fatalf("verify round trip sent %q, want the offer token as Bearer", *gotAuth)
	}
	if !strings.Contains(out, "paired with") {
		t.Fatalf("stdout = %q", out)
	}
	rem, err := pairing.LoadRemote(dir)
	if err != nil || rem == nil {
		t.Fatalf("remote credential not saved: %+v (%v)", rem, err)
	}
	if rem.Endpoint != srv.URL || rem.Token != "pair-token-1" || rem.Label != "laptop" {
		t.Fatalf("saved remote = %+v", rem)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccountID != "acc-pair" || cfg.Site != "" || cfg.Email != "" || cfg.Token != "" {
		t.Fatalf("config after pair = %+v", cfg)
	}
	// GDK-1276: pairing turns the wiki mirror on, scoped to every global
	// space the home origin lists — the paired twin of the block local-origin
	// init seeds. Without it the wiki pass reports "not configured" forever.
	if cfg.Confluence == nil || len(cfg.Confluence.Spaces) != 0 {
		t.Fatalf("config after pair: confluence = %+v, want an empty (all global spaces) block", cfg.Confluence)
	}

	// The paired workspace's origin.Client must talk to the serve with
	// the stored token.
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !origin.TransportIsServe(c.HTTP.Transport) {
		t.Fatalf("paired Client transport = %T, want serve passthrough", c.HTTP.Transport)
	}
}

func TestInitPairingCodeWritesNothingOn401(t *testing.T) {
	srv, gotAuth := pairingOriginServe(t, http.StatusUnauthorized)
	offer := mustOffer(t, pairing.Offer{
		V: pairing.OfferV1, Endpoint: srv.URL, Token: "pair-token-1", Label: "laptop",
	})
	dir := emptyHome(t)

	_, _, err := captureErr(t, func() error {
		return cmdInit([]string{"--pairing-code", offer})
	})
	if err == nil || !strings.Contains(err.Error(), "refused this pairing token") {
		t.Fatalf("401 verify must fail with the refused-token cause, got %v", err)
	}
	if *gotAuth == "" {
		t.Fatal("verify never reached the serve")
	}
	// Nothing written: no remote credential, no config rewrite.
	if rem, err := pairing.LoadRemote(dir); err != nil || rem != nil {
		t.Fatalf("remote credential written despite 401: %+v (%v)", rem, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("config.json written despite 401 (stat err = %v)", err)
	}
}

func TestInitPairingCodeWritesNothingOnUnreachableServe(t *testing.T) {
	srv, _ := pairingOriginServe(t, http.StatusOK)
	url := srv.URL
	srv.Close() // unreachable from here on
	offer := mustOffer(t, pairing.Offer{
		V: pairing.OfferV1, Endpoint: url, Token: "pair-token-1", Label: "laptop",
	})
	dir := emptyHome(t)

	_, _, err := captureErr(t, func() error {
		return cmdInit([]string{"--pairing-code", offer})
	})
	if err == nil || !strings.Contains(err.Error(), "could not verify") {
		t.Fatalf("unreachable serve must fail as a reachability cause, got %v", err)
	}
	if rem, err := pairing.LoadRemote(dir); err != nil || rem != nil {
		t.Fatalf("remote credential written despite unreachable serve: %+v (%v)", rem, err)
	}
}

func TestInitPairingCodeRejectsUnknownVersion(t *testing.T) {
	srv, _ := pairingOriginServe(t, http.StatusOK)
	offer := mustOffer(t, pairing.Offer{
		V: 2, Endpoint: srv.URL, Token: "pair-token-1", Label: "laptop",
	})
	dir := emptyHome(t)

	_, _, err := captureErr(t, func() error {
		return cmdInit([]string{"--pairing-code", offer})
	})
	if err == nil || !strings.Contains(err.Error(), "version 2") {
		t.Fatalf("unknown version must be an explicit refusal naming it, got %v", err)
	}
	if rem, err := pairing.LoadRemote(dir); err != nil || rem != nil {
		t.Fatalf("unknown version still wrote: %+v (%v)", rem, err)
	}
}

func TestInitPairingCodeRefusesProfilesWithAnOrigin(t *testing.T) {
	srv, _ := pairingOriginServe(t, http.StatusOK)
	offer := mustOffer(t, pairing.Offer{
		V: pairing.OfferV1, Endpoint: srv.URL, Token: "pair-token-1", Label: "laptop",
	})
	pairingHome(t) // local-origin workspace already owns this profile

	_, _, err := captureErr(t, func() error {
		return cmdInit([]string{"--pairing-code", offer})
	})
	if err == nil || !strings.Contains(err.Error(), "already owns an origin") {
		t.Fatalf("pairing over an existing origin must be refused, got %v", err)
	}
}

func TestInitPairingCodeStdin(t *testing.T) {
	srv, _ := pairingOriginServe(t, http.StatusOK)
	offer := mustOffer(t, pairing.Offer{
		V: pairing.OfferV1, Endpoint: srv.URL, Token: "pair-token-1", Label: "laptop",
	})
	emptyHome(t)
	saved := initStdin
	initStdin = strings.NewReader(offer + "\n")
	t.Cleanup(func() { initStdin = saved })

	out, _, err := captureErr(t, func() error {
		return cmdInit([]string{"--pairing-code-stdin"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "paired with") {
		t.Fatalf("stdin pairing stdout = %q", out)
	}
	// The offer (token) must not be echoed anywhere.
	if strings.Contains(out, offer) {
		t.Fatal("stdout echoed the offer")
	}
}

// GDK-449: paired workspaces must surface the remote-origin credential on
// status/doctor/profiles/pairing list instead of looking like a bare
// connected site with no pairing fields.
func TestPairedStatusSurfacesRemoteOrigin(t *testing.T) {
	seedPairedProfile(t)

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	var st struct {
		Kind    string `json:"kind"`
		Pairing *struct {
			Endpoint string `json:"endpoint"`
			Label    string `json:"label"`
		} `json:"pairing"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if st.Kind != config.KindConnected {
		t.Fatalf("kind = %q, want connected (paired is not a new kind)", st.Kind)
	}
	if st.Pairing == nil {
		t.Fatalf("status --json missing pairing object: %s", out)
	}
	if st.Pairing.Endpoint != "https://home.ts.net:8443" || st.Pairing.Label != "laptop" {
		t.Fatalf("pairing = %+v", st.Pairing)
	}

	text, err := capture(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v\n%s", err, text)
	}
	if !strings.Contains(text, `paired with "laptop" (https://home.ts.net:8443)`) {
		t.Fatalf("status text missing paired-with line:\n%s", text)
	}
}

func TestDoctorPairedDescribesServeWithoutHost(t *testing.T) {
	seedPairedProfile(t)
	t.Setenv("HOME", os.Getenv("GADAK_HOME"))

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, `paired gadak serve (label "laptop")`) {
		t.Fatalf("doctor origin missing paired serve+label:\n%s", out)
	}
	if strings.Contains(out, "home.ts.net") || strings.Contains(out, "8443") {
		t.Fatalf("doctor leaked pairing endpoint host (safe-to-paste):\n%s", out)
	}

	js, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, js)
	}
	var doc struct {
		Origin        string `json:"origin"`
		WorkspaceKind string `json:"workspace_kind"`
		Site          string `json:"site"`
	}
	if err := json.Unmarshal([]byte(js), &doc); err != nil {
		t.Fatalf("decode %q: %v", js, err)
	}
	if doc.WorkspaceKind != config.KindConnected {
		t.Fatalf("workspace_kind = %q, want connected", doc.WorkspaceKind)
	}
	if doc.Origin != `paired gadak serve (label "laptop")` {
		t.Fatalf("origin = %q", doc.Origin)
	}
	if strings.Contains(js, "home.ts.net") {
		t.Fatalf("doctor --json leaked endpoint host: %s", js)
	}
}

func TestProfilesPairedSiteColumnShowsHost(t *testing.T) {
	seedPairedProfile(t)

	out, err := capture(t, func() error { return cmdProfiles(nil) })
	if err != nil {
		t.Fatalf("profiles: %v\n%s", err, out)
	}
	if !strings.Contains(out, "home.ts.net") {
		t.Fatalf("profiles SITE missing pairing host:\n%s", out)
	}

	js, err := capture(t, func() error { return cmdProfiles([]string{"--json"}) })
	if err != nil {
		t.Fatalf("profiles --json: %v\n%s", err, js)
	}
	var inv struct {
		Profiles []struct {
			Name     string `json:"name"`
			SiteHost string `json:"site_host"`
			Kind     string `json:"kind"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(js), &inv); err != nil {
		t.Fatalf("decode %q: %v", js, err)
	}
	if len(inv.Profiles) == 0 || inv.Profiles[0].SiteHost != "home.ts.net" {
		t.Fatalf("profiles json site_host = %+v", inv.Profiles)
	}
	if inv.Profiles[0].Kind != config.KindConnected {
		t.Fatalf("kind = %q, want connected", inv.Profiles[0].Kind)
	}
}

func TestPairingListOnPairedAnswersSelfStatus(t *testing.T) {
	seedPairedProfile(t)

	out, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"list"})
	})
	if err != nil {
		t.Fatalf("pairing list on paired workspace: %v\n%s", err, out)
	}
	if !strings.Contains(out, `this workspace is paired with "laptop" (https://home.ts.net:8443)`) {
		t.Fatalf("list missing self-status: %q", out)
	}
	if !strings.Contains(out, "as Home User") {
		t.Fatalf("list missing TokenOwner: %q", out)
	}
	if strings.Contains(out, "pairing is for local-origin") {
		t.Fatalf("paired list still used the connected-workspace refusal: %q", out)
	}
}

func TestPairingListJSONOnPairedStaysJSON(t *testing.T) {
	// The self-status branch returns before the token listing, and it used
	// to print an English sentence on stdout whatever --json said. A verb
	// whose JSON is machine-readable on one branch and prose on another is
	// not a JSON verb, so this workspace answers with the pairing it is a
	// client of — and an empty `tokens`, so a reader takes one path.
	seedPairedProfile(t)

	out, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"list", "--json"})
	})
	if err != nil {
		t.Fatalf("pairing list --json on paired workspace: %v\n%s", err, out)
	}
	var got struct {
		Paired struct {
			Endpoint string `json:"endpoint"`
			Label    string `json:"label"`
			Owner    string `json:"owner"`
		} `json:"paired"`
		Tokens []map[string]any `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if got.Paired.Endpoint != "https://home.ts.net:8443" || got.Paired.Label != "laptop" {
		t.Fatalf("paired block: %+v", got.Paired)
	}
	if got.Paired.Owner != "Home User" {
		t.Fatalf("owner: %q", got.Paired.Owner)
	}
	if got.Tokens == nil || len(got.Tokens) != 0 {
		t.Fatalf("tokens must be present and empty, got %#v", got.Tokens)
	}
	if strings.Contains(out, "this workspace is paired with") {
		t.Fatalf("--json still printed the human sentence: %q", out)
	}
}

func TestPairingMintRevokeOnPairedRefusedWithSelfStatus(t *testing.T) {
	seedPairedProfile(t)

	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "phone", "--endpoint", "http://127.0.0.1:9"})
	})
	if err == nil {
		t.Fatal("mint on a paired workspace must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, `this workspace is paired with "laptop"`) {
		t.Fatalf("mint refusal missing self-status: %v", err)
	}
	if strings.Contains(msg, "pairing is for local-origin workspaces") {
		t.Fatalf("paired mint used the connected-workspace refusal: %v", err)
	}

	_, _, err = captureErr(t, func() error {
		return cmdPairing([]string{"revoke", "laptop"})
	})
	if err == nil {
		t.Fatal("revoke on a paired workspace must be refused")
	}
	if !strings.Contains(err.Error(), `this workspace is paired with "laptop"`) {
		t.Fatalf("revoke refusal missing self-status: %v", err)
	}
}

func TestInitPairedNextStepsOmitLocalServe(t *testing.T) {
	srv, _ := pairingOriginServe(t, http.StatusOK)
	offer := mustOffer(t, pairing.Offer{
		V: pairing.OfferV1, Endpoint: srv.URL, Token: "pair-token-1", Label: "laptop",
	})
	emptyHome(t)

	out, _, err := captureErr(t, func() error {
		return cmdInit([]string{"--pairing-code", offer})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "gadak sync") || !strings.Contains(out, "gadak status") {
		t.Fatalf("paired next-steps must name sync and status:\n%s", out)
	}
	if strings.Contains(out, "read it in the browser") {
		t.Fatalf("paired next-steps still recommend local gadak serve:\n%s", out)
	}
}

// GDK-453: a down home serve must not be mislabeled as a missing --project.
func TestCreatePairedUnreachableIsNotPassProject(t *testing.T) {
	dir := emptyHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	if err := pairing.SaveRemote(dir, pairing.Remote{Endpoint: url, Token: "pair-token", Label: "laptop"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.TokenOwner = "Home User"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	_, err = capture(t, func() error {
		return cmdCreate([]string{"hello from a paired workspace"})
	})
	if err == nil {
		t.Fatal("create against a closed home serve must fail")
	}
	msg := err.Error()
	if strings.Contains(msg, "pass --project") {
		t.Fatalf("connection failure was folded into pass --project: %v", err)
	}
	if !strings.Contains(msg, "cannot reach the home serve") {
		t.Fatalf("want pairing unreachable sentence, got %v", err)
	}
	if strings.HasPrefix(msg, "GET ") || strings.HasPrefix(msg, "POST ") {
		t.Fatalf("REST method leaked onto the first line: %v", err)
	}
}

func TestCreatePairedRevokedTokenNamesRevoke(t *testing.T) {
	dir := emptyHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Gadak-Pairing", "revoked")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"pairing_rejected","reason":"revoked"}`))
	}))
	t.Cleanup(srv.Close)
	if err := pairing.SaveRemote(dir, pairing.Remote{Endpoint: srv.URL, Token: "revoked-token", Label: "laptop"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	_, err = capture(t, func() error {
		return cmdCreate([]string{"hello", "--project", "STD"})
	})
	if err == nil {
		t.Fatal("create with a revoked pairing token must fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "pairing:") || !strings.Contains(msg, "revoked") {
		t.Fatalf("want pairing revoked sentence, got %v", err)
	}
	if strings.Contains(msg, "credential rejected") || strings.HasPrefix(msg, "GET ") || strings.HasPrefix(msg, "POST ") {
		t.Fatalf("Jira/REST noise on the first line: %v", err)
	}
}

func TestForceUpsertClearsLastError(t *testing.T) {
	// GDK-453: write-through (SyncIssue → UpsertIssues Force) is a successful
	// origin round-trip and must clear last_error the way RecordSync nil does.
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync(context.Background(), "jira", store.SyncResult{Err: context.DeadlineExceeded}); err != nil {
		t.Fatal(err)
	}
	st, err := db.SyncState(context.Background(), "jira")
	if err != nil || st.LastError == nil {
		t.Fatalf("precondition last_error: %+v (%v)", st.LastError, err)
	}

	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Force:      true,
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:STD-1", SourceID: "jira", Kind: "issue", ExternalID: "1", Key: "STD-1",
				Title: "after write", CreatedAt: "2026-08-20T00:00:00.000Z", UpdatedAt: "2026-08-20T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "STD", Status: "To Do", StatusID: "1", StatusCategory: "new"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	st, err = db.SyncState(context.Background(), "jira")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError != nil {
		t.Fatalf("last_error still %q after Force upsert", *st.LastError)
	}
}

func TestEndpointFromAdvertise(t *testing.T) {
	// The advertise file stores the raw bind address (host:port); the mint
	// default must arrive at --endpoint validation as a URL. Found live:
	// mint against a running serve failed with `bad endpoint "127.0.0.1:17391"`.
	for in, want := range map[string]string{
		"127.0.0.1:17391":        "http://127.0.0.1:17391",
		"[::1]:17391":            "http://[::1]:17391",
		"http://127.0.0.1:17391": "http://127.0.0.1:17391",
		"https://gadak.ts.net":   "https://gadak.ts.net",
		"":                       "",
	} {
		if got := endpointFromAdvertise(in); got != want {
			t.Fatalf("endpointFromAdvertise(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTTL(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"90d": 90 * 24 * time.Hour,
		"24h": 24 * time.Hour,
		"30m": 30 * time.Minute,
		"45s": 45 * time.Second,
	} {
		got, err := parseTTL(in)
		if err != nil || got != want {
			t.Fatalf("parseTTL(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "90", "1x", "0d", "-1h", "1.5h", "d"} {
		if _, err := parseTTL(bad); err == nil {
			t.Fatalf("parseTTL(%q) accepted", bad)
		}
	}
}

func tokenSHA(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func mintLaptop(t *testing.T) {
	t.Helper()
	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop", "--ttl", "1h", "--endpoint", "http://127.0.0.1:9"})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func activeHomeHash(t *testing.T, dir string) string {
	t.Helper()
	toks, err := pairing.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range toks {
		if m.Label == "_home" && m.RevokedAt == nil {
			return m.Hash
		}
	}
	t.Fatal("no active _home token")
	return ""
}

func assertRevokeHomeRefused(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("revoke _home must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "_home is this machine's own routing key") {
		t.Fatalf("want routing-key reason, got: %v", err)
	}
	if !strings.Contains(msg, "gadak pairing mint --label _home") {
		t.Fatalf("want rotate action, got: %v", err)
	}
}

// markHomeRevoked rewrites pairing.json so _home is revoked without going
// through pairing.Revoke — the locked-user shape GDK-450 has to recover
// from, and the only way to seed it once Revoke itself refuses _home.
func markHomeRevoked(t *testing.T, dir string) {
	t.Helper()
	p := pairing.StorePath(dir)
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tokens []map[string]any `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	found := false
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, tok := range doc.Tokens {
		if tok["label"] == "_home" {
			tok["revoked_at"] = now
			found = true
		}
	}
	if !found {
		t.Fatal("no _home row to mark revoked")
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestPairingRevokeHomeRefused is GDK-450: `pairing revoke _home` must not
// succeed — it locks local writes while a serve is running.
func TestPairingRevokeHomeRefused(t *testing.T) {
	dir := pairingHome(t)
	mintLaptop(t)
	before, err := pairing.LoadRemote(dir)
	if err != nil || before == nil {
		t.Fatalf("expected _home routing credential: %+v (%v)", before, err)
	}

	_, _, err = captureErr(t, func() error {
		return cmdPairing([]string{"revoke", "_home"})
	})
	assertRevokeHomeRefused(t, err)

	after, err := pairing.LoadRemote(dir)
	if err != nil || after == nil || after.Token != before.Token {
		t.Fatalf("refused revoke must not touch remote-origin.json: %+v (%v)", after, err)
	}
	if got := activeHomeHash(t, dir); got != tokenSHA(before.Token) {
		t.Fatalf("refused revoke mutated the store hash: %s vs %s", got, tokenSHA(before.Token))
	}
}

func TestPairingRevokeHomeByHashRefused(t *testing.T) {
	dir := pairingHome(t)
	mintLaptop(t)
	hash := activeHomeHash(t, dir)

	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"revoke", hash[:8]})
	})
	assertRevokeHomeRefused(t, err)
	if v, _ := pairing.Authorize(dir, mustRemoteToken(t, dir), time.Now()); v != pairing.VerdictAccept {
		t.Fatalf("hash revoke of _home must leave the routing token active, verdict=%v", v)
	}
}

func mustRemoteToken(t *testing.T, dir string) string {
	t.Helper()
	rem, err := pairing.LoadRemote(dir)
	if err != nil || rem == nil {
		t.Fatalf("remote: %+v (%v)", rem, err)
	}
	return rem.Token
}

// TestPairingMintHomeRotatesRemote is GDK-450: `pairing mint --label _home`
// rotates the routing token and points remote-origin.json at the new hash.
func TestPairingMintHomeRotatesRemote(t *testing.T) {
	dir := pairingHome(t)
	mintLaptop(t)
	old := mustRemoteToken(t, dir)

	out, stderr, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "_home", "--endpoint", "http://127.0.0.1:9"})
	})
	if err != nil {
		t.Fatalf("mint --label _home must rotate, got %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("rotate must not print a pairing offer, stdout=%q", out)
	}
	if !strings.Contains(stderr, "rotated") || !strings.Contains(stderr, "_home") {
		t.Fatalf("stderr must say what rotated, got %q", stderr)
	}

	rem, err := pairing.LoadRemote(dir)
	if err != nil || rem == nil {
		t.Fatalf("remote after rotate: %+v (%v)", rem, err)
	}
	if rem.Token == old {
		t.Fatal("remote-origin.json still holds the pre-rotate token")
	}
	if rem.Label != "_home" {
		t.Fatalf("label = %q, want _home", rem.Label)
	}
	want := tokenSHA(rem.Token)
	if got := activeHomeHash(t, dir); got != want {
		t.Fatalf("remote-origin.json token hash %s != active store hash %s", want, got)
	}
	if v, _ := pairing.Authorize(dir, old, time.Now()); v == pairing.VerdictAccept {
		t.Fatal("old routing token still accepted after rotate")
	}
	if v, _ := pairing.Authorize(dir, rem.Token, time.Now()); v != pairing.VerdictAccept {
		t.Fatalf("new routing token verdict %v, want Accept", v)
	}
}

func TestPairingListHomeScopeLocalRouting(t *testing.T) {
	pairingHome(t)
	mintLaptop(t)
	out, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"list"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "local-routing") {
		t.Fatalf("list must distinguish _home scope from device tokens, got %q", out)
	}
	// Device tokens keep the origin scope.
	if !strings.Contains(out, "laptop") {
		t.Fatalf("list lost the device row: %q", out)
	}
}

func TestEnsureHomeRoutingTokenRecoversStaleRemote(t *testing.T) {
	dir := pairingHome(t)
	mintLaptop(t)
	stale := mustRemoteToken(t, dir)
	markHomeRevoked(t, dir)
	if v, _ := pairing.Authorize(dir, stale, time.Now()); v == pairing.VerdictAccept {
		t.Fatal("setup: stale token should not authorize")
	}

	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "phone", "--ttl", "1h", "--endpoint", "http://127.0.0.1:9"})
	})
	if err != nil {
		t.Fatal(err)
	}
	rem, err := pairing.LoadRemote(dir)
	if err != nil || rem == nil {
		t.Fatalf("remote after recovery mint: %+v (%v)", rem, err)
	}
	if rem.Token == stale {
		t.Fatal("stale remote-origin.json token was not reissued")
	}
	if got := activeHomeHash(t, dir); got != tokenSHA(rem.Token) {
		t.Fatalf("reissued token hash %s != active store hash %s", tokenSHA(rem.Token), got)
	}
	if v, _ := pairing.Authorize(dir, rem.Token, time.Now()); v != pairing.VerdictAccept {
		t.Fatalf("reissued token verdict %v, want Accept", v)
	}
}

// TestPairingMintRequiresLabelBeforeEndpoint is GDK-456: --label is the
// user's first job. A mint with neither --label nor a live serve used to
// fail on endpoint resolution and never mention --label.
func TestPairingMintRequiresLabelBeforeEndpoint(t *testing.T) {
	pairingHome(t)
	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint"})
	})
	if err == nil {
		t.Fatal("mint without --label must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--label") {
		t.Fatalf("want --label named first, got %v", err)
	}
	if strings.Contains(msg, "no live serve") {
		t.Fatalf("label check ran after endpoint resolution: %v", err)
	}
}

// TestPairingMintTellsTheOtherMachineWhatToRun is GDK-456: device mint
// stderr must say how to consume the offer. The offer itself stays one
// stdout line (pipe contract). "token shown once" is gone — the offer is
// reusable until expiry.
func TestPairingMintTellsTheOtherMachineWhatToRun(t *testing.T) {
	pairingHome(t)
	out, stderr, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop", "--ttl", "1h", "--endpoint", "http://127.0.0.1:9"})
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 || strings.TrimSpace(lines[0]) == "" {
		t.Fatalf("mint stdout must be exactly the offer line, got %q", out)
	}
	if _, err := pairing.DecodeOffer(lines[0]); err != nil {
		t.Fatalf("stdout is not an offer: %v", err)
	}
	// The suggested profile is the device label, not the home's profile —
	// the remote picks its own name and the label is the shared one.
	if !strings.Contains(stderr, "on the other machine: gadak --workspace laptop init --pairing-code-stdin") {
		t.Fatalf("stderr missing other-machine command:\n%s", stderr)
	}
	if !strings.Contains(stderr, "paste the line above") {
		t.Fatalf("stderr missing paste hint:\n%s", stderr)
	}
	if strings.Contains(stderr, "shown once") {
		t.Fatalf("stderr still implies a one-shot token:\n%s", stderr)
	}
	if !strings.Contains(stderr, "reusable") {
		t.Fatalf("stderr must say the offer is reusable until expiry:\n%s", stderr)
	}
}

// TestPairingMintJSON is GDK-456: mint --json matches the other write
// verbs' JSON shape. stdout is one object {offer,label,endpoint,expires_at};
// the offer field is still a decodeable offer line.
func TestPairingMintJSON(t *testing.T) {
	pairingHome(t)
	out, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop", "--ttl", "1h", "--endpoint", "http://127.0.0.1:9", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Offer     string `json:"offer"`
		Label     string `json:"label"`
		Endpoint  string `json:"endpoint"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout must be JSON, got %q: %v", out, err)
	}
	if doc.Label != "laptop" || doc.Endpoint != "http://127.0.0.1:9" || doc.Offer == "" || doc.ExpiresAt == "" {
		t.Fatalf("json = %+v", doc)
	}
	offer, err := pairing.DecodeOffer(doc.Offer)
	if err != nil {
		t.Fatalf("offer field: %v", err)
	}
	if offer.Label != "laptop" || offer.Endpoint != doc.Endpoint || offer.Token == "" {
		t.Fatalf("decoded offer = %+v", offer)
	}
}

// TestPairingMintHomeJSONStdoutEmpty is GDK-450 × GDK-456: rotating
// _home must not print an offer or a JSON document even when --json is set.
func TestPairingMintHomeJSONStdoutEmpty(t *testing.T) {
	pairingHome(t)
	mintLaptop(t)
	out, stderr, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "_home", "--endpoint", "http://127.0.0.1:9", "--json"})
	})
	if err != nil {
		t.Fatalf("mint --label _home --json: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("rotate must not print JSON or an offer, stdout=%q", out)
	}
	if !strings.Contains(stderr, "rotated") || !strings.Contains(stderr, "_home") {
		t.Fatalf("stderr must still say what rotated, got %q", stderr)
	}
}

func TestPairingHelpNamesJSONAndHashPrefixFloor(t *testing.T) {
	out := formatHelp("pairing", nil)
	if !strings.Contains(out, "--json") {
		t.Fatalf("pairing help missing --json:\n%s", out)
	}
	if !strings.Contains(out, "8") || !strings.Contains(strings.ToLower(out), "hash") {
		t.Fatalf("pairing help must say revoke matches a hash prefix of 8+ chars:\n%s", out)
	}
}

// pairingListJSONRow is the --json shape of one pairing-list table row
// (GDK-947). Keys match the table columns, not pairing.Meta's json tags
// (those include the full hash).
type pairingListJSONRow struct {
	Hash     string `json:"hash"`
	Label    string `json:"label"`
	Scope    string `json:"scope"`
	Created  string `json:"created"`
	Expires  string `json:"expires"`
	LastUsed string `json:"last_used"`
	State    string `json:"state"`
}

// TestPairingNoArgsLists is GDK-947: bare `gadak pairing` is list, like
// views/dashboards/recipes, not a usage error.
func TestPairingNoArgsLists(t *testing.T) {
	pairingHome(t)
	out, _, err := captureErr(t, func() error {
		return cmdPairing(nil)
	})
	if err != nil {
		t.Fatalf("pairing with no args must list (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "no pairing tokens") {
		t.Fatalf("empty pairing list stdout = %q", out)
	}

	mintLaptop(t)
	out, _, err = captureErr(t, func() error {
		return cmdPairing(nil)
	})
	if err != nil {
		t.Fatalf("pairing with no args after mint: %v\n%s", err, out)
	}
	if !strings.Contains(out, "laptop") {
		t.Fatalf("no-args list missing minted label:\n%s", out)
	}
}

// TestPairingListJSON is GDK-947: `pairing list --json` is a JSON document
// whose tokens array carries the table columns, never the plaintext token,
// and the empty list is [] not the human sentence.
func TestPairingListJSON(t *testing.T) {
	pairingHome(t)

	out, stderr, err := captureErr(t, func() error {
		return cmdPairing([]string{"list", "--json"})
	})
	if err != nil {
		t.Fatalf("empty pairing list --json: %v\nstdout=%q stderr=%q", err, out, stderr)
	}
	var empty struct {
		Tokens []pairingListJSONRow `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(out), &empty); err != nil {
		t.Fatalf("empty list --json must parse as JSON, got %q: %v", out, err)
	}
	if empty.Tokens == nil {
		t.Fatalf("empty tokens must be [], not null: %s", out)
	}
	if len(empty.Tokens) != 0 {
		t.Fatalf("empty tokens = %+v", empty.Tokens)
	}
	if strings.Contains(out, "no pairing tokens") {
		t.Fatalf("JSON stdout must not carry the human empty-list sentence: %q", out)
	}

	mintOut, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop", "--ttl", "1h", "--endpoint", "http://127.0.0.1:9"})
	})
	if err != nil {
		t.Fatal(err)
	}
	offer, err := pairing.DecodeOffer(strings.TrimSpace(mintOut))
	if err != nil {
		t.Fatalf("mint offer: %v", err)
	}
	if offer.Token == "" {
		t.Fatal("fixture token is empty")
	}

	out, stderr, err = captureErr(t, func() error {
		return cmdPairing([]string{"list", "--json"})
	})
	if err != nil {
		t.Fatalf("pairing list --json: %v\n%s", err, out)
	}
	combined := out + stderr
	if strings.Contains(combined, offer.Token) {
		t.Fatalf("list --json leaked the fixture token value")
	}

	var doc struct {
		Tokens []pairingListJSONRow `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	wantKeys := []string{"hash", "label", "scope", "created", "expires", "last_used", "state"}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	arr, _ := raw["tokens"].([]any)
	if len(arr) == 0 {
		t.Fatalf("list --json has no rows: %s", out)
	}
	row0, _ := arr[0].(map[string]any)
	for _, k := range wantKeys {
		if _, ok := row0[k]; !ok {
			t.Errorf("list --json row missing key %q: %s", k, out)
		}
	}

	var laptop *pairingListJSONRow
	for i := range doc.Tokens {
		if doc.Tokens[i].Label == "laptop" {
			laptop = &doc.Tokens[i]
		}
	}
	if laptop == nil {
		t.Fatalf("list --json missing laptop row: %s", out)
	}
	if len(laptop.Hash) != 8 {
		t.Errorf("hash prefix = %q, want 8 hex chars", laptop.Hash)
	}
	if laptop.Scope != "origin" {
		t.Errorf("laptop scope = %q, want origin", laptop.Scope)
	}
	if laptop.State != "active" {
		t.Errorf("laptop state = %q, want active", laptop.State)
	}
	if laptop.Created == "" || laptop.Expires == "" {
		t.Errorf("laptop timestamps empty: %+v", laptop)
	}
	if laptop.LastUsed != "-" {
		t.Errorf("unused last_used = %q, want the table's dash", laptop.LastUsed)
	}

	bare, bareErrOut, err := captureErr(t, func() error {
		return cmdPairing(nil)
	})
	if err != nil {
		t.Fatalf("pairing no-args after mint: %v", err)
	}
	if strings.Contains(bare+bareErrOut, offer.Token) {
		t.Fatal("pairing no-args leaked the fixture token value")
	}
}

// GDK-1266: a device mint whose endpoint was *discovered* from the live
// serve and is loopback is refused, not warned about — the offer could
// never work on the remote device it is labelled for. An explicit
// `--endpoint http://127.0.0.1:…` stays allowed (the caller said "this
// machine"). FAIL-first: before the fix this minted with a stderr warning.
func TestPairingMintRefusesDiscoveredLoopbackEndpoint(t *testing.T) {
	pairingHome(t)
	addr, _ := startGadakProbe(t, "")
	runDir, err := serveaddr.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := serveaddr.Write(runDir, addr, ""); err != nil {
		t.Fatal(err)
	}
	// No tailscale: the refusal alone, no completed-command hint. The
	// status probe is stubbed rather than a script planted on PATH — the
	// fork of that script is what ran past its bound under load (GDK-1306).
	stubTailnetStatus(t, nil, errors.New("tailscale: not on PATH"))

	out, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop"})
	})
	if err == nil {
		t.Fatalf("discovered loopback endpoint %s minted an offer: %q", addr, out)
	}
	if got := exitStatus(err); got != 64 {
		t.Fatalf("exit %d, want 64 (usage): %v", got, err)
	}
	if !strings.Contains(err.Error(), "--endpoint https://") || strings.Contains(err.Error(), "tailscale") {
		t.Fatalf("refusal must prescribe --endpoint and carry no hint without tailscale: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("stdout must stay empty on refusal, got %q", out)
	}
	toks, err := pairing.List(configDirOrFatal(t))
	if err != nil || len(toks) != 0 {
		t.Fatalf("refused mint left tokens: %+v (%v)", toks, err)
	}

	// With tailscale answering, the refusal completes the command for the
	// user from Self.DNSName (trailing dot dropped) — suggested, never adopted.
	stubTailnetStatus(t, []byte(`{"Self":{"DNSName":"home.example.ts.net."}}`), nil)
	_, _, err = captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop", "--scope", "serve"})
	})
	if err == nil || exitStatus(err) != 64 {
		t.Fatalf("refusal with tailscale present: %v", err)
	}
	if !strings.Contains(err.Error(), "gadak pairing mint --label laptop --scope serve --endpoint https://home.example.ts.net") {
		t.Fatalf("hint must be the completed command: %v", err)
	}
}

// stubTailnetStatus replaces the `tailscale status --json` seam for one
// test (GDK-1306): what tailscale would have printed, or the error that
// stands for "not installed / daemon wedged".
func stubTailnetStatus(t *testing.T, out []byte, err error) {
	t.Helper()
	prev := tailnetStatusJSON
	tailnetStatusJSON = func() ([]byte, error) { return out, err }
	t.Cleanup(func() { tailnetStatusJSON = prev })
}
