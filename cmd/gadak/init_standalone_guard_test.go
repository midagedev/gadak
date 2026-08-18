package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// seedStandaloneWithIssue is a standalone workspace that already holds a
// locally originated issue (write-through into the mirror). GADAK_HOME is
// the temp dir; the caller must not point at a real profile.
func seedStandaloneWithIssue(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error {
		return cmdInit([]string{"--standalone", "--json"})
	}); err != nil {
		t.Fatalf("init --standalone: %v", err)
	}
	if _, err := capture(t, func() error {
		return cmdCreate([]string{"local only issue"})
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// CLI commands are one process each; Close flushes the debounce window
	// the same way main() does, so persist is on disk for the next init.
	if err := origin.Close(); err != nil {
		t.Fatalf("origin.Close: %v", err)
	}
	return home
}

// TestInitConnectedRefusesStandaloneWithData is the GDK-238 recurrence
// layer: a connected init over a standalone workspace that holds locally
// originated issues must not succeed silently.
func TestInitConnectedRefusesStandaloneWithData(t *testing.T) {
	home := seedStandaloneWithIssue(t)
	srv := myselfServer(t)
	withClosedStdin(t, func() {
		_, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
			})
		})
		if err == nil {
			t.Fatal("connected init over a standalone workspace with local issues must not succeed silently")
		}
		if !strings.Contains(err.Error(), "only here") {
			t.Fatalf("refusal must say local issues exist only here, got: %v", err)
		}
		persist := origin.PersistPath(home)
		if !strings.Contains(err.Error(), persist) {
			t.Fatalf("refusal must name persist path %q, got: %v", persist, err)
		}
		if !strings.Contains(err.Error(), "gadak --profile") {
			t.Fatalf("refusal must name gadak --profile, got: %v", err)
		}
		if !strings.Contains(err.Error(), "gadak profiles") {
			t.Fatalf("refusal must name gadak profiles, got: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsStandalone() {
		t.Fatalf("kind after refused init = %q, want standalone", cfg.WorkspaceKind())
	}
	if cfg.Site != "" || cfg.Token != "" {
		t.Fatalf("refused init must not write a site credential: site=%q token_set=%t", cfg.Site, cfg.Token != "")
	}
}

func TestInitConnectedJSONRefusesStandaloneWithData(t *testing.T) {
	home := seedStandaloneWithIssue(t)
	srv := myselfServer(t)
	var out string
	withClosedStdin(t, func() {
		var err error
		out, err = capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
				"--json",
			})
		})
		if err == nil {
			t.Fatal("connected init --json over standalone data must not succeed silently")
		}
	})
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout must be JSON, got %q: %v", out, err)
	}
	if doc["error"] != "standalone_data_present" {
		t.Fatalf("error = %v, want standalone_data_present", doc["error"])
	}
	persist := origin.PersistPath(home)
	if doc["persist"] != persist {
		t.Fatalf("persist = %v, want %s", doc["persist"], persist)
	}
	n, ok := doc["issues"].(float64)
	if !ok || n < 1 {
		t.Fatalf("issues = %v, want >= 1", doc["issues"])
	}
}

func TestInitConnectedEmptyStandaloneSucceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error {
		return cmdInit([]string{"--standalone", "--json"})
	}); err != nil {
		t.Fatalf("init --standalone: %v", err)
	}
	srv := myselfServer(t)
	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
			})
		}); err != nil {
			t.Fatalf("empty standalone → connected must succeed: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IsStandalone() {
		t.Fatal("empty standalone connect must clear Kind")
	}
	if cfg.Site != srv.URL {
		t.Fatalf("site = %q, want %s", cfg.Site, srv.URL)
	}
}

func TestInitReplaceStandaloneOptIn(t *testing.T) {
	home := seedStandaloneWithIssue(t)
	srv := myselfServer(t)
	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
				"--replace-standalone",
			})
		}); err != nil {
			t.Fatalf("--replace-standalone must proceed: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IsStandalone() {
		t.Fatal("--replace-standalone must clear Kind")
	}
	// Persist file is the origin; the flag does not delete it.
	if _, err := os.Stat(origin.PersistPath(home)); err != nil {
		t.Fatalf("persist must survive --replace-standalone: %v", err)
	}
}

func TestInitConnectedRefusesPersistOnlyData(t *testing.T) {
	home := seedStandaloneWithIssue(t)
	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		_ = os.Remove(p)
	}
	if _, err := os.Stat(origin.PersistPath(home)); err != nil {
		t.Fatalf("persist must exist for this case: %v", err)
	}
	srv := myselfServer(t)
	withClosedStdin(t, func() {
		_, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
			})
		})
		if err == nil {
			t.Fatal("persist-only standalone data must still refuse a connected init")
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsStandalone() {
		t.Fatalf("kind after persist-only refuse = %q", cfg.WorkspaceKind())
	}
}

func TestInitReplaceStandaloneRejectedWithStandalone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	_, err := capture(t, func() error {
		return cmdInit([]string{"--standalone", "--replace-standalone"})
	})
	if err == nil {
		t.Fatal("expected error combining --standalone and --replace-standalone")
	}
	if !strings.Contains(err.Error(), "--replace-standalone") {
		t.Fatalf("error %v", err)
	}
}
