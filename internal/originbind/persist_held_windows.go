//go:build windows

package originbind

import (
	"os"

	"golang.org/x/sys/windows"
)

// persistHeld reports whether another process holds origin's persist lock
// (the sidecar at persist+".lock"). It reuses that path rather than a third
// HTTP probe. Missing lock file means no owner has embedded.
func persistHeld(persist string) (bool, error) {
	if persist == "" {
		return false, nil
	}
	f, err := os.OpenFile(persist+".lock", os.O_RDWR, 0o600)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err != nil {
		if err == windows.ERROR_LOCK_VIOLATION {
			return true, nil
		}
		return false, err
	}
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
	return false, nil
}
