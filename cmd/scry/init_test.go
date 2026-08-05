package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/scry/internal/config"
)

// myselfServer answers GET /rest/api/3/myself the way verifyCredential expects.
func myselfServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/myself" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") == "" {
			t.Errorf("myself: missing Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"displayName":"Test User"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// withClosedStdin proves init does not block on a prompt: stdin is a non-TTY
// pipe with no data.
func withClosedStdin(t *testing.T, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	saved := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = saved
		_ = r.Close()
	}()
	fn()
}

func TestInitFlagsNoPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	// Clear env sources so only flags count.
	t.Setenv("SCRY_SITE", "")
	t.Setenv("SCRY_EMAIL", "")
	t.Setenv("SCRY_TOKEN", "")
	t.Setenv("SCRY_PROJECTS", "")
	config.SetProfile("")

	srv := myselfServer(t)
	secret := "flag-only-token-not-in-argv-history"

	withClosedStdin(t, func() {
		out, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--projects", "abc, def",
				"--token-file", writeTokenFile(t, home, secret),
			})
		})
		if err != nil {
			t.Fatalf("init: %v", err)
		}
		if !strings.Contains(out, "verified as Test User") {
			t.Fatalf("human success line missing: %q", out)
		}
	})

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site != srv.URL || cfg.Email != "agent@example.com" || cfg.Token != secret {
		t.Fatalf("config fields: site=%q email=%q token set=%v", cfg.Site, cfg.Email, cfg.Token != "")
	}
	if len(cfg.Projects) != 2 || cfg.Projects[0] != "ABC" || cfg.Projects[1] != "DEF" {
		t.Fatalf("projects: %v", cfg.Projects)
	}
	// File must exist at the profile path with the saved values.
	p, _ := config.Path()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("config file not written: %v", err)
	}
}

func TestInitTokenFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Setenv("SCRY_SITE", "")
	t.Setenv("SCRY_EMAIL", "")
	t.Setenv("SCRY_PROJECTS", "")
	secret := "env-supplied-token-value-xyz"
	t.Setenv("SCRY_TOKEN", secret)
	config.SetProfile("")

	srv := myselfServer(t)

	withClosedStdin(t, func() {
		_, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "env@example.com",
				"--projects", "NMB",
			})
		})
		if err != nil {
			t.Fatalf("init: %v", err)
		}
	})

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != secret {
		t.Fatalf("token not taken from SCRY_TOKEN")
	}
}

func TestInitMissingNonTTY(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Setenv("SCRY_SITE", "")
	t.Setenv("SCRY_EMAIL", "")
	t.Setenv("SCRY_TOKEN", "")
	t.Setenv("SCRY_PROJECTS", "")
	config.SetProfile("")

	withClosedStdin(t, func() {
		_, err := capture(t, func() error {
			return cmdInit([]string{"--email", "only@example.com"})
		})
		if err == nil {
			t.Fatal("expected error for missing values")
		}
		msg := err.Error()
		// projects is optional; only site and token are still required.
		for _, name := range []string{"site", "token"} {
			if !strings.Contains(msg, name) {
				t.Errorf("error should name missing %q: %s", name, msg)
			}
		}
		first := msg
		if i := strings.Index(msg, "\n"); i >= 0 {
			first = msg[:i]
		}
		if strings.Contains(first, "projects") {
			t.Errorf("projects is optional but listed missing: %s", first)
		}
		// email was supplied; must not be listed as missing.
		if strings.Contains(first, "email") {
			t.Errorf("email was supplied but listed missing: %s", first)
		}
	})
}

// TestInitAllowsEmptyProjects: site/email/token alone is enough; blank projects
// means every project the account can see.
func TestInitAllowsEmptyProjects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Setenv("SCRY_SITE", "")
	t.Setenv("SCRY_EMAIL", "")
	t.Setenv("SCRY_TOKEN", "")
	t.Setenv("SCRY_PROJECTS", "")
	config.SetProfile("")

	srv := myselfServer(t)
	var out string
	withClosedStdin(t, func() {
		var err error
		out, err = capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "no-projects-token"),
			})
		})
		if err != nil {
			t.Fatalf("init without projects: %v", err)
		}
	})
	if !strings.Contains(out, "no project filter — syncing everything this account can see; narrow it later in Settings → Sync") {
		t.Fatalf("expected empty-projects guidance line: %q", out)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 0 {
		t.Fatalf("projects = %v, want empty", cfg.Projects)
	}
}

func TestInitRejectsTokenFlag(t *testing.T) {
	t.Setenv("SCRY_HOME", t.TempDir())
	config.SetProfile("")

	_, err := capture(t, func() error {
		return cmdInit([]string{
			"--site", "https://example.atlassian.net",
			"--email", "a@b.c",
			"--projects", "X",
			"--token", "should-not-be-accepted",
		})
	})
	if err == nil {
		t.Fatal("expected --token to be rejected")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--token is not accepted") {
		t.Fatalf("want friendly rejection, got: %s", msg)
	}
	if !strings.Contains(msg, "SCRY_TOKEN") || !strings.Contains(msg, "--token-file") || !strings.Contains(msg, "--token-stdin") {
		t.Fatalf("rejection should list safe alternatives: %s", msg)
	}
}

