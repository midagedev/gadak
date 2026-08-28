//go:build !windows

package main

// The GoStream terminal protocol (GDK-892), pinned against a real PTY.
//
// internal/term/session_unix_test.go is the pattern: a shell is cheap and a
// mock PTY would only pin the mock. What is mocked here is the *connection* —
// a live *application.StreamConn needs a webview and a held poll to exist,
// and none of that is what this file is about. serveTerminalStream takes the
// narrow termStreamConn interface for exactly that reason; the compile-time
// assertion in terminal_stream.go is what keeps the real type on it.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/term"
)

// ⓪ The webview's requests must reach the API as what they are: in-process
// calls with no network peer. wails stamps a synthetic RFC 5737 TEST-NET peer
// on every one of them, and internal/server's terminal gate reads the peer as
// well as the Host (GDK-863) — so under that stamp the app looked exactly
// like the remote client the gate exists to refuse, and every terminal
// request in Gadak.app was answered forbidden_host without a log line.
//
// 2026-08-25 — GDK-892, measured, not read: the round's own effect
// confirmation found the pane still saying `terminal.unavailable` with no
// shell process spawned, which is the REST create failing, not the transport.
// FAIL-first: without normalizeWebviewPeer this test sees "192.0.2.1:1234".
//
// Pinned on the peer the API actually receives rather than on a gate verdict,
// because the gate is internal/server's contract and this is the desktop's
// half of it: present the connection honestly.
func TestWebviewRequestsReachTheAPIAsInProcess(t *testing.T) {
	var got []*http.Request
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r)
		w.WriteHeader(http.StatusNoContent)
	})
	h := fallbackHandler(api, nil, nil, nil, newBrowseTabs(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/terminal/sessions/", nil)
	// What the webview really sends, both parts of it.
	req.Host = "wails.localhost"
	req.Header.Set("Origin", "wails://wails.localhost")
	req.RemoteAddr = "192.0.2.1:1234"
	h.ServeHTTP(httptest.NewRecorder(), req)

	if len(got) != 1 {
		t.Fatalf("the API saw %d requests; want 1", len(got))
	}
	seen := got[0]
	if seen.RemoteAddr != "" {
		t.Errorf("RemoteAddr = %q; want empty — a synthetic peer makes the app "+
			"indistinguishable from a remote client and the terminal gate refuses it",
			seen.RemoteAddr)
	}
	if seen.Host != "127.0.0.1" {
		t.Errorf("Host = %q; want 127.0.0.1", seen.Host)
	}
	if o := seen.Header.Get("Origin"); o != "" {
		t.Errorf("Origin = %q; want it deleted", o)
	}
}

const testTimeout = 10 * time.Second

// fakeConn is a termStreamConn backed by two channels. Send never blocks
// within the depth used here, which is the point: the blocking-Send
// backpressure contract belongs to internal/term and is pinned there.
type fakeConn struct {
	in     chan []byte
	out    chan []byte
	ctx    context.Context
	cancel context.CancelFunc
}

func newFakeConn() *fakeConn {
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeConn{
		in:     make(chan []byte, 64),
		out:    make(chan []byte, 1024),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (f *fakeConn) Send(data []byte) error {
	select {
	case f.out <- data:
		return nil
	case <-f.ctx.Done():
		return context.Canceled
	}
}

func (f *fakeConn) Receive() ([]byte, error) {
	select {
	case frame := <-f.in:
		return frame, nil
	case <-f.ctx.Done():
		return nil, context.Canceled
	}
}

func (f *fakeConn) Context() context.Context { return f.ctx }

// write queues a client frame. The tag is the caller's, so a test can send a
// deliberately malformed first frame.
func (f *fakeConn) write(frame []byte) { f.in <- frame }

func (f *fakeConn) writeCtrl(t *testing.T, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %v: %v", v, err)
	}
	f.write(append([]byte{termFrameCtrl}, data...))
}

func (f *fakeConn) writeData(p []byte) { f.write(append([]byte{termFrameData}, p...)) }

// serve runs the handler on its own goroutine and reports when it returned,
// so "the handler returned" is an assertable event rather than a sleep.
func (f *fakeConn) serve(mgr *term.Manager) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		serveTerminalStream(f, mgr)
	}()
	return done
}

// newTestTermManager is the production shape (New(Config{})) with cleanup.
// The 60 s reconnect grace is never waited out here; ⑥ asserts the session is
// still present, which is true from the instant the connection ends.
func newTestTermManager(t *testing.T) *term.Manager {
	t.Helper()
	m := term.New(term.Config{})
	t.Cleanup(m.CloseAll)
	return m
}

