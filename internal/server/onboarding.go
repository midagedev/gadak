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
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/sync"
)

// maxProjects caps the picker list. A site with more projects than this is one
// where typing keys in settings beats scrolling a checkbox list.
const maxProjects = 500

/* ── step 1: connect ── */

type connectDoc struct {
	Site              string `json:"site"`
	JiraEmail         string `json:"jira_email"`
	APIToken          string `json:"api_token"`
	TokenExpiresAt    string `json:"token_expires_at"`
	ReplaceStandalone bool   `json:"replace_standalone"`
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
	// Refuse before /myself: a standalone workspace that holds local issues
	// must not send the pasted token anywhere, and must not write it to disk.
	if err := originbind.RefuseReplace(s.config(), in.ReplaceStandalone); err != nil {
		var refused *originbind.ReplaceRefusedError
		if errors.As(err, &refused) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   originbind.ErrCodeReplaceRefused,
				"issues":  refused.Issues,
				"persist": refused.Persist,
			})
			return
		}
		serverError(w, r, err)
		return
	}
	// Snapshot before save: first credential may need to kick the serve Watch loop.
	hadCredential := s.config().HasCredential()
	me, err := origin.Connected(site, email, token).Myself(r.Context())
	if err != nil {
		if errors.Is(err, jira.ErrAuth) {
			fail(w, http.StatusUnauthorized, rejectedCredentialCode(token))
			return
		}
		failJira(w, r, err)
		return
	}
	wasStandalone := s.config().IsStandalone()
	next := *s.config()
	next.Site, next.Email, next.Token = site, email, token
	next.TokenOwner, next.TokenVerifiedAt = me.DisplayName, store.Now()
	next.AccountID = me.AccountID
	if err := next.ApplyTokenExpiry(in.TokenExpiresAt, next.TokenVerifiedAt); err != nil {
		fail(w, http.StatusBadRequest, "invalid_token_expires")
		return
	}
	originbind.ClearStandalone(&next)
	if wasStandalone {
		reset, err := originbind.DropStandaloneProjection(&next, s.db)
		if err != nil {
			serverError(w, r, err)
			return
		}
		// The response is the credential document, so this is the only place
		// the count can be said. Worth saying: "my feed emptied after
		// connecting" is otherwise unanswerable after the fact.
		if line := reset.String(); line != "" {
			log.Printf("onboarding: %s", line)
		}
	}
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

// orgKeyPrefix is the organization API key Atlassian issues from
// admin.atlassian.com. Those keys authenticate against organization admin APIs
// only and 401 against every product endpoint (docs/STATE_OF_PLAY.md,
// "hard-won knowledge" #1); the same prefix is what internal/secretscan keys on.
const orgKeyPrefix = "ATCTT"

