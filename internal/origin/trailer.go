package origin

// The actor trailer (GDK-586's Jira/Linear half): when a process writes as
// a resolved actor onto an origin that cannot record one — Jira Cloud and
// Linear take the write under a person's credential — the identity travels
// inside the write instead, as one trailing line of the body:
//
//	— via gadak · Claude Code (claude:354bff2b)
//
// The built-in origin (local or paired) already records the actor as the
// write's author through X-Issuetap-Actor (origin.go), so it never sees a
// trailer; a second marker there would be noise. The ledger the backlog
// wants is reconstructible from the mirror alone — comments whose body
// carries the line — which keeps it a cache, not a second original.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/claim"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

// ActorTrailer is the one trailing line an agent-authored write carries on
// a Jira or Linear origin: "— via gadak · <Name> (<slug>)" with a display
// name, "— via gadak · <slug>" without. Empty when the actor has no slug
// (nothing to attribute). Exactly one line, no link, no emoji — the line is
// a machine-written literal, and docs/RECIPES.md filters on it verbatim.
func ActorTrailer(a config.ResolvedActor) string {
	slug := strings.TrimSpace(a.Slug)
	if slug == "" {
		return ""
	}
	if name := strings.TrimSpace(a.Name); name != "" {
		return fmt.Sprintf("— via gadak · %s (%s)", name, slug)
	}
	return "— via gadak · " + slug
}

// WithActorTrailer stamps agent-authored writes with the actor trailer.
// It returns w unchanged when no actor resolves, when actor.trailer is
// explicitly false (config set actor.trailer false — the one switch for the
// trailer and any ledger derived from it), or when the workspace origin is
// the built-in tracker, local or paired, where X-Issuetap-Actor already
// carries the identity. The origin's kind has a single owner,
// (*config.Config).OriginType — never re-derived from the site host.
func WithActorTrailer(w Writer, cfg *config.Config) Writer {
	if w == nil {
		return w
	}
	if cfg != nil && cfg.OriginType() == config.OriginGadak {
		return w
	}
	if !cfg.ActorTrailerEnabled() {
		return w
	}
	actor, ok := config.ResolveActor(cfg)
	if !ok {
		return w
	}
	trailer := ActorTrailer(actor)
	if trailer == "" {
		return w
	}
	// Two shapes, because absence itself is information for two faces that
	// are not the As* family: claim's `c.(claim.Origin)` refusal names the
	// two halves a Linear workspace should use instead, and transition's
	// issueStatusReader probe degrades to "run the transition" only when
	// the method is missing. A wrapper that carried Claim/Myself/IssueStatus
	// unconditionally would flip both answers for a wrapped linearWriter;
	// around the Jira family (the jiraClient probe writer.go's transport
	// detection already uses) the fuller shape keeps gadak claim and the
	// already-in-category no-op exactly as they were.
	if isJiraFamilyWriter(w) {
		return &jiraActorTrailerWriter{actorTrailerWriter{Writer: w, trailer: trailer}}
	}
	return &actorTrailerWriter{Writer: w, trailer: trailer}
}

// isJiraFamilyWriter reports the writers built around *jira.Client —
// jiraWriter and the bare client — using the same probe clientTransport
// keys on. Single owner's shape, second reader.
func isJiraFamilyWriter(w Writer) bool {
	if _, ok := w.(*jira.Client); ok {
		return true
	}
	_, ok := w.(interface{ jiraClient() *jira.Client })
	return ok
}

// actorTrailerWriter wraps a Writer and appends the trailer to the three
// bodies an agent authors: comments (standalone and transition-carried) and
// created-issue descriptions. Everything else delegates unchanged.
//
// EditIssue is deliberately never stamped: editing a body to add the line
// would rewrite text a person may have written, and the changelog already
// carries the edit's author. UpdateFields is the same refusal — it is the
// edit path for single fields.
type actorTrailerWriter struct {
	Writer
	trailer string
}

func (w *actorTrailerWriter) AddComment(ctx context.Context, key string, body json.RawMessage, visibility *CommentVisibility, internal bool) (Comment, error) {
	return w.Writer.AddComment(ctx, key, appendActorTrailer(body, w.trailer), visibility, internal)
}

func (w *actorTrailerWriter) Transition(ctx context.Context, key, transitionID string, fields map[string]any, comment json.RawMessage) error {
	// A nil (or empty) comment stays exactly that — the trailer never
	// invents a comment a transition did not carry.
	if len(comment) > 0 {
		comment = appendActorTrailer(comment, w.trailer)
	}
	return w.Writer.Transition(ctx, key, transitionID, fields, comment)
}

