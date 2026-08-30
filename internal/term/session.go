package term

import (
	"errors"
	"sync"
	"time"
)

// bellByte is BEL (^G). A shell, an agent or a TUI writes it when it wants
// a person, which makes it the one signal on this stream that means
// "blocked on you" rather than "busy" (GDK-1163).
const bellByte = 0x07

// Why an attachment ended, as the socket reports it to its client.
const (
	// ReasonSlow: the client's pending backlog grew past AttachBytes.
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
// as the first backlog, then live bytes, then an End. The backlog is a
// byte quantity — N pending chunks are exactly their concatenation
// (GDK-1042) — so a client that is many small PTY reads behind is handed
// fewer, larger writes, never cut off for it.
//
// Wake() is the edge signal that the backlog is non-empty. Take() is not
// gated on the attachment ending — Done() is that signal, and a backlog
// that was pending when the end arrived stays readable through Take(), so
// a socket can flush what it has before sending its close frame.
type Attachment struct {
	sess *Session
	done chan struct{}
	// wake is the capacity-1 edge signal: "Take has something". Producers
	// send without blocking; a full channel already means "signalled".
	wake chan struct{}

	once    sync.Once
	mu      sync.Mutex
	end     End
	pending []byte
}

// Wake yields when Take has something to return. Edge-triggered: one
// pending token covers any amount of backlog, and a token may outlive the
// bytes it announced — a Take that returns nil is the reader's cue to go
// back to sleep, not an error.
func (a *Attachment) Wake() <-chan struct{} { return a.wake }

// Take returns the whole pending backlog, oldest byte first, and clears
// it; nil when nothing is pending. The returned slice is the caller's —
// Take keeps no alias to it.
func (a *Attachment) Take() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.pending) == 0 {
		return nil
	}
	out := a.pending
	a.pending = nil
	return out
}

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
	backlogMax   int64
	coalesced    int64
	graceTimer   *time.Timer
	graceArmedAt time.Time
	detachedAt   time.Time
	graceExts    int
	resizes      int
	attention    bool
	// bells carries the escape state between PTY reads: a chunk can end in
	// the middle of an OSC string, and whether the next 0x07 is a bell or
	// that string's terminator depends on it (bell.go).
	bells       bellScanner
	issueKey    string
	closing     bool
	finished    bool
	exited      bool
	exitCode    int
	closeReason string
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

// Info is this session's Snapshot row. PIDs stays empty here — only
// Snapshot (the list path) fills it, so the create path never walks the
// process table (GDK-988).
func (s *Session) Info() Info {
	s.mu.Lock()
	info := Info{
		ID:                 s.id,
		PID:                s.pid,
		Cols:               s.cols,
		Rows:               s.rows,
		Attached:           len(s.attached),
		CreatedAt:          s.createdAt,
		LastOutputAt:       s.lastOutputAt,
		BytesOut:           s.bytesOut,
		DroppedAttachments: s.dropped,
		BacklogMaxBytes:    s.backlogMax,
		CoalescedChunks:    s.coalesced,
		Exited:             s.exited,
		ExitCode:           s.exitCode,
		DetachedAt:         s.detachedAt,
		GraceExtensions:    s.graceExts,
		Resizes:            s.resizes,
		NeedsAttention:     s.attention,
		IssueKey:           s.issueKey,
	}
	s.mu.Unlock()
	return info
}

// SetIssueKey binds this session to the issue a claim in its shell took.
// One session holds one issue at a time: a new key replaces the old, and an
// empty key clears the binding. Runtime state only — see Info.IssueKey for
// why nothing persists it.
func (s *Session) SetIssueKey(key string) {
	s.mu.Lock()
	s.issueKey = key
	s.mu.Unlock()
}

// IssueKey is the issue this session is bound to, empty when none.
func (s *Session) IssueKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.issueKey
}

// Write sends bytes to the shell's stdin.
func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	fin := s.finished
	// Typing into a session is the other way the bell gets answered, and
	// the one that matters while the pane is already attached: attaching
	// cannot lower a bit raised after the attach, so without this a
	// session someone is watching would keep asking for them forever.
	s.attention = false
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
	if s.cols != cols || s.rows != rows {
		s.resizes++
	}
	s.cols, s.rows = cols, rows
	s.mu.Unlock()
	return s.proc.resize(cols, rows)
}

