package origin

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/jira"
	issuetap "github.com/midagedev/issuetap"
)

// PersistRel is the workspace-relative path of the issuetap write-through
// snapshot. The file is the origin; the SQLite mirror remains a disposable
// cache filled by sync.
const PersistRel = "origin/issuetap.yaml"

// DefaultProjectKey is the project seeded into a new standalone origin so
// create/createmeta have somewhere to file. Issuetap also creates a project
// on first write if a caller names another key.
const DefaultProjectKey = "STD"

// DefaultSpaceKey is the wiki space seeded into a new standalone origin so
// page create has somewhere to file. Short and obviously local — not a
// display name, and not a site-specific key.
const DefaultSpaceKey = "LOC"

// DefaultConfluenceConfig is what initStandalone writes so the wiki
// sync pass is on and scoped to the seeded space. Presence of the block is
// the on switch (internal/sync/confluence.go).
func DefaultConfluenceConfig() *config.ConfluenceConfig {
	return &config.ConfluenceConfig{Spaces: []string{DefaultSpaceKey}}
}

// in-process Basic credentials presented to issuetap. They never leave this
// process: they are not written to config, a log, or disk. Issuetap rejects
// a completely empty Authorization (Accepts).
const (
	inProcessUser   = "standalone"
	inProcessSecret = "standalone"
)

// errNeedCredential is the connected-workspace refusal. Wording matches the
// existing sync/init gates so a missing token still reads the same.
var errNeedCredential = errors.New("origin: site, email and token are required")

// PersistPath is the absolute issuetap snapshot path inside a profile directory.
func PersistPath(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, filepath.FromSlash(PersistRel))
}

// Connected builds a client for an explicit site/email/token — verifying a
// credential the user just typed, not "this workspace's origin".
func Connected(site, email, token string) *jira.Client {
	return jira.New(site, email, token)
}

// Client is the single owner of "this workspace's Jira client".
// A connected workspace gets the same jira.New(site, email, token) as before.
// A standalone workspace gets a client whose Transport is the in-process
// issuetap handler, unless a live `gadak serve` for this profile advertised
// itself — then Transport is that serve's origin passthrough so persist has
// one owner. BaseURL stays empty so stored browse links are /browse/KEY
// rather than a fake https origin a person might click.
func Client(cfg *config.Config) (*jira.Client, error) {
	if cfg == nil {
		return nil, errors.New("origin: nil config")
	}
	if cfg.IsStandalone() {
		return standaloneClient(cfg)
	}
	if cfg.Site == "" || cfg.Email == "" || cfg.Token == "" {
		return nil, errNeedCredential
	}
	return Connected(cfg.Site, cfg.Email, cfg.Token), nil
}

// Describe answers doctor: which kind of workspace, and where the origin is.
// Connected reports "jira" (no hostname — doctor is safe to paste).
// Standalone reports the persist path.
func Describe(cfg *config.Config) (kind, origin string) {
	if cfg == nil || !cfg.IsStandalone() {
		return config.KindConnected, "jira"
	}
	dir, err := profileDir(cfg)
	if err != nil {
		return config.KindStandalone, PersistRel
	}
	return config.KindStandalone, PersistPath(dir)
}

type session struct {
	emb    *issuetap.Embedded
	client *jira.Client
	wiki   *confluence.Client
}

type sessionFlight struct {
	done chan struct{}
	s    *session
	err  error
}

var (
	// mu guards live and flights. Contract: no IO under this lock. The
	// critical section is a map lookup or insert; MkdirAll,
	// issuetap.NewEmbedded (persist read/write), and Embedded.Close run
	// with the lock released. This mutex is process-global and keyed by
	// persist path — holding it across persist IO queues every standalone
	// workspace behind one disk.
	mu      sync.Mutex
	live    = map[string]*session{}
	flights = map[string]*sessionFlight{}

	sessionInFlight     atomic.Int64
	sessionsDiscarded   atomic.Uint64
	sessionsConstructed atomic.Uint64
	inProcess           atomic.Bool
)