func (w *actorTrailerWriter) CreateIssue(ctx context.Context, fields map[string]any) (string, error) {
	d, ok := fields["description"]
	if !ok || d == nil {
		// A summary-only create has no body to stamp; absent stays absent.
		return w.Writer.CreateIssue(ctx, fields)
	}
	var stamped json.RawMessage
	switch v := d.(type) {
	case json.RawMessage:
		stamped = appendActorTrailer(v, w.trailer)
		if string(stamped) == string(v) {
			return w.Writer.CreateIssue(ctx, fields)
		}
	case map[string]any:
		raw, err := json.Marshal(v)
		if err != nil {
			return w.Writer.CreateIssue(ctx, fields)
		}
		if next := appendActorTrailer(raw, w.trailer); string(next) != string(raw) {
			stamped = next
		}
	default:
		// A shape this decorator does not know (a plain string is not an
		// ADF doc): send it as the caller built it.
		return w.Writer.CreateIssue(ctx, fields)
	}
	next := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		next[k] = v
	}
	next["description"] = stamped
	return w.Writer.CreateIssue(ctx, next)
}

// The optional faces forward through the wrapper. Interface embedding only
// promotes the Writer method set, so without these a wrapped jiraWriter
// would stop answering AsCreateFieldCatalog and friends — create --field,
// edit --fix-version and link would all refuse on a Jira origin the moment
// an actor resolved. Each face delegates when the wrapped writer has it;
// without it the face's own verb returns the same ErrNo* a bare miss
// returned, so the refusal sentence is byte-identical with and without the
// trailer — it surfaces from the verb call instead of the As* assertion.

func (w *actorTrailerWriter) jiraClient() *jira.Client {
	if h, ok := w.Writer.(interface{ jiraClient() *jira.Client }); ok {
		return h.jiraClient()
	}
	return nil
}

func (w *actorTrailerWriter) ProjectVersions(ctx context.Context, projectKey string) ([]Version, error) {
	if v, ok := w.Writer.(VersionCatalog); ok {
		return v.ProjectVersions(ctx, projectKey)
	}
	return nil, ErrNoVersionCatalog
}

func (w *actorTrailerWriter) CreatesVersionsByName() bool {
	if v, ok := w.Writer.(VersionCatalog); ok {
		return v.CreatesVersionsByName()
	}
	return false
}

func (w *actorTrailerWriter) IssueLinkTypes(ctx context.Context) ([]IssueLinkType, error) {
	if v, ok := w.Writer.(IssueLinker); ok {
		return v.IssueLinkTypes(ctx)
	}
	return nil, ErrNoIssueLinks
}

func (w *actorTrailerWriter) LinkIssues(ctx context.Context, typeID, outwardKey, inwardKey string) error {
	if v, ok := w.Writer.(IssueLinker); ok {
		return v.LinkIssues(ctx, typeID, outwardKey, inwardKey)
	}
	return ErrNoIssueLinks
}

func (w *actorTrailerWriter) IssueLinks(ctx context.Context, key string) ([]IssueLink, error) {
	if v, ok := w.Writer.(IssueLinker); ok {
		return v.IssueLinks(ctx, key)
	}
	return nil, ErrNoIssueLinks
}

func (w *actorTrailerWriter) DeleteIssueLink(ctx context.Context, id string) error {
	if v, ok := w.Writer.(IssueLinker); ok {
		return v.DeleteIssueLink(ctx, id)
	}
	return ErrNoIssueLinks
}

func (w *actorTrailerWriter) CreateFields(ctx context.Context, projectIDOrKey, issueTypeID string) ([]CreateFieldMeta, error) {
	if v, ok := w.Writer.(CreateFieldCatalog); ok {
		return v.CreateFields(ctx, projectIDOrKey, issueTypeID)
	}
	return nil, ErrNoCreateFields
}

func (w *actorTrailerWriter) MediaRef(ctx context.Context, attachmentID string) (mediaID, filename string, err error) {
	if v, ok := w.Writer.(MediaRef); ok {
		return v.MediaRef(ctx, attachmentID)
	}
	return "", "", ErrNoMediaRef
}

func (w *actorTrailerWriter) RemoteLinks(ctx context.Context, key string) ([]RemoteLink, error) {
	if v, ok := w.Writer.(RemoteLinker); ok {
		return v.RemoteLinks(ctx, key)
	}
	return nil, ErrNoRemoteLinks
}

