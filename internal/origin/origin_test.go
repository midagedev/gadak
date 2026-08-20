package origin

import (
	"context"
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

	cfg.Kind = config.KindStandalone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	rem, err = PairedStatus(cfg)
	if err != nil || rem != nil {
		t.Fatalf("standalone home routing is not a paired origin: %+v (%v)", rem, err)
	}
}

func TestDescribeConnectedAndStandalone(t *testing.T) {
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
	cfg.Kind = config.KindStandalone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// Reload so Directory() is set the way production Load works.
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	kind, src = Describe(cfg)
	if kind != config.KindStandalone {
		t.Fatalf("kind %q", kind)
	}
	want := PersistPath(home)
	if src != want {
		t.Fatalf("origin %q, want %q", src, want)
	}
}

func TestStandaloneClientCreateAndPersist(t *testing.T) {
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
	cfg.Kind = config.KindStandalone
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
		t.Fatalf("standalone BaseURL = %q, want empty (no fake https origin)", c.BaseURL())
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
		t.Fatal("Client should reuse the live standalone session")
	}

	// Confirm the persist path is under the workspace directory, nowhere else.
	if !strings.HasPrefix(persist, home+string(filepath.Separator)) {
		t.Fatalf("persist %q not under %q", persist, home)
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
