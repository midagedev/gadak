package origin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/statuscat"
)

// linearWriter adapts the origin.Writer verbs onto Linear's GraphQL
// mutations (GDK-361). It produces origin DTOs (GDK-665), not jira types.
// The mapping discipline is ids only: stateId/assigneeId are UUIDs,
// priority is Linear's 0-4 scale carried in NamedID.ID. Capability
// negotiation stays EditMeta/CreateMeta — what this origin cannot edit is
// simply absent there. Optional faces (VersionCatalog, IssueLinker,
// CreateFieldCatalog, MediaRef) are not implemented; callers type-assert
// and surface the matching ErrNo* (GDK-641).
type linearWriter struct {
	c *linear.Client
}

// linearPriorityNames indexes Linear's fixed 0-4 scale. Index = wire value.
var linearPriorityNames = []string{"No priority", "Urgent", "High", "Medium", "Low"}

func newLinearWriter(cfg *config.Config) (*linearWriter, error) {
	c, err := Linear(cfg)
	if err != nil {
		return nil, err
	}
	return &linearWriter{c: c}, nil
}

// resolve turns a user-typed key ("MID-5") into the issue, which carries the
// UUID every mutation wants plus the team the transition catalog needs.
func (w *linearWriter) resolve(ctx context.Context, key string) (linear.Issue, error) {
	return w.c.Issue(ctx, key)
}

func (w *linearWriter) PriorityCatalog(ctx context.Context) ([]NamedID, error) {
	return namedIDsFromLinearPriorities(), nil
}

func (w *linearWriter) Transitions(ctx context.Context, key string) ([]Transition, error) {
	iss, err := w.resolve(ctx, key)
	if err != nil {
		return nil, err
	}
	states, err := w.c.WorkflowStates(ctx, iss.Team.ID)
	if err != nil {
		return nil, err
	}
	out := make([]Transition, 0, len(states))
	for _, s := range states {
		if s.ID == iss.State.ID {
			continue // Linear has no self-transition; offering one is noise
		}
		out = append(out, transitionFromLinearState(s))
	}
	return out, nil
}

func (w *linearWriter) Transition(ctx context.Context, key, transitionID string, fields map[string]any, comment json.RawMessage) error {
	if len(fields) > 0 || len(comment) > 0 {
		return unsupported("linear transitions do not carry screen fields")
	}
	iss, err := w.resolve(ctx, key)
	if err != nil {
		return err
	}
	_, err = w.c.UpdateIssue(ctx, iss.ID, linear.IssueUpdate{StateID: &transitionID})
	return err
}

func (w *linearWriter) AddComment(ctx context.Context, key string, body json.RawMessage, visibility *CommentVisibility, internal bool) (Comment, error) {
	if visibility != nil || internal {
		return Comment{}, unsupported("linear comments do not support visibility or internal")
	}
	iss, err := w.resolve(ctx, key)
	if err != nil {
		return Comment{}, err
	}
	// The server builds ADF from the user's markdown; Linear speaks
	// markdown, so the document is serialized back (adf.Markdown — the
	// identity on the markdown subset, GDK-1386). Mentions and inline media
	// degrade to their text — the files are attached to the issue either way.
	cm, err := w.c.CreateComment(ctx, iss.ID, adf.Markdown(body))
	if err != nil {
		return Comment{}, err
	}
	return commentFromLinear(cm, body), nil
}

func (w *linearWriter) SetAssignee(ctx context.Context, key, accountID string) error {
	iss, err := w.resolve(ctx, key)
	if err != nil {
		return err
	}
	// Empty accountID unassigns — the explicit-null case IssueUpdate encodes.
	_, err = w.c.UpdateIssue(ctx, iss.ID, linear.IssueUpdate{AssigneeID: &accountID})
	return err
}

func (w *linearWriter) SearchUsers(ctx context.Context, query string) ([]User, error) {
	users, err := w.c.Users(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(users))
	for _, u := range users {
		out = append(out, userFromLinear(u))
	}
	return out, nil
}

// editableFields is the EditMeta answer for every Linear issue: the fields
// the adapter can write. Linear has no per-issue edit permissions the API
// exposes, so the catalog is static. Labels are absent deliberately — the
// Writer vocabulary carries label names, Linear wants label UUIDs, and a
// name→id mapping is a separate decision (GDK-362 note), not a silent guess.
func (w *linearWriter) EditMeta(ctx context.Context, key string) (map[string]FieldMeta, error) {
	if _, err := w.resolve(ctx, key); err != nil {
		return nil, err
	}
	return fieldMetaFromLinearEditable(), nil
}

