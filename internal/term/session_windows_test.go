//go:build windows

package term

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The ConPTY contract, pinned against a real pseudoconsole. These tests
// are the only place this package's Windows half executes — the dev
// machines are macOS, so CI's Desktop Windows job runs them (GDK-891).
// Every test starts through startProc or Manager.Create, which on the
// pre-GDK-891 stub failed with ErrUnsupportedPlatform at the first
// assertion: that stub is the FAIL-first these tests were red against.

// testWindowsManager is New with a grace short enough to observe. The
// grace is the one contract a test cannot wait out honestly (60s), so
// Config takes it.
func testWindowsManager(t *testing.T) *Manager {
	t.Helper()
	m := New(Config{Grace: 2 * time.Second})
	t.Cleanup(m.CloseAll)
	return m
}

// comspec names the one shell every Windows image has: $SHELL's Windows
// counterpart, cmd.exe when nothing says otherwise. Pinning the test to
// COMSPEC rather than a developer's setting is the same discipline the
// unix half pins to /bin/sh.
func comspec() string {
	if s := os.Getenv("COMSPEC"); s != "" {
		return s
	}
	return "cmd.exe"
}

// comspecSession starts that shell under a real session.
func comspecSession(t *testing.T, m *Manager, opts Options) *Session {
	t.Helper()
	if opts.Shell == "" {
		opts.Shell = comspec()
	}
	s, err := m.Create(opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// readUntil collects output until want appears or the deadline passes.
// On Done it drains once before failing: a backlog pending at the end is
// still readable through Take, so a marker that arrived in the last chunk
// must not be reported missing.
func readUntil(t *testing.T, a *Attachment, want string, within time.Duration) string {
	t.Helper()
	var got strings.Builder
	deadline := time.After(within)
	for {
		select {
		case <-a.Wake():
			got.Write(a.Take())
			if strings.Contains(got.String(), want) {
				return got.String()
			}
		case <-a.Done():
			got.Write(a.Take())
			if strings.Contains(got.String(), want) {
				return got.String()
			}
			t.Fatalf("waiting for %q; attachment ended with %q", want, got.String())
		case <-deadline:
			t.Fatalf("waiting for %q; got %q", want, got.String())
		}
	}
}

// drainFor collects output until the stream has been quiet for the whole
// gap — the "clear the banner and the echoed input" helper the phases
// below run between their commands.
func drainFor(t *testing.T, a *Attachment, gap time.Duration) string {
	t.Helper()
	var got strings.Builder
	quiet := time.NewTimer(gap)
	defer quiet.Stop()
	for {
		select {
		case <-a.Wake():
			got.Write(a.Take())
			quiet.Reset(gap)
		case <-a.Done():
			got.Write(a.Take())
			return got.String()
		case <-quiet.C:
			return got.String()
		}
	}
}

// ① The roundtrip: a command this test typed comes back as its output.
// The value is assembled from two variables so the marker exists in that
// form only in the shell's OUTPUT — the pane echoes the raw input lines,
// and a marker that appears in them proves nothing about the direction of
// the pipe.
func TestSessionEchoRoundtrip(t *testing.T) {
	m := testWindowsManager(t)
	s := comspecSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	for _, line := range []string{
		"set gadak_probe_a=con_pty_",
		"set gadak_probe_b=roundtrip",
		"echo %gadak_probe_a%%gadak_probe_b%",
	} {
		if _, err := s.Write([]byte(line + "\r")); err != nil {
			t.Fatalf("Write(%q): %v", line, err)
		}
	}
	readUntil(t, a, "con_pty_roundtrip", 20*time.Second)
}

// ② resize, answered by the child itself: `mode con` prints the console
// size the shell sees, which on a healthy pseudoconsole is the size
// Resize just applied — the same axis the unix half pins with stty size.
// winsize() is along for the record it keeps, which is all it can keep:
// ConPTY has no read-back, so the remembered size is the pinned contract
// and the child's answer is the evidence.
func TestResizeChildAnswers(t *testing.T) {
	m := testWindowsManager(t)
	s := comspecSession(t, m, Options{Cols: 80, Rows: 24})
	a, err := s.Attach()
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	cols, rows, err := s.TTYSize()
	if err != nil || cols != 80 || rows != 24 {
		t.Fatalf("initial winsize = %dx%d, %v; want 80x24", cols, rows, err)
	}
	drainFor(t, a, 500*time.Millisecond)
	if err := s.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if _, err := s.Write([]byte("mode con\r")); err != nil {
		t.Fatalf("Write(mode con): %v", err)
	}
	out := readUntil(t, a, "Columns:", 20*time.Second)
	colRe := regexp.MustCompile(`(?im)^\s*Columns:\s+(\d+)`)
	lineRe := regexp.MustCompile(`(?im)^\s*Lines:\s+(\d+)`)
	cm, lm := colRe.FindStringSubmatch(out), lineRe.FindStringSubmatch(out)
	if cm == nil || lm == nil {
		t.Fatalf("mode con output had no Columns/Lines pair:\n%s", out)
	}
	gotCols, _ := strconv.Atoi(cm[1])
	gotRows, _ := strconv.Atoi(lm[1])
	if gotCols != 120 || gotRows != 40 {
		t.Fatalf("child reports %dx%d after Resize(120,40);\nmode con output:\n%s", gotCols, gotRows, out)
	}
	if cols, rows, err = s.TTYSize(); err != nil || cols != 120 || rows != 40 {
		t.Fatalf("winsize after resize = %dx%d, %v; want 120x40", cols, rows, err)
	}
}

// ③ The round's core contract: kill takes the child's child with it. The
// unix half pins a HUP-immune grandchild; the Windows machine is one Job
// Object, so the axis is the job's own member list, before (the
// grandchild must be a member — spawned by the shell, no breakaway) and
// after (nobody), with wait() answering in between.
func TestKillTakesTheTreeWithIt(t *testing.T) {
	p, err := startProc(Options{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("startProc: %v", err)
	}
	t.Cleanup(func() { _ = p.closePTY() })
	if _, err := p.Write([]byte("start /b ping -n 60 127.0.0.1\r")); err != nil {
		t.Fatalf("Write(start): %v", err)
	}
	var members []int
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if members = membersOf(p.pid()); len(members) >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(members) < 2 {
		t.Fatalf("grandchild never joined the job; members=%v", members)
	}
	if err := p.kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	waited := make(chan error, 1)
	go func() {
		_, werr := p.wait()
		waited <- werr
	}()
	select {
	case werr := <-waited:
		if werr != nil {
			t.Fatalf("wait after kill: %v", werr)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("wait did not return after kill")
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if members = membersOf(p.pid()); len(members) == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("job still has members after kill: %v", members)
}

// ④ closePTY is safe to run twice, and after hangup: the pseudoconsole
// close is shared between them, so the ladder hangup → closePTY →
// closePTY must not double-close any handle — on Windows a double close
// can land on a recycled handle value, which is somebody else's live
// object.
func TestClosePTYTwiceAfterHangup(t *testing.T) {
	p, err := startProc(Options{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("startProc: %v", err)
	}
	if err := p.hangup(); err != nil {
		t.Fatalf("hangup: %v", err)
	}
	if err := p.closePTY(); err != nil {
		t.Fatalf("first closePTY: %v", err)
	}
	if err := p.closePTY(); err != nil {
		t.Fatalf("second closePTY: %v", err)
	}
	if err := p.hangup(); err != nil {
		t.Fatalf("hangup after close: %v", err)
	}
}
