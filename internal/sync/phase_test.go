package sync

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// TestWatchCallsPhasePerSource: Watch announces issues, then documents when
// Confluence is on, then idle — so a caller can report what the mirror is
// fetching. Without Confluence, documents never appears.
func TestWatchCallsPhasePerSource(t *testing.T) {
	t.Run("jira only", func(t *testing.T) {
		got := runWatchPhases(t, false)
		want := []string{PhaseIssues, PhaseIdle}
		if !phasesEqual(got, want) {
			t.Fatalf("phases = %q, want %q", got, want)
		}
	})
	t.Run("with confluence", func(t *testing.T) {
		got := runWatchPhases(t, true)
		want := []string{PhaseIssues, PhaseDocuments, PhaseIdle}
		if !phasesEqual(got, want) {
			t.Fatalf("phases = %q, want %q", got, want)
		}
	})
}

// runWatchPhases drives one Watch cycle and returns phases up to (and including)
// the first idle. A later idle from the return-path defer is ignored.
func runWatchPhases(t *testing.T, withConfluence bool) []string {
	t.Helper()
	site := newSite(t, "en")
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.SyncIntervalSec, cfg.ReconcileIntervalSec = 3600, 3600
	if withConfluence {
		// Presence alone is the on-switch. RunConfluence may fail against the
		// Jira fixture (no wiki routes) — Phase still fires before that call.
		cfg.Confluence = &config.ConfluenceConfig{}
	}

	var mu sync.Mutex
	var phases []string
	firstIdle := make(chan struct{})
	var idleOnce sync.Once

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{
			Full:   true,
			Client: client,
			Phase: func(source string) {
				mu.Lock()
				phases = append(phases, source)
				n := len(phases)
				// First idle closes the "one cycle done" signal.
				if source == PhaseIdle {
					idleOnce.Do(func() { close(firstIdle) })
				}
				mu.Unlock()
				// Cancel after idle so the loop does not sit on the ticker.
				if source == PhaseIdle && n >= 2 {
					cancel()
				}
			},
		})
	}()

	select {
	case <-firstIdle:
	case err := <-done:
		t.Fatalf("Watch ended before first idle: %v", err)
	}
	// Let the loop notice cancel (or finish returning).
	err := <-done
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	return phasesUntilFirstIdle(phases)
}

func phasesUntilFirstIdle(all []string) []string {
	out := make([]string, 0, len(all))
	for _, p := range all {
		out = append(out, p)
		if p == PhaseIdle {
			return out
		}
	}
	return out
}

func phasesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
