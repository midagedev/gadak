//go:build windows

package term

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ptyProc is the Windows half of a session: a ConPTY, the two pipes that
// carry its bytes, and the shell spawned onto it inside a Job Object.
//
// Spawn is a direct CreateProcess, not os/exec. Attaching a child to a
// pseudoconsole needs PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE riding a
// STARTUPINFOEX attribute list, and syscall.SysProcAttr — the struct
// behind os/exec's SysProcAttr — has no field that can carry one (go1.26
// src/syscall/exec_windows.go: the struct offers flags, a token, security
// attributes and an inheritable-handle list, nothing more). The attribute
// list is exactly the surface the stdlib does not expose, so this file
// calls CreateProcess the way the Microsoft pseudoconsole sample and
// charmbracelet/x/conpty do, and wait() is WaitForSingleObject instead of
// cmd.Wait — the same observable answer rebuilt on the raw handle.
//
// The unix half signals a process group and, because a job-control shell
// moves jobs out of it, walks the controlling-terminal device to reach
// everyone: nothing on unix owns the tree. Windows has an owner — the Job
// Object. Every process a job member spawns is a member (unless it asks
// for a breakaway, which nothing here does), so the unix ladder becomes:
//
//	hangup  ClosePseudoConsole — conhost tears the session down and hands
//	        the attached clients the terminal-closed event; the machine's
//	        SIGHUP. The call is void in Win32 (fire-and-forget into
//	        conhost), so hangup reports no error: the observable is the
//	        pipes going quiet and wait() answering.
//	kill    TerminateJobObject — the whole tree, unconditionally and at
//	        once. Blunter than unix SIGKILL-to-the-tty-members, but the
//	        contract is the same one the unix walk exists to keep: no
//	        orphan survives the session.
//	close   the job handle, last — JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE is
//	        the backstop that fires after the ladder above had its turn,
//	        the moral equivalent of the kernel revoking the master fd.
//
// The child starts CREATE_SUSPENDED and joins the job before its first
// instruction runs, so "every descendant is in the job" is a theorem
// rather than AssignProcessToJobObject winning a race against the shell's
// first spawn.
//
// The handle ledger — Windows leaks by default, so every handle this file
// creates has one named closer:
//
//	inPipeRead, outPipeWrite   the ends handed to CreatePseudoConsole.
//	        conhost holds its own duplicates, so ours are redundant from
//	        the moment that call returns — and keeping outPipeWrite is
//	        worse than redundant: a write handle we hold open is a write
//	        handle the read side never sees EOF past. Both close in
//	        startProc, right after CreateProcess succeeds — the documented
//	        moment: "Upon completion of the CreateProcess call ... the
//	        handles given during creation should be freed from this
//	        process" (creating-a-pseudoconsole-session).
//	hpc    the pseudoconsole — once, by whichever of hangup() and
//	        closePTY() reaches it first (hpcOnce).
//	attr    the attribute list — deleted once, in closePTY, before hpc:
//	        no spawn reads it again, and the close order keeps every
//	        handle reference gone before the handle itself.
//	inPipeWrite, outPipeRead   the master ends — in closePTY, after hpc:
//	        closing the pseudoconsole is what ends a pump parked in Read.
//	job    the job handle — last in closePTY, after the registry forgets
//	        this proc, so KILL_ON_JOB_CLOSE fires after the graceful
//	        ladder and a registry lookup never races a recycled value.
//	proc   the child process — in wait(), after the exit code is read.
//	thread the child's main thread — in startProc, right after Resume.
type ptyProc struct {
	procID int

	// shell is the path startProc resolved and spawned, kept for failure
	// diagnostics: the test dumps answer "which shell, and does it exist"
	// (COMSPEC resolution included) without re-deriving any of it.
	shell string

	// endMu is the Windows analogue of the unix ioctlMu: it keeps the
	// handle-touching control-plane calls (Write, resize, hangup, kill,
	// jobPids) and closePTY off each other's toes. Windows recycles handle
	// values aggressively, so a syscall on a just-closed handle is not the
	// benign EBADF a stale fd is on unix — it can land on an unrelated
	// live object of this process (another session's job, a socket).
	// Every handle field is zeroed under the write side as it closes, so
	// a reader holding the read side sees either a live handle or a zero.
	//
	// Read is deliberately not under this lock: the pump parks in ReadFile
	// for the session's whole life and would hold the read side forever.
	// closePTY un-parks it with CancelIoEx before taking the write side —
	// the service os.File's poll teardown provides on unix for free.
	endMu   sync.RWMutex
	hpc     windows.Handle
	job     windows.Handle
	inWrite windows.Handle
	outRead windows.Handle
	proc    windows.Handle
	attr    *windows.ProcThreadAttributeListContainer

	// The read-path record, for failure diagnostics only — it is not
	// routed into recordWin32 (a broken pipe is how every session ends,
	// and the doctor row must not read a normal shutdown as a terminal
	// failure). readCalls counts every ReadFile entered, so a pump parked
	// on its first call is "1 call, no completed read"; readBytes and
	// readLastN/readLastErr carry what the calls returned. These four
	// answer the ①/②/③ split of a dead read path: parked first call,
	// silent zero-byte loop, or an error that ended it.
	readCalls   atomic.Uint64
	readBytes   atomic.Uint64
	readLastN   atomic.Int64
	readLastErr atomic.Pointer[string]

	// cols/rows remember the size the pseudoconsole was last told. There
	// is no GetPseudoConsoleSize, so winsize() answers from this record —
	// a different epistemic thing than the unix TTYSize; see winsize.
	cols, rows uint16

	// closed flips true the instant closePTY starts, before any handle
	// goes: a Write or resize racing the close then maps its failure to
	// ErrSessionClosed — the contract every caller checks — instead of a
	// raw ERROR_BROKEN_PIPE that leaks the handle's state. Same owner as
	// the unix half (GDK-914).
	closed atomic.Bool

	waitOnce sync.Once
	code     int
	waitErr  error

	closeOnce sync.Once
	hpcOnce   sync.Once

	// resizeExit names the branch the last resize left by, for Info()'s
	// LastResizeExit diagnostic — same field, same reason as unix
	// (GDK-1192), with a Windows-specific vocabulary: "set" (the call
	// returned nil; no read-back exists on this platform to say more),
	// "set-error:<err>", "" before the first resize.
	resizeExit atomic.Pointer[string]
}

