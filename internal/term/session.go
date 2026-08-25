package term

import (
	"errors"
	"sync"
	"time"
)

// Why an attachment ended, as the socket reports it to its client.
const (
	// ReasonSlow: the client fell further behind than its channel bound.
	// The attachment is dropped; the PTY is never stalled for it.
	ReasonSlow = "slow_client"
	// ReasonRevoked: the pairing token this session was opened with is no
	// longer active.
	ReasonRevoked = "token_revoked"
	// ReasonReaped: nothing was attached for the reconnect grace.
	ReasonReaped = "idle_timeout"
	// ReasonShutdown: the serve is going away.
	ReasonShutdown = "server_shutdown"
	// ReasonClosed: an explicit DELETE or Close.
	ReasonClosed = "closed"
)

// EndKind is why an Attachment stopped.
type EndKind int

const (
	// EndDetached: this client let go. The session may still be running.
	EndDetached EndKind = iota
	// EndExited: the shell exited; Code is its status.
	EndExited
	// EndDropped: backpressure. Reason says which.
	EndDropped
	// EndClosed: the session was closed out from under the client.
	// Reason says why (revoked, reaped, shutdown, explicit close).
	EndClosed
)

// End is the terminal event of one attachment.
type End struct {
	Kind   EndKind
	Code   int
	Reason string
}

// Attachment is one client's view of a session's output: the ring replayed
// as the first chunk, then live bytes, then an End.
//
// C() is not closed when the attachment ends — Done() is the signal, and
// chunks already buffered stay readable after it so a socket can flush
// what it has before sending its close frame.
type Attachment struct {
	sess *Session
	ch   chan []byte
	done chan struct{}

	once sync.Once
	mu   sync.Mutex
	end  End
}

// C yields output chunks, oldest first.
func (a *Attachment) C() <-chan []byte { return a.ch }

// Done closes when this attachment ends, for any of the four reasons.
func (a *Attachment) Done() <-chan struct{} { return a.done }

// End is why it ended. Read it after Done.
func (a *Attachment) End() End {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.end
}

// Detach lets this client go without touching the session. The reconnect
// grace starts if it was the last one.
func (a *Attachment) Detach() { a.sess.detach(a, End{Kind: EndDetached}) }

func (a *Attachment) finish(end End) {
	a.once.Do(func() {
		a.mu.Lock()
		a.end = end
		a.mu.Unlock()
		close(a.done)
	})
}

// Session is one shell under one PTY.
type Session struct {
	id        string
	mgr       *Manager
	tokenID   string
	createdAt time.Time
	pid       int
	proc      *ptyProc
	done      chan struct{}

	mu           sync.Mutex
	cols, rows   uint16
	ring         *ring
	attached     map[*Attachment]struct{}
	lastOutputAt time.Time
	bytesOut     int64
	dropped      int
	graceTimer   *time.Timer
	closing      bool
	finished     bool
	exited       bool
	exitCode     int
	closeReason  string
}

// ID is the session id a socket URL carries.
func (s *Session) ID() string { return s.id }

// PID is the shell's process id — also its process-group id, because the
// child is started with Setsid.
func (s *Session) PID() int { return s.pid }

// TokenID is the pairing token this session was opened with, empty for a
// loopback client. Not exposed by Snapshot.
func (s *Session) TokenID() string { return s.tokenID }

// Done closes when the shell is gone and every attachment has been told.
func (s *Session) Done() <-chan struct{} { return s.done }

// Info is this session's Snapshot row.
func (s *Session) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Info{
		ID:                 s.id,
		PID:                s.pid,
		Cols:               s.cols,
		Rows:               s.rows,
		Attached:           len(s.attached),
		CreatedAt:          s.createdAt,
		LastOutputAt:       s.lastOutputAt,
		BytesOut:           s.bytesOut,
		DroppedAttachments: s.dropped,
		Exited:             s.exited,
		ExitCode:           s.exitCode,
	}
}

// Write sends bytes to the shell's stdin.
func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	fin := s.finished
	s.mu.Unlock()
	if fin {
		return 0, ErrSessionClosed
	}
	return s.proc.Write(p)
}

// Resize sets the PTY window size; on unix the child receives SIGWINCH.
func (s *Session) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return errors.New("term: resize needs a non-zero size")
	}
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return ErrSessionClosed
	}
	s.cols, s.rows = cols, rows
	s.mu.Unlock()
	return s.proc.resize(cols, rows)
}

