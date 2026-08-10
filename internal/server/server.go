// Package server serves the HTTP read API the web UI already speaks
// (specs/000-product/contracts/api.md). Every response is assembled from the
// SQLite mirror; the one call that leaves this process is the attachment byte
// proxy, which is also the only endpoint that needs credentials.
//
// The server has no authentication: `scry serve` refuses a non-loopback bind
// instead. Personal-state endpoints therefore never answer 401/403.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/midagedev/scry/internal/attachcache"
	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/selfupdate"
	"github.com/midagedev/scry/internal/store"
)

// sourceID is the only connector v0.1 configures. Both the ETag and the sync
// health panel read its sync_state row.
// ponytail: one hardcoded source id; take a source list when a second connector
// lands and the ETag has to combine versions.
const sourceID = "jira"

// Base paths, mirrored in the config document the UI boots from. They are part
// of the contract: the client validates attachment URLs against apiBase.
const (
	apiBase  = "/api/v1/issues/"
	authBase = "/api/v1/auth/"
)

type server struct {
	db  *store.DB
	cfg atomic.Pointer[config.Config]
	// gen moves when PUT settings/ replaces cfg, which invalidates everything
	// derived from it.
	gen atomic.Int64

	// profile is the config profile this handler serves ("" = default/root).
	// Used by runtimeInfo so workspace handlers report their own paths.
	profile string

	mu     sync.Mutex
	cached *derivedView

	// cache holds attachment bytes on disk. nil disables caching and every view
	// proxies, which is the pre-cache behavior.
	cache *attachcache.Cache

	// updateMu protects the GitHub release snapshot used by bootstrap only.
	// Separate from mu so a release check never contends with derived views.
	updateMu   sync.Mutex
	updateInfo selfupdate.Info
	updateOK   bool

	// syncStarter starts the background sync loop after a first-run credential
	// is saved via PUT onboarding/connect/. Set once by cmdServe when serve
	// starts without a credential; fired at most once via syncStarterOnce.
	syncStarter     func()
	syncStarterOnce sync.Once

	// syncKick starts a background sync. Nil means the real one. Tests replace it
	// to assert that a settings write asks for a resync without doing one.
	syncKick func(cfg *config.Config, full bool) bool

	// Per-instance sync job and activity (not package-global): workspace mode
	// runs one server per profile in the same process, so each must track its own.
	syncMu   sync.Mutex
	syncJob  progressDoc
	activity mirrorActivity
}

// Handler is the HTTP API plus optional update-check control. It implements
// http.Handler; mount it at "/api/".
type Handler struct {
	mux *http.ServeMux
	s   *server
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Identity markers for a second `scry serve` that finds this port busy
	// (cmd/scry port fallback). Fixed marker + profile — no version injection
	// into constructors. Set before browserGuard so every response carries them.
	w.Header().Set("X-Scry", "1")
	w.Header().Set("X-Scry-Profile", h.s.profile)
	if !browserGuard(w, r) {
		return
	}
	h.mux.ServeHTTP(w, r)
}

// New returns the API handler. Mount it at "/api/" — the patterns below carry
// their full paths, so nothing strips a prefix.
func New(db *store.DB, cfg *config.Config) *Handler {
	return NewWithCache(db, cfg, nil)
}

// NewWithCache is New plus an attachment byte cache. `scry serve` passes one
// rooted under SCRY_HOME; tests pass nil when they do not exercise attachments.
func NewWithCache(db *store.DB, cfg *config.Config, cache *attachcache.Cache) *Handler {
	return newServer(db, cfg, cache, config.Profile())
}

// NewWorkspace is NewWithCache bound to a named profile (for /w/<name>/ mounts).
// profile is used for runtime paths and display; it does not re-read global config.
func NewWorkspace(db *store.DB, cfg *config.Config, cache *attachcache.Cache, profile string) *Handler {
	return newServer(db, cfg, cache, profile)
}

