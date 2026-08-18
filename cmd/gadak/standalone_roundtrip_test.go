package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// TestStandaloneCreateSyncSQL is the plumbing round-trip: init --standalone,
// create an issue through the origin (issuetap), sync into the SQLite
// mirror, read it back with gadak sql. No network, no real site.
func TestStandaloneCreateSyncSQL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	var initJSON struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
	}
	out, err := capture(t, func() error {
		return cmdInit([]string{"--standalone", "--json"})
	})
	if err != nil {
		t.Fatalf("init --standalone: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &initJSON); err != nil {
		t.Fatalf("init json: %v\n%s", err, out)
	}
	if initJSON.Kind != config.KindStandalone {
		t.Fatalf("kind %q", initJSON.Kind)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsStandalone() {
		t.Fatal("loaded config is not standalone")
	}
	if cfg.Site != "" || cfg.Email != "" || cfg.Token != "" {
		t.Fatalf("standalone leaked credential fields: site=%q email=%q token_set=%t", cfg.Site, cfg.Email, cfg.Token != "")
	}

	// The seeded project offers several issue types, so without a recorded
	// default `create <summary>` would demand --type on every call — the one
	// thing a standalone workspace exists to make cheap. init resolves it
	// from the origin's createmeta and writes it here.
	if cfg.DefaultProject == "" || cfg.DefaultIssueTypeID == "" {
		t.Fatalf("init did not record create defaults: project=%q typeID=%q", cfg.DefaultProject, cfg.DefaultIssueTypeID)
	}

	// Summary only: no --project, no --type.
	created, err := capture(t, func() error {
		return cmdCreate([]string{"standalone roundtrip"})
	})
	if err != nil {
		t.Fatalf("create: %v\n%s", err, created)
	}
	first := strings.TrimSpace(strings.Split(created, "\n")[0])
	key := strings.Split(first, "\t")[0]
	if !strings.HasPrefix(key, origin.DefaultProjectKey+"-") {
		t.Fatalf("created key %q from %q", key, created)
	}

	if _, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync: %v", err)
	}

	sqlOut, err := capture(t, func() error {
		return cmdSQL([]string{"--no-header", "select key, summary from issues_full where key = '" + key + "'"})
	})
	if err != nil {
		t.Fatalf("sql: %v\n%s", err, sqlOut)
	}
	if !strings.Contains(sqlOut, key) || !strings.Contains(sqlOut, "standalone roundtrip") {
		t.Fatalf("mirror missing created issue:\n%s", sqlOut)
	}

	// Origin snapshot is debounced; Close flushes it into the workspace dir.
	if err := origin.Close(); err != nil {
		t.Fatalf("origin.Close: %v", err)
	}
	persist := origin.PersistPath(home)
	if _, err := os.Stat(persist); err != nil {
		t.Fatalf("persist %s: %v", persist, err)
	}

	// doctor names the kind and the origin path (tilde-abbreviated).
	doc, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, doc)
	}
	if !strings.Contains(doc, "workspace_kind:") || !strings.Contains(doc, "standalone") {
		t.Fatalf("doctor missing standalone kind:\n%s", doc)
	}
	if !strings.Contains(doc, "origin:") {
		t.Fatalf("doctor missing origin:\n%s", doc)
	}
	if strings.Contains(doc, "http://") || strings.Contains(doc, "https://") {
		t.Fatalf("doctor leaked a URL:\n%s", doc)
	}

	// Existing default config.json next to the persist file, no extra profile.
	if _, err := os.Stat(filepath.Join(home, "config.json")); err != nil {
		t.Fatal(err)
	}
}

func TestInitStandaloneRejectsCredentialFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	_, err := capture(t, func() error {
		return cmdInit([]string{"--standalone", "--site", "https://example.atlassian.net"})
	})
	if err == nil {
		t.Fatal("expected error combining --standalone and --site")
	}
	if !strings.Contains(err.Error(), "--standalone") {
		t.Fatalf("error %v", err)
	}
}

func TestInitWithoutStandaloneUnchanged(t *testing.T) {
	// Connected init still requires site/email/token when stdin is not a TTY.
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	withClosedStdin(t, func() {
		_, err := capture(t, func() error { return cmdInit(nil) })
		if err == nil {
			t.Fatal("bare init without TTY must still refuse")
		}
		if !strings.Contains(err.Error(), "missing:") {
			t.Fatalf("want missing: listing, got %v", err)
		}
	})
}
