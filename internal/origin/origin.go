package origin

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/midagedev/gadak/internal/config"
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
// issuetap handler; BaseURL is empty so stored browse links are /browse/KEY
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
}

var (
	mu   sync.Mutex
	live = map[string]*session{}
)

func standaloneClient(cfg *config.Config) (*jira.Client, error) {
	dir, err := profileDir(cfg)
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, errors.New("origin: standalone workspace has no profile directory")
	}
	persist := PersistPath(dir)

	mu.Lock()
	defer mu.Unlock()
	if s, ok := live[persist]; ok {
		return s.client, nil
	}

	if err := os.MkdirAll(filepath.Dir(persist), 0o700); err != nil {
		return nil, fmt.Errorf("origin: persist dir: %w", err)
	}

	emb, err := issuetap.NewEmbedded(issuetap.EmbeddedConfig{
		PersistPath:  persist,
		FixtureBytes: defaultStandaloneFixture,
	})
	if err != nil {
		return nil, fmt.Errorf("origin: issuetap: %w", err)
	}

	c := Connected("", inProcessUser, inProcessSecret)
	if c.HTTP == nil {
		c.HTTP = &http.Client{}
	}
	c.HTTP.Transport = &handlerTransport{h: emb}

	live[persist] = &session{emb: emb, client: c}
	return c, nil
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
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	var first error
	for path, s := range live {
		if s != nil && s.emb != nil {
			if err := s.emb.Close(); err != nil && first == nil {
				first = err
			}
		}
		delete(live, path)
	}
	return first
}

// defaultStandaloneFixture is applied only when PersistPath does not yet
// exist. It names one project so createmeta/create have a target; it does
// not seed issues. When the persist file is present, issuetap skips this.
var defaultStandaloneFixture = []byte(`projects:
  - id: "10000"
    key: STD
    name: Standalone
    type: software
    style: classic
`)
