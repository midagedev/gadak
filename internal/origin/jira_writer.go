package origin

import (
	"context"
	"encoding/json"
	"io"

	"github.com/midagedev/gadak/internal/jira"
)

// jiraWriter is the Jira/local-origin Writer. It wraps *jira.Client so a
// caller cannot type-assert the HTTP client out of the interface (the
// hole cmd/gadak/create.go used to punch — GDK-665). Optional faces
// (VersionCatalog, IssueLinker, CreateFieldCatalog, MediaRef) and
// IssueStatus promote from the embedded client.
type jiraWriter struct {
	*jira.Client
}

func newJiraWriter(c *jira.Client) *jiraWriter {
	return &jiraWriter{Client: c}
}

// jiraClient hands the embedded client to the capability tests in
// writer.go (which transport is this origin talking over?). Unexported —
// it is a within-package accessor, not a face callers may reach for.
func (w *jiraWriter) jiraClient() *jira.Client { return w.Client }

func (w *jiraWriter) CreateMeta(ctx context.Context, projects []string) ([]CreateMetaProject, error) {
	raw, err := w.Client.CreateMeta(ctx, projects)
	if err != nil {
		return nil, err
	}
	return createMetaFromJira(raw), nil
}

func (w *jiraWriter) EditMeta(ctx context.Context, key string) (map[string]FieldMeta, error) {
	raw, err := w.Client.EditMeta(ctx, key)
	if err != nil {
		return nil, err
	}
	return fieldMetaFromJira(raw), nil
}

func (w *jiraWriter) Transitions(ctx context.Context, key string) ([]Transition, error) {
	raw, err := w.Client.Transitions(ctx, key)
	if err != nil {
		return nil, err
	}
	return transitionsFromJira(raw), nil
}

func (w *jiraWriter) AddComment(ctx context.Context, key string, adf json.RawMessage, visibility *CommentVisibility, internal bool) (Comment, error) {
	raw, err := w.Client.AddComment(ctx, key, adf, visibility, internal)
	if err != nil {
		return Comment{}, err
	}
	return commentFromJira(raw), nil
}

func (w *jiraWriter) SearchUsers(ctx context.Context, query string) ([]User, error) {
	raw, err := w.Client.SearchUsers(ctx, query)
	if err != nil {
		return nil, err
	}
	return usersFromJira(raw), nil
}

func (w *jiraWriter) PriorityCatalog(ctx context.Context) ([]NamedID, error) {
	raw, err := w.Client.PriorityCatalog(ctx)
	if err != nil {
		return nil, err
	}
	return namedIDsFromJira(raw), nil
}

func (w *jiraWriter) Upload(ctx context.Context, key, filename string, file io.Reader) ([]Attachment, error) {
	raw, err := w.Client.Upload(ctx, key, filename, file)
	if err != nil {
		return nil, err
	}
	return attachmentsFromJira(raw), nil
}

// The FromJira names are the adapter frame: a stack that stops here is the
// Jira origin failing to produce that DTO, not the Linear one.

func createMetaFromJira(in []jira.CreateMetaProject) []CreateMetaProject {
	return in
}

func fieldMetaFromJira(in map[string]jira.FieldMeta) map[string]FieldMeta {
	return in
}

func transitionsFromJira(in []jira.Transition) []Transition {
	return in
}

func commentFromJira(in jira.Comment) Comment {
	return in
}

func usersFromJira(in []jira.User) []User {
	return in
}

func namedIDsFromJira(in []jira.NamedID) []NamedID {
	return in
}

func attachmentsFromJira(in []jira.Attachment) []Attachment {
	return in
}

var _ Writer = (*jiraWriter)(nil)
var _ VersionCatalog = (*jiraWriter)(nil)
var _ IssueLinker = (*jiraWriter)(nil)
var _ CreateFieldCatalog = (*jiraWriter)(nil)
var _ MediaRef = (*jiraWriter)(nil)
