package main

import (
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
)

// workspaceNameRe is the only allowed shape for /w/<name>/ segments.
var workspaceNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type wsEntry struct {
	handler http.Handler
	db      *store.DB
	cfg     *config.Config
}

// workspaceRegistry lazy-opens profile mirrors on first /w/<name>/ request and
// closes them when the serve process exits.
type workspaceRegistry struct {
	mu      sync.Mutex
	entries map[string]*wsEntry
}

func newWorkspaceRegistry() *workspaceRegistry {
	return &workspaceRegistry{entries: make(map[string]*wsEntry)}
}

// Close closes every opened workspace DB. Safe to call once at shutdown.
func (r *workspaceRegistry) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, e := range r.entries {
		if e != nil && e.db != nil {
			_ = e.db.Close()
		}
		delete(r.entries, name)
	}
}

// get returns a cached workspace entry, opening the profile on first use.
func (r *workspaceRegistry) get(name string) (*wsEntry, error) {
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
	e := &wsEntry{handler: h, db: db, cfg: cfg}
	r.entries[name] = e
	return e, nil
}

// profileExists reports whether name is a directory under profiles/ (disk is
// the source of truth for workspace mounts).
func profileExists(name string) bool {
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

// handleWorkspace routes /w/<name>/… to the named profile's API, config, healthz,
// or SPA. Invalid names and unknown profiles answer 404.
func (r *workspaceRegistry) handleWorkspace(spa http.Handler) http.HandlerFunc {
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
		if !workspaceNameRe.MatchString(name) || !profileExists(name) {
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

		entry, err := r.get(name)
		if err != nil {
			log.Printf("workspace %s unavailable: %v", name, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "workspace_unavailable"})
			return
		}

		if strings.HasPrefix(rest, "api/") {
			http.StripPrefix(prefix, entry.handler).ServeHTTP(w, req)
			return
		}
		// SPA for everything else under /w/<name>/
		http.StripPrefix(prefix, spa).ServeHTTP(w, req)
	}
}

// workspaceListEntry is one item in GET /api/v1/workspaces. Never carries
// Token or Email — only site URL and project keys for the picker.
type workspaceListEntry struct {
	Name     string   `json:"name"`
	Site     string   `json:"site,omitempty"`
	Projects []string `json:"projects,omitempty"`
	Active   bool     `json:"active,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// handleListWorkspaces answers GET /api/v1/workspaces with the primary profile
// first (active:true) and every named profile after. Credentials never appear.
func handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	primary := config.Profile()
	primaryName := primary
	if primaryName == "" {
		primaryName = "default"
	}

	var list []workspaceListEntry

	// Active (serve) profile first.
	if cfg, err := config.LoadFor(primary); err != nil {
		list = append(list, workspaceListEntry{Name: primaryName, Active: true, Error: "unreadable"})
	} else {
		list = append(list, workspaceListEntry{
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
			list = append(list, workspaceListEntry{Name: name, Error: "unreadable"})
			continue
		}
		list = append(list, workspaceListEntry{
			Name:     name,
			Site:     cfg.Site,
			Projects: cfg.Projects,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"workspaces": list})
}

// buildServeMux wires the serve HTTP tree: primary API + SPA, workspace mounts,
// and the workspace list. Extracted so tests can exercise routing without a
// real listener or flag parse.
func buildServeMux(primaryAPI http.Handler, spa http.Handler, reg *workspaceRegistry) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": version})
	})
	// PUT settings/ rewrites the config on disk, so re-read it per request.
	mux.HandleFunc("/config.json", func(w http.ResponseWriter, r *http.Request) {
		cur, err := config.Load()
		if err != nil {
			http.Error(w, `{"error":"config_unreadable"}`, http.StatusInternalServerError)
			return
		}
		doc, err := server.WebConfig(cur)
		if err != nil {
			http.Error(w, `{"error":"config_unreadable"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(doc)
	})
	// More specific than /api/ so the list is not swallowed by the primary handler.
	mux.HandleFunc("GET /api/v1/workspaces", handleListWorkspaces)
	mux.HandleFunc("GET /api/v1/workspaces/{$}", handleListWorkspaces)
	mux.Handle("/api/", primaryAPI)
	if reg != nil {
		mux.HandleFunc("/w/", reg.handleWorkspace(spa))
	}
	mux.Handle("/", spa)
	return mux
}