func (w *actorTrailerWriter) SetRemoteLink(ctx context.Context, key string, rl RemoteLink) error {
	if v, ok := w.Writer.(RemoteLinker); ok {
		return v.SetRemoteLink(ctx, key, rl)
	}
	return ErrNoRemoteLinks
}

func (w *actorTrailerWriter) DeleteRemoteLink(ctx context.Context, key, id string) error {
	if v, ok := w.Writer.(RemoteLinker); ok {
		return v.DeleteRemoteLink(ctx, key, id)
	}
	return ErrNoRemoteLinks
}

// jiraActorTrailerWriter is the Jira-family shape: the base wrapper plus the
// verbs that live on *jira.Client outside every origin interface — Claim,
// Myself, IssueStatus. Those three are reached by ad-hoc type assertions
// (cmd/gadak claim's claim.Origin, transition's alreadyInCategory), where a
// missing method is the signal, not an error to return; the compile-time
// assertion below pins this shape to claim.Origin so a signature drift in
// either package fails here instead of silently un-claiming a wrapped
// writer. Constructed only around writers isJiraFamilyWriter admitted.
type jiraActorTrailerWriter struct {
	actorTrailerWriter
}

var _ claim.Origin = (*jiraActorTrailerWriter)(nil)

func (w *jiraActorTrailerWriter) Claim(ctx context.Context, key, transitionID string, takeOver bool) (jira.ClaimResult, error) {
	if o, ok := w.Writer.(claim.Origin); ok {
		return o.Claim(ctx, key, transitionID, takeOver)
	}
	return jira.ClaimResult{}, unsupported("claim is not supported on this writer")
}

func (w *jiraActorTrailerWriter) Myself(ctx context.Context) (jira.User, error) {
	if o, ok := w.Writer.(claim.Origin); ok {
		return o.Myself(ctx)
	}
	return jira.User{}, unsupported("claim is not supported on this writer")
}

func (w *jiraActorTrailerWriter) IssueStatus(ctx context.Context, key string) (jira.Status, *jira.User, error) {
	if o, ok := w.Writer.(claim.Origin); ok {
		return o.IssueStatus(ctx, key)
	}
	return jira.Status{}, nil, unsupported("claim is not supported on this writer")
}

// Resolutions forwards GET /resolution for transition's --resolution
// name lookup (resolveResolution's resolutionCatalog probe). On this shape
// only, like Claim: a wrapped linearWriter must keep the probe's miss —
// that miss is the "does not expose a resolution catalog" refusal — while
// the Jira family keeps resolving names exactly as it did unwrapped.
func (w *jiraActorTrailerWriter) Resolutions(ctx context.Context) ([]jira.NamedID, error) {
	if o, ok := w.Writer.(interface {
		Resolutions(context.Context) ([]jira.NamedID, error)
	}); ok {
		return o.Resolutions(ctx)
	}
	return nil, unsupported("this origin does not expose a resolution catalog")
}

// appendActorTrailer appends the trailer as the final paragraph of an ADF
// document and returns the new raw JSON. Defense, in order: an empty or
// null body, a body that is not a doc, and a doc with no content array are
// all returned unchanged — the trailer never repairs or rewrites a body it
// cannot read. Idempotent: when the doc's last paragraph already is this
// trailer (a retry after a network error re-sends the same stamped body),
// the input is returned untouched, so a write is never double-stamped.
func appendActorTrailer(body json.RawMessage, trailer string) json.RawMessage {
	if len(body) == 0 || trailer == "" {
		return body
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil || doc["type"] != "doc" {
		return body
	}
	content, ok := doc["content"].([]any)
	if !ok {
		return body
	}
	if n := len(content); n > 0 {
		if last, ok := content[n-1].(map[string]any); ok && last["type"] == "paragraph" {
			if paragraphText(last) == trailer {
				return body
			}
		}
	}
	doc["content"] = append(content, map[string]any{
		"type": "paragraph",
		"content": []any{
			map[string]any{"type": "text", "text": trailer},
		},
	})
	out, err := json.Marshal(doc)
	if err != nil {
		return body
	}
	return out
}

// paragraphText is the plain text of a paragraph node: the concatenation of
// its direct text children. Marks and nested structure are ignored — this
// reads back exactly the single-text-node paragraph appendActorTrailer
// writes, no more.
func paragraphText(n map[string]any) string {
	var b strings.Builder
	cs, ok := n["content"].([]any)
	if !ok {
		return ""
	}
	for _, c := range cs {
		m, ok := c.(map[string]any)
		if !ok || m["type"] != "text" {
			continue
		}
		if s, ok := m["text"].(string); ok {
			b.WriteString(s)
		}
	}
	return b.String()
}
