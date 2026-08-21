package main

import (
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

// seedPairedProfile is a workspace whose origin is a remote gadak serve
// (remote-origin.json, not KindStandalone). That is the GDK-452 shape:
// `init --pairing-code` already refuses to re-pair it, but bare init and
// `init --standalone` currently rebind the origin.
func seedPairedProfile(t *testing.T) string {
	t.Helper()
	dir := emptyHome(t)
	if err := pairing.SaveRemote(dir, pairing.Remote{
		Endpoint: "https://home.ts.net:8443",
		Token:    "pair-token-keep",
		Label:    "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AccountID = "acc-pair"
	cfg.TokenOwner = "Home User"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertPairedOriginRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("init on a paired profile must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, `this workspace is paired with "laptop"`) {
		t.Fatalf("want paired-with-label refusal, got: %v", err)
	}
	if !strings.Contains(msg, "https://home.ts.net:8443") {
		t.Fatalf("want endpoint in refusal, got: %v", err)
	}
	if !strings.Contains(msg, "gadak --workspace") || !strings.Contains(msg, "init") {
		t.Fatalf("want next-action new-profile init, got: %v", err)
	}
	if strings.Contains(msg, "later slice") {
		t.Fatalf("internal roadmap leaked into user copy: %v", err)
	}
}

func assertPairedOriginUntouched(t *testing.T, dir string) {
	t.Helper()
	rem, err := pairing.LoadRemote(dir)
	if err != nil || rem == nil {
		t.Fatalf("remote credential must survive a refused init: %+v (%v)", rem, err)
	}
	if rem.Token != "pair-token-keep" || rem.Label != "laptop" || rem.Endpoint != "https://home.ts.net:8443" {
		t.Fatalf("refused init rebound the pairing credential: %+v", rem)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IsStandalone() {
		t.Fatal("refused init must not flip the workspace to standalone")
	}
	if cfg.Site != "" || cfg.Token != "" {
		t.Fatalf("refused init must not write a site credential: site=%q token_set=%t", cfg.Site, cfg.Token != "")
	}
	if _, err := os.Stat(origin.PersistPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("refused init must not create a local persist: %v", err)
	}
}

// TestInitPairedRefusesStandalone is GDK-452: `gadak init --standalone` on a
// profile bound to a remote serve must not replace the origin with an empty
// local persist.
func TestInitPairedRefusesStandalone(t *testing.T) {
	dir := seedPairedProfile(t)
	_, err := capture(t, func() error {
		return cmdInit([]string{"--standalone"})
	})
	assertPairedOriginRefusal(t, err)
	assertPairedOriginUntouched(t, dir)
}

// TestInitPairedRefusesBare is GDK-452: non-tty `gadak init` on a paired
// profile must refuse as paired, not fall through to the Jira wizard.
func TestInitPairedRefusesBare(t *testing.T) {
	dir := seedPairedProfile(t)
	withClosedStdin(t, func() {
		_, err := capture(t, func() error {
			return cmdInit(nil)
		})
		assertPairedOriginRefusal(t, err)
	})
	assertPairedOriginUntouched(t, dir)
}

func TestInitPairedRefusesSiteFlags(t *testing.T) {
	dir := seedPairedProfile(t)
	srv := myselfServer(t)
	withClosedStdin(t, func() {
		_, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, dir, "id-token"),
			})
		})
		assertPairedOriginRefusal(t, err)
	})
	assertPairedOriginUntouched(t, dir)
}

func TestInitPairedRefusesRepairing(t *testing.T) {
	dir := seedPairedProfile(t)
	srv, _ := pairingOriginServe(t, 200)
	offer := mustOffer(t, pairing.Offer{
		V: pairing.OfferV1, Endpoint: srv.URL, Token: "fresh-token", Label: "phone",
	})
	_, _, err := captureErr(t, func() error {
		return cmdInit([]string{"--pairing-code", offer})
	})
	assertPairedOriginRefusal(t, err)
	assertPairedOriginUntouched(t, dir)
}

// Home machines store a routing token in the same file. That is not a
// paired origin (origin.pairedRemote excludes standalone); init --standalone
// must not pick up the paired-origin refusal.
func TestInitStandaloneHomeRoutingNotRefused(t *testing.T) {
	pairingHome(t)
	_, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop", "--ttl", "1h", "--endpoint", "http://127.0.0.1:9"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capture(t, func() error {
		return cmdInit([]string{"--standalone", "--json"})
	}); err != nil {
		t.Fatalf("home routing token must not block init --standalone: %v", err)
	}
}
