package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/create"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/jirafields"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/parenthint"
	"github.com/midagedev/gadak/internal/statuscat"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/sync"
	"github.com/midagedev/gadak/internal/transition"
)

// Write-through: every endpoint here calls Jira with the configured credential,
// re-reads the issue into the mirror, and answers with the refreshed row. There
// is no local queue — a write that Jira rejected is a write that did not happen,
// and the person who asked for it finds out immediately
// (contracts/api.md, "Write-through").

// maxUpload caps an attachment. Jira's own default is 10 MB; this is the memory
// this process is willing to hold either way.
const maxUpload = 64 << 20

// client returns the unrouted Jira client for this request. Issue write
// handlers must not call it (GDK-681: TestWriteHandlersDoNotCallClient) —
// origin.Client is Jira-only, and a Linear apiKey still passes HasCredential.
// Catalog GETs that are Jira-shaped by contract remain; each is named on
// that test's allowlist with a reason.
//
// A connected workspace without a token answers 409 credential_required so
// the UI opens its credential dialog. Standalone origin failures are mapped
// by failOriginClient — they are not a missing token.
func (s *server) client(w http.ResponseWriter) (*jira.Client, *config.Config, bool) {
	cfg := s.config()
	// HasCredential is true for standalone (no site token) and for a
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
// no token. This path is standalone persist/lock/path errors that used to
// be disguised as that same 409 (GDK-345), which opened the token dialog
// on a workspace that has no token.
func failOriginClient(w http.ResponseWriter, err error) {
	if errors.Is(err, origin.ErrWorkspaceFrozen) {
		fail(w, http.StatusConflict, "workspace_frozen")
		return
	}
	if errors.Is(err, origin.ErrWorkspaceBusy) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":   "workspace_busy",
			"message": err.Error(),
		})
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
func failJira(w http.ResponseWriter, r *http.Request, cfg *config.Config, err error) {
	err = origin.FoldPairedError(cfg, err)
	var apiErr *jira.APIError
	var confErr *confluence.APIError
	var pairErr *origin.PairingError
	switch {
	case errors.As(err, &pairErr):
		// Folded pairing sentence — do not collapse it to jira_unavailable.
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": pairErr.Error()})
	case errors.Is(err, origin.ErrWorkspaceFrozen):
		// GDK-507 (b): the client mint refused before anything left the
		// process. Same code the sync gate uses, so the web copy that
		// carries the unfreeze sentence applies to writes too.
		fail(w, http.StatusConflict, "workspace_frozen")
	case errors.Is(err, jira.ErrAuth), errors.Is(err, confluence.ErrAuth), errors.Is(err, linear.ErrAuth):
		// Stored token is wrong or expired — distinct from never having one
		// (credential_required), so the UI can say "replace your token".
		fail(w, http.StatusConflict, "credential_rejected")
	case errors.Is(err, sync.ErrNotFound), errors.Is(err, confluence.ErrNotFound):
		fail(w, http.StatusNotFound, "not_found")
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
	case errors.Is(err, origin.ErrUnsupported):
		// Origin is up and answered "I cannot do that". Not 502, and not
		// a code that says the origin is down — the sentence stays in
		// `error` (same shape as link.go).
		log.Printf("server: write refused origin capability: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case transition.IsRefused(err):
		// Caller-side refusal (bad identifier, missing required screen
		// field, unknown resolution). Origin was not written. Same 400
		// shape handleTransition used to forge as jira.APIError.
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		log.Printf("server: %s %s: %v", r.Method, r.URL.Path, err)
		fail(w, http.StatusBadGateway, "jira_unavailable")
	}
}

// wikiWriter is the wiki write gate: connected without a token is 409
// credential_required (same code as issue writes). Standalone HasCredential
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

// keyWriter is writerFor routed per key: the Jira client for jira/standalone
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
// WriterFor's default Jira-family origin (connected Jira or standalone).
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
		failJira(w, r, s.config(), err)
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

/* ── credential ── */

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

func (s *server) handleGetCredential(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, credential(s.config()))
}

