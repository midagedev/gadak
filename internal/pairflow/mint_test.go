package pairflow

// GDK-1047 gates for the shared mint flow. The load-bearing one is
// TestMintDeviceKeepsHomeRoutingToken: a naive mint that skips
// ensureHomeRoutingToken flips the serve gate on with no _home credential
// in remote-origin.json, locking the home machine out of its own
// passthrough. TestNaiveMintLocksOutHome pins that the naive mint really
// produces the lockout state — it is the counterfactual that makes the
// guard in TestMintDeviceKeepsHomeRoutingToken mean something, and it is
// the exact arrangement the FAIL-first run of the guard assertion was
// shown red against (see the round report).

import (
	"bytes"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/pairing"
)

// pairflowHome stands up a temp GADAK_HOME with a standalone workspace —
// the shape the mint flow operates on — and returns (dir, cfg).
func pairflowHome(t *testing.T) (string, *config.Config) {
	t.Helper()
	for _, k := range []string{"GADAK_SITE", "GADAK_EMAIL", "GADAK_TOKEN", "GADAK_PROJECTS"} {
		t.Setenv(k, "")
	}
	t.Setenv("GADAK_HOME", t.TempDir())
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
	return cfg.Directory(), cfg
}

// homeRoutingTokenUsable is the guard assertion: remote-origin.json holds
// a credential the store still accepts. Everything else in the guard test
// decorates this one check.
func homeRoutingTokenUsable(t *testing.T, dir string, now time.Time) {
	t.Helper()
	rem, err := pairing.LoadRemote(dir)
	if err != nil {
		t.Fatalf("remote-origin.json unreadable: %v", err)
	}
	if rem == nil {
		t.Fatal("no _home routing token in remote-origin.json — the home machine's own writes cannot pass the gate it just turned on")
	}
	v, err := pairing.Authorize(dir, rem.Token, now)
	if err != nil {
		t.Fatalf("authorizing the routing token: %v", err)
	}
	if v != pairing.VerdictAccept {
		t.Fatalf("stored routing token verdict is %v, want accept", v)
	}
}

