package main

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

// GDK-433 CLI tests: the mint→list→revoke round trip (⑤), the
// verify-before-save contract of `init --pairing-code` (⑥), and the
// explicit refusal of unknown offer versions (⑦).

// pairingHome stands up a temp GADAK_HOME with a standalone workspace —
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
	cfg.Kind = config.KindStandalone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return cfg.Directory()
}

// emptyHome stands up a temp GADAK_HOME with no origin at all — the shape
// `init --pairing-code` requires.
func emptyHome(t *testing.T) string {
	t.Helper()
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

func TestPairingRefusesConnectedWorkspace(t *testing.T) {
	clearCredentialEnv(t)
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "x", "--endpoint", "http://127.0.0.1:9"})
	})
	if err == nil || !strings.Contains(err.Error(), "standalone") {
		t.Fatalf("mint on connected workspace must be refused, got %v", err)
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
	pairingHome(t) // standalone workspace already owns this profile

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
