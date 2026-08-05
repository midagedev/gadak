package store

import "encoding/json"

// The types in this file are the store's input contract and they are
// source-neutral on purpose: a connector maps its own payload onto them and the
// store persists it. Nothing here may grow a Jira-specific field name
// (Constitution Article 6).

// Source is one configured connector instance.
type Source struct {
	ID      string // stable slug, e.g. "jira"
	Kind    string
	BaseURL string
}

// Item is the neutral spine row: anything with a title, body, author and
// timestamp fits it.
type Item struct {
	ID         string // "<source_id>:<external_id>"
	SourceID   string
	Kind       string // "issue" in v0.1
	ExternalID string
	Key        string // human-facing key, unique per source
	Title      string
	BodyText   string // flattened body, what FTS indexes
	Author     string
	AuthorID   string
	URL        string
	CreatedAt  string
	UpdatedAt  string
}

// Issue is the tracker projection. `item_id`, `key`, `created_at`, `updated_at`
// are taken from the record's Item and every derived field is computed by the
// store, so a connector fills none of them.
type Issue struct {
	ProjectKey     string
	IssueType      string
	IssueTypeID    string
	Status         string
	StatusID       string
	StatusCategory string // new | inprogress | done
	Priority       string
	Assignee       string
	AssigneeID     string
	AssigneeEmail  string
	Reporter       string
	ReporterID     string
	ReporterEmail  string
	ParentKey      string
	// HierarchyLevel is the source-neutral tree rank of this issue's type
	// (e.g. epic=1, standard=0, sub-task=-1). Used to derive EpicKey.
	HierarchyLevel  int
	Labels          []string
	Components      []string
	FixVersions     []string
	AffectsVersions []string
	EnvironmentText string
	Duedate         string
	Resolution      string
	DescriptionADF  json.RawMessage
	Custom          map[string]any // mapped custom fields, keyed by config alias
	Raw             json.RawMessage
}

// Comment is stored flat: the source API exposes no thread parent.
type Comment struct {
	ID         string // "<source_id>:<comment_id>"
	ExternalID string
	Author     string
	AuthorID   string
	BodyADF    json.RawMessage
	BodyText   string
	CreatedAt  string
	UpdatedAt  string
}

// Attachment is metadata only. Bytes are proxied on demand, never mirrored.
type Attachment struct {
	ID         string
	ExternalID string
	Filename   string
	MimeType   string
	Size       int64
	Author     string
	CreatedAt  string
}

// ChangeEntry is one field change. Status entries carry ids because display
// names are localized per account.
type ChangeEntry struct {
	ID        string
	At        string
	Author    string
	Field     string // "status", "assignee", "priority", ...
	FromValue string
	FromID    string
	ToValue   string
	ToID      string
}

// Link is an edge out of an item. TargetKey may point outside the mirror.
type Link struct {
	Type      string
	Direction string // inward | outward
	TargetKey string
}

// IssueRecord is one item and everything hanging off it. Child lists are
// replaced wholesale on upsert, so a partial list would delete rows.
type IssueRecord struct {
	Item        Item
	Issue       Issue
	Comments    []Comment
	Attachments []Attachment
	Changelog   []ChangeEntry
	Links       []Link
}

// Page is the document projection (one row in the pages table). Field names
// match the projection columns, not any source API (same status as Issue).
type Page struct {
	SpaceKey string
	ParentID string
	Version  int
	Status   string
	// BodyADF is the raw Atlas Document Format body for rendering. Empty when
	// the mirror only has flattened body_text (pre-v10 rows).
	BodyADF json.RawMessage
}

// PageRecord is one document item plus its projection and comments. Comments
// are replaced wholesale on upsert, matching IssueRecord.
type PageRecord struct {
	Item     Item
	Page     Page
	Comments []Comment
}

// Batch is one page of sync output. Categories and Priorities come from the
// site's own metadata endpoints (which are not localized) and feed the derived
// field rules.
type Batch struct {
	Categories map[string]string // status id -> status category
	Priorities []string          // priority display names, most urgent first
	Records    []IssueRecord
	// Force rewrites rows whose updated_at is unchanged. Off, an unchanged row
	// is skipped entirely, which is what keeps an incremental re-run from
	// bumping sync_state.version.
	Force bool
}
