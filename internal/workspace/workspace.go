// Package workspace mounts additional scry profiles under /w/<name>/ and lists
// them at GET /api/v1/workspaces. Shared by `scry serve` and the desktop app.
package workspace

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/midagedev/scry/internal/attachcache"
	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/server"
	"github.com/midagedev/scry/internal/store"
	syncer "github.com/midagedev/scry/internal/sync"
)

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

// WatchAll starts one sync loop per profile that has a credential, skipping
// primary (its caller already watches that one). Returns the profile names
// that got a loop. Log receives status lines; never pass secrets into it —
// only profile names are logged from here.
func (r *Registry) WatchAll(ctx context.Context, primary string, logf func(string)) []string {
	if r == nil {
		return nil
	}
	names, err := config.Profiles()
	if err != nil {
		if logf != nil {
			logf("workspaces: list profiles: " + err.Error())
		}
		return nil
	}
	var started []string
	for _, name := range names {
		if sameProfile(name, primary) {
			continue
		}
		// Credential first, mirror second. Get() opens the database, which runs
		// migrations — a profile that will never sync should not have its file
		// rewritten every time the app starts just to learn it has no token.
		if cfg, err := config.LoadFor(name); err != nil || !cfg.HasCredential() {
			continue
		}
		entry, err := r.Get(name)
		if err != nil {
			if logf != nil {
				logf("workspace " + name + ": open failed: " + err.Error())
			}
			continue
		}
		if entry.Cfg == nil || !entry.Cfg.HasCredential() {
			continue
		}
		cfg, db := entry.Cfg, entry.DB
		profileName := name
		go func() {
			opts := syncer.Options{}
			if logf != nil {
				// Prefix with profile name only — no site/email/token.
				opts.Log = func(s string) { logf("workspace " + profileName + ": " + s) }
			}
			if err := syncer.Watch(ctx, cfg, db, opts); err != nil && ctx.Err() == nil {
				if logf != nil {
					logf("workspace " + profileName + ": sync loop stopped: " + err.Error())
				}
			}
		}()
		started = append(started, name)
	}
	return started
}
