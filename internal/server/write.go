package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/parenthint"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/sync"
	"github.com/midagedev/gadak/internal/transition"
)

// Write-through: every endpoint here calls Jira with the configured credential,
// re-reads the issue into the mirror, and answers with the refreshed row. There
// is no local queue — a write that Jira rejected is a write that did not happen,
// and the person who asked for it finds out immediately
// (contracts/api.md, "Write-through").
//
// Split by resource into write_{credential,issue,create,catalog,page}.go
// (GDK-1922); this file keeps the write-through helpers every one of them
// routes through.

// client returns the unrouted Jira client for this request. Issue write
// handlers must not call it (GDK-681: TestWriteHandlersDoNotCallClient) —
// origin.Client is Jira-only, and a Linear apiKey still passes HasCredential.
// Catalog GETs that are Jira-shaped by contract remain; each is named on
// that test's allowlist with a reason.
//
// A connected workspace without a token answers 409 credential_required so
// the UI opens its credential dialog. Built-in origin failures are mapped
// by failOriginClient — they are not a missing token.
func (s *server) client(w http.ResponseWriter) (*jira.Client, *config.Config, bool) {
	cfg := s.config()
	// HasCredential is true for built-in (no site token) and for a
	// connected workspace that has site+email+token. Connected without
	// a token still 409s here — that gate is not weakened.
	if !cfg.HasCredential() {
		fail(w, http.StatusConflict, "credential_required")
		return nil, nil, false
	}
	c, err := origin.Client(cfg)
	if err != nil {
		failOriginClient(w, err)
		return nil, nil, false
	}
	return c, cfg, true
}

// failOriginClient maps origin.Client construction failures. HasCredential
// already answers 409 credential_required when a connected workspace has
// no token. This path is built-in persist/path errors that used to
// be disguised as that same 409 (GDK-345), which opened the token dialog
// on a workspace that has no token.
func failOriginClient(w http.ResponseWriter, err error) {
	if errors.Is(err, origin.ErrWorkspaceFrozen) {
		fail(w, http.StatusConflict, "workspace_frozen")
		return
	}
	log.Printf("server: origin client: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error": err.Error(),
	})
}

