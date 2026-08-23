// Package workspace mounts additional gadak profiles under /w/<name>/ and lists
// them at GET /api/v1/workspaces. Shared by `gadak serve` and the desktop app.
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// errRegistryClosed is Get after Close. Close is process-exit; a late
// constructor must not publish into a registry that has already snapped.
var errRegistryClosed = errors.New("workspace: registry closed")

// watchRescanInterval is how often WatchAll re-scans profiles for a credential
// that appeared after boot (CLI `gadak init` while serve is already running).
// Onboarding does not wait for this: the workspace handler's sync starter
// calls EnsureWatch as soon as connect saves a token.
const watchRescanInterval = 30 * time.Second

// workspaceNameRe is the only allowed shape for /w/<name>/ segments.
var workspaceNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Entry is one opened workspace mirror (handler + DB + config).
type Entry struct {
	// Handler is the concrete server handler, not http.Handler: this entry
	// owns its lifetime, and Close has to be able to stop the background sync
	// before the mirror it writes to is closed (GDK-270).
	Handler *server.Handler
	DB      *store.DB
	Cfg     *config.Config
}

// Registry lazy-opens profile mirrors on first /w/<name>/ request and closes
// them when the process exits.
type Registry struct {
	// mu guards entries, flights, watching, closed, and the watch-owner
	// fields. Contract: no IO under this lock. The critical section is a
	// map lookup or insert; construction (config.LoadFor, store.Open,
	// attachcache.New, server.NewWorkspace) and Close of discarded or
	// snapshotted entries run with the lock released. Holding it across
	// disk IO serialises every other profile's Get/EnsureWatch/Close
	// behind one migration.
	mu      sync.Mutex
	entries map[string]*Entry
	flights map[string]*openFlight
	closed  bool

	// Watch ownership (D8): one loop per credentialed non-primary profile.
	// WatchAll arms these; EnsureWatch / the rescan ticker start loops when
	// a credential appears later. watching prevents a double start.
	watching      map[string]bool
	watchCtx      context.Context
	watchLogf     func(string)
	watchPrimary  string
	rescanStarted bool
}

// openFlight is one in-flight construct for a profile name. Same-key racers
// wait on done instead of calling store.Open twice (migrate is not safe
// against one file from two Open calls at once).
type openFlight struct {
	done chan struct{}
	e    *Entry
	err  error
}

// New returns an empty workspace registry.
func New() *Registry {
	return &Registry{
		entries: make(map[string]*Entry),
		flights: make(map[string]*openFlight),
	}
}

// testBeforeConstruct, if set, runs at the start of construction. Tests use
// it as a barrier to prove the registry mutex is not held across store.Open.
// Production is nil (one pointer compare on the miss path).
var testBeforeConstruct func(name string)

// Close closes every opened workspace DB. Safe to call once at shutdown.
// In-flight constructors are waited on (they see closed and close their
// own object) so this never closes a half-built entry.
func (r *Registry) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.closed = true
	wait := make([]chan struct{}, 0, len(r.flights))
	for _, f := range r.flights {
		wait = append(wait, f.done)
	}
	r.mu.Unlock()
	for _, done := range wait {
		<-done
	}
	r.mu.Lock()
	snapshot := r.entries
	r.entries = make(map[string]*Entry)
	r.mu.Unlock()
	for _, e := range snapshot {
		closeEntry(e)
	}
}

func closeEntry(e *Entry) {
	if e == nil {
		return
	}
	// Stop this workspace's background sync before closing the mirror
	// it writes to; a job that outlives the DB holds a WAL connection
	// (GDK-270).
	if e.Handler != nil {
		_ = e.Handler.Close()
	}
	if e.DB != nil {
		_ = e.DB.Close()
	}
}

// Get returns a cached workspace entry, opening the profile on first use.
func (r *Registry) Get(name string) (*Entry, error) {
	if r == nil {
		return nil, errRegistryClosed
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errRegistryClosed
	}
	if e, ok := r.entries[name]; ok {
		r.mu.Unlock()
		return e, nil
	}
	r.mu.Unlock()
	e, err := r.open(name)
	if err != nil {
		return nil, err
	}
	// First open after CLI init: start Watch if a credential is already on disk
	// and WatchAll has armed the owner. Per-request path after this is a map hit.
	r.EnsureWatch(name)
	return e, nil
}