// rejectedCredentialCode names the token trap behind a 401 when the pasted
// token gives it away, so the client can say which mistake this is instead of
// listing every one of them.
//
// Only the organization key is recognisable here. The other trap at this
// moment is a *scoped* token ("Create API token with scopes"), which is issued
// for api.atlassian.com/ex/{product}/{cloudId} and cannot Basic-auth against a
// site URL — but nothing in this repository documents a prefix that separates a
// scoped token from a classic one, and the 401 body that might is dropped at
// the internal/atlhttp boundary (atlhttp.Do turns 401/403 into a bodyless
// AuthError). So a scoped token and a mistyped one share the generic code and
// the copy behind it names both, rather than a prefix rule being invented here.
//
// Classification happens only after Jira has already rejected the credential:
// a prefix is a hint about a live service, not a right to refuse a token this
// build has never sent. The token is read, never returned, logged, or embedded
// in the code — the result is one of two constants.
func rejectedCredentialCode(token string) string {
	if strings.HasPrefix(token, orgKeyPrefix) {
		return "credential_rejected_org_key"
	}
	return "credential_rejected"
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
	Error string `json:"error"`
	// ErrorCode is the machine-readable classification of Error, present only
	// when the failure means the stored credential was rejected:
	// "credential_rejected", the same code the write path answers 409 with.
	// Every other failure — transport (500, timeout, DNS), or a wiki-only
	// failure after the Jira pass succeeded — leaves it absent, so clients key
	// recovery affordances on this field and never on the Error prose.
	ErrorCode  string `json:"error_code,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

// mirrorActivity is any sync pass in this process — the one-shot job and the
// background watch loop both report here. progressDoc stays what it always
// was: the one-shot job's own lifecycle, which the client's start/poll flow
// depends on. A caller asking "is the mirror moving right now" needs this one.
type mirrorActivity struct {
	Running   bool   `json:"running"`
	Source    string `json:"source"`  // issues | documents | "" when idle
	Fetched   int    `json:"fetched"` // running total for the CURRENT source's pass
	Changed   int    `json:"changed"`
	StartedAt string `json:"started_at"`
}

// progressResponse is GET/POST sync progress: the one-shot job fields plus the
// activity slot for this server instance. Existing field meanings are unchanged.
type progressResponse struct {
	progressDoc
	Activity mirrorActivity `json:"activity"`
}

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
	writeJSON(w, http.StatusAccepted, s.syncProgressResponse())
}

// startSyncJob begins a background sync unless one is already running.
// Returns false when a run was already in progress.
func (s *server) startSyncJob(cfg *config.Config, full bool) bool {
	if s.syncKick != nil {
		return s.syncKick(cfg, full)
	}
	s.syncMu.Lock()
	if s.jobsCtx != nil && s.jobsCtx.Err() != nil {
		s.syncMu.Unlock()
		return false
	}
	if s.syncJob.Running {
		s.syncMu.Unlock()
		return false
	}
	s.syncJob = progressDoc{Running: true, Phase: "syncing", StartedAt: store.Now()}
	s.jobsWG.Add(1)
	s.syncMu.Unlock()

	// The request is answered immediately, so the run cannot hang off r.Context().
	// Lifetime is the server's jobsCtx: Shutdown cancels it and waits (GDK-270).
	ctx := s.jobsCtx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		defer s.jobsWG.Done()
		s.runSyncJob(ctx, cfg, full)
	}()
	return true
}

func (s *server) runSyncJob(ctx context.Context, cfg *config.Config, full bool) {
	// Always close activity when the job finishes (success, Jira fail, panic).
	defer s.reportActivityPhase(sync.PhaseIdle)

	s.reportActivityPhase(sync.PhaseIssues)
	res, err := sync.Run(ctx, cfg, s.db, sync.Options{
		Full: full,
		Progress: func(fetched, changed int) {
			s.syncMu.Lock()
			s.syncJob.Fetched, s.syncJob.Changed = fetched, changed
			if s.activity.Running {
				s.activity.Fetched, s.activity.Changed = fetched, changed
			}
			s.syncMu.Unlock()
		},
	})
	fetched, changed, deleted := res.Fetched, res.Changed, res.Deleted
	jobErr := err
	// Confluence is best-effort: a failure must not block Jira (same policy as
	// sync.Watch). Only run after Jira succeeds; leave its error on the job so
	// the UI can surface a partial failure without treating the whole job as a
	// hard fail when Jira completed.
	if jobErr == nil && cfg != nil && cfg.Confluence != nil {
		// Job counters stay the sum of both sources; activity is per-source.
		jiraFetched, jiraChanged := fetched, changed
		s.reportActivityPhase(sync.PhaseDocuments)
		cres, cerr := sync.RunConfluence(ctx, cfg, s.db, sync.Options{
			Full: full,
			Progress: func(f, c int) {
				s.syncMu.Lock()
				s.syncJob.Fetched = jiraFetched + f
				s.syncJob.Changed = jiraChanged + c
				if s.activity.Running {
					s.activity.Fetched, s.activity.Changed = f, c
				}
				s.syncMu.Unlock()
			},
		})
		fetched += cres.Fetched
		changed += cres.Changed
		deleted += cres.Deleted
		if cerr != nil {
			jobErr = cerr
		}
	}

	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.syncJob.Running = false
	s.syncJob.FinishedAt = store.Now()
	s.syncJob.Fetched, s.syncJob.Changed, s.syncJob.Deleted = fetched, changed, deleted
	if err != nil {
		// Jira failed: hard error. sync.Run keeps the token out of its errors.
		s.syncJob.Phase, s.syncJob.Error = "error", err.Error()
		// One owner decides "this failure means the credential is dead" —
		// sync.IsRejectedCredential — and this is the only place its answer
		// reaches the wire, as a code the client can act on.
		if sync.IsRejectedCredential(err) {
			s.syncJob.ErrorCode = "credential_rejected"
		}
		return
	}
	// Jira succeeded. Confluence failure is recorded on Error but the job is
	// still done — counters include both sources. Deliberately no ErrorCode
	// here, even for a wiki 401: the Jira pass authenticated with the same
	// token moments earlier, so a wiki-only rejection is a product permission
	// gap, not a dead credential (the same split Watch makes — Jira rejection
	// fatal, wiki rejection not). Telling the user to replace a working token
	// is the failure mode ErrorCode exists to avoid.
	s.syncJob.Phase, s.syncJob.Done = "done", true
	if jobErr != nil {
		s.syncJob.Error = jobErr.Error()
	}
}

func (s *server) handleSyncProgress(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.syncProgressResponse())
}

// SyncActivityHooks returns the Phase and Progress callbacks a background
// watch loop should report through, so the UI can say what the mirror is
// fetching and how far along. Safe for concurrent use; nil Handler returns
// nil funcs (callers pass them straight into sync.Options).
func (h *Handler) SyncActivityHooks() (phase func(string), progress func(fetched, changed int)) {
	if h == nil {
		return nil, nil
	}
	return h.s.reportActivityPhase, h.s.reportActivityProgress
}

// reportActivityPhase opens or closes this server's activity slot.
// source "" ends the activity; any other value starts a new pass (counters reset).
func (s *server) reportActivityPhase(source string) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if source == sync.PhaseIdle {
		s.activity = mirrorActivity{}
		return
	}
	s.activity = mirrorActivity{
		Running:   true,
		Source:    source,
		StartedAt: store.Now(),
	}
}

// reportActivityProgress updates counters for the current activity pass.
// Ignored when nothing is in flight.
func (s *server) reportActivityProgress(fetched, changed int) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if !s.activity.Running {
		return
	}
	s.activity.Fetched, s.activity.Changed = fetched, changed
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
	runs, err := s.db.SyncRuns(r.Context(), id, 20)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if runs == nil {
		runs = []store.SyncRun{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs, "source": src})
}

func (s *server) syncProgress() progressDoc {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	doc := s.syncJob
	if doc.Phase == "" {
		doc.Phase = "idle"
	}
	return doc
}

// syncProgressResponse is the polled document: one-shot job fields plus activity.
func (s *server) syncProgressResponse() progressResponse {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	doc := s.syncJob
	if doc.Phase == "" {
		doc.Phase = "idle"
	}
	return progressResponse{progressDoc: doc, Activity: s.activity}
}