// failJira turns an origin failure into the body the client parses: `error` for
// the message it shows, `jira_errors` for Jira's per-field rejections, and
// `message` for a Confluence/origin snippet. Confluence and Linear sentinels
// share this mapper with Jira so a wiki write does not become 502 jira_unavailable.
// It returns the status it wrote so mutate can log the failure (GDK-1982);
// callers that only answer keep ignoring it.
func failJira(w http.ResponseWriter, r *http.Request, cfg *config.Config, err error) int {
	err = origin.FoldPairedError(cfg, err)
	var apiErr *jira.APIError
	var confErr *confluence.APIError
	var pairErr *origin.PairingError
	switch {
	case errors.As(err, &pairErr):
		// Folded pairing sentence — do not collapse it to jira_unavailable.
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": pairErr.Error()})
		return http.StatusBadGateway
	case errors.Is(err, origin.ErrWorkspaceFrozen):
		// GDK-507 (b): the client mint refused before anything left the
		// process. Same code the sync gate uses, so the web copy that
		// carries the unfreeze sentence applies to writes too.
		fail(w, http.StatusConflict, "workspace_frozen")
		return http.StatusConflict
	case errors.Is(err, jira.ErrAuth), errors.Is(err, confluence.ErrAuth), errors.Is(err, linear.ErrAuth):
		// Stored token is wrong or expired — distinct from never having one
		// (credential_required), so the UI can say "replace your token".
		fail(w, http.StatusConflict, "credential_rejected")
		return http.StatusConflict
	case errors.Is(err, sync.ErrNotFound), errors.Is(err, confluence.ErrNotFound):
		fail(w, http.StatusNotFound, "not_found")
		return http.StatusNotFound
	case errors.As(err, &apiErr):
		status := apiErr.Status
		if status < 400 || status > 499 {
			status = http.StatusBadGateway
		}
		msg := apiErr.Message()
		// parenthint.Wrap appends the mirror hierarchy sentence without
		// replacing the origin 400 (GDK-19). Surface it in `error` the
		// same way the CLI wraps — a new field would miss the web toast,
		// which only reads `error` / `jira_errors`.
		var hinted *parenthint.Hinted
		if errors.As(err, &hinted) && hinted.Hint != "" {
			msg = msg + "\n" + hinted.Hint
		}
		body := map[string]any{"error": msg}
		if len(apiErr.Errors) > 0 {
			body["jira_errors"] = apiErr.Errors
		}
		writeJSON(w, status, body)
		return status
	case errors.As(err, &confErr):
		status := confErr.Status
		if status < 400 || status > 499 {
			status = http.StatusBadGateway
		}
		msg := confErr.Body
		if msg == "" {
			msg = confErr.Error()
		}
		writeJSON(w, status, map[string]string{
			"error":   "origin_rejected",
			"message": msg,
		})
		return status
	case errors.Is(err, origin.ErrUnsupported):
		// Origin is up and answered "I cannot do that". Not 502, and not
		// a code that says the origin is down — the sentence stays in
		// `error` (same shape as link.go).
		log.Printf("server: write refused origin capability: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return http.StatusBadRequest
	case transition.IsRefused(err):
		// Caller-side refusal (bad identifier, missing required screen
		// field, unknown resolution). Origin was not written. Same 400
		// shape handleTransition used to forge as jira.APIError.
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return http.StatusBadRequest
	default:
		log.Printf("server: %s %s: %v", r.Method, r.URL.Path, err)
		fail(w, http.StatusBadGateway, "jira_unavailable")
		return http.StatusBadGateway
	}
}

// wikiWriter is the wiki write gate: connected without a token is 409
// credential_required (same code as issue writes). Built-in HasCredential
// is true, so it passes through to origin.Wiki.
func (s *server) wikiWriter(w http.ResponseWriter) (*confluence.Client, *config.Config, bool) {
	cfg := s.config()
	if !cfg.HasCredential() {
		fail(w, http.StatusConflict, "credential_required")
		return nil, nil, false
	}
	wc, err := origin.Wiki(cfg)
	if err != nil {
		failOriginClient(w, err)
		return nil, nil, false
	}
	return wc, cfg, true
}

// validADF is the REST adf-string gate: well-formed JSON whose top-level
// type is "doc". Empty is the caller's problem (text path / empty_comment).
func validADF(adf string) bool {
	b := []byte(adf)
	if !json.Valid(b) {
		return false
	}
	var top struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &top); err != nil {
		return false
	}
	return top.Type == "doc"
}

// keySource reads which origin owns a key. A key the mirror does not know
// (or a read error) answers "" — the default origin — because refusing the
// write would break the one case that matters there: a row that has not
// synced yet still belongs to the workspace's own tracker.
func (s *server) keySource(ctx context.Context, key string) (string, error) {
	src, err := s.db.KeySource(ctx, key)
	if err != nil {
		if errors.Is(err, store.ErrKeyAmbiguous) {
			// Two sources mint this key; routing by preference wrote to a
			// tracker the screen was not showing (GDK-400). Refuse.
			return "", err
		}
		return "", nil
	}
	return src, nil
}

// writerFor is the single mint for an issue write: origin.WriterFor plus
// the owning origin's credential gate. keyWriter and createWriter both
// end here so a Linear apiKey cannot 409-skip a Jira row (or the reverse).
func (s *server) writerFor(w http.ResponseWriter, src string) (origin.Writer, *config.Config, bool) {
	cfg := s.config()
	// The credential gate is the owning origin's: a Linear row needs the
	// Linear key (HasCredential counts it), a Jira row needs the Atlassian
	// credential — a Linear apiKey must not 409-skip that.
	if src != "linear" && !cfg.HasAtlassianCredential() {
		fail(w, http.StatusConflict, "credential_required")
		return nil, nil, false
	}
	c, err := origin.WriterFor(cfg, src)
	if err != nil {
		failOriginClient(w, err)
		return nil, nil, false
	}
	return c, cfg, true
}

// keyWriter is writerFor routed per key: the Jira client for jira/built-in
// rows, the Linear adapter for linear rows (GDK-361).
func (s *server) keyWriter(w http.ResponseWriter, r *http.Request, key string) (origin.Writer, *config.Config, string, bool) {
	src, err := s.keySource(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":   "key_ambiguous",
			"message": err.Error(),
		})
		return nil, nil, "", false
	}
	c, cfg, ok := s.writerFor(w, src)
	return c, cfg, src, ok
}

