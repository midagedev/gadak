package term

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

// Defaults named in doc.go. They are package constants so the contract has
// one owner; Config overrides them for tests (a 60-second grace is not
// something a test may sleep through).
const (
	// DefaultRingBytes is the scrollback a session replays on reattach.
	DefaultRingBytes = 256 << 10
	// DefaultAttachBytes is how far behind, in bytes, one attachment may
	// fall before it is dropped. On a loopback socket, four megabytes of
	// unread output is a client that is gone, not one that is busy — and
	// it is 16x the ring, so a drop can never be caused by the replay
	// alone.
	DefaultAttachBytes = 4 << 20
	// DefaultGrace is how long a session outlives its last attachment
	// with nothing running under it.
	DefaultGrace = 60 * time.Second
	// DefaultMaxDetachedLife is the ceiling the grace may not raise: a
	// session that keeps re-arming because work is still on its terminal
	// is closed once it has been unattached this long. Without it, one
	// `sleep infinity` would hold a shell for the life of the serve.
	DefaultMaxDetachedLife = 24 * time.Hour
	// CloseGrace is how long Close waits after SIGHUP before SIGKILL.
	CloseGrace = 2 * time.Second
	// readChunk is the PTY read size. Big enough that a busy shell is a
	// few chunks per frame, small enough that the ring is not one write.
	readChunk = 32 << 10
)

// ErrUnsupportedPlatform is Create's answer where gadak has no PTY yet.
var ErrUnsupportedPlatform = errors.New("term: no PTY on this platform")

// ErrNotFound is Get/Close for a session id the manager does not hold —
// including one already reaped after its grace.
var ErrNotFound = errors.New("term: no such session")

// ErrSessionClosed is a write or resize on a session whose shell is gone.
var ErrSessionClosed = errors.New("term: session closed")

// Config tunes a Manager. Zero values take the package defaults, so
// New(Config{}) is the production shape.
type Config struct {
	// WorkDir is the cwd every session starts in unless Options.Dir names
	// another — the workspace directory in `gadak serve`.
	WorkDir string
	// Grace is how long a session outlives its last attachment.
	Grace time.Duration
	// MaxDetachedLife caps how long re-arming may keep an unattached
	// session alive. Zero takes DefaultMaxDetachedLife.
	MaxDetachedLife time.Duration
	// RingBytes is the per-session scrollback.
	RingBytes int
	// AttachBytes is the per-attachment backlog bound, in bytes. Zero
	// takes DefaultAttachBytes.
	AttachBytes int
	// AfterFunc is time.AfterFunc, injectable so a test can pin the
	// reconnect grace without sleeping a minute.
	AfterFunc func(time.Duration, func()) *time.Timer
	// Now is time.Now, injectable for the same reason.
	Now func() time.Time
}

// Manager owns every live session in this process.
type Manager struct {
	cfg Config

	mu       sync.Mutex
	sessions map[string]*Session
	closed   bool
}

