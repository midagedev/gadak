//go:build !windows

// Command term-leak-probe measures whether a gadak PTY session leaves a
// SIGHUP-immune grandchild alive after Close.
//
//	go run ./tools/term-leak-probe [shell [n]]
//
// Default shell is /bin/sh, default n is 20. Each iteration starts a
// session, backgrounds `(trap ” HUP; sleep 300)`, closes, and checks
// whether the grandchild is gone. Survivors are SIGKILL'd so the probe
// itself does not leak.
package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/midagedev/gadak/internal/term"
)

func main() {
	shell := "/bin/sh"
	n := 20
	args := os.Args[1:]
	if len(args) >= 1 {
		if args[0] == "-h" || args[0] == "--help" {
			fmt.Fprintf(os.Stderr, "usage: term-leak-probe [shell [n]]\n")
			os.Exit(2)
		}
		shell = args[0]
	}
	if len(args) >= 2 {
		v, err := strconv.Atoi(args[1])
		if err != nil || v < 1 {
			fmt.Fprintf(os.Stderr, "term-leak-probe: n must be a positive integer\n")
			os.Exit(2)
		}
		n = v
	}

	leaked := 0
	for i := 0; i < n; i++ {
		if err := one(shell); err != nil {
			fmt.Fprintf(os.Stderr, "term-leak-probe: iter %d: %v\n", i+1, err)
			leaked++
		}
	}
	fmt.Printf("shell=%s n=%d leaked=%d/%d\n", shell, n, leaked, n)
	if leaked > 0 {
		os.Exit(1)
	}
}

func one(shell string) error {
	m := term.New(term.Config{Grace: time.Hour})
	defer m.CloseAll()
	s, err := m.Create(term.Options{Shell: shell})
	if err != nil {
		return err
	}
	a, err := s.Attach()
	if err != nil {
		return err
	}
	if _, err := s.Write([]byte("(trap '' HUP; sleep 300) & printf 'k%s=%s\\n' id $!\n")); err != nil {
		return err
	}
	out, err := readUntil(a, "kid=", 10*time.Second)
	if err != nil {
		return err
	}
	child, err := grandchildPID(out)
	if err != nil {
		return err
	}
	defer killPID(child)
	if !processAlive(child) {
		return fmt.Errorf("grandchild %d never started", child)
	}
	if err := s.Close(); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	if processAlive(child) {
		return fmt.Errorf("grandchild %d still alive after Close", child)
	}
	return nil
}

func readUntil(a *term.Attachment, want string, within time.Duration) (string, error) {
	var got bytes.Buffer
	deadline := time.After(within)
	for {
		select {
		case <-a.Wake():
			got.Write(a.Take())
			if strings.Contains(got.String(), want) {
				return got.String(), nil
			}
		case <-a.Done():
			// A backlog pending at the end stays readable; drain it before
			// declaring failure (internal/term).
			got.Write(a.Take())
			if strings.Contains(got.String(), want) {
				return got.String(), nil
			}
			return "", fmt.Errorf("waiting for %q; attachment ended; got %q", want, got.String())
		case <-deadline:
			got.Write(a.Take())
			if strings.Contains(got.String(), want) {
				return got.String(), nil
			}
			return "", fmt.Errorf("waiting for %q; got %q", want, got.String())
		}
	}
}

func grandchildPID(out string) (int, error) {
	pid := 0
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", "\n"), "\n") {
		_, rest, ok := strings.Cut(strings.TrimSpace(line), "kid=")
		if !ok || rest == "" {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil && n > 0 {
			pid = n
		}
	}
	if pid == 0 {
		return 0, fmt.Errorf("no child pid in %q", out)
	}
	return pid, nil
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func killPID(pid int) {
	if pid <= 0 || !processAlive(pid) {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