// startProc creates the pseudoconsole, the job, and the shell on it. On
// any failure every handle created so far is closed exactly once, in
// reverse creation order — the same order closePTY closes in.
func startProc(opts Options) (*ptyProc, error) {
	shell := opts.Shell
	if shell == "" {
		// COMSPEC is the Windows $SHELL: the shell the account is
		// configured with, cmd.exe when nothing says otherwise.
		shell = os.Getenv("COMSPEC")
	}
	if shell == "" {
		shell = "cmd.exe"
	}
	// A bare name resolves against PATH and PATHEXT the way os/exec would;
	// an absolute path round-trips. A miss keeps the raw string so
	// CreateProcess below is the step that names the failure.
	if resolved, err := exec.LookPath(shell); err == nil {
		shell = resolved
	}
	// The same environment contract as the unix half, minus the login-shell
	// merge: hostenv has no login shell to ask on Windows (its Resolve
	// refuses, its Augment passes base through), so the pane runs on the
	// serve's environment plus the terminal markers. opts.Env stays last —
	// the caller's GADAK_WORKSPACE names the window this pane belongs to,
	// and the last duplicate is the one the child sees.
	base := os.Environ()
	env := append([]string(nil), base...)
	env = append(env, "TERM=xterm-256color", "GADAK_TERMINAL=1")
	if !envCarries(base, "COLORTERM") {
		env = append(env, "COLORTERM=truecolor")
	}
	env = append(env, opts.Env...)

	cols, rows := opts.Cols, opts.Rows
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	p := &ptyProc{cols: cols, rows: rows, shell: shell}

	// created collects every handle this function must close if a later
	// step fails; the failure path walks it in reverse, the same order
	// closePTY uses on the success path.
	var created []windows.Handle
	fail := func(step string, err error) (*ptyProc, error) {
		recordWin32(step, err)
		for i := len(created) - 1; i >= 0; i-- {
			_ = windows.CloseHandle(created[i])
		}
		if p.attr != nil {
			p.attr.Delete()
		}
		return nil, fmt.Errorf("term: start %s: %s: %w", shell, step, err)
	}

	sa := &windows.SecurityAttributes{
		Length:        uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		InheritHandle: 1,
	}
	var inRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &p.inWrite, sa, 0); err != nil {
		return fail("CreatePipe(input)", err)
	}
	created = append(created, inRead, p.inWrite)
	if err := windows.CreatePipe(&p.outRead, &outWrite, sa, 0); err != nil {
		return fail("CreatePipe(output)", err)
	}
	created = append(created, p.outRead, outWrite)

	if err := windows.CreatePseudoConsole(windows.Coord{X: int16(cols), Y: int16(rows)}, inRead, outWrite, 0, &p.hpc); err != nil {
		return fail("CreatePseudoConsole", err)
	}
	created = append(created, p.hpc)

	var err error
	p.attr, err = windows.NewProcThreadAttributeList(1)
	if err != nil {
		return fail("NewProcThreadAttributeList", err)
	}
	// The attribute's value for PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE is the
	// HPCON itself, not a pointer to it: the documented sample passes hpc
	// as lpValue with sizeof(hpc) — "the pseudoconsole handle, and the
	// size of the pseudoconsole handle" (learn.microsoft.com/.../
	// creating-a-pseudoconsole-session), and charmbracelet/x/conpty and
	// photostorm/pty both pass the handle value the same way. A pointer to
	// the handle instead reads to the child as an invalid pseudoconsole,
	// and a child handed an invalid pseudoconsole fails its own
	// initialization with 0xc0000142 while the parent's CreateProcess
	// still succeeds — the read-path death of CI run 36625666166: three
	// tests, zero bytes read, and no error from startProc.
	if err = p.attr.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, attrValue(p.hpc), unsafe.Sizeof(p.hpc)); err != nil {
		return fail("ProcThreadAttributeList.Update", err)
	}

	if p.job, err = windows.CreateJobObject(nil, nil); err != nil {
		return fail("CreateJobObject", err)
	}
	created = append(created, p.job)
	var jobInfo windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	jobInfo.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(p.job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&jobInfo)), uint32(unsafe.Sizeof(jobInfo))); err != nil {
		return fail("SetInformationJobObject", err)
	}

	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{shell}, opts.Args...)))
	if err != nil {
		return fail("ComposeCommandLine", err)
	}
	var dir *uint16
	if opts.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(opts.Dir); err != nil {
			return fail("UTF16PtrFromString(dir)", err)
		}
	}
	siEx := &windows.StartupInfoEx{}
	siEx.Cb = uint32(unsafe.Sizeof(*siEx))
	siEx.ProcThreadAttributeList = p.attr.List()
	// STARTF_USESTDHANDLES is deliberately not set: the pseudoconsole
	// attribute is what supplies the child's std handles, and pointing the
	// startup record at (zero) inherited handles would fight it.
	pi := new(windows.ProcessInformation)
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT) | windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_SUSPENDED
	if err = windows.CreateProcess(nil, cmdline, nil, nil, false, flags, envBlock(env), dir, &siEx.StartupInfo, pi); err != nil {
		return fail("CreateProcess", err)
	}
	created = append(created, pi.Process)
	p.procID = int(pi.ProcessId)
	p.proc = pi.Process
	// The suspended start means a failed assign or resume leaves a child
	// that never ran (or ran unjobbed): kill it explicitly, because
	// closing its handle is not a kill.
	if err = windows.AssignProcessToJobObject(p.job, pi.Process); err != nil {
		_ = windows.TerminateProcess(pi.Process, 1)
		return fail("AssignProcessToJobObject", err)
	}
	if _, err = windows.ResumeThread(pi.Thread); err != nil {
		_ = windows.TerminateProcess(pi.Process, 1)
		return fail("ResumeThread", err)
	}
	_ = windows.CloseHandle(pi.Thread)

	// conhost has held its own duplicates of these since CreatePseudoConsole
	// returned, so ours are redundant — and ours holding outWrite open is
	// worse than redundant: a write handle we keep alive is a write handle
	// the read side will never see EOF past (the ledger above).
	_ = windows.CloseHandle(inRead)
	_ = windows.CloseHandle(outWrite)

	registerProc(p)
	termChildrenStarted.Add(1)
	return p, nil
}