func newServer(db *store.DB, cfg *config.Config, cache *attachcache.Cache, profile string) *Handler {
	if cfg == nil {
		cfg = &config.Config{}
	}
	s := &server{db: db, cache: cache, profile: profile}
	s.cfg.Store(cfg)

	// Every pattern is anchored with {$}: a trailing slash alone would make each
	// literal endpoint a subtree that overlaps `{key}/detail/`, which ServeMux
	// rejects as ambiguous.
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+apiBase+"bootstrap/{$}", s.handleBootstrap)
	mux.HandleFunc("GET "+apiBase+"delta/{$}", s.handleDelta)
	mux.HandleFunc("GET "+apiBase+"search/{$}", s.handleSearch)
	// Confluence pages list (R2). Detail shares GET {key}/{action}/ below —
	// ServeMux rejects pages/{key}/ vs {key}/detail/ (both match pages/detail/).
	// ETag on the list is confluence sync_state.version only; jira bootstrap/
	// delta ETags are unchanged.
	mux.HandleFunc("GET "+apiBase+"pages/{$}", s.handlePages)
	mux.HandleFunc("GET "+apiBase+"settings/{$}", s.handleGetSettings)
	mux.HandleFunc("PUT "+apiBase+"settings/{$}", s.handlePutSettings)
	// Live Confluence space picker for Settings (scope written via PUT settings/).
	mux.HandleFunc("GET "+apiBase+"settings/spaces/{$}", s.handleSettingsSpaces)
	mux.HandleFunc("GET "+apiBase+"views/{$}", s.handleGetViews)
	mux.HandleFunc("POST "+apiBase+"views/{$}", s.handlePostView)
	mux.HandleFunc("DELETE "+apiBase+"views/{id}/{$}", s.handleDeleteView)
	mux.HandleFunc("GET "+apiBase+"watches/{$}", s.handleGetWatches)
	mux.HandleFunc("DELETE "+apiBase+"watches/{key}/{$}", s.handleDeleteWatch)
	mux.HandleFunc("GET "+apiBase+"favorites/{$}", s.handleGetFavorites)
	mux.HandleFunc("DELETE "+apiBase+"favorites/{key}/{$}", s.handleDeleteFavorite)
	// Personal feed (computed from the mirror; read receipts in feed_reads).
	// Literal patterns beat `{key}/{action}/` and `{key}/detail/`.
	mux.HandleFunc("GET "+apiBase+"feed/{$}", s.handleGetFeed)
	mux.HandleFunc("POST "+apiBase+"feed/read/{$}", s.handleMarkFeedRead)
	// People axis: comments by author (exact author_id). Three-segment path so it
	// does not collide with `{key}/{action}/`.
	mux.HandleFunc("GET "+apiBase+"people/{author_id}/comments/{$}", s.handlePeopleComments)
	// The client's two PUT shapes — `watches/{key}/` and `{key}/assignee/` — are
	// mirror images that overlap on `watches/assignee/`, and ServeMux rejects an
	// ambiguous pair with no way to break the tie (there is no third-pattern
	// exception). So they share one pattern and are told apart here.
	// favorites/{key}/ is the same shape; add a case, do not register a second PUT.
	mux.HandleFunc("PUT "+apiBase+"{key}/{action}/{$}", func(w http.ResponseWriter, r *http.Request) {
		switch key, action := r.PathValue("key"), r.PathValue("action"); {
		case key == "watches":
			s.setWatch(w, r, action, true)
		case key == "favorites":
			s.setFavorite(w, r, action, true)
		case action == "assignee":
			s.handleAssignee(w, r)
		default:
			handleNotFound(w, r)
		}
	})
	// GET two-segment fan-out: issue detail, page detail (pages/{id}), and the
	// write-meta GETs that share the same shape. Literal routes above and
	// three-segment attachment content below beat this pattern where they apply.
	mux.HandleFunc("GET "+apiBase+"{key}/{action}/{$}", func(w http.ResponseWriter, r *http.Request) {
		key, action := r.PathValue("key"), r.PathValue("action")
		switch {
		case key == "pages":
			// GET pages/{pageKey}/ — page detail (R2).
			s.handlePageDetailKey(w, r, action)
		case action == "detail":
			s.handleDetail(w, r)
		case action == "transitions":
			s.handleTransitions(w, r)
		case action == "editmeta":
			s.handleEditMeta(w, r)
		default:
			handleNotFound(w, r)
		}
	})
	mux.HandleFunc("GET "+apiBase+"{key}/attachments/{id}/content/{$}", s.handleAttachment)
	mux.HandleFunc("GET "+authBase+"me/{$}", s.handleMe)

	// Write-through (T4). Everything below calls Jira and then re-reads the issue.
	mux.HandleFunc("GET "+apiBase+"credential/{$}", s.handleGetCredential)
	mux.HandleFunc("PUT "+apiBase+"credential/{$}", s.handlePutCredential)
	mux.HandleFunc("DELETE "+apiBase+"credential/{$}", s.handleDeleteCredential)
	mux.HandleFunc("GET "+apiBase+"meta/write/{$}", s.handleWriteMeta)
	// First-run onboarding (onboarding.go). These literal patterns are more
	// specific than the `{key}/{action}/` pair above, so ServeMux prefers them.
	mux.HandleFunc("PUT "+apiBase+"onboarding/connect/{$}", s.handleConnect)
	mux.HandleFunc("GET "+apiBase+"projects/available/{$}", s.handleAvailableProjects)
	mux.HandleFunc("POST "+apiBase+"sync/{$}", s.handleStartSync)
	mux.HandleFunc("GET "+apiBase+"sync/progress/{$}", s.handleSyncProgress)
	mux.HandleFunc("GET "+apiBase+"sync/runs/{$}", s.handleSyncRuns)
	mux.HandleFunc("GET "+apiBase+"create-meta/{$}", s.handleCreateMeta)
	mux.HandleFunc("POST "+apiBase+"create/{$}", s.handleCreate)
	mux.HandleFunc("GET "+apiBase+"users/{$}", s.handleUsers)
	mux.HandleFunc("POST "+apiBase+"{key}/transition/{$}", s.handleTransition)
	mux.HandleFunc("POST "+apiBase+"{key}/comment/{$}", s.handleComment)
	mux.HandleFunc("POST "+apiBase+"{key}/attachments/{$}", s.handleUpload)
	mux.HandleFunc("PUT "+apiBase+"{key}/assignee/{$}", s.handleAssignee)
	mux.HandleFunc("PATCH "+apiBase+"{key}/fields/{$}", s.handleFields)
	// Deferred and cut endpoints (notifications, presence, mentions,
	// data-quality, login/logout) fall through to here. The UI hides a surface on
	// a clean 404 and only breaks on a 500.
	mux.HandleFunc("/", handleNotFound)
	return &Handler{mux: mux, s: s}
}

