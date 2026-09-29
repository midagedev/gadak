//go:build windows

package term

// sessionMembers answers membersOf's "every process on this session's
// terminal" (the idle verdict and the roster's PIDs column) with a query
// to the session's Job Object — see jobPids.
//
// The unix halves walk the controlling-terminal device because nothing
// there owns the tree; the walk is once-and-remembered, with start-time
// stamps against pid recycling, because the terminal is revoked under it.
// Here the job owns the tree, so the member list is a query instead of a
// walk: no revocation race to survive, no stamps to check. A pid the
// registry does not know (never started, killed, or closed) answers nil,
// which the callers read as "nothing running" — the same fail direction
// as the unix walk coming back empty.
func sessionMembers(shellPID int) []int {
	p := lookupProc(shellPID)
	if p == nil {
		return nil
	}
	return p.jobPids()
}