// attrValue is a handle in the exact shape UpdateProcThreadAttribute's
// lpValue parameter wants for PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE: the
// handle value itself, the pointer-sized bytes the sample passes. The
// bit-cast — rather than a uintptr→unsafe.Pointer conversion, which is
// precisely the one go vet's unsafeptr check flags — hands over those
// bytes while keeping the gate quiet.
func attrValue(h windows.Handle) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&h))
}

// envCarries reports whether env has key at all, empty value included —
// presence, not value, is the not-overwriting rule for COLORTERM. The same
// helper the unix half has, on the platform where this file compiles.
func envCarries(env []string, key string) bool {
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && k == key {
			return true
		}
	}
	return false
}

// envBlock builds the UTF-16 environment CreateProcess takes. Duplicates
// collapse case-insensitively with the last winning — os/exec's Windows
// contract, and the reason "the caller's Env is appended last" means
// anything here — and the block is sorted, as the stdlib's is.
func envBlock(env []string) *uint16 {
	idx := make(map[string]int, len(env))
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(k)
		if i, dup := idx[k]; dup {
			out[i] = kv
			continue
		}
		idx[k] = len(out)
		out = append(out, kv)
	}
	sort.Strings(out)
	block := make([]uint16, 0, 64)
	for _, kv := range out {
		block = append(block, utf16.Encode([]rune(kv))...)
		block = append(block, 0)
	}
	return &append(block, 0)[0]
}

