package origin

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/pairing"
	issuetap "github.com/midagedev/issuetap"
)

// PersistRel is the workspace-relative path of the issuetap write-through
// SQLite state file (WAL). The file is the origin; gadak.db remains a
// disposable cache filled by sync.
const PersistRel = "origin/issuetap.db"

// LegacyYAMLRel is the pre-SQLite persist path. When PersistRel is absent
// and this file exists, NewEmbedded seeds from it as FixturePath once.
// The YAML is left in place as a rollback asset.
const LegacyYAMLRel = "origin/issuetap.yaml"

// DefaultProjectKey is the project seeded into a new local-origin origin so
// create/createmeta have somewhere to file. Issuetap also creates a project
// on first write if a caller names another key.
const DefaultProjectKey = "STD"

// DefaultSpaceKey is the wiki space seeded into a new local-origin origin so
// page create has somewhere to file. Short and obviously local — not a
// display name, and not a site-specific key.
const DefaultSpaceKey = "LOC"

// DefaultConfluenceConfig is what initLocalOrigin writes so the wiki
// sync pass is on and scoped to the seeded space. Presence of the block is
// the on switch (internal/sync/confluence.go).
func DefaultConfluenceConfig() *config.ConfluenceConfig {
	return &config.ConfluenceConfig{Spaces: []string{DefaultSpaceKey}}
}

// PairedConfluenceConfig is the paired twin: what pairing writes so the
// wiki pass is on. Spaces stays empty — every team space the home origin
// lists — because this side has no site of its own to name and cannot see
// which keys the home machine seeded (GDK-1276: without the block a paired
// workspace mirrored issues but reported its wiki as not configured).
func PairedConfluenceConfig() *config.ConfluenceConfig {
	return &config.ConfluenceConfig{}
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

// errNoWikiOrigin is Wiki's Linear-only refusal. Linear has no
// wiki; quoting errNeedCredential sent the user to fill a Jira site that
// this workspace does not have.
var errNoWikiOrigin = errors.New("origin: this workspace has no wiki origin — Linear does not provide a wiki")

// InProcessAuthB64 is the base64 payload of the in-process Basic
// credential the local CLI presents to the origin passthrough. Exported so
// the server's pairing gate can rewrite a *validated* Bearer into the exact
// Authorization shape the embedded issuetap graph has always seen: the gate
// authenticates the caller, then speaks to the origin as the in-process
// user it always did (GDK-433).
func InProcessAuthB64() string {
	return base64.StdEncoding.EncodeToString([]byte(inProcessUser + ":" + inProcessSecret))
}

// PersistPath is the absolute issuetap SQLite state path inside a profile directory.
func PersistPath(dir string) string {
	return joinRel(dir, PersistRel)
}

// LegacyYAMLPath is the absolute pre-SQLite persist path inside a profile
// directory. Empty dir yields empty path.
func LegacyYAMLPath(dir string) string {
	return joinRel(dir, LegacyYAMLRel)
}

func joinRel(dir, rel string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, filepath.FromSlash(rel))
}

// ErrWorkspaceFrozen: frozen means no request leaves for the origin — pulls
// and writes alike (GDK-507 decision). The gate lives here, on the client
// mint, so every surface (CLI verbs, REST writes, per-issue resync, the api
// escape hatch, page writes) refuses in one place instead of each carrying
// its own check. A scrubbed demo fixture with a live credential must neither
// re-pollute its mirror nor create real issues on the origin.
var ErrWorkspaceFrozen = errors.New("this workspace is frozen — no requests leave for the origin; unfreeze with `gadak config set frozen false`")

// Connected builds a client for an explicit site/email/token — verifying a
// credential the user just typed, not "this workspace's origin".
func Connected(site, email, token string) *jira.Client {
	return jira.New(site, email, token)
}

