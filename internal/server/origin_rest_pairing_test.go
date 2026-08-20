package server

import (
	"net/http"
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
		if !strings.Contains(rec.Body.String(), "pairing_token_required") {
			t.Fatalf("%s: body %s, want pairing_token_required", name, rec.Body.String())
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