// New returns a Manager. It starts no goroutines: a Manager with no
// sessions costs nothing, which is what every `gadak serve` that never
// opens a terminal should pay.
func New(cfg Config) *Manager {
	if cfg.Grace <= 0 {
		cfg.Grace = DefaultGrace
	}
	if cfg.MaxDetachedLife <= 0 {
		cfg.MaxDetachedLife = DefaultMaxDetachedLife
	}
	if cfg.RingBytes <= 0 {
		cfg.RingBytes = DefaultRingBytes
	}
	if cfg.AttachBytes <= 0 {
		cfg.AttachBytes = DefaultAttachBytes
	}
	if cfg.AfterFunc == nil {
		cfg.AfterFunc = time.AfterFunc
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Manager{cfg: cfg, sessions: map[string]*Session{}}
}

// Options is one Create call.
type Options struct {
	// Dir overrides Config.WorkDir for this session.
	Dir string
	// Cols and Rows are the initial PTY size. Zero takes 80x24 — a shell
	// with a zero-sized terminal draws nothing and is a support ticket.
	Cols, Rows uint16
	// Env is appended after the inherited environment and the two
	// variables this package always sets.
	Env []string
	// Shell overrides $SHELL. The settings catalog's terminal.shell
	// reaches here through handleTerminalCreate (GDK-896); tests use it
	// too.
	Shell string
	// Args are the shell's arguments.
	Args []string
	// TokenID is the pairing token this session was opened with — empty
	// for a loopback client, which needs none. CloseByToken reads it.
	TokenID string
}

// Info is one row of Snapshot: everything a `gadak terminal list` needs and
// nothing a socket carries. No output bytes, no token id.
type Info struct {
	ID                 string    `json:"id"`
	PID                int       `json:"pid"`
	Cols               uint16    `json:"cols"`
	Rows               uint16    `json:"rows"`
	Attached           int       `json:"attached"`
	CreatedAt          time.Time `json:"created_at"`
	LastOutputAt       time.Time `json:"last_output_at"`
	BytesOut           int64     `json:"bytes_out"`
	DroppedAttachments int       `json:"dropped_attachments"`
	Exited             bool      `json:"exited"`
	ExitCode           int       `json:"exit_code"`
	// BacklogMaxBytes is the high-water mark of any one attachment's
	// pending backlog over the session's life — the number that answers
	// "was this anywhere near the drop bound?". CoalescedChunks counts
	// how many emits landed on a non-empty backlog, i.e. how often
	// merging actually did work (GDK-1042).
	BacklogMaxBytes int64 `json:"backlog_max_bytes"`
	CoalescedChunks int64 `json:"coalesced_chunks"`
	// PIDs is every process currently on this session's controlling
	// terminal, including the shell. Empty when the enumerator cannot
	// see a tty (Windows, or a pid with no controlling terminal).
	// Filled only by Snapshot — the list path; the create path does not
	// walk the process table (GDK-988).
	PIDs []int `json:"pids,omitempty"`
	// DetachedAt is when this session last lost its final attachment,
	// zero while anything is attached. GraceExtensions counts how many
	// times the reconnect grace re-armed instead of reaping because work
	// was still on the terminal. The pair answers "why is this shell
	// still here" from `gadak terminal list`, without a debugger.
	DetachedAt      time.Time `json:"detached_at"`
	GraceExtensions int       `json:"grace_extensions,omitempty"`
}

// Create spawns a shell under a PTY and returns its session.
func (m *Manager) Create(opts Options) (*Session, error) {
	if opts.Cols == 0 {
		opts.Cols = 80
	}
	if opts.Rows == 0 {
		opts.Rows = 24
	}
	if opts.Dir == "" {
		opts.Dir = m.cfg.WorkDir
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrSessionClosed
	}
	m.mu.Unlock()

	id, err := newID()
	if err != nil {
		return nil, err
	}
	proc, err := startProc(opts)
	if err != nil {
		return nil, err
	}
	now := m.cfg.Now()
	s := &Session{
		id:        id,
		mgr:       m,
		tokenID:   opts.TokenID,
		createdAt: now,
		cols:      opts.Cols,
		rows:      opts.Rows,
		proc:      proc,
		pid:       proc.pid(),
		ring:      newRing(m.cfg.RingBytes),
		attached:  map[*Attachment]struct{}{},
		done:      make(chan struct{}),
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = proc.kill()
		return nil, ErrSessionClosed
	}
	m.sessions[id] = s
	m.mu.Unlock()

	// A session born with no attachment is already inside its grace: an
	// id nobody ever connects to must not hold a shell forever.
	s.armGrace()
	go s.pump()
	return s, nil
}

// Get returns a live session by id.
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

// List returns every live session, oldest first.
func (m *Manager) List() []*Session {
	m.mu.Lock()
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].createdAt.Before(out[j].createdAt) })
	return out
}

// Snapshot is the introspection surface — see doc.go. It is the one place
// PIDs is filled: Info stays a pure field copy so the create path never
// walks the process table (GDK-988).
func (m *Manager) Snapshot() []Info {
	sessions := m.List()
	out := make([]Info, 0, len(sessions))
	for _, s := range sessions {
		info := s.Info()
		info.PIDs = membersOf(info.PID)
		out = append(out, info)
	}
	return out
}

// CloseAll reaps every session. Called on server shutdown: an exiting
// serve must not leave shells behind.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	m.closed = true
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	// Concurrently: each Close is SIGHUP, wait, then SIGKILL after
	// CloseGrace, so doing them in sequence would make a serve's exit
	// time proportional to how many shells happen to be ignoring HUP.
	var wg sync.WaitGroup
	for _, s := range all {
		wg.Add(1)
		go func(s *Session) {
			defer wg.Done()
			s.closeWithReason(ReasonShutdown)
		}(s)
	}
	wg.Wait()
}

// CloseByToken reaps every session opened with tokenID. This is what
// `gadak pairing revoke` reaches through: a revoked terminal token must
// lose the shell it opened, not just the next request. An empty tokenID
// matches nothing — loopback sessions are not token-bound and revoking a
// token must not cut the local pane.
func (m *Manager) CloseByToken(tokenID string) int {
	if tokenID == "" {
		return 0
	}
	m.mu.Lock()
	var hit []*Session
	for _, s := range m.sessions {
		if s.tokenID == tokenID {
			hit = append(hit, s)
		}
	}
	m.mu.Unlock()
	for _, s := range hit {
		s.closeWithReason(ReasonRevoked)
	}
	return len(hit)
}

// TokenIDs is the distinct set of non-empty token ids live sessions were
// opened with — what the revoke watchdog re-checks against the store.
func (m *Manager) TokenIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	out := []string{}
	for _, s := range m.sessions {
		if s.tokenID != "" && !seen[s.tokenID] {
			seen[s.tokenID] = true
			out = append(out, s.tokenID)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Manager) forget(s *Session) {
	m.mu.Lock()
	if cur, ok := m.sessions[s.id]; ok && cur == s {
		delete(m.sessions, s.id)
	}
	m.mu.Unlock()
}

// newID is 128 bits of crypto/rand, hex. Sequential ids would make a
// session guessable from a neighbour's URL.
func newID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