// StartUpdateCheck runs a GitHub release lookup immediately and every 24h.
// Results feed bootstrap's latest_version / release_url when the running build
// is older. No-op when cfg.UpdateCheckEnabled() is false. Safe with no
// credential (Jira-independent). Errors are silent.
func (h *Handler) StartUpdateCheck(ctx context.Context, cacheDir string) {
	if h == nil || h.s == nil {
		return
	}
	if !h.s.config().UpdateCheckEnabled() {
		return
	}
	go h.s.loopUpdateCheck(ctx, cacheDir)
}

// setUpdateInfo stores a release snapshot (tests and the background loop).
func (s *server) setUpdateInfo(info selfupdate.Info, ok bool) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.updateInfo = info
	s.updateOK = ok
}

// bootstrapUpdate returns latest/url only when the cached release is newer
// than the running Version.
func (s *server) bootstrapUpdate() (latest, url string) {
	s.updateMu.Lock()
	info, ok := s.updateInfo, s.updateOK
	s.updateMu.Unlock()
	if !ok || !selfupdate.Newer(Version, info.Latest) {
		return "", ""
	}
	return info.Latest, info.URL
}

func (s *server) loopUpdateCheck(ctx context.Context, cacheDir string) {
	run := func() {
		info, ok := selfupdate.Check(ctx, cacheDir, Version, s.config().UpdateCheckEnabled())
		if ok {
			s.setUpdateInfo(info, true)
		}
	}
	run()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// WebConfig renders the config document the UI fetches before mount
// (`ScryConfig` in web/src/lib/config.ts). Credentials never appear in it.
func WebConfig(cfg *config.Config) ([]byte, error) {
	return WebConfigBase(cfg, "")
}

// WebConfigBase is WebConfig with APIBase/AuthBase prefixed (e.g. "/w/work" for
// a workspace mount). prefix has no trailing slash; empty means root bases.
func WebConfigBase(cfg *config.Config, prefix string) ([]byte, error) {
	doc := webConfig(cfg)
	if prefix != "" {
		doc.APIBase = prefix + apiBase
		doc.AuthBase = prefix + authBase
	}
	return json.Marshal(doc)
}

func (s *server) config() *config.Config { return s.cfg.Load() }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("server: write response: %v", err)
	}
}

// fail sends the error body shape the client parses: `{"error": "<code>"}`.
func fail(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func handleNotFound(w http.ResponseWriter, r *http.Request) {
	fail(w, http.StatusNotFound, "not_found")
}

func serverError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("server: %s %s: %v", r.Method, r.URL.Path, err)
	fail(w, http.StatusInternalServerError, "internal_error")
}

// handleMe answers from the stored credential. Verifying it against Jira's
// /myself belongs to the credential endpoint (T4), not to every page load.
func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	if !cfg.HasCredential() {
		// Not an auth failure — a local tool with no credential simply has no
		// identity yet. 200 keeps the boot-time probe out of the browser console.
		writeJSON(w, http.StatusOK, map[string]any{"email": nil})
		return
	}
	name, dept, accountID := cfg.Email, "", cfg.AccountID
	for _, m := range cfg.Members {
		if m.Email != cfg.Email {
			continue
		}
		if m.DisplayName != "" {
			name = m.DisplayName
		} else if m.Name != "" {
			name = m.Name
		}
		dept = m.Department
		if accountID == "" {
			accountID = m.JiraAccountID
		}
		break
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"email":      cfg.Email,
		"account_id": nilIfEmpty(accountID),
		"name":       name,
		"department": nilIfEmpty(dept),
	})
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func etag(version int64) string { return fmt.Sprintf("%q", fmt.Sprintf("sv-%d", version)) }
