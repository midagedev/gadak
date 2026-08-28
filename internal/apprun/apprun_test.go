package apprun

import (
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
	if _, err := os.Stat(filepath.Join(rt.Cfg.Directory(), "serve-origin.json")); !os.IsNotExist(err) {
		t.Fatal("DeferStandalone Open must not write serve-origin.json")
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

func TestOpenStandaloneDoesNotWriteAdvertise(t *testing.T) {
	testHome(t)
	saveStandalone(t)
	rt, err := Open(Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	if _, err := os.Stat(filepath.Join(rt.Cfg.Directory(), "serve-origin.json")); !os.IsNotExist(err) {
		t.Fatal("standalone Open must not write serve-origin.json")
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