// Client is the single owner of "this workspace's Jira client".
// A connected workspace gets the same jira.New(site, email, token) as before.
// A local-origin workspace embeds issuetap over the persist SQLite file
// (WAL). A paired remote workspace talks to the home serve's RESTPrefix
// passthrough. BaseURL stays empty so stored browse links are /browse/KEY
// rather than a fake https origin a person might click.
func Client(cfg *config.Config) (*jira.Client, error) {
	if cfg == nil {
		return nil, errors.New("origin: nil config")
	}
	if cfg.SyncFrozen() {
		return nil, ErrWorkspaceFrozen
	}
	if rem, err := pairedRemote(cfg); err != nil {
		return nil, err
	} else if rem != nil {
		return pairedJira(cfg, rem)
	}
	if cfg.HasLocalOrigin() {
		return localOriginClient(cfg)
	}
	if cfg.Site == "" || cfg.Email == "" || cfg.Token == "" {
		return nil, errNeedCredential
	}
	return Connected(cfg.Site, cfg.Email, cfg.Token), nil
}

// PairedStatus is the single owner of "is this workspace paired with a
// remote gadak serve?". status, doctor, profiles, and pairing list read
// this instead of opening remote-origin.json themselves. Local-origin is
// excluded: the same file on the home machine only carries the local
// pairing-gate token (`_home`), not a remote origin.
func PairedStatus(cfg *config.Config) (*pairing.Remote, error) {
	return pairedRemote(cfg)
}

// pairedRemote resolves the stored pairing credential that makes this
// workspace's origin a remote gadak serve (GDK-433). Local-origin is
// excluded on purpose: a local-origin workspace owns a local persist, and
// on the home machine the same file only ever carries the local
// pairing-gate token (`_home`) — there it must not flip Client into a
// remote client. A malformed file is an error, not a fallthrough to the
// connected path: silently treating a paired workspace as credential-less
// would answer errNeedCredential, which points the user at the wrong fix.
func pairedRemote(cfg *config.Config) (*pairing.Remote, error) {
	if cfg == nil || cfg.HasLocalOrigin() {
		return nil, nil
	}
	dir, err := profileDir(cfg)
	if err != nil {
		return nil, err
	}
	rem, err := pairing.LoadRemote(dir)
	if err != nil {
		return nil, fmt.Errorf("origin: paired workspace: %w", err)
	}
	return rem, nil
}

// pairedJira builds the Jira client for a paired workspace: the remote
// serve's REST passthrough with the device token as Bearer. BaseURL stays
// empty for the same reason as local-origin — the endpoint is an API target,
// not a site a person browses. The process's actor (GDK-586) rides along:
// the home serve's pairing gate rewrites Authorization but forwards
// X-Issuetap-Actor, so a remote agent's writes attribute to it, not to the
// home machine's identity.

// transportJira and transportWiki are the single owners of the injected
// transport assembly (GDK-619): a client built like a connected one, then
// rewired so every request rides tr — the paired serve's passthrough or the
// in-process issuetap handler. The in-process credential pair is a
// placeholder; tr carries the real authentication. jira.New and
// confluence.New always install an HTTP client, so there is no nil-HTTP
// case to defend.
func transportJira(tr http.RoundTripper) *jira.Client {
	c := Connected("", inProcessUser, inProcessSecret)
	c.HTTP.Transport = tr
	c.EnableNameCreatedVersions()
	return c
}

func transportWiki(tr http.RoundTripper) *confluence.Client {
	w := confluence.New("", inProcessUser, inProcessSecret)
	w.HTTP.Transport = tr
	return w
}

func pairedJira(cfg *config.Config, rem *pairing.Remote) (*jira.Client, error) {
	tr, err := newRemoteOriginTransport(rem.Endpoint, rem.Token)
	if err != nil {
		return nil, err
	}
	if a, ok := config.ResolveActor(cfg); ok {
		tr.actor, tr.actorName = a.Slug, a.Name
	}
	c := transportJira(tr)
	return c, nil
}