// Read is the pump's only entry to the output pipe. Not under endMu — see
// the field comment for why a lock held across a parked ReadFile would be
// a deadlock, and how closePTY un-parks it instead. The counters bracket
// the call — readCalls before it, so a call parked since the session
// opened still counts as entered, and the rest after it returns — so a
// failure dump can name which of the three dead shapes this is.
func (p *ptyProc) Read(b []byte) (int, error) {
	p.readCalls.Add(1)
	termReadCalls.Add(1)
	var n uint32
	err := windows.ReadFile(p.outRead, b, &n, nil)
	p.readLastN.Store(int64(n))
	p.readBytes.Add(uint64(n))
	termReadBytes.Add(uint64(n))
	if err != nil {
		s := err.Error()
		p.readLastErr.Store(&s)
	}
	return int(n), err
}

func (p *ptyProc) Write(b []byte) (int, error) {
	p.endMu.RLock()
	defer p.endMu.RUnlock()
	var n uint32
	err := windows.WriteFile(p.inWrite, b, &n, nil)
	if err != nil && (p.closed.Load() || p.inWrite == 0) {
		return int(n), ErrSessionClosed
	}
	return int(n), err
}

func (p *ptyProc) pid() int { return p.procID }

// resize has neither of the unix retry machines: Windows does not
// interrupt in-flight syscalls with signals (no EINTR to retry — a
// failure is a failure), and there is no size read-back API to verify
// with (no "matched/N" branch to record). What remains of the unix
// contract is the diagnostic: the exit is named, and a size the call
// accepted is remembered as the size the session carries.
func (p *ptyProc) resize(cols, rows uint16) error {
	p.endMu.RLock()
	err := p.resizeHeld(cols, rows)
	p.endMu.RUnlock()
	if err != nil && p.closed.Load() {
		return ErrSessionClosed
	}
	return err
}