func (w *linearWriter) UpdateFields(ctx context.Context, key string, fields map[string]any) error {
	iss, err := w.resolve(ctx, key)
	if err != nil {
		return err
	}
	var upd linear.IssueUpdate
	for name, v := range fields {
		switch name {
		case "summary":
			s, err := stringField("summary", v)
			if err != nil {
				return err
			}
			upd.Title = &s
		case "description":
			text := ""
			if v != nil {
				raw, err := json.Marshal(v)
				if err != nil {
					return err
				}
				text = adf.Markdown(raw)
			}
			upd.Description = &text
		case "priority":
			p := 0 // clearing a priority is Linear's 0, "No priority"
			if v != nil {
				n, err := priorityID(v)
				if err != nil {
					return err
				}
				p = n
			}
			upd.Priority = &p
		case "duedate":
			if v == nil {
				return unsupported("linear: clearing a due date is not supported yet")
			}
			s, err := stringField("duedate", v)
			if err != nil {
				return err
			}
			upd.DueDate = &s
		case "assignee":
			// SetAssignee interpretation, verbatim: an accountId (a user
			// UUID). Nil or an empty accountId unassigns — the explicit
			// null IssueUpdate encodes for AssigneeID.
			id, err := accountIDField("assignee", v)
			if err != nil {
				return err
			}
			upd.AssigneeID = &id
		case "issuetype":
			return ErrNoIssueTypes
		default:
			return unsupportedf("linear: field %q is not editable on this origin", name)
		}
	}
	_, err = w.c.UpdateIssue(ctx, iss.ID, upd)
	return err
}

// EditIssue is Jira's two-part edit (fields + update ops). The fields half
// maps like UpdateFields; the update half is Jira operation syntax (label
// add/remove) with no Linear counterpart yet — refused honestly rather than
// half-applied.
func (w *linearWriter) EditIssue(ctx context.Context, key string, fields, update map[string]any) error {
	if len(update) > 0 {
		return unsupported("linear: label add/remove operations are not supported on this origin yet")
	}
	return w.UpdateFields(ctx, key, fields)
}

func (w *linearWriter) CreateMeta(ctx context.Context, projects []string) ([]CreateMetaProject, error) {
	teams, err := w.c.Teams(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CreateMetaProject, 0, len(teams))
	for _, t := range teams {
		if len(projects) > 0 && !slices.Contains(projects, t.Key) {
			continue
		}
		out = append(out, createMetaFromLinearTeam(t))
	}
	return out, nil
}

// stringField rejects a non-string so a wrong-typed any cannot become the
// empty string Linear would store (GDK-643).
func stringField(name string, v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("linear: field %q wants string, got %T", name, v)
	}
	return s, nil
}

// priorityID reads the Writer-shaped priority value (create.PriorityField
// is map[string]string{"id": …}; JSON-decoded maps are map[string]any).
func priorityID(v any) (int, error) {
	id := ""
	switch pv := v.(type) {
	case map[string]string:
		id = pv["id"]
	case map[string]any:
		if raw := pv["id"]; raw != nil {
			s, err := stringField("priority", raw)
			if err != nil {
				return 0, err
			}
			id = s
		}
	default:
		return 0, fmt.Errorf("linear: field %q wants string, got %T", "priority", v)
	}
	n, err := strconv.Atoi(id)
	if err != nil || n < 0 || n > 4 {
		return 0, fmt.Errorf("linear: priority id %q is not on the 0-4 scale", id)
	}
	return n, nil
}

// accountIDField reads the Writer-shaped user value down to the accountId
// SetAssignee takes: the accountId object both real callers build
// (fields.ValueFromIDs "user" is map[string]string; a JSON-decoded copy is
// map[string]any) or nil, the editor clear. Nil and an empty accountId are
// the unassign SetAssignee encodes with an empty string. Every other shape
// is refused before the wire — reading one as the empty accountId would
// silently unassign the issue (the GDK-643 wrong-typed-any rule, with
// higher stakes than an empty string).
func accountIDField(name string, v any) (string, error) {
	id := ""
	switch av := v.(type) {
	case nil:
	case map[string]string:
		raw, ok := av["accountId"]
		if !ok {
			return "", fmt.Errorf("linear: field %q wants an accountId, got a user object without one", name)
		}
		id = raw
	case map[string]any:
		raw, ok := av["accountId"]
		if !ok {
			return "", fmt.Errorf("linear: field %q wants an accountId, got a user object without one", name)
		}
		if raw != nil {
			s, err := stringField(name, raw)
			if err != nil {
				return "", err
			}
			id = s
		}
	default:
		return "", fmt.Errorf("linear: field %q wants an accountId object, got %T", name, v)
	}
	return id, nil
}

