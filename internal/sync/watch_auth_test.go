package sync

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/jira"
)

// TestWatchStopsOnErrAuth: a revoked token must end the loop, record last_error
// (status / sync_health read that column), log once, and not schedule another
// request. A later one-shot Run with a working credential must still succeed.
func TestWatchStopsOnErrAuth(t *testing.T) {
	site := newSite(t, "en")
	site.authStatus = http.StatusUnauthorized
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.SyncIntervalSec, cfg.ReconcileIntervalSec = 1, 3600

	var mu sync.Mutex
	var logs []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{
			Client: client,
			Log: func(line string) {
				mu.Lock()
				logs = append(logs, line)
				mu.Unlock()
			},
		})
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not stop after ErrAuth")
	}
	if !errors.Is(err, jira.ErrAuth) {
		t.Fatalf("Watch returned %v, want ErrAuth", err)
	}

	site.mu.Lock()
	hitsAfterStop := site.hits
	site.mu.Unlock()
	if hitsAfterStop == 0 {
		t.Fatal("Watch returned ErrAuth without requesting Jira")
	}

	time.Sleep(1500 * time.Millisecond)
	site.mu.Lock()
	hitsLater := site.hits
	site.mu.Unlock()
	if hitsLater != hitsAfterStop {
		t.Fatalf("Watch requested Jira %d more times after ErrAuth (hits %d → %d)",
			hitsLater-hitsAfterStop, hitsAfterStop, hitsLater)
	}

	st, err := db.SyncState(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError == nil || !strings.Contains(*st.LastError, "credential rejected") {
		t.Fatalf("last_error = %v, want the ErrAuth text status/sync_health already surface", st.LastError)
	}

	mu.Lock()
	got := append([]string{}, logs...)
	mu.Unlock()
	var failed int
	for _, line := range got {
		if strings.Contains(line, "sync failed") {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("sync-failed log lines = %d, want 1 (once, not per tick); logs = %q", failed, got)
	}
	if !strings.Contains(strings.Join(got, "\n"), "credential rejected") {
		t.Fatalf("log did not carry the existing ErrAuth text: %q", got)
	}

	// Manual `gadak sync` after a new token: one-shot Run must resume and clear
	// last_error. Stopping Watch is process-lifetime; it must not poison the row.
	site.mu.Lock()
	site.authStatus = 0
	site.mu.Unlock()
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatalf("one-shot Run after new token: %v", err)
	}
	st, err = db.SyncState(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError != nil {
		t.Fatalf("last_error = %q after a successful Run — must not be permanent", *st.LastError)
	}
}

// TestWatchRetriesTransportError: a 500 is not ErrAuth — the loop must keep
// requesting on the next tick instead of exiting.
func TestWatchRetriesTransportError(t *testing.T) {
	site := newSite(t, "en")
	site.authStatus = http.StatusInternalServerError
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.SyncIntervalSec, cfg.ReconcileIntervalSec = 1, 3600

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{Client: client})
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		site.mu.Lock()
		n := site.hits
		site.mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Watch hits = %d after a 500, want >= 2 (retry on the next tick)", n)
		}
		select {
		case err := <-done:
			t.Fatalf("Watch returned %v on a transport error; only ErrAuth may stop the loop", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v, want context.Canceled", err)
	}
}
