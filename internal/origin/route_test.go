package origin_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
)

func localOriginHome(t *testing.T) (*config.Config, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		origin.ResetInProcess()
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
	return cfg, home
}

func searchKey(t *testing.T, c *jira.Client, key string) bool {
	t.Helper()
	found := false
	err := c.Search(context.Background(), `key = "`+key+`"`, []string{"summary"}, false, func(issues []jira.Issue) error {
		for _, iss := range issues {
			if iss.Key == key {
				found = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	return found
}

func writeStaleAdvertise(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "serve-origin.json")
	doc := []byte(`{"addr":"127.0.0.1:1","pid":1,"startedAt":"2020-01-01T00:00:00Z"}`)
	if err := os.WriteFile(p, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestStaleServeOriginJSONIsIgnored is the GDK-936 back-compat pin: a
// leftover serve-origin.json from a previous version is not an error and
// does not route Client onto HTTP.
func TestStaleServeOriginJSONIsIgnored(t *testing.T) {
	cfg, home := localOriginHome(t)
	p := writeStaleAdvertise(t, home)

	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("Client with leftover serve-origin.json: %v", err)
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("Transport type %T, want embedded (stale advertise must be ignored)", c.HTTP.Transport)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("leftover file should remain (ignored, not deleted): %v", err)
	}
}

func TestClientSecondSessionSeesWrite(t *testing.T) {
	cfg, _ := localOriginHome(t)
	a, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	key, err := a.CreateIssue(context.Background(), map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "wal share",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	origin.ForgetLive()
	origin.ResetInProcess()

	b, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("second Client: %v", err)
	}
	if !origin.TransportIsEmbedded(b.HTTP.Transport) {
		t.Fatalf("Transport type %T, want embedded", b.HTTP.Transport)
	}
	if !searchKey(t, b, key) {
		t.Fatalf("second session cannot see %s", key)
	}
}

func TestMountedWorkspaceIgnoresStaleAdvertise(t *testing.T) {
	_, _ = localOriginHome(t)

	jt, err := config.LoadFor("jt")
	if err != nil {
		t.Fatal(err)
	}
	jt.Kind = config.KindLocalOrigin
	if err := jt.Save(); err != nil {
		t.Fatal(err)
	}
	jt, err = config.LoadFor("jt")
	if err != nil {
		t.Fatal(err)
	}
	writeStaleAdvertise(t, jt.Directory())

	before := origin.SessionsConstructed()
	c, err := origin.Client(jt)
	if err != nil {
		t.Fatal(err)
	}
	if origin.TransportIsServe(c.HTTP.Transport) {
		t.Fatal("mounted workspace routed through leftover advertise")
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("Transport type %T, want the mount's own embedded session", c.HTTP.Transport)
	}
	if got := origin.SessionsConstructed(); got != before+1 {
		t.Fatalf("SessionsConstructed %d → %d, want +1 (jt's own graph)", before, got)
	}
}

func TestForgetLiveThenClientEmbedsAcrossGC(t *testing.T) {
	cfg, _ := localOriginHome(t)
	if _, err := origin.Client(cfg); err != nil {
		t.Fatal(err)
	}
	origin.ForgetLive()
	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("after GC: %v", err)
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("Transport type %T, want embedded", c.HTTP.Transport)
	}
}

func TestSetInProcessReusesLiveSession(t *testing.T) {
	cfg, _ := localOriginHome(t)
	if _, err := origin.Client(cfg); err != nil {
		t.Fatal(err)
	}
	origin.SetInProcess(cfg, true)
	t.Cleanup(func() { origin.SetInProcess(cfg, false) })

	before := origin.SessionsConstructed()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("Transport type %T, want embedded", c.HTTP.Transport)
	}
	if got := origin.SessionsConstructed(); got != before {
		t.Fatalf("in-process Client constructed a new session: %d → %d", before, got)
	}
}

func TestOwnerStatusEmbeddedIgnoresStaleFile(t *testing.T) {
	cfg, home := localOriginHome(t)
	if got := origin.OwnerStatus(cfg); got != "embedded (no live serve)" {
		t.Fatalf("no file: %q", got)
	}
	writeStaleAdvertise(t, home)
	if got := origin.OwnerStatus(cfg); got != "embedded (no live serve)" {
		t.Fatalf("stale advertise: %q", got)
	}
}

func TestConnectedOwnerStatusEmpty(t *testing.T) {
	if got := origin.OwnerStatus(&config.Config{}); got != "" {
		t.Fatalf("connected OwnerStatus = %q", got)
	}
	if got := origin.OwnerStatus(nil); got != "" {
		t.Fatalf("nil OwnerStatus = %q", got)
	}
}