func (w *linearWriter) CreateIssue(ctx context.Context, fields map[string]any) (string, error) {
	for _, name := range []string{"assignee", "labels", "parent", "issuetype"} {
		if _, ok := fields[name]; ok {
			return "", unsupportedf("linear: field %q is not supported on create", name)
		}
	}
	teamKey := ""
	if p, ok := fields["project"].(map[string]any); ok {
		if raw, has := p["key"]; has && raw != nil {
			s, err := stringField("project.key", raw)
			if err != nil {
				return "", err
			}
			teamKey = s
		}
	}
	if teamKey == "" {
		return "", fmt.Errorf("linear: create needs project.key (the team key)")
	}
	title, err := stringField("summary", fields["summary"])
	if err != nil {
		return "", err
	}
	in := linear.IssueCreate{Title: title}
	if d, ok := fields["description"]; ok && d != nil {
		raw, err := json.Marshal(d)
		if err != nil {
			return "", err
		}
		in.Description = adf.Markdown(raw)
	}
	if p, ok := fields["priority"]; ok && p != nil {
		n, err := priorityID(p)
		if err != nil {
			return "", err
		}
		in.Priority = &n
	}
	if d, ok := fields["duedate"]; ok && d != nil {
		s, err := stringField("duedate", d)
		if err != nil {
			return "", err
		}
		in.DueDate = s
	}
	teams, err := w.c.Teams(ctx)
	if err != nil {
		return "", err
	}
	teamID := ""
	for _, t := range teams {
		if t.Key == teamKey {
			teamID = t.ID
			break
		}
	}
	if teamID == "" {
		return "", fmt.Errorf("linear: no team with key %q", teamKey)
	}
	in.TeamID = teamID
	iss, err := w.c.CreateIssue(ctx, in)
	if err != nil {
		return "", err
	}
	return iss.Identifier, nil
}

func (w *linearWriter) Upload(ctx context.Context, key, filename string, file io.Reader) ([]Attachment, error) {
	iss, err := w.resolve(ctx, key)
	if err != nil {
		return nil, err
	}
	// Linear attachments are URL-first: reserve storage, PUT the bytes
	// (headers verbatim — the signed URL rejects the write without them;
	// measured live 2026-08-20), then bind the assetUrl to the issue. The
	// PUT carries no API key: it goes to the storage host, not the API.
	body, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	ct := http.DetectContentType(body)
	target, err := w.c.UploadFile(ctx, filename, ct, len(body))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target.UploadURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", ct)
	for k, v := range target.Headers {
		req.Header.Set(k, v)
	}
	res, err := w.c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("linear: storage upload for %s answered %d", filename, res.StatusCode)
	}
	att, err := w.c.CreateAttachment(ctx, iss.ID, target.AssetURL, filename)
	if err != nil {
		return nil, err
	}
	return []Attachment{attachmentFromLinear(att.ID, filename, ct, int64(len(body)))}, nil
}

// Linear→DTO constructors. The names are the adapter frame: a stack that
// stops here is Linear failing to produce that DTO, not the Jira origin.

func namedIDsFromLinearPriorities() []NamedID {
	out := make([]NamedID, 0, len(linearPriorityNames))
	// Urgent..Low first, "No priority" last — pickers list most urgent first,
	// the same order the mirror's priority_rank encodes.
	for p := 1; p <= 4; p++ {
		out = append(out, NamedID{ID: strconv.Itoa(p), Name: linearPriorityNames[p]})
	}
	return append(out, NamedID{ID: "0", Name: linearPriorityNames[0]})
}

func transitionFromLinearState(s linear.WorkflowState) Transition {
	cat, _ := linear.StatusCategory(s.Type)
	t := Transition{ID: s.ID, Name: s.Name}
	t.To.ID = s.ID
	t.To.Name = s.Name
	// Transition.To carries the Jira REST statusCategory key (new /
	// indeterminate / done) because that is what jira.Client unmarshals
	// and handleTransitions already runs through statuscat.Category.
	t.To.StatusCategory.Key = statuscat.CategoryKey(cat)
	return t
}

func commentFromLinear(cm linear.Comment, body json.RawMessage) Comment {
	out := Comment{ID: cm.ID, Created: cm.CreatedAt}
	out.Body = body
	return out
}

func userFromLinear(u linear.User) User {
	name := u.DisplayName
	if name == "" {
		name = u.Name
	}
	return User{AccountID: u.ID, DisplayName: name, Email: u.Email, Active: true}
}

func fieldMetaFromLinearEditable() map[string]FieldMeta {
	set := func(typ string) FieldMeta {
		var m FieldMeta
		m.Operations = []string{"set"}
		m.Schema.Type = typ
		return m
	}
	return map[string]FieldMeta{
		"summary":     set("string"),
		"description": set("string"),
		"priority":    set("priority"),
		"duedate":     set("date"),
		"assignee":    set("user"),
	}
}

func createMetaFromLinearTeam(t linear.Team) CreateMetaProject {
	return CreateMetaProject{
		Key:  t.Key,
		Name: t.Name,
		// Linear issues have no type; one entry keeps every picker
		// rendering without a special case.
		IssueTypes: []CreateMetaIssueType{{ID: "issue", Name: "Issue"}},
	}
}

func attachmentFromLinear(id, filename, mime string, size int64) Attachment {
	return Attachment{ID: id, Filename: filename, MimeType: mime, Size: size}
}

var _ Writer = (*linearWriter)(nil)
