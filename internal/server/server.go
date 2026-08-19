// Package server is the loopback HTTP mux for the web UI and the API
// (specs/000-product/contracts/api.md). Reads are assembled from the SQLite
// mirror. Write-through endpoints call Jira and re-read the issue into the
// mirror. Credentials are needed for those writes and for fetching attachment
// bytes that are not already on disk.
//
// The server has no authentication: `gadak serve` refuses a non-loopback bind
// instead. Personal-state endpoints therefore never answer 401/403.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/selfupdate"
	"github.com/midagedev/gadak/internal/store"
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

	// updateMu protects the GitHub release snapshot used by bootstrap and
	// delta (same source — see updateFields). Separate from mu so a release
	// check never contends with derived views.
	updateMu   sync.Mutex
	updateInfo selfupdate.Info
	updateOK   bool
	// updateCacheDir is where selfupdate writes update-check.json. Set by
	// StartUpdateCheck / CheckNow so POST update/ can find it.
	updateCacheDir string
	// updateLastAttempt is the last Check call (success or fail). Distinct
	// from Info.CheckedAt, which is only written on a successful fetch.
	updateLastAttempt time.Time
	// updateLastUser* is the last Check for Updates / POST update/ result.
	// Background checks never touch these — silent failure stays silent.
	updateLastUserAt     time.Time
	updateLastUserStatus string
	updateLastUserErr    string

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

	// jobsCtx is cancelled by Shutdown so a startSyncJob goroutine cannot
	// outlive the server. jobsWG counts those goroutines; Close waits on it.
	// Nothing else owned this lifetime, so a settings PUT in a test (or a
	// serve process exiting) left the writer holding a WAL connection
	// (GDK-270).
	jobsCtx    context.Context
	jobsCancel context.CancelFunc
	jobsWG     sync.WaitGroup

	// originH is the standalone issuetap handler this process already
	// holds. Lazy: first /api/v1/origin/ request constructs it via
	// origin.StandaloneHandler. Tests pin it to keep a session alive
	// after origin.live is evicted (cross-process simulation).
	originOnce sync.Once
	originH    http.Handler
}

// Handler is the HTTP API plus optional update-check control. It implements
// http.Handler; mount it at "/api/".
type Handler struct {
	mux     *http.ServeMux
	guarded http.Handler
	s       *server
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Identity markers for a second `gadak serve` that finds this port busy
	// (cmd/gadak port fallback). Fixed marker + profile — no version injection
	// into constructors. Set before the guard so every response carries them.
	w.Header().Set("X-Gadak", "1")
	w.Header().Set("X-Gadak-Profile", h.s.profile)
	h.guarded.ServeHTTP(w, r)
}

// New returns the API handler. Mount it at "/api/" — the patterns below carry
// their full paths, so nothing strips a prefix.
func New(db *store.DB, cfg *config.Config) *Handler {
	return NewWithCache(db, cfg, nil)
}

