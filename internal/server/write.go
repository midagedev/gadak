package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
	"github.com/midagedev/scry/internal/sync"
)

// Write-through: every endpoint here calls Jira with the configured credential,
// re-reads the issue into the mirror, and answers with the refreshed row. There
// is no local queue — a write that Jira rejected is a write that did not happen,
// and the person who asked for it finds out immediately
// (contracts/api.md, "Write-through").

// maxUpload caps an attachment. Jira's own default is 10 MB; this is the memory
// this process is willing to hold either way.
const maxUpload = 64 << 20

// client returns the Jira client for this request, or answers 409 so the UI opens
// its credential dialog.
func (s *server) client(w http.ResponseWriter) (*jira.Client, *config.Config, bool) {
	cfg := s.config()
	if !cfg.HasCredential() {
		fail(w, http.StatusConflict, "credential_required")
		return nil, nil, false
	}
	return jira.New(cfg.Site, cfg.Email, cfg.Token), cfg, true
}

// failJira turns a Jira failure into the body the client parses: `error` for the
// message it shows, `jira_errors` for the per-field rejections it renders inline.
func failJira(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *jira.APIError
	switch {
	case errors.Is(err, jira.ErrAuth):
		// The stored token is wrong or expired: same signal as having none.
		fail(w, http.StatusConflict, "credential_required")
	case errors.Is(err, sync.ErrNotFound):
		fail(w, http.StatusNotFound, "not_found")
	case errors.As(err, &apiErr):
		status := apiErr.Status
		if status < 400 || status > 499 {
			status = http.StatusBadGateway
		}
		body := map[string]any{"error": apiErr.Message()}
		if len(apiErr.Errors) > 0 {
			body["jira_errors"] = apiErr.Errors
		}
		writeJSON(w, status, body)
	default:
		log.Printf("server: %s %s: %v", r.Method, r.URL.Path, err)
		fail(w, http.StatusBadGateway, "jira_unavailable")
	}
}

// mutate is the whole write-through shape: call Jira, re-read the issue, answer
// with the refreshed row plus whatever else the endpoint adds.
func (s *server) mutate(w http.ResponseWriter, r *http.Request, key string,
	fn func(context.Context, *jira.Client) (map[string]any, error)) {
	c, cfg, ok := s.client(w)
	if !ok {
		return
	}
	extra, err := fn(r.Context(), c)
	if err != nil {
		failJira(w, r, err)
		return
	}
	if err := sync.SyncIssue(r.Context(), cfg, s.db, key, sync.Options{Client: c}); err != nil {
		// The write landed; only the re-read failed. Say so rather than reporting a
		// failure the user would retry.
		log.Printf("server: mirror refresh after write to %s: %v", key, err)
		fail(w, http.StatusBadGateway, "write_applied_mirror_stale")
		return
	}
	s.respondIssue(w, r, key, extra)
}

