// Parsers for the Linux /proc/<pid>/stat line. Deliberately without a
// //go:build linux tag, and named without a platform suffix while its
// callers (members_linux.go) carry one: these are pure functions over a
// string, so tagging them would only stop them being tested anywhere but
// Linux. `go test ./...` runs on ubuntu in CI, but a change made on a mac
// would then have no local gate at all.
package term

import (
	"strconv"
	"strings"
)

// parseProcStat reads pid and tty_nr (field 7) from a /proc/<pid>/stat
// blob. comm (field 2) is in parentheses and may contain spaces or
// parentheses, so fields after it are parsed from the last ')'.
func parseProcStat(stat string) (pid int, tty int32, ok bool) {
	lparen := strings.IndexByte(stat, '(')
	rparen := strings.LastIndexByte(stat, ')')
	if lparen <= 0 || rparen < lparen {
		return 0, 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(stat[:lparen]))
	if err != nil || pid <= 0 {
		return 0, 0, false
	}
	fields := strings.Fields(stat[rparen+1:])
	// After comm: state ppid pgrp session tty_nr ...
	if len(fields) < 5 {
		return 0, 0, false
	}
	n, err := strconv.ParseInt(fields[4], 10, 32)
	if err != nil {
		return 0, 0, false
	}
	return pid, int32(n), true
}

// parseProcStartTime reads starttime (field 22) from a /proc/<pid>/stat
// blob. It identifies an incarnation of a pid: the kernel recycles pids,
// and a stale one must not be signalled. Same last-')' rule as above —
// after comm the fields are state ppid pgrp session tty_nr tpgid flags
// minflt cminflt majflt cmajflt utime stime cutime cstime priority nice
// num_threads itrealvalue starttime, so starttime is index 19.
func parseProcStartTime(stat string) (uint64, bool) {
	rparen := strings.LastIndexByte(stat, ')')
	if rparen < 0 {
		return 0, false
	}
	fields := strings.Fields(stat[rparen+1:])
	if len(fields) < 20 {
		return 0, false
	}
	n, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
