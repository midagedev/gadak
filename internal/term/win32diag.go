package term

import "sync/atomic"

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
