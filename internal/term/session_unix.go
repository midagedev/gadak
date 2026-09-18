//go:build !windows

package term

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// ptyProc is the unix half of a session: the PTY master and the shell
// started under it.
//
// pty.StartWithSize sets Setsid and Setctty, so the child is a session
// leader and this PTY is its controlling terminal. A job-control shell
// puts a background job in a new process group, so syscall.Kill(-pid, …)
// does not reach that grandchild. Close walks every process that shares
// the shell's controlling-terminal device and signals those.
//
// That walk happens once, at the first signal, and is remembered: it
// stops working exactly when it is needed most. When the session leader
// exits, the kernel revokes the session's controlling terminal, so a walk
// performed after the SIGHUP killed the shell returns nothing and a
// HUP-immune grandchild would never be reached (GDK-950, measured:
// members=[shell grandchild] at SIGHUP, members=[] at SIGKILL). Each
// remembered pid is paired with a start-time stamp so a pid recycled
// during the grace is not signalled in its successor's place. See
// captureMembers for why the walk must not be repeated.
type ptyProc struct {
	f   *os.File
	cmd *exec.Cmd

	waitOnce sync.Once
	code     int
	waitErr  error

	closeOnce sync.Once
	// closed flips true the instant closePTY starts, before the master fd
	// is actually closed. Write and resize consult it after the syscall
	// fails: a failure once the PTY is closed is that close, not a shell
	// problem, so it maps to ErrSessionClosed — the contract every caller
	// checks — instead of a raw os.ErrClosed / EBADF that leaks the fd's
	// state. This is the one place that owns "is the fd gone" (GDK-914);
	// mapping here means Session.Write, Session.Resize, and any future
	// caller of proc.Write/resize get the same answer without repeating it.
	closed atomic.Bool

	// holdMu guards the size the hold loop is defending and whether one is
	// running. One loop per proc, never one per call: a pane dragged across
	// the screen resizes many times a second, and a goroutine each would be
	// a crowd re-applying each other's stale sizes.
	holdMu      sync.Mutex
	holdCols    uint16
	holdRows    uint16
	holdRunning bool

	// resizeExit records which exit the last resize took (GDK-1192): the
	// read-back loop has two silent success paths — matched and trusted-but-
	// unverified — that a CI failure log cannot otherwise tell apart, and the
	// flake under diagnosis is exactly "nil returned, kernel disagrees".
	// atomic.Pointer rather than a mutex: written on every resize, read by
	// Info() from any goroutine, and a one-word store/load is the whole
	// contention story.
	resizeExit atomic.Pointer[string]

	ttyOnce sync.Once
	ttyDev  int32
	ttyOK   bool

	seenMu sync.Mutex
	seen   map[int]uint64 // pid -> start-time stamp
}

func startProc(opts Options) (*ptyProc, error) {
	shell := opts.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, opts.Args...)
	cmd.Dir = opts.Dir
	env := append(os.Environ(), "TERM=xterm-256color", "GADAK_TERMINAL=1")
	cmd.Env = append(env, opts.Env...)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: opts.Cols, Rows: opts.Rows})
	if err != nil {
		return nil, fmt.Errorf("term: start %s: %w", shell, err)
	}
	return &ptyProc{f: f, cmd: cmd}, nil
}

func (p *ptyProc) Read(b []byte) (int, error) { return p.f.Read(b) }

func (p *ptyProc) Write(b []byte) (int, error) {
	n, err := p.f.Write(b)
	if err != nil && p.closed.Load() {
		return n, ErrSessionClosed
	}
	return n, err
}

func (p *ptyProc) pid() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// ptySetsize / ptyGetsize are the TIOCSWINSZ / TIOCGWINSZ seams — vars so
// the EINTR and read-back contracts are testable without a way to make a
// real ioctl misbehave on cue.
var (
	ptySetsize = pty.Setsize
	ptyGetsize = pty.GetsizeFull
)

// resizeReadBackAttempts bounds the set → read-back loop below.
const resizeReadBackAttempts = 5

// resizeHoldSchedule is how long after a verified resize the owner keeps
// asking the kernel whether the size is still there, and the gaps it waits
// between asks. Cumulative 3s.
//
// It exists because the read-back is not durable (GDK-1192). Two macOS CI
// runs, 34969829676 and 35091105299, were recovered from the attempts API
// and say the same thing to the field: `LastResizeExit:matched/1` — the set
// returned nil and TIOCGWINSZ on the same master immediately answered
// 132x43 — and then the master reads the session's *creation* size, 80x24,
// with nothing in this package having written a size in between. This
// package has exactly one writer of the window size, the call above.
//
// The schedule is measured, not chosen: in both runs the child's first
// answer after the resize was already the old size, and the probe interval
// is two seconds, so the revert happens inside two seconds of a set that
// read back correct. Three covers it with room. No theory of why darwin's
// ptmx undoes it is written here — `reverted@…` in the exit string is what
// the next occurrence will say instead of a guess.
var resizeHoldSchedule = []time.Duration{
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	200 * time.Millisecond,
	400 * time.Millisecond,
	800 * time.Millisecond,
	1425 * time.Millisecond,
}

