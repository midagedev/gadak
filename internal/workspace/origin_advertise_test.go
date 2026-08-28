package workspace

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
)

func seedStandaloneProfile(t *testing.T, name string) *config.Config {
	t.Helper()
	seedProfile(t, name, &config.Config{Projects: []string{origin.DefaultProjectKey}})
	cfg, err := config.LoadFor(name)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindStandalone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.LoadFor(name)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestSTD3MountedStandaloneBindsOrigin(t *testing.T) {
	setupHome(t)
	cfg := seedStandaloneProfile(t, "probe")

	reg := New()
	t.Cleanup(func() {
		_ = origin.Close()
		origin.ResetInProcess()
	})
	t.Cleanup(reg.Close)

	e, err := reg.Get("probe")
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("mounted standalone entry is nil")
	}
	if !e.ownsOrigin {
		t.Fatal("mounted standalone did not bind its origin")
	}
	if _, err := os.Stat(cfg.Directory() + "/serve-origin.json"); !os.IsNotExist(err) {
		t.Fatal("mounted standalone wrote leftover advertise file")
	}

	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("mount owner Client: %v", err)
	}
	key, err := c.CreateIssue(context.Background(), map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "STD-3 mount owner write",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("mount owner CreateIssue: %v", err)
	}

	origin.ForgetLive()
	origin.ResetInProcess()

	c2, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("simulated CLI Client: %v", err)
	}
	if !origin.TransportIsEmbedded(c2.HTTP.Transport) {
		t.Fatalf("simulated CLI transport %T, want embedded", c2.HTTP.Transport)
	}
	if _, err := c2.CreateIssue(context.Background(), map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "STD-3 second embed write",
		"issuetype": map[string]any{"name": "Task"},
	}); err != nil {
		t.Fatalf("second embed CreateIssue: %v", err)
	}
	_ = key
}

func TestMountedConnectedDoesNotOwnOrigin(t *testing.T) {
	setupHome(t)
	seedProfile(t, "connected", &config.Config{Projects: []string{"CON"}})
	cfg, err := config.LoadFor("connected")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IsStandalone() {
		t.Fatal("fixture unexpectedly standalone")
	}

	reg := New()
	t.Cleanup(func() {
		_ = origin.Close()
		origin.ResetInProcess()
	})
	t.Cleanup(reg.Close)
	entry, err := reg.Get("connected")
	if err != nil {
		t.Fatal(err)
	}
	if entry.ownsOrigin {
		t.Fatal("connected mount claimed an origin")
	}
}

func TestMountedStandaloneCloseReleasesOrigin(t *testing.T) {
	setupHome(t)
	cfg := seedStandaloneProfile(t, "probe")

	reg := New()
	t.Cleanup(func() {
		_ = origin.Close()
		origin.ResetInProcess()
	})
	t.Cleanup(reg.Close)
	entry, err := reg.Get("probe")
	if err != nil {
		t.Fatal(err)
	}
	if !entry.ownsOrigin {
		t.Fatal("mounted standalone did not claim its origin")
	}

	reg.Close()
	if origin.IsInProcess(cfg) {
		t.Fatal("in-process mark remains after Registry.Close")
	}
	before := origin.SessionsConstructed()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("Client after Registry.Close: %v", err)
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("Client after Registry.Close transport %T, want embedded", c.HTTP.Transport)
	}
	if got := origin.SessionsConstructed(); got != before+1 {
		t.Fatalf("SessionsConstructed %d → %d, want +1 after persist release", before, got)
	}
}

func TestRegistryCloseWaitsForOriginOwnership(t *testing.T) {
	setupHome(t)
	_ = seedStandaloneProfile(t, "probe")
	reg := New()
	t.Cleanup(func() {
		_ = origin.Close()
		origin.ResetInProcess()
	})
	t.Cleanup(reg.Close)

	started := make(chan struct{})
	release := make(chan struct{})
	testBeforeOriginAcquire = func(name string) {
		if name != "probe" {
			t.Errorf("ownership attempt for %q, want probe", name)
		}
		close(started)
		<-release
	}
	t.Cleanup(func() { testBeforeOriginAcquire = nil })

	got := make(chan error, 1)
	go func() {
		_, err := reg.Get("probe")
		got <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("origin ownership did not start")
	}

	closed := make(chan struct{})
	go func() {
		reg.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Registry.Close returned before origin ownership completed")
	default:
	}

	close(release)
	if err := <-got; err != nil {
		t.Fatalf("Get: %v", err)
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Registry.Close did not return after origin ownership completed")
	}
	cfg, err := config.LoadFor("probe")
	if err != nil {
		t.Fatal(err)
	}
	if origin.IsInProcess(cfg) {
		t.Fatal("in-process mark remains after Registry.Close")
	}
}

func TestMountedStandaloneSkipsDoorAOwner(t *testing.T) {
	setupHome(t)
	cfg := seedStandaloneProfile(t, "probe")
	t.Cleanup(func() {
		_ = origin.Close()
		origin.ResetInProcess()
	})

	origin.SetInProcess(cfg, true)
	originHandler, err := origin.StandaloneHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	dbPath, err := config.DBPathFor("probe")
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	api := server.NewWorkspace(db, cfg, nil, "probe")
	api.BindOriginHandler(originHandler)
	t.Cleanup(func() { _ = api.Close() })

	reg := New()
	t.Cleanup(reg.Close)
	entry, err := reg.Get("probe")
	if err != nil {
		t.Fatal(err)
	}
	if entry.ownsOrigin {
		t.Fatal("mounted entry claimed the primary's origin")
	}
}
