package server

// First-run onboarding: the three calls that let a browser finish setup without
// the CLI — connect a verified credential (including the site, which `PUT
// credential/` expects to already exist), list the site's projects to pick from,
// and run the first full sync while watching it progress.
//
// Same trust model as the rest of this package: a loopback-only local tool with
// no authentication. The token arrives once on connect and never appears in a
// response, a log line or the progress document.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	sysync "sync"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
	"github.com/midagedev/scry/internal/sync"
)

// maxProjects caps the picker list. A site with more projects than this is one
// where typing keys in settings beats scrolling a checkbox list.
const maxProjects = 500

/* ── step 1: connect ── */

type connectDoc struct {
	Site      string `json:"site"`
	JiraEmail string `json:"jira_email"`
	APIToken  string `json:"api_token"`
}

// handleConnect verifies the credential against Jira /myself before storing it,
// and is the only endpoint that writes cfg.Site: onboarding is where the site is
// first known, and `PUT credential/` refuses to run without it.
func (s *server) handleConnect(w http.ResponseWriter, r *http.Request) {
	var in connectDoc
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	site := normalizeSite(in.Site)
	email, token := strings.TrimSpace(in.JiraEmail), strings.TrimSpace(in.APIToken)
	if site == "" {
		fail(w, http.StatusBadRequest, "site_required")
		return
	}
	if email == "" || token == "" {
		fail(w, http.StatusBadRequest, "email_and_token_required")
		return
	}
	// Snapshot before save: first credential may need to kick the serve Watch loop.
	hadCredential := s.config().HasCredential()
	me, err := jira.New(site, email, token).Myself(r.Context())
	if err != nil {
		if errors.Is(err, jira.ErrAuth) {
			fail(w, http.StatusUnauthorized, "credential_rejected")
			return
		}
		failJira(w, r, err)
		return
	}
	next := *s.config()
	next.Site, next.Email, next.Token = site, email, token
	next.TokenOwner, next.TokenVerifiedAt = me.DisplayName, store.Now()
	next.AccountID = me.AccountID
	if err := next.Save(); err != nil {
		serverError(w, r, err)
		return
	}
	s.cfg.Store(&next)
	s.gen.Add(1)
	// When serve started without a credential, start the background sync once.
	s.fireSyncStarterIfNeeded(hadCredential)
	// Same credential document the settings UI reads — never the token itself.
	writeJSON(w, http.StatusOK, credential(&next))
}

