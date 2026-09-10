package origin

import (
	"context"

	"github.com/midagedev/gadak/internal/jira"
)

// jiraWriter is the Jira/built-in Writer. It wraps *jira.Client so a
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

// The Writer DTO methods (CreateMeta, EditMeta, Transitions, AddComment,
// SearchUsers, PriorityCatalog, Upload) are the embedded client's own:
// dto.go aliases the jira payload types, so the promoted methods already
// carry the Writer signatures. There is no conversion frame to name — an
// identity function cannot fail, so it never appeared in a stack trace
// anyway (GDK-689). Sprint is the one real conversion, so it stays a method.

var _ Writer = (*jiraWriter)(nil)
var _ VersionCatalog = (*jiraWriter)(nil)
var _ IssueLinker = (*jiraWriter)(nil)
var _ CreateFieldCatalog = (*jiraWriter)(nil)
var _ MediaRef = (*jiraWriter)(nil)
var _ CommentEditor = (*jiraWriter)(nil)

func (w *jiraWriter) MoveToSprint(ctx context.Context, sprintID int64, keys []string) error {
	return w.Client.MoveToSprint(ctx, sprintID, keys)
}

func (w *jiraWriter) MoveToBacklog(ctx context.Context, keys []string) error {
	return w.Client.MoveToBacklog(ctx, keys)
}

func (w *jiraWriter) CreateSprint(ctx context.Context, boardID int64, name, goal string) (Sprint, error) {
	s, err := w.Client.CreateSprint(ctx, boardID, name, goal)
	return Sprint{ID: s.ID, Name: s.Name, State: s.State}, err
}

func (w *jiraWriter) UpdateSprint(ctx context.Context, sprintID int64, fields map[string]any) (Sprint, error) {
	s, err := w.Client.UpdateSprint(ctx, sprintID, fields)
	return Sprint{ID: s.ID, Name: s.Name, State: s.State}, err
}
