package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// builtInTrackerHome is `init --local` plus two issues, with the embedded
// origin still open (the WAL sidecars on disk are the "serve is running"
// shape backup must copy through).
func builtInTrackerHome(t *testing.T) {
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
	if out, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatalf("init --local: %v\n%s", err, out)
	}
	for _, s := range []string{"backup one", "backup two"} {
		if out, err := capture(t, func() error { return cmdCreate([]string{s}) }); err != nil {
			t.Fatalf("create: %v\n%s", err, out)
		}
	}
}

func countPersistIssues(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM issues").Scan(&n); err != nil {
		t.Fatalf("count issues in %s: %v", path, err)
	}
	return n
}

func TestBackupCopiesBuiltInTrackerPersist(t *testing.T) {
	builtInTrackerHome(t)
	dir := t.TempDir()

	out, err := capture(t, func() error { return cmdBackup([]string{"--to", dir, "--json"}) })
	if err != nil {
		t.Fatalf("backup: %v\n%s", err, out)
	}
	var rep struct {
		Path   string `json:"path"`
		Issues int    `json:"issues"`
		Bytes  int64  `json:"bytes"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("backup --json: %v\n%s", err, out)
	}
	if filepath.Dir(rep.Path) != dir || !strings.HasSuffix(rep.Path, ".db") {
		t.Fatalf("backup landed at %q, want a .db inside %q", rep.Path, dir)
	}
	if rep.Bytes <= 0 {
		t.Fatalf("bytes %d", rep.Bytes)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	_, src := origin.Describe(cfg)
	want := countPersistIssues(t, src)
	if want != 2 {
		t.Fatalf("fixture has %d issues, want 2", want)
	}
	if got := countPersistIssues(t, rep.Path); got != want || rep.Issues != want {
		t.Fatalf("copy has %d issues, report says %d, source has %d", got, rep.Issues, want)
	}
	// The copy is a single file: VACUUM INTO leaves no WAL sidecar to lose.
	if _, err := os.Stat(rep.Path + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("backup left a -wal sidecar (err=%v)", err)
	}

	// Text mode: one line, the path, nothing else.
	file := filepath.Join(dir, "explicit.db")
	out, err = capture(t, func() error { return cmdBackup([]string{"--to", file}) })
	if err != nil {
		t.Fatalf("backup --to file: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != file {
		t.Fatalf("stdout %q, want %q", out, file)
	}
	// An existing target is refused, never overwritten.
	if _, err := capture(t, func() error { return cmdBackup([]string{"--to", file}) }); err == nil {
		t.Fatal("backup onto an existing file must fail")
	}
}

func TestBackupRefusesJiraWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	cfg := &config.Config{Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "tok"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error { return cmdBackup([]string{"--to", t.TempDir()}) })
	if err == nil || !strings.Contains(err.Error(), "built-in tracker") {
		t.Fatalf("want built-in-tracker refusal, got %v", err)
	}
}

func TestBackupRefusesPairedWorkspace(t *testing.T) {
	seedPairedProfile(t)
	_, err := capture(t, func() error { return cmdBackup([]string{"--to", t.TempDir()}) })
	if err == nil || !strings.Contains(err.Error(), "home machine") || !strings.Contains(err.Error(), `"laptop"`) {
		t.Fatalf("want run-on-home-machine refusal naming the pairing label, got %v", err)
	}
	if strings.Contains(err.Error(), "home.ts.net") {
		t.Fatalf("endpoint leaked into refusal: %v", err)
	}
}
