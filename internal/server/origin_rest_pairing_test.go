package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

// The pairing gate on the origin passthrough (GDK-433). Fixture and request
// helpers come from origin_rest_test.go: standaloneServer builds a real
// standalone workspace under a temp GADAK_HOME, get() drives the Handler.
//
// Gate contract under test:
//   - no active token  -> passthrough behaves exactly as before (①)
//   - active token + missing/wrong/expired/revoked Bearer -> 401 (②④)
//   - active token + valid Bearer -> 200, last_used_at recorded (③)
//
// The in-process Basic credential ("standalone:standalone") is what today's
// local CLI sends; a request carrying it must also be refused once the gate
// is on — a tailnet proxy reaches the serve as loopback, so "local-looking"
// is not an identity.

func basicStandalone(t *testing.T) string {
	t.Helper()
	return "Basic " + origin.InProcessAuthB64()
}

func TestPairingGateOffWithoutTokens(t *testing.T) {
	h, cfg := standaloneServer(t)
	if _, err := pairing.List(cfg.Directory()); err != nil {
		t.Fatal(err)
	}
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": basicStandalone(t),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("no tokens minted: %d %s; want 200 (today's behavior)", rec.Code, rec.Body.String())
	}
}

func TestPairingGateRejectsWithoutValidBearer(t *testing.T) {
	h, cfg := standaloneServer(t)
	dir := cfg.Directory()
	token, _, err := pairing.Mint(dir, "laptop", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for name, headers := range map[string]map[string]string{
		// Today's local-CLI shape: Basic in-process credential, no Bearer.
		"basic only": {"Authorization": basicStandalone(t)},
		"no auth":    nil,
		"wrong bear": {"Authorization": "Bearer " + token + "x"},
		"other schm": {"Authorization": "Token " + token},
	} {
		rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", headers)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d %s; want 401", name, rec.Code, rec.Body.String())
		}
		// GDK-453 (2026-08-21): 401 is pairing_rejected + reason, not
		// pairing_token_required. FAIL-first: TestPairingGate401CarriesRejectReason.
		if !strings.Contains(rec.Body.String(), "pairing_rejected") {
			t.Fatalf("%s: body %s, want pairing_rejected", name, rec.Body.String())
		}
	}
}

func TestPairingGateAcceptsValidBearer(t *testing.T) {
	h, cfg := standaloneServer(t)
	dir := cfg.Directory()
	token, meta, err := pairing.Mint(dir, "laptop", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": "Bearer " + token,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("valid bearer: %d %s", rec.Code, rec.Body.String())
	}
	got, err := pairing.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got {
		if m.Hash == meta.Hash && m.LastUsedAt == nil {
			t.Fatal("accepted request did not record last_used_at")
		}
	}
}

