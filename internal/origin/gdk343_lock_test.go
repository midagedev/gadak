package origin

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// TestGDK343SecondProcessSeesFirstWrite: two processes (simulated with
// ForgetLive) open the same persist. WAL shares the write (GDK-936);
// the persist lock that used to refuse a second process is gone.
func TestGDK343SecondProcessSeesFirstWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = Close()
		config.SetProfile("")
	})
	cfg := &config.Config{Kind: config.KindStandalone}

	a, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	key, err := a.CreateIssue(context.Background(), map[string]any{
		"project":   map[string]any{"key": DefaultProjectKey},
		"summary":   "gdk-343 first owner",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue on first owner: %v", err)
	}

	ForgetLive()

	b, err := Client(cfg)
	if err != nil {
		t.Fatalf("second process Client: %v", err)
	}
	if !searchKey(t, b, key) {
		t.Fatalf("second process cannot see %s — WAL did not share the write", key)
	}
	if _, err := Wiki(cfg); err != nil {
		t.Fatalf("second Wiki: %v", err)
	}
}

// TestGDK343ConstructAfterClose: Close drops the session, so the
// process is allowed to come back (origin.Close contract).
func TestGDK343ConstructAfterClose(t *testing.T) {
	persist := filepath.Join(t.TempDir(), filepath.FromSlash(PersistRel))
	a, err := constructStandalone(persist, nil, config.ResolvedActor{}, "en")
	if err != nil {
		t.Fatal(err)
	}
	closeSession(a)
	b, err := constructStandalone(persist, nil, config.ResolvedActor{}, "en")
	if err != nil {
		t.Fatalf("construct after close: %v", err)
	}
	closeSession(b)
}