func (s *server) respondIssue(w http.ResponseWriter, r *http.Request, key string, extra map[string]any) {
	st, err := s.db.SyncState(sourceID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	lites, err := s.db.IssueLites()
	if err != nil {
		serverError(w, r, err)
		return
	}
	view, err := s.derived(st.Version, lites)
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
	fail(w, http.StatusBadGateway, "write_applied_mirror_stale")
}

/* ── credential ── */

type credentialDoc struct {
	Configured  bool   `json:"configured"`
	JiraEmail   string `json:"jira_email"`
	DisplayName string `json:"display_name"`
	VerifiedAt  string `json:"verified_at"`
	TokenHint   string `json:"token_hint"`
}

// credential never carries the token. The hint is the last four characters, which
// is enough to tell two tokens apart and useless to anyone who steals it.
func credential(cfg *config.Config) credentialDoc {
	d := credentialDoc{
		Configured:  cfg.HasCredential(),
		JiraEmail:   cfg.Email,
		DisplayName: cfg.TokenOwner,
		VerifiedAt:  cfg.TokenVerifiedAt,
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
		JiraEmail string `json:"jira_email"`
		APIToken  string `json:"api_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	body.JiraEmail, body.APIToken = strings.TrimSpace(body.JiraEmail), strings.TrimSpace(body.APIToken)
	if body.JiraEmail == "" || body.APIToken == "" {
		fail(w, http.StatusBadRequest, "email_and_token_required")
		return
	}
	next := *s.config()
	if next.Site == "" {
		// Nothing to verify against: the site comes from settings, not from here.
		fail(w, http.StatusBadRequest, "site_required")
		return
	}
	// Verify before storing, so a typo never becomes the stored credential.
	me, err := jira.New(next.Site, body.JiraEmail, body.APIToken).Myself(r.Context())
	if err != nil {
		if errors.Is(err, jira.ErrAuth) {
			fail(w, http.StatusUnauthorized, "credential_rejected")
			return
		}
		failJira(w, r, err)
		return
	}
	next.Email, next.Token = body.JiraEmail, body.APIToken
	next.TokenOwner, next.TokenVerifiedAt = me.DisplayName, store.Now()
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
	next.Email, next.Token, next.TokenOwner, next.TokenVerifiedAt = "", "", "", ""
	if err := next.Save(); err != nil {
		serverError(w, r, err)
		return
	}
	s.cfg.Store(&next)
	s.gen.Add(1)
	writeJSON(w, http.StatusOK, credential(&next))
}

/* ── transitions ── */

type transitionDoc struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ToStatus   string `json:"to_status"`
	ToCategory string `json:"to_category"`
}

func (s *server) handleTransitions(w http.ResponseWriter, r *http.Request) {
	c, _, ok := s.client(w)
	if !ok {
		return
	}
	list, err := c.Transitions(r.Context(), r.PathValue("key"))
	if err != nil {
		failJira(w, r, err)
		return
	}
	out := make([]transitionDoc, 0, len(list))
	for _, t := range list {
		out = append(out, transitionDoc{
			ID: t.ID, Name: t.Name, ToStatus: t.To.Name,
			// Jira's own category key, which is what the client's type documents.
			ToCategory: t.To.StatusCategory.Key,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"transitions": out})
}

func (s *server) handleTransition(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TransitionID string `json:"transition_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TransitionID == "" {
		fail(w, http.StatusBadRequest, "transition_id_required")
		return
	}
	key := r.PathValue("key")
	s.mutate(w, r, key, func(ctx context.Context, c *jira.Client) (map[string]any, error) {
		return nil, c.Transition(ctx, key, body.TransitionID)
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
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		fail(w, http.StatusBadRequest, "text_required")
		return
	}
	mentions := map[string]string{}
	for _, m := range body.Mentions {
		if m.DisplayName != "" && m.AccountID != "" {
			mentions[m.DisplayName] = m.AccountID
		}
	}
	key := r.PathValue("key")
	s.mutate(w, r, key, func(ctx context.Context, c *jira.Client) (map[string]any, error) {
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
			mediaID, filename, err := c.MediaRef(ctx, id)
			if err != nil {
				log.Printf("server: comment media ref for attachment %s: %v", id, err)
				continue
			}
			media = append(media, jira.Media{ID: mediaID, Filename: filename})
		}
		created, err := c.AddComment(ctx, key, jira.DocWithMedia(body.Text, mentions, media))
		if err != nil {
			return nil, err
		}
		return map[string]any{"comment": map[string]any{
			"comment_id": created.ID,
			"author":     created.Author.DisplayName,
			"body":       jira.PlainText(created.Body),
			"created_at": jira.ISOTime(created.Created),
		}}, nil
	})
}

/* ── attachment upload ── */

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	c, cfg, ok := s.client(w)
	if !ok {
		return
	}
	key := r.PathValue("key")
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	file, header, err := r.FormFile("file")
	if err != nil {
		fail(w, http.StatusBadRequest, "file_required")
		return
	}
	defer file.Close()

	uploaded, err := c.Upload(r.Context(), key, header.Filename, file)
	if err != nil {
		failJira(w, r, err)
		return
	}
	// The issue's attachment list changed, so the mirror has to catch up before the
	// detail panel re-renders.
	if err := sync.SyncIssue(r.Context(), cfg, s.db, key, sync.Options{Client: c}); err != nil {
		log.Printf("server: mirror refresh after upload to %s: %v", key, err)
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
	writeJSON(w, http.StatusOK, map[string]any{"attachments": out})
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
	s.mutate(w, r, key, func(ctx context.Context, c *jira.Client) (map[string]any, error) {
		return nil, c.SetAssignee(ctx, key, deref(body.AccountID))
	})
}

/* ── field edit ── */

// editKinds maps a Jira field schema onto the three editors the UI has. A field
// whose schema is none of them has no editor and is left out of editmeta.
func editKind(m jira.FieldMeta) string {
	switch {
	case m.Schema.Type == "option":
		return "option"
	case m.Schema.Type == "user":
		return "user"
	case m.Schema.Type == "array" && m.Schema.Items == "version":
		return "version_array"
	}
	return ""
}

func (s *server) handleEditMeta(w http.ResponseWriter, r *http.Request) {
	c, cfg, ok := s.client(w)
	if !ok {
		return
	}
	if len(cfg.EditableFields) == 0 {
		// No allowlist means no inline editor at all, which is the default.
		writeJSON(w, http.StatusOK, map[string]any{"fields": map[string]any{}})
		return
	}
	meta, err := c.EditMeta(r.Context(), r.PathValue("key"))
	if err != nil {
		failJira(w, r, err)
		return
	}
	fields := map[string]any{}
	for alias, id := range cfg.EditableFields {
		m, present := meta[id]
		kind := editKind(m)
		if !present || kind == "" {
			continue
		}
		options := make([]map[string]string, 0, len(m.AllowedValues))
		for _, v := range m.AllowedValues {
			label := v.Value
			if label == "" {
				label = v.Name
			}
			options = append(options, map[string]string{"id": v.ID, "value": label})
		}
		fields[alias] = map[string]any{"kind": kind, "editable": true, "options": options}
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields})
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
	c, cfg, ok := s.client(w)
	if !ok {
		return
	}
	// The allowlist is the whole authorization story for field edits: anything not
	// configured is refused here, whether or not the UI offered it.
	id, allowed := cfg.EditableFields[body.Field]
	if !allowed || id == "" {
		fail(w, http.StatusForbidden, "field_not_editable")
		return
	}
	key := r.PathValue("key")
	meta, err := c.EditMeta(r.Context(), key)
	if err != nil {
		failJira(w, r, err)
		return
	}
	kind := editKind(meta[id])
	if _, present := meta[id]; !present || kind == "" {
		fail(w, http.StatusForbidden, "field_not_editable")
		return
	}
	value, err := fieldValue(kind, body.Value)
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_value")
		return
	}
	s.mutate(w, r, key, func(ctx context.Context, c *jira.Client) (map[string]any, error) {
		return nil, c.UpdateFields(ctx, key, map[string]any{id: value})
	})
}

