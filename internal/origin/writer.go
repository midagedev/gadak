package origin

import (
	"context"
	"encoding/json"
	"io"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

// Writer is the write surface of an origin: exactly the verbs the server
// and CLI call when writing through. Jira (connected) and standalone
// (issuetap speaks the Jira API) both satisfy it via *jira.Client; a
// Linear adapter implements the same verbs over GraphQL (GDK-358).
//
// The vocabulary is deliberately Jira-shaped — including the types — because
// two of the three origins already speak it natively. Capability negotiation
// stays EditMeta/CreateMeta: an origin that cannot edit a field omits it
// there, and the existing UI already turns fields on and off from that
// answer. An unsupported verb returns an honest error, never a silent no-op.
type Writer interface {
	CreateMeta(ctx context.Context, projects []string) ([]jira.CreateMetaProject, error)
	CreateIssue(ctx context.Context, fields map[string]any) (string, error)
	EditMeta(ctx context.Context, key string) (map[string]jira.FieldMeta, error)
	UpdateFields(ctx context.Context, key string, fields map[string]any) error
	Transitions(ctx context.Context, key string) ([]jira.Transition, error)
	Transition(ctx context.Context, key, transitionID string) error
	AddComment(ctx context.Context, key string, adf json.RawMessage) (jira.Comment, error)
	SetAssignee(ctx context.Context, key, accountID string) error
	SearchUsers(ctx context.Context, query string) ([]jira.User, error)
	PriorityCatalog(ctx context.Context) ([]jira.NamedID, error)
	Upload(ctx context.Context, key, filename string, file io.Reader) ([]jira.Attachment, error)
	MediaRef(ctx context.Context, attachmentID string) (mediaID, filename string, err error)
}

// *jira.Client is the Writer for connected and standalone workspaces.
var _ Writer = (*jira.Client)(nil)

// WriterFor picks the write path for one issue key. Today every key routes
// to the Jira client (standalone included); the Linear adapter slots in
// here, keyed by the mirror's source for that key (GDK-361). An empty key
// means "no issue yet" (create paths) and routes to the default origin.
func WriterFor(cfg *config.Config, key string) (Writer, error) {
	_ = key
	return Client(cfg)
}