// Attach returns a reader that first yields the ring, then live output.
// Attaching cancels a pending reap.
func (s *Session) Attach() (*Attachment, error) {
	s.mu.Lock()
	// closing, not just finished: a reap that has already claimed this
	// session is going to kill the shell, and handing out an attachment
	// on the way there would show a client a pane that dies under it.
	if s.finished || s.closing {
		s.mu.Unlock()
		return nil, ErrSessionClosed
	}
	a := &Attachment{
		sess: s,
		done: make(chan struct{}),
		wake: make(chan struct{}, 1),
	}
	if replay := s.ring.bytes(); len(replay) > 0 {
		// Seeded while still under s.mu and before this attachment is
		// registered: the replay cannot interleave with live bytes, and no
		// reader exists yet to contend for a.pending. Replay bytes count
		// toward the backlog like any other — AttachBytes is 16x the ring,
		// so the replay alone can never drop a client.
		a.pending = append(a.pending, replay...)
		if n := int64(len(a.pending)); n > s.backlogMax {
			s.backlogMax = n
		}
		select {
		case a.wake <- struct{}{}:
		default:
		}
	}
	s.attached[a] = struct{}{}
	// A person arriving is the answer to the question the bell asked.
	s.attention = false
	s.stopGraceLocked()
	// Someone is watching again, so the detached clock starts over: the
	// absolute cap measures one unattended stretch, not the session's age.
	s.detachedAt = time.Time{}
	s.graceExts = 0
	s.mu.Unlock()
	return a, nil
}

// Close reaps the session: SIGHUP every process on the shell's
// controlling terminal, then SIGKILL whoever is still there after
// CloseGrace if the pump has not finished.
func (s *Session) Close() error {
	s.closeWithReason(ReasonClosed)
	return nil
}

// closeClaim is who runs a shutdown, decided under one lock.
type closeClaim int

const (
	// closeOwned: this caller signals the shell and waits for the pump.
	closeOwned closeClaim = iota
	// closeInProgress: another caller is already doing it; wait for Done.
	closeInProgress
	// closeSkip: finished already, or the claim's precondition is gone.
	closeSkip
)

// claimClose flips the session into closing under a single lock, so the
// decision to kill and the state that makes it true cannot drift apart.
// idleOnly is reap's claim: it also fails if something attached while the
// caller was deciding. That matters because reap walks the process table
// with the lock released — a window of milliseconds, not microseconds —
// and an Attach landing inside it must win, not lose its shell.
func (s *Session) claimClose(reason string, idleOnly bool) closeClaim {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return closeSkip
	}
	if idleOnly && (s.closing || len(s.attached) > 0) {
		return closeSkip
	}
	if s.closeReason == "" {
		s.closeReason = reason
	}
	if s.closing {
		return closeInProgress
	}
	s.closing = true
	s.stopGraceLocked()
	return closeOwned
}

func (s *Session) closeWithReason(reason string) {
	switch s.claimClose(reason, false) {
	case closeSkip:
		return
	case closeInProgress:
		<-s.done
		return
	}
	s.terminate()
}

