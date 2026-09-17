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
	"github.com/midagedev/gadak/internal/create"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/jirafields"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/parenthint"
	"github.com/midagedev/gadak/internal/statuscat"
	"github.com/midagedev/gadak/internal/sync"
	"github.com/midagedev/gadak/internal/transition"
)

// Issue-field writes: transitions, comments, attachment upload, and the
// field-level editors (priority, duedate, parent, summary, description,
// labels, assignee, editmeta, custom fields) — everything that mutates one
// existing issue. Split from write.go (GDK-1922); the write-through shape
// itself (mutate, keyWriter) stays in write.go.

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
			Key: key,
			// The body field is named transition_id and every real caller (web,
			// phone, MIRROR.md curl) sends the id of an object out of GET
			// transitions/ — the machine path, not a human's identifier (GDK-1982).
			TransitionID: body.TransitionID,
			Resolution:   body.Resolution,
			Fields:       body.Fields,
			Comment:      body.Comment,
			// The mirror tiebreak the CLI write already carries (GDK-1521):
			// two same-named destinations fold, and this surface must pick
			// the one the project actually uses, not payload order.
			StatusUse: transition.MirrorStatusUse(ctx, s.db, key),
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"changed": res.Changed}, nil
	})
}

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
	if err := origin.RefuseBodyPlaceholders(s.config(), body.Text); err != nil {
		failMsg(w, http.StatusConflict, "placeholder", err.Error())
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
		created, err := c.AddComment(ctx, key,
			origin.BodyValue(s.config(), body.Text, jira.DocWithMedia(body.Text, mentions, media)),
			visibility, body.Internal)
		if err != nil {
			return nil, err
		}
		row := map[string]any{
			"comment_id": created.ID,
			"author":     created.Author.DisplayName,
			"body":       adf.PlainText(created.Body),
			"created_at": jira.ISOTime(created.Created),
		}
		// The restriction echoed the way the origin states it (GDK-528), so
		// the just-posted comment can wear its badge before the detail
		// re-read. Same wire keys the detail response carries; absent when
		// the origin sent none.
		if created.Visibility != nil {
			row["visibility_type"] = created.Visibility.Type
			row["visibility_value"] = created.Visibility.Value
		}
		if created.JsdPublic != nil {
			row["jsd_public"] = *created.JsdPublic
		}
		return map[string]any{"comment": row}, nil
	})
}

// maxUpload caps an attachment. Jira's own default is 10 MB; this is the memory
// this process is willing to hold either way.
const maxUpload = 64 << 20

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
		// The same artifact verdict the detail will carry, so the
		// just-uploaded HTML page can render before any re-read.
		artifactURLFor := ""
		if isHTMLMediaType(a.MimeType) {
			artifactURLFor = artifactURL(key, a.ID)
		}
		out = append(out, map[string]any{
			"id": a.ID, "filename": a.Filename, "mime_type": a.MimeType, "size": a.Size,
			"media_id":     "",
			"is_image":     strings.HasPrefix(a.MimeType, "image/"),
			"is_video":     strings.HasPrefix(a.MimeType, "video/"),
			"is_artifact":  isHTMLMediaType(a.MimeType),
			"content_url":  attachmentURL(key, a.ID),
			"artifact_url": artifactURLFor,
		})
	}
	label := writeOriginLabel(src)
	log.Printf("server: write %s origin=%s", key, label)
	writeJSON(w, http.StatusOK, map[string]any{"attachments": out, "origin": label})
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

// PUT <key>/description/ — {"description": markdown | null, "force": bool}.
// The markdown becomes ADF; the placeholders the detail's description_md
// carried put the body's preserved nodes back (adf, GDK-1396), read from the
// origin now. A text with no placeholder over a body that has preserved
// nodes is a plain replace: 409 format_loss unless force. A placeholder the
// current body cannot honour (changed since read, unknown, misplaced) is
// 409 placeholder with the reason in `message`. Deleted placeholders delete
// their nodes and the response names them in `dropped`.
func (s *server) handleDescription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Description json.RawMessage `json:"description"`
		Force       bool            `json:"force"`
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
	var doc json.RawMessage
	var dropped []string
	if !clear {
		_, cfg, src, ok := s.keyWriter(w, r, key)
		if !ok {
			return
		}
		var err error
		doc, dropped, err = descriptionFromMarkdown(r.Context(), cfg, src, key, text, body.Force)
		if err != nil {
			var pe *adf.PlaceholderError
			switch {
			case errors.As(err, &pe):
				failMsg(w, http.StatusConflict, "placeholder", pe.Error())
			case errors.Is(err, errFormatLoss):
				fail(w, http.StatusConflict, "format_loss")
			default:
				failJira(w, r, cfg, err)
			}
			return
		}
	}
	s.mutate(w, r, key, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		if clear {
			return nil, c.UpdateFields(ctx, key, map[string]any{"description": nil})
		}
		extra := map[string]any{}
		if len(dropped) > 0 {
			extra["dropped"] = dropped
		}
		return extra, c.UpdateFields(ctx, key, map[string]any{"description": doc})
	})
}

// errFormatLoss: the text has no placeholders and the body has preserved
// nodes — the plain replace GDK-1001 refuses without force.
var errFormatLoss = errors.New("format_loss")

// descriptionFromMarkdown is the server's half of the CLI's function of the
// same name (cmd/gadak/edit.go): the ADF for a markdown description, with the
// current body's preserved nodes back in place. dropped names the ones the
// text no longer references ("panel #1").
func descriptionFromMarkdown(ctx context.Context, cfg *config.Config, src, key, text string, force bool) (json.RawMessage, []string, error) {
	// A Jira Server body is wiki markup carried verbatim (GDK-1637): the
	// save sends the typed characters as a string, and the placeholder
	// machinery below — markers, preserved nodes, the origin read they
	// need — has nothing to do on an origin whose bodies lose nothing.
	if origin.BodyDialect(cfg) == adf.DialectWiki {
		return origin.BodyValue(cfg, text, nil), nil, nil
	}
	if src == "linear" {
		if adf.HasPlaceholders(text) {
			return nil, nil, &adf.PlaceholderError{Msg: "a Linear body has no preserved nodes to put behind a placeholder — drop the markers"}
		}
		return origin.BodyValue(cfg, text, jira.Doc(text, nil)), nil, nil
	}
	cur, found, err := origin.CurrentDescription(ctx, cfg, key)
	if err != nil {
		return nil, nil, err
	}
	if !found || len(adf.Preserved(cur)) == 0 {
		if adf.HasPlaceholders(text) {
			return nil, nil, &adf.PlaceholderError{Msg: "the current description has no preserved nodes — drop the markers"}
		}
		return origin.BodyValue(cfg, text, jira.Doc(text, nil)), nil, nil
	}
	if !adf.HasPlaceholders(text) {
		if !force {
			return nil, nil, errFormatLoss
		}
		return origin.BodyValue(cfg, text, jira.Doc(text, nil)), nil, nil
	}
	doc, kept, err := adf.FromMarkdownWith(text, cur)
	if err != nil {
		return nil, nil, err
	}
	var dropped []string
	for _, k := range kept {
		dropped = append(dropped, fmt.Sprintf("%s #%d", k.Type, k.N))
	}
	return doc, dropped, nil
}

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