// NewWithCache is New plus an attachment byte cache. `gadak serve` passes one
// rooted under GADAK_HOME; tests pass nil when they do not exercise attachments.
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
	s.jobsCtx, s.jobsCancel = context.WithCancel(context.Background())

	// Every pattern is anchored with {$}: a trailing slash alone would make each
	// literal endpoint a subtree that overlaps `{key}/detail/`, which ServeMux
	// rejects as ambiguous.
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+apiBase+"bootstrap/{$}", s.handleBootstrap)
	mux.HandleFunc("GET "+apiBase+"delta/{$}", s.handleDelta)
	// Update snapshot + user-initiated force check. Literals beat {key}/{action}/.
	mux.HandleFunc("GET "+apiBase+"update/{$}", s.handleGetUpdate)
	mux.HandleFunc("POST "+apiBase+"update/{$}", s.handlePostUpdate)
	mux.HandleFunc("GET "+apiBase+"search/{$}", s.handleSearch)
	mux.HandleFunc("GET "+apiBase+"ui-focus/{$}", s.handleUIFocus)
	mux.HandleFunc("GET "+apiBase+"jql/{$}", s.handleJql)
	mux.HandleFunc("POST "+apiBase+"jql/{$}", s.handleJql)
	mux.HandleFunc("POST "+apiBase+"jql/emit/{$}", s.handleJqlEmit)
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
	// Local history (local.db). Literals beat `{key}/{action}/`.
	mux.HandleFunc("GET "+apiBase+"history/{$}", s.handleGetHistory)
	mux.HandleFunc("POST "+apiBase+"history/visits/{$}", s.handlePostVisit)
	mux.HandleFunc("POST "+apiBase+"history/searches/{$}", s.handlePostSearch)
	mux.HandleFunc("PATCH "+apiBase+"history/searches/{id}/{$}", s.handlePatchSearch)
	// Recent-use history (local.db). Literals beat `{key}/{action}/`.
	s.registerRecency(mux)
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
	mux.HandleFunc("GET "+apiBase+"priorities/{$}", s.handlePriorities)
	mux.HandleFunc("POST "+apiBase+"{key}/transition/{$}", s.handleTransition)
	mux.HandleFunc("POST "+apiBase+"{key}/comment/{$}", s.handleComment)
	mux.HandleFunc("POST "+apiBase+"{key}/attachments/{$}", s.handleUpload)
	mux.HandleFunc("PUT "+apiBase+"{key}/assignee/{$}", s.handleAssignee)
	mux.HandleFunc("PUT "+apiBase+"{key}/labels/{$}", s.handleLabels)
	mux.HandleFunc("PUT "+apiBase+"{key}/priority/{$}", s.handlePriority)
	mux.HandleFunc("PUT "+apiBase+"{key}/summary/{$}", s.handleSummary)
	mux.HandleFunc("PUT "+apiBase+"{key}/description/{$}", s.handleDescription)
	mux.HandleFunc("PUT "+apiBase+"{key}/duedate/{$}", s.handleDuedate)
	mux.HandleFunc("PATCH "+apiBase+"{key}/fields/{$}", s.handleFields)
	// Single-item re-read after in-app browser edit (no upstream write).
	// Three-segment pages/{id}/resync/ is a literal; {key}/resync/ is two-segment.
	mux.HandleFunc("POST "+apiBase+"{key}/resync/{$}", s.handleResync)
	mux.HandleFunc("POST "+apiBase+"pages/{id}/resync/{$}", s.handlePageResync)
	mux.HandleFunc("PUT "+apiBase+"pages/{id}/edit/{$}", s.handlePageEdit)
	mux.HandleFunc("POST "+apiBase+"pages/{id}/comment/{$}", s.handlePageComment)
	mux.HandleFunc("POST "+apiBase+"pages/{$}", s.handlePageCreate)
	// Origin passthrough: CLI writes on a standalone workspace go through
	// the live serve's issuetap so persist has one owner (GDK-333).
	mux.Handle(origin.RESTPrefix+"/", http.HandlerFunc(s.handleOriginREST))
	// Deferred and cut endpoints (notifications, presence, mentions,
	// data-quality, login/logout) fall through to here. The UI hides a surface on
	// a clean 404 and only breaks on a 500.
	mux.HandleFunc("/", handleNotFound)
	h := &Handler{mux: mux, s: s}
	h.guarded = GuardBrowser(mux)
	if testRegisterHandler != nil {
		testRegisterHandler(h)
	}
	return h
}

// testRegisterHandler is set from tests so fixture cleanup can Shutdown every
// Handler that opened a given DB before closing it. Production is nil.
var testRegisterHandler func(*Handler)

// closeWait is how long Close waits for in-flight startSyncJob goroutines.
// Same bound as the HTTP server's shutdown window in cmd/gadak/serve.go
// (the 3-second context.WithTimeout around srv.Shutdown). Past this, Close
// returns and the caller may close the database anyway; a still-running job
// then fails the pool assertion (tests) or races WAL files (production).
const closeWait = 3 * time.Second

