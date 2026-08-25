package workspace

import (
	"bytes"
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

func TestSTD3MountedStandaloneAdvertisesAndRoutes(t *testing.T) {
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
	if _, err := os.Stat(origin.AdvertisePath(cfg.Directory())); err != nil {
		t.Errorf("mounted standalone did not advertise its origin: %v", err)
	}

	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("mount owner Client: %v", err)
	}
	if _, err := c.CreateIssue(context.Background(), map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "STD-3 mount owner write",
		"issuetype": map[string]any{"name": "Task"},
	}); err != nil {
		t.Fatalf("mount owner CreateIssue: %v", err)
	}

	origin.ForgetLive()
	origin.ResetInProcess()

	c2, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("simulated CLI Client: %v", err)
	}
	if !origin.TransportIsServe(c2.HTTP.Transport) {
		t.Fatalf("simulated CLI transport %T, want serve passthrough", c2.HTTP.Transport)
	}
	if _, err := c2.CreateIssue(context.Background(), map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "STD-3 routed CLI write",
		"issuetype": map[string]any{"name": "Task"},
	}); err != nil {
		t.Fatalf("routed CLI CreateIssue: %v", err)
	}
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
	if entry.ownsOrigin || entry.stopOrigin != nil {
		t.Fatal("connected mount claimed an origin")
	}
	if _, err := os.Stat(origin.AdvertisePath(cfg.Directory())); !os.IsNotExist(err) {
		t.Fatalf("connected mount wrote advertise: %v", err)
	}
	if _, err := os.Stat(origin.PersistPath(cfg.Directory()) + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("connected mount took persist lock: %v", err)
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
	if _, err := os.Stat(origin.AdvertisePath(cfg.Directory())); !os.IsNotExist(err) {
		t.Fatalf("advertise remains after Registry.Close: %v", err)
	}
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
	cfg := seedStandaloneProfile(t, "probe")
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
	if _, err := os.Stat(origin.AdvertisePath(cfg.Directory())); !os.IsNotExist(err) {
		t.Fatalf("advertise remains after Registry.Close: %v", err)
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
	stop, err := origin.ServeOriginPassthrough(cfg.Directory(), api)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	before, err := os.ReadFile(origin.AdvertisePath(cfg.Directory()))
	if err != nil {
		t.Fatal(err)
	}

	reg := New()
	t.Cleanup(reg.Close)
	entry, err := reg.Get("probe")
	if err != nil {
		t.Fatal(err)
	}
	if entry.ownsOrigin || entry.stopOrigin != nil {
		t.Fatal("mounted entry claimed the primary's origin")
	}
	after, err := os.ReadFile(origin.AdvertisePath(cfg.Directory()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("mounted entry replaced the primary origin advertise")
	}
}
