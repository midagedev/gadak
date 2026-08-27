//go:build !windows

package term

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The session core, pinned against a real PTY. Every contract doc.go
// states has a test here; a shell is cheap and a mock PTY would only pin
// the mock.

// testManager is New with a grace short enough to observe. The grace is
// the one contract a test cannot wait out honestly (60s), so Config takes
// it — the production default lives in DefaultGrace, checked below.
func testManager(t *testing.T, cfg Config) *Manager {
	t.Helper()
	m := New(cfg)
	t.Cleanup(m.CloseAll)
	return m
}

// shellSession starts /bin/sh, which is the one shell every CI image has.
// $SHELL is what production uses; pinning it here would make the test
// depend on the developer's login shell.
func shellSession(t *testing.T, m *Manager, opts Options) *Session {
	t.Helper()
	if opts.Shell == "" {
		opts.Shell = "/bin/sh"
	}
	s, err := m.Create(opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// readUntil collects output until want appears or the deadline passes.
func readUntil(t *testing.T, a *Attachment, want string, within time.Duration) string {
	t.Helper()
	var got bytes.Buffer
	deadline := time.After(within)
	for {
		select {
		case b := <-a.C():
			got.Write(b)
			if strings.Contains(got.String(), want) {
				return got.String()
			}
		case <-deadline:
			t.Fatalf("waiting for %q; got %q", want, got.String())
		}
	}
}

// ① The roundtrip: bytes written to the session come back through an
// attachment. Without this nothing else in the package means anything.
func TestSessionEchoRoundtrip(t *testing.T) {
	m := testManager(t, Config{})
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("echo gadak-roundtrip-ok\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, a, "gadak-roundtrip-ok", 10*time.Second)
}

// ② Env and cwd are the contract, not the shell's defaults: the pane runs
// an agent, and an agent that starts in the wrong directory with no TERM
// is a different product.
func TestSessionEnvAndDir(t *testing.T) {
	dir := t.TempDir()
	// macOS hands out /var symlinks for TempDir; the shell reports the
	// resolved path, so compare resolved to resolved.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := testManager(t, Config{WorkDir: dir})
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("printf 'D=%s T=%s G=%s\\n' \"$(pwd -P)\" \"$TERM\" \"$GADAK_TERMINAL\"\n")); err != nil {
		t.Fatal(err)
	}
	out := readUntil(t, a, "T=xterm-256color", 10*time.Second)
	if !strings.Contains(out, "D="+real) {
		t.Errorf("cwd: %q does not contain D=%s", out, real)
	}
	if !strings.Contains(out, "G=1") {
		t.Errorf("GADAK_TERMINAL: %q", out)
	}
}

// ③ Resize must reach the child, not just the master: the renderer sends
// a size and the program inside has to see it. Asking the child for its
// own tty size is the only proof that matters.
func TestResizeReachesChild(t *testing.T) {
	m := testManager(t, Config{})
	s := shellSession(t, m, Options{Cols: 80, Rows: 24})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("stty size\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, a, "24 80", 10*time.Second)
	if err := s.Resize(132, 43); err != nil {
		t.Fatal(err)
	}
	// Ask the child again rather than trapping WINCH: what has to be true
	// is that the process inside the PTY now reads the new size. (A WINCH
	// trap in /bin/sh only runs between commands, which makes it a test of
	// the shell's trap scheduling, not of the ioctl.)
	if _, err := s.Write([]byte("stty size\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, a, "43 132", 10*time.Second)
	if info := s.Info(); info.Cols != 132 || info.Rows != 43 {
		t.Fatalf("Info after resize: %dx%d; want 132x43", info.Cols, info.Rows)
	}
}

// ④ Close signals the process group, so a grandchild the shell
// backgrounded dies too. Signalling only the shell leaves `sleep &`
// running for its full duration attached to nothing — the orphan class
// this contract exists to close.
func TestCloseKillsProcessGroup(t *testing.T) {
	m := testManager(t, Config{})
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	// The PTY echoes the command line back, so the marker must not appear
	// in the text typed: printf assembles `kid=<pid>` from pieces.
	if _, err := s.Write([]byte("sleep 300 & printf 'k%s=%s\\n' id $!\n")); err != nil {
		t.Fatal(err)
	}
	out := readUntil(t, a, "kid=", 10*time.Second)
	child := grandchildPID(t, out)
	t.Cleanup(func() { killPID(child) })
	if !processAlive(child) {
		t.Fatalf("grandchild %d never started", child)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for processAlive(child) {
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d still alive after Close", child)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := m.Get(s.ID()); err == nil {
		t.Fatal("closed session still listed")
	}
}

// TestCloseKillsHUPImmuneGrandchild is the FAIL-first pin for GDK-950.
// A job-control shell puts `sleep &` in a new process group, so
// kill(-shell, SIGKILL) cannot reach it. bash may re-send SIGHUP to its
// jobs on exit, which hides the leak; a grandchild that ignores SIGHUP
// makes the miss deterministic on every platform.
func TestCloseKillsHUPImmuneGrandchild(t *testing.T) {
	m := testManager(t, Config{})
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("(trap '' HUP; sleep 300) & printf 'k%s=%s\\n' id $!\n")); err != nil {
		t.Fatal(err)
	}
	out := readUntil(t, a, "kid=", 10*time.Second)
	child := grandchildPID(t, out)
	t.Cleanup(func() { killPID(child) })
	if !processAlive(child) {
		t.Fatalf("grandchild %d never started", child)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for processAlive(child) {
		if time.Now().After(deadline) {
			t.Fatalf("HUP-immune grandchild %d still alive after Close", child)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSessionMembers pins the enumerator contract: the shell and a
// background grandchild share a controlling tty, so both appear; a
// zero/-1 device is not a key (that would sweep every daemon).
func TestSessionMembers(t *testing.T) {
	if got := pidsOnTty(0); len(got) != 0 {
		t.Fatalf("pidsOnTty(0) = %v; want empty", got)
	}
	if got := pidsOnTty(-1); len(got) != 0 {
		t.Fatalf("pidsOnTty(-1) = %v; want empty", got)
	}

	m := testManager(t, Config{})
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("(trap '' HUP; sleep 300) & printf 'k%s=%s\\n' id $!\n")); err != nil {
		t.Fatal(err)
	}
	out := readUntil(t, a, "kid=", 10*time.Second)
	child := grandchildPID(t, out)
	t.Cleanup(func() { killPID(child) })
	if !processAlive(child) {
		t.Fatalf("grandchild %d never started", child)
	}

	members := SessionMembers(s.PID())
	if !containsPID(members, s.PID()) {
		t.Fatalf("SessionMembers %v missing shell %d", members, s.PID())
	}
	if !containsPID(members, child) {
		t.Fatalf("SessionMembers %v missing grandchild %d", members, child)
	}
	if got := s.Members(); !containsPID(got, s.PID()) || !containsPID(got, child) {
		t.Fatalf("Members %v missing shell %d or grandchild %d", got, s.PID(), child)
	}
}

func containsPID(pids []int, want int) bool {
	for _, p := range pids {
		if p == want {
			return true
		}
	}
	return false
}

func killPID(pid int) {
	if pid <= 0 || !processAlive(pid) {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// grandchildPID reads the `kid=<pid>` line the shell echoed. The PTY
// adds carriage returns and the echo of the command itself, so take the
// last match.
func grandchildPID(t *testing.T, out string) int {
	t.Helper()
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
		t.Fatalf("no child pid in %q", out)
	}
	return pid
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 tests for existence. A reparented-but-exited process is
	// reaped by init, so this does not see zombies of our own children.
	return p.Signal(syscall.Signal(0)) == nil
}

// ⑤ Backpressure: a reader that never reads is dropped once it is more
// than its channel bound behind, and it never delays a reader that is
// keeping up — the fast reader receives chunk N before chunk N+1 is
// produced, throughout.
//
// The output is driven through the session's own broadcast rather than
// through a shell flood on purpose: "does not delay beyond the bound" is a
// statement about ordering, and a racing shell can only ever produce
// evidence that it happened to be fast enough this time. The PTY half —
// that the pump is alive after a drop — is asserted at the end through the
// real shell.
func TestSlowAttachmentDroppedWithoutStallingOthers(t *testing.T) {
	const bound = 4
	m := testManager(t, Config{AttachBuffer: bound, RingBytes: 4096})
	s := shellSession(t, m, Options{})
	slow, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	fast, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < bound+3; i++ {
		chunk := []byte{byte('a' + i)}
		s.emit(chunk)
		// The fast reader gets this chunk before the next one exists.
		select {
		case got := <-fast.C():
			if !bytes.Equal(got, chunk) {
				t.Fatalf("chunk %d: fast reader got %q, want %q", i, got, chunk)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("chunk %d: the slow attachment delayed the fast one", i)
		}
		if i < bound {
			select {
			case <-slow.Done():
				t.Fatalf("slow attachment dropped at chunk %d, inside its bound of %d", i, bound)
			default:
			}
		}
	}
	select {
	case <-slow.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("slow attachment was never dropped")
	}
	if end := slow.End(); end.Kind != EndDropped || end.Reason != ReasonSlow {
		t.Fatalf("slow end %+v; want dropped/%s", end, ReasonSlow)
	}
	select {
	case <-fast.Done():
		t.Fatalf("fast attachment ended: %+v", fast.End())
	default:
	}
	if info := s.Info(); info.DroppedAttachments != 1 || info.Attached != 1 {
		t.Fatalf("Info %+v; want 1 dropped, 1 attached", info)
	}
	// The PTY was never the thing dropped: the shell still answers.
	if _, err := s.Write([]byte("echo gadak-after-drop\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, fast, "gadak-after-drop", 10*time.Second)
}

// ⑥ Reattaching inside the grace replays the ring: a phone that loses the
// network for ten seconds comes back to its scrollback, not a blank pane.
func TestReattachReplaysRing(t *testing.T) {
	m := testManager(t, Config{Grace: 5 * time.Second})
	s := shellSession(t, m, Options{})
	first, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("echo gadak-scrollback-mark\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, first, "gadak-scrollback-mark", 10*time.Second)
	first.Detach()
	if end := first.End(); end.Kind != EndDetached {
		t.Fatalf("detached end %+v", end)
	}

	second, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-second.C():
		if !strings.Contains(string(b), "gadak-scrollback-mark") {
			t.Fatalf("replay %q missing the mark", b)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no ring replay on reattach")
	}
	// And the session is still live after the reattach.
	if _, err := s.Write([]byte("echo gadak-still-live\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, second, "gadak-still-live", 10*time.Second)
}

// ⑦ The reconnect grace, both directions: an unattached session is reaped
// when it elapses, and reattaching inside it cancels the reap. Grace is
// injected (Config.Grace) so this costs milliseconds, not a minute — the
// production value is asserted separately below.
func TestGraceReapsOnlyUnattachedSessions(t *testing.T) {
	m := testManager(t, Config{Grace: 120 * time.Millisecond})
	reaped := shellSession(t, m, Options{})
	a, err := reaped.Attach()
	if err != nil {
		t.Fatal(err)
	}
	a.Detach()
	select {
	case <-reaped.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("unattached session was not reaped after the grace")
	}
	if _, err := m.Get(reaped.ID()); err == nil {
		t.Fatal("reaped session still in the manager")
	}

	kept := shellSession(t, m, Options{})
	live, err := kept.Attach()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-kept.Done():
		t.Fatal("a session with a live attachment was reaped")
	case <-time.After(500 * time.Millisecond):
	}
	if _, err := kept.Write([]byte("echo gadak-not-reaped\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, live, "gadak-not-reaped", 10*time.Second)
}

// ⑧ The grace default is the product contract (60s), not whatever a test
// happened to inject.
func TestDefaultsAreTheDocumentedContract(t *testing.T) {
	if DefaultGrace != 60*time.Second {
		t.Errorf("DefaultGrace = %v; doc.go and SECURITY.md say 60s", DefaultGrace)
	}
	if DefaultMaxDetachedLife != 24*time.Hour {
		t.Errorf("DefaultMaxDetachedLife = %v; doc.go says 24h", DefaultMaxDetachedLife)
	}
	if DefaultRingBytes != 256<<10 {
		t.Errorf("DefaultRingBytes = %d; the contract is 256 KiB", DefaultRingBytes)
	}
	m := New(Config{})
	if m.cfg.Grace != DefaultGrace || m.cfg.RingBytes != DefaultRingBytes || m.cfg.AttachBuffer != DefaultAttachBuffer || m.cfg.MaxDetachedLife != DefaultMaxDetachedLife {
		t.Fatalf("New(Config{}) did not take the defaults: %+v", m.cfg)
	}
}

// ⑨ Snapshot is the debug surface: the shape a future `gadak terminal
// list` reads, and the fields it must never grow (no output, no token).
func TestSnapshotShape(t *testing.T) {
	m := testManager(t, Config{})
	s := shellSession(t, m, Options{Cols: 100, Rows: 30, TokenID: "deadbeef"})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("echo gadak-snapshot\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, a, "gadak-snapshot", 10*time.Second)

	snap := m.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len %d; want 1", len(snap))
	}
	got := snap[0]
	if got.ID != s.ID() || got.PID <= 0 || got.Cols != 100 || got.Rows != 30 {
		t.Fatalf("Snapshot row %+v", got)
	}
	if got.Attached != 1 || got.BytesOut <= 0 {
		t.Fatalf("Snapshot row %+v; want 1 attached and non-zero bytes_out", got)
	}
	if got.CreatedAt.IsZero() || got.LastOutputAt.IsZero() {
		t.Fatalf("Snapshot row %+v; timestamps unset", got)
	}
	if got.DroppedAttachments != 0 || got.Exited {
		t.Fatalf("Snapshot row %+v", got)
	}
	if !containsPID(got.PIDs, got.PID) {
		t.Fatalf("Snapshot PIDs %v missing shell %d", got.PIDs, got.PID)
	}
	// Session ids are random, not sequential: a second session must not
	// be guessable from the first.
	s2 := shellSession(t, m, Options{})
	if len(s2.ID()) != 32 || s2.ID() == s.ID() {
		t.Fatalf("ids %q and %q are not 128-bit random hex", s.ID(), s2.ID())
	}
}

// ⑩ CloseByToken is the revoke path: only sessions opened with that token
// die, and their clients are told why. A loopback session (empty token id)
// is never matched — revoking a phone's token must not kill the local
// pane.
func TestCloseByTokenCutsOnlyThatToken(t *testing.T) {
	m := testManager(t, Config{})
	paired := shellSession(t, m, Options{TokenID: "tok-a"})
	other := shellSession(t, m, Options{TokenID: "tok-b"})
	local := shellSession(t, m, Options{})
	a, err := paired.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if ids := m.TokenIDs(); len(ids) != 2 || ids[0] != "tok-a" || ids[1] != "tok-b" {
		t.Fatalf("TokenIDs %v; want [tok-a tok-b]", ids)
	}
	if n := m.CloseByToken("tok-a"); n != 1 {
		t.Fatalf("CloseByToken closed %d; want 1", n)
	}
	select {
	case <-a.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("attachment not ended by CloseByToken")
	}
	if end := a.End(); end.Kind != EndClosed || end.Reason != ReasonRevoked {
		t.Fatalf("end %+v; want closed/%s", end, ReasonRevoked)
	}
	if _, err := m.Get(paired.ID()); err == nil {
		t.Fatal("revoked session still live")
	}
	if _, err := m.Get(other.ID()); err != nil {
		t.Fatal("a different token's session was cut")
	}
	if _, err := m.Get(local.ID()); err != nil {
		t.Fatal("a loopback session was cut by a token revoke")
	}
	if n := m.CloseByToken(""); n != 0 {
		t.Fatalf("CloseByToken(\"\") closed %d sessions; must match nothing", n)
	}
}

// ⑪ A shell that exits on its own ends its attachments with its status,
// and leaves the manager.
func TestShellExitEndsAttachments(t *testing.T) {
	m := testManager(t, Config{})
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("exit 7\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("attachment did not end when the shell exited")
	}
	if end := a.End(); end.Kind != EndExited || end.Code != 7 {
		t.Fatalf("end %+v; want exited/7", end)
	}
	if _, err := s.Write([]byte("x\n")); err == nil {
		t.Fatal("write to an exited session succeeded")
	}
	if _, err := m.Get(s.ID()); err == nil {
		t.Fatal("exited session still in the manager")
	}
}

// ⑫ CloseAll is server shutdown: no shell outlives the serve.
func TestCloseAllReapsEverything(t *testing.T) {
	m := New(Config{})
	a := shellSession(t, m, Options{})
	b := shellSession(t, m, Options{})
	m.CloseAll()
	for _, s := range []*Session{a, b} {
		select {
		case <-s.Done():
		case <-time.After(5 * time.Second):
			t.Fatalf("session %s survived CloseAll", s.ID())
		}
	}
	if n := len(m.Snapshot()); n != 0 {
		t.Fatalf("Snapshot after CloseAll: %d rows", n)
	}
	if _, err := m.Create(Options{Shell: "/bin/sh"}); err == nil {
		t.Fatal("Create succeeded after CloseAll")
	}
}

// ⑬ The ring is bounded and replays the tail, not the head: a session that
// printed a megabyte replays the last 256 KiB worth, in order.
func TestRingKeepsTheTail(t *testing.T) {
	r := newRing(8)
	r.write([]byte("abc"))
	if string(r.bytes()) != "abc" {
		t.Fatalf("ring %q", r.bytes())
	}
	r.write([]byte("defghij"))
	if got := string(r.bytes()); got != "cdefghij" {
		t.Fatalf("ring %q; want cdefghij", got)
	}
	r.write([]byte("0123456789"))
	if got := string(r.bytes()); got != "23456789" {
		t.Fatalf("ring %q; want 23456789", got)
	}
	if r.size() != 8 {
		t.Fatalf("ring grew to %d", r.size())
	}
}

// ⑭ A shell that cannot be started is an error, not a half-built session
// the manager then holds forever.
func TestCreateFailureLeavesNoSession(t *testing.T) {
	m := testManager(t, Config{})
	if _, err := m.Create(Options{Shell: filepath.Join(t.TempDir(), "no-such-shell")}); err == nil {
		t.Fatal("Create with a missing shell succeeded")
	}
	if n := len(m.Snapshot()); n != 0 {
		t.Fatalf("Snapshot after a failed Create: %d rows", n)
	}
}

func TestMain(m *testing.M) {
	// `stty` is what TestResizeReachesChild proves the child saw. Skipping
	// silently would leave the resize contract unpinned, so say it.
	if _, err := exec.LookPath("stty"); err != nil {
		os.Stderr.WriteString("term tests: stty not found; the resize contract cannot be pinned here\n")
	}
	os.Exit(m.Run())
}

// TestMemberWalkHappensOnceAndCannotCatchALaterSession pins the invariant
// the first GDK-950 fix got wrong: the controlling-terminal walk must
// happen once, while the shell is alive and owns the device, and never
// again.
//
// Repeating it at the SIGKILL stage looks harmless and is not. The kernel
// revokes the terminal when the session leader exits and hands the device
// number to the next PTY, so the second walk returns either nothing or
// another session's shell — which then gets SIGKILLed. Measured on Linux,
// where every /dev/pts slot reported tty_nr 0x8800: closing shell 19
// re-walked and found shell 20, a session opened moments later.
//
// The sequence below is that race made deterministic: capture, release the
// device, let another session take it, signal again. Signal 0 is used
// because only the target set is under test, not the delivery. On a
// platform that does not reuse the device the assertion is simply always
// true; the pin is meaningful where the reuse is real.
func TestMemberWalkHappensOnceAndCannotCatchALaterSession(t *testing.T) {
	m := testManager(t, Config{})
	first := shellSession(t, m, Options{})
	firstProc := first.proc

	// Capture while the shell is alive: this is the only trustworthy walk.
	if err := firstProc.signalSession(syscall.Signal(0)); err != nil {
		t.Fatal(err)
	}
	firstProc.seenMu.Lock()
	captured := len(firstProc.seen)
	firstProc.seenMu.Unlock()
	if captured == 0 {
		t.Skip("no controlling-terminal members visible on this platform")
	}

	// Release the device so the next PTY can be handed the same number.
	_ = first.Close()

	second := shellSession(t, m, Options{})
	secondPID := second.PID()
	if secondPID <= 0 {
		t.Fatal("second session has no pid")
	}

	// A second walk here is what the regression did. It must not happen.
	if err := firstProc.signalSession(syscall.Signal(0)); err != nil {
		t.Fatal(err)
	}
	firstProc.seenMu.Lock()
	_, caught := firstProc.seen[secondPID]
	members := make([]int, 0, len(firstProc.seen))
	for pid := range firstProc.seen {
		members = append(members, pid)
	}
	firstProc.seenMu.Unlock()
	if caught {
		t.Fatalf("the closed session's member set %v picked up shell %d of a later session", members, secondPID)
	}
}

// ⑫ Work outlives the tab. A detached session whose shell still holds a
// process — an agent, a build, an editor — is not idle, and the reconnect
// grace must not reap it. Nothing else on the machine knows that work is
// there: the shell is its only parent (GDK-994).
func TestGraceSparesADetachedSessionThatIsStillWorking(t *testing.T) {
	m := testManager(t, Config{Grace: 150 * time.Millisecond})
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	// A long job, started the way a user starts one and left running.
	if _, err := s.Write([]byte("sleep 300 & printf 'k%s=%s\\n' id $!\n")); err != nil {
		t.Fatal(err)
	}
	out := readUntil(t, a, "kid=", 10*time.Second)
	child := grandchildPID(t, out)
	t.Cleanup(func() { killPID(child) })

	a.Detach() // the tab closes, the laptop sleeps, the phone backgrounds

	select {
	case <-s.Done():
		t.Fatal("a detached session with a live child was reaped: the work is gone")
	case <-time.After(1 * time.Second):
	}
	if !processAlive(child) {
		t.Fatalf("child %d was killed with the session", child)
	}
	// And it can be picked up again, with its scrollback.
	back, err := s.Attach()
	if err != nil {
		t.Fatalf("reattach after the grace: %v", err)
	}
	if _, err := s.Write([]byte("echo gadak-work-survived\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, back, "gadak-work-survived", 10*time.Second)
}

// ⑬ The ceiling on ⑫. Work keeps re-arming the grace, but not forever:
// past MaxDetachedLife an unattended session is closed however busy it
// looks, so one stray `sleep infinity` cannot hold a shell for the life
// of the serve.
func TestDetachedLifeIsCapped(t *testing.T) {
	m := testManager(t, Config{Grace: 60 * time.Millisecond, MaxDetachedLife: 400 * time.Millisecond})
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("sleep 300 & printf 'k%s=%s\\n' id $!\n")); err != nil {
		t.Fatal(err)
	}
	out := readUntil(t, a, "kid=", 10*time.Second)
	child := grandchildPID(t, out)
	t.Cleanup(func() { killPID(child) })

	a.Detach()
	select {
	case <-s.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("a busy session outlived MaxDetachedLife")
	}
	// It got there by sparing first, not by ignoring the work.
	if got := s.Info().GraceExtensions; got == 0 {
		t.Fatal("the session was reaped at the first grace; the cap, not the work check, should have ended it")
	}
}

// ⑭ The second signal on its own. A shell that is still talking is not
// idle either, even with nothing but itself on the terminal — the member
// walk is one of two answers, not the only one.
func TestOutputDuringTheGraceSparesTheSession(t *testing.T) {
	m := testManager(t, Config{Grace: time.Hour}) // the timer must not fire; we call reap's owner directly
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	a.Detach()
	if got := s.idleForReap(); got != reapClose {
		t.Fatalf("a quiet detached shell: idleForReap = %v; want reapClose", got)
	}
	s.emit([]byte("still here\n"))
	if got := s.idleForReap(); got != reapSpare {
		t.Fatalf("output arrived inside the grace: idleForReap = %v; want reapSpare", got)
	}
	// And an attachment beats both signals.
	if _, err := s.Attach(); err != nil {
		t.Fatal(err)
	}
	if got := s.idleForReap(); got != reapGone {
		t.Fatalf("reattached: idleForReap = %v; want reapGone", got)
	}
}

// ⑮ The window ⑫ opened, closed. reap now walks the process table with
// the lock released, so an Attach can land between "not idle" and the
// kill. The claim is the arbiter: with something attached it refuses, and
// once it succeeds the session takes no new attachments — a client either
// wins outright or is told the session is closed, never handed a pane
// that dies under it.
func TestReapClaimAndAttachCannotBothWin(t *testing.T) {
	m := testManager(t, Config{Grace: time.Hour}) // the timer must not fire
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	if got := s.claimClose(ReasonReaped, true); got != closeSkip {
		t.Fatalf("claim with a live attachment = %v; want closeSkip", got)
	}
	a.Detach()
	if got := s.claimClose(ReasonReaped, true); got != closeOwned {
		t.Fatalf("claim on an idle session = %v; want closeOwned", got)
	}
	if _, err := s.Attach(); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("Attach on a claimed session: err = %v; want ErrSessionClosed", err)
	}
	s.terminate()
	<-s.Done()
}
