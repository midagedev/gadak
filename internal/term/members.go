package term

import "sort"

// SessionMembers returns every process whose controlling terminal is the
// same device as shellPID's, including the shell itself. The empty set
// means the pid is gone or has no controlling terminal: a zero (or, on
// darwin, -1) device is not a key, because that would sweep every daemon.
func SessionMembers(shellPID int) []int {
	if shellPID <= 0 {
		return nil
	}
	out := sessionMembers(shellPID)
	sort.Ints(out)
	return out
}