// Attach returns a reader that first yields the ring, then live output.
// Attaching cancels a pending reap.
func (s *Session) Attach() (*Attachment, error) {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return nil, ErrSessionClosed
	}
	a := &Attachment{
		sess: s,
		ch:   make(chan []byte, s.mgr.cfg.AttachBuffer),
		done: make(chan struct{}),
	}
	if replay := s.ring.bytes(); len(replay) > 0 {
		// Buffered, capacity >= 1, and this attachment is not yet
		// registered: the replay cannot block and cannot be interleaved
		// with live bytes.
		a.ch <- replay
	}
	s.attached[a] = struct{}{}
	s.stopGraceLocked()
	s.mu.Unlock()
	return a, nil
}

// Close reaps the session: SIGHUP to the process group, then SIGKILL after
// CloseGrace if the pump has not finished.
func (s *Session) Close() error {
	s.closeWithReason(ReasonClosed)
	return nil
}

func (s *Session) closeWithReason(reason string) {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	if s.closeReason == "" {
		s.closeReason = reason
	}
	already := s.closing
	s.closing = true
	s.stopGraceLocked()
	s.mu.Unlock()
	if already {
		<-s.done
		return
	}
	// SIGHUP the whole process group, not just the shell: the point of
	// Setsid at spawn is that a grandchild (`sleep 300 &`) is reachable
	// here. Then SIGKILL the same group if the pump has not ended.
	_ = s.proc.hangup()
	select {
	case <-s.done:
		return
	case <-time.After(CloseGrace):
	}
	_ = s.proc.kill()
	select {
	case <-s.done:
	case <-time.After(CloseGrace):
		// The read did not return even after SIGKILL; closing the master
		// is what unblocks it.
		_ = s.proc.closePTY()
		<-s.done
	}
}

// pump is the single PTY reader. Nothing else reads the master, so a slow
// client is a channel problem, never a shell problem.
func (s *Session) pump() {
	buf := make([]byte, readChunk)
	for {
		n, err := s.proc.Read(buf)
		if n > 0 {
			s.emit(buf[:n])
		}
		if err != nil {
			break
		}
	}
	code, _ := s.proc.wait()
	s.finish(code)
}

func (s *Session) emit(p []byte) {
	chunk := make([]byte, len(p))
	copy(chunk, p)
	s.mu.Lock()
	s.ring.write(chunk)
	s.bytesOut += int64(len(chunk))
	s.lastOutputAt = s.mgr.cfg.Now()
	var dropped []*Attachment
	for a := range s.attached {
		select {
		case a.ch <- chunk:
		default:
			// Bounded channel full: drop this client, keep the pump.
			dropped = append(dropped, a)
			delete(s.attached, a)
			s.dropped++
		}
	}
	empty := len(s.attached) == 0
	s.mu.Unlock()
	for _, a := range dropped {
		a.finish(End{Kind: EndDropped, Reason: ReasonSlow})
	}
	if len(dropped) > 0 && empty {
		s.armGrace()
	}
}

func (s *Session) finish(code int) {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.exited = true
	s.exitCode = code
	reason := s.closeReason
	s.stopGraceLocked()
	att := make([]*Attachment, 0, len(s.attached))
	for a := range s.attached {
		att = append(att, a)
	}
	s.attached = map[*Attachment]struct{}{}
	s.mu.Unlock()

	// Leave the manager before telling anyone: a client woken by Done and
	// a `gadak terminal list` racing it must not still see this id.
	s.mgr.forget(s)
	_ = s.proc.closePTY()

	end := End{Kind: EndExited, Code: code}
	if reason != "" {
		end = End{Kind: EndClosed, Reason: reason, Code: code}
	}
	for _, a := range att {
		a.finish(end)
	}
	close(s.done)
}

// armGrace starts the reconnect grace. Called wherever the attachment
// count reaches zero, including at Create: an id nobody connects to must
// not hold a shell forever.
func (s *Session) armGrace() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || s.closing || len(s.attached) > 0 {
		return
	}
	s.stopGraceLocked()
	s.graceTimer = s.mgr.cfg.AfterFunc(s.mgr.cfg.Grace, s.reap)
}

func (s *Session) stopGraceLocked() {
	if s.graceTimer != nil {
		s.graceTimer.Stop()
		s.graceTimer = nil
	}
}

func (s *Session) reap() {
	s.mu.Lock()
	idle := !s.finished && !s.closing && len(s.attached) == 0
	s.mu.Unlock()
	if idle {
		s.closeWithReason(ReasonReaped)
	}
}

func (s *Session) detach(a *Attachment, end End) {
	s.mu.Lock()
	_, live := s.attached[a]
	if live {
		delete(s.attached, a)
	}
	empty := len(s.attached) == 0
	fin := s.finished
	s.mu.Unlock()
	if !live {
		return
	}
	a.finish(end)
	if empty && !fin {
		s.armGrace()
	}
}
