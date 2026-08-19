package origin

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// TestGDK343SecondProcessCannotEmbed: two processes (simulated with
// ForgetLive — origin.go's existing technique) open the same persist.
// Before the flock fix both embedded and the last Close won (silent
// last-writer-wins loss). After the fix the second Client must not embed:
// with no advertise to route to, it fails with ErrWorkspaceBusy.
//
// FAIL-first (2026-08-20, pre-fix): the second Client succeeded, both
// sessions wrote STD-1/STD-2 into their own graphs, and
// SessionsConstructed rose by 2 — the double-embed hazard was live.
func TestGDK343SecondProcessCannotEmbed(t *testing.T) {
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
	if _, err := a.CreateIssue(context.Background(), map[string]any{
		"project":   map[string]any{"key": DefaultProjectKey},
		"summary":   "gdk-343 first owner",
		"issuetype": map[string]any{"name": "Task"},
	}); err != nil {
		t.Fatalf("CreateIssue on first owner: %v", err)
	}

	// Simulate a second process: the first session still holds the graph
	// (and the lock), but this "process" no longer finds it in live.
	ForgetLive()

	_, err = Client(cfg)
	if err == nil {
		t.Fatal("second process embedded the same persist — double-owner hazard (GDK-343)")
	}
	if !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("second Client error = %v, want ErrWorkspaceBusy", err)
	}
	if _, err := Wiki(cfg); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("second Wiki error = %v, want ErrWorkspaceBusy", err)
	}
}

// TestGDK343LockReleasedOnClose: Close releases the persist lock, so the
// process is allowed to come back (origin.Close contract).
func TestGDK343LockReleasedOnClose(t *testing.T) {
	persist := filepath.Join(t.TempDir(), "origin", "issuetap.yaml")
	a, err := constructStandalone(persist)
	if err != nil {
		t.Fatal(err)
	}
	closeSession(a)
	b, err := constructStandalone(persist)
	if err != nil {
		t.Fatalf("construct after close: %v", err)
	}
	closeSession(b)
}