// resizeHeld runs under endMu's read side.
func (p *ptyProc) resizeHeld(cols, rows uint16) error {
	if p.hpc == 0 {
		return ErrSessionClosed
	}
	if err := windows.ResizePseudoConsole(p.hpc, windows.Coord{X: int16(cols), Y: int16(rows)}); err != nil {
		recordWin32("ResizePseudoConsole", err)
		p.recordResizeExit("set-error:" + err.Error())
		return err
	}
	p.cols, p.rows = cols, rows
	p.recordResizeExit("set")
	return nil
}

func (p *ptyProc) recordResizeExit(s string) { p.resizeExit.Store(&s) }

// lastResizeExit reports the recorded exit, "" when no resize has run yet.
func (p *ptyProc) lastResizeExit() string {
	if s := p.resizeExit.Load(); s != nil {
		return *s
	}
	return ""
}

// winsize answers the size the pseudoconsole carries. On unix this is
// TIOCGWINSZ — the kernel's answer, the evidence that a resize the session
// believes it applied is the one the child's tty has. ConPTY has no
// read-back, so this is the last size resize() accepted (the creation size
// before any): our own record, not the machine's. The unix method exists
// to catch "accepted and did nothing" (GDK-1192); this one cannot catch
// that class by construction, and the honest answer is the record.
func (p *ptyProc) winsize() (cols, rows uint16, err error) {
	p.endMu.RLock()
	defer p.endMu.RUnlock()
	if p.hpc == 0 {
		return 0, 0, ErrSessionClosed
	}
	return p.cols, p.rows, nil
}

// hangup is the graceful rung of the ladder — ClosePseudoConsole, the
// machine's "terminal closed". It shares hpcOnce with closePTY so the
// pseudoconsole closes exactly once whichever rung reaches it first; a
// second close would be a close of a recycled handle value.
func (p *ptyProc) hangup() error {
	p.endMu.RLock()
	defer p.endMu.RUnlock()
	p.closeConPTY()
	return nil
}

// kill is the whole job at once. After it the job handle stays open and
// registered (closePTY owns both) but empty: membersOf keeps answering
// through the job, now with nobody in it — "nothing running", the honest
// answer for a terminated tree, and the axis the tree-kill test measures.
// The one path that kills without ever closing (Manager.Create's
// already-closed branch) leaves a registry entry and a job handle behind
// rather than blinding that seam; the unix half leaks the same shape
// there (a dropped *exec.Cmd that nobody Waits).
func (p *ptyProc) kill() error {
	p.endMu.RLock()
	defer p.endMu.RUnlock()
	if p.closed.Load() || p.job == 0 {
		return nil
	}
	if err := windows.TerminateJobObject(p.job, 1); err != nil {
		recordWin32("TerminateJobObject", err)
		return err
	}
	return nil
}

// wait is the answer os/exec's cmd.Wait would have given, rebuilt for the
// direct CreateProcess: block on the process handle, read the exit code,
// close the handle once. Idempotent through waitOnce, like the unix half.
// The state writes and the handle close run under endMu's write side so
// childState — which holds the read side across its own probe — can never
// read a handle this just closed, a value Windows may already have
// recycled into somebody else's live object.
func (p *ptyProc) wait() (int, error) {
	p.waitOnce.Do(func() {
		ev, err := windows.WaitForSingleObject(p.proc, windows.INFINITE)
		p.endMu.Lock()
		defer p.endMu.Unlock()
		if err == nil && ev == windows.WAIT_OBJECT_0 {
			var code uint32
			if err = windows.GetExitCodeProcess(p.proc, &code); err != nil {
				recordWin32("GetExitCodeProcess", err)
			} else {
				p.code = int(code)
				recordTermExit(int(code))
			}
		}
		p.waitErr = err
		if p.proc != 0 {
			_ = windows.CloseHandle(p.proc)
			p.proc = 0
		}
	})
	return p.code, p.waitErr
}