// SessionInFlight is how many standaloneSession constructions are running
// outside mu. Same shape as store.WriteBusyRetries: a cheap accessor, no logs.
func SessionInFlight() int64 { return sessionInFlight.Load() }

// SessionsDiscarded is how many constructed sessions lost the publish race
// and were closed. Same shape as store.WriteBusyRetries.
func SessionsDiscarded() uint64 { return sessionsDiscarded.Load() }

// SessionsConstructed is how many times constructStandalone ran. Tests
// use a delta to prove a routed Client did not open a second persist graph.
func SessionsConstructed() uint64 { return sessionsConstructed.Load() }

// SetInProcess marks this process as the persist owner (`gadak serve` on
// a standalone workspace). Client and Wiki then always use the embedded
// session and never proxy to the advertise file — that would loop back
// onto this same listener.
func SetInProcess(v bool) { inProcess.Store(v) }

// ForgetLive drops cached embedded sessions without closing them. Tests
// use this to simulate a second process: the serve handler still holds
// the graph, but Client no longer finds it in this process.
func ForgetLive() {
	mu.Lock()
	live = map[string]*session{}
	flights = map[string]*sessionFlight{}
	mu.Unlock()
}

func standaloneClient(cfg *config.Config) (*jira.Client, error) {
	if c, ok := routedJira(cfg); ok {
		return c, nil
	}
	s, err := standaloneSession(cfg)
	if err != nil {
		return nil, err
	}
	return s.client, nil
}

// StandaloneHandler is the in-process issuetap HTTP surface for this
// workspace. Serve's origin passthrough uses it so CLI writes land on
// the same graph the UI already holds. Always embeds — never routes —
// so the serve process cannot proxy to itself through this entry.
func StandaloneHandler(cfg *config.Config) (http.Handler, error) {
	s, err := standaloneSession(cfg)
	if err != nil {
		return nil, err
	}
	if s == nil || s.emb == nil {
		return nil, errors.New("origin: standalone handler is missing")
	}
	return s.emb, nil
}

// Wiki is the single owner of "this workspace's Confluence client".
// A connected workspace gets confluence.New(site, email, token).
// A standalone workspace shares the in-process issuetap handler with Client.
func Wiki(cfg *config.Config) (*confluence.Client, error) {
	if cfg == nil {
		return nil, errors.New("origin: nil config")
	}
	if cfg.IsStandalone() {
		return standaloneWiki(cfg)
	}
	if cfg.Site == "" || cfg.Email == "" || cfg.Token == "" {
		return nil, errNeedCredential
	}
	return confluence.New(cfg.Site, cfg.Email, cfg.Token), nil
}

func standaloneWiki(cfg *config.Config) (*confluence.Client, error) {
	if w, ok := routedWiki(cfg); ok {
		return w, nil
	}
	s, err := standaloneSession(cfg)
	if err != nil {
		return nil, err
	}
	return s.wiki, nil
}

// testBeforeStandalone, if set, runs after the live-session lookup and before
// MkdirAll / issuetap.NewEmbedded. Tests use it as a barrier to prove the
// process-global mutex is not held across persist IO. Production is nil.
var testBeforeStandalone func(persist string)

