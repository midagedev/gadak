package origin

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

// Writer is the write surface of an origin: the verbs every origin implements
// and the server and CLI call when writing through. Jira (connected) and
// standalone (issuetap speaks the Jira API) both satisfy it via *jira.Client;
// a Linear adapter implements the same verbs over GraphQL (GDK-358).
//
// The vocabulary is deliberately Jira-shaped — including the types — because
// two of the three origins already speak it natively. Capability negotiation
// stays EditMeta/CreateMeta: an origin that cannot edit a field omits it
// there, and the existing UI already turns fields on and off from that
// answer. An unsupported verb returns an honest error, never a silent no-op.
//
// Optional faces (VersionCatalog, IssueLinker, CreateFieldCatalog, MediaRef)
// are not part of Writer. Callers type-assert via As*; a missing face
// returns the matching ErrNo* string, never a silent no-op (GDK-641).
type Writer interface {
	CreateMeta(ctx context.Context, projects []string) ([]jira.CreateMetaProject, error)
	CreateIssue(ctx context.Context, fields map[string]any) (string, error)
	EditMeta(ctx context.Context, key string) (map[string]jira.FieldMeta, error)
	UpdateFields(ctx context.Context, key string, fields map[string]any) error
	EditIssue(ctx context.Context, key string, fields, update map[string]any) error
	Transitions(ctx context.Context, key string) ([]jira.Transition, error)
	Transition(ctx context.Context, key, transitionID string, fields map[string]any, comment json.RawMessage) error
	AddComment(ctx context.Context, key string, adf json.RawMessage, visibility *jira.CommentVisibility, internal bool) (jira.Comment, error)
	SetAssignee(ctx context.Context, key, accountID string) error
	SearchUsers(ctx context.Context, query string) ([]jira.User, error)
	PriorityCatalog(ctx context.Context) ([]jira.NamedID, error)
	Upload(ctx context.Context, key, filename string, file io.Reader) ([]jira.Attachment, error)
}

// VersionCatalog is GET /rest/api/3/project/{key}/versions. Linear has no
// counterpart (GDK-516): AsVersionCatalog returns ErrNoVersionCatalog.
type VersionCatalog interface {
	ProjectVersions(ctx context.Context, projectKey string) ([]jira.Version, error)
}

// IssueLinker is GET /rest/api/3/issueLinkType plus POST /rest/api/3/issueLink.
// Names and inward/outward descriptions can be renamed; writes send the id.
// Linear has relations but no counterpart in this adapter yet (GDK-19):
// AsIssueLinker returns ErrNoIssueLinks.
type IssueLinker interface {
	IssueLinkTypes(ctx context.Context) ([]jira.IssueLinkType, error)
	LinkIssues(ctx context.Context, typeID, outwardKey, inwardKey string) error
}

// CreateFieldCatalog is what this project+type requires and accepts at create
// time — the create-side sibling of EditMeta (GDK-254). An origin that
// cannot answer is missing the face; callers degrade, they do not block.
// Linear: AsCreateFieldCatalog returns ErrNoCreateFields.
type CreateFieldCatalog interface {
	CreateFields(ctx context.Context, projectIDOrKey, issueTypeID string) ([]jira.CreateFieldMeta, error)
}

// MediaRef resolves an attachment id to the media UUID Jira needs in an ADF
// comment node plus the filename. Linear has no counterpart; AsMediaRef
// returns ErrNoMediaRef and the comment path already degrades on that error.
type MediaRef interface {
	MediaRef(ctx context.Context, attachmentID string) (mediaID, filename string, err error)
}

// These strings were the linearWriter stubs' Error() values. Callers that
// type-assert a missing face must return the same text (GDK-641).
var (
	ErrNoVersionCatalog = errors.New("linear: project versions are not supported on this origin")
	ErrNoIssueLinks     = errors.New("linear: issue links are not supported on this origin")
	ErrNoCreateFields   = errors.New("linear: create-time field metadata is not supported on this origin")
	ErrNoMediaRef       = errors.New("linear: inline comment media is not supported; the file is attached to the issue")
)

// AsVersionCatalog returns w as VersionCatalog, or ErrNoVersionCatalog.
func AsVersionCatalog(w Writer) (VersionCatalog, error) {
	v, ok := w.(VersionCatalog)
	if !ok {
		return nil, ErrNoVersionCatalog
	}
	return v, nil
}

// AsIssueLinker returns w as IssueLinker, or ErrNoIssueLinks.
func AsIssueLinker(w Writer) (IssueLinker, error) {
	v, ok := w.(IssueLinker)
	if !ok {
		return nil, ErrNoIssueLinks
	}
	return v, nil
}

// AsCreateFieldCatalog returns w as CreateFieldCatalog, or ErrNoCreateFields.
func AsCreateFieldCatalog(w Writer) (CreateFieldCatalog, error) {
	v, ok := w.(CreateFieldCatalog)
	if !ok {
		return nil, ErrNoCreateFields
	}
	return v, nil
}

// AsMediaRef returns w as MediaRef, or ErrNoMediaRef.
func AsMediaRef(w Writer) (MediaRef, error) {
	v, ok := w.(MediaRef)
	if !ok {
		return nil, ErrNoMediaRef
	}
	return v, nil
}

// *jira.Client is the Writer for connected and standalone workspaces, and
// it implements every optional face.
var _ Writer = (*jira.Client)(nil)
var _ VersionCatalog = (*jira.Client)(nil)
var _ IssueLinker = (*jira.Client)(nil)
var _ CreateFieldCatalog = (*jira.Client)(nil)
var _ MediaRef = (*jira.Client)(nil)

// WriterFor picks the write path for one issue's source — the caller reads
// it from the mirror (store.KeySource), because a key's shape cannot tell a
// Linear "MID-5" from a Jira "MID-5". Jira and standalone rows share the
// Jira client; "linear" routes to the GraphQL adapter (GDK-361). An empty
// source (a key the mirror does not know yet, or a create) routes to the
// default origin.
func WriterFor(cfg *config.Config, source string) (Writer, error) {
	if source == "linear" {
		return newLinearWriter(cfg)
	}
	return Client(cfg)
}