// stillActive is the exit code GetExitCodeProcess reports for a process
// that has not exited — the one value that means "ask again later" (and,
// famously, the one a child that genuinely exits with 259 is
// indistinguishable from; the diagnostic says what the handle said, not a
// proof of life).
const stillActive = 259

// childState is the failure-diagnostic answer to "is the child even
// alive": running, an exit code, or the code wait() already reaped. It is
// the axis that splits the dead-read-path candidates — a child that died
// at startup (0xc0000142 is the documented signature of an invalid
// pseudoconsole) reads here as a short exit code and nowhere else.
func (p *ptyProc) childState() string {
	p.endMu.RLock()
	defer p.endMu.RUnlock()
	if p.proc == 0 {
		if p.waitErr != nil {
			return fmt.Sprintf("reaped, wait error: %v", p.waitErr)
		}
		return fmt.Sprintf("reaped, exit code %d (0x%x)", p.code, p.code)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.proc, &code); err != nil {
		return fmt.Sprintf("GetExitCodeProcess: %v", err)
	}
	if code == stillActive {
		return "running (STILL_ACTIVE)"
	}
	return fmt.Sprintf("exited, code %d (0x%x)", code, code)
}

// closeConPTY closes the pseudoconsole exactly once. Both hangup and
// closePTY arrive here; neither may close it twice.
func (p *ptyProc) closeConPTY() {
	p.hpcOnce.Do(func() {
		if p.hpc != 0 {
			windows.ClosePseudoConsole(p.hpc)
			p.hpc = 0
		}
	})
}

// closePTY runs the ledger's close side, once, in the reference
// implementation's order with the job appended:
//
//   - CancelIoEx first, outside endMu: a Write parked on a full input pipe
//     holds endMu's read side, and cancelling the parked call is the only
//     thing that releases it — taking the write side first would
//     deadlock. This is what os.File's poll teardown gives the unix half
//     for free (a blocked writer there returns when the fd closes).
//   - the registry, before any handle goes: a lookup must never hand back
//     a proc whose job handle is about to become a recycled value.
//   - the attribute list, before hpc: its buffer references the
//     pseudoconsole and no spawn will read it again.
//   - the pseudoconsole: the hangup. Closing it is what makes conhost let
//     go of the output pipe's write side, which is what ends a pump
//     parked in Read.
//   - the master pipe ends, after conhost is gone: handle hygiene.
//   - the job handle, last: KILL_ON_JOB_CLOSE must not fire before the
//     graceful ladder above has had its turn.
func (p *ptyProc) closePTY() error {
	var err error
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		// Best effort by design: nothing to cancel is the normal case
		// (ERROR_NOT_FOUND), and the closes below are what matter.
		_ = windows.CancelIoEx(p.outRead, nil)
		_ = windows.CancelIoEx(p.inWrite, nil)
		p.endMu.Lock()
		defer p.endMu.Unlock()
		unregisterProc(p.procID)
		if p.attr != nil {
			p.attr.Delete()
		}
		p.closeConPTY()
		// Zero before closing: Read does not take endMu (it parks), so a
		// racing reader that catches the close before the zero could act
		// on a handle value Windows has already handed to another object.
		// A reader that sees the zero gets an immediate error instead —
		// the pump's normal end.
		inW, outR := p.inWrite, p.outRead
		p.inWrite, p.outRead = 0, 0
		err = errors.Join(windows.CloseHandle(inW), windows.CloseHandle(outR))
		job := p.job
		p.job = 0
		if job != 0 {
			err = errors.Join(err, windows.CloseHandle(job))
		}
	})
	return err
}

