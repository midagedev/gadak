// Package server serves the HTTP read API the web UI already speaks
// (specs/000-product/contracts/api.md). Every response is assembled from the
// SQLite mirror; the one call that leaves this process is the attachment byte
// proxy, which is also the only endpoint that needs credentials.
//
// The server has no authentication: `scry serve` refuses a non-loopback bind
// instead. Personal-state endpoints therefore never answer 401/403.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/midagedev/scry/internal/attachcache"
	"github.com/midagedev/scry/internal/config"
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

	mu     sync.Mutex
	cached *derivedView

	// cache holds attachment bytes on disk. nil disables caching and every view
	// proxies, which is the pre-cache behavior.
	cache *attachcache.Cache
}

// New returns the API handler. Mount it at "/api/" — the patterns below carry
// their full paths, so nothing strips a prefix.
func New(db *store.DB, cfg *config.Config) http.Handler {
	return NewWithCache(db, cfg, nil)
}

// NewWithCache is New plus an attachment byte cache. `scry serve` passes one
// rooted under SCRY_HOME; tests pass nil when they do not exercise attachments.
func NewWithCache(db *store.DB, cfg *config.Config, cache *attachcache.Cache) http.Handler {
	if cfg == nil {
		cfg = &config.Config{}
	}
	s := &server{db: db, cache: cache}
	s.cfg.Store(cfg)

	// Every pattern is anchored with {$}: a trailing slash alone would make each
	// literal endpoint a subtree that overlaps `{key}/detail/`, which ServeMux
	// rejects as ambiguous.
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+apiBase+"bootstrap/{$}", s.handleBootstrap)
	mux.HandleFunc("GET "+apiBase+"delta/{$}", s.handleDelta)
	mux.HandleFunc("GET "+apiBase+"search/{$}", s.handleSearch)
	mux.HandleFunc("GET "+apiBase+"settings/{$}", s.handleGetSettings)
	mux.HandleFunc("PUT "+apiBase+"settings/{$}", s.handlePutSettings)
	mux.HandleFunc("GET "+apiBase+"views/{$}", s.handleGetViews)
	mux.HandleFunc("POST "+apiBase+"views/{$}", s.handlePostView)
	mux.HandleFunc("DELETE "+apiBase+"views/{id}/{$}", s.handleDeleteView)
	mux.HandleFunc("GET "+apiBase+"watches/{$}", s.handleGetWatches)
	mux.HandleFunc("DELETE "+apiBase+"watches/{key}/{$}", s.handleDeleteWatch)
	// Personal feed (computed from the mirror; read receipts in feed_reads).
	// Literal patterns beat `{key}/{action}/` and `{key}/detail/`.
	mux.HandleFunc("GET "+apiBase+"feed/{$}", s.handleGetFeed)
	mux.HandleFunc("POST "+apiBase+"feed/read/{$}", s.handleMarkFeedRead)
	// The client's two PUT shapes — `watches/{key}/` and `{key}/assignee/` — are
	// mirror images that overlap on `watches/assignee/`, and ServeMux rejects an
	// ambiguous pair with no way to break the tie (there is no third-pattern
	// exception). So they share one pattern and are told apart here.
	mux.HandleFunc("PUT "+apiBase+"{key}/{action}/{$}", func(w http.ResponseWriter, r *http.Request) {
		switch key, action := r.PathValue("key"), r.PathValue("action"); {
		case key == "watches":
			s.setWatch(w, r, action, true)
		case action == "assignee":
			s.handleAssignee(w, r)
		default:
			handleNotFound(w, r)
		}
	})
	mux.HandleFunc("GET "+apiBase+"{key}/detail/{$}", s.handleDetail)
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
	mux.HandleFunc("GET "+apiBase+"create-meta/{$}", s.handleCreateMeta)
	mux.HandleFunc("POST "+apiBase+"create/{$}", s.handleCreate)
	mux.HandleFunc("GET "+apiBase+"users/{$}", s.handleUsers)
	mux.HandleFunc("GET "+apiBase+"{key}/transitions/{$}", s.handleTransitions)
	mux.HandleFunc("POST "+apiBase+"{key}/transition/{$}", s.handleTransition)
	mux.HandleFunc("POST "+apiBase+"{key}/comment/{$}", s.handleComment)
	mux.HandleFunc("POST "+apiBase+"{key}/attachments/{$}", s.handleUpload)
	mux.HandleFunc("PUT "+apiBase+"{key}/assignee/{$}", s.handleAssignee)
	mux.HandleFunc("PATCH "+apiBase+"{key}/fields/{$}", s.handleFields)
	mux.HandleFunc("GET "+apiBase+"{key}/editmeta/{$}", s.handleEditMeta)
	// Deferred and cut endpoints (notifications, presence, mentions,
	// data-quality, login/logout) fall through to here. The UI hides a surface on
	// a clean 404 and only breaks on a 500.
	mux.HandleFunc("/", handleNotFound)
	return mux
}

// WebConfig renders the config document the UI fetches before mount
// (`ScryConfig` in web/src/lib/config.ts). Credentials never appear in it.
func WebConfig(cfg *config.Config) ([]byte, error) {
	return json.Marshal(webConfig(cfg))
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
	name, dept := cfg.Email, ""
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
		break
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"email":      cfg.Email,
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