// recordResizeExit names the branch the last resize left by, so the next CI
// recurrence's Info() line says which of the silent exits it took.
func (p *ptyProc) recordResizeExit(s string) {
	p.resizeExit.Store(&s)
}

// lastResizeExit reports the recorded exit, "" when no resize has run yet.
func (p *ptyProc) lastResizeExit() string {
	if s := p.resizeExit.Load(); s != nil {
		return *s
	}
	return ""
}

func (p *ptyProc) resize(cols, rows uint16) error {
	// TIOCSWINSZ rides creack/pty's raw syscall.Syscall, which the runtime
	// does not restart — under load the runtime's own SIGURG preemption can
	// land mid-ioctl and the call returns EINTR with the size NOT applied.
	// The stream handlers deliberately discard Resize errors ("a bad size is
	// the client's bug"), so an unretried EINTR is a silently lost resize:
	// the child answers every later probe with the old size (GDK-1102,
	// fourth recurrence of the resize flake — CI run 33212911928 attempt 1).
	// EINTR means the kernel did not execute the request, so retrying is
	// the contract, exactly as os.File does for its own syscalls.
	//
	// And the ioctl can return success without the size taking: CI run
	// 33736397672 (macOS arm64) applied 132x43, got nil, and TIOCGWINSZ on
	// the same master read 80x24 twenty-eight seconds later while the child
	// kept answering 24 80 (GDK-1192, with the read-back diagnostic that
	// landed one commit earlier). The kernel's answer is the only proof, so
	// the set is verified by reading it back and re-issued until it holds;
	// a size that never holds is an error, not a silent no-op.
	// ponytail: five bounded attempts with a short backoff, not a theory of
	// why darwin's ptmx drops the set — widen if the error line ever shows
	// up in a CI log.
	var err error
	for attempt := 0; ; attempt++ {
		for {
			err = ptySetsize(p.f, &pty.Winsize{Cols: cols, Rows: rows})
			if !errors.Is(err, syscall.EINTR) {
				break
			}
		}
		if err != nil {
			p.recordResizeExit("set-error:" + err.Error())
			break
		}
		got, gerr := ptyGetsize(p.f)
		if gerr != nil {
			// Unverifiable is trusted: a read-back failure is not evidence
			// the set was lost, and refusing here would turn every such pty
			// into an unresizable one.
			p.recordResizeExit("unverified:" + gerr.Error())
			break
		}
		if got.Cols == cols && got.Rows == rows {
			p.recordResizeExit(fmt.Sprintf("matched/%d", attempt+1))
			p.holdSize(cols, rows)
			break
		}
		if attempt+1 >= resizeReadBackAttempts {
			err = fmt.Errorf("term: resize to %dx%d accepted %d times but the pty still reads %dx%d", cols, rows, attempt+1, got.Cols, got.Rows)
			p.recordResizeExit("exhausted")
			break
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	if err != nil && p.closed.Load() {
		return ErrSessionClosed
	}
	return err
}

// holdSize keeps the size that was just verified, for as long as
// resizeHoldSchedule says. A later resize replaces what is being defended
// rather than starting a second loop.
//
// A proc with no master file has nothing to hold: that is the unit tests'
// bare ptyProc, and it is also the honest answer.
func (p *ptyProc) holdSize(cols, rows uint16) {
	if p.f == nil {
		return
	}
	p.holdMu.Lock()
	p.holdCols, p.holdRows = cols, rows
	if p.holdRunning {
		p.holdMu.Unlock()
		return
	}
	p.holdRunning = true
	p.holdMu.Unlock()
	go p.holdLoop()
}

func (p *ptyProc) holdLoop() {
	defer func() {
		p.holdMu.Lock()
		p.holdRunning = false
		p.holdMu.Unlock()
	}()
	started := time.Now()
	for _, gap := range resizeHoldSchedule {
		time.Sleep(gap)
		if p.closed.Load() {
			return
		}
		p.holdMu.Lock()
		cols, rows := p.holdCols, p.holdRows
		p.holdMu.Unlock()
		got, err := ptyGetsize(p.f)
		if err != nil {
			// The same reasoning the read-back loop uses: an unreadable
			// size is not evidence the size is gone.
			return
		}
		if got.Cols == cols && got.Rows == rows {
			continue
		}
		since := time.Since(started).Round(time.Millisecond)
		if err := ptySetsize(p.f, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
			p.recordResizeExit(fmt.Sprintf("reverted@%s→reapply-error:%v", since, err))
			return
		}
		p.recordResizeExit(fmt.Sprintf("reverted@%s→reapplied", since))
	}
}

// winsize reads the size the kernel holds for the pty back from the master
// (TIOCGWINSZ) — the answer that says whether a resize the session believes
// it applied is the one the child's tty carries (GDK-1192 diagnostics).
func (p *ptyProc) winsize() (cols, rows uint16, err error) {
	ws, err := pty.GetsizeFull(p.f)
	if err != nil {
		return 0, 0, err
	}
	return ws.Cols, ws.Rows, nil
}

func (p *ptyProc) hangup() error { return p.signalSession(syscall.SIGHUP) }
func (p *ptyProc) kill() error   { return p.signalSession(syscall.SIGKILL) }

// captureMembers walks the shell's controlling terminal exactly once and
// remembers what it found, with a start-time stamp per pid.
//
// Once, and only at the first signal, is the whole point. The walk is
// trustworthy only while the shell is alive and holding the master, which
// is what makes the device exclusively ours. Afterwards it is worse than
// useless: the kernel revokes the terminal when the session leader exits,
// and the device number is then handed to the next PTY. A second walk
// during the SIGKILL stage therefore returns either nothing or, worse,
// somebody else's session — measured on Linux, where every /dev/pts slot
// reported the same tty_nr and a closing session's re-walk found the
// shell of a session opened moments later (dev=0x8800, closing shell 19,
// walk returned [20]). Killing an unrelated live terminal is a far worse
// defect than the orphan this fix exists to prevent.
//
// The cost of walking once is that a process the shell spawns *during*
// the SIGHUP grace is not seen. That is a corner (the shell has just been
// told to hang up) and the trade is deliberate.
func (p *ptyProc) captureMembers() {
	p.ttyOnce.Do(func() {
		pid := p.pid()
		if pid <= 0 {
			return
		}
		p.ttyDev, p.ttyOK = controllingTty(pid)
		if !p.ttyOK {
			return
		}
		self := os.Getpid()
		members := pidsOnTty(p.ttyDev)
		p.seenMu.Lock()
		p.seen = make(map[int]uint64, len(members))
		for _, m := range members {
			if m <= 0 || m == self {
				continue
			}
			stamp, ok := processStamp(m)
			if !ok {
				stamp = 0
			}
			p.seen[m] = stamp
		}
		p.seenMu.Unlock()
	})
}

func (p *ptyProc) signalSession(sig syscall.Signal) error {
	p.captureMembers()

	p.seenMu.Lock()
	targets := make([]int, 0, len(p.seen))
	for m, stamp := range p.seen {
		// The remembered pid must still be the same incarnation: pids are
		// recycled, and by the SIGKILL stage this one may belong to a
		// stranger. A zero stamp means the platform cannot tell us; signal
		// it anyway, because leaving a known member alive is the defect
		// this closes and there is no better answer on that platform.
		if now, ok := processStamp(m); ok && stamp != 0 && now != stamp {
			delete(p.seen, m)
			continue
		}
		targets = append(targets, m)
	}
	p.seenMu.Unlock()

	sent := false
	for _, m := range targets {
		_ = syscall.Kill(m, sig)
		sent = true
	}
	if !sent {
		return p.signalGroup(sig)
	}
	return nil
}

func (p *ptyProc) signalGroup(sig syscall.Signal) error {
	pid := p.pid()
	if pid <= 0 {
		return nil
	}
	// Negative pid is the process group. Used only when the tty walk
	// returned nothing (no controlling terminal, /proc unreadable).
	if err := syscall.Kill(-pid, sig); err != nil {
		if err == syscall.ESRCH {
			return nil
		}
		return syscall.Kill(pid, sig)
	}
	return nil
}

func (p *ptyProc) wait() (int, error) {
	p.waitOnce.Do(func() {
		err := p.cmd.Wait()
		p.waitErr = err
		if p.cmd.ProcessState != nil {
			p.code = p.cmd.ProcessState.ExitCode()
		} else if err != nil {
			p.code = -1
		}
	})
	return p.code, p.waitErr
}

func (p *ptyProc) closePTY() error {
	var err error
	p.closeOnce.Do(func() {
		// Mark closed before the fd actually goes away, so a Write or
		// resize racing this close sees the flag and maps its failure to
		// ErrSessionClosed (GDK-914).
		p.closed.Store(true)
		err = p.f.Close()
	})
	return err
}
