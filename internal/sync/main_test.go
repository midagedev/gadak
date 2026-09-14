package sync

import (
	"os"
	"path/filepath"
	"testing"
)

// A full sync can persist discovered configuration. Keep every test's default
// config path away from the user's live ~/.gadak even when a case forgets to
// call t.Setenv, or the caller exported GADAK_HOME to the live workspace.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "gadak-sync-test-home-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("GADAK_HOME", root); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}

func TestSuiteGadakHomeIsIsolated(t *testing.T) {
	root := os.Getenv("GADAK_HOME")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if root == "" || filepath.Clean(root) == filepath.Join(home, ".gadak") {
		t.Fatalf("sync tests must never write the live gadak home: GADAK_HOME=%q", root)
	}
}
