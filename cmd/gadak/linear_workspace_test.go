package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

const linearTestKey = "linear-test-key-not-a-real-secret"

// linearOnlyHome is a connected workspace whose only origin credential is
// a Linear API key — the shape the lead measured on 2026-08-22.
func linearOnlyHome(t *testing.T) *config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	cfg := &config.Config{Linear: &config.LinearConfig{APIKey: linearTestKey}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func plantLinearSync(t *testing.T, watermark string) {
	t.Helper()
	path, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "linear", Kind: "linear", BaseURL: "https://linear.app"}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync(ctx, "linear", store.SyncResult{Watermark: watermark}); err != nil {
		t.Fatal(err)
	}
}

// GDK-654 FAIL-first: a Linear-only workspace used to print ErrNotConfigured
// on status (and JSON watermark/sync_count stayed the empty Jira row).
func TestStatusLinearOnlyIsConfiguredAndShowsLinearWatermark(t *testing.T) {
	linearOnlyHome(t)
	const wm = "2026-08-22T12:00:00.000Z"
	plantLinearSync(t, wm)

	stdout, stderr, err := captureBoth(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	if strings.Contains(stderr, config.ErrNotConfigured.Error()) {
		t.Fatalf("Linear-only status printed the unconfigured sentence:\nstderr=%q\nstdout=%s", stderr, stdout)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("status --json stdout: %v\n%s", err, stdout)
	}
	if got, _ := doc["watermark"].(string); got != wm {
		t.Fatalf("watermark = %v, want Linear %q; body %s", doc["watermark"], wm, stdout)
	}
	if n, _ := doc["sync_count"].(float64); n < 1 {
		t.Fatalf("sync_count = %v, want Linear's successful run; body %s", doc["sync_count"], stdout)
	}
}

// GDK-655 FAIL-first: create on a Linear-only workspace used to send the
// user to `gadak init` instead of naming the real cause (or routing).
func TestCreateLinearOnlyDoesNotAskForInit(t *testing.T) {
	linearOnlyHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	origin.LinearEndpoint = srv.URL
	t.Cleanup(func() { origin.LinearEndpoint = "" })
	_, err := capture(t, func() error {
		return cmdCreate([]string{"a summary", "--project", "FIX"})
	})
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), config.ErrNotConfigured.Error()) {
		t.Fatalf("Linear-only create sent the user to gadak init: %v", err)
	}
}

// GDK-655 FAIL-first: page create used to quote origin.errNeedCredential
// ("site, email and token are required") on a workspace that has no wiki.
func TestPageCreateLinearOnlyNamesMissingWiki(t *testing.T) {
	linearOnlyHome(t)
	_, err := capture(t, func() error {
		return cmdPageCreate([]string{"--space", "ENG", "--title", "notes", "-m", "draft"})
	})
	if err == nil {
		t.Fatal("page create succeeded on a Linear-only workspace; Linear has no wiki")
	}
	msg := err.Error()
	if strings.Contains(msg, config.ErrNotConfigured.Error()) {
		t.Fatalf("page create sent the user to gadak init: %v", err)
	}
	if strings.Contains(msg, "site, email and token are required") {
		t.Fatalf("page create still premises a Jira site: %v", err)
	}
	if !strings.Contains(strings.ToLower(msg), "wiki") {
		t.Fatalf("page create must say the wiki origin is missing, got %v", err)
	}
}

func TestCreateLinearOnlyRoutesToLinear(t *testing.T) {
	linearOnlyHome(t)
	var creates int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		q := string(raw)
		switch {
		case strings.Contains(q, "query Teams"):
			_, _ = w.Write(linearCLITestdata(t, "teams.json"))
		case strings.Contains(q, "mutation IssueCreate"):
			creates++
			_, _ = w.Write(linearCLITestdata(t, "issue_create.json"))
		case strings.Contains(q, "query Issue("):
			_, _ = w.Write(linearCLITestdata(t, "issues_page1.json"))
		default:
			_, _ = w.Write([]byte(`{"data":{}}`))
		}
	}))
	t.Cleanup(srv.Close)
	origin.LinearEndpoint = srv.URL
	t.Cleanup(func() { origin.LinearEndpoint = "" })

	out, err := capture(t, func() error {
		return cmdCreate([]string{"a summary", "--project", "FIX"})
	})
	if err != nil && strings.Contains(err.Error(), config.ErrNotConfigured.Error()) {
		t.Fatalf("Linear create still sent the user to init: %v", err)
	}
	if creates != 1 {
		t.Fatalf("IssueCreate mutations = %d, want 1; err=%v out=%q", creates, err, out)
	}
}

func linearCLITestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "internal", "linear", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLinearOnlyHomeDoesNotReadDevConfig(t *testing.T) {
	linearOnlyHome(t)
	if os.Getenv("GADAK_HOME") == "" {
		t.Fatal("GADAK_HOME must be isolated")
	}
	if strings.Contains(os.Getenv("GADAK_HOME"), "gadak-dev") {
		t.Fatal("test home must not be ~/.config/gadak-dev")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("GADAK_HOME"), "config.json")); err != nil {
		t.Fatal(err)
	}
}
