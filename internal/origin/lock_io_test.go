package origin

import (
	"os"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func seedStandalone(t *testing.T, name string) *config.Config {
	t.Helper()
	dir, err := config.DirFor(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.LoadFor(name)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Kind = config.KindStandalone
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err = config.LoadFor(name)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// TestStandaloneSessionReleasesLockDuringOpen is the GDK-282 recurrence
// gate for site 3: two persist paths must construct at once. The signal is
// structural (TryLock + a second construct hook), not a wall-clock compare.
func TestStandaloneSessionReleasesLockDuringOpen(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = Close()
		config.SetProfile("")
	})

	cfgA := seedStandalone(t, "alpha")
	cfgB := seedStandalone(t, "beta")
	pathA := PersistPath(cfgA.Directory())
	pathB := PersistPath(cfgB.Directory())

	aStarted := make(chan struct{})
	aHold := make(chan struct{})
	bStarted := make(chan struct{})
	t.Cleanup(func() {
		testBeforeStandalone = nil
		select {
		case <-aHold:
		default:
			close(aHold)
		}
	})

	testBeforeStandalone = func(persist string) {
		switch persist {
		case pathA:
			close(aStarted)
			<-aHold
		case pathB:
			close(bStarted)
		}
	}

	errc := make(chan error, 2)
	go func() {
		_, err := Client(cfgA)
		errc <- err
	}()
	<-aStarted

	if !mu.TryLock() {
		t.Fatal("standaloneSession still holds the process-global mutex during NewEmbedded — one persist write serialises every workspace")
	}
	mu.Unlock()

	go func() {
		_, err := Client(cfgB)
		errc <- err
	}()
	<-bStarted

	close(aHold)
	for i := 0; i < 2; i++ {
		if err := <-errc; err != nil {
			t.Fatalf("Client: %v", err)
		}
	}
}
