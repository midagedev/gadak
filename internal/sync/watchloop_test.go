package sync

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func restoreWatchLoop(t *testing.T) {
	t.Helper()
	origFn := watchFn
	origPause := watchRestartPause
	t.Cleanup(func() {
		watchFn = origFn
		watchRestartPause = origPause
	})
}

// TestWatchLoopReentersOnWatchError: Watch returning an error must not end
// the process loop — WatchLoop re-enters until ctx is cancelled.
func TestWatchLoopReentersOnWatchError(t *testing.T) {
	restoreWatchLoop(t)
	watchRestartPause = time.Millisecond

	var mu sync.Mutex
	n := 0
	watchFn = func(context.Context, *config.Config, *store.DB, Options) error {
		mu.Lock()
		n++
		mu.Unlock()
		return errors.New("boom")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		WatchLoop(ctx, &config.Config{}, nil, Options{Log: func(string) {}})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := n
		mu.Unlock()
		if got >= 2 {
			cancel()
			<-done
			return
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	mu.Lock()
	got := n
	mu.Unlock()
	t.Fatalf("WatchLoop reentered %d times, want >= 2", got)
}

// TestWatchLoopReturnsOnCancelDuringWatch: cancelling ctx while Watch is
// blocked must return without waiting out watchRestartPause.
func TestWatchLoopReturnsOnCancelDuringWatch(t *testing.T) {
	restoreWatchLoop(t)
	watchRestartPause = time.Hour

	entered := make(chan struct{})
	watchFn = func(ctx context.Context, _ *config.Config, _ *store.DB, _ Options) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		WatchLoop(ctx, &config.Config{}, nil, Options{})
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("WatchLoop did not enter Watch")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WatchLoop did not return after ctx cancel during Watch")
	}
}

// TestWatchLoopReturnsOnCancelDuringPause: cancelling ctx during the restart
// pause must return immediately, not after watchRestartPause.
func TestWatchLoopReturnsOnCancelDuringPause(t *testing.T) {
	restoreWatchLoop(t)
	watchRestartPause = time.Hour

	logged := make(chan struct{}, 1)
	watchFn = func(context.Context, *config.Config, *store.DB, Options) error {
		return errors.New("boom")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		WatchLoop(ctx, &config.Config{}, nil, Options{
			Log: func(string) {
				select {
				case logged <- struct{}{}:
				default:
				}
			},
		})
	}()

	select {
	case <-logged:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("WatchLoop did not log a stop before pausing")
	}
	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WatchLoop did not return after ctx cancel during pause")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("WatchLoop took %v to return after cancel; want immediate", d)
	}
}

// TestWatchLoopReloadAppliesOnNextEntry: Reload's config is what the next
// Watch entry sees.
func TestWatchLoopReloadAppliesOnNextEntry(t *testing.T) {
	restoreWatchLoop(t)
	watchRestartPause = time.Millisecond

	const initial = "http://initial.example.invalid"
	const reloaded = "http://reloaded.example.invalid"

	var mu sync.Mutex
	var sites []string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	watchFn = func(_ context.Context, cfg *config.Config, _ *store.DB, _ Options) error {
		site := ""
		if cfg != nil {
			site = cfg.Site
		}
		mu.Lock()
		sites = append(sites, site)
		n := len(sites)
		mu.Unlock()
		if n >= 2 {
			cancel()
		}
		return errors.New("stop")
	}

	WatchLoop(ctx, &config.Config{Site: initial}, nil, Options{
		Log: func(string) {},
		Reload: func() (*config.Config, error) {
			return &config.Config{Site: reloaded}, nil
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if len(sites) < 2 {
		t.Fatalf("entries %d, want >= 2: %v", len(sites), sites)
	}
	if sites[0] != initial {
		t.Fatalf("first entry site = %q, want %q", sites[0], initial)
	}
	if sites[1] != reloaded {
		t.Fatalf("second entry site = %q, want %q", sites[1], reloaded)
	}
}

// TestWatchLoopLogReceivesStopMessage: when opts.Log is set, the stop line
// goes there instead of the process-wide logger.
func TestWatchLoopLogReceivesStopMessage(t *testing.T) {
	restoreWatchLoop(t)
	watchRestartPause = time.Hour

	var mu sync.Mutex
	var logs []string
	watchFn = func(context.Context, *config.Config, *store.DB, Options) error {
		return errors.New("boom")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		WatchLoop(ctx, &config.Config{}, nil, Options{
			Log: func(s string) {
				mu.Lock()
				logs = append(logs, s)
				mu.Unlock()
			},
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := append([]string(nil), logs...)
		mu.Unlock()
		for _, line := range got {
			if strings.Contains(line, "sync loop stopped:") && strings.Contains(line, "boom") {
				cancel()
				<-done
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	mu.Lock()
	got := logs
	mu.Unlock()
	t.Fatalf("opts.Log never received stop message, logs=%q", got)
}