func TestInitJSONNoTokenLeak(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Setenv("SCRY_SITE", "")
	t.Setenv("SCRY_EMAIL", "")
	t.Setenv("SCRY_TOKEN", "")
	t.Setenv("SCRY_PROJECTS", "")
	config.SetProfile("work")
	t.Cleanup(func() { config.SetProfile("") })

	srv := myselfServer(t)
	secret := "json-must-never-echo-this-token-9f3a"

	var out string
	withClosedStdin(t, func() {
		var err error
		out, err = capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "json@example.com",
				"--projects", "ABC",
				"--token-file", writeTokenFile(t, home, secret),
				"--json",
			})
		})
		if err != nil {
			t.Fatalf("init --json: %v", err)
		}
	})

	if strings.Contains(out, secret) {
		t.Fatalf("token leaked into --json output: %q", out)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if doc["account"] != "Test User" {
		t.Fatalf("account: %v", doc["account"])
	}
	if doc["profile"] != "work" {
		t.Fatalf("profile: %v", doc["profile"])
	}
	if doc["site"] != srv.URL {
		t.Fatalf("site: %v", doc["site"])
	}
	if _, ok := doc["token"]; ok {
		t.Fatal("JSON must not have a token field")
	}
	// Whole serialization must not contain the secret (field or nested).
	raw, _ := json.Marshal(doc)
	if strings.Contains(string(raw), secret) {
		t.Fatal("token appears in re-serialized JSON")
	}
	if path, _ := doc["path"].(string); path == "" || !strings.Contains(path, "config.json") {
		t.Fatalf("path: %v", doc["path"])
	}
}

func writeTokenFile(t *testing.T, dir, secret string) string {
	t.Helper()
	p := filepath.Join(dir, "token.txt")
	// Leading/trailing whitespace proves TrimSpace on --token-file.
	if err := os.WriteFile(p, []byte("  "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestInitClassicReplacesTokenOnly is the expired-token re-run: saved config is
// complete, classic interactive mode re-prompts all four, and empty answers
// keep site/email/projects while a new token line replaces the secret.
func TestInitClassicReplacesTokenOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Setenv("SCRY_SITE", "")
	t.Setenv("SCRY_EMAIL", "")
	t.Setenv("SCRY_TOKEN", "")
	t.Setenv("SCRY_PROJECTS", "")
	config.SetProfile("")

	srv := myselfServer(t)
	oldToken := "expired-old-token"
	cfg := &config.Config{
		Site:     srv.URL,
		Email:    "keep@example.com",
		Token:    oldToken,
		Projects: []string{"ABC", "DEF"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	savedTerm := initIsTerminal
	savedIn := initStdin
	t.Cleanup(func() {
		initIsTerminal = savedTerm
		initStdin = savedIn
	})
	initIsTerminal = func() bool { return true }
	initStdin = strings.NewReader("\n\nNEWTOKEN\n\n")

	out, err := capture(t, func() error { return cmdInit(nil) })
	if err != nil {
		t.Fatalf("classic init: %v\nout=%q", err, out)
	}
	if strings.Contains(out, oldToken) {
		t.Fatalf("old token must never be printed: %q", out)
	}
	if strings.Contains(out, "NEWTOKEN") {
		t.Fatalf("new token must not appear on stdout: %q", out)
	}
	if !strings.Contains(out, "configured; enter to keep") {
		t.Fatalf("expected keep-hint on token prompt: %q", out)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Site != srv.URL || got.Email != "keep@example.com" {
		t.Fatalf("site/email changed: site=%q email=%q", got.Site, got.Email)
	}
	if got.Token != "NEWTOKEN" {
		t.Fatalf("token not replaced: %q", got.Token)
	}
	if len(got.Projects) != 2 || got.Projects[0] != "ABC" || got.Projects[1] != "DEF" {
		t.Fatalf("projects changed: %v", got.Projects)
	}
}

// failReader fails the test if anything tries to read (proves no prompt).
type failReader struct{ t *testing.T }

func (f failReader) Read(p []byte) (int, error) {
	f.t.Fatal("initStdin.Read called: non-classic path must not prompt")
	return 0, io.EOF
}

// TestInitAnyFlagDisablesPrompt even on a TTY: one supply flag opts fully into
// non-interactive fill, so missing values error without reading stdin.
func TestInitAnyFlagDisablesPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Setenv("SCRY_SITE", "")
	t.Setenv("SCRY_EMAIL", "")
	t.Setenv("SCRY_TOKEN", "")
	t.Setenv("SCRY_PROJECTS", "")
	config.SetProfile("")

	savedTerm := initIsTerminal
	savedIn := initStdin
	t.Cleanup(func() {
		initIsTerminal = savedTerm
		initStdin = savedIn
	})
	initIsTerminal = func() bool { return true }
	initStdin = failReader{t: t}

	_, err := capture(t, func() error {
		return cmdInit([]string{"--site", "https://example.atlassian.net"})
	})
	if err == nil {
		t.Fatal("expected missing error when only --site is supplied")
	}
	msg := err.Error()
	for _, name := range []string{"email", "token"} {
		if !strings.Contains(msg, name) {
			t.Errorf("error should name missing %q: %s", name, msg)
		}
	}
	first := msg
	if i := strings.Index(msg, "\n"); i >= 0 {
		first = msg[:i]
	}
	if strings.Contains(first, "site") {
		t.Errorf("site was supplied but listed missing: %s", first)
	}
	if strings.Contains(first, "projects") {
		t.Errorf("projects is optional but listed missing: %s", first)
	}
}