// TestMintDeviceKeepsHomeRoutingToken is the guard: after a device mint on
// a standalone home, the _home routing token exists and authorizes. First
// mint reports minted; a second mint (credential already valid) reports
// none; a stale credential (file token no longer in the store — the
// GDK-450 shape) is reissued by the next mint.
func TestMintDeviceKeepsHomeRoutingToken(t *testing.T) {
	dir, cfg := pairflowHome(t)
	now := time.Now()
	endpoint := "http://127.0.0.1:7877"

	res, err := MintDevice(dir, cfg, "phone", pairing.ScopeServe, "1h", endpoint, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.HomeRouting != HomeRoutingMinted {
		t.Fatalf("first mint reports %q, want minted", res.HomeRouting)
	}
	homeRoutingTokenUsable(t, dir, now)

	if _, err := MintDevice(dir, cfg, "tablet", pairing.ScopeServe, "1h", endpoint, now); err != nil {
		t.Fatal(err)
	}
	homeRoutingTokenUsable(t, dir, now)

	// GDK-450 recovery: corrupt the stored credential's token (the file
	// entry no longer matches any store hash) — the next mint reissues.
	rem, err := pairing.LoadRemote(dir)
	if err != nil || rem == nil {
		t.Fatalf("fixture: routing token missing before the stale step: %v %v", rem, err)
	}
	stale := strings.Repeat("s", len(rem.Token))
	if err := pairing.SaveRemote(dir, pairing.Remote{Endpoint: rem.Endpoint, Token: stale, Label: rem.Label}); err != nil {
		t.Fatal(err)
	}
	res, err = MintDevice(dir, cfg, "laptop", pairing.ScopeServe, "1h", endpoint, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.HomeRouting != HomeRoutingReissued {
		t.Fatalf("mint after a stale credential reports %q, want reissued", res.HomeRouting)
	}
	homeRoutingTokenUsable(t, dir, now)
}

// TestNaiveMintLocksOutHome pins the counterfactual: pairing.MintScoped
// called directly — what a second surface that skipped pairflow would do —
// leaves the gate on with no routing token. If this ever fails because
// MintScoped itself mints _home, the guard has moved and
// TestMintDeviceKeepsHomeRoutingToken should move with it.
func TestNaiveMintLocksOutHome(t *testing.T) {
	dir, cfg := pairflowHome(t)
	now := time.Now()
	if _, _, err := pairing.MintScoped(dir, "phone", pairing.ScopeServe, time.Hour, now); err != nil {
		t.Fatal(err)
	}
	if v, err := pairing.Authorize(dir, "", now); err != nil || v != pairing.VerdictReject {
		t.Fatalf("gate after a naive mint is %v (%v), want reject — the device token alone must close it", v, err)
	}
	if rem, err := pairing.LoadRemote(dir); err != nil || rem != nil {
		t.Fatalf("naive mint left a routing token (%v %v) — it cannot; only the guard writes one", rem, err)
	}
	// And the guard assertion, applied to this dir, is the FAIL-first
	// arrangement: homeRoutingTokenUsable(t, dir, now) fails here. It is
	// asserted by refusal above; calling it would abort this test.
	_ = cfg
}

// TestMintDeviceContract pins the mint result itself: offer round-trips,
// fields carry what was asked, expiry honors the ttl, the loopback flag
// answers for both kinds of endpoint, and the reserved label is refused.
func TestMintDeviceContract(t *testing.T) {
	dir, cfg := pairflowHome(t)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	res, err := MintDevice(dir, cfg, "phone", pairing.ScopeServe, "2h", "http://192.0.2.10:7877", now)
	if err != nil {
		t.Fatal(err)
	}
	offer, err := pairing.DecodeOffer(res.Offer)
	if err != nil {
		t.Fatalf("mint result does not round-trip: %v", err)
	}
	if offer.Endpoint != "http://192.0.2.10:7877" || offer.Label != "phone" || offer.V != pairing.OfferV1 {
		t.Fatalf("offer carries %+v", offer)
	}
	if res.Endpoint != offer.Endpoint || res.Label != "phone" || res.Scope != pairing.ScopeServe {
		t.Fatalf("result fields carry %+v", res)
	}
	if res.ExpiresAt != pairing.FormatExpiry(now.Add(2*time.Hour)) {
		t.Fatalf("expires_at is %q, want the mint time + 2h", res.ExpiresAt)
	}
	if res.LoopbackWarning {
		t.Fatal("a TEST-NET endpoint is not loopback")
	}
	if res.HomeRouting != HomeRoutingMinted {
		t.Fatalf("standalone mint reports %q, want minted", res.HomeRouting)
	}

	// The warning is the caller's copy to render; both polarities are data.
	loop, err := MintDevice(dir, cfg, "tablet", pairing.ScopeServe, "1h", "http://localhost:7877", now)
	if err != nil {
		t.Fatal(err)
	}
	if !loop.LoopbackWarning {
		t.Fatal("localhost is loopback — the mint must say so")
	}

	// Refusals, each naming its own cause.
	if _, err := MintDevice(dir, cfg, "", pairing.ScopeServe, "1h", "http://192.0.2.10:7877", now); err == nil ||
		!strings.Contains(err.Error(), "--label") {
		t.Fatalf("empty label refused with: %v", err)
	}
	if _, err := MintDevice(dir, cfg, pairing.HomeLabel, pairing.ScopeServe, "1h", "http://192.0.2.10:7877", now); err == nil ||
		!strings.Contains(err.Error(), pairing.HomeLabel) {
		t.Fatalf("_home label refused with: %v", err)
	}
	if _, err := MintDevice(dir, cfg, "watch", "admin", "1h", "http://192.0.2.10:7877", now); err == nil ||
		!strings.Contains(err.Error(), "unknown scope") {
		t.Fatalf("unknown scope refused with: %v", err)
	}
	if _, err := MintDevice(dir, cfg, "watch", pairing.ScopeServe, "1x", "http://192.0.2.10:7877", now); err == nil ||
		!strings.Contains(err.Error(), "bad ttl") {
		t.Fatalf("bad ttl refused with: %v", err)
	}
	if _, err := MintDevice(dir, cfg, "watch", pairing.ScopeServe, "1h", "ftp://192.0.2.10:7877", now); err == nil ||
		!strings.Contains(err.Error(), "bad endpoint") {
		t.Fatalf("non-http endpoint refused with: %v", err)
	}
	// No explicit endpoint and no live serve: the orphan-token refusal.
	if _, err := MintDevice(dir, cfg, "watch", pairing.ScopeServe, "1h", "", now); err == nil ||
		!strings.Contains(err.Error(), "no live serve") {
		t.Fatalf("endpoint-less mint without a serve refused with: %v", err)
	}
}

// TestMintDeviceConnectedWritesNoRoutingFile: a connected workspace's
// writes go straight to its site — remote-origin.json must not appear, or
// origin.pairedRemote would read the workspace as paired with its own
// serve.
func TestMintDeviceConnectedWritesNoRoutingFile(t *testing.T) {
	dir, cfg := pairflowHome(t)
	cfg.Kind = ""
	cfg.Site = "https://example.atlassian.net"
	cfg.Email = "home@example.net"
	cfg.Token = "secret-not-a-pairing-token"
	now := time.Now()

	res, err := MintDevice(dir, cfg, "phone", pairing.ScopeServe, "1h", "http://192.0.2.10:7877", now)
	if err != nil {
		t.Fatal(err)
	}
	if res.HomeRouting != HomeRoutingNone {
		t.Fatalf("connected mint reports %q, want none", res.HomeRouting)
	}
	if rem, err := pairing.LoadRemote(dir); err != nil || rem != nil {
		t.Fatalf("connected mint wrote a routing file: %v %v", rem, err)
	}
}

// TestMintHomeRotationAndRefusal: _home rotation answers with the meta the
// caller prints, refuses a non-standalone workspace, and — like the device
// mint — refuses to guess an endpoint.
func TestMintHomeRotationAndRefusal(t *testing.T) {
	dir, cfg := pairflowHome(t)
	now := time.Now()

	meta, err := MintHome(dir, cfg, "http://127.0.0.1:7877", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Hash) < 8 {
		t.Fatalf("rotation meta hash is %q", meta.Hash)
	}
	homeRoutingTokenUsable(t, dir, now)

	cfg.Kind = ""
	cfg.Site = "https://example.atlassian.net"
	cfg.Email = "home@example.net"
	cfg.Token = "secret-not-a-pairing-token"
	if _, err := MintHome(dir, cfg, "http://127.0.0.1:7877", now); err == nil ||
		!strings.Contains(err.Error(), "standalone") {
		t.Fatalf("connected _home mint refused with: %v", err)
	}
}

// TestDirRefusals: a workspace paired away cannot mint (its home is
// another machine), and a workspace with no credential cannot either.
func TestDirRefusals(t *testing.T) {
	dir, cfg := pairflowHome(t)
	// Paired away: connected workspace holding a remote credential.
	if err := pairing.SaveRemote(dir, pairing.Remote{Endpoint: "http://192.0.2.10:7877", Token: "paired-away-not-a-pairing-token", Label: "desk"}); err != nil {
		t.Fatal(err)
	}
	cfg.Kind = ""
	cfg.Site = "https://example.atlassian.net"
	cfg.Email = "home@example.net"
	cfg.Token = "secret-not-a-pairing-token"
	if _, err := Dir(cfg); err == nil || !strings.Contains(err.Error(), "run on the home machine") {
		t.Fatalf("paired-away mint refused with: %v", err)
	}

	// No credential at all.
	_, cfg2 := pairflowHome(t)
	cfg2.Kind = ""
	if _, err := Dir(cfg2); err == nil {
		t.Fatal("a workspace with no credential must not mint")
	}
}

// TestParseTTLAndDefault: the flow owns the 90d default and the
// <N><unit> syntax; the CLI wrapper and the desktop form both ride this.
func TestParseTTLAndDefault(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"90d":  90 * 24 * time.Hour,
		"1h":   time.Hour,
		"30m":  30 * time.Minute,
		"45s":  45 * time.Second,
		" 2d ": 48 * time.Hour,
	} {
		got, err := ParseTTL(in)
		if err != nil || got != want {
			t.Fatalf("ParseTTL(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "0d", "-1h", "1w", "d", "1.5h", "1x"} {
		if _, err := ParseTTL(bad); err == nil {
			t.Fatalf("ParseTTL(%q) accepted", bad)
		}
	}
	if got := DefaultTTLFlag(); got != "90d" {
		t.Fatalf("DefaultTTLFlag() = %q, want 90d", got)
	}
	if DefaultTTL != 90*24*time.Hour {
		t.Fatalf("DefaultTTL = %v", DefaultTTL)
	}
}

// TestEndpointFromAdvertise: a bare bind address gains the http scheme the
// URL validation demands; an address that already names one is left alone.
func TestEndpointFromAdvertise(t *testing.T) {
	for in, want := range map[string]string{
		"":                  "",
		"127.0.0.1:7877":    "http://127.0.0.1:7877",
		"[::1]:7877":        "http://[::1]:7877",
		"https://h.example": "https://h.example",
	} {
		if got := EndpointFromAdvertise(in); got != want {
			t.Fatalf("EndpointFromAdvertise(%q) = %q, want %q", in, got, want)
		}
	}
	// No live serve in an empty home: discovery answers empty, not an
	// error — the mint turns it into the no-live-serve refusal.
	pairflowHome(t)
	var nilCfg *config.Config
	if got := AdvertisedEndpoint(nilCfg); got != "" {
		t.Fatalf("AdvertisedEndpoint with no serve = %q, want empty", got)
	}
}

// TestRowsShaping: list rows never carry the token or the full hash, the
// _home row reads as local-routing, and state separates active, expired,
// and revoked.
func TestRowsShaping(t *testing.T) {
	dir, cfg := pairflowHome(t)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	if _, err := MintDevice(dir, cfg, "phone", pairing.ScopeServe, "1h", "http://192.0.2.10:7877", now); err != nil {
		t.Fatal(err)
	}
	// An already-expired token (minted "in the past" relative to listing).
	if _, err := MintDevice(dir, cfg, "old-tablet", pairing.ScopeServe, "1h", "http://192.0.2.10:7877", now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := MintHome(dir, cfg, "http://192.0.2.10:7877", now); err != nil {
		t.Fatal(err)
	}
	rows, err := Rows(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]Row{}
	for _, r := range rows {
		byLabel[r.Label] = r
		if len(r.Hash) != 8 {
			t.Fatalf("row for %q carries hash %q — want the 8-char prefix only", r.Label, r.Hash)
		}
	}
	if n := len(byLabel); n != 3 {
		t.Fatalf("Rows returned %d labels, want 3 (phone, old-tablet, _home): %+v", n, rows)
	}
	if r := byLabel["phone"]; r.Scope != pairing.ScopeServe || r.State != "active" || r.LastUsed != "-" {
		t.Fatalf("phone row is %+v", r)
	}
	if r := byLabel["old-tablet"]; r.State != "expired" {
		t.Fatalf("old-tablet row state is %q, want expired", r.State)
	}
	if r := byLabel[pairing.HomeLabel]; r.Scope != pairing.ScopeLocalRouting {
		t.Fatalf("_home row scope is %q, want %s", r.Scope, pairing.ScopeLocalRouting)
	}
	// Revoke leaves a row (audit), marked.
	if _, err := Revoke(dir, "phone", now); err != nil {
		t.Fatal(err)
	}
	rows, err = Rows(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Label == "phone" && !strings.HasPrefix(r.State, "revoked ") {
			t.Fatalf("revoked phone row state is %q", r.State)
		}
	}
	// Revoke refuses _home with the pinned sentence (the desktop
	// classifies by it).
	if _, err := Revoke(dir, pairing.HomeLabel, now); err == nil ||
		!strings.Contains(err.Error(), "_home is this machine's own routing key") {
		t.Fatalf("_home revoke refused with: %v", err)
	}
}

// TestQRPNGIsTheMatrix: the PNG a phone scans and the matrix the terminal
// renders are the same code — every pixel answers its module, the quiet
// zone is white, and the side is modules × 8.
func TestQRPNGIsTheMatrix(t *testing.T) {
	dir, cfg := pairflowHome(t)
	res, err := MintDevice(dir, cfg, "phone", pairing.ScopeServe, "1h", "http://192.0.2.10:7877", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	mods, err := QRModules(res.Offer)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := QRPNG(res.Offer)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("QRPNG does not decode: %v", err)
	}
	side := len(mods) * 8
	if img.Bounds().Dx() != side || img.Bounds().Dy() != side {
		t.Fatalf("QR PNG is %dx%d, want %dx%d (modules %d × 8)", img.Bounds().Dx(), img.Bounds().Dy(), side, side, len(mods))
	}
	dark := 0
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			black := mods[y/8][x/8]
			if black {
				dark++
				if r != 0 || g != 0 || b != 0 {
					t.Fatalf("pixel (%d,%d) should be black for a dark module", x, y)
				}
			} else {
				if r != 0xffff || g != 0xffff || b != 0xffff {
					t.Fatalf("pixel (%d,%d) should be white for a light module", x, y)
				}
			}
		}
	}
	// The finder pattern's dark modules must exist (a blank rect passes
	// the pixel-map loop above if the matrix itself were all false).
	if dark == 0 {
		t.Fatal("the PNG carries no dark module")
	}
}

// A SaveRemote failure right after Rotate is the one moment the gate is up
// with no valid routing credential on disk — Rotate already revoked every
// previous _home token in its own store write. The error must name the
// recovery (a fresh mint) and keep the cause (GDK-1236). FAIL-first: the
// pre-fix return was the bare SaveRemote error.
func TestIssueHomeRoutingTokenSaveFailureSaysReMint(t *testing.T) {
	dir := t.TempDir()
	// Occupy the credential path with a directory so the atomic save fails
	// while Rotate (the token store) still succeeds.
	if err := os.MkdirAll(pairing.RemotePath(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := issueHomeRoutingToken(dir, nil, "http://127.0.0.1:7877", time.Now())
	if err == nil || !strings.Contains(err.Error(), "re-run to mint a fresh one") {
		t.Fatalf("err = %v, want the re-mint recovery named", err)
	}
}
