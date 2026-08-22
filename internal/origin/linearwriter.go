package origin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/linear"
)

// linearWriter adapts the Jira-shaped Writer vocabulary onto Linear's
// GraphQL mutations (GDK-361). The mapping discipline is ids only:
// stateId/assigneeId are UUIDs, priority is Linear's 0-4 scale carried in
// NamedID.ID. Capability negotiation stays EditMeta/CreateMeta — what this
// origin cannot edit is simply absent there, and the one verb Linear has no
// counterpart for (MediaRef) returns an honest error the comment path
// already degrades on.
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

func (w *linearWriter) PriorityCatalog(ctx context.Context) ([]jira.NamedID, error) {
	out := make([]jira.NamedID, 0, len(linearPriorityNames))
	// Urgent..Low first, "No priority" last — pickers list most urgent first,
	// the same order the mirror's priority_rank encodes.
	for p := 1; p <= 4; p++ {
		out = append(out, jira.NamedID{ID: strconv.Itoa(p), Name: linearPriorityNames[p]})
	}
	return append(out, jira.NamedID{ID: "0", Name: linearPriorityNames[0]}), nil
}

// ProjectVersions has no Linear counterpart (issues do not carry Jira
// fix versions). Refuse locally so a CLI --fix-version name never looks
// like an empty catalog.
func (w *linearWriter) ProjectVersions(context.Context, string) ([]jira.Version, error) {
	return nil, fmt.Errorf("linear: project versions are not supported on this origin")
}

// IssueLinkTypes has a Linear counterpart (relations) that this adapter
// does not yet map. Refuse locally so a CLI --type token never looks
// like an empty catalog.
func (w *linearWriter) IssueLinkTypes(context.Context) ([]jira.IssueLinkType, error) {
	return nil, fmt.Errorf("linear: issue links are not supported on this origin")
}

// LinkIssues is refused with the same local error as IssueLinkTypes.
func (w *linearWriter) LinkIssues(context.Context, string, string, string) error {
	return fmt.Errorf("linear: issue links are not supported on this origin")
}

func (w *linearWriter) Transitions(ctx context.Context, key string) ([]jira.Transition, error) {
	iss, err := w.resolve(ctx, key)
	if err != nil {
		return nil, err
	}
	states, err := w.c.WorkflowStates(ctx, iss.Team.ID)
	if err != nil {
		return nil, err
	}
	out := make([]jira.Transition, 0, len(states))
	for _, s := range states {
		if s.ID == iss.State.ID {
			continue // Linear has no self-transition; offering one is noise
		}
		t := jira.Transition{ID: s.ID, Name: s.Name}
		t.To.ID = s.ID
		t.To.Name = s.Name
		t.To.StatusCategory.Key = jiraCategoryKey(s.Type)
		out = append(out, t)
	}
	return out, nil
}

// jiraCategoryKey maps a Linear workflow-state type onto Jira's status
// category keys (new/indeterminate/done), which is what the web keys its
// transition buttons on. Unknown types read as new — open, never silently
// done (same collapse rule as the sync's linearCategory).
func jiraCategoryKey(stateType string) string {
	switch stateType {
	case "started":
		return jira.CategoryKey("inprogress")
	case "completed", "canceled", "duplicate":
		return jira.CategoryKey("done")
	default:
		return jira.CategoryKey("new")
	}
}

func (w *linearWriter) Transition(ctx context.Context, key, transitionID string, fields map[string]any, comment json.RawMessage) error {
	if len(fields) > 0 || len(comment) > 0 {
		return fmt.Errorf("linear transitions do not carry screen fields")
	}
	iss, err := w.resolve(ctx, key)
	if err != nil {
		return err
	}
	_, err = w.c.UpdateIssue(ctx, iss.ID, linear.IssueUpdate{StateID: &transitionID})
	return err
}

func (w *linearWriter) AddComment(ctx context.Context, key string, body json.RawMessage, visibility *jira.CommentVisibility, internal bool) (jira.Comment, error) {
	if visibility != nil || internal {
		return jira.Comment{}, fmt.Errorf("linear comments do not support visibility or internal")
	}
	iss, err := w.resolve(ctx, key)
	if err != nil {
		return jira.Comment{}, err
	}
	// The server builds the ADF from user text; Linear speaks markdown, so
	// the text is lifted back out. Mentions and inline media degrade to
	// their text — the files are attached to the issue either way.
	cm, err := w.c.CreateComment(ctx, iss.ID, adf.PlainText(body))
	if err != nil {
		return jira.Comment{}, err
	}
	out := jira.Comment{ID: cm.ID, Created: cm.CreatedAt}
	out.Body = body
	return out, nil
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

func (w *linearWriter) SearchUsers(ctx context.Context, query string) ([]jira.User, error) {
	users, err := w.c.Users(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]jira.User, 0, len(users))
	for _, u := range users {
		name := u.DisplayName
		if name == "" {
			name = u.Name
		}
		out = append(out, jira.User{AccountID: u.ID, DisplayName: name, Email: u.Email, Active: true})
	}
	return out, nil
}

