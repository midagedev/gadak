package apprun

// A held persist lock with no advertise is the state that makes every
// concurrent CLI write fail busy with nowhere to route (GDK-343). It is the
// exact defect the mounted-workspace owner path was written to close, and
// the helper that path was lifted out of has to hold the same contract:
// if the advertise fails after the session is taken, drop the session.
//
// Today's only caller goes on to Runtime.Close, which releases it anyway.
// That is why this was invisible — and why it is worth a test rather than a
// second reading of the caller.

import (
	"os"
	"testing"

	"github.com/midagedev/gadak/internal/origin"
)

func TestStartOriginPassthroughReleasesTheLockWhenAdvertiseFails(t *testing.T) {
	rt, cfg := standaloneRuntime(t)
	dir := cfg.Directory()

	// Poison the advertise target: WriteAdvertise renames its temp file
	// onto this path, and rename onto a directory fails. Cheaper and more
	// honest than an unwritable profile dir, which would also break the
	// persist open we need to succeed first.
	if err := os.MkdirAll(origin.AdvertisePath(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = origin.Close()
		origin.ResetInProcess()
	})

	stop, err := StartOriginPassthrough(cfg, rt.API)
	if err == nil {
		stop()
		t.Fatal("advertise onto a directory should have failed")
	}
	if origin.IsInProcess(cfg) {
		t.Error("failed advertise left this process marked as the persist owner")
	}

	// Simulate the second process: forget the cached session so the next
	// Client has to take the lock itself. If the failed attempt still held
	// it, this is where a real CLI gets ErrWorkspaceBusy — with no
	// advertise file to route through, which is the dead end.
	origin.ForgetLive()
	if _, err := origin.Client(cfg); err != nil {
		t.Fatalf("a second process could not embed after the failed advertise — the lock was never released: %v", err)
	}
}