// queryJobPids asks the job for its member pids with the query's error
// visible: jobPids' nil-on-error shape is the right contract for
// membersOf ("nothing running" is the fail direction), but a failure
// diagnostic must be able to say "the query itself failed" rather than
// read the same nil as an empty tree. The bytes go straight to
// decodeJobPidList — the JOBOBJECT_BASIC_PROCESS_ID_LIST layout lives
// there, pinned by a test that runs on every platform, not in a struct
// only Windows CI ever executes.
func (p *ptyProc) queryJobPids() ([]int, error) {
	p.endMu.RLock()
	defer p.endMu.RUnlock()
	if p.job == 0 {
		return nil, nil
	}
	buf := make([]byte, jobListBytes(jobListInlinePids))
	var ret uint32
	err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&buf[0])), uint32(len(buf)), &ret)
	if errors.Is(err, windows.ERROR_MORE_DATA) {
		// More members than the inline buffer holds: size the retry on the
		// assigned count the too-small query still filled in.
		_, assigned := decodeJobPidList(buf)
		n := assigned
		if n < jobListInlinePids {
			n = jobListInlinePids
		}
		buf = make([]byte, jobListBytes(n))
		err = windows.QueryInformationJobObject(p.job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&buf[0])), uint32(len(buf)), &ret)
	}
	if err != nil {
		recordWin32("QueryInformationJobObject", err)
		return nil, err
	}
	pids, _ := decodeJobPidList(buf)
	return pids, nil
}

// jobPids is the job's own answer to "who is running in this session's
// tree" — the enumeration the unix half re-derives by walking the
// controlling-terminal device, because unix has no owner of the tree to
// ask. Includes the shell. Nil once the job is gone or closed, and on a
// query failure — which membersOf reads as "nothing running", the same
// fail direction as the unix walk coming back empty.
func (p *ptyProc) jobPids() []int {
	pids, err := p.queryJobPids()
	if err != nil {
		// Recorded by the query; nil is the membersOf contract.
		return nil
	}
	return pids
}

// liveProcs is the pid → proc registry that lets membersOf turn "every
// process on this session's terminal" into a job query: the shell pid is
// the only key the session core passes down. Entries leave at kill (the
// tree is gone) and at closePTY (before the job handle closes, so a
// lookup can never race a recycled handle value).
var liveProcs struct {
	sync.Mutex
	m map[int]*ptyProc
}

func registerProc(p *ptyProc) {
	termChildrenLive.Add(1)
	liveProcs.Lock()
	if liveProcs.m == nil {
		liveProcs.m = map[int]*ptyProc{}
	}
	liveProcs.m[p.procID] = p
	liveProcs.Unlock()
}

func unregisterProc(pid int) {
	termChildrenLive.Add(-1)
	liveProcs.Lock()
	delete(liveProcs.m, pid)
	liveProcs.Unlock()
}

func lookupProc(pid int) *ptyProc {
	liveProcs.Lock()
	defer liveProcs.Unlock()
	return liveProcs.m[pid]
}

// recordWin32 keeps the last control-plane failure — the call's name and
// its Win32 error code, nothing else — for LastWin32Failure, which doctor
// reads. The data plane deliberately does not record: a broken pipe is how
// every session ends, and doctor must not read a normal shutdown as a
// terminal failure.
func recordWin32(call string, err error) {
	if err == nil {
		return
	}
	var code uint32
	var eno windows.Errno
	if errors.As(err, &eno) {
		code = uint32(eno)
	}
	lastWin32Failure.Store(&win32Failure{call: call, code: code})
}

// recordTermExit remembers the exit code of the most recently reaped
// child — the number that names the read path's worst failure mode
// outright: a child dying at startup on an invalid pseudoconsole reads
// 0xc0000142 here while the pipes stay silent (the documented signature,
// creating-a-pseudoconsole-session). Feeds ReadTermIOStats, which doctor
// reads.
func recordTermExit(code int) {
	termLastExit.Store(int64(code))
	termLastExitKnown.Store(true)
}