func (s *server) handlePutCredential(w http.ResponseWriter, r *http.Request) {
	var body struct {
		JiraEmail      string `json:"jira_email"`
		APIToken       string `json:"api_token"`
		TokenExpiresAt string `json:"token_expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	body.JiraEmail, body.APIToken = strings.TrimSpace(body.JiraEmail), strings.TrimSpace(body.APIToken)
	// GDK-455: on a configured workspace an empty field means "keep the stored
	// value" — parity with `gadak init`. The empty check below still guards the
	// unconfigured case, where empty can only mean missing.
	cur := s.config()
	tokenReplaced := body.APIToken != ""
	if body.APIToken == "" && cur.Token != "" {
		body.APIToken = cur.Token
	}
	if body.JiraEmail == "" && cur.Email != "" {
		body.JiraEmail = cur.Email
	}
	if body.JiraEmail == "" || body.APIToken == "" {
		fail(w, http.StatusBadRequest, "email_and_token_required")
		return
	}
	next := *cur
	if next.Site == "" {
		// Nothing to verify against: the site comes from settings, not from here.
		fail(w, http.StatusBadRequest, "site_required")
		return
	}
	// Verify before storing, so a typo never becomes the stored credential.
	me, err := origin.Connected(next.Site, body.JiraEmail, body.APIToken).Myself(r.Context())
	if err != nil {
		if errors.Is(err, jira.ErrAuth) {
			fail(w, http.StatusUnauthorized, "credential_rejected")
			return
		}
		failJira(w, r, s.config(), err)
		return
	}
	next.Email, next.Token = body.JiraEmail, body.APIToken
	next.TokenOwner, next.TokenVerifiedAt = me.DisplayName, store.Now()
	next.AccountID = me.AccountID
	if err := next.ApplyTokenExpiryIfNeeded(body.TokenExpiresAt, next.TokenVerifiedAt, tokenReplaced); err != nil {
		fail(w, http.StatusBadRequest, "invalid_token_expires")
		return
	}
	if err := next.Save(); err != nil {
		serverError(w, r, err)
		return
	}
	s.cfg.Store(&next)
	s.gen.Add(1)
	writeJSON(w, http.StatusOK, credential(&next))
}

func (s *server) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	next := *s.config()
	next.Email, next.Token, next.TokenOwner, next.TokenVerifiedAt, next.AccountID = "", "", "", "", ""
	next.ClearTokenExpiry()
	if err := next.Save(); err != nil {
		serverError(w, r, err)
		return
	}
	s.cfg.Store(&next)
	s.gen.Add(1)
	writeJSON(w, http.StatusOK, credential(&next))
}

/* ── single-item resync (no Jira write; re-read only) ── */

// handleResync re-fetches one issue from Jira into the mirror and answers with
// the refreshed IssueLite. Same shape as mutate after the write step is skipped.
func (s *server) handleResync(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	_, cfg, src, ok := s.keyWriter(w, r, key)
	if !ok {
		return
	}
	if err := sync.RefreshIssue(r.Context(), cfg, s.db, key, src); err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	s.respondIssue(w, r, key, nil)
}

// handlePageResync re-fetches one Confluence page into the mirror. Success is
// 204: the client reloads detail separately. Must not advance the confluence
// watermark (see sync.SyncPage).
func (s *server) handlePageResync(w http.ResponseWriter, r *http.Request) {
	_, cfg, ok := s.client(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if err := sync.SyncPage(r.Context(), cfg, s.db, id); err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

/* ── transitions ── */

type transitionDoc struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ToStatus string `json:"to_status"`
	// ToID is the target status id — what issues_full exposes as status_id,
	// so a reader can carry it straight back into POST transition (GDK-341).
	ToID       string `json:"to_id"`
	ToCategory string `json:"to_category"`
	// Fields is required screen fields only (GDK-83). Omitted when none.
	Fields []transitionFieldDoc `json:"fields,omitempty"`
}

type transitionFieldDoc struct {
	ID      string                  `json:"id"`
	Name    string                  `json:"name"`
	Type    string                  `json:"type"`
	Options []transitionFieldOption `json:"options"`
}

type transitionFieldOption struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// requiredTransitionFields copies Required=true entries. Option labels follow
// handleEditMeta: AllowedValues.value, then name (resolution uses name).
func requiredTransitionFields(fields map[string]jira.TransitionField) []transitionFieldDoc {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for k, f := range fields {
		if f.Required {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	out := make([]transitionFieldDoc, 0, len(keys))
	for _, k := range keys {
		f := fields[k]
		opts := make([]transitionFieldOption, 0, len(f.AllowedValues))
		for _, v := range f.AllowedValues {
			label := v.Value
			if label == "" {
				label = v.Name
			}
			opts = append(opts, transitionFieldOption{ID: v.ID, Value: label})
		}
		out = append(out, transitionFieldDoc{
			ID: k, Name: f.Name, Type: f.Schema.Type, Options: opts,
		})
	}
	return out
}

func (s *server) handleTransitions(w http.ResponseWriter, r *http.Request) {
	c, _, _, ok := s.keyWriter(w, r, r.PathValue("key"))
	if !ok {
		return
	}
	list, err := c.Transitions(r.Context(), r.PathValue("key"))
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	out := make([]transitionDoc, 0, len(list))
	for _, t := range list {
		out = append(out, transitionDoc{
			ID: t.ID, Name: t.Name, ToStatus: t.To.Name, ToID: t.To.ID,
			// The mapped token (new|inprogress|done) the write resolver
			// accepts — not Jira's raw key, which includes "indeterminate"
			// and would be rejected on the round trip (GDK-564).
			ToCategory: statuscat.Category(t.To.StatusCategory.Key),
			Fields:     requiredTransitionFields(t.Fields),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"transitions": out})
}

func (s *server) handleTransition(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TransitionID string         `json:"transition_id"`
		Fields       map[string]any `json:"fields"`
		Comment      string         `json:"comment"`
		Resolution   string         `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TransitionID == "" {
		fail(w, http.StatusBadRequest, "transition_id_required")
		return
	}
	key := r.PathValue("key")
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		res, err := transition.Apply(ctx, c, s.config(), transition.Request{
			Key:        key,
			Target:     body.TransitionID,
			Resolution: body.Resolution,
			Fields:     body.Fields,
			Comment:    body.Comment,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"changed": res.Changed}, nil
	})
}