// newTestShell starts /bin/sh, the one shell every CI image has. $SHELL is
// what production uses; pinning it here would make the test depend on the
// developer's login shell.
func newTestShell(t *testing.T, m *term.Manager, cols, rows uint16) *term.Session {
	t.Helper()
	s, err := m.Create(term.Options{Shell: "/bin/sh", Cols: cols, Rows: rows})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// attachOK sends the opening attach frame and waits for the attached ack —
// the frame the frontend turns into "I am talking to a shell".
func attachOK(t *testing.T, f *fakeConn, id string) {
	t.Helper()
	f.writeCtrl(t, map[string]any{"t": "attach", "id": id})
	msg := f.nextCtrl(t)
	if msg["t"] != "attached" {
		t.Fatalf("first control frame %v; want attached", msg)
	}
}

// nextCtrl reads frames until a control frame arrives, ignoring PTY output.
func (f *fakeConn) nextCtrl(t *testing.T) map[string]any {
	t.Helper()
	deadline := time.After(testTimeout)
	for {
		select {
		case frame := <-f.out:
			if len(frame) == 0 {
				t.Fatal("empty frame on the wire")
			}
			if frame[0] != termFrameCtrl {
				continue
			}
			var msg map[string]any
			if err := json.Unmarshal(frame[1:], &msg); err != nil {
				t.Fatalf("control frame %q: %v", frame[1:], err)
			}
			return msg
		case <-deadline:
			t.Fatal("timed out waiting for a control frame")
		}
	}
}

// dataUntil collects tagged PTY output until want appears. It also asserts
// the tag on every output frame, which is the "output arrives tagged 0x00"
// contract — there is no other frame shape that could carry it.
func (f *fakeConn) dataUntil(t *testing.T, want string) string {
	t.Helper()
	var got bytes.Buffer
	deadline := time.After(testTimeout)
	for {
		select {
		case frame := <-f.out:
			if len(frame) == 0 {
				t.Fatal("empty frame on the wire")
			}
			if frame[0] == termFrameCtrl {
				t.Fatalf("unexpected control frame while waiting for %q: %s", want, frame[1:])
			}
			if frame[0] != termFrameData {
				t.Fatalf("frame tag %#x; want %#x or %#x", frame[0], termFrameData, termFrameCtrl)
			}
			got.Write(frame[1:])
			if strings.Contains(got.String(), want) {
				return got.String()
			}
		case <-deadline:
			t.Fatalf("waiting for %q; got %q", want, got.String())
		}
	}
}

// ① A connection that does not open with a valid attach is a protocol error,
// and the handler returns rather than waiting for a better frame. Without
// this a malformed page could hold a handler goroutine open indefinitely.
func TestTerminalStreamFirstFrameMustBeAttach(t *testing.T) {
	mgr := newTestTermManager(t)
	for _, tc := range []struct {
		name  string
		frame []byte
	}{
		{"data frame", append([]byte{termFrameData}, []byte("hello")...)},
		{"untagged", []byte("{\"t\":\"attach\"}")},
		{"not json", append([]byte{termFrameCtrl}, []byte("nonsense")...)},
		{"wrong type", append([]byte{termFrameCtrl}, []byte(`{"t":"resize","cols":80,"rows":24}`)...)},
		{"attach without id", append([]byte{termFrameCtrl}, []byte(`{"t":"attach"}`)...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := newFakeConn()
			done := conn.serve(mgr)
			conn.write(tc.frame)
			msg := conn.nextCtrl(t)
			if msg["t"] != "error" || msg["code"] != termErrProtocol {
				t.Fatalf("got %v; want an error/%s control frame", msg, termErrProtocol)
			}
			select {
			case <-done:
			case <-time.After(testTimeout):
				t.Fatal("handler did not return after a protocol error")
			}
		})
	}
}

// ② An id the manager does not hold is not_found — the same answer
// handleTerminalWS gives (a 404 on the upgrade), because the pane's response
// to both is to drop the kept id and create a new session.
func TestTerminalStreamUnknownSession(t *testing.T) {
	mgr := newTestTermManager(t)
	conn := newFakeConn()
	done := conn.serve(mgr)
	conn.writeCtrl(t, map[string]any{"t": "attach", "id": "no-such-session"})
	msg := conn.nextCtrl(t)
	if msg["t"] != "error" || msg["code"] != termErrNotFound {
		t.Fatalf("got %v; want an error/%s control frame", msg, termErrNotFound)
	}
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("handler did not return after not_found")
	}
}

// ③ The roundtrip: a 0x00 frame reaches the shell's stdin and its output
// comes back tagged 0x00. Without this nothing else here means anything.
func TestTerminalStreamDataRoundtrip(t *testing.T) {
	mgr := newTestTermManager(t)
	sess := newTestShell(t, mgr, 80, 24)
	conn := newFakeConn()
	conn.serve(mgr)
	attachOK(t, conn, sess.ID())

	conn.writeData([]byte("printf 'gdk892-%s\\n' ok\n"))
	conn.dataUntil(t, "gdk892-ok")
}

