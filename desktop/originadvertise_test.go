package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
)

func standaloneApp(t *testing.T) (*config.Config, *server.Handler) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		origin.SetInProcess(false)
		config.SetProfile("")
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindStandalone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(home, "mirror.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	api := server.New(db, cfg)
	t.Cleanup(func() { _ = api.Close() })
	return cfg, api
}

// TestGDK340StandaloneAppAdvertisesOrigin is the desktop side of
// route_test: an app holding a standalone workspace must advertise a
// loopback origin passthrough, or a concurrent CLI dual-writes persist.
// FAIL-first evidence lives in the report for GDK-340 — with the
// pre-fix no-op wiring this fails at "advertise file missing".
func TestGDK340StandaloneAppAdvertisesOrigin(t *testing.T) {
	cfg, api := standaloneApp(t)
	origin.SetInProcess(true)

	stop, err := startStandaloneOriginListener(cfg, api)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	advPath := origin.AdvertisePath(cfg.Directory())
	data, err := os.ReadFile(advPath)
	if err != nil {
		t.Fatalf("FAIL-first GDK-340: advertise file missing while the app holds a standalone workspace: %v", err)
	}
	var adv origin.Advertise
	if err := json.Unmarshal(data, &adv); err != nil {
		t.Fatal(err)
	}
	if adv.Addr == "" || adv.PID != os.Getpid() {
		t.Fatalf("advertise doc %+v", adv)
	}

	// The probe path answers with the identity headers port_fallback and
	// origin.probeMatches require.
	res, err := http.Get("http://" + adv.Addr + origin.ProbePath)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	res.Body.Close()
	if res.Header.Get("X-Gadak") != "1" {
		t.Fatalf("probe response lacks X-Gadak (status %d)", res.StatusCode)
	}
	if got := res.Header.Get("X-Gadak-Profile"); got != config.Profile() {
		t.Fatalf("X-Gadak-Profile = %q, want %q", got, config.Profile())
	}

	// The passthrough serves real origin REST: myself on the embedded
	// issuetap. This is the route a CLI write takes.
	req, err := http.NewRequest(http.MethodGet, "http://"+adv.Addr+origin.RESTPrefix+"/rest/api/3/myself", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("standalone:standalone")))
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("passthrough: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("passthrough /rest/api/3/myself = %d", res.StatusCode)
	}

	// stop withdraws the advertise file.
	stop()
	if _, err := os.Stat(advPath); !os.IsNotExist(err) {
		t.Fatalf("advertise file still present after stop: %v", err)
	}
}

// The listener is origin-passthrough only — the app must not grow a full
// loopback UI/API surface ("no forced server/port" invariant).
func TestGDK340ListenerServesOnlyOriginPaths(t *testing.T) {
	cfg, api := standaloneApp(t)
	origin.SetInProcess(true)

	stop, err := startStandaloneOriginListener(cfg, api)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	data, err := os.ReadFile(origin.AdvertisePath(cfg.Directory()))
	if err != nil {
		t.Fatalf("advertise file: %v", err)
	}
	var adv origin.Advertise
	if err := json.Unmarshal(data, &adv); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		"/",
		"/api/v1/bootstrap/",
		"/api/v1/issues/",
		"/config.json",
		"/healthz",
		"/api/v1/issues/sync/progress/deeper", // subtree of the probe path
	} {
		res, err := http.Get("http://" + adv.Addr + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", p, res.StatusCode)
		}
	}
}

// A connected workspace opens no listener and writes no advertise file.
func TestGDK340ConnectedAppNoListener(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	stop, err := startStandaloneOriginListener(cfg, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if _, err := os.Stat(origin.AdvertisePath(home)); !os.IsNotExist(err) {
		t.Fatalf("connected workspace wrote advertise: %v", err)
	}
}