/* ── comment ── */

func (s *server) handleComment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text     string `json:"text"`
		Mentions []struct {
			AccountID   string `json:"account_id"`
			DisplayName string `json:"display_name"`
		} `json:"mentions"`
		// AttachmentIDs are ids returned by the upload endpoint. They render inline
		// in the comment body; the files are attached to the issue regardless.
		AttachmentIDs []string `json:"attachment_ids"`
		// Visibility restricts the comment to a role or group; Internal marks a
		// JSM internal comment. Same passthrough the CLI has (GDK-511/528).
		Visibility *struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"visibility"`
		Internal bool `json:"internal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		fail(w, http.StatusBadRequest, "text_required")
		return
	}
	var visibility *jira.CommentVisibility
	if body.Visibility != nil {
		if (body.Visibility.Type != "role" && body.Visibility.Type != "group") ||
			strings.TrimSpace(body.Visibility.Value) == "" {
			fail(w, http.StatusBadRequest, "visibility_needs_role_or_group")
			return
		}
		visibility = &jira.CommentVisibility{Type: body.Visibility.Type, Value: body.Visibility.Value}
	}
	mentions := map[string]string{}
	for _, m := range body.Mentions {
		if m.DisplayName != "" && m.AccountID != "" {
			mentions[m.DisplayName] = m.AccountID
		}
	}
	key := r.PathValue("key")
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		// attachment_ids arrive from the upload endpoint, which already attached the
		// files to the issue. Rendering them *inside* the comment needs Jira's media
		// UUID, which is only exposed through the attachment redirect. An id that
		// cannot be resolved is dropped rather than failing the comment: the file is
		// still attached to the issue either way.
		var media []jira.Media
		for _, id := range body.AttachmentIDs {
			if id == "" {
				continue
			}
			im, err := origin.AsMediaRef(c)
			if err != nil {
				log.Printf("server: comment media ref for attachment %s: %v", id, err)
				continue
			}
			mediaID, filename, err := im.MediaRef(ctx, id)
			if err != nil {
				log.Printf("server: comment media ref for attachment %s: %v", id, err)
				continue
			}
			media = append(media, jira.Media{ID: mediaID, Filename: filename})
		}
		created, err := c.AddComment(ctx, key, jira.DocWithMedia(body.Text, mentions, media), visibility, body.Internal)
		if err != nil {
			return nil, err
		}
		return map[string]any{"comment": map[string]any{
			"comment_id": created.ID,
			"author":     created.Author.DisplayName,
			"body":       adf.PlainText(created.Body),
			"created_at": jira.ISOTime(created.Created),
		}}, nil
	})
}

