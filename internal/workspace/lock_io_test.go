package workspace

import (
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// TestGetReleasesLockDuringOpen is the GDK-282 recurrence gate for site 2:
// two profiles must be able to construct at once. The signal is structural
// (TryLock + a second construct hook), not a wall-clock comparison.
func TestGetReleasesLockDuringOpen(t *testing.T) {
	setupHome(t)
	seedProfile(t, "alpha", &config.Config{
		Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "test-token", Projects: []string{"AAA"},
	})
	seedProfile(t, "beta", &config.Config{
		Site: "http://127.0.0.1:1", Email: "b@example.invalid", Token: "test-token", Projects: []string{"BBB"},
	})

	reg := New()
	t.Cleanup(func() { reg.Close() })

	aStarted := make(chan struct{})
	aHold := make(chan struct{})
	bStarted := make(chan struct{})
	t.Cleanup(func() {
		testBeforeConstruct = nil
		select {
		case <-aHold:
		default:
			close(aHold)
		}
	})

	testBeforeConstruct = func(name string) {
		switch name {
		case "alpha":
			close(aStarted)
			<-aHold
		case "beta":
			close(bStarted)
		}
	}

	errc := make(chan error, 2)
	go func() {
		_, err := reg.Get("alpha")
		errc <- err
	}()
	<-aStarted

	// Alpha is inside construction. If r.mu is still held, this fails
	// immediately — that is the pre-fix shape (FAIL-first, 2026-08-19).
	if !reg.mu.TryLock() {
		t.Fatal("Get still holds r.mu during construction — one profile's store.Open serialises every other Get/EnsureWatch/Close")
	}
	reg.mu.Unlock()

	go func() {
		_, err := reg.Get("beta")
		errc <- err
	}()
	<-bStarted

	close(aHold)
	for i := 0; i < 2; i++ {
		if err := <-errc; err != nil {
			t.Fatalf("Get: %v", err)
		}
	}

	if _, err := reg.Get("alpha"); err != nil {
		t.Fatalf("cached Get(alpha): %v", err)
	}
}
