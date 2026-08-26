//go:build !windows

package origin

import (
	"errors"
	"os"
	"syscall"
)

// processAlive asks the kernel whether pid still exists. Signal 0 performs
// the permission and existence checks without delivering anything.
//
// EPERM means the process is alive and owned by someone else — a real state
// on a shared machine, and "alive" is the honest answer. Reading it as dead
// would let a conversion run under a holder it merely cannot signal.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid) // never fails on unix
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