// open is lookup → unlock → construct → re-lock and publish. Same-name
// racers share one store.Open (migrate is not safe twice on one file).
func (r *Registry) open(name string) (*Entry, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errRegistryClosed
	}
	if e, ok := r.entries[name]; ok {
		r.mu.Unlock()
		return e, nil
	}
	if f, ok := r.flights[name]; ok {
		r.mu.Unlock()
		<-f.done
		return f.e, f.err
	}
	f := &openFlight{done: make(chan struct{})}
	if r.flights == nil {
		r.flights = map[string]*openFlight{}
	}
	r.flights[name] = f
	r.mu.Unlock()

	if testBeforeConstruct != nil {
		testBeforeConstruct(name)
	}

	e, err := r.construct(name)

	r.mu.Lock()
	delete(r.flights, name)
	if err != nil {
		f.err = err
		close(f.done)
		r.mu.Unlock()
		return nil, err
	}
	if r.closed {
		r.mu.Unlock()
		closeEntry(e)
		f.err = errRegistryClosed
		close(f.done)
		return nil, errRegistryClosed
	}
	if existing, ok := r.entries[name]; ok {
		r.mu.Unlock()
		closeEntry(e)
		f.e = existing
		close(f.done)
		return existing, nil
	}
	r.entries[name] = e
	f.e = e
	close(f.done)
	r.mu.Unlock()
	return e, nil
}