/* ── attachment upload ── */

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	c, cfg, src, ok := s.keyWriter(w, r, key)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	file, header, err := r.FormFile("file")
	if err != nil {
		fail(w, http.StatusBadRequest, "file_required")
		return
	}
	defer file.Close()

	uploaded, err := c.Upload(r.Context(), key, header.Filename, file)
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	// The issue's attachment list changed, so the mirror has to catch up before the
	// detail panel re-renders. Jira already accepted the upload: a re-read
	// failure is 502 write_applied_mirror_stale (contracts/api.md), not 200.
	if err := sync.RefreshIssue(r.Context(), cfg, s.db, key, src); err != nil {
		failMirrorStale(w, key, err)
		return
	}
	out := make([]map[string]any, 0, len(uploaded))
	for _, a := range uploaded {
		out = append(out, map[string]any{
			"id": a.ID, "filename": a.Filename, "mime_type": a.MimeType, "size": a.Size,
			"media_id":    "",
			"is_image":    strings.HasPrefix(a.MimeType, "image/"),
			"is_video":    strings.HasPrefix(a.MimeType, "video/"),
			"content_url": attachmentURL(key, a.ID),
		})
	}
	label := writeOriginLabel(src)
	log.Printf("server: write %s origin=%s", key, label)
	writeJSON(w, http.StatusOK, map[string]any{"attachments": out, "origin": label})
}

/* ── priority ── */

func writePriorityCatalog(w http.ResponseWriter, list []jira.NamedID) {
	out := make([]map[string]string, 0, len(list))
	for _, p := range list {
		if p.ID == "" {
			continue
		}
		out = append(out, map[string]string{"id": p.ID, "name": p.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"priorities": out})
}

func (s *server) handlePriorities(w http.ResponseWriter, r *http.Request) {
	c, _, ok := s.client(w)
	if !ok {
		return
	}
	list, err := c.PriorityCatalog(r.Context())
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	writePriorityCatalog(w, list)
}

func (s *server) handleKeyPriorities(w http.ResponseWriter, r *http.Request) {
	c, _, _, ok := s.keyWriter(w, r, r.PathValue("key"))
	if !ok {
		return
	}
	list, err := c.PriorityCatalog(r.Context())
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	writePriorityCatalog(w, list)
}

func (s *server) handlePriority(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PriorityID *string `json:"priority_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	key := r.PathValue("key")
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		id := strings.TrimSpace(deref(body.PriorityID))
		if id == "" {
			return nil, c.UpdateFields(ctx, key, map[string]any{"priority": nil})
		}
		return nil, c.UpdateFields(ctx, key, map[string]any{"priority": create.PriorityField(id)})
	})
}

/* ── summary ── */

// Jira Cloud's own cap on the summary field.
const maxSummary = 255

func (s *server) handleDuedate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Duedate *string `json:"duedate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	raw := strings.TrimSpace(deref(body.Duedate))
	if raw != "" && !fields.DateOnlyLiteral(raw) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("duedate %q is not a date (want YYYY-MM-DD)", raw),
		})
		return
	}
	key := r.PathValue("key")
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		if raw == "" {
			return nil, c.UpdateFields(ctx, key, map[string]any{"duedate": nil})
		}
		return nil, c.UpdateFields(ctx, key, map[string]any{"duedate": raw})
	})
}

func (s *server) handleParent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Parent *string `json:"parent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	raw := strings.TrimSpace(deref(body.Parent))
	parentKey := strings.ToUpper(raw)
	if raw != "" && !fields.IssueKeyLiteral(parentKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("parent %q is not a Jira key (want ABC-123)", raw),
		})
		return
	}
	key := r.PathValue("key")
	if raw != "" && parentKey == strings.ToUpper(strings.TrimSpace(key)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("parent %q is this issue", parentKey),
		})
		return
	}
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		var err error
		if raw == "" {
			err = c.UpdateFields(ctx, key, map[string]any{"parent": nil})
		} else {
			err = c.UpdateFields(ctx, key, map[string]any{"parent": map[string]string{"key": parentKey}})
		}
		return nil, parenthint.Wrap(err, parentKey, s.db)
	})
}

func (s *server) handleSummary(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Summary *string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Summary == nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	summary := strings.TrimSpace(*body.Summary)
	if summary == "" {
		fail(w, http.StatusBadRequest, "summary_required")
		return
	}
	if utf8.RuneCountInString(summary) > maxSummary {
		fail(w, http.StatusBadRequest, "summary_too_long")
		return
	}
	key := r.PathValue("key")
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		return nil, c.UpdateFields(ctx, key, map[string]any{"summary": summary})
	})
}

func (s *server) handleDescription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Description json.RawMessage `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Description) == 0 {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	var text string
	clear := string(body.Description) == "null"
	if !clear {
		if err := json.Unmarshal(body.Description, &text); err != nil {
			fail(w, http.StatusBadRequest, "invalid_body")
			return
		}
		text = strings.TrimSpace(text)
		clear = text == ""
	}
	key := r.PathValue("key")
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		if clear {
			return nil, c.UpdateFields(ctx, key, map[string]any{"description": nil})
		}
		return nil, c.UpdateFields(ctx, key, map[string]any{"description": jira.Doc(text, nil)})
	})
}

