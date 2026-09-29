package term

import (
	"encoding/binary"
	"sync/atomic"
	"unsafe"
)

// jobListWord is the byte width of one ULONG_PTR — the unit the
// JOBOBJECT_BASIC_PROCESS_ID_LIST header counts and pids are made of
// (learn.microsoft.com/windows/win32/api/winnt/ns-winnt-jobobject_basic_process_id_list:
// NumberOfAssignedProcesses and NumberOfProcessIdsInList are ULONG_PTR, so
// the counts are pointer-sized, not 32-bit). This file builds on every
// platform so the decoder's contract is pinnable by a test that runs on
// the dev machines too — the CI-only alternative left the layout
// unverified by anything until it ran on Windows (GDK-891).
const jobListWord = int(unsafe.Sizeof(uintptr(0)))

// jobListInlinePids is the fixed query buffer's pid capacity: a session's
// ordinary tree inline, bigger trees retried on the heap by jobPids.
const jobListInlinePids = 64

// jobListBytes is the byte size of a JOBOBJECT_BASIC_PROCESS_ID_LIST
// carrying n pids: two pointer-sized counts, then n pointer-sized pids.
func jobListBytes(pids int) int { return (2 + pids) * jobListWord }

// readWord decodes one little-endian pointer-sized word at the head of b.
func readWord(b []byte) uint64 {
	if jobListWord == 8 {
		return binary.LittleEndian.Uint64(b[:8])
	}
	return uint64(binary.LittleEndian.Uint32(b[:4]))
}

// decodeJobPidList decodes the bytes QueryInformationJobObject wrote for
// JobObjectBasicProcessIdList into the member pids and the assigned count
// (the count a too-small first query still fills in, which sizes the
// retry). Zero pids are skipped and an empty list answers nil — the
// "nothing running" fail direction membersOf reads.
//
// Both counts are ULONG_PTR — pointer-sized words at offsets 0 and
// jobListWord, pids from 2*jobListWord — never 32-bit fields: reading
// them 32-bit makes the in-list count the high half of the assigned
// count, which is zero, and every job answers empty (the members=[] of
// CI run 36625666166, pinned by TestDecodeJobPidList).
func decodeJobPidList(b []byte) (pids []int, assigned int) {
	if len(b) < 2*jobListWord {
		return nil, 0
	}
	assigned = int(readWord(b))
	inList := int(readWord(b[jobListWord:]))
	if inList > (len(b)-2*jobListWord)/jobListWord {
		inList = (len(b) - 2*jobListWord) / jobListWord
	}
	for i := 0; i < inList; i++ {
		if pid := int(readWord(b[(2+i)*jobListWord:])); pid != 0 {
			pids = append(pids, pid)
		}
	}
	return pids, assigned
}

// win32Failure is the ConPTY control plane's last recorded failure: which
// Win32 call failed and with what error code. A call name and a count —
// the paste-safe rule the doctor banner states; no paths, no error prose.
type win32Failure struct {
	call string
	code uint32
}

// lastWin32Failure is process-wide on purpose: doctor reports on the
// serve, not on one session, and "the terminal subsystem's last error" is
// a serve-shaped question. The recorder (recordWin32, in
// session_windows.go) is Windows-only code; on every other platform this
// stays nil and LastWin32Failure reports nothing.
var lastWin32Failure atomic.Pointer[win32Failure]

// LastWin32Failure is the diagnostic hook `gadak doctor` reads (GDK-891):
// the name of the Win32 call that last failed in the terminal's control
// plane — creation, resize, teardown — and its error code. ok is false
// when no failure has been recorded, which on non-Windows builds is
// always.
func LastWin32Failure() (call string, code int, ok bool) {
	if f := lastWin32Failure.Load(); f != nil {
		return f.call, int(f.code), true
	}
	return "", 0, false
}

// The ConPTY data-plane and child-lifetime counters doctor reads beside
// the failure record (GDK-891 follow-up): counts only, the same
// paste-safe rule the failure itself keeps. The recorders live in
// session_windows.go beside recordWin32 — every platform builds these
// vars, only Windows writes them, so doctor's row answers "none
// recorded" honestly everywhere else.
var (
	termReadCalls       atomic.Uint64
	termReadBytes       atomic.Uint64
	termChildrenStarted atomic.Uint64
	termChildrenLive    atomic.Int64
	termLastExit        atomic.Int64
	termLastExitKnown   atomic.Bool
)

// TermIOStats is the serve-wide snapshot of the ConPTY data plane: how
// many ReadFile calls the pumps have entered and how many bytes came
// back, how many children were started and are still registered, and the
// last reaped child's exit code. Zero counts and an unknown exit are the
// honest answer on every non-Windows build.
type TermIOStats struct {
	ReadCalls       uint64
	ReadBytes       uint64
	ChildrenStarted uint64
	ChildrenLive    uint64
	LastChildExit   int
	LastExitKnown   bool
}

// ReadTermIOStats is the doctor hook for the counters above — counts and
// one exit code, no paths, no prose.
func ReadTermIOStats() TermIOStats {
	return TermIOStats{
		ReadCalls:       termReadCalls.Load(),
		ReadBytes:       termReadBytes.Load(),
		ChildrenStarted: termChildrenStarted.Load(),
		ChildrenLive:    uint64(termChildrenLive.Load()),
		LastChildExit:   int(termLastExit.Load()),
		LastExitKnown:   termLastExitKnown.Load(),
	}
}
