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
	PriorityID     string
	Assignee       string
	AssigneeID     string
	AssigneeEmail  string
	Reporter       string
	ReporterID     string
	ReporterEmail  string
	ParentKey      string
	// HierarchyLevel is the source-neutral tree rank of this issue's type
	// (e.g. epic=1, standard=0, sub-task=-1). Used to derive EpicKey.
	HierarchyLevel int
	Labels         []string
	Components     []string
	FixVersions    []string
	// FixVersionIDs is the same-order source ids for FixVersions (column
	// fix_version_ids). Join the versions catalog on these, not on names —
	// names rename. Empty slice stores "[]", same rule as FixVersions.
	FixVersionIDs   []string
	AffectsVersions []string
	EnvironmentText string
	Duedate         string
	Resolution      string
	ResolutionID    string
	// SprintID/SprintName/SprintState are the one sprint projected from the
	// origin's sprint array (active > future > closed, then larger id). Nil /
	// empty when the site has no sprint field, the array is empty, or an
	// element was not an object. Linear leaves them unset.
	SprintID    *int64
	SprintName  string
	SprintState string
	// SecurityLevelID/SecurityLevel are the origin issue security level.
	// Empty when the origin sent no security object (unrestricted, or a
	// source that has none — Linear). Id is the key; the name is
	// display-only and localizes. nz() stores empty as NULL.
	SecurityLevelID string
	SecurityLevel   string
	DescriptionADF  json.RawMessage
	Custom          map[string]any // mapped custom fields, keyed by config alias
	Raw             json.RawMessage
}

// DevLink is one development-panel link (GDK-497): a pull request the origin
// associates with the issue. URL is the idempotent key per issue.
type DevLink struct {
	Kind       string // pullrequest | deployment | build (GDK-592)
	ExternalID string
	URL        string
	Title      string
	// Status is the stored form of the origin's OPEN|MERGED|DECLINED
	// vocabulary (lowercase). Produced by jira.DevPRStatus.Stored();
	// unknown origin tokens stay the lowercased payload.
	Status string
	// Author is the pull request's author (login). Actor/ActorName are who
	// wrote the link (issuetap's X-Issuetap-Actor accountId and display
	// name). Different axes — a bot links a human's PR — never merged
	// (GDK-589). Branch is the PR head ref. All '' when the origin sent
	// nothing (v33 columns are NOT NULL DEFAULT '').
	Author    string
	Actor     string
	ActorName string
	Branch    string
	// Environment is a deployment row's target (production, staging, …)
	// — kind data with its own v36 column, never a title slot. Empty on
	// pull-request and build rows (GDK-592).
	Environment string
	UpdatedAt   string
}

// DevLinksUpdate is a successful origin answer for one issue's
// development-panel links. The type exists only after a completed fetch
// (or a deliberate drain such as Cloud opt-out). Nil *DevLinksUpdate
// skips the rewrite; a non-nil value with empty Links drains. A fetch
// error cannot construct this value, so it cannot reach ReplaceDevLinks
// or the upsert rewrite (GDK-536 / GDK-580).
type DevLinksUpdate struct {
	Links []DevLink
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
	// VisibilityType/VisibilityValue are the origin restriction (Jira
	// visibility.type/value). Empty means unrestricted. Linear and wiki
	// comments have no such field and stay empty.
	VisibilityType  string
	VisibilityValue string
	// JsdPublic is JSM's jsdPublic marker. nil means the origin omitted
	// the key (not a JSM project, or Jira did not send it); false is an
	// internal comment. Absence and false are distinct.
	JsdPublic *bool
}

// Attachment is metadata only. Bytes are proxied on demand, never mirrored.
type Attachment struct {
	ID         string
	ExternalID string
	Filename   string
	MimeType   string
	Size       int64
	Author     string
	AuthorID   string
	CreatedAt  string
	// URL is the origin content URL when the source does not share Jira's
	// /attachment/content/{id} shape. Empty for Jira (the proxy builds it).
	URL string
}

// ChangeEntry is one field change. Status entries carry ids because display
// names are localized per account.
type ChangeEntry struct {
	ID        string
	At        string
	Author    string
	AuthorID  string
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

// UserAccount is one row of the origin's account catalog (GDK-590). The
// connector collects every user payload the sync already reads — assignee,
// reporter, creator, comment/changelog/attachment authors — and the store
// caches the union in the users table. AccountType keeps the origin's
// spelling ("agent", "app", "atlassian", …); source-neutral on purpose, the
// bot judgement on those values lives in the jira package.
type UserAccount struct {
	AccountID   string
	Name        string
	Email       string
	AccountType string
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
	// Users feeds the account catalog cache. Unlike the child lists above it
	// merges rather than replaces: a row with an empty name or account_type
	// keeps what the catalog already knows (some payloads carry less than the
	// first one that mentioned the account).
	Users []UserAccount
	// DevLinks, when non-nil, is a complete origin answer (including
	// empty). Nil means the origin was not observed and existing rows
	// stay (GDK-536 / GDK-580).
	DevLinks *DevLinksUpdate
}

// Page is the document projection (one row in the pages table). Field names
// match the projection columns, not any source API (same status as Issue).
type Page struct {
	SpaceKey string
	ParentID string
	Version  int
	Status   string
	// Labels are source-neutral tag names (JSON array column). Empty slice is
	// stored as "[]", never NULL. Sync sorts alphabetically for determinism.
	Labels []string
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

// SpaceRow is one wiki space (key + human name + kind). Source-neutral: a
// connector maps its space listing or page-embedded space ref onto this.
// Kind is the source type string (e.g. "global", "personal"); empty is allowed
// when only a name is known from a page hit.
// HomepageID is the content id of the space root page when known; empty on
// page-hit upserts so they do not wipe a value filled by a space listing.
type SpaceRow struct {
	Key        string
	Name       string
	Kind       string
	HomepageID string
}

// VersionRow is one row of the project version catalog (GDK-532). Id is the
// join key; names rename. Released/Archived are origin flags; ReleaseDate is
// a date-only string or empty.
type VersionRow struct {
	ID          string
	ProjectKey  string
	Name        string
	Released    bool
	Archived    bool
	ReleaseDate string
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
