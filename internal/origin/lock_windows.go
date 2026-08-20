//go:build windows

package origin

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// lockPersist takes the exclusive cross-process lock for a persist path via
// LockFileEx on a sidecar (persist + ".lock" — issuetap replaces the persist
// file itself by temp+rename). Windows releases the lock when the process
// exits, matching the flock behavior on the other platforms.
func lockPersist(persist string) (func(), error) {
	f, err := os.OpenFile(persist+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("origin: persist lock: %w", err)
	}
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err != nil {
		_ = f.Close()
		if err == windows.ERROR_LOCK_VIOLATION {
			return nil, busyError(persist)
		}
		return nil, fmt.Errorf("origin: persist lock: %w", err)
	}
	writeLockPID(f)
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
		_ = f.Close()
	}, nil
}