func standaloneSession(cfg *config.Config) (*session, error) {
	dir, err := profileDir(cfg)
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, errors.New("origin: standalone workspace has no profile directory")
	}
	persist := PersistPath(dir)

	mu.Lock()
	if s, ok := live[persist]; ok {
		mu.Unlock()
		return s, nil
	}
	if f, ok := flights[persist]; ok {
		mu.Unlock()
		<-f.done
		return f.s, f.err
	}
	f := &sessionFlight{done: make(chan struct{})}
	if flights == nil {
		flights = map[string]*sessionFlight{}
	}
	flights[persist] = f
	mu.Unlock()

	if testBeforeStandalone != nil {
		testBeforeStandalone(persist)
	}

	sessionInFlight.Add(1)
	s, err := constructStandalone(persist)
	sessionInFlight.Add(-1)

	mu.Lock()
	delete(flights, persist)
	if err != nil {
		f.err = err
		close(f.done)
		mu.Unlock()
		return nil, err
	}
	if existing, ok := live[persist]; ok {
		sessionsDiscarded.Add(1)
		mu.Unlock()
		closeSession(s)
		f.s = existing
		close(f.done)
		return existing, nil
	}
	live[persist] = s
	f.s = s
	close(f.done)
	mu.Unlock()
	return s, nil
}

func constructStandalone(persist string) (*session, error) {
	sessionsConstructed.Add(1)
	if err := os.MkdirAll(filepath.Dir(persist), 0o700); err != nil {
		return nil, fmt.Errorf("origin: persist dir: %w", err)
	}

	emb, err := issuetap.NewEmbedded(issuetap.EmbeddedConfig{
		PersistPath:  persist,
		FixtureBytes: defaultStandaloneFixture,
		// The persist file is the origin — a write we acknowledged must be
		// on disk before the response, not a debounce window later. A
		// negative debounce is issuetap's durable-before-return mode
		// (GDK-342); the mirror keeps its own transactionality either way.
		PersistDebounce: -1,
	})
	if err != nil {
		return nil, fmt.Errorf("origin: issuetap: %w", err)
	}

	tr := &handlerTransport{h: emb}

	c := Connected("", inProcessUser, inProcessSecret)
	if c.HTTP == nil {
		c.HTTP = &http.Client{}
	}
	c.HTTP.Transport = tr

	w := confluence.New("", inProcessUser, inProcessSecret)
	if w.HTTP == nil {
		w.HTTP = &http.Client{}
	}
	w.HTTP.Transport = tr

	return &session{emb: emb, client: c, wiki: w}, nil
}

func closeSession(s *session) {
	if s != nil && s.emb != nil {
		_ = s.emb.Close()
	}
}

func profileDir(cfg *config.Config) (string, error) {
	if cfg != nil {
		if d := cfg.Directory(); d != "" {
			return d, nil
		}
	}
	return config.Dir()
}

// Close flushes every live standalone origin and drops the sessions.
// Safe to call more than once. The process owner (cmd/gadak main) calls
// this on the way out so the last PersistDebounce window is not lost.
//
// In-flight constructors are waited on (they publish, then this snapshots)
// so Close never closes a half-built Embedded. There is no permanent
// closed flag: Client after Close must open a new session (persist is
// the origin; the process is allowed to come back).
func Close() error {
	mu.Lock()
	wait := make([]chan struct{}, 0, len(flights))
	for _, f := range flights {
		wait = append(wait, f.done)
	}
	mu.Unlock()
	for _, done := range wait {
		<-done
	}
	mu.Lock()
	snapshot := live
	live = map[string]*session{}
	mu.Unlock()
	var first error
	for _, s := range snapshot {
		if s != nil && s.emb != nil {
			if err := s.emb.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

// defaultStandaloneFixture is applied only when PersistPath does not yet
// exist. It names one project so createmeta/create have a target, and one
// space so page create has a target; it does not seed issues or pages.
// When the persist file is present, issuetap skips this.
//
// Keys come from DefaultProjectKey / DefaultSpaceKey so the literals are
// not scattered.
var defaultStandaloneFixture = []byte("projects:\n" +
	"  - id: \"10000\"\n" +
	"    key: " + DefaultProjectKey + "\n" +
	"    name: Standalone\n" +
	"    type: software\n" +
	"    style: classic\n" +
	"spaces:\n" +
	"  - id: \"40000\"\n" +
	"    key: " + DefaultSpaceKey + "\n" +
	"    name: Local\n" +
	"    type: global\n")