// construct builds an Entry with no lock held. Caller publishes or closes it.
func (r *Registry) construct(name string) (*Entry, error) {
	cfg, err := config.LoadFor(name)
	if err != nil {
		return nil, err
	}
	dbPath, err := config.DBPathFor(name)
	if err != nil {
		return nil, err
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	var cache *attachcache.Cache
	if dir, err := config.AttachmentDirFor(name); err == nil {
		if c, err := attachcache.New(dir, int64(cfg.AttachmentCacheMB)<<20); err == nil {
			cache = c
		} else {
			log.Printf("workspace %s: attachment cache disabled: %v", name, err)
		}
	}
	h := server.NewWorkspace(db, cfg, cache, name)
	profileName := name
	// Same seam as the primary serve handler: first successful onboarding
	// connect starts this profile's Watch exactly once (server-side Once).
	h.SetSyncStarter(func() { r.EnsureWatch(profileName) })
	return &Entry{Handler: h, DB: db, Cfg: cfg}, nil
}

// ProfileExists reports whether name is a directory under profiles/ (disk is
// the source of truth for workspace mounts).
func ProfileExists(name string) bool {
	names, err := config.Profiles()
	if err != nil {
		return false
	}
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// Handler routes /w/<name>/… to the named profile's API, config, healthz, or
// SPA. Invalid names and unknown profiles answer 404. version is reported in
// healthz (same field as the primary /healthz).
func (r *Registry) Handler(spa http.Handler, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/w/")
		if path == "" || path == req.URL.Path {
			http.NotFound(w, req)
			return
		}
		name, rest, cut := strings.Cut(path, "/")
		if !cut {
			name = path
			rest = ""
		}
		if !workspaceNameRe.MatchString(name) || !ProfileExists(name) {
			http.NotFound(w, req)
			return
		}

		prefix := "/w/" + name

		// Paths that do not need the mirror open yet.
		switch {
		case rest == "healthz" || rest == "healthz/":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": version})
			return
		case rest == "config.json":
			cur, err := config.LoadFor(name)
			if err != nil {
				http.Error(w, `{"error":"config_unreadable"}`, http.StatusInternalServerError)
				return
			}
			doc, err := server.WebConfigBase(cur, prefix)
			if err != nil {
				http.Error(w, `{"error":"config_unreadable"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(doc)
			return
		}

		entry, err := r.Get(name)
		if err != nil {
			log.Printf("workspace %s unavailable: %v", name, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "workspace_unavailable"})
			return
		}

		if strings.HasPrefix(rest, "api/") {
			http.StripPrefix(prefix, entry.Handler).ServeHTTP(w, req)
			return
		}
		// SPA for everything else under /w/<name>/
		http.StripPrefix(prefix, spa).ServeHTTP(w, req)
	}
}

// ListEntry is one item in GET /api/v1/workspaces. Never carries Token or
// Email — only site URL and project keys for the picker.
type ListEntry struct {
	Name     string   `json:"name"`
	Site     string   `json:"site,omitempty"`
	Projects []string `json:"projects,omitempty"`
	Active   bool     `json:"active,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// ListHandler answers GET /api/v1/workspaces with the primary profile first
// (active:true) and every named profile after. Credentials never appear.
func ListHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		primary := config.Profile()
		primaryName := config.NormalizeProfile(primary)

		var list []ListEntry

		// Active (serve) profile first.
		if cfg, err := config.LoadFor(primary); err != nil {
			list = append(list, ListEntry{Name: primaryName, Active: true, Error: "unreadable"})
		} else {
			list = append(list, ListEntry{
				Name:     primaryName,
				Site:     cfg.Site,
				Projects: cfg.Projects,
				Active:   true,
			})
		}

		names, err := config.Profiles()
		if err != nil {
			log.Printf("workspaces: list profiles: %v", err)
		}
		for _, name := range names {
			// Skip the primary when serve is running under --profile <name>.
			if name == primary || name == primaryName {
				continue
			}
			cfg, err := config.LoadFor(name)
			if err != nil {
				list = append(list, ListEntry{Name: name, Error: "unreadable"})
				continue
			}
			list = append(list, ListEntry{
				Name:     name,
				Site:     cfg.Site,
				Projects: cfg.Projects,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"workspaces": list})
	}
}

// sameProfile reports whether a and b name the same profile. The empty string
// and "default" are the same (config.Profile() returns "" for the default).
func sameProfile(a, b string) bool {
	return config.NormalizeProfile(a) == config.NormalizeProfile(b)
}

// WatchAll arms the credential-appearance owner and starts a loop for every
// profile that already has a credential, skipping primary (its caller already
// watches that one). Returns the profile names that got a loop. Subsequent
// credentials (onboarding connect, or a later EnsureWatches / rescan) start
// through the same owner and cannot double-start. Log receives status lines;
// never pass secrets into it — only profile names are logged from here.
func (r *Registry) WatchAll(ctx context.Context, primary string, logf func(string)) []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.watchCtx = ctx
	r.watchLogf = logf
	r.watchPrimary = primary
	if r.watching == nil {
		r.watching = map[string]bool{}
	}
	startRescan := !r.rescanStarted && ctx != nil
	if startRescan {
		r.rescanStarted = true
	}
	r.mu.Unlock()

	started := r.EnsureWatches()
	if startRescan {
		go r.rescanLoop(ctx)
	}
	return started
}

// EnsureWatches is the single "credential appeared" scan: start a Watch for
// every non-primary profile that now has a credential and does not already
// have a loop. Returns the names that were started this call.
func (r *Registry) EnsureWatches() []string {
	if r == nil {
		return nil
	}
	names, err := config.Profiles()
	if err != nil {
		r.mu.Lock()
		logf := r.watchLogf
		r.mu.Unlock()
		if logf != nil {
			logf("workspaces: list profiles: " + err.Error())
		}
		return nil
	}
	var started []string
	for _, name := range names {
		if r.EnsureWatch(name) {
			started = append(started, name)
		}
	}
	return started
}

// EnsureWatch starts this profile's Watch if WatchAll has armed the owner, the
// profile now has a credential, and no loop is already running. Idempotent.
// LoadFor and store.Open run with r.mu released.
func (r *Registry) EnsureWatch(name string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	if !r.watchReadyLocked(name) {
		r.mu.Unlock()
		return false
	}
	r.mu.Unlock()

	// Credential first, mirror second. Opening the database runs migrations —
	// a profile that will never sync should not have its file rewritten just
	// to learn it has no token.
	cfg, err := config.LoadFor(name)
	if err != nil || !cfg.HasCredential() {
		return false
	}
	// Frozen workspaces refuse pulls (GDK-181). Stay quiet: EnsureWatches is
	// on a rescan loop and a log line here would fire every scan.
	if cfg.SyncFrozen() {
		return false
	}

	e, err := r.open(name)
	if err != nil {
		r.mu.Lock()
		logf := r.watchLogf
		r.mu.Unlock()
		if logf != nil {
			logf("workspace " + name + ": open failed: " + err.Error())
		}
		return false
	}
	if e.DB == nil {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.watchReadyLocked(name) {
		return false
	}
	if r.watching == nil {
		r.watching = map[string]bool{}
	}
	r.watching[name] = true
	ctx, logf, db := r.watchCtx, r.watchLogf, e.DB
	profileName := name
	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.watching, profileName)
			r.mu.Unlock()
		}()
		opts := syncer.Options{
			Reload: func() (*config.Config, error) {
				return config.LoadFor(profileName)
			},
		}
		if logf != nil {
			// Prefix with profile name only — no site/email/token.
			opts.Log = func(s string) { logf("workspace " + profileName + ": " + s) }
		}
		cur := cfg
		if next, err := config.LoadFor(profileName); err == nil && next != nil {
			cur = next
		}
		// WatchLoop owns re-entry: Watch returning on fatal auth must not
		// leave watching[name] stuck, and must not hot-loop (GDK-541).
		syncer.WatchLoop(ctx, cur, db, opts)
	}()
	if logf != nil {
		logf("workspace " + profileName + ": watch started")
	}
	return true
}

// Watching returns the profile names whose Watch loop has been started.
// Debug surface for D8 (settings / tests); order is sorted.
func (r *Registry) Watching() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.watching))
	for name, on := range r.watching {
		if on {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (r *Registry) rescanLoop(ctx context.Context) {
	tick := time.NewTicker(watchRescanInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_ = r.EnsureWatches()
		}
	}
}

// watchReadyLocked reports whether EnsureWatch may proceed past the
// in-memory gates. Caller holds r.mu. Does not touch disk.
func (r *Registry) watchReadyLocked(name string) bool {
	if r.closed {
		return false
	}
	if r.watchCtx == nil || r.watchCtx.Err() != nil {
		return false
	}
	if sameProfile(name, r.watchPrimary) {
		return false
	}
	if r.watching[name] {
		return false
	}
	return true
}