// fieldValue shapes the client's `string | string[] | null` into what Jira wants
// for that kind. A null clears the field, which is what the editor's "없음" does.
func fieldValue(kind string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		if kind == "version_array" {
			return []any{}, nil
		}
		return nil, nil
	}
	if kind == "version_array" {
		var ids []string
		if err := json.Unmarshal(raw, &ids); err != nil {
			return nil, err
		}
		out := make([]any, 0, len(ids))
		for _, id := range ids {
			out = append(out, map[string]string{"id": id})
		}
		return out, nil
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	if kind == "user" {
		return map[string]string{"accountId": id}, nil
	}
	return map[string]string{"id": id}, nil
}

/* ── create ── */

func (s *server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var p struct {
		ProjectKey        string   `json:"project_key"`
		IssueType         string   `json:"issue_type"`
		Summary           string   `json:"summary"`
		DescriptionText   string   `json:"description_text"`
		AssigneeAccountID *string  `json:"assignee_account_id"`
		Priority          string   `json:"priority"`
		Labels            []string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if p.ProjectKey == "" || p.IssueType == "" || strings.TrimSpace(p.Summary) == "" {
		fail(w, http.StatusBadRequest, "project_issue_type_and_summary_required")
		return
	}
	c, cfg, ok := s.client(w)
	if !ok {
		return
	}
	// An issue filed outside the mirrored projects would never come back from the
	// re-read, so refuse it here rather than answering with a stale-mirror error.
	if !contains(cfg.Projects, p.ProjectKey) {
		fail(w, http.StatusBadRequest, "project_not_mirrored")
		return
	}
	fields := map[string]any{
		"project":   map[string]string{"key": p.ProjectKey},
		"issuetype": map[string]string{"id": p.IssueType},
		"summary":   p.Summary,
	}
	if strings.TrimSpace(p.DescriptionText) != "" {
		fields["description"] = jira.Doc(p.DescriptionText, nil)
	}
	if id := deref(p.AssigneeAccountID); id != "" {
		fields["assignee"] = map[string]string{"accountId": id}
	}
	if p.Priority != "" {
		fields["priority"] = map[string]string{"name": p.Priority}
	}
	if len(p.Labels) > 0 {
		fields["labels"] = p.Labels
	}

	key, err := c.CreateIssue(r.Context(), fields)
	if err != nil {
		failJira(w, r, err)
		return
	}
	if err := sync.SyncIssue(r.Context(), cfg, s.db, key, sync.Options{Client: c}); err != nil {
		log.Printf("server: mirror refresh after creating %s: %v", key, err)
		fail(w, http.StatusBadGateway, "write_applied_mirror_stale")
		return
	}
	s.respondIssue(w, r, key, nil)
}

/* ── metadata ── */

func (s *server) handleCreateMeta(w http.ResponseWriter, r *http.Request) {
	c, cfg, ok := s.client(w)
	if !ok {
		return
	}
	projects, err := c.CreateMeta(r.Context(), cfg.Projects)
	if err != nil {
		failJira(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": createMeta(projects)})
}

func createMeta(projects []jira.CreateMetaProject) []map[string]any {
	out := make([]map[string]any, 0, len(projects))
	for _, p := range projects {
		types := make([]map[string]string, 0, len(p.IssueTypes))
		for _, t := range p.IssueTypes {
			types = append(types, map[string]string{"id": t.ID, "name": t.Name})
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
	projects, err := jira.New(cfg.Site, cfg.Email, cfg.Token).CreateMeta(r.Context(), cfg.Projects)
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

func (s *server) handleUsers(w http.ResponseWriter, r *http.Request) {
	c, _, ok := s.client(w)
	if !ok {
		return
	}
	users, err := c.SearchUsers(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		failJira(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, map[string]any{
			"account_id": u.AccountID, "display_name": u.DisplayName, "email": u.Email,
			"avatar_url": u.Avatar(), "active": u.Active,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}