// normalizeSite accepts what people paste: with or without a scheme, with or
// without a trailing slash. Anything that is not a plain host URL is rejected by
// returning "", which the caller answers as site_required.
func normalizeSite(raw string) string {
	v := strings.TrimRight(strings.TrimSpace(raw), "/")
	if v == "" {
		return ""
	}
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

/* ── step 2: project picker ── */

func (s *server) handleAvailableProjects(w http.ResponseWriter, r *http.Request) {
	c, _, ok := s.client(w)
	if !ok {
		return
	}
	list, truncated, err := c.Projects(r.Context(), maxProjects)
	if err != nil {
		failJira(w, r, err)
		return
	}
	if list == nil {
		list = []jira.Project{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": list, "truncated": truncated})
}

/* ── step 3: first sync ── */

// progressDoc is the polled document. It carries counters only: no site, no
// email, nothing derived from the token.
type progressDoc struct {
	Running bool   `json:"running"`
	Phase   string `json:"phase"` // idle | syncing | done | error
	Fetched int    `json:"fetched"`
	Changed int    `json:"changed"`
	Deleted int    `json:"deleted"`
	Done    bool   `json:"done"`
	// Error is the failure text for the user. Jira's own message is safe here —
	// the client never sends the token in a URL or a body Jira echoes back.
	Error      string `json:"error"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

// One sync job at a time, process-wide.
// ponytail: package-level because `scry serve` runs exactly one server; make it a
// server field if a second instance ever shares this process.
var (
	syncMu  sysync.Mutex
	syncJob progressDoc
)

// startSyncBody is optional. Empty body keeps the historical full-sync default
// (onboarding first run). Daily "Sync now" sends {"mode":"incremental"}.
type startSyncBody struct {
	Mode string `json:"mode"` // "full" | "incremental"; default "full"
}

func (s *server) handleStartSync(w http.ResponseWriter, r *http.Request) {
	// Empty projects is a valid scope now: every project the account can see.
	cfg := s.config()
	if !cfg.HasCredential() {
		fail(w, http.StatusBadRequest, "credential_required")
		return
	}

	full := true
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<10))
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		var in startSyncBody
		if err := json.Unmarshal(raw, &in); err != nil {
			fail(w, http.StatusBadRequest, "invalid_body")
			return
		}
		switch strings.ToLower(strings.TrimSpace(in.Mode)) {
		case "", "full":
			full = true
		case "incremental":
			full = false
		default:
			fail(w, http.StatusBadRequest, "invalid_mode")
			return
		}
	}

	if !s.startSyncJob(cfg, full) {
		fail(w, http.StatusConflict, "sync_in_progress")
		return
	}
	writeJSON(w, http.StatusAccepted, syncProgress())
}

// startSyncJob begins a background sync unless one is already running.
// Returns false when a run was already in progress.
func (s *server) startSyncJob(cfg *config.Config, full bool) bool {
	if s.syncKick != nil {
		return s.syncKick(cfg, full)
	}
	syncMu.Lock()
	if syncJob.Running {
		syncMu.Unlock()
		return false
	}
	syncJob = progressDoc{Running: true, Phase: "syncing", StartedAt: store.Now()}
	syncMu.Unlock()

	// The request is answered immediately, so the run cannot hang off r.Context().
	go runSyncJob(context.Background(), cfg, s.db, full)
	return true
}

func runSyncJob(ctx context.Context, cfg *config.Config, db *store.DB, full bool) {
	res, err := sync.Run(ctx, cfg, db, sync.Options{
		Full: full,
		Progress: func(fetched, changed int) {
			syncMu.Lock()
			syncJob.Fetched, syncJob.Changed = fetched, changed
			syncMu.Unlock()
		},
	})
	fetched, changed, deleted := res.Fetched, res.Changed, res.Deleted
	jobErr := err
	// Confluence is best-effort: a failure must not block Jira (same policy as
	// sync.Watch). Only run after Jira succeeds; leave its error on the job so
	// the UI can surface a partial failure without treating the whole job as a
	// hard fail when Jira completed.
	if jobErr == nil && cfg != nil && cfg.Confluence != nil {
		cres, cerr := sync.RunConfluence(ctx, cfg, db, sync.Options{Full: full})
		fetched += cres.Fetched
		changed += cres.Changed
		deleted += cres.Deleted
		if cerr != nil {
			jobErr = cerr
		}
	}

	syncMu.Lock()
	defer syncMu.Unlock()
	syncJob.Running = false
	syncJob.FinishedAt = store.Now()
	syncJob.Fetched, syncJob.Changed, syncJob.Deleted = fetched, changed, deleted
	if err != nil {
		// Jira failed: hard error. sync.Run keeps the token out of its errors.
		syncJob.Phase, syncJob.Error = "error", err.Error()
		return
	}
	// Jira succeeded. Confluence failure is recorded on Error but the job is
	// still done — counters include both sources.
	syncJob.Phase, syncJob.Done = "done", true
	if jobErr != nil {
		syncJob.Error = jobErr.Error()
	}
}

func (s *server) handleSyncProgress(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, syncProgress())
}

// handleSyncRuns lists recent meaningful sync runs, newest first. Backs the
// history popover behind the sidebar's sync timestamp.
//
// ?source= selects which connector's history to return. Absent or "jira" is the
// historical default (compat); "confluence" is the wiki source; anything else
// is 400 invalid_source. The response echoes source so clients can tell rows
// apart without reading store.SyncRun (which has no source field).
func (s *server) handleSyncRuns(w http.ResponseWriter, r *http.Request) {
	src := strings.TrimSpace(r.URL.Query().Get("source"))
	var id string
	switch src {
	case "", "jira":
		src = "jira"
		id = sourceID
	case "confluence":
		id = sync.ConfluenceSourceID
	default:
		fail(w, http.StatusBadRequest, "invalid_source")
		return
	}
	runs, err := s.db.SyncRuns(id, 20)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if runs == nil {
		runs = []store.SyncRun{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs, "source": src})
}

func syncProgress() progressDoc {
	syncMu.Lock()
	defer syncMu.Unlock()
	doc := syncJob
	if doc.Phase == "" {
		doc.Phase = "idle"
	}
	return doc
}
