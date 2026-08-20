package origin

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// writeLockPID records the owner's PID in the already-locked sidecar so a
// busy verdict can name who holds it (GDK-421). Best-effort: the lock is
// the flock/LockFileEx, never this content — a stale PID from a crashed
// owner is harmless because the kernel released the lock with the process.
func writeLockPID(f *os.File) {
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
}

// busyError is ErrWorkspaceBusy, naming the holder when the sidecar knows
// it — the audit's remaining window is a holder that hangs before it
// advertises, where "busy" alone sends the user to lsof (GDK-421).
func busyError(persist string) error {
	if pid := readLockPID(persist); pid != "" {
		return fmt.Errorf("%w — held by pid %s", ErrWorkspaceBusy, pid)
	}
	return ErrWorkspaceBusy
}

// readLockPID returns the PID recorded in the sidecar, or "" when absent
// or unreadable. Only meaningful while the lock is actually held.
func readLockPID(persist string) string {
	b, err := os.ReadFile(persist + ".lock")
	if err != nil {
		return ""
	}
	pid := strings.TrimSpace(string(b))
	if _, err := strconv.Atoi(pid); err != nil {
		return ""
	}
	return pid
}
