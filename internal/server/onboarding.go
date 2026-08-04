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
	"context"
	"encoding/json"
	"errors"
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
	if err := next.Save(); err != nil {
		serverError(w, r, err)
		return
	}
	s.cfg.Store(&next)
	s.gen.Add(1)
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

func (s *server) handleStartSync(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	switch {
	case !cfg.HasCredential():
		fail(w, http.StatusBadRequest, "credential_required")
		return
	case len(cfg.Projects) == 0:
		fail(w, http.StatusBadRequest, "projects_required")
		return
	}

	syncMu.Lock()
	if syncJob.Running {
		syncMu.Unlock()
		fail(w, http.StatusConflict, "sync_in_progress")
		return
	}
	syncJob = progressDoc{Running: true, Phase: "syncing", StartedAt: store.Now()}
	syncMu.Unlock()

	// The request is answered immediately, so the run cannot hang off r.Context().
	go runFirstSync(context.Background(), cfg, s.db)
	writeJSON(w, http.StatusAccepted, syncProgress())
}

func runFirstSync(ctx context.Context, cfg *config.Config, db *store.DB) {
	res, err := sync.Run(ctx, cfg, db, sync.Options{
		Full: true,
		Progress: func(fetched, changed int) {
			syncMu.Lock()
			syncJob.Fetched, syncJob.Changed = fetched, changed
			syncMu.Unlock()
		},
	})
	syncMu.Lock()
	defer syncMu.Unlock()
	syncJob.Running = false
	syncJob.FinishedAt = store.Now()
	syncJob.Fetched, syncJob.Changed, syncJob.Deleted = res.Fetched, res.Changed, res.Deleted
	if err != nil {
		// sync.Run and the Jira client both keep the token out of their errors.
		syncJob.Phase, syncJob.Error = "error", err.Error()
		return
	}
	syncJob.Phase, syncJob.Done = "done", true
}

func (s *server) handleSyncProgress(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, syncProgress())
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