/* ── labels ── */

func (s *server) handleLabels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Labels *[]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Labels == nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	labels := normalizeLabels(*body.Labels)
	key := r.PathValue("key")
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		return nil, c.UpdateFields(ctx, key, map[string]any{"labels": labels})
	})
}

func normalizeLabels(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

/* ── assignee ── */

func (s *server) handleAssignee(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountID *string `json:"account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	key := r.PathValue("key")
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		return nil, c.SetAssignee(ctx, key, deref(body.AccountID))
	})
}

/* ── field edit ── */

func (s *server) handleEditMeta(w http.ResponseWriter, r *http.Request) {
	c, cfg, _, ok := s.keyWriter(w, r, r.PathValue("key"))
	if !ok {
		return
	}
	allow := fields.EditableAliases(cfg)
	if len(allow) == 0 {
		// Nil/absent config: no inline editor. A live config always includes
		// built-in system aliases; fields missing from this issue's editmeta
		// stay omitted below.
		writeJSON(w, http.StatusOK, map[string]any{"fields": map[string]any{}})
		return
	}
	meta, err := c.EditMeta(r.Context(), r.PathValue("key"))
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	out := map[string]any{}
	for alias, ea := range allow {
		id, kind, present := jirafields.ResolveEditable(ea.IDs, meta, ea.Kind)
		if !present {
			continue
		}
		m := meta[id]
		options := make([]map[string]string, 0, len(m.AllowedValues))
		for _, v := range m.AllowedValues {
			label := v.Value
			if label == "" {
				label = v.Name
			}
			options = append(options, map[string]string{"id": v.ID, "value": label})
		}
		out[alias] = map[string]any{"kind": kind, "editable": true, "options": options}
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": out})
}

func (s *server) handleFields(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Field string          `json:"field"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	key := r.PathValue("key")
	c, cfg, _, ok := s.keyWriter(w, r, key)
	if !ok {
		return
	}
	// The allowlist (+ built-in system fields) is the whole authorization story
	// for field edits: anything not on it is refused here, whether or not the UI
	// offered it.
	ea, allowed := fields.EditableAliases(cfg)[body.Field]
	if !allowed || len(ea.IDs) == 0 {
		fail(w, http.StatusForbidden, "field_not_editable")
		return
	}
	meta, err := c.EditMeta(r.Context(), key)
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	id, kind, present := jirafields.ResolveEditable(ea.IDs, meta, ea.Kind)
	if !present {
		fail(w, http.StatusForbidden, "field_not_editable")
		return
	}
	value, err := fields.FieldValue(kind, body.Value)
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_value")
		return
	}
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		return nil, c.UpdateFields(ctx, key, map[string]any{id: value})
	})
}