// sizeUntil keeps asking the child for its own tty size until want shows up
// in the data stream. One `stty size` plus dataUntil's fixed wall-clock
// deadline is the shape that flaked under CI load (GDK-977/1007/1071): the
// deadline was a bet on scheduling, not on the contract. The question is
// idempotent — the resize ioctl is synchronous, so once it has been applied
// every later probe must answer with the new size — which makes re-asking
// the state-based wait. The outer deadline only guards a size that truly
// never arrives (a real defect, not load).
func (f *fakeConn) sizeUntil(t *testing.T, want string, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	var got bytes.Buffer
	for {
		f.writeData([]byte("stty size\n"))
		probe := time.After(2 * time.Second)
	drain:
		for {
			select {
			case frame := <-f.out:
				if len(frame) == 0 {
					t.Fatal("empty frame on the wire")
				}
				if frame[0] == termFrameCtrl {
					t.Fatalf("unexpected control frame while waiting for %q: %s", want, frame[1:])
				}
				if frame[0] != termFrameData {
					t.Fatalf("frame tag %#x; want %#x", frame[0], termFrameData)
				}
				got.Write(frame[1:])
				if strings.Contains(got.String(), want) {
					return
				}
			case <-probe:
				break drain
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting for tty size %q; got %q", want, got.String())
		}
	}
}

// ④ A resize control frame changes the size the *child* sees, not just the
// master. Asking the child for its own tty size is the only proof that
// matters (the same argument internal/term's TestResizeReachesChild makes).
// Asking again rather than trapping WINCH is deliberate: a WINCH trap in
// /bin/sh only runs between commands, which would test the shell's trap
// scheduling, not the ioctl.
func TestTerminalStreamResizeReachesChild(t *testing.T) {
	if _, err := exec.LookPath("stty"); err != nil {
		t.Skip("stty not found; the resize contract cannot be pinned here")
	}
	mgr := newTestTermManager(t)
	sess := newTestShell(t, mgr, 80, 24)
	conn := newFakeConn()
	conn.serve(mgr)
	attachOK(t, conn, sess.ID())

	conn.sizeUntil(t, "24 80", 30*time.Second)

	conn.writeCtrl(t, map[string]any{"t": "resize", "cols": 132, "rows": 43})
	conn.sizeUntil(t, "43 132", 30*time.Second)
	if info := sess.Info(); info.Cols != 132 || info.Rows != 43 {
		t.Fatalf("Info after resize: %dx%d; want 132x43", info.Cols, info.Rows)
	}
}

// ⑤ The shell exiting produces the exit control frame carrying its status,
// which is the frame the pane turns into "exited (7)".
func TestTerminalStreamExitCarriesCode(t *testing.T) {
	mgr := newTestTermManager(t)
	sess := newTestShell(t, mgr, 80, 24)
	conn := newFakeConn()
	done := conn.serve(mgr)
	attachOK(t, conn, sess.ID())

	conn.writeData([]byte("exit 7\n"))
	msg := conn.nextCtrl(t)
	if msg["t"] != "exit" {
		t.Fatalf("got %v; want an exit control frame", msg)
	}
	code, ok := msg["code"].(float64)
	if !ok || int(code) != 7 {
		t.Fatalf("exit code %v; want 7", msg["code"])
	}
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("handler did not return after the shell exited")
	}
}

// ⑥ The connection ending detaches; it never closes the session. This is the
// contract that makes closing the pane survivable: the shell keeps running
// for the reconnect grace, so a reopen inside it replays the ring. A
// sess.Close() here instead of att.Detach() would pass every test above and
// silently kill the user's shell on a window reload.
func TestTerminalStreamSessionSurvivesConnectionEnd(t *testing.T) {
	mgr := newTestTermManager(t)
	sess := newTestShell(t, mgr, 80, 24)
	conn := newFakeConn()
	done := conn.serve(mgr)
	attachOK(t, conn, sess.ID())

	conn.writeData([]byte("printf 'gdk892-%s\\n' alive\n"))
	conn.dataUntil(t, "gdk892-alive")

	// The page navigated away: wails cancels the connection's context.
	conn.cancel()
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("handler did not return when the connection ended")
	}

	got, err := mgr.Get(sess.ID())
	if err != nil {
		t.Fatalf("Get after the connection ended: %v; want the session still there", err)
	}
	if got.ID() != sess.ID() {
		t.Fatalf("Get returned %q; want %q", got.ID(), sess.ID())
	}
	if info := got.Info(); info.Exited {
		t.Fatal("the shell exited when the connection ended; it must survive the grace")
	}
	// And it is still usable: a second connection reattaches to the same
	// shell, which is what a reopened pane does.
	second := newFakeConn()
	second.serve(mgr)
	attachOK(t, second, sess.ID())
	second.writeData([]byte("printf 'gdk892-%s\\n' reattached\n"))
	second.dataUntil(t, "gdk892-reattached")
}
