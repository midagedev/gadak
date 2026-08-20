//go:build !windows

package origin

import (
	"fmt"
	"os"
	"syscall"
)

// lockPersist takes the exclusive cross-process lock for a persist path.
// The lock lives on a sidecar (persist + ".lock") because issuetap replaces
// the persist file itself by temp+rename — a lock on that inode would not
// survive the first write. flock is released by the kernel when the process
// dies, so a crashed owner never leaves the workspace stuck (the stale
// advertise file, F6's leftover, is thereby harmless too).
//
// A held lock returns ErrWorkspaceBusy; any other failure is its own error.
func lockPersist(persist string) (func(), error) {
	f, err := os.OpenFile(persist+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("origin: persist lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, busyError(persist)
		}
		return nil, fmt.Errorf("origin: persist lock: %w", err)
	}
	writeLockPID(f)
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
