package origin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/pairing"
)

func TestConnectedMatchesJiraNew(t *testing.T) {
	got := Connected("https://example.atlassian.net/", "a@b.c", "tok")
	want := jira.New("https://example.atlassian.net/", "a@b.c", "tok")
	if got.BaseURL() != want.BaseURL() {
		t.Fatalf("BaseURL %q != jira.New %q", got.BaseURL(), want.BaseURL())
	}
}

// TestCreatesVersionsByName is GDK-678: the mint-by-name capability lives on
// the origin client, not on workspace.kind. Connected Cloud is false;
// issuetap (localOrigin Client via transportJira) is true.
func TestCreatesVersionsByName(t *testing.T) {
	connected := Connected("https://example.atlassian.net/", "a@b.c", "tok")
	if CreatesVersionsByName(connected) {
		t.Fatal("connected Jira CreatesVersionsByName = true, want false")
	}

	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = Close()
		config.SetProfile("")
	})

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}

	c, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !CreatesVersionsByName(c) {
		t.Fatal("local-origin issuetap CreatesVersionsByName = false, want true")
	}
}

func TestClientNilAndConnectedMissingCreds(t *testing.T) {
	if _, err := Client(nil); err == nil {
		t.Fatal("Client(nil) succeeded")
	}
	if _, err := Client(&config.Config{}); err == nil {
		t.Fatal("Client(empty connected) succeeded")
	}
	if _, err := Client(&config.Config{Site: "https://x.atlassian.net", Email: "a@b.c"}); err == nil {
		t.Fatal("Client(no token) succeeded")
	}
}

func TestPairedStatusReadsRemoteOrigin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	rem, err := PairedStatus(cfg)
	if err != nil || rem != nil {
		t.Fatalf("empty workspace: %+v (%v)", rem, err)
	}

	if err := pairing.SaveRemote(cfg.Directory(), pairing.Remote{
		Endpoint: "https://home.ts.net:8443", Token: "pair-token", Label: "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	rem, err = PairedStatus(cfg)
	if err != nil || rem == nil || rem.Label != "laptop" || rem.Endpoint != "https://home.ts.net:8443" {
		t.Fatalf("paired: %+v (%v)", rem, err)
	}

	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	rem, err = PairedStatus(cfg)
	if err != nil || rem != nil {
		t.Fatalf("local-origin home routing is not a paired origin: %+v (%v)", rem, err)
	}
}

func TestDescribeConnectedAndLocalOrigin(t *testing.T) {
	kind, src := Describe(nil)
	if kind != config.KindConnected || src != "jira" {
		t.Fatalf("nil: %q %q", kind, src)
	}
	kind, src = Describe(&config.Config{Site: "https://x.atlassian.net"})
	if kind != config.KindConnected || src != "jira" {
		t.Fatalf("connected: %q %q", kind, src)
	}

	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// Reload so Directory() is set the way production Load works.
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	kind, src = Describe(cfg)
	if kind != config.KindLocalOrigin {
		t.Fatalf("kind %q", kind)
	}
	want := PersistPath(home)
	if src != want {
		t.Fatalf("origin %q, want %q", src, want)
	}
}

func TestLocalOriginClientCreateAndPersist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = Close()
		config.SetProfile("")
	})

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}

	c, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL() != "" {
		t.Fatalf("local-origin BaseURL = %q, want empty (no fake https origin)", c.BaseURL())
	}

	ctx := context.Background()
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": DefaultProjectKey},
		"summary":   "origin persist probe",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if !strings.HasPrefix(key, DefaultProjectKey+"-") {
		t.Fatalf("key %q", key)
	}

	if err := Close(); err != nil {
		t.Fatal(err)
	}
	persist := PersistPath(home)
	if _, err := os.Stat(persist); err != nil {
		t.Fatalf("persist file missing at %s: %v", persist, err)
	}

	c2, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	err = c2.Search(ctx, `key = "`+key+`"`, []string{"summary"}, false, func(issues []jira.Issue) error {
		for _, iss := range issues {
			if iss.Key == key {
				found = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Search after restart: %v", err)
	}
	if !found {
		t.Fatalf("issue %s missing after Close+Client", key)
	}

	// Same persist path must reuse the live session (byte-identical client).
	c3, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c2 != c3 {
		t.Fatal("Client should reuse the live local-origin session")
	}

	// Confirm the persist path is under the workspace directory, nowhere else.
	if !strings.HasPrefix(persist, home+string(filepath.Separator)) {
		t.Fatalf("persist %q not under %q", persist, home)
	}
}

// TestLocalOriginCreateMetaCarriesSubtaskAndHierarchyLevel is GDK-329:
// issuetap already sends both fields; CreateMeta must keep them.
func TestLocalOriginCreateMetaCarriesSubtaskAndHierarchyLevel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = Close()
		config.SetProfile("")
	})

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}

	c, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := c.CreateMeta(context.Background(), []string{DefaultProjectKey})
	if err != nil {
		t.Fatalf("CreateMeta: %v", err)
	}
	if len(projects) == 0 {
		t.Fatal("local-origin createmeta returned no projects")
	}
	if raw, err := json.Marshal(projects[0].IssueTypes); err == nil {
		t.Logf("local-origin CreateMeta issuetypes JSON: %s", raw)
	}
	var sawSubtask, sawEpic, sawStandard bool
	for _, p := range projects {
		for _, it := range p.IssueTypes {
			switch {
			case it.Subtask && it.HierarchyLevel == -1:
				sawSubtask = true
			case it.HierarchyLevel == 1:
				sawEpic = true
			case !it.Subtask && it.HierarchyLevel == 0:
				sawStandard = true
			}
		}
	}
	if !sawSubtask || !sawEpic || !sawStandard {
		t.Fatalf("local-origin createmeta types missing hierarchy: subtask=%t epic=%t standard=%t projects=%+v",
			sawSubtask, sawEpic, sawStandard, projects)
	}
}

func TestCloseIdempotent(t *testing.T) {
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
}
