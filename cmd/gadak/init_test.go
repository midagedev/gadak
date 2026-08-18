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

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// clearCredentialEnv treats every GADAK_* / SCRY_* init source as unset.
// After D2, empty GADAK_* falls through to SCRY_*, so both prefixes must
// be cleared when a test wants flags-only / missing-env behavior.
func clearCredentialEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"GADAK_SITE", "GADAK_EMAIL", "GADAK_TOKEN", "GADAK_PROJECTS",
		"SCRY_SITE", "SCRY_EMAIL", "SCRY_TOKEN", "SCRY_PROJECTS",
	} {
		t.Setenv(k, "")
	}
}

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
		_, _ = w.Write([]byte(`{"displayName":"Test User","accountId":"acc-test"}`))
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

func TestInitStoresAccountID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")

	srv := myselfServer(t)
	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
			})
		}); err != nil {
			t.Fatalf("init: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccountID != "acc-test" {
		t.Fatalf("AccountID = %q, want acc-test", cfg.AccountID)
	}
	if cfg.TokenOwner != "Test User" {
		t.Fatalf("TokenOwner = %q, want Test User", cfg.TokenOwner)
	}
	if cfg.TokenVerifiedAt == "" {
		t.Fatal("TokenVerifiedAt is empty")
	}
	if cfg.TokenExpirySource != config.TokenExpirySourceAssumed || cfg.TokenExpiresAt == "" {
		t.Fatalf("verified init should assume expiry: source=%q at=%q", cfg.TokenExpirySource, cfg.TokenExpiresAt)
	}
}

func TestInitStoresUserExpiry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")

	srv := myselfServer(t)
	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "agent@example.com",
				"--token-file", writeTokenFile(t, home, "id-token"),
				"--token-expires", "2027-04-01",
			})
		}); err != nil {
			t.Fatalf("init: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenExpirySource != config.TokenExpirySourceUser || cfg.TokenExpiresAt != "2027-04-01T00:00:00.000Z" {
		t.Fatalf("user expiry: source=%q at=%q", cfg.TokenExpirySource, cfg.TokenExpiresAt)
	}
}

func TestInitOfflineSavesWithoutIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")

	// 418 is not auth and is not a retryable 429/503, so this stays fast.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"errorMessages":["unreachable"]}`))
	}))
	t.Cleanup(srv.Close)

	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "offline@example.com",
				"--token-file", writeTokenFile(t, home, "offline-token"),
			})
		}); err != nil {
			t.Fatalf("offline init must save credentials: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site != srv.URL || cfg.Email != "offline@example.com" || cfg.Token != "offline-token" {
		t.Fatalf("credentials not saved: site=%q email=%q token=%q", cfg.Site, cfg.Email, cfg.Token)
	}
	if cfg.AccountID != "" || cfg.TokenOwner != "" || cfg.TokenVerifiedAt != "" {
		t.Fatalf("offline init must not invent identity: id=%q owner=%q at=%q", cfg.AccountID, cfg.TokenOwner, cfg.TokenVerifiedAt)
	}
	if cfg.TokenExpiresAt != "" || cfg.TokenExpirySource != "" {
		t.Fatalf("offline init must not invent expiry: at=%q source=%q", cfg.TokenExpiresAt, cfg.TokenExpirySource)
	}
}

func TestInitAuthFailureDoesNotSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["auth"]}`))
	}))
	t.Cleanup(srv.Close)

	withClosedStdin(t, func() {
		_, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "bad@example.com",
				"--token-file", writeTokenFile(t, home, "bad-token"),
			})
		})
		if err == nil {
			t.Fatal("expected auth failure")
		}
		if !strings.Contains(err.Error(), "credential check failed") {
			t.Fatalf("want credential check failed, got %v", err)
		}
	})
	p, _ := config.Path()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("auth failure must not write config; stat=%v", err)
	}
}

func TestInitFlagsNoPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	// Clear env sources so only flags count.
	clearCredentialEnv(t)
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
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	secret := "env-supplied-token-value-xyz"
	t.Setenv("GADAK_TOKEN", secret)
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
		t.Fatalf("token not taken from GADAK_TOKEN")
	}
}

func TestInitMissingNonTTY(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
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
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
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
	if !strings.Contains(out, "no project filter — syncing everything this account can see; narrow it later in Settings → Sources") {
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
	t.Setenv("GADAK_HOME", t.TempDir())
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
	if !strings.Contains(msg, "GADAK_TOKEN") || !strings.Contains(msg, "--token-file") || !strings.Contains(msg, "--token-stdin") {
		t.Fatalf("rejection should list safe alternatives: %s", msg)
	}
}

func TestInitJSONNoTokenLeak(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
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

// runInitWithSpaces runs a non-interactive init with the given --spaces value
// (empty string = flag omitted) and returns the saved config.
func runInitWithSpaces(t *testing.T, spaces string) *config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	srv := myselfServer(t)
	args := []string{
		"--site", srv.URL,
		"--email", "spaces@example.com",
		"--projects", "ABC",
		"--token-file", writeTokenFile(t, home, "test-token"),
		"--json",
	}
	if spaces != "" {
		args = append(args, "--spaces", spaces)
	}
	var out string
	withClosedStdin(t, func() {
		var err error
		out, err = capture(t, func() error { return cmdInit(args) })
		if err != nil {
			t.Fatalf("init --spaces %q: %v", spaces, err)
		}
	})
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json: %v out=%q", err, out)
	}
	// Sanity: existing keys still present.
	if doc["profile"] == nil || doc["site"] == nil {
		t.Fatalf("json missing core keys: %v", doc)
	}
	// confluence key present in --json (off | all | list).
	if _, ok := doc["confluence"]; !ok {
		t.Fatalf("json missing confluence: %v", doc)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestInitSpacesList(t *testing.T) {
	cfg := runInitWithSpaces(t, "ENG,PROD")
	if cfg.Confluence == nil || len(cfg.Confluence.Spaces) != 2 ||
		cfg.Confluence.Spaces[0] != "ENG" || cfg.Confluence.Spaces[1] != "PROD" {
		t.Fatalf("want Spaces [ENG PROD], got %+v", cfg.Confluence)
	}
}

func TestInitSpacesAll(t *testing.T) {
	cfg := runInitWithSpaces(t, "all")
	if cfg.Confluence == nil || len(cfg.Confluence.Spaces) != 0 {
		t.Fatalf("want Confluence on with empty Spaces, got %+v", cfg.Confluence)
	}
	// Case-insensitive reserved word.
	cfg2 := runInitWithSpaces(t, "ALL")
	if cfg2.Confluence == nil || len(cfg2.Confluence.Spaces) != 0 {
		t.Fatalf("ALL: want empty Spaces, got %+v", cfg2.Confluence)
	}
}

func TestInitSpacesNone(t *testing.T) {
	// Start with Confluence on, then none should clear it.
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	// Seed an on state via first init with --spaces ENG.
	srv := myselfServer(t)
	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "spaces@example.com",
				"--token-file", writeTokenFile(t, home, "test-token"),
				"--spaces", "ENG",
			})
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	})
	seeded, _ := config.Load()
	if seeded.Confluence == nil {
		t.Fatal("seed should enable Confluence")
	}
	withClosedStdin(t, func() {
		out, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "spaces@example.com",
				"--token-file", writeTokenFile(t, home, "test-token"),
				"--spaces", "none",
				"--json",
			})
		})
		if err != nil {
			t.Fatalf("none: %v", err)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("json: %v", err)
		}
		if doc["confluence"] != "off" {
			t.Fatalf("json confluence: %v, want off", doc["confluence"])
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Confluence != nil {
		t.Fatalf("none should clear Confluence, got %+v", cfg.Confluence)
	}
}

func TestInitSpacesFlagAbsentLeavesConfluence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	srv := myselfServer(t)
	// Seed with Confluence on.
	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "spaces@example.com",
				"--token-file", writeTokenFile(t, home, "test-token"),
				"--spaces", "ENG,PROD",
			})
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	})
	// Re-init without --spaces must preserve Confluence.
	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "spaces@example.com",
				"--token-file", writeTokenFile(t, home, "test-token"),
				"--projects", "XYZ",
				"--json",
			})
		}); err != nil {
			t.Fatalf("re-init: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Confluence == nil || len(cfg.Confluence.Spaces) != 2 ||
		cfg.Confluence.Spaces[0] != "ENG" || cfg.Confluence.Spaces[1] != "PROD" {
		t.Fatalf("flag absent should leave Confluence: %+v", cfg.Confluence)
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0] != "XYZ" {
		t.Fatalf("projects should update: %v", cfg.Projects)
	}
}

func TestInitSpacesJSONShapes(t *testing.T) {
	// --spaces ENG,PROD → ["ENG","PROD"]
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	srv := myselfServer(t)

	check := func(spaces string, want any) {
		t.Helper()
		args := []string{
			"--site", srv.URL,
			"--email", "spaces@example.com",
			"--token-file", writeTokenFile(t, home, "test-token"),
			"--json",
		}
		if spaces != "" {
			args = append(args, "--spaces", spaces)
		}
		var out string
		withClosedStdin(t, func() {
			var err error
			out, err = capture(t, func() error { return cmdInit(args) })
			if err != nil {
				t.Fatalf("init: %v", err)
			}
		})
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("json: %v", err)
		}
		got := doc["confluence"]
		switch w := want.(type) {
		case string:
			if got != w {
				t.Fatalf("spaces=%q confluence=%v want %q", spaces, got, w)
			}
		case []string:
			arr, ok := got.([]any)
			if !ok || len(arr) != len(w) {
				t.Fatalf("spaces=%q confluence=%v want %v", spaces, got, w)
			}
			for i, k := range w {
				if arr[i] != k {
					t.Fatalf("spaces=%q confluence[%d]=%v want %s", spaces, i, arr[i], k)
				}
			}
		}
	}
	check("ENG,PROD", []string{"ENG", "PROD"})
	check("all", "all")
	check("none", "off")
}

// TestInitClassicReplacesTokenOnly is the expired-token re-run: saved config is
// complete, classic interactive mode re-prompts all four, and empty answers
// keep site/email/projects while a new token line replaces the secret.
func TestInitClassicReplacesTokenOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
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
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
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

func TestInitSpacesPreservesPersonalKeyCase(t *testing.T) {
	cfg := runInitWithSpaces(t, "~abcDef")
	if cfg.Confluence == nil || len(cfg.Confluence.Spaces) != 1 || cfg.Confluence.Spaces[0] != "~abcDef" {
		t.Fatalf("want Spaces [~abcDef] (case preserved), got %+v", cfg.Confluence)
	}
	cfg2 := runInitWithSpaces(t, "eng,~abcDef")
	if cfg2.Confluence == nil || len(cfg2.Confluence.Spaces) != 2 ||
		cfg2.Confluence.Spaces[0] != "eng" || cfg2.Confluence.Spaces[1] != "~abcDef" {
		t.Fatalf("want Spaces [eng ~abcDef] (no ToUpper), got %+v", cfg2.Confluence)
	}
}

func TestStatusUnknownProfileDoesNotCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "profiles", "demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "profiles", "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("nosuch")

	err := cmdStatus(nil)
	if err == nil {
		t.Fatal("status on a missing named profile must error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `profile "nosuch" not found`) {
		t.Errorf("error %q, want profile not found", msg)
	}
	if !strings.Contains(msg, `gadak init --profile "nosuch"`) {
		t.Errorf("error %q, want init hint", msg)
	}
	if !strings.Contains(msg, "available: demo, work") {
		t.Errorf("error %q, want available list", msg)
	}

	missing := filepath.Join(home, "profiles", "nosuch")
	if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
		t.Fatalf("must not create profile dir %s; stat=%v", missing, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(missing, "gadak.db")); !os.IsNotExist(statErr) {
		t.Fatalf("must not create mirror; stat=%v", statErr)
	}
}

func TestStatusExistingNamedProfileOK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	dir := filepath.Join(home, "profiles", "demo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("demo")

	if err := cmdStatus(nil); err != nil {
		t.Fatalf("existing named profile status: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gadak.db")); err != nil {
		t.Fatalf("status may create the mirror inside an existing profile dir: %v", err)
	}
}

func TestStatusDefaultProfileMayCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	if err := cmdStatus(nil); err != nil {
		t.Fatalf("default profile status: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "gadak.db")); err != nil {
		t.Fatalf("default profile may create the first-run mirror: %v", err)
	}
}

func TestParseSpaceKeysPreservesCase(t *testing.T) {
	got := parseSpaceKeys(" ~abcDef , ENG ")
	if len(got) != 2 || got[0] != "~abcDef" || got[1] != "ENG" {
		t.Fatalf("parseSpaceKeys = %v, want [~abcDef ENG]", got)
	}
	// Project keys still upper-case; that path must not change.
	proj := parseProjectKeys("~abcDef")
	if len(proj) != 1 || proj[0] != "~ABCDEF" {
		t.Fatalf("parseProjectKeys = %v, want [~ABCDEF]", proj)
	}
}

func TestOpenStoreAllowedWhenCreateFlagSet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() {
		config.SetProfile("")
		allowProfileCreate = false
	})
	config.SetProfile("onboard")
	allowProfileCreate = true
	db, err := openStore()
	if err != nil {
		t.Fatalf("serve-like create: %v", err)
	}
	_ = db.Close()
	if _, err := os.Stat(filepath.Join(home, "profiles", "onboard", "gadak.db")); err != nil {
		t.Fatalf("allowProfileCreate should mint the mirror: %v", err)
	}
}

func TestVersionIgnoresMissingProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() {
		config.SetProfile("")
		allowProfileCreate = false
	})
	config.SetProfile("typo")

	if err := checkProfileForCommand("version", nil); err != nil {
		t.Fatalf("version must not require an existing profile, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, "profiles", "typo")); !os.IsNotExist(statErr) {
		t.Fatalf("version must not create the profile dir; stat=%v", statErr)
	}
}

func TestHelpStatusIgnoresMissingProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() {
		config.SetProfile("")
		allowProfileCreate = false
	})
	config.SetProfile("typo")

	// gadak help status is rewritten in main to [status --help].
	if err := checkProfileForCommand("status", []string{"--help"}); err != nil {
		t.Fatalf("help status must not require an existing profile, got %v", err)
	}
	if err := checkProfileForCommand("status", []string{"-h"}); err != nil {
		t.Fatalf("status -h must not require an existing profile, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, "profiles", "typo")); !os.IsNotExist(statErr) {
		t.Fatalf("help must not create the profile dir; stat=%v", statErr)
	}
}

func TestProfileCreateWhitelist(t *testing.T) {
	if !profileCreateOK["init"] || !profileCreateOK["serve"] {
		t.Fatal("init and serve must be allowed to create a named profile")
	}
	if !profileIndependent["version"] {
		t.Fatal("version must be profile-independent (no require, no create)")
	}
	if profileCreateOK["version"] {
		t.Fatal("version must not be on the create whitelist")
	}
	for _, name := range []string{"status", "sql", "issue", "search", "sync"} {
		if profileCreateOK[name] {
			t.Errorf("%s must not be on the create whitelist", name)
		}
	}
}

func TestSQLUnknownProfileDoesNotCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("nosuch")

	err := cmdSQL([]string{"select 1"})
	if err == nil {
		t.Fatal("sql on a missing named profile must error")
	}
	if !strings.Contains(err.Error(), `profile "nosuch" not found`) {
		t.Errorf("error %q, want profile not found", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, "profiles", "nosuch")); !os.IsNotExist(statErr) {
		t.Fatalf("must not create profile dir; stat=%v", statErr)
	}
}

// TestInitConvertingEmptyStandaloneDropsSeededSpace guards the half of "a
// workspace is bound to one origin" that the --replace-standalone flag does not
// cover. An *empty* standalone workspace is deliberately allowed to convert
// without that flag (refuseStandaloneReplace returns nil at n==0), so a guard
// keyed on the flag leaves the seeded issuetap space (LOC) in the config of a
// now-connected workspace — and the wiki pass then asks a real Atlassian site
// for a space that only ever existed in the in-process origin.
func TestInitConvertingEmptyStandaloneDropsSeededSpace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	// origin keeps live in-process sessions in a process-global map, and a
	// standalone init opens one. Left behind, its debounced snapshot flush
	// targets a TempDir that has already been removed, and the *next* test in
	// this package to call origin.Close() is the one that fails.
	t.Cleanup(func() { _ = origin.Close() })

	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{"--standalone", "--json"})
		}); err != nil {
			t.Fatalf("standalone init: %v", err)
		}
	})
	seeded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if seeded.Confluence == nil || len(seeded.Confluence.Spaces) == 0 {
		t.Fatalf("standalone init should seed a wiki space, got %+v", seeded.Confluence)
	}

	// No --replace-standalone: the workspace holds no locally originated
	// issues, so this conversion is allowed through.
	srv := myselfServer(t)
	withClosedStdin(t, func() {
		if _, err := capture(t, func() error {
			return cmdInit([]string{
				"--site", srv.URL,
				"--email", "convert@example.com",
				"--token-file", writeTokenFile(t, home, "test-token"),
				"--json",
			})
		}); err != nil {
			t.Fatalf("convert: %v", err)
		}
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IsStandalone() {
		t.Fatalf("converted workspace still reports standalone: kind=%q", cfg.Kind)
	}
	if cfg.Confluence != nil {
		t.Fatalf("seeded standalone space survived the conversion: %+v", cfg.Confluence)
	}
}