// terminate is the shutdown itself. Only a caller holding closeOwned runs
// it.
func (s *Session) terminate() {
	// SIGHUP every process on the shell's controlling terminal, then
	// always SIGKILL whoever is still there. Returning when the shell
	// has already exited would skip a grandchild that ignored SIGHUP
	// (TestCloseKillsHUPImmuneGrandchild).
	//
	// hangup walks the terminal once and remembers what it found; kill
	// signals that same remembered set. Walking again at kill time is
	// not an option — see captureMembers: by then the terminal has been
	// revoked and its device number reissued, so the second walk finds
	// either nothing or another session's shell.
	_ = s.proc.hangup()
	select {
	case <-s.done:
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
	// The single place the attention bit can be raised: emit is the only
	// reader of the PTY, so every byte this session will ever produce
	// passes here exactly once. Deriving it anywhere else would be a
	// second opinion on the same stream. The scanner — not a plain byte
	// search — is what keeps a window-title OSC's terminator from reading
	// as a bell; see bell.go for the prompt that made every Linux session
	// ask for a person forever.
	if s.bells.scan(chunk) {
		s.attention = true
	}
	var dropped []*Attachment
	for a := range s.attached {
		if !a.push(chunk) {
			// Backlog past AttachBytes: drop this client, keep the pump.
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

// push appends one chunk of output to the attachment's backlog and edges
// wake. It reports false — appending nothing — when the result would
// exceed cfg.AttachBytes; emit reads that as "drop this client".
//
// Lock ordering, fixed here: emit holds s.mu across the walk and takes
// a.mu inside push, so no Attachment method may take s.mu while holding
// a.mu — that inverts the order and deadlocks the pump. It is also why
// the session counters below may be touched without a second lock: push
// runs with s.mu already held by its only caller.
func (a *Attachment) push(p []byte) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.pending)+len(p) > a.sess.mgr.cfg.AttachBytes {
		return false
	}
	if len(a.pending) > 0 {
		a.sess.coalesced++
	}
	a.pending = append(a.pending, p...)
	if n := int64(len(a.pending)); n > a.sess.backlogMax {
		a.sess.backlogMax = n
	}
	select {
	case a.wake <- struct{}{}:
	default:
		// A token is already pending; one signal covers everything Take
		// has not collected yet.
	}
	return true
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
// not hold a shell forever. reap calls it again when it decides to spare
// the session; detachedAt survives that, so the absolute cap still runs.
func (s *Session) armGrace() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || s.closing || len(s.attached) > 0 {
		return
	}
	now := s.mgr.cfg.Now()
	if s.detachedAt.IsZero() {
		s.detachedAt = now
	}
	s.graceArmedAt = now
	s.stopGraceLocked()
	s.graceTimer = s.mgr.cfg.AfterFunc(s.mgr.cfg.Grace, s.reap)
}

func (s *Session) stopGraceLocked() {
	if s.graceTimer != nil {
		s.graceTimer.Stop()
		s.graceTimer = nil
	}
}

// reapVerdict is what idleForReap decided. Three outcomes, because
// "not idle" and "no longer ours to reap" want different follow-ups.
type reapVerdict int

const (
	// reapClose: nothing is attached and nothing is running. Reap.
	reapClose reapVerdict = iota
	// reapSpare: unattached, but work is still on this terminal. Give it
	// another grace.
	reapSpare
	// reapGone: attached again, or already closing/finished. Do nothing.
	reapGone
)

// idleForReap is the single owner of "is this session idle", because the
// bug it exists to close was that answer being derived inline from one
// signal: the attachment count. A shell with an agent, a build, or an
// editor under it is not idle just because the last tab went away — that
// is the whole point of leaving a job running and watching from a phone,
// and until GDK-994 the grace killed it 60 seconds later.
//
// The signals, cheapest first:
//
//   - an attachment: someone is watching. Never reap.
//   - output since the grace was armed: the shell is still talking.
//   - a process besides the shell on this controlling terminal: work.
//
// MaxDetachedLife is the one thing none of them may raise: past it an
// unattended session is closed however busy it looks, so a stray
// `sleep infinity` cannot hold a shell for the life of the serve.
func (s *Session) idleForReap() reapVerdict {
	s.mu.Lock()
	if s.finished || s.closing {
		s.mu.Unlock()
		return reapGone
	}
	if len(s.attached) > 0 {
		s.mu.Unlock()
		return reapGone
	}
	pid := s.pid
	armedAt, lastOut, since := s.graceArmedAt, s.lastOutputAt, s.detachedAt
	s.mu.Unlock()

	if !since.IsZero() && s.mgr.cfg.Now().Sub(since) >= s.mgr.cfg.MaxDetachedLife {
		return reapClose
	}
	if lastOut.After(armedAt) {
		return reapSpare
	}
	// Walked outside the lock deliberately: the enumerator reads every
	// process on the machine, and holding s.mu across it would stall the
	// pump. Snapshot's PIDs fill keeps the same rule for the same reason
	// (GDK-988). An enumerator that cannot see a tty returns nothing,
	// which reads as idle — the pre-GDK-994 behaviour, which is the right
	// way to fail here.
	if len(membersOf(pid)) > 1 {
		return reapSpare
	}
	return reapClose
}

func (s *Session) reap() {
	switch s.idleForReap() {
	case reapGone:
		return
	case reapSpare:
		s.mu.Lock()
		s.graceExts++
		s.mu.Unlock()
		s.armGrace()
	default:
		// idleOnly: re-check under the lock that decides. If an Attach
		// landed during the walk, this session is not idle any more and
		// the client that just arrived keeps its shell.
		if s.claimClose(ReasonReaped, true) == closeOwned {
			s.terminate()
		}
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