// createWriter is writerFor routed the way CLI withCreateSession is: a
// project the mirror already knows as Linear goes there; a Linear-only
// workspace (no Atlassian credential) always routes to Linear. The routing
// rule itself is origin.ResolveCreateSource, shared with the CLI (GDK-820).
func (s *server) createWriter(w http.ResponseWriter, r *http.Request, project string) (origin.Writer, *config.Config, string, bool) {
	src, err := origin.ResolveCreateSource(r.Context(), s.config(), s.db, project)
	if err != nil {
		if errors.Is(err, store.ErrKeyAmbiguous) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":   "key_ambiguous",
				"message": err.Error(),
			})
			return nil, nil, "", false
		}
		serverError(w, r, err)
		return nil, nil, "", false
	}
	c, cfg, ok := s.writerFor(w, src)
	return c, cfg, src, ok
}

// writeOriginLabel is the source id the write actually used. Empty is
// WriterFor's default Jira-family origin (connected Jira or builtIn).
func writeOriginLabel(src string) string {
	if src == "" {
		return sync.SourceID
	}
	return src
}

// mutate is the whole write-through shape: call the origin that owns the
// key, re-read the issue, answer with the refreshed row plus whatever else
// the endpoint adds.
func (s *server) mutate(w http.ResponseWriter, r *http.Request, key string,
	fn func(context.Context, origin.Writer) (map[string]any, error)) {
	c, cfg, src, ok := s.keyWriter(w, r, key)
	if !ok {
		return
	}
	extra, err := fn(r.Context(), c)
	if err != nil {
		status := failJira(w, r, s.config(), err)
		// A refused or failed write must be visible where it happens
		// (GDK-1982): a 400 refusal — the transition ambiguity this fix
		// grew out of — was the one branch failJira never logged, so in
		// production it left nothing in the journal at all. Verb, route,
		// key, status, refusal text; never a body, token or header value.
		// A successful write keeps its single existing line below.
		log.Printf("server: write failed %s %s key=%s status=%d: %v", r.Method, r.URL.Path, key, status, err)
		return
	}
	if err := sync.RefreshIssue(r.Context(), cfg, s.db, key, src); err != nil {
		failMirrorStale(w, key, err)
		return
	}
	if extra == nil {
		extra = map[string]any{}
	}
	extra["origin"] = writeOriginLabel(src)
	log.Printf("server: write %s origin=%s", key, extra["origin"])
	s.respondIssue(w, r, key, extra)
}

func (s *server) respondIssue(w http.ResponseWriter, r *http.Request, key string, extra map[string]any) {
	st, err := s.db.SyncState(r.Context(), sourceID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	lites, err := s.db.IssueLites(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	view, err := s.derived(r.Context(), st.Version, lites)
	if err != nil {
		serverError(w, r, err)
		return
	}
	for _, l := range lites {
		if l.IssueKey != key {
			continue
		}
		body := map[string]any{"issue": view.issues([]store.IssueLite{l})[0]}
		for k, v := range extra {
			body[k] = v
		}
		writeJSON(w, http.StatusOK, body)
		return
	}
	failMirrorStale(w, key, nil)
}

// failMirrorStale is the one REST owner of 502 write_applied_mirror_stale.
// Status and wire code stay byte-identical to contracts/api.md; call sites
// classify a landed write whose re-read failed rather than each deciding
// the code. handleResync / handlePageResync stay on failJira (re-read only).
func failMirrorStale(w http.ResponseWriter, key string, err error) {
	if err != nil {
		log.Printf("server: mirror refresh after write to %s: %v", key, err)
	}
	fail(w, http.StatusBadGateway, "write_applied_mirror_stale")
}

type credentialDoc struct {
	Configured  bool   `json:"configured"`
	JiraEmail   string `json:"jira_email"`
	DisplayName string `json:"display_name"`
	VerifiedAt  string `json:"verified_at"`
	TokenHint   string `json:"token_hint"`
	Linear      bool   `json:"linear"`
}

// credential never carries the token. The hint is the last four characters, which
// is enough to tell two tokens apart and useless to anyone who steals it.
func credential(cfg *config.Config) credentialDoc {
	d := credentialDoc{
		Configured:  cfg.HasCredential(),
		JiraEmail:   cfg.Email,
		DisplayName: cfg.TokenOwner,
		VerifiedAt:  cfg.TokenVerifiedAt,
		Linear:      cfg.Linear != nil,
	}
	if n := len(cfg.Token); n > 4 {
		d.TokenHint = "…" + cfg.Token[n-4:]
	}
	return d
}
