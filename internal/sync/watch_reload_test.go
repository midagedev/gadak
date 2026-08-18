package sync

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// TestWatchReloadsConfigEachCycle: Reload runs at the top of every cycle and the
// new config is what the subsequent Run uses (project filter appears in JQL).
func TestWatchReloadsConfigEachCycle(t *testing.T) {
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
		if reloads >= 2 {
			c.Projects = []string{"ZZZ"}
		} else {
			c.Projects = []string{"NMB"}
		}
		return &c, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	passes := make(chan struct{}, 8)
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, cfg, db.DB, Options{
			Full:   true,
			Client: client,
			Reload: reload,
			Tick:   testTick, // existing Watch seam; do not sit on the 1s floor
			Log: func(line string) {
				if strings.HasPrefix(line, "done:") {
					select {
					case passes <- struct{}{}:
					default:
					}
				}
			},
		})
	}()

	select {
	case <-passes:
	case err := <-done:
		t.Fatalf("Watch ended before first pass: %v", err)
	}
	select {
	case <-passes:
	case err := <-done:
		t.Fatalf("Watch ended before second pass: %v", err)
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v", err)
	}

	mu.Lock()
	n := reloads
	mu.Unlock()
	if n < 2 {
		t.Fatalf("Reload called %d times, want >= 2", n)
	}

	site.mu.Lock()
	jqls := append([]string{}, site.allJQLs...)
	site.mu.Unlock()
	sawZZZ := false
	for _, j := range jqls {
		if strings.Contains(j, "ZZZ") {
			sawZZZ = true
			break
		}
	}
	if !sawZZZ {
		t.Fatalf("reloaded project ZZZ never appeared in JQL; got %v", jqls)
	}
}

// TestSyncScopeCountsOnly: log-facing scope string holds counts, never keys.
func TestSyncScopeCountsOnly(t *testing.T) {
	got := syncScope(&config.Config{
		Projects:   []string{"NMB", "SECRETKEY"},
		Confluence: &config.ConfluenceConfig{Spaces: []string{"ENG"}},
	})
	for _, leak := range []string{"NMB", "SECRETKEY", "ENG"} {
		if strings.Contains(got, leak) {
			t.Fatalf("syncScope leaked %q: %q", leak, got)
		}
	}
	if !strings.Contains(got, "2 projects") {
		t.Fatalf("want project count in %q", got)
	}
	if !strings.Contains(got, "1 space") {
		t.Fatalf("want space count in %q", got)
	}

	off := syncScope(&config.Config{})
	if strings.Contains(off, "NMB") || strings.Contains(off, "ENG") {
		t.Fatalf("empty scope leaked keys: %q", off)
	}
	if !strings.Contains(off, "all projects") || !strings.Contains(off, "confluence off") {
		t.Fatalf("empty scope = %q", off)
	}
}
