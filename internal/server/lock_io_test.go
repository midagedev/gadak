package server

import (
	"context"
	"testing"
)

// TestDerivedReleasesLockDuringRebuild is the GDK-282 recurrence gate for
// site 1: a cache-miss rebuild must not hold s.mu across IssueLites /
// buildView. The signal is structural (TryLock + a second construct hook
// on a different key), not a wall-clock comparison.
func TestDerivedReleasesLockDuringRebuild(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	s := h.s
	s.cached = nil

	aStarted := make(chan struct{})
	aHold := make(chan struct{})
	bStarted := make(chan struct{})
	t.Cleanup(func() {
		testBeforeDerived = nil
		select {
		case <-aHold:
		default:
			close(aHold)
		}
	})

	testBeforeDerived = func() {
		close(aStarted)
		<-aHold
	}

	errc := make(chan error, 2)
	go func() {
		_, err := s.derived(context.Background(), 1, nil)
		errc <- err
	}()
	<-aStarted

	if !s.mu.TryLock() {
		t.Fatal("derived still holds s.mu during rebuild — one cache miss serialises every other derived caller")
	}
	s.mu.Unlock()

	testBeforeDerived = func() { close(bStarted) }
	go func() {
		_, err := s.derived(context.Background(), 2, nil)
		errc <- err
	}()
	<-bStarted

	close(aHold)
	for i := 0; i < 2; i++ {
		if err := <-errc; err != nil {
			t.Fatalf("derived: %v", err)
		}
	}
}
