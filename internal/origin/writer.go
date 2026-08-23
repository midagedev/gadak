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
// standalone (issuetap speaks the Jira API) satisfy it via jiraWriter
// wrapping *jira.Client; a Linear adapter implements the same verbs over
// GraphQL (GDK-358). The method vocabulary is Jira-shaped (EditMeta,
// Transitions) so existing callers keep working; the types are origin DTOs
// (GDK-665), not internal/jira names.
//
// Capability negotiation stays EditMeta/CreateMeta: an origin that cannot
// edit a field omits it there, and the existing UI already turns fields on
// and off from that answer. An unsupported verb returns an honest error,
// never a silent no-op.
//
// Optional faces (VersionCatalog, IssueLinker, CreateFieldCatalog, MediaRef)
// are not part of Writer. Callers type-assert via As*; a missing face
// returns the matching ErrNo* string, never a silent no-op (GDK-641).
type Writer interface {
	CreateMeta(ctx context.Context, projects []string) ([]CreateMetaProject, error)
	CreateIssue(ctx context.Context, fields map[string]any) (string, error)
	EditMeta(ctx context.Context, key string) (map[string]FieldMeta, error)
	UpdateFields(ctx context.Context, key string, fields map[string]any) error
	EditIssue(ctx context.Context, key string, fields, update map[string]any) error
	Transitions(ctx context.Context, key string) ([]Transition, error)
	Transition(ctx context.Context, key, transitionID string, fields map[string]any, comment json.RawMessage) error
	AddComment(ctx context.Context, key string, adf json.RawMessage, visibility *CommentVisibility, internal bool) (Comment, error)
	SetAssignee(ctx context.Context, key, accountID string) error
	SearchUsers(ctx context.Context, query string) ([]User, error)
	PriorityCatalog(ctx context.Context) ([]NamedID, error)
	Upload(ctx context.Context, key, filename string, file io.Reader) ([]Attachment, error)
}

// VersionCatalog is GET /rest/api/3/project/{key}/versions. Linear has no
// counterpart (GDK-516): AsVersionCatalog returns ErrNoVersionCatalog.
//
// CreatesVersionsByName is the issuetap mint-by-name capability (GDK-678):
// a fixVersions add {"name": token} creates the version when it is missing
// from the catalog. Cloud Jira is false — unknown names 400, and creating a
// version is a separate project-admin permission. Same shape as
// sync.OSNotifier.Supported: a boolean on the face that already owns the
// verb, not a workspace-kind string.
type VersionCatalog interface {
	ProjectVersions(ctx context.Context, projectKey string) ([]jira.Version, error)
	CreatesVersionsByName() bool
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

// CreatesVersionsByName reports whether w mints a project version from a
// fixVersions add {"name": token}. True for issuetap (standalone in-process,
// routed serve, paired home). False for Cloud Jira and Linear (GDK-678).
func CreatesVersionsByName(w Writer) bool {
	vc, err := AsVersionCatalog(w)
	if err != nil {
		return false
	}
	return vc.CreatesVersionsByName()
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

// *jira.Client still matches Writer's method set (the DTO names alias the
// HTTP payload types) and implements every optional face. WriterFor wraps
// it in jiraWriter so callers cannot punch through to the HTTP client.
var _ Writer = (*jira.Client)(nil)
var _ VersionCatalog = (*jira.Client)(nil)
var _ IssueLinker = (*jira.Client)(nil)
var _ CreateFieldCatalog = (*jira.Client)(nil)
var _ MediaRef = (*jira.Client)(nil)

// WriterFor picks the write path for one issue's source — the caller reads
// it from the mirror (store.KeySource), because a key's shape cannot tell a
// Linear "MID-5" from a Jira "MID-5". Jira and standalone rows share the
// Jira client (wrapped); "linear" routes to the GraphQL adapter (GDK-361).
// An empty source (a key the mirror does not know yet, or a create) routes
// to the default origin.
func WriterFor(cfg *config.Config, source string) (Writer, error) {
	if source == "linear" {
		return newLinearWriter(cfg)
	}
	c, err := Client(cfg)
	if err != nil {
		return nil, err
	}
	return newJiraWriter(c), nil
}
