package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/serveaddr"
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
		if !strings.Contains(err.Error(), "gadak --workspace") {
			t.Fatalf("refusal must name gadak --workspace, got: %v", err)
		}
		if !strings.Contains(err.Error(), "gadak workspaces") {
			t.Fatalf("refusal must name gadak workspaces, got: %v", err)
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

// occupyLiveServe records a live UI-serve in serveaddr and answers the
// identity probe so RefuseIfOpen sees this profile as open.
func occupyLiveServe(t *testing.T, profile string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	mux := http.NewServeMux()
	mux.HandleFunc(origin.ProbePath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Gadak", "1")
		w.Header().Set("X-Gadak-Profile", profile)
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	dir, err := serveaddr.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := serveaddr.Write(dir, ln.Addr().String(), profile); err != nil {
		t.Fatal(err)
	}
}

// TestInitReplaceStandaloneRefusesWhenServeLive is GDK-415: converting
// while a serve has the workspace open must not proceed.
func TestInitReplaceStandaloneRefusesWhenServeLive(t *testing.T) {
	home := seedStandaloneWithIssue(t)
	occupyLiveServe(t, config.Profile())
	srv := myselfServer(t)
	withClosedStdin(t, func() {
		_, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
				"--replace-standalone",
			})
		})
		if err == nil {
			t.Fatal("replace-standalone must refuse when another process has the workspace open")
		}
		if !strings.Contains(err.Error(), "has this workspace open") {
			t.Fatalf("want workspace-open refusal, got: %v", err)
		}
		if strings.Contains(err.Error(), "Jira") {
			t.Fatalf("must not assume Jira: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsStandalone() {
		t.Fatal("refused conversion must leave the workspace standalone")
	}
}

// writeForeignOpenMark makes the persist's open marker name another live
// process. PID 1 always exists and is owned by root, so this also crosses
// the liveness check's EPERM path — "alive but not signallable by me".
func writeForeignOpenMark(t *testing.T, home string) string {
	t.Helper()
	p := origin.PersistPath(home) + ".open"
	if err := os.WriteFile(p, []byte(`{"pid":1,"startedAt":"2026-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestInitReplaceStandaloneRefusesWhenAppHoldsIt is GDK-971, and it is the
// half of GDK-415 that serveaddr discovery cannot cover: Gadak.app opens no
// port, so nothing on the network says it is holding the workspace. Before
// GDK-936 the persist lock answered this by accident; the open marker
// answers it on purpose.
//
// FAIL-first: on the tree that deleted the lock and had no marker yet, this
// test fails — RefuseIfOpen returns nil and the conversion proceeds under a
// live holder.
func TestInitReplaceStandaloneRefusesWhenAppHoldsIt(t *testing.T) {
	home := seedStandaloneWithIssue(t)
	writeForeignOpenMark(t, home)
	srv := myselfServer(t)
	withClosedStdin(t, func() {
		_, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
				"--replace-standalone",
			})
		})
		if err == nil {
			t.Fatal("replace-standalone must refuse while another process holds the workspace")
		}
		if !strings.Contains(err.Error(), "has this workspace open") {
			t.Fatalf("want workspace-open refusal, got: %v", err)
		}
		// A holder with no port must not render as "port )".
		if strings.Contains(err.Error(), "port )") {
			t.Fatalf("portless holder rendered a blank port: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsStandalone() {
		t.Fatal("refused conversion must leave the workspace standalone")
	}
}

// A marker left by a process that died must never block conversion: the
// kernel released the old flock on exit, and nothing releases this file.
// Forever-refusing after one crash would be worse than not checking.
func TestInitReplaceStandaloneIgnoresDeadHolder(t *testing.T) {
	home := seedStandaloneWithIssue(t)
	p := origin.PersistPath(home) + ".open"
	// A PID that cannot be running: the kernel's own maximum plus one.
	if err := os.WriteFile(p, []byte(`{"pid":4194305,"startedAt":"2020-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := originbind.RefuseIfOpen(cfg); err != nil {
		t.Fatalf("a dead holder must not refuse conversion: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a dead holder's marker should be removed on sight")
	}
}

func TestInitReplaceStandaloneIgnoresStaleAdvertise(t *testing.T) {
	home := seedStandaloneWithIssue(t)
	if err := os.WriteFile(filepath.Join(home, "serve-origin.json"),
		[]byte(`{"addr":"127.0.0.1:1","pid":1,"startedAt":"2020-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := originbind.RefuseIfOpen(cfg); err != nil {
		t.Fatalf("leftover serve-origin.json must not refuse conversion: %v", err)
	}
}

// TestInitStandaloneProjectsSeedsOrigin is GDK-390: --projects keys must
// land in the origin persist fixture, so create --project IDEA works.
//
// FAIL-first: defaultStandaloneFixture only names STD.
func TestInitStandaloneProjectsSeedsOrigin(t *testing.T) {
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
		return cmdInit([]string{"--standalone", "--json", "--projects", "IDEA,FOO"})
	}); err != nil {
		t.Fatalf("init --standalone --projects: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("origin.Client: %v", err)
	}
	list, _, err := c.Projects(context.Background(), 50)
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	have := map[string]bool{}
	for _, p := range list {
		have[p.Key] = true
	}
	for _, key := range []string{"IDEA", "FOO"} {
		if !have[key] {
			t.Fatalf("origin missing project %s (have %v)", key, have)
		}
	}
	out, err := capture(t, func() error {
		return cmdCreate([]string{"--project", "IDEA", "--type", "Task", "from seeded project"})
	})
	if err != nil {
		t.Fatalf("create --project IDEA: %v\n%s", err, out)
	}
	if !strings.Contains(out, "IDEA-") {
		t.Fatalf("created key should be IDEA-N, got %q", out)
	}
}

// TestCreateStandaloneUnknownProjectDoesNotAssumeCredential is GDK-390's
// create-error wording: standalone has no site credential.
//
// FAIL-first: MetaFor always says "this credential cannot create issues".
func TestCreateStandaloneUnknownProjectDoesNotAssumeCredential(t *testing.T) {
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
	_, err := capture(t, func() error {
		return cmdCreate([]string{"--project", "NOSUCH", "--type", "Task", "nowhere"})
	})
	if err == nil {
		t.Fatal("create in a missing standalone project must fail")
	}
	if strings.Contains(err.Error(), "credential") {
		t.Fatalf("standalone must not assume a credential: %v", err)
	}
	if !strings.Contains(err.Error(), "does not exist in this workspace") {
		t.Fatalf("want project-does-not-exist wording, got: %v", err)
	}
}