// pairedWiki is Wiki's paired twin: the serve's Confluence passthrough
// with the same Bearer and the same actor stamp.
func pairedWiki(cfg *config.Config, rem *pairing.Remote) (*confluence.Client, error) {
	tr, err := newRemoteOriginTransport(rem.Endpoint, rem.Token)
	if err != nil {
		return nil, err
	}
	if a, ok := config.ResolveActor(cfg); ok {
		tr.actor, tr.actorName = a.Slug, a.Name
	}
	w := transportWiki(tr)
	return w, nil
}

// VerifyPaired proves a pairing offer before anything is saved (GDK-433
// verify-before-save): one GET /rest/api/3/myself over the exact transport
// the paired workspace would use. A serve that answers 401 surfaces as
// jira.ErrAuth; an unreachable or broken endpoint surfaces as a transport
// error — the caller tells those apart without retrying. The offer string
// itself never enters an error. Deliberately no actor header (GDK-586):
// verification must not provision an agent identity on an origin the
// workspace does not exist on yet.
func VerifyPaired(ctx context.Context, endpoint, token string) (jira.User, error) {
	tr, err := newRemoteOriginTransport(endpoint, token)
	if err != nil {
		return jira.User{}, err
	}
	c := transportJira(tr)
	return c.Myself(ctx)
}

// Describe answers doctor: which kind of workspace, and where the origin is.
// Connected reports "jira" — "jira+linear" when the Linear source is on
// (no hostname or key either way; doctor is safe to paste).
// Local-origin reports the persist path.
func Describe(cfg *config.Config) (kind, origin string) {
	if cfg == nil || !cfg.HasLocalOrigin() {
		if cfg != nil && cfg.Linear != nil {
			return config.KindConnected, "jira+linear"
		}
		return config.KindConnected, "jira"
	}
	dir, err := profileDir(cfg)
	if err != nil {
		return config.KindLocalOrigin, PersistRel
	}
	return config.KindLocalOrigin, PersistPath(dir)
}

type session struct {
	emb    *issuetap.Embedded
	client *jira.Client
	wiki   *confluence.Client
	// locale is what the embedded store was last set to (GDK-597). Guarded
	// by mu so a racing session lookup cannot half-see a switch.
	locale string
}

type sessionFlight struct {
	done chan struct{}
	s    *session
	err  error
}

var (
	// mu guards live, flights, and inProcessPersist. Contract: no IO under
	// this lock. The critical section is a map lookup or insert; MkdirAll,
	// issuetap.NewEmbedded (persist read/write), and Embedded.Close run
	// with the lock released. This mutex is process-global and keyed by
	// persist path — holding it across persist IO queues every local-origin
	// workspace behind one disk.
	mu               sync.Mutex
	live             = map[string]*session{}
	flights          = map[string]*sessionFlight{}
	inProcessPersist = map[string]bool{}

	sessionsConstructed atomic.Uint64
)

// SessionsConstructed is how many times constructLocalOrigin ran. Tests
// use a delta to prove a live session was reused.
func SessionsConstructed() uint64 { return sessionsConstructed.Load() }

// SetInProcess marks/unmarks this process as the persist owner of cfg's
// workspace. Keyed by persist path — the same key as live — so owning one
// workspace's persist does not disable routing for any other (STD-3).
func SetInProcess(cfg *config.Config, v bool) {
	p := persistKeyOf(cfg)
	if p == "" {
		return
	}
	mu.Lock()
	if v {
		inProcessPersist[p] = true
	} else {
		delete(inProcessPersist, p)
	}
	mu.Unlock()
}

// IsInProcess reports whether this process owns cfg's persist.
func IsInProcess(cfg *config.Config) bool {
	p := persistKeyOf(cfg)
	if p == "" {
		return false
	}
	mu.Lock()
	owned := inProcessPersist[p]
	mu.Unlock()
	return owned
}

// ResetInProcess clears every ownership mark. Tests only — production
// unmarks per workspace (Runtime.Close / closeEntry).
func ResetInProcess() {
	mu.Lock()
	inProcessPersist = map[string]bool{}
	mu.Unlock()
}

