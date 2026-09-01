package main

// GDK-1047 gates for the Devices tab's route surface. The load-bearing
// one is the guard at the HTTP boundary: a mint through
// /desktop/pairing/mint must leave a usable _home routing token in
// remote-origin.json — the exact assertion the FAIL-first run showed red
// against a naive pairing.MintScoped mint (round report has it verbatim).
// Beyond that: the QR data URI is the same matrix the terminal draws, the
// offer appears in exactly one response and nowhere else, scope origin|serve
// only (terminal is refused by design), `_home` is refused, revoke works
// by hash prefix, and the paired-away workspace is answered with a code,
// never a stack-flavored 500.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/pairflow"
	"github.com/midagedev/gadak/internal/pairing"
)

func pairingMuxForTest() http.Handler {
	return fallbackHandler(http.NotFoundHandler(), nil, nil, nil, newBrowseTabs(), nil)
}

// localOriginDesktopHome stands up a temp GADAK_HOME holding a local-origin
// workspace — the home-serve shape the Devices tab operates on.
func localOriginDesktopHome(t *testing.T) string {
	t.Helper()
	for _, k := range []string{"GADAK_SITE", "GADAK_EMAIL", "GADAK_TOKEN"} {
		t.Setenv(k, "")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", filepath.Join(home, ".gadak"))
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

func postPairing(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

func getPairing(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/desktop/pairing", nil))
	return rec
}

func pairingErrCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var doc struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("error body is not {\"error\":code}: %d %s", rec.Code, rec.Body.String())
	}
	return doc.Error
}

// TestPairingMintGuardAndQR is the round's namesake gate: mint through the
// route, and the local-origin home still holds a usable _home routing token;
// the QR in the response is the same module matrix the terminal renders;
// the offer round-trips and appears only where it belongs.
func TestPairingMintGuardAndQR(t *testing.T) {
	dir := localOriginDesktopHome(t)
	h := pairingMuxForTest()

	rec := postPairing(t, h, "/desktop/pairing/mint",
		`{"label":"phone","scope":"serve","ttl":"1h","endpoint":"http://192.0.2.10:7877"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("mint: %d %s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("mint Cache-Control is %q — a credential response must be no-store", cc)
	}
	var doc struct {
		Offer           string `json:"offer"`
		Label           string `json:"label"`
		Scope           string `json:"scope"`
		Endpoint        string `json:"endpoint"`
		ExpiresAt       string `json:"expires_at"`
		HashPrefix      string `json:"hash_prefix"`
		LoopbackWarning bool   `json:"loopback_warning"`
		QRPNG           string `json:"qr_png"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("mint body: %v\n%s", err, rec.Body.String())
	}
	if doc.Offer == "" || doc.Label != "phone" || doc.Scope != "serve" || doc.Endpoint != "http://192.0.2.10:7877" {
		t.Fatalf("mint carries %+v", doc)
	}
	if len(doc.HashPrefix) != 8 {
		t.Fatalf("hash_prefix %q, want 8 chars", doc.HashPrefix)
	}
	if doc.LoopbackWarning {
		t.Fatal("a TEST-NET endpoint is not loopback")
	}
	offer, err := pairing.DecodeOffer(doc.Offer)
	if err != nil {
		t.Fatalf("offer does not round-trip: %v", err)
	}
	if offer.Endpoint != doc.Endpoint || offer.Label != "phone" {
		t.Fatalf("offer carries %+v", offer)
	}

	// The guard, asserted at the HTTP boundary: the mint this route ran
	// left the home machine a routing token it can still use.
	rem, err := pairing.LoadRemote(dir)
	if err != nil || rem == nil {
		t.Fatalf("no _home routing token after a route mint: %v %v", rem, err)
	}
	if v, err := pairing.Authorize(dir, rem.Token, time.Now()); err != nil || v != pairing.VerdictAccept {
		t.Fatalf("routing token verdict %v (%v), want accept", v, err)
	}

	// The QR is a PNG data URI of the same matrix the terminal draws.
	if !strings.HasPrefix(doc.QRPNG, "data:image/png;base64,") {
		t.Fatalf("qr_png prefix: %q", doc.QRPNG[:40])
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(doc.QRPNG, "data:image/png;base64,"))
	if err != nil {
		t.Fatalf("qr_png is not base64: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("qr_png does not decode as PNG: %v", err)
	}
	mods, err := pairflow.QRModules(doc.Offer)
	if err != nil {
		t.Fatal(err)
	}
	side := len(mods) * 8
	if img.Bounds().Dx() != side || img.Bounds().Dy() != side {
		t.Fatalf("QR is %dx%d, want %d (modules %d × 8px)", img.Bounds().Dx(), img.Bounds().Dy(), side, len(mods))
	}

	// The loopback flag is data, both polarities: a loopback endpoint warns.
	rec = postPairing(t, h, "/desktop/pairing/mint",
		`{"label":"tablet","scope":"serve","ttl":"1h","endpoint":"http://127.0.0.1:7877"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback mint: %d %s", rec.Code, rec.Body.String())
	}
	var loop struct {
		LoopbackWarning bool `json:"loopback_warning"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &loop)
	if !loop.LoopbackWarning {
		t.Fatal("a loopback endpoint must set loopback_warning")
	}
}

// TestPairingMintRefusals: the route refuses what it must, with stable
// codes — terminal scope (by design: it opens a shell), the reserved
// _home label, an empty label, a non-http endpoint, no endpoint with no
// live serve, and a duplicate active label.
func TestPairingMintRefusals(t *testing.T) {
	localOriginDesktopHome(t)
	h := pairingMuxForTest()

	cases := []struct {
		name, body string
		status     int
		code       string
	}{
		{"terminal scope", `{"label":"sh","scope":"terminal","endpoint":"http://192.0.2.10:7877"}`, 400, "bad_scope"},
		{"made-up scope", `{"label":"x","scope":"admin","endpoint":"http://192.0.2.10:7877"}`, 400, "bad_scope"},
		{"reserved label", `{"label":"_home","scope":"serve","endpoint":"http://192.0.2.10:7877"}`, 400, "reserved_label"},
		{"empty label", `{"label":"  ","scope":"serve"}`, 400, "label_required"},
		{"bad endpoint scheme", `{"label":"p","scope":"serve","endpoint":"ftp://192.0.2.10:7877"}`, 400, "bad_endpoint"},
		{"no serve no endpoint", `{"label":"p","scope":"serve"}`, 409, "no_serve"},
		{"bad body", `not json`, 400, "bad_request"},
	}
	for _, tc := range cases {
		rec := postPairing(t, h, "/desktop/pairing/mint", tc.body)
		if rec.Code != tc.status {
			t.Fatalf("%s: %d %s, want %d", tc.name, rec.Code, rec.Body.String(), tc.status)
		}
		if got := pairingErrCode(t, rec); got != tc.code {
			t.Fatalf("%s: code %q, want %q", tc.name, got, tc.code)
		}
	}

	// Duplicate active label: the first mint owns it.
	if rec := postPairing(t, h, "/desktop/pairing/mint",
		`{"label":"phone","scope":"serve","ttl":"1h","endpoint":"http://192.0.2.10:7877"}`); rec.Code != http.StatusOK {
		t.Fatalf("first mint: %d %s", rec.Code, rec.Body.String())
	}
	rec := postPairing(t, h, "/desktop/pairing/mint",
		`{"label":"phone","scope":"serve","ttl":"1h","endpoint":"http://192.0.2.10:7877"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate label: %d %s", rec.Code, rec.Body.String())
	}
	if got := pairingErrCode(t, rec); got != "label_exists" {
		t.Fatalf("duplicate label code %q", got)
	}
}

// TestPairingListAndRevoke walks the tab's loop: GET shows the minted
// devices (and _home, marked as local-routing — it is not a device), the
// list never carries a token or an offer, revoke closes by the hash
// prefix the list itself printed, and revoking _home is refused with the
// same sentence the CLI refuses it with.
func TestPairingListAndRevoke(t *testing.T) {
	localOriginDesktopHome(t)
	h := pairingMuxForTest()

	getDevices := func() []map[string]any {
		rec := getPairing(t, h)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
		}
		var doc struct {
			Devices []map[string]any `json:"devices"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("GET body: %v\n%s", err, rec.Body.String())
		}
		return doc.Devices
	}

	if rec := postPairing(t, h, "/desktop/pairing/mint",
		`{"label":"phone","scope":"serve","ttl":"1h","endpoint":"http://192.0.2.10:7877"}`); rec.Code != http.StatusOK {
		t.Fatalf("mint: %d %s", rec.Code, rec.Body.String())
	}
	devices := getDevices()
	byLabel := map[string]map[string]any{}
	for _, d := range devices {
		byLabel[d["label"].(string)] = d
	}
	if len(byLabel) != 2 {
		t.Fatalf("devices after one mint: %v", byLabel)
	}
	phone, ok := byLabel["phone"]
	if !ok {
		t.Fatalf("phone row missing: %v", byLabel)
	}
	if phone["scope"] != "serve" || phone["state"] != "active" {
		t.Fatalf("phone row is %v", phone)
	}
	home, ok := byLabel[pairing.HomeLabel]
	if !ok || home["scope"] != pairing.ScopeLocalRouting {
		t.Fatalf("_home row missing or mislabeled: %v", home)
	}
	prefix, _ := phone["hash_prefix"].(string)
	if len(prefix) != 8 {
		t.Fatalf("hash_prefix %q", prefix)
	}

	// Revoke by the prefix the list printed.
	rec := postPairing(t, h, "/desktop/pairing/revoke", `{"selector":"`+prefix+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	for _, d := range getDevices() {
		if d["label"] == "phone" {
			if state, _ := d["state"].(string); !strings.HasPrefix(state, "revoked ") {
				t.Fatalf("revoked row state is %q", state)
			}
		}
	}

	// The refusals, with codes.
	for _, tc := range []struct {
		name, selector, code string
		status               int
	}{
		{"home refused", "_home", "home_refused", 400},
		{"unknown selector", "deadbeef", "not_found", 404},
		{"empty selector", "  ", "bad_request", 400},
	} {
		rec := postPairing(t, h, "/desktop/pairing/revoke", `{"selector":"`+tc.selector+`"}`)
		if rec.Code != tc.status {
			t.Fatalf("%s: %d %s, want %d", tc.name, rec.Code, rec.Body.String(), tc.status)
		}
		if got := pairingErrCode(t, rec); got != tc.code {
			t.Fatalf("%s: code %q, want %q", tc.name, got, tc.code)
		}
	}
}

// TestPairingPairedAway: a workspace that is itself paired to another
// machine owns no devices — GET answers the state as data and mint/revoke
// answer a 409 code, never a stack-flavored 500.
func TestPairingPairedAway(t *testing.T) {
	dir := localOriginDesktopHome(t)
	if err := pairing.SaveRemote(dir, pairing.Remote{
		Endpoint: "http://192.0.2.10:7877",
		Token:    "paired-away-not-a-pairing-token",
		Label:    "desk",
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = ""
	cfg.Site = "https://example.atlassian.net"
	cfg.Email = "home@example.net"
	cfg.Token = "secret-not-a-pairing-token"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	h := pairingMuxForTest()

	rec := getPairing(t, h)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"unavailable":"paired_away"`) {
		t.Fatalf("GET body does not name the state: %s", rec.Body.String())
	}
	rec = postPairing(t, h, "/desktop/pairing/mint",
		`{"label":"phone","scope":"serve","endpoint":"http://192.0.2.10:7877"}`)
	if rec.Code != http.StatusConflict || pairingErrCode(t, rec) != "paired_away" {
		t.Fatalf("mint on a paired-away workspace: %d %s", rec.Code, rec.Body.String())
	}
	rec = postPairing(t, h, "/desktop/pairing/revoke", `{"selector":"phone"}`)
	if rec.Code != http.StatusConflict || pairingErrCode(t, rec) != "paired_away" {
		t.Fatalf("revoke on a paired-away workspace: %d %s", rec.Code, rec.Body.String())
	}
}

// TestPairingNeverLeaksTheOffer: the offer appears in exactly one response
// — the mint's. Error bodies and the devices list never carry it.
func TestPairingNeverLeaksTheOffer(t *testing.T) {
	localOriginDesktopHome(t)
	h := pairingMuxForTest()

	rec := postPairing(t, h, "/desktop/pairing/mint",
		`{"label":"phone","scope":"serve","ttl":"1h","endpoint":"http://192.0.2.10:7877"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("mint: %d %s", rec.Code, rec.Body.String())
	}
	var minted struct {
		Offer string `json:"offer"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &minted); err != nil || minted.Offer == "" {
		t.Fatalf("mint body has no offer: %v %s", err, rec.Body.String())
	}
	// The devices list never carries it.
	if body := getPairing(t, h).Body.String(); strings.Contains(body, minted.Offer) {
		t.Fatal("the devices list carries the offer")
	}
	// Neither do refusal bodies (the duplicate-label one reuses the label).
	for _, tc := range []struct{ path, body string }{
		{"/desktop/pairing/mint", `{"label":"phone","scope":"serve","endpoint":"http://192.0.2.10:7877"}`},
		{"/desktop/pairing/mint", `{"label":"p2","scope":"terminal","endpoint":"http://192.0.2.10:7877"}`},
		{"/desktop/pairing/revoke", `{"selector":"deadbeef"}`},
		{"/desktop/pairing/revoke", `{"selector":"_home"}`},
	} {
		rec := postPairing(t, h, tc.path, tc.body)
		if strings.Contains(rec.Body.String(), minted.Offer) {
			t.Fatalf("%s %s leaked the offer: %s", tc.path, tc.body, rec.Body.String())
		}
	}
}