// failCreate maps shared create-resolution errors onto stable wire codes.
// Need* errors are surface-neutral; CLI flag names stay in cmd/gadak.
// Pairing/dial failures are not Need* — handleCreate probes before calling this.
func failCreate(w http.ResponseWriter, err error) {
	var np *create.NeedProjectError
	if errors.As(err, &np) {
		fail(w, http.StatusBadRequest, "project_required")
		return
	}
	var nt *create.NeedTypeError
	if errors.As(err, &nt) {
		fail(w, http.StatusBadRequest, "issue_type_required")
		return
	}
	var npri *create.NeedPriorityError
	if errors.As(err, &npri) {
		fail(w, http.StatusBadRequest, "priority_required")
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

// failCreateOrPairing is handleCreate's Need* path. NeedProject/NeedType are
// local config ambiguity; probe the origin so a pairing/dial failure is not
// relabeled as project_required / issue_type_required (CLI createOne, GDK-453).
// failCreate stays a pure mapping function. The probe is CreateMeta (on
// origin.Writer) rather than Jira Projects: Writer has no Projects verb,
// and a pairing/dial failure surfaces on either call.
func failCreateOrPairing(w http.ResponseWriter, r *http.Request, wr origin.Writer, cfg *config.Config, err error) {
	var np *create.NeedProjectError
	var nt *create.NeedTypeError
	if (errors.As(err, &np) || errors.As(err, &nt)) && wr != nil {
		catalog, perr := wr.CreateMeta(r.Context(), cfg.Projects)
		if perr != nil && origin.IsPairingFailure(perr) {
			failJira(w, r, cfg, perr)
			return
		}
		if errors.As(err, &np) && len(np.Configured) == 0 && perr == nil {
			err = create.FillNeedProject(err, catalog)
		}
	}
	failCreate(w, err)
}

/* ── create ── */

func (s *server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var p struct {
		ProjectKey        string   `json:"project_key"`
		IssueType         string   `json:"issue_type"`
		Summary           string   `json:"summary"`
		DescriptionText   string   `json:"description_text"`
		AssigneeAccountID *string  `json:"assignee_account_id"`
		PriorityID        string   `json:"priority_id"`
		Labels            []string `json:"labels"`
		Duedate           string   `json:"duedate"`
		Parent            string   `json:"parent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	// Only the summary is required now that project and type resolve to
	// defaults below. The code keeps its old name: callers and the i18n
	// catalog key on it (web/src/lib/i18n/en.ts), and renaming a wire code to
	// tidy it would break them for no gain in what the reader is told.
	if strings.TrimSpace(p.Summary) == "" {
		fail(w, http.StatusBadRequest, "project_issue_type_and_summary_required")
		return
	}
	c, cfg, src, ok := s.createWriter(w, r, p.ProjectKey)
	if !ok {
		return
	}
	proj, err := create.Project(p.ProjectKey, cfg)
	if err != nil {
		failCreateOrPairing(w, r, c, cfg, err)
		return
	}
	// An issue filed outside the mirrored projects would never come back from the
	// re-read, so refuse it here rather than answering with a stale-mirror error.
	// The empty-list semantics ("no explicit scope", not deny-all) live in
	// Config.ProjectMirrored — the one owner shared with the CLI pre-check.
	if !cfg.ProjectMirrored(proj.Value) {
		log.Printf("server: create refused project %q: not in the mirrored list (%d configured)", proj.Value, len(cfg.Projects))
		fail(w, http.StatusBadRequest, "project_not_mirrored")
		return
	}
	meta, err := c.CreateMeta(r.Context(), []string{proj.Value})
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	metaProj, types, err := create.MetaForWithCatalog(r.Context(), c, meta, proj.Value, cfg)
	if err != nil {
		failCreateOrPairing(w, r, c, cfg, err)
		return
	}
	typ, err := create.Type(p.IssueType, types, cfg, proj.Value)
	if err != nil {
		failCreateOrPairing(w, r, c, cfg, err)
		return
	}

	// The due date is validated before payload assembly: the map below is
	// named fields and would shadow the package for the check.
	duedate := strings.TrimSpace(p.Duedate)
	if duedate != "" && !fields.DateOnlyLiteral(duedate) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("duedate %q is not a date (want YYYY-MM-DD)", p.Duedate),
		})
		return
	}
	parent := strings.TrimSpace(p.Parent)
	parentKey := strings.ToUpper(parent)
	if parent != "" && !fields.IssueKeyLiteral(parentKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("parent %q is not a Jira key (want ABC-123)", p.Parent),
		})
		return
	}

	fields := map[string]any{
		// map[string]any so Linear CreateIssue can read project.key
		// (map[string]string fails that type assert).
		"project": map[string]any{"key": metaProj.Key},
		"summary": p.Summary,
	}
	// Linear CreateIssue refuses issuetype; CLI createLinearOne omits it.
	if src != "linear" {
		fields["issuetype"] = map[string]string{"id": typ.Value}
	}
	// Optional fields are omitted, never sent as "". Empty string is "no
	// value" (resolve / skip), not "set this field to empty".
	if strings.TrimSpace(p.DescriptionText) != "" {
		fields["description"] = jira.Doc(p.DescriptionText, nil)
	}
	if id := deref(p.AssigneeAccountID); id != "" {
		fields["assignee"] = map[string]string{"accountId": id}
	}
	if id := strings.TrimSpace(p.PriorityID); id != "" {
		fields["priority"] = create.PriorityField(id)
	}
	if labels := normalizeLabels(p.Labels); len(labels) > 0 {
		fields["labels"] = labels
	}
	if duedate != "" {
		fields["duedate"] = duedate
	}
	if parentKey != "" {
		fields["parent"] = map[string]string{"key": parentKey}
	}

	key, err := c.CreateIssue(r.Context(), fields)
	if err != nil {
		failJira(w, r, s.config(), parenthint.Wrap(err, parentKey, s.db))
		return
	}
	if err := sync.RefreshIssue(r.Context(), cfg, s.db, key, src); err != nil {
		failMirrorStale(w, key, err)
		return
	}
	label := writeOriginLabel(src)
	log.Printf("server: write %s origin=%s", key, label)
	s.respondIssue(w, r, key, map[string]any{
		"origin": label,
		"resolved": map[string]any{
			"project":    proj,
			"issue_type": typ,
		},
	})
}

/* ── metadata ── */

func (s *server) handleCreateMeta(w http.ResponseWriter, r *http.Request) {
	c, cfg, ok := s.client(w)
	if !ok {
		return
	}
	projects, err := c.CreateMeta(r.Context(), cfg.Projects)
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": createMeta(projects)})
}

// handleCreateFields is the create-time field list for one project+type
// (GDK-254). The server does not decide which fields to warn about: it
// returns every field the origin listed. Not a boot path — missing
// credential is 409, same as create-meta/. Origin errors go through
// failJira so they stay 4xx/502, never 500.
func (s *server) handleCreateFields(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	issueType := strings.TrimSpace(r.URL.Query().Get("issue_type"))
	if project == "" || issueType == "" {
		fail(w, http.StatusBadRequest, "project_and_issue_type_required")
		return
	}
	c, _, ok := s.client(w)
	if !ok {
		return
	}
	list, err := c.CreateFields(r.Context(), project, issueType)
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, f := range list {
		out = append(out, map[string]any{
			"field_id":    f.FieldID,
			"name":        f.Name,
			"required":    f.Required,
			"has_default": f.HasDefaultValue,
			"type":        f.Schema.Type,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": out})
}

func createMeta(projects []jira.CreateMetaProject) []map[string]any {
	out := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		types := make([]map[string]any, 0, len(p.IssueTypes))
		for _, t := range p.IssueTypes {
			row := map[string]any{"id": t.ID, "name": t.Name}
			// Same omitempty as jira.CreateMetaIssueType: false/0 stay
			// off the wire so older clients see the previous shape.
			if t.Subtask {
				row["subtask"] = true
			}
			if t.HierarchyLevel != 0 {
				row["hierarchyLevel"] = t.HierarchyLevel
			}
			types = append(types, row)
		}
		out = append(out, map[string]any{"key": p.Key, "name": p.Name, "issue_types": types})
	}
	return out
}

// handleWriteMeta is the boot-time cache for the write UI. The transition map is
// empty on purpose: filling it costs one Jira call per project and status, and the
// client already falls back to fetching an issue's transitions when it opens the
// menu. An unconfigured credential answers 200 with nothing rather than an error,
// because this runs on every boot.
//
// ponytail: no precomputed transitions. Fill it if opening the status menu ever
// feels slow.
func (s *server) handleWriteMeta(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{
		"transitions": map[string]any{},
		"create_meta": map[string]any{"projects": []map[string]any{}},
		"updated_at":  nil,
	}
	cfg := s.config()
	if !cfg.HasCredential() {
		writeJSON(w, http.StatusOK, body)
		return
	}
	c, err := origin.Client(cfg)
	if err != nil {
		log.Printf("server: meta/write: %v", err)
		writeJSON(w, http.StatusOK, body)
		return
	}
	projects, err := c.CreateMeta(r.Context(), cfg.Projects)
	if err != nil {
		// Degrade rather than block the boot: every surface this feeds has a fallback.
		log.Printf("server: meta/write: %v", err)
		writeJSON(w, http.StatusOK, body)
		return
	}
	body["create_meta"] = map[string]any{"projects": createMeta(projects)}
	body["updated_at"] = store.Now()
	writeJSON(w, http.StatusOK, body)
}

func writeUserCatalog(w http.ResponseWriter, users []jira.User) {
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		row := map[string]any{
			"account_id": u.AccountID, "display_name": u.DisplayName, "email": u.Email,
			"avatar_url": u.Avatar(), "active": u.Active,
		}
		// The bot axis (GDK-590), so a picker can de-emphasize bots without
		// guessing from names. Omitted when the origin sent no accountType.
		if u.AccountType != "" {
			row["account_type"] = u.AccountType
			row["is_bot"] = jira.IsBotAccountType(u.AccountType)
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (s *server) handleUsers(w http.ResponseWriter, r *http.Request) {
	c, _, ok := s.client(w)
	if !ok {
		return
	}
	users, err := c.SearchUsers(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	writeUserCatalog(w, users)
}

func (s *server) handleKeyUsers(w http.ResponseWriter, r *http.Request) {
	c, _, _, ok := s.keyWriter(w, r, r.PathValue("key"))
	if !ok {
		return
	}
	users, err := c.SearchUsers(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	writeUserCatalog(w, users)
}

/* ── page edit (GDK-380) ── */

// handlePageEdit PUTs one wiki page through the owning origin: connected
// Confluence or the in-process issuetap (origin.Wiki owns that choice).
// Body: {"title": ...} and/or {"adf": "<ADF JSON string>"} and/or
// {"text": ...} — text is built into a fresh ADF doc and REPLACES the whole
// body, so callers editing rich pages should send adf. Omitted parts keep
// the origin's current value. Optional "version" is the caller's base
// (optimistic lock: that value+1, origin 409 if stale). Omitted version is
// last-write-wins from origin HEAD+1. "force": true skips the format_loss
// gate on a text replace of a non-simple origin ADF.
func (s *server) handlePageEdit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title   string  `json:"title"`
		ADF     string  `json:"adf"`
		Text    *string `json:"text"`
		Version *int    `json:"version"`
		Force   bool    `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if body.Title == "" && body.ADF == "" && body.Text == nil {
		fail(w, http.StatusBadRequest, "nothing_to_edit")
		return
	}
	wc, cfg, ok := s.wikiWriter(w)
	if !ok {
		return
	}
	if body.ADF != "" && !validADF(body.ADF) {
		fail(w, http.StatusBadRequest, "invalid_adf")
		return
	}
	id := r.PathValue("id")
	cur, err := wc.Page(r.Context(), id)
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	title := cur.Title
	if body.Title != "" {
		title = body.Title
	}
	// doc, not adf: the package name is adf, and shadowing it here is what
	// kept the richness judgment a private copy in this file (GDK-682).
	doc := ""
	if cur.Body.AtlasDocFormat != nil {
		doc = cur.Body.AtlasDocFormat.Value
	}
	switch {
	case body.ADF != "":
		doc = body.ADF
	case body.Text != nil:
		if !body.Force && !adf.IsSimple(doc) {
			fail(w, http.StatusConflict, "format_loss")
			return
		}
		doc = string(jira.Doc(*body.Text, nil))
	}
	next := cur.Version.Number + 1
	if body.Version != nil {
		next = *body.Version + 1
	}
	if _, err := wc.UpdatePage(r.Context(), id, title, doc, next); err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	if err := sync.RefreshPage(r.Context(), cfg, s.db, id); err != nil {
		failMirrorStale(w, id, err)
		return
	}
	detail, err := s.db.PageDetail(r.Context(), id)
	if err != nil || detail == nil {
		failMirrorStale(w, id, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": detail})
}

// handlePageCreate POSTs a new wiki page through the owning origin (GDK-382).
// Body: {"space": KEY, "title": ..., "adf"?: "<ADF JSON string>", "text"?: ...,
// "parent"?: page id}. Responds with the mirrored PageDetail.
func (s *server) handlePageCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Space  string `json:"space"`
		Title  string `json:"title"`
		ADF    string `json:"adf"`
		Text   string `json:"text"`
		Parent string `json:"parent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if body.Space == "" || strings.TrimSpace(body.Title) == "" {
		fail(w, http.StatusBadRequest, "space_and_title_required")
		return
	}
	wc, cfg, ok := s.wikiWriter(w)
	if !ok {
		return
	}
	adf := body.ADF
	if adf == "" {
		adf = string(jira.Doc(body.Text, nil))
	} else if !validADF(adf) {
		fail(w, http.StatusBadRequest, "invalid_adf")
		return
	}
	created, err := wc.CreatePage(r.Context(), body.Space, body.Title, adf, body.Parent)
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	if err := sync.RefreshPage(r.Context(), cfg, s.db, created.ID); err != nil {
		failMirrorStale(w, created.ID, err)
		return
	}
	detail, err := s.db.PageDetail(r.Context(), created.ID)
	if err != nil || detail == nil {
		failMirrorStale(w, created.ID, nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"page": detail})
}

// handlePageComment POSTs a top-level comment on one wiki page through the
// owning origin (GDK-381). Body: {"adf": "<ADF JSON string>"} or {"text": ...}.
func (s *server) handlePageComment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ADF  string `json:"adf"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	wc, cfg, ok := s.wikiWriter(w)
	if !ok {
		return
	}
	adf := body.ADF
	if adf == "" {
		if strings.TrimSpace(body.Text) == "" {
			fail(w, http.StatusBadRequest, "empty_comment")
			return
		}
		adf = string(jira.Doc(body.Text, nil))
	} else if !validADF(adf) {
		fail(w, http.StatusBadRequest, "invalid_adf")
		return
	}
	id := r.PathValue("id")
	if _, err := wc.AddPageComment(r.Context(), id, adf); err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	if err := sync.RefreshPage(r.Context(), cfg, s.db, id); err != nil {
		failMirrorStale(w, id, err)
		return
	}
	detail, err := s.db.PageDetail(r.Context(), id)
	if err != nil || detail == nil {
		failMirrorStale(w, id, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": detail})
}