// Shutdown cancels background startSyncJob work and waits for those
// goroutines to return, or until ctx is done. A timed-out wait returns
// ctx.Err(); the job may still be running and still hold a database
// connection. Idempotent.
//
// Returning from the job goroutine is not enough: database/sql rolls a
// cancelled Tx back from a helper goroutine (Tx.awaitDone), and that
// helper can still hold the pool connection after runSyncJob has
// returned. Waiting for InUse==0 is waiting for that writer, which is
// the WAL leak GDK-270 actually is.
func (h *Handler) Shutdown(ctx context.Context) error {
	if h == nil || h.s == nil {
		return nil
	}
	if h.s.jobsCancel != nil {
		h.s.jobsCancel()
	}
	done := make(chan struct{})
	go func() {
		h.s.jobsWG.Wait()
		close(done)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
	case <-ctx.Done():
		return fmt.Errorf("background sync still running after shutdown bound: %w", ctx.Err())
	}
	return waitPoolIdle(ctx, h.s.db)
}

// waitPoolIdle waits until no connection is checked out, or ctx is done.
func waitPoolIdle(ctx context.Context, db *store.DB) error {
	if db == nil {
		return nil
	}
	if db.PoolStats().InUse == 0 {
		return nil
	}
	t := time.NewTicker(5 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("background sync returned but %d connection(s) still checked out: %w", db.PoolStats().InUse, ctx.Err())
		case <-t.C:
			if db.PoolStats().InUse == 0 {
				return nil
			}
		}
	}
}

// Close is Shutdown with a 3s bound — the same window cmd/gadak/serve.go
// uses for http.Server.Shutdown.
func (h *Handler) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeWait)
	defer cancel()
	return h.Shutdown(ctx)
}

// StartUpdateCheck runs a GitHub release lookup immediately and every 24h.
// Results feed latest_version / release_url on bootstrap and delta when the
// running build is older. Records cacheDir even when disabled so a later
// user-initiated CheckNow still knows where the file lives. The background
// loop is a no-op when cfg.UpdateCheckEnabled() is false. Safe with no
// credential (Jira-independent). Background errors are silent.
func (h *Handler) StartUpdateCheck(ctx context.Context, cacheDir string) {
	if h == nil || h.s == nil {
		return
	}
	h.s.setUpdateCacheDir(cacheDir)
	if !h.s.config().UpdateCheckEnabled() {
		return
	}
	go h.s.loopUpdateCheck(ctx, cacheDir)
}

// UpdateStatus is GET/POST update/ and CheckNow: what the server currently
// knows about the latest published release, plus the outcome of a
// user-initiated check when one has run.
type UpdateStatus struct {
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	URL             string `json:"release_url,omitempty"`
	Notes           string `json:"release_notes,omitempty"`
	NotesLen        int    `json:"release_notes_len"`
	CheckedAt       string `json:"checked_at,omitempty"`
	Newer           bool   `json:"newer,omitempty"`
	Status          string `json:"status,omitempty"` // newer|current|error|dev — this CheckNow
	Error           string `json:"error,omitempty"`
	LastUserCheckAt string `json:"last_user_check_at,omitempty"`
	LastUserStatus  string `json:"last_user_status,omitempty"`
}

// CheckNow bypasses the 24h disk cache, hits GitHub once, and records the
// result for GET update/ and for bootstrap/delta. Background checks stay
// silent; this path is the one that reports current / error / dev.
func (h *Handler) CheckNow(ctx context.Context, cacheDir string) UpdateStatus {
	if h == nil || h.s == nil {
		return UpdateStatus{Current: Version, Status: "error", Error: "check_failed"}
	}
	return h.s.checkNow(ctx, cacheDir)
}

// SnapshotSync is the debug document for "what background work is running
// right now": the same one-shot job + activity picture that
// GET /api/v1/issues/sync/progress/ already returns. No new endpoint — that
// GET already carries it; this is the in-process form.
func (h *Handler) SnapshotSync() progressResponse {
	if h == nil || h.s == nil {
		return progressResponse{}
	}
	return h.s.syncProgressResponse()
}

// setUpdateInfo stores a release snapshot (tests and the background loop).
func (s *server) setUpdateInfo(info selfupdate.Info, ok bool) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.updateInfo = info
	s.updateOK = ok
}

func (s *server) setUpdateCacheDir(dir string) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	if dir != "" {
		s.updateCacheDir = dir
	}
}

