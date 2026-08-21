package sync

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// frozenJSON is the on-disk shape a demo/recording workspace writes. Tests
// unmarshal it rather than constructing Config.Frozen by name so a missing
// field is an assertion failure (JSON drop) instead of a compile failure.
const frozenJSON = `{
	"frozen": true,
	"site": "http://127.0.0.1:1",
	"email": "a@example.invalid",
	"token": "tok",
	"confluence": {},
	"linear": {"apiKey": "lin"}
}`

func loadFrozenCfg(t *testing.T) *config.Config {
	t.Helper()
	var cfg config.Config
	if err := json.Unmarshal([]byte(frozenJSON), &cfg); err != nil {
		t.Fatal(err)
	}
	return &cfg
}

// TestFrozenEntryPointsRefuseBeforeNetwork is GDK-181: every exported pull
// must return ErrFrozen with no network and no store write. 127.0.0.1:1 is
// connection-refused immediately — if the gate is missing, the error is a
// transport failure, not ErrFrozen.
func TestFrozenEntryPointsRefuseBeforeNetwork(t *testing.T) {
	cfg := loadFrozenCfg(t)
	if !cfg.SyncFrozen() {
		t.Fatal("unmarshaled config is not frozen — Frozen field missing or json tag wrong")
	}
	db := newMirror(t)
	ctx := context.Background()

	t.Run("Run", func(t *testing.T) {
		_, err := Run(ctx, cfg, db.DB, Options{})
		if !errors.Is(err, ErrFrozen) {
			t.Fatalf("Run: %v, want ErrFrozen", err)
		}
	})
	t.Run("RunConfluence", func(t *testing.T) {
		_, err := RunConfluence(ctx, cfg, db.DB, Options{})
		if !errors.Is(err, ErrFrozen) {
			t.Fatalf("RunConfluence: %v, want ErrFrozen", err)
		}
	})
	t.Run("RunLinear", func(t *testing.T) {
		_, err := RunLinear(ctx, cfg, db.DB, Options{})
		if !errors.Is(err, ErrFrozen) {
			t.Fatalf("RunLinear: %v, want ErrFrozen", err)
		}
	})
	t.Run("Watch", func(t *testing.T) {
		wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		err := Watch(wctx, cfg, db.DB, Options{Tick: 20 * time.Millisecond})
		if !errors.Is(err, ErrFrozen) {
			t.Fatalf("Watch: %v, want ErrFrozen", err)
		}
	})
}

// TestUnfrozenCfgDoesNotReturnErrFrozen: the gate is a no-op when frozen is
// off. Missing credential is still the existing error (TestRunRejectsMissingConfig).
func TestUnfrozenCfgDoesNotReturnErrFrozen(t *testing.T) {
	db := newMirror(t)
	_, err := Run(context.Background(), &config.Config{}, db.DB, Options{})
	if err == nil {
		t.Fatal("empty cfg: want credential error")
	}
	if errors.Is(err, ErrFrozen) {
		t.Fatalf("unfrozen cfg returned ErrFrozen: %v", err)
	}
	if !strings.Contains(err.Error(), "site, email and token are required") {
		t.Fatalf("unfrozen empty cfg error changed: %v", err)
	}
}

func TestWatchReloadFrozenSkipsInsteadOfExiting(t *testing.T) {
	// GDK-541: Reload seeing frozen must wait for the next tick, not return.
	site := newSite(t, "en")
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.SyncIntervalSec, cfg.ReconcileIntervalSec = 1, 3600

	var mu sync.Mutex
	reloads := 0
	reload := func() (*config.Config, error) {
		mu.Lock()
		defer mu.Unlock()
		reloads++
		c := *cfg
		c.SyncIntervalSec, c.ReconcileIntervalSec = 1, 3600
		if reloads >= 2 && reloads <= 5 {
			c.Frozen = true
		}
		return &c, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{
			Client: client,
			Reload: reload,
			Tick:   testTick,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := reloads
		mu.Unlock()
		if n >= 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Reload called %d times, want >= 4 (frozen skip stays in the loop)", n)
		}
		select {
		case err := <-done:
			t.Fatalf("Watch returned while frozen: %v (reloads %d)", err, n)
		case <-time.After(testTick):
		}
	}
	cancel()
	err := <-done
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v", err)
	}
}
