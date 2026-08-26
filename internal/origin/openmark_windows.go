//go:build windows

package origin

import "os"

// processAlive asks Windows for a handle to pid. Unlike unix, FindProcess
// here is a real OpenProcess and fails for a PID that no longer exists,
// which is exactly the question being asked.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