func (s *server) getUpdateCacheDir() string {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	return s.updateCacheDir
}

func (s *server) noteUpdateAttempt() {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.updateLastAttempt = time.Now().UTC()
}

func (s *server) recordUserCheck(at time.Time, status, errCode string) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.updateLastUserAt = at
	s.updateLastUserStatus = status
	s.updateLastUserErr = errCode
}

// updateFields is the single owner of latest_version / release_url /
// release_notes for both bootstrap and delta. Empty strings omit via json
// omitempty. Filling only one document was the GDK-214 defect.
func (s *server) updateFields() (latest, url, notes string) {
	s.updateMu.Lock()
	info, ok := s.updateInfo, s.updateOK
	s.updateMu.Unlock()
	if !ok || !selfupdate.Newer(Version, info.Latest) {
		return "", "", ""
	}
	return info.Latest, info.URL, info.Notes
}

func (s *server) snapshotUpdate() UpdateStatus {
	s.updateMu.Lock()
	info, ok := s.updateInfo, s.updateOK
	attempt := s.updateLastAttempt
	userAt := s.updateLastUserAt
	userStatus := s.updateLastUserStatus
	userErr := s.updateLastUserErr
	s.updateMu.Unlock()

	out := UpdateStatus{Current: Version}
	if ok {
		out.Latest = info.Latest
		out.URL = info.URL
		out.Notes = info.Notes
		out.NotesLen = len(info.Notes)
		out.CheckedAt = info.CheckedAt
		out.Newer = selfupdate.Newer(Version, info.Latest)
	}
	if out.CheckedAt == "" && !attempt.IsZero() {
		out.CheckedAt = attempt.UTC().Format(time.RFC3339)
	}
	if !userAt.IsZero() {
		out.LastUserCheckAt = userAt.UTC().Format(time.RFC3339)
		out.LastUserStatus = userStatus
		out.Error = userErr
	}
	return out
}

func (s *server) checkNow(ctx context.Context, cacheDir string) UpdateStatus {
	if cacheDir != "" {
		s.setUpdateCacheDir(cacheDir)
	}
	cacheDir = s.getUpdateCacheDir()
	now := time.Now().UTC()
	out := UpdateStatus{Current: Version}

	if Version == "" || Version == "0.0.0-dev" {
		out.Status = "dev"
		s.recordUserCheck(now, "dev", "")
		return out
	}
	if cacheDir == "" {
		out.Status = "error"
		out.Error = "check_failed"
		s.recordUserCheck(now, "error", "check_failed")
		return out
	}

	// Drop the 24h cache so Check hits the network — selfupdate owns the
	// filename, so a rename there cannot leave this silently pointing at a
	// file that no longer exists.
	_ = selfupdate.DropCache(cacheDir)

	s.noteUpdateAttempt()
	info, ok := selfupdate.Check(ctx, cacheDir, Version, true)
	if !ok {
		out.Status = "error"
		out.Error = "check_failed"
		s.recordUserCheck(now, "error", "check_failed")
		return out
	}
	s.setUpdateInfo(info, true)
	out.Latest = info.Latest
	out.URL = info.URL
	out.Notes = info.Notes
	out.NotesLen = len(info.Notes)
	out.CheckedAt = info.CheckedAt
	if selfupdate.Newer(Version, info.Latest) {
		out.Status = "newer"
		out.Newer = true
	} else {
		out.Status = "current"
	}
	s.recordUserCheck(now, out.Status, "")
	out.LastUserCheckAt = now.Format(time.RFC3339)
	out.LastUserStatus = out.Status
	return out
}

func (s *server) handleGetUpdate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.snapshotUpdate())
}

func (s *server) handlePostUpdate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.checkNow(r.Context(), ""))
}

func (s *server) loopUpdateCheck(ctx context.Context, cacheDir string) {
	run := func() {
		s.noteUpdateAttempt()
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
// (`GadakConfig` in web/src/lib/config.ts). Credentials never appear in it.
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
	// /w/<name>/config.json is that profile's document, not the process primary.
	if name, ok := strings.CutPrefix(prefix, "/w/"); ok && name != "" {
		doc.Profile = profileDisplay(name)
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
