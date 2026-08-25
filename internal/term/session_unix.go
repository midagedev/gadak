//go:build !windows

package term

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

// ptyProc is the unix half of a session: the PTY master, the shell, and
// the process group both belong to.
//
// pty.StartWithSize sets Setsid and Setctty, so the child is a session
// leader and its pid is its process-group id. That is what makes
// `syscall.Kill(-pid, …)` reach a grandchild the shell backgrounded —
// signalling the shell alone would leave `sleep 300 &` orphaned.
type ptyProc struct {
	f   *os.File
	cmd *exec.Cmd

	waitOnce sync.Once
	code     int
	waitErr  error

	closeOnce sync.Once
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

func (p *ptyProc) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *ptyProc) Write(b []byte) (int, error) { return p.f.Write(b) }

func (p *ptyProc) pid() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *ptyProc) resize(cols, rows uint16) error {
	return pty.Setsize(p.f, &pty.Winsize{Cols: cols, Rows: rows})
}

func (p *ptyProc) hangup() error { return p.signalGroup(syscall.SIGHUP) }
func (p *ptyProc) kill() error   { return p.signalGroup(syscall.SIGKILL) }

func (p *ptyProc) signalGroup(sig syscall.Signal) error {
	pid := p.pid()
	if pid <= 0 {
		return nil
	}
	// Negative pid is the process group. Fall back to the process itself
	// if the group is already gone, so a partially reaped session still
	// gets its signal.
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
	p.closeOnce.Do(func() { err = p.f.Close() })
	return err
}
