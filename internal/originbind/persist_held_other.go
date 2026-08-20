//go:build !windows

package originbind

import (
	"os"
	"syscall"
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
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return true, nil
		}
		return false, err
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false, nil
}
