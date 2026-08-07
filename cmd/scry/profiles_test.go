package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

// seedNamedProfile writes config.json (optional) and a small mirror under the
// current SCRY_HOME for the named profile ("" = default). Credentials are
// obviously fake so a leak is obvious in assertions.
func seedNamedProfile(t *testing.T, name string, cfg *config.Config, issues, pages int, stampSync bool) {
	t.Helper()
	dir, err := config.DirFor(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		loaded, err := config.LoadFor(name)
		if err != nil {
			t.Fatal(err)
		}
		loaded.Site = cfg.Site
		loaded.Email = cfg.Email
		loaded.Token = cfg.Token
		loaded.Projects = cfg.Projects
		if err := loaded.Save(); err != nil {
			t.Fatal(err)
		}
	}
	dbPath := filepath.Join(dir, "scry.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	site := "https://example.invalid"
	if cfg != nil && cfg.Site != "" {
		site = cfg.Site
	}
	if err := db.UpsertSource(store.Source{ID: "jira", Kind: "jira", BaseURL: site}); err != nil {
		t.Fatal(err)
	}
	if pages > 0 {
		if err := db.UpsertSource(store.Source{ID: "confluence", Kind: "confluence", BaseURL: site + "/wiki"}); err != nil {
			t.Fatal(err)
		}
	}

	if issues > 0 {
		recs := make([]store.IssueRecord, 0, issues)
		for i := 0; i < issues; i++ {
			key := fmt.Sprintf("TST-%d", i+1)
			recs = append(recs, store.IssueRecord{
				Item: store.Item{
					ID: "jira:" + key, SourceID: "jira", Kind: "issue", ExternalID: key, Key: key,
					Title:     "fixture " + key,
					CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
				},
				Issue: store.Issue{
					ProjectKey: "TST", Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			})
		}
		if _, err := db.UpsertIssues(store.Batch{
			Categories: map[string]string{"1": "new"},
			Records:    recs,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if pages > 0 {
		precs := make([]store.PageRecord, 0, pages)
		for i := 0; i < pages; i++ {
			ext := fmt.Sprintf("%d", 100+i)
			precs = append(precs, store.PageRecord{
				Item: store.Item{
					ID: "confluence:" + ext, SourceID: "confluence", Kind: "page",
					ExternalID: ext, Key: ext, Title: "page " + ext,
					CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
				},
				Page: store.Page{SpaceKey: "ENG", Version: 1, Status: "current"},
			})
		}
		if _, err := db.UpsertPages(precs); err != nil {
			t.Fatal(err)
		}
	}
	if stampSync {
		if err := db.RecordSync("jira", store.SyncResult{
			Watermark: "2026-08-01T00:00:00.000Z",
			FullSync:  true,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func profilesFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	// default: no credential, empty mirror (file created with 0 rows).
	seedNamedProfile(t, "", nil, 0, 0, false)

	// demo: credential + issues + pages
	seedNamedProfile(t, "demo", &config.Config{
		Site:  "https://demo.example.invalid/wiki/home",
		Email: "demo-user@example.invalid",
		Token: "test-token-demo-never-real",
	}, 3, 2, true)

	// work: credential + more issues, no pages
	seedNamedProfile(t, "work", &config.Config{
		Site:  "https://work.example.invalid",
		Email: "work-user@example.invalid",
		Token: "test-token-work-never-real",
	}, 5, 0, true)

	return home
}

func TestProfilesJSONThreeRowsAndActive(t *testing.T) {
	profilesFixture(t)

	out, err := capture(t, func() error { return cmdProfiles([]string{"--json"}) })
	if err != nil {
		t.Fatalf("profiles --json: %v\n%s", err, out)
	}

	var inv profileInventory
	if err := json.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if inv.Active != "default" {
		t.Errorf("active = %q, want default", inv.Active)
	}
	if len(inv.Profiles) != 3 {
		t.Fatalf("profiles = %d, want 3: %+v", len(inv.Profiles), inv.Profiles)
	}
	if inv.Profiles[0].Name != "default" {
		t.Errorf("first profile = %q, want default", inv.Profiles[0].Name)
	}

	// ② SCRY_PROFILE=work → active work, only that row active:true
	config.SetProfile("work")
	out, err = capture(t, func() error { return cmdProfiles([]string{"--json"}) })
	if err != nil {
		t.Fatalf("profiles --json under work: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if inv.Active != "work" {
		t.Errorf("active = %q, want work", inv.Active)
	}
	var activeCount int
	for _, p := range inv.Profiles {
		if p.Active {
			activeCount++
			if p.Name != "work" {
				t.Errorf("active row name = %q, want work", p.Name)
			}
		}
	}
	if activeCount != 1 {
		t.Errorf("active rows = %d, want 1", activeCount)
	}
}

func TestProfilesJSONNoSecretsAndSiteHost(t *testing.T) {
	profilesFixture(t)
	config.SetProfile("work")

	out, err := capture(t, func() error { return cmdProfiles([]string{"--json"}) })
	if err != nil {
		t.Fatalf("profiles --json: %v\n%s", err, out)
	}

	// ③ token and email must not appear in output bytes
	secrets := []string{
		"test-token-demo-never-real",
		"test-token-work-never-real",
		"demo-user@example.invalid",
		"work-user@example.invalid",
	}
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("output leaked secret %q:\n%s", s, out)
		}
	}
	if strings.Contains(out, "https://") {
		t.Errorf("output must not contain full site URLs:\n%s", out)
	}

	var inv profileInventory
	if err := json.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}

	byName := map[string]profileEntry{}
	for _, p := range inv.Profiles {
		byName[p.Name] = p
	}

	// ④ site_host is host only
	demo := byName["demo"]
	if demo.SiteHost != "demo.example.invalid" {
		t.Errorf("demo site_host = %q, want demo.example.invalid", demo.SiteHost)
	}
	if strings.Contains(demo.SiteHost, "/") || strings.Contains(demo.SiteHost, ":") {
		t.Errorf("site_host must be host only: %q", demo.SiteHost)
	}
	work := byName["work"]
	if work.SiteHost != "work.example.invalid" {
		t.Errorf("work site_host = %q", work.SiteHost)
	}
	if work.Issues != 5 || work.Documents != 0 {
		t.Errorf("work counts issues=%d docs=%d", work.Issues, work.Documents)
	}
	if work.LastSyncAt == nil {
		t.Error("work last_sync_at should be set")
	}
	if demo.Documents != 2 || demo.Issues != 3 {
		t.Errorf("demo counts issues=%d docs=%d", demo.Issues, demo.Documents)
	}

	// ⑤ no credential → configured false, empty site_host
	def := byName["default"]
	if def.Configured {
		t.Error("default should not be configured")
	}
	if def.SiteHost != "" {
		t.Errorf("default site_host = %q, want empty", def.SiteHost)
	}
}

func TestProfilesTextStarAndFooter(t *testing.T) {
	profilesFixture(t)
	config.SetProfile("demo")

	out, err := capture(t, func() error { return cmdProfiles(nil) })
	if err != nil {
		t.Fatalf("profiles: %v\n%s", err, out)
	}

	// ⑥ active row marked with *, footer two lines present
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var foundStar bool
	for _, line := range lines {
		if strings.HasPrefix(line, "*") && strings.Contains(line, "demo") {
			foundStar = true
		}
		if strings.HasPrefix(line, "*") && strings.Contains(line, "default") && !strings.Contains(line, "the profile") {
			t.Errorf("default should not be starred when demo is active:\n%s", line)
		}
	}
	if !foundStar {
		t.Fatalf("expected * on demo row:\n%s", out)
	}
	if !strings.Contains(out, "* = the profile this command ran against") {
		t.Fatalf("missing footer guidance:\n%s", out)
	}
	if !strings.Contains(out, "export SCRY_PROFILE=work") {
		t.Fatalf("missing SCRY_PROFILE hint:\n%s", out)
	}
	// blank line before footer
	if !strings.Contains(out, "\n\n* = the profile") {
		t.Fatalf("expected blank line before footer:\n%s", out)
	}
	// secrets stay out of text too
	if strings.Contains(out, "test-token") || strings.Contains(out, "@example.invalid") {
		t.Fatalf("text leaked secrets:\n%s", out)
	}
}

func TestProfilesUnreadableStillSucceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	// default empty + one broken named profile
	seedNamedProfile(t, "", nil, 0, 0, false)
	brokenDir := filepath.Join(home, "profiles", "broken")
	if err := os.MkdirAll(brokenDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, "config.json"), []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}

	// ⑦ command exits 0 and reports unreadable
	out, err := capture(t, func() error { return cmdProfiles(nil) })
	if err != nil {
		t.Fatalf("profiles with broken config must succeed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "broken") || !strings.Contains(out, "unreadable") {
		t.Fatalf("expected unreadable row:\n%s", out)
	}

	jout, err := capture(t, func() error { return cmdProfiles([]string{"--json"}) })
	if err != nil {
		t.Fatalf("profiles --json with broken config: %v\n%s", err, jout)
	}
	var inv profileInventory
	if err := json.Unmarshal([]byte(jout), &inv); err != nil {
		t.Fatalf("json: %v\n%s", err, jout)
	}
	var found bool
	for _, p := range inv.Profiles {
		if p.Name == "broken" {
			found = true
			if p.Error != "unreadable" {
				t.Errorf("broken error = %q, want unreadable", p.Error)
			}
		}
	}
	if !found {
		t.Fatalf("missing broken profile in JSON: %s", jout)
	}
}

func TestRelativeAge(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		iso  string
		want string
	}{
		{"", "never"},
		{now.Add(-30 * time.Second).Format(time.RFC3339), "just now"},
		{now.Add(-4 * time.Minute).Format(time.RFC3339), "4m ago"},
		{now.Add(-2 * time.Hour).Format(time.RFC3339), "2h ago"},
		{now.Add(-3 * 24 * time.Hour).Format(time.RFC3339), "3d ago"},
	}
	for _, tc := range cases {
		if got := relativeAge(tc.iso, now); got != tc.want {
			t.Errorf("relativeAge(%q) = %q, want %q", tc.iso, got, tc.want)
		}
	}
}

func TestFormatIntComma(t *testing.T) {
	cases := map[int]string{
		0:     "0",
		12:    "12",
		999:   "999",
		1000:  "1,000",
		6832:  "6,832",
		12345: "12,345",
	}
	for n, want := range cases {
		if got := formatIntComma(n); got != want {
			t.Errorf("formatIntComma(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestSiteHostOnly(t *testing.T) {
	cases := map[string]string{
		"":                                   "",
		"https://x.atlassian.net/foo":        "x.atlassian.net",
		"https://example.atlassian.net:443/": "example.atlassian.net",
		"https://work.example.invalid":       "work.example.invalid",
		"not a url :::":                      "",
	}
	for in, want := range cases {
		if got := siteHostOnly(in); got != want {
			t.Errorf("siteHostOnly(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProfilesDoesNotCreateMissingMirror(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCRY_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	// Named profile with config only — no scry.db.
	dir := filepath.Join(home, "profiles", "empty")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFor("empty")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Site = "https://empty.example.invalid"
	cfg.Email = "empty@example.invalid"
	cfg.Token = "test-token"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdProfiles(nil) })
	if err != nil {
		t.Fatalf("profiles: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "scry.db")); !os.IsNotExist(err) {
		t.Fatalf("profiles must not create scry.db; stat err=%v", err)
	}
	// Text should use em dash for missing mirror counts.
	if !strings.Contains(out, "\u2014") {
		t.Fatalf("expected em dash for missing counts:\n%s", out)
	}
}