// editableFields is the EditMeta answer for every Linear issue: the fields
// the adapter can write. Linear has no per-issue edit permissions the API
// exposes, so the catalog is static. Labels are absent deliberately — the
// Writer vocabulary carries label names, Linear wants label UUIDs, and a
// name→id mapping is a separate decision (GDK-362 note), not a silent guess.
func (w *linearWriter) EditMeta(ctx context.Context, key string) (map[string]jira.FieldMeta, error) {
	if _, err := w.resolve(ctx, key); err != nil {
		return nil, err
	}
	set := func(typ string) jira.FieldMeta {
		var m jira.FieldMeta
		m.Operations = []string{"set"}
		m.Schema.Type = typ
		return m
	}
	return map[string]jira.FieldMeta{
		"summary":     set("string"),
		"description": set("string"),
		"priority":    set("priority"),
		"duedate":     set("date"),
		"assignee":    set("user"),
	}, nil
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
			s, _ := v.(string)
			upd.Title = &s
		case "description":
			text := ""
			if v != nil {
				raw, err := json.Marshal(v)
				if err != nil {
					return err
				}
				text = adf.PlainText(raw)
			}
			upd.Description = &text
		case "priority":
			p := 0 // clearing a priority is Linear's 0, "No priority"
			if v != nil {
				id := ""
				switch pv := v.(type) {
				case map[string]string:
					id = pv["id"]
				case map[string]any:
					id, _ = pv["id"].(string)
				}
				n, err := strconv.Atoi(id)
				if err != nil || n < 0 || n > 4 {
					return fmt.Errorf("linear: priority id %q is not on the 0-4 scale", id)
				}
				p = n
			}
			upd.Priority = &p
		case "duedate":
			if v == nil {
				return fmt.Errorf("linear: clearing a due date is not supported yet")
			}
			s, _ := v.(string)
			upd.DueDate = &s
		default:
			return fmt.Errorf("linear: field %q is not editable on this origin", name)
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
		return fmt.Errorf("linear: label add/remove operations are not supported on this origin yet")
	}
	return w.UpdateFields(ctx, key, fields)
}

// CreateFields has no Linear counterpart: issue types are a Jira concept
// and Linear does not expose per-team required create fields. Callers
// degrade; this is not a silent empty list.
func (w *linearWriter) CreateFields(context.Context, string, string) ([]jira.CreateFieldMeta, error) {
	return nil, fmt.Errorf("linear: create-time field metadata is not supported on this origin")
}

func (w *linearWriter) CreateMeta(ctx context.Context, projects []string) ([]jira.CreateMetaProject, error) {
	teams, err := w.c.Teams(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]jira.CreateMetaProject, 0, len(teams))
	for _, t := range teams {
		if len(projects) > 0 && !contains(projects, t.Key) {
			continue
		}
		out = append(out, jira.CreateMetaProject{
			Key:  t.Key,
			Name: t.Name,
			// Linear issues have no type; one entry keeps every picker
			// rendering without a special case.
			IssueTypes: []jira.CreateMetaIssueType{{ID: "issue", Name: "Issue"}},
		})
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (w *linearWriter) CreateIssue(ctx context.Context, fields map[string]any) (string, error) {
	for _, name := range []string{"assignee", "labels", "parent", "issuetype"} {
		if _, ok := fields[name]; ok {
			return "", fmt.Errorf("linear: field %q is not supported on create", name)
		}
	}
	teamKey := ""
	if p, ok := fields["project"].(map[string]any); ok {
		teamKey, _ = p["key"].(string)
	}
	if teamKey == "" {
		return "", fmt.Errorf("linear: create needs project.key (the team key)")
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
	in := linear.IssueCreate{TeamID: teamID}
	in.Title, _ = fields["summary"].(string)
	if d, ok := fields["description"]; ok && d != nil {
		raw, err := json.Marshal(d)
		if err != nil {
			return "", err
		}
		in.Description = adf.PlainText(raw)
	}
	if p, ok := fields["priority"]; ok && p != nil {
		id := ""
		switch pv := p.(type) {
		case map[string]string:
			id = pv["id"]
		case map[string]any:
			id, _ = pv["id"].(string)
		}
		n, err := strconv.Atoi(id)
		if err != nil || n < 0 || n > 4 {
			return "", fmt.Errorf("linear: priority id %q is not on the 0-4 scale", id)
		}
		in.Priority = &n
	}
	if d, ok := fields["duedate"].(string); ok && d != "" {
		in.DueDate = d
	}
	iss, err := w.c.CreateIssue(ctx, in)
	if err != nil {
		return "", err
	}
	return iss.Identifier, nil
}

func (w *linearWriter) Upload(ctx context.Context, key, filename string, file io.Reader) ([]jira.Attachment, error) {
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
	return []jira.Attachment{{ID: att.ID, Filename: filename, MimeType: ct, Size: int64(len(body))}}, nil
}

// MediaRef has no Linear counterpart: attachments do not render inline in
// comments by media id. The comment path already degrades on this error —
// the file stays attached to the issue, only the inline embed is skipped.
func (w *linearWriter) MediaRef(ctx context.Context, attachmentID string) (string, string, error) {
	return "", "", fmt.Errorf("linear: inline comment media is not supported; the file is attached to the issue")
}

var _ Writer = (*linearWriter)(nil)