func TestPairingGate401CarriesRejectReason(t *testing.T) {
	// GDK-453: the home serve knows why a token died; 401 must name
	// revoked/expired for tokens that existed, and unknown otherwise.
	h, cfg := standaloneServer(t)
	dir := cfg.Directory()
	revoked, _, err := pairing.Mint(dir, "revoked-device", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pairing.Mint(dir, "witness", time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := pairing.Revoke(dir, "revoked-device", time.Now()); err != nil {
		t.Fatal(err)
	}

	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": "Bearer " + revoked,
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked: %d %s; want 401", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Gadak-Pairing") != "revoked" {
		t.Fatalf("X-Gadak-Pairing = %q, want revoked; body %s", rec.Header().Get("X-Gadak-Pairing"), rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"error":"pairing_rejected"`) || !strings.Contains(rec.Body.String(), `"reason":"revoked"`) {
		t.Fatalf("body %s, want pairing_rejected reason=revoked", rec.Body.String())
	}

	rec = get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": "Bearer not-a-real-token",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown: %d %s; want 401", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Gadak-Pairing") != "unknown" {
		t.Fatalf("unknown token must not reveal existence: header %q body %s", rec.Header().Get("X-Gadak-Pairing"), rec.Body.String())
	}
}

func TestPairingGateExpiredAndRevokedRejected(t *testing.T) {
	h, cfg := standaloneServer(t)
	dir := cfg.Directory()
	keeper, _, err := pairing.Mint(dir, "keeper", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// A 1ns TTL is expired by the time the request runs.
	expired, _, err := pairing.Mint(dir, "expired", time.Nanosecond, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// A revoked token is only provably rejected while another *active*
	// token keeps the gate on — with none left, the gate lifts by design.
	if _, _, err := pairing.Mint(dir, "witness", time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}
	rec := get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": "Bearer " + expired,
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired token: %d %s; want 401 (keeper/witness keep the gate on)", rec.Code, rec.Body.String())
	}
	if _, err := pairing.Revoke(dir, "keeper", time.Now()); err != nil {
		t.Fatal(err)
	}
	rec = get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": "Bearer " + keeper,
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token: %d %s; want 401 (witness keeps the gate on)", rec.Code, rec.Body.String())
	}
	if _, err := pairing.Revoke(dir, "witness", time.Now()); err != nil {
		t.Fatal(err)
	}
	// With every token inactive the gate lifts: requests pass as before.
	rec = get(t, h, origin.RESTPrefix+"/rest/api/3/myself", map[string]string{
		"Authorization": basicStandalone(t),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("gate must lift when no active token remains: %d %s", rec.Code, rec.Body.String())
	}
}

// A real tailnet delivers the passthrough with the MagicDNS name in Host —
// tailscale serve forwards the original Host upstream (measured, GDK-443).
// The rebinding guard must step aside for exactly that case and nothing else:
// only under RESTPrefix, and only while active tokens make the Bearer gate
// stand right behind it.
func TestPairedHostExemptLetsTailnetNameThrough(t *testing.T) {
	h, cfg := standaloneServer(t)
	dir := cfg.Directory()
	// Any DNS name exercises the same rejection as a MagicDNS *.ts.net one;
	// an example.com shape keeps the secret scanner's tailnet regex quiet.
	tsHost := map[string]string{"Host": "home.tailnet.example.com:8443"}

	// No tokens: a DNS-named Host stays forbidden — today's rebinding answer.
	rec := getWithHost(t, h, origin.RESTPrefix+"/rest/api/3/myself", nil, tsHost["Host"])
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "forbidden_host") {
		t.Fatalf("no tokens: %d %s; want 403 forbidden_host", rec.Code, rec.Body.String())
	}

	token, _, err := pairing.Mint(dir, "laptop", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Tokens exist, no Bearer: the guard defers, the gate rejects.
	rec = getWithHost(t, h, origin.RESTPrefix+"/rest/api/3/myself", nil, tsHost["Host"])
	// GDK-453 (2026-08-21): same 401 shape as TestPairingGateRejectsWithoutValidBearer.
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "pairing_rejected") {
		t.Fatalf("tokens, no bearer: %d %s; want 401 pairing_rejected", rec.Code, rec.Body.String())
	}

	// Tokens exist, valid Bearer: the paired request goes through.
	rec = getWithHost(t, h, origin.RESTPrefix+"/rest/api/3/myself",
		map[string]string{"Authorization": "Bearer " + token}, tsHost["Host"])
	if rec.Code != http.StatusOK {
		t.Fatalf("tokens, valid bearer: %d %s; want 200", rec.Code, rec.Body.String())
	}

	// The exemption is passthrough-or-allowlist, never the wider API. An
	// allowlisted mirror path with tokens minted defers to the mirror gate
	// (GDK-797): no Bearer is a pairing rejection, and this origin-scope
	// token is the wrong scope for the mirror — one-way doors.
	rec = getWithHost(t, h, "/api/v1/issues/views/", nil, tsHost["Host"])
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "pairing_rejected") {
		t.Fatalf("allowlisted path, no bearer: %d %s; want 401 pairing_rejected", rec.Code, rec.Body.String())
	}
	rec = getWithHost(t, h, "/api/v1/issues/views/",
		map[string]string{"Authorization": "Bearer " + token}, tsHost["Host"])
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "scope_rejected") {
		t.Fatalf("allowlisted path, origin token: %d %s; want 403 scope_rejected", rec.Code, rec.Body.String())
	}
	// Everything off both surfaces keeps the guard even with tokens minted —
	// the tailnet name never opens the credential API or the UI.
	rec = getWithHost(t, h, "/api/v1/issues/credential/", nil, tsHost["Host"])
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "forbidden_host") {
		t.Fatalf("non-exempt path: %d %s; want 403 forbidden_host", rec.Code, rec.Body.String())
	}
}

func getWithHost(t *testing.T, h http.Handler, path string, headers map[string]string, host string) *httptest.ResponseRecorder {
	t.Helper()
	req := testRequest(http.MethodGet, path, nil)
	req.Host = host
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
