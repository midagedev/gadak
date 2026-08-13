package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func TestDoctorNoMirrorSucceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor with no mirror: %v\n%s", err, out)
	}
	if !strings.Contains(out, "not found") {
		t.Fatalf("expected not-found mirror line, got:\n%s", out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "# gadak doctor") {
		t.Fatalf("missing safety banner:\n%s", out)
	}
	// Must not create a database just by diagnosing.
	if _, err := os.Stat(filepath.Join(home, "gadak.db")); !os.IsNotExist(err) {
		t.Fatalf("doctor must not create gadak.db when missing; stat err=%v", err)
	}
}

func TestDoctorDemoDBCounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	src := filepath.Join("..", "..", "examples", "demo.db")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read demo.db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "gadak.db"), raw, 0o600); err != nil {
		t.Fatalf("copy demo.db: %v", err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "mirror:") || !strings.Contains(out, "present") {
		t.Fatalf("expected present mirror:\n%s", out)
	}
	// Sanity: open the same copy and compare schema + row counts.
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open copy: %v", err)
	}
	defer db.Close()
	wantSchema := db.SchemaVersion()
	if !strings.Contains(out, "schema_version:") || !strings.Contains(out, strconv.Itoa(wantSchema)) {
		t.Fatalf("expected schema_version %d in:\n%s", wantSchema, out)
	}
	if strings.Contains(out, "issues:                n/a") {
		t.Fatalf("issues should be counted:\n%s", out)
	}
	n, err := db.TableCount(context.Background(), "issues")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n < 1 {
		t.Fatalf("demo.db has no issues?")
	}
	if !strings.Contains(out, strconv.Itoa(n)) {
		t.Fatalf("output missing issues count %d:\n%s", n, out)
	}
}

func TestDoctorRedaction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	// Seed a tiny mirror with project keys that must never appear in output.
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"3": "inprogress"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "LEAKY-42", Title: "do not leak this summary",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "SECRET", IssueType: "Bug", IssueTypeID: "1",
				Status: "Open", StatusID: "3", StatusCategory: "inprogress",
			},
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Plant a last_error that embeds a host, key, and URL — classifier only.
	if err := db.RecordSync(context.Background(), "jira", store.SyncResult{
		Err: errString("GET /rest/api/3/search: jira: 403: You do not have access to LEAKY-42 on https://example.atlassian.net"),
	}); err != nil {
		t.Fatalf("record sync: %v", err)
	}
	_ = db.Close()

	const (
		fakeSite  = "https://example.atlassian.net"
		fakeEmail = "alice.secret@example.invalid"
		fakeToken = "NOT-A-REAL-TOKEN-fixture-value-0123456789"
	)
	cfg := &config.Config{
		Site:     fakeSite,
		Email:    fakeEmail,
		Token:    fakeToken,
		Projects: []string{"SECRET", "LEAKME"},
		Fields: []config.FieldSpec{{
			Alias: "internal_risk",
			Label: "Internal Risk Score",
			IDs:   []string{"customfield_99100"},
			Role:  "facet",
		}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	jsonOut, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, jsonOut)
	}

	forbidden := []string{
		fakeSite,
		"example.atlassian.net",
		"example",
		fakeEmail,
		"alice.secret",
		fakeToken,
		"NOT-A-REAL-TOKEN",
		"LEAKY-42",
		"SECRET",
		"LEAKME",
		"do not leak this summary",
		"Internal Risk Score",
		"internal_risk",
		"customfield_99100",
		"You do not have access",
	}
	for _, bad := range forbidden {
		if strings.Contains(out, bad) {
			t.Errorf("human output leaked %q", bad)
		}
		if strings.Contains(jsonOut, bad) {
			t.Errorf("json output leaked %q", bad)
		}
	}
	// Positive redaction markers that should appear.
	for _, want := range []string{
		"credential:            present",
		"email:                 configured",
		"<redacted>.atlassian.net",
		"http 403 (auth)",
		"custom_fields:         1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
	// Username from the temp path must not appear if home-relative.
	if home, err := os.UserHomeDir(); err == nil {
		base := filepath.Base(home)
		// Only flag when the home base is a plausible username length and
		// appears as a path segment (not as part of unrelated words).
		if base != "" && base != "tmp" && strings.Contains(out, "/"+base+"/") {
			t.Errorf("username path segment %q appeared in output:\n%s", base, out)
		}
	}
}

func TestDoctorJSONMatchesHumanFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	// Empty home is enough — doctor succeeds without a mirror.
	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("human: %v", err)
	}
	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.GadakVersion == "" || rep.GoVersion == "" || rep.OS == "" || rep.Arch == "" {
		t.Fatalf("missing runtime fields: %+v", rep)
	}
	if rep.Profile != "default" {
		t.Fatalf("profile = %q, want default", rep.Profile)
	}
	if rep.Mirror.Status != "not_found" {
		t.Fatalf("mirror.status = %q, want not_found", rep.Mirror.Status)
	}
	// Human form carries the same facts (as text).
	for _, want := range []string{
		"gadak_version:",
		"go_version:",
		"os:",
		"profile:",
		"mirror_path:",
		"mirror:",
		"schema_version:",
		"migrations:",
		"credential:",
		"site:",
		"email:",
		"api_usage.day:",
		"api_usage.requests:",
		"api_usage.throttled:",
		"api_usage.retries:",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("human missing %q", want)
		}
	}
	if rep.APIUsage.Day == "" {
		t.Fatal("json api_usage.day empty")
	}
	if rep.Credential != "absent" || rep.Site != "none" || rep.Email != "none" {
		t.Fatalf("empty config: credential=%q site=%q email=%q", rep.Credential, rep.Site, rep.Email)
	}
}

func TestClassifyLastError(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "none"},
		{"GET /rest/api/3/search: jira: 403: denied", "http 403 (auth)"},
		{"jira: 401: unauthorized", "http 401 (auth)"},
		{"jira: 429: Rate limited", "http 429 (throttled)"},
		{"upstream status 503", "http 503 (server)"},
		{"HTTP 404 not found", "http 404 (not_found)"},
		{"context deadline exceeded", "timeout"},
		{"dial tcp: connection refused", "network"},
		{"jira: credential rejected", "auth"},
		{"something opaque went wrong", "error"},
	}
	for _, tc := range cases {
		if got := classifyLastError(tc.in); got != tc.want {
			t.Errorf("classifyLastError(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRedactSite(t *testing.T) {
	cases := map[string]string{
		"":                               "none",
		"https://example.atlassian.net":  "<redacted>.atlassian.net",
		"https://example.atlassian.net/": "<redacted>.atlassian.net",
		"https://jira.example.com":       "configured (cloud)",
		"http://localhost:8080":          "configured (cloud)",
	}
	for in, want := range cases {
		if got := redactSite(in); got != want {
			t.Errorf("redactSite(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTildeHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	got := tildeHome(filepath.Join(home, ".gadak", "gadak.db"))
	if got != "~/.gadak/gadak.db" && got != `~\.gadak\gadak.db` {
		// On Unix we expect forward slashes from filepath under home.
		if !strings.HasPrefix(got, "~") || strings.Contains(got, home) {
			t.Fatalf("tildeHome = %q, still contains home or missing ~", got)
		}
	}
	if strings.Contains(got, home) {
		t.Fatalf("tildeHome leaked home dir: %q", got)
	}
}

// errString plants a last_error without pulling in fmt at every call site.
type errString string

func (e errString) Error() string { return string(e) }
