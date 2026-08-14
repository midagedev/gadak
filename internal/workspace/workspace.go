// Package workspace mounts additional gadak profiles under /w/<name>/ and lists
// them at GET /api/v1/workspaces. Shared by `gadak serve` and the desktop app.
package workspace

import (
	"context"
	"encoding/json"
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

// watchRescanInterval is how often WatchAll re-scans profiles for a credential
// that appeared after boot (CLI `gadak init` while serve is already running).
// Onboarding does not wait for this: the workspace handler's sync starter
// calls EnsureWatch as soon as connect saves a token.
const watchRescanInterval = 30 * time.Second

// workspaceNameRe is the only allowed shape for /w/<name>/ segments.
var workspaceNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Entry is one opened workspace mirror (handler + DB + config).
type Entry struct {
	Handler http.Handler
	DB      *store.DB
	Cfg     *config.Config
}

// Registry lazy-opens profile mirrors on first /w/<name>/ request and closes
// them when the process exits.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*Entry

	// Watch ownership (D8): one loop per credentialed non-primary profile.
	// WatchAll arms these; EnsureWatch / the rescan ticker start loops when
	// a credential appears later. watching prevents a double start.
	watching      map[string]bool
	watchCtx      context.Context
	watchLogf     func(string)
	watchPrimary  string
	rescanStarted bool
}

// New returns an empty workspace registry.
func New() *Registry {
	return &Registry{entries: make(map[string]*Entry)}
}

// Close closes every opened workspace DB. Safe to call once at shutdown.
func (r *Registry) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, e := range r.entries {
		if e != nil && e.DB != nil {
			_ = e.DB.Close()
		}
		delete(r.entries, name)
	}
}

// Get returns a cached workspace entry, opening the profile on first use.
func (r *Registry) Get(name string) (*Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[name]; ok {
		return e, nil
	}
	e, err := r.openLocked(name)
	if err != nil {
		return nil, err
	}
	// First open after CLI init: start Watch if a credential is already on disk
	// and WatchAll has armed the owner. Per-request path after this is a map hit.
	r.ensureWatchLocked(name)
	return e, nil
}

// openLocked creates and caches a workspace entry. Caller holds r.mu.
func (r *Registry) openLocked(name string) (*Entry, error) {
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
	e := &Entry{Handler: h, DB: db, Cfg: cfg}
	r.entries[name] = e
	return e, nil
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
		primaryName := primary
		if primaryName == "" {
			primaryName = "default"
		}

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
	if a == "" {
		a = "default"
	}
	if b == "" {
		b = "default"
	}
	return a == b
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
func (r *Registry) EnsureWatch(name string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ensureWatchLocked(name)
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

// ensureWatchLocked starts one Watch. Caller holds r.mu.
func (r *Registry) ensureWatchLocked(name string) bool {
	if r.watchCtx == nil || r.watchCtx.Err() != nil {
		return false
	}
	if sameProfile(name, r.watchPrimary) {
		return false
	}
	if r.watching[name] {
		return false
	}
	// Credential first, mirror second. Opening the database runs migrations —
	// a profile that will never sync should not have its file rewritten just
	// to learn it has no token.
	cfg, err := config.LoadFor(name)
	if err != nil || !cfg.HasCredential() {
		return false
	}
	e := r.entries[name]
	if e == nil {
		e, err = r.openLocked(name)
		if err != nil {
			if r.watchLogf != nil {
				r.watchLogf("workspace " + name + ": open failed: " + err.Error())
			}
			return false
		}
	}
	if e.DB == nil {
		return false
	}
	if r.watching == nil {
		r.watching = map[string]bool{}
	}
	r.watching[name] = true
	ctx, logf, db := r.watchCtx, r.watchLogf, e.DB
	profileName := name
	go func() {
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
		if err := syncer.Watch(ctx, cur, db, opts); err != nil && ctx.Err() == nil {
			if logf != nil {
				logf("workspace " + profileName + ": sync loop stopped: " + err.Error())
			}
		}
	}()
	if logf != nil {
		logf("workspace " + profileName + ": watch started")
	}
	return true
}
