package apprun

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

func testHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		origin.ResetInProcess()
		config.SetProfile("")
	})
}

func saveStandalone(t *testing.T) *config.Config {
	t.Helper()
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
	return cfg
}

func recordSteps(t *testing.T) *[]string {
	t.Helper()
	var steps []string
	testStep = func(s string) { steps = append(steps, s) }
	t.Cleanup(func() { testStep = nil })
	return &steps
}

func TestOpenSequenceConnected(t *testing.T) {
	testHome(t)
	steps := recordSteps(t)
	rt, err := Open(Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	want := []string{"config", "store", "handler", "registry"}
	if got := *steps; !equalSteps(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	if rt.Cfg == nil || rt.DB == nil || rt.API == nil || rt.Reg == nil {
		t.Fatal("runtime missing Cfg/DB/API/Reg")
	}
}

func TestOpenSequenceDeferStandaloneSkipsPersist(t *testing.T) {
	testHome(t)
	saveStandalone(t)
	before := origin.SessionsConstructed()
	steps := recordSteps(t)
	rt, err := Open(Options{DeferStandalone: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	if origin.SessionsConstructed() != before {
		t.Fatal("GDK-658: DeferStandalone Open must not take persist (desktop SingleInstance has not run)")
	}
	for _, s := range *steps {
		if s == "standalone-persist" {
			t.Fatalf("DeferStandalone Open recorded persist: %v", *steps)
		}
	}
	want := []string{"config", "store", "handler", "registry"}
	if got := *steps; !equalSteps(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	if _, err := os.Stat(origin.AdvertisePath(rt.Cfg.Directory())); !os.IsNotExist(err) {
		t.Fatal("DeferStandalone Open must not advertise")
	}
}

func TestOpenSequenceStandaloneAcquiresPersistBeforeStore(t *testing.T) {
	testHome(t)
	saveStandalone(t)
	before := origin.SessionsConstructed()
	steps := recordSteps(t)
	rt, err := Open(Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	if origin.SessionsConstructed() == before {
		t.Fatal("standalone Open without DeferStandalone must take persist")
	}
	want := []string{"config", "standalone-persist", "store", "handler", "registry"}
	if got := *steps; !equalSteps(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
}

func TestAfterConfigRunsBeforePersist(t *testing.T) {
	// GDK-468: live-serve handoff (AfterConfig) must run before persist.
	testHome(t)
	saveStandalone(t)
	before := origin.SessionsConstructed()
	stop := errors.New("after-config stop")
	_, err := Open(Options{
		AfterConfig: func(*config.Config) error {
			if origin.SessionsConstructed() != before {
				t.Error("persist taken before AfterConfig returned")
			}
			return stop
		},
	})
	if !errors.Is(err, stop) {
		t.Fatalf("err = %v, want after-config stop", err)
	}
	if origin.SessionsConstructed() != before {
		t.Fatal("GDK-468: persist must not be taken when AfterConfig fails")
	}
}

func TestVersionStampIsFirst(t *testing.T) {
	testHome(t)
	steps := recordSteps(t)
	rt, err := Open(Options{Version: "0.0.0-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	if len(*steps) == 0 || (*steps)[0] != "version" {
		t.Fatalf("version must be first, got %v", *steps)
	}
}

func TestPublishAdvertiseWritesAndRemoves(t *testing.T) {
	testHome(t)
	cfg := saveStandalone(t)
	unpublish, err := PublishAdvertise(cfg, "127.0.0.1:7998")
	if err != nil {
		t.Fatal(err)
	}
	p := origin.AdvertisePath(cfg.Directory())
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("advertise missing: %v", err)
	}
	unpublish()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("advertise still present")
	}
}

func TestPublishAdvertiseSkipsConnected(t *testing.T) {
	testHome(t)
	cfg := &config.Config{Kind: config.KindConnected}
	unpublish, err := PublishAdvertise(cfg, "127.0.0.1:7998")
	if err != nil {
		t.Fatal(err)
	}
	unpublish()
	home := os.Getenv("GADAK_HOME")
	if _, err := os.Stat(filepath.Join(home, origin.AdvertiseRel)); !os.IsNotExist(err) {
		t.Fatal("connected workspace must not write serve-origin.json")
	}
}

func equalSteps(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