// ForgetLive drops cached embedded sessions without closing them. Tests
// use this to simulate a second process: the serve handler still holds
// the graph, but Client no longer finds it in this process.
func ForgetLive() {
	mu.Lock()
	for _, s := range live {
		forgotten = append(forgotten, s)
	}
	live = map[string]*session{}
	flights = map[string]*sessionFlight{}
	mu.Unlock()
}

// forgotten keeps ForgetLive's dropped sessions reachable so tests that
// simulate a second process do not Close the first embedding via GC.
// Production never calls ForgetLive; sessions stay in live for the
// process lifetime.
var forgotten []*session

func localOriginClient(cfg *config.Config) (*jira.Client, error) {
	s, err := localOriginSession(cfg)
	if err != nil {
		return nil, err
	}
	return s.client, nil
}

// LocalOriginHandler is the in-process issuetap HTTP surface for this
// workspace. The serve RESTPrefix passthrough uses it so a paired remote
// client lands on the same origin the UI already holds. Always embeds.
func LocalOriginHandler(cfg *config.Config) (http.Handler, error) {
	s, err := localOriginSession(cfg)
	if err != nil {
		return nil, err
	}
	if s == nil || s.emb == nil {
		return nil, errors.New("origin: local-origin handler is missing")
	}
	return s.emb, nil
}

// LinearEndpoint, when non-empty, is the GraphQL URL Linear() installs on
// the client. Tests point it at httptest; production leaves it empty so New
// keeps linear.Endpoint. This is not a config.json field — an install
// URL must not become a persisted setting.
var LinearEndpoint string

// Linear is the single owner of "this workspace's Linear client" — the same
// role Wiki plays for Confluence (GDK-258: a third source beside the Jira
// client, never a facade behind its Transport). There is no local-origin
// variant: issuetap has no Linear surface, and the block carries its own
// credential rather than the Atlassian one.
func Linear(cfg *config.Config) (*linear.Client, error) {
	if cfg == nil || cfg.Linear == nil {
		return nil, errors.New("origin: linear is not configured")
	}
	if cfg.SyncFrozen() {
		return nil, ErrWorkspaceFrozen
	}
	if cfg.Linear.APIKey == "" {
		return nil, errors.New("origin: linear api key is required")
	}
	c := linear.New(cfg.Linear.APIKey)
	if LinearEndpoint != "" {
		c.Endpoint = LinearEndpoint
	}
	return c, nil
}

// Wiki is the single owner of "this workspace's Confluence client".
// A connected workspace gets confluence.New(site, email, token).
// A local-origin workspace shares the in-process issuetap handler with Client.
func Wiki(cfg *config.Config) (*confluence.Client, error) {
	if cfg == nil {
		return nil, errors.New("origin: nil config")
	}
	if cfg.SyncFrozen() {
		return nil, ErrWorkspaceFrozen
	}
	if rem, err := pairedRemote(cfg); err != nil {
		return nil, err
	} else if rem != nil {
		return pairedWiki(cfg, rem)
	}
	if cfg.HasLocalOrigin() {
		return localOriginWiki(cfg)
	}
	if cfg.Site == "" || cfg.Email == "" || cfg.Token == "" {
		if cfg.HasLinearCredential() && cfg.Site == "" {
			return nil, errNoWikiOrigin
		}
		return nil, errNeedCredential
	}
	return confluence.New(cfg.Site, cfg.Email, cfg.Token), nil
}

func localOriginWiki(cfg *config.Config) (*confluence.Client, error) {
	s, err := localOriginSession(cfg)
	if err != nil {
		return nil, err
	}
	return s.wiki, nil
}

// testBeforeLocalOrigin, if set, runs after the live-session lookup and before
// MkdirAll / issuetap.NewEmbedded. Tests use it as a barrier to prove the
// process-global mutex is not held across persist IO. Production is nil.
var testBeforeLocalOrigin func(persist string)

// localOriginSession returns this workspace's embedded origin session with
// the store speaking the workspace locale (GDK-597). The locale is part of
// the session contract, not just of construction: a config change must
// reach the already-live store in place — dropping the session would close
// the store a long-lived `gadak serve` is holding.
func localOriginSession(cfg *config.Config) (*session, error) {
	s, err := openLocalOriginSession(cfg)
	if err != nil {
		return nil, err
	}
	loc := "en"
	if cfg != nil {
		loc = cfg.EffectiveLocale()
	}
	mu.Lock()
	if s.locale == loc {
		mu.Unlock()
		return s, nil
	}
	prev, target := s.locale, loc
	s.locale = loc // reserve under mu so a racing caller skips the store call
	mu.Unlock()
	if err := s.emb.SetLocale(target); err != nil {
		mu.Lock()
		s.locale = prev
		mu.Unlock()
		return nil, fmt.Errorf("origin: set locale %q: %w", target, err)
	}
	return s, nil
}

func openLocalOriginSession(cfg *config.Config) (*session, error) {
	dir, err := profileDir(cfg)
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, errors.New("origin: local-origin workspace has no profile directory")
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

	if testBeforeLocalOrigin != nil {
		testBeforeLocalOrigin(persist)
	}

	var projects []string
	var locale string
	if cfg != nil {
		projects = cfg.Projects
		locale = cfg.EffectiveLocale()
	} else {
		locale = "en"
	}
	actor, _ := config.ResolveActor(cfg)
	s, err := constructLocalOrigin(persist, projects, actor, locale)

	mu.Lock()
	delete(flights, persist)
	if err != nil {
		f.err = err
		close(f.done)
		mu.Unlock()
		return nil, err
	}
	if existing, ok := live[persist]; ok {
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
	// Outside the lock: mu's contract is no IO under it, and this is a file
	// write. Advisory only — see openmark.go. Nothing here can fail the open.
	markOpen(persist)
	return s, nil
}

// constructLocalOrigin embeds issuetap over persist. actor is the process's
// resolved acting identity (GDK-586): when set, the session's transport
// stamps X-Issuetap-Actor on every request so writes attribute to the
// agent, not the in-process user. Resolution happens once per session —
// env and config are stable for the process lifetime.
// locale is the workspace's display-name language (GDK-597), always
// non-empty: passing it explicitly keeps the persist file's own locale
// field from winning — gadak owns the workspace language; the persist is
// the origin's state.
func constructLocalOrigin(persist string, projects []string, actor config.ResolvedActor, locale string) (*session, error) {
	sessionsConstructed.Add(1)
	if err := os.MkdirAll(filepath.Dir(persist), 0o700); err != nil {
		return nil, fmt.Errorf("origin: persist dir: %w", err)
	}

	fixturePath, fixtureBytes := selectLocalOriginSeed(persist, projects)
	emb, err := issuetap.NewEmbedded(issuetap.EmbeddedConfig{
		PersistPath:  persist,
		FixturePath:  fixturePath,
		FixtureBytes: fixtureBytes,
		// A local-origin workspace is a real tracker: records carry wall
		// time, not issuetap's deterministic seed clock (GDK-369 — a
		// January created_at read as a sync bug).
		WallClock: true,
		// …and it speaks the workspace's language for display names, with
		// Cloud fidelity: priority names stay English (GDK-597).
		Locale: locale,
	})
	if err != nil {
		return nil, fmt.Errorf("origin: issuetap: %w", err)
	}

	tr := &handlerTransport{h: emb, actor: actor.Slug, actorName: actor.Name}

	c := transportJira(tr)
	w := transportWiki(tr)

	return &session{emb: emb, client: c, wiki: w, locale: locale}, nil
}

func closeSession(s *session) {
	if s == nil {
		return
	}
	if s.emb != nil {
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

func persistKeyOf(cfg *config.Config) string {
	dir, err := profileDir(cfg)
	if err != nil || dir == "" {
		return ""
	}
	return PersistPath(dir)
}

// Close checkpoints every live local-origin origin (WAL) and drops the
// sessions. Safe to call more than once. The process owner (cmd/gadak
// main) calls this on the way out. Writes commit before ACK; Close is a
// checkpoint, not a debounce flush.
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
	for persist, s := range snapshot {
		if s == nil {
			continue
		}
		// Every exit path has to drop the marker, not just CloseLocalOrigin:
		// this is the one the CLI actually takes, and hooking only the other
		// left a dead PID's marker in every workspace after every command.
		// Liveness still made that harmless, but a stale PID is one reuse
		// away from refusing a conversion nobody is holding.
		clearOpen(persist)
		if s.emb != nil {
			if err := s.emb.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

// CloseLocalOrigin checkpoints and drops the live session for cfg's persist.
// Waits an in-flight constructor for the same key. No-op when nothing is
// live. Callers that marked SetInProcess unmark it themselves — this only
// owns the session.
func CloseLocalOrigin(cfg *config.Config) error {
	p := persistKeyOf(cfg)
	if p == "" {
		return nil
	}
	for {
		mu.Lock()
		if f, ok := flights[p]; ok {
			mu.Unlock()
			<-f.done
			continue
		}
		s := live[p]
		delete(live, p)
		mu.Unlock()
		if s == nil {
			return nil
		}
		clearOpen(p)
		if s.emb != nil {
			return s.emb.Close()
		}
		return nil
	}
}

// selectLocalOriginSeed follows issuetap's load order: an existing SQLite
// persist is the graph (no fixture); else a sibling legacy YAML is
// FixturePath (one-shot seed, file left in place); else FixtureBytes from
// localOriginFixture.
func selectLocalOriginSeed(persist string, projects []string) (fixturePath string, fixtureBytes []byte) {
	if persist != "" {
		if _, err := os.Stat(persist); err == nil {
			return "", nil
		}
	}
	if persist != "" {
		yamlPath := filepath.Join(filepath.Dir(persist), filepath.Base(filepath.FromSlash(LegacyYAMLRel)))
		if _, err := os.Stat(yamlPath); err == nil {
			return yamlPath, nil
		}
	}
	return "", localOriginFixture(projects)
}

// localOriginFixture is applied only when PersistPath does not yet exist
// and there is no sibling legacy YAML. It names the requested projects
// (or DefaultProjectKey when the list is empty) so createmeta/create have
// a target, and one space so page create has a target; it does not seed
// issues or pages. When the persist file is present, issuetap skips this.
//
// Keys come from DefaultProjectKey / DefaultSpaceKey so the literals are
// not scattered.
func localOriginFixture(projects []string) []byte {
	keys := make([]string, 0, len(projects))
	seen := map[string]bool{}
	for _, k := range projects {
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		keys = []string{DefaultProjectKey}
	}
	var b []byte
	b = append(b, "projects:\n"...)
	for i, key := range keys {
		name := "Local-origin"
		if key != DefaultProjectKey {
			name = key
		}
		b = append(b, fmt.Sprintf("  - id: %q\n    key: %q\n    name: %q\n    type: software\n    style: classic\n",
			fmt.Sprintf("%d", 10000+i), key, name)...)
	}
	b = append(b, "spaces:\n  - id: \"40000\"\n    key: "+DefaultSpaceKey+"\n    name: Local\n    type: global\n"...)
	return b
}

// CurrentDescription reads KEY's description as the origin holds it right
// now — never the mirror, which is a stale cache while a decision about the
// write's next instant is being made (GDK-1001, GDK-1396: the markdown
// loss guard and placeholder substitution both work from this). found is
// false when the origin returned no row for the key; the write that follows
// is then the authority on whether the issue exists.
func CurrentDescription(ctx context.Context, cfg *config.Config, key string) (raw json.RawMessage, found bool, err error) {
	c, err := Client(cfg)
	if err != nil {
		return nil, false, err
	}
	err = c.Search(ctx, fmt.Sprintf("key = %q", key), []string{"description"}, false, func(issues []jira.Issue) error {
		for _, iss := range issues {
			if iss.Key != key {
				continue
			}
			found = true
			raw = iss.Fields.Description
			return nil
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return raw, found, nil
}
