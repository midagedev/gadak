package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// IssueLite is the row shape the read API hydrates the client with. Field names
// are the ones web/src/lib/types.ts already parses (contracts/api.md,
// "IssueLite"): adding is safe, renaming is not.
type IssueLite struct {
	IssueKey       string  `json:"issue_key"`
	Summary        string  `json:"summary"`
	ProjectKey     string  `json:"project_key"`
	IssueType      string  `json:"issue_type"`
	IssueTypeID    string  `json:"issue_type_id"`
	Status         string  `json:"status"`
	StatusID       string  `json:"status_id"`
	StatusCategory string  `json:"status_category"`
	Priority       *string `json:"priority"`
	// PriorityID is the stable Jira priority id (issues.priority_id). Empty
	// on rows a sync has not rewritten since the column was added — clients
	// fall back to the display name for those, the same way they do for a
	// missing status_id / issue_type_id.
	PriorityID    string  `json:"priority_id"`
	PriorityRank  int     `json:"priority_rank"`
	Assignee      *string `json:"assignee"`
	AssigneeID    *string `json:"assignee_id"`
	AssigneeEmail *string `json:"assignee_email"`
	Reporter      *string `json:"reporter"`
	ReporterID    *string `json:"reporter_id"`
	ReporterEmail *string `json:"reporter_email"`
	// EpicKey is the nearest hierarchy_level==1 ancestor (derived). Nil when
	// none. Distinct from ParentKey, which is the direct parent only.
	EpicKey *string `json:"epic_key"`
	// ParentKey is the direct parent issue key (source field), not the epic.
	ParentKey       *string  `json:"parent_key"`
	Labels          []string `json:"labels"`
	Components      []string `json:"components"`
	FixVersions     []string `json:"fix_versions"`
	Duedate         *string  `json:"duedate"`
	Resolution      *string  `json:"resolution"`
	CreatedAt       *string  `json:"created_at"`
	UpdatedAt       *string  `json:"updated_at"`
	StatusChangedAt *string  `json:"status_changed_at"`
	ResolvedAt      *string  `json:"resolved_at"`
	ReopenCount     int      `json:"reopen_count"`
	ReopenedAt      *string  `json:"reopened_at"`
	ReopenReason    *string  `json:"reopen_reason"`
	ClonedFrom      *string  `json:"cloned_from"`
	// SourceProject is ClonedFrom's project prefix, precomputed because the
	// list filter groups by it.
	SourceProject *string `json:"source_project"`
	CommentCount  int     `json:"comment_count"`
	// Custom holds the configured field aliases from issues.custom. The server
	// spreads them into the response as top-level keys, which is where the client
	// reads severity and friends.
	Custom map[string]any `json:"custom,omitempty"`
	// Source is items.source_id (jira / linear / …). Write pickers key on it
	// so a Linear row does not consume a Jira catalog or credential.
	Source string `json:"source,omitempty"`
}

const issueLiteSelect = `
	SELECT i.key, COALESCE(it.title, ''), COALESCE(i.project_key, ''),
	       COALESCE(i.issue_type, ''), COALESCE(i.issue_type_id, ''),
	       COALESCE(i.status, ''), COALESCE(i.status_id, ''), COALESCE(i.status_category, ''),
	       i.priority, COALESCE(i.priority_id, ''), i.priority_rank,
	       i.assignee, i.assignee_id, i.assignee_email, i.reporter, i.reporter_id, i.reporter_email,
	       i.epic_key, i.parent_key,
	       COALESCE(i.labels, '[]'), COALESCE(i.components, '[]'), COALESCE(i.fix_versions, '[]'),
	       i.duedate, i.resolution, i.created_at, i.updated_at,
	       i.status_changed_at, i.resolved_at, i.reopen_count, i.reopened_at,
	       COALESCE(i.reopen_reason, ''), COALESCE(i.cloned_from, ''), i.comment_count,
	       COALESCE(i.custom, '{}'), COALESCE(it.source_id, '')
	FROM issues i JOIN items it ON it.id = i.item_id`

// KeySource returns the source_id owning key in the mirror, "" when the key
// is not mirrored. Write-through gates use it to refuse keys mirrored from a
// read-only source (Linear) before spending a Jira lookup on them. A key can
// exist under more than one source (Jira project ENG and a Linear team key
// ENG both mint ENG-1); write-through is a Jira surface, so a jira row wins —
// a bare LIMIT 1 would refuse a legitimate Jira write nondeterministically.
func (db *DB) KeySource(ctx context.Context, key string) (string, error) {
	var src string
	err := db.sql.QueryRowContext(ctx,
		`SELECT source_id FROM items WHERE key = ? ORDER BY source_id = 'jira' DESC LIMIT 1`,
		key).Scan(&src)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return src, err
}

// IssueLites returns the whole mirror, which is what `bootstrap` sends.
func (db *DB) IssueLites(ctx context.Context) ([]IssueLite, error) {
	return db.issueLites(ctx, issueLiteSelect+` ORDER BY it.updated_at DESC`)
}

// IssueLitesSince returns rows written at or after the given cursor. Only a row
// whose content actually changed has a newer synced_at, so an idle poll returns
// none. The bound is inclusive on purpose: re-sending a row the client already
// has is a harmless upsert, while missing one leaves it stale forever.
func (db *DB) IssueLitesSince(ctx context.Context, since string) ([]IssueLite, error) {
	return db.issueLites(ctx, issueLiteSelect+` WHERE it.synced_at >= ? ORDER BY it.updated_at DESC`, since)
}

// IssueLitesByKeys returns the IssueLite rows for keys, in the order asked.
// Missing and empty keys are skipped. Duplicate keys yield duplicate rows.
// The SQL IN-list is de-duplicated; callers that need FTS rank pass the
// ranked key list and get that order back.
func (db *DB) IssueLitesByKeys(ctx context.Context, keys []string) ([]IssueLite, error) {
	if len(keys) == 0 {
		return []IssueLite{}, nil
	}
	seen := make(map[string]struct{}, len(keys))
	uniq := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, k)
	}
	if len(uniq) == 0 {
		return []IssueLite{}, nil
	}
	// Same IN-placeholder shape as PruneConfluenceSpaces (write.go).
	qs := strings.Repeat("?,", len(uniq)-1) + "?"
	args := make([]any, len(uniq))
	for i, k := range uniq {
		args[i] = k
	}
	rows, err := db.issueLites(ctx, issueLiteSelect+` WHERE i.key IN (`+qs+`)`, args...)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]IssueLite, len(rows))
	for _, l := range rows {
		byKey[l.IssueKey] = l
	}
	out := make([]IssueLite, 0, len(keys))
	for _, k := range keys {
		if l, ok := byKey[k]; ok {
			out = append(out, l)
		}
	}
	return out, nil
}

func (db *DB) issueLites(ctx context.Context, query string, args ...any) ([]IssueLite, error) {
	rows, err := db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IssueLite{}
	for rows.Next() {
		var v IssueLite
		var labels, components, fixVersions, custom string
		var reopenReason, clonedFrom string
		if err := rows.Scan(&v.IssueKey, &v.Summary, &v.ProjectKey, &v.IssueType, &v.IssueTypeID,
			&v.Status, &v.StatusID, &v.StatusCategory, &v.Priority, &v.PriorityID, &v.PriorityRank,
			&v.Assignee, &v.AssigneeID, &v.AssigneeEmail, &v.Reporter, &v.ReporterID, &v.ReporterEmail,
			&v.EpicKey, &v.ParentKey,
			&labels, &components, &fixVersions,
			&v.Duedate, &v.Resolution, &v.CreatedAt, &v.UpdatedAt,
			&v.StatusChangedAt, &v.ResolvedAt, &v.ReopenCount, &v.ReopenedAt,
			&reopenReason, &clonedFrom, &v.CommentCount,
			&custom, &v.Source,
		); err != nil {
			return nil, err
		}
		if reopenReason != "" {
			v.ReopenReason = &reopenReason
		}
		if clonedFrom != "" {
			v.ClonedFrom = &clonedFrom
			if i := strings.IndexByte(clonedFrom, '-'); i > 0 {
				prefix := clonedFrom[:i]
				v.SourceProject = &prefix
			}
		}
		v.Labels = parseArray(&labels)
		v.Components = parseArray(&components)
		v.FixVersions = parseArray(&fixVersions)
		_ = json.Unmarshal([]byte(custom), &v.Custom)
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeletedKeysSince lists keys tombstoned after the cursor. `delta` must report
// these: a missed deletion leaves a tombstone visible in the client forever.
func (db *DB) DeletedKeysSince(ctx context.Context, since string) ([]string, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT key FROM deleted_items WHERE deleted_at >= ? ORDER BY deleted_at`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// DetailComment is one comment as the detail panel renders it.
type DetailComment struct {
	ID         string          `json:"id"`
	ExternalID string          `json:"external_id"`
	Author     string          `json:"author"`
	AuthorID   string          `json:"author_id"`
	BodyADF    json.RawMessage `json:"body_adf"`
	Body       string          `json:"body"` // flattened; the client's fallback when ADF will not render
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
}

// DetailAttachment is metadata only; the handler turns ExternalID into the
// content proxy path. URL is the origin content URL when stored (Linear);
// empty for Jira.
type DetailAttachment struct {
	ID         string `json:"id"`
	ExternalID string `json:"external_id"`
	Filename   string `json:"filename"`
	MimeType   string `json:"mime_type"`
	Size       int64  `json:"size"`
	Author     string `json:"author,omitempty"`
	AuthorID   string `json:"author_id,omitempty"`
	CreatedAt  string `json:"created_at"`
	URL        string `json:"url,omitempty"`
}

// DetailChange is one history row. The ids come along because display values are
// localized: a caller that wants the `from_category` / `to_category` the detail
// contract allows has to resolve them from an id, never from a name.
type DetailChange struct {
	At        string `json:"at"`
	Author    string `json:"author"`
	AuthorID  string `json:"author_id,omitempty"`
	Field     string `json:"field"`
	FromValue string `json:"from_value"`
	FromID    string `json:"from_id"`
	ToValue   string `json:"to_value"`
	ToID      string `json:"to_id"`
}

// DetailLink is an edge plus whatever the mirror knows about the far side. A
// target outside the mirror keeps an empty summary and category.
type DetailLink struct {
	Key            string `json:"key"`
	Type           string `json:"type"`
	Direction      string `json:"direction"`
	Summary        string `json:"summary"`
	StatusCategory string `json:"status_category"`
}

// Detail is everything the on-demand detail view needs, assembled from the
// mirror with no call to the source.
type Detail struct {
	IssueKey       string          `json:"issue_key"`
	DescriptionADF json.RawMessage `json:"description_adf"`
	// DescriptionText is items.body_text. Linear (and any source that does not
	// store ADF) lands markdown/plain here; surfaces fall back to it when
	// DescriptionADF is empty. Never stuff markdown into DescriptionADF.
	DescriptionText string             `json:"description_text,omitempty"`
	Comments        []DetailComment    `json:"comments"`
	Attachments     []DetailAttachment `json:"attachments"`
	History         []DetailChange     `json:"history"`
	LinkedIssues    []DetailLink       `json:"linked_issues"`
	// RefPages are wiki pages this issue's body/comments mention (item_refs,
	// target_kind=page). Only pages present in the mirror; empty omitted.
	RefPages []PageLite `json:"ref_pages,omitempty"`
	// BacklinkPages are wiki pages that mention this issue key. Empty omitted.
	BacklinkPages []PageLite `json:"backlink_pages,omitempty"`
	// Custom is the issue's full alias→value map. List rows strip body-role
	// values (they can be document-sized); detail is where they surface.
	Custom map[string]any `json:"-"`
}

// Detail assembles one issue. An unknown key returns ErrNotFound so the
// handler can answer 404 without importing database/sql.
func (db *DB) Detail(ctx context.Context, key string) (*Detail, error) {
	var itemID string
	var adf *string
	var customJSON string
	var bodyText string
	if err := db.sql.QueryRowContext(ctx, `
		SELECT i.item_id, i.description_adf, COALESCE(i.custom, '{}'), COALESCE(it.body_text, '')
		FROM issues i JOIN items it ON it.id = i.item_id
		WHERE i.key = ?`, key).
		Scan(&itemID, &adf, &customJSON, &bodyText); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	d := &Detail{
		IssueKey:        key,
		DescriptionADF:  rawOrNull(adf),
		DescriptionText: bodyText,
		Comments:        []DetailComment{},
		Attachments:     []DetailAttachment{},
		History:         []DetailChange{},
		LinkedIssues:    []DetailLink{},
	}
	_ = json.Unmarshal([]byte(customJSON), &d.Custom)

	if err := each(ctx, db.sql, `
		SELECT id, COALESCE(external_id,''), COALESCE(author,''), COALESCE(author_id,''),
		       body_adf, COALESCE(body_text,''), COALESCE(created_at,''), COALESCE(updated_at,'')
		FROM comments WHERE item_id = ? ORDER BY created_at, id`,
		func(rows *sql.Rows) error {
			var c DetailComment
			var body *string
			if err := rows.Scan(&c.ID, &c.ExternalID, &c.Author, &c.AuthorID, &body, &c.Body, &c.CreatedAt, &c.UpdatedAt); err != nil {
				return err
			}
			c.BodyADF = rawOrNull(body)
			d.Comments = append(d.Comments, c)
			return nil
		}, itemID); err != nil {
		return nil, err
	}

	if err := each(ctx, db.sql, `
		SELECT id, COALESCE(external_id,''), COALESCE(filename,''), COALESCE(mime_type,''),
		       COALESCE(size,0), COALESCE(author,''), COALESCE(author_id,''), COALESCE(created_at,''),
		       COALESCE(url,'')
		FROM attachments WHERE item_id = ? ORDER BY created_at, id`,
		func(rows *sql.Rows) error {
			var a DetailAttachment
			if err := rows.Scan(&a.ID, &a.ExternalID, &a.Filename, &a.MimeType, &a.Size, &a.Author, &a.AuthorID, &a.CreatedAt, &a.URL); err != nil {
				return err
			}
			d.Attachments = append(d.Attachments, a)
			return nil
		}, itemID); err != nil {
		return nil, err
	}

	if err := each(ctx, db.sql, `
		SELECT COALESCE(at,''), COALESCE(author,''), COALESCE(author_id,''), COALESCE(field,''),
		       COALESCE(from_value,''), COALESCE(from_id,''),
		       COALESCE(to_value,''), COALESCE(to_id,'')
		FROM changelog WHERE item_id = ? ORDER BY at, id`,
		func(rows *sql.Rows) error {
			var c DetailChange
			if err := rows.Scan(&c.At, &c.Author, &c.AuthorID, &c.Field, &c.FromValue, &c.FromID, &c.ToValue, &c.ToID); err != nil {
				return err
			}
			d.History = append(d.History, c)
			return nil
		}, itemID); err != nil {
		return nil, err
	}

	if err := each(ctx, db.sql, `
		SELECT l.target_key, l.type, l.direction,
		       COALESCE(it.title, ''), COALESCE(i.status_category, '')
		FROM links l
		LEFT JOIN issues i ON i.key = l.target_key
		LEFT JOIN items it ON it.id = i.item_id
		WHERE l.item_id = ? ORDER BY l.type, l.target_key`,
		func(rows *sql.Rows) error {
			var l DetailLink
			if err := rows.Scan(&l.Key, &l.Type, &l.Direction, &l.Summary, &l.StatusCategory); err != nil {
				return err
			}
			d.LinkedIssues = append(d.LinkedIssues, l)
			return nil
		}, itemID); err != nil {
		return nil, err
	}

	// Outgoing page refs (this issue mentions pages).
	refPages, err := db.pageLitesFromRefs(ctx, `
		SELECT COALESCE(it.key, ''), COALESCE(it.title, ''), COALESCE(p.space_key, ''),
		       COALESCE(sp.name, ''), COALESCE(sp.homepage_id, ''), COALESCE(p.parent_id, ''), COALESCE(it.author, ''),
		       COALESCE(it.author_id, ''),
		       COALESCE(it.updated_at, ''), COALESCE(p.version, 0), COALESCE(it.url, ''),
		       COALESCE(p.excerpt, ''), COALESCE(p.labels, '[]')
		FROM item_refs r
		JOIN items it ON it.kind = 'page' AND it.key = r.target_key
		JOIN pages p ON p.item_id = it.id
		LEFT JOIN spaces sp ON sp.source_id = it.source_id AND sp.key = p.space_key
		WHERE r.item_id = ? AND r.target_kind = 'page'
		ORDER BY it.updated_at DESC, it.key`, itemID)
	if err != nil {
		return nil, err
	}
	d.RefPages = refPages

	// Incoming page backlinks (pages that mention this issue key).
	backPages, err := db.pageLitesFromRefs(ctx, `
		SELECT COALESCE(it.key, ''), COALESCE(it.title, ''), COALESCE(p.space_key, ''),
		       COALESCE(sp.name, ''), COALESCE(sp.homepage_id, ''), COALESCE(p.parent_id, ''), COALESCE(it.author, ''),
		       COALESCE(it.author_id, ''),
		       COALESCE(it.updated_at, ''), COALESCE(p.version, 0), COALESCE(it.url, ''),
		       COALESCE(p.excerpt, ''), COALESCE(p.labels, '[]')
		FROM item_refs r
		JOIN items it ON it.id = r.item_id AND it.kind = 'page'
		JOIN pages p ON p.item_id = it.id
		LEFT JOIN spaces sp ON sp.source_id = it.source_id AND sp.key = p.space_key
		WHERE r.target_kind = 'issue' AND r.target_key = ?
		ORDER BY it.updated_at DESC, it.key`, key)
	if err != nil {
		return nil, err
	}
	d.BacklinkPages = backPages

	return d, nil
}

// AttachmentBelongs reports whether the mirror lists attachmentID on issueKey.
// The id is the Jira external id when that column is set, otherwise the store
// row id — the same rule handleDetail uses when it builds content URLs.
// One EXISTS query: serving bytes must not pay for Detail's comments/history/links/refs.
func (db *DB) AttachmentBelongs(ctx context.Context, issueKey, attachmentID string) (bool, error) {
	if issueKey == "" || attachmentID == "" {
		return false, nil
	}
	var one int
	err := db.sql.QueryRowContext(ctx, `
		SELECT 1
		FROM attachments a
		JOIN issues i ON i.item_id = a.item_id
		WHERE i.key = ?
		  AND COALESCE(NULLIF(a.external_id, ''), a.id) = ?
		LIMIT 1`, issueKey, attachmentID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// KeysBySource returns issue keys mirrored from one source, in key order.
func (db *DB) KeysBySource(ctx context.Context, sourceID string) ([]string, error) {
	var keys []string
	err := each(ctx, db.sql, `
		SELECT key FROM items WHERE source_id = ? AND kind = 'issue' AND key != '' ORDER BY key`,
		func(rows *sql.Rows) error {
			var k string
			if err := rows.Scan(&k); err != nil {
				return err
			}
			keys = append(keys, k)
			return nil
		}, sourceID)
	return keys, err
}

// AttachmentOrigin is the proxy's lookup: which source owns this attachment
// and, when stored, the origin content URL. Matches AttachmentBelongs' id rule
// (external_id when set, else store id).
func (db *DB) AttachmentOrigin(ctx context.Context, issueKey, attachmentID string) (sourceID, contentURL string, err error) {
	if issueKey == "" || attachmentID == "" {
		return "", "", ErrNotFound
	}
	err = db.sql.QueryRowContext(ctx, `
		SELECT it.source_id, COALESCE(a.url, '')
		FROM attachments a
		JOIN issues i ON i.item_id = a.item_id
		JOIN items it ON it.id = a.item_id
		WHERE i.key = ?
		  AND COALESCE(NULLIF(a.external_id, ''), a.id) = ?
		LIMIT 1`, issueKey, attachmentID).Scan(&sourceID, &contentURL)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return sourceID, contentURL, err
}

// pageLitesFromRefs runs a PageLite-shaped SELECT and returns the rows (nil when empty).
func (db *DB) pageLitesFromRefs(ctx context.Context, query string, args ...any) ([]PageLite, error) {
	var out []PageLite
	err := each(ctx, db.sql, query, func(rows *sql.Rows) error {
		var v PageLite
		var labels string
		if err := rows.Scan(&v.Key, &v.Title, &v.SpaceKey, &v.SpaceName, &v.SpaceHomepageID, &v.ParentID,
			&v.Author, &v.AuthorID, &v.UpdatedAt, &v.Version, &v.URL, &v.Excerpt, &labels); err != nil {
			return err
		}
		v.Labels = parseArray(&labels)
		out = append(out, v)
		return nil
	}, args...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// PageLite is the list/search row for a mirrored wiki page. Field names are
// snake_case JSON to match IssueLite and the read API contract.
type PageLite struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	SpaceKey string `json:"space_key"`
	// SpaceName is the human-readable space title from the spaces table join.
	// Empty when no spaces row is mirrored for this key (never omitted in JSON).
	SpaceName string `json:"space_name"`
	// SpaceHomepageID is the content id of the space root page (spaces.homepage_id).
	// Empty when the space row is missing or homepage has not been learned yet.
	SpaceHomepageID string `json:"space_homepage_id"`
	ParentID        string `json:"parent_id"`
	Author          string `json:"author"`
	// AuthorID is items.author_id. Empty on legacy rows that only have a
	// display name — clients group by this when present and fall back to Author.
	AuthorID  string `json:"author_id"`
	UpdatedAt string `json:"updated_at"`
	Version   int    `json:"version"`
	URL       string `json:"url"`
	// Excerpt is a one-line body preview derived from body_adf (schema v15):
	// whitespace-normalized, at most 200 runes. Empty when the body is empty.
	Excerpt string   `json:"excerpt"`
	Labels  []string `json:"labels"`
}

// PageComment is one comment on a page detail response.
type PageComment struct {
	Author    string          `json:"author"`
	CreatedAt string          `json:"created_at"`
	BodyADF   json.RawMessage `json:"body_adf"`
	BodyText  string          `json:"body_text"`
}

// PageDetail is PageLite plus the raw ADF body and comments.
type PageDetail struct {
	PageLite
	BodyADF  json.RawMessage `json:"body_adf"`
	Comments []PageComment   `json:"comments"`
	// RefIssueKeys are issue keys this page's body mentions (item_refs). Only
	// issues present in the mirror, sorted ascending. Empty omitted.
	RefIssueKeys []string `json:"ref_issue_keys,omitempty"`
	// BacklinkIssueKeys are issue keys that mention this page. Empty omitted.
	BacklinkIssueKeys []string `json:"backlink_issue_keys,omitempty"`
}

// PageStamp is the mirror's record of one page's upstream identity: the
// source's version number and the lastModified the row was written from.
// Both together, never the number alone — a rollback can reuse a number.
type PageStamp struct {
	Version   int
	UpdatedAt string
}

// PageStamps returns the version stamp of every mirrored page in one space,
// keyed by the source's own page id (items.external_id). It exists so an
// incremental sync can answer "do I already hold this page?" from the mirror
// instead of spending a body fetch to find out — a search hit already carries
// version.number and version.when.
//
// The mirror is a disposable cache, so a missing or malformed row simply
// means "fetch it": callers must treat an absent key as unknown, never as
// unchanged.
func (db *DB) PageStamps(ctx context.Context, sourceID, spaceKey string) (map[string]PageStamp, error) {
	out := map[string]PageStamp{}
	if sourceID == "" || spaceKey == "" {
		return out, nil
	}
	err := each(ctx, db.sql, `
		SELECT COALESCE(it.external_id, ''), COALESCE(p.version, 0), COALESCE(it.updated_at, '')
		FROM pages p
		JOIN items it ON it.id = p.item_id
		WHERE it.source_id = ? AND it.kind = 'page' AND p.space_key = ?`,
		func(rows *sql.Rows) error {
			var id string
			var st PageStamp
			if err := rows.Scan(&id, &st.Version, &st.UpdatedAt); err != nil {
				return err
			}
			if id == "" {
				return nil
			}
			out[id] = st
			return nil
		}, sourceID, spaceKey)
	return out, err
}

// PageLites returns every mirrored page, ordered by space then title.
func (db *DB) PageLites(ctx context.Context) ([]PageLite, error) {
	out := []PageLite{}
	err := each(ctx, db.sql, `
		SELECT COALESCE(it.key, ''), COALESCE(it.title, ''), COALESCE(p.space_key, ''),
		       COALESCE(sp.name, ''), COALESCE(sp.homepage_id, ''), COALESCE(p.parent_id, ''), COALESCE(it.author, ''),
		       COALESCE(it.author_id, ''),
		       COALESCE(it.updated_at, ''), COALESCE(p.version, 0), COALESCE(it.url, ''),
		       COALESCE(p.excerpt, ''), COALESCE(p.labels, '[]')
		FROM pages p
		JOIN items it ON it.id = p.item_id
		LEFT JOIN spaces sp ON sp.source_id = it.source_id AND sp.key = p.space_key
		WHERE it.kind = 'page'
		ORDER BY p.space_key, it.title`,
		func(rows *sql.Rows) error {
			var v PageLite
			var labels string
			if err := rows.Scan(&v.Key, &v.Title, &v.SpaceKey, &v.SpaceName, &v.SpaceHomepageID, &v.ParentID,
				&v.Author, &v.AuthorID, &v.UpdatedAt, &v.Version, &v.URL, &v.Excerpt, &labels); err != nil {
				return err
			}
			v.Labels = parseArray(&labels)
			out = append(out, v)
			return nil
		})
	return out, err
}

// PageDetail assembles one page. An unknown key returns (nil, nil).
func (db *DB) PageDetail(ctx context.Context, key string) (*PageDetail, error) {
	var itemID string
	var bodyADF *string
	var labels string
	var d PageDetail
	err := db.sql.QueryRowContext(ctx, `
		SELECT it.id, COALESCE(it.key, ''), COALESCE(it.title, ''), COALESCE(p.space_key, ''),
		       COALESCE(sp.name, ''), COALESCE(sp.homepage_id, ''), COALESCE(p.parent_id, ''), COALESCE(it.author, ''),
		       COALESCE(it.author_id, ''),
		       COALESCE(it.updated_at, ''), COALESCE(p.version, 0), COALESCE(it.url, ''),
		       COALESCE(p.excerpt, ''), p.body_adf, COALESCE(p.labels, '[]')
		FROM pages p
		JOIN items it ON it.id = p.item_id
		LEFT JOIN spaces sp ON sp.source_id = it.source_id AND sp.key = p.space_key
		WHERE it.kind = 'page' AND it.key = ?`, key).
		Scan(&itemID, &d.Key, &d.Title, &d.SpaceKey, &d.SpaceName, &d.SpaceHomepageID, &d.ParentID,
			&d.Author, &d.AuthorID, &d.UpdatedAt, &d.Version, &d.URL, &d.Excerpt, &bodyADF, &labels)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Labels = parseArray(&labels)
	d.BodyADF = rawOrNull(bodyADF)
	d.Comments = []PageComment{}
	if err := each(ctx, db.sql, `
		SELECT COALESCE(author, ''), COALESCE(created_at, ''),
		       body_adf, COALESCE(body_text, '')
		FROM comments WHERE item_id = ? ORDER BY created_at, id`,
		func(rows *sql.Rows) error {
			var c PageComment
			var body *string
			if err := rows.Scan(&c.Author, &c.CreatedAt, &body, &c.BodyText); err != nil {
				return err
			}
			c.BodyADF = rawOrNull(body)
			d.Comments = append(d.Comments, c)
			return nil
		}, itemID); err != nil {
		return nil, err
	}

	// Outgoing: this page mentions these issue keys (mirrored issues only).
	var refKeys []string
	if err := each(ctx, db.sql, `
		SELECT r.target_key
		FROM item_refs r
		JOIN items it ON it.kind = 'issue' AND it.key = r.target_key
		WHERE r.item_id = ? AND r.target_kind = 'issue'
		ORDER BY r.target_key`,
		func(rows *sql.Rows) error {
			var k string
			if err := rows.Scan(&k); err != nil {
				return err
			}
			refKeys = append(refKeys, k)
			return nil
		}, itemID); err != nil {
		return nil, err
	}
	d.RefIssueKeys = refKeys

	// Incoming: issues that mention this page id.
	var backKeys []string
	if err := each(ctx, db.sql, `
		SELECT it.key
		FROM item_refs r
		JOIN items it ON it.id = r.item_id AND it.kind = 'issue'
		WHERE r.target_kind = 'page' AND r.target_key = ?
		ORDER BY it.key`,
		func(rows *sql.Rows) error {
			var k string
			if err := rows.Scan(&k); err != nil {
				return err
			}
			backKeys = append(backKeys, k)
			return nil
		}, key); err != nil {
		return nil, err
	}
	d.BacklinkIssueKeys = backKeys

	return &d, nil
}

// SearchMatch says which FTS column matched and shows a plain-text snippet.
// Field is "title" | "body" | "comment". Snippet has no HTML or highlight
// markers — the client highlights against its own query string.
type SearchMatch struct {
	Field   string `json:"field"`
	Snippet string `json:"snippet"`
}

// SearchExplain is why one returned hit sits where it does. Reason is
// "key-exact", "key-prefix", or "fts". Field and Score are set only for fts
// (winning column and bm25). Filled only by SearchExplain; Search leaves
// Explain nil so the normal path does not allocate it.
type SearchExplain struct {
	Key    string   `json:"key"`
	Reason string   `json:"reason"`
	Field  string   `json:"field,omitempty"`
	Score  *float64 `json:"score,omitempty"`
}

// SearchResult is a kind-aware hit list. Keys are issue keys (key-lookup
// hits first, then FTS among issues); Pages are page hits in the same
// window. Total is the number of hits returned (keys + pages), matching
// the pre-R2 meaning of total = len(results after limit).
// Matches maps each returned issue or page key to the winning FTS column
// match when FTS contributed one.
type SearchResult struct {
	Keys    []string               `json:"keys"`
	Pages   []PageLite             `json:"pages"`
	Total   int                    `json:"total"`
	Matches map[string]SearchMatch `json:"matches"`
	Explain []SearchExplain        `json:"explain,omitempty"`
	// ElapsedMS is the searchAll wall time in milliseconds. Search leaves it
	// 0 (omitted from JSON); SearchExplain fills it so --explain can name
	// the query cost without changing the default Search contract.
	ElapsedMS float64 `json:"elapsed_ms,omitempty"`

	ftsHits []ftsHit // unexported: FTS stream order + bm25, for merge/explain
}

type ftsHit struct {
	kind  string
	key   string
	score float64
}

// searchSnippetRunes is the target plain-snippet length (~120 runes).
const searchSnippetRunes = 120

// ftsBM25Title/Body/Comments are the bm25 column weights for items_fts
// (title, body_text, comments_text). Measured 2026-08-17:
//
//   - Relevance fixture, default equal weights: a 24-repeat body hit (REL-B)
//     ranked above a single title hit (REL-T): [REL-B, REL-T, REL-C].
//   - 10 / 2 / 1 still left REL-B first (body tf outweighed the title column).
//   - 20 / 2 / 1 flips that fixture to [REL-T, REL-B, REL-C].
//   - examples/demo.db "retry": default started NMA-111 (body-heavy);
//     20 / 2 / 1 (same as 10 / 2 / 1) promoted title hits NMA-57, NMA-67 first.
//   - "NMB-140" as an FTS query still matches only mention pages (the key
//     is not indexed); the key lookup below is what puts NMB-140 first.
//
// Do not invent a second ranker — these weights plus key promotion are
// the whole scoring model.
const (
	ftsBM25Title    = 20.0
	ftsBM25Body     = 2.0
	ftsBM25Comments = 1.0
)

func ftsRankSQL() string {
	return fmt.Sprintf("bm25(items_fts, %g, %g, %g)", ftsBM25Title, ftsBM25Body, ftsBM25Comments)
}

// Search runs a key lookup (when the query looks like a key) then an FTS5
// query over titles, bodies and comment text. Key hits are reserved at the
// front of Keys/Pages so FTS cannot drop them when filling limit. Bare terms
// are rewritten as quoted prefix queries (see ftsPrefixQuery). A query FTS5
// cannot parse is retried as a literal phrase rather than surfaced as an
// error, because this is fed raw user input. limit applies to the combined
// result (key hits + FTS), with key hits taking slots first.
func (db *DB) Search(ctx context.Context, query string, limit int) (SearchResult, error) {
	return db.searchAll(ctx, query, limit, false)
}

// SearchExplain is Search plus a per-hit reason list. The FTS query is the
// same; only the returned Explain slice is extra.
func (db *DB) SearchExplain(ctx context.Context, query string, limit int) (SearchResult, error) {
	return db.searchAll(ctx, query, limit, true)
}

func (db *DB) searchAll(ctx context.Context, query string, limit int, explain bool) (SearchResult, error) {
	empty := SearchResult{Keys: []string{}, Pages: []PageLite{}, Matches: map[string]SearchMatch{}}
	query = strings.TrimSpace(query)
	if query == "" {
		return empty, nil
	}
	if limit <= 0 {
		limit = 50
	}
	start := time.Now()

	var keyHits []keyHit
	if looksLikeKey(query) {
		hits, err := db.lookupKeyHits(ctx, query, limit)
		if err != nil {
			return empty, err
		}
		keyHits = hits
	}

	fts := empty
	if len(keyHits) < limit {
		match := ftsPrefixQuery(query)
		res, err := db.search(ctx, match, query, limit)
		if err != nil {
			res, err = db.search(ctx, `"`+strings.ReplaceAll(query, `"`, `""`)+`"`, query, limit)
			if err != nil {
				if len(keyHits) == 0 {
					return empty, err
				}
				res = empty
			}
		}
		fts = res
	}
	out := mergeSearch(keyHits, fts, limit, explain)
	if explain {
		out.ElapsedMS = float64(time.Since(start).Microseconds()) / 1000
	}
	return out, nil
}

func mergeSearch(keyHits []keyHit, fts SearchResult, limit int, explain bool) SearchResult {
	out := SearchResult{Keys: []string{}, Pages: []PageLite{}, Matches: map[string]SearchMatch{}}
	if explain {
		out.Explain = []SearchExplain{}
	}
	ftsPages := make(map[string]PageLite, len(fts.Pages))
	for _, p := range fts.Pages {
		ftsPages[p.Key] = p
	}
	seen := map[string]bool{}
	add := func(kind, key, reason, field string, score *float64, page *PageLite) {
		if key == "" || seen[key] || len(out.Keys)+len(out.Pages) >= limit {
			return
		}
		switch kind {
		case "issue":
			out.Keys = append(out.Keys, key)
		case "page":
			if page == nil {
				if p, ok := ftsPages[key]; ok {
					cp := p
					page = &cp
				}
			}
			if page == nil {
				return
			}
			out.Pages = append(out.Pages, *page)
		default:
			return
		}
		seen[key] = true
		if m, ok := fts.Matches[key]; ok {
			out.Matches[key] = m
			if field == "" {
				field = m.Field
			}
		}
		if explain {
			e := SearchExplain{Key: key, Reason: reason, Field: field}
			if score != nil {
				sc := *score
				e.Score = &sc
			}
			out.Explain = append(out.Explain, e)
		}
	}

	for _, h := range keyHits {
		add(h.kind, h.key, h.reason, "", nil, h.page)
	}
	for _, h := range fts.ftsHits {
		score := h.score
		add(h.kind, h.key, "fts", "", &score, nil)
	}
	out.Total = len(out.Keys) + len(out.Pages)
	return out
}

// ftsPrefixQuery rewrites bare terms into quoted prefix queries ("텀"*).
// Korean is agglutinative — 로그인이/로그인을/로그인은 are all one FTS token, so
// whole-token matching misses nearly every inflected form; prefix matching
// recovers it (and helps English: retri* → retries). Queries that already use
// FTS5 syntax (quotes, operators, parens, *) pass through untouched.
func ftsPrefixQuery(q string) string {
	if strings.ContainsAny(q, `"*()`) {
		return q
	}
	toks := strings.Fields(q)
	for _, t := range toks {
		switch t {
		case "AND", "OR", "NOT", "NEAR":
			return q
		}
	}
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		out = append(out, `"`+strings.ReplaceAll(t, `"`, `""`)+`"*`)
	}
	return strings.Join(out, " ")
}

func (db *DB) search(ctx context.Context, match, rawQuery string, limit int) (SearchResult, error) {
	res := SearchResult{Keys: []string{}, Pages: []PageLite{}, Matches: map[string]SearchMatch{}}
	// Two-pass: the inner query resolves only rowid+bm25+LIMIT (the MATCH/rank
	// scan we have to pay). Snippets, comment concat, and body_text run on at
	// most `limit` rows. Column hits used to be three correlated
	// EXISTS(... MATCH 'title : …') probes — on a contentless FTS those re-scan
	// the prefix posting list per returned row (GDK-166: ~75 ms × 3 × limit
	// for q="p" on 20k). Field attribution is the same prefix test in Go on
	// the source text we already fetch for snippets.
	// items_fts is contentless (content=''), so FTS5 snippet()/highlight()
	// return NULL — we still SELECT them for non-contentless compatibility.
	rank := ftsRankSQL()
	err := each(ctx, db.sql, `
		SELECT it.kind, COALESCE(it.key, ''), COALESCE(it.title, ''),
		       COALESCE(it.author, ''), COALESCE(it.author_id, ''), COALESCE(it.updated_at, ''), COALESCE(it.url, ''),
		       COALESCE(p.space_key, ''), COALESCE(sp.name, ''), COALESCE(sp.homepage_id, ''), COALESCE(p.parent_id, ''),
		       COALESCE(p.version, 0), COALESCE(p.excerpt, ''), COALESCE(p.labels, '[]'),
		       snippet(items_fts, 0, char(1), char(2), '…', 18),
		       snippet(items_fts, 1, char(1), char(2), '…', 18),
		       snippet(items_fts, 2, char(1), char(2), '…', 18),
		       COALESCE(it.body_text, ''),
		       COALESCE((SELECT group_concat(c.body_text, char(10)) FROM comments c WHERE c.item_id = it.id), ''),
		       ranked.rank
		FROM (
			SELECT rowid, `+rank+` AS rank
			FROM items_fts
			WHERE items_fts MATCH ?
			ORDER BY `+rank+`
			LIMIT ?
		) ranked
		JOIN items_fts ON items_fts.rowid = ranked.rowid
		JOIN items it ON it.rowid = ranked.rowid
		LEFT JOIN pages p ON p.item_id = it.id
		LEFT JOIN spaces sp ON sp.source_id = it.source_id AND sp.key = p.space_key
		ORDER BY ranked.rank`,
		func(rows *sql.Rows) error {
			var kind, key, title, author, authorID, updatedAt, url, spaceKey, spaceName, spaceHomepageID, parentID, excerpt, labels string
			var version int
			var snipTitle, snipBody, snipComment sql.NullString
			var bodyText, commentsText string
			var score float64
			if err := rows.Scan(&kind, &key, &title, &author, &authorID, &updatedAt, &url,
				&spaceKey, &spaceName, &spaceHomepageID, &parentID, &version, &excerpt, &labels,
				&snipTitle, &snipBody, &snipComment, &bodyText, &commentsText,
				&score); err != nil {
				return err
			}
			switch kind {
			case "issue":
				if key != "" {
					res.Keys = append(res.Keys, key)
				}
			case "page":
				res.Pages = append(res.Pages, PageLite{
					Key: key, Title: title, SpaceKey: spaceKey, SpaceName: spaceName,
					SpaceHomepageID: spaceHomepageID,
					ParentID:        parentID, Author: author, AuthorID: authorID, UpdatedAt: updatedAt,
					Version: version, URL: url, Excerpt: excerpt, Labels: parseArray(&labels),
				})
			}
			if key != "" {
				res.ftsHits = append(res.ftsHits, ftsHit{kind: kind, key: key, score: score})
				if m, ok := resolveSearchMatch(
					snipTitle.String, snipBody.String, snipComment.String,
					title, bodyText, commentsText, rawQuery,
					ftsColumnPrefixHit(title, rawQuery),
					ftsColumnPrefixHit(bodyText, rawQuery),
					ftsColumnPrefixHit(commentsText, rawQuery),
				); ok {
					res.Matches[key] = m
				}
			}
			return nil
		}, match, limit)
	res.Total = len(res.Keys) + len(res.Pages)
	return res, err
}

// resolveSearchMatch picks the winning field (title > body > comment) and a
// plain-text snippet. Prefer FTS snippet() markers when present; otherwise use
// column-filter hits and build a window from source text (contentless FTS).
func resolveSearchMatch(
	snipTitle, snipBody, snipComment, title, body, comments, rawQuery string,
	titleHit, bodyHit, commentHit bool,
) (SearchMatch, bool) {
	tMark := strings.Contains(snipTitle, "\x01")
	bMark := strings.Contains(snipBody, "\x01")
	cMark := strings.Contains(snipComment, "\x01")
	if tMark || bMark || cMark {
		switch {
		case tMark:
			snip := plainFromFTSSnippet(snipTitle)
			if snip == "" {
				snip = makeSearchSnippet(title, rawQuery)
			}
			return SearchMatch{Field: "title", Snippet: snip}, true
		case bMark:
			return SearchMatch{Field: "body", Snippet: plainFromFTSSnippet(snipBody)}, true
		default:
			return SearchMatch{Field: "comment", Snippet: plainFromFTSSnippet(snipComment)}, true
		}
	}
	// Contentless path: no markers. Column filters decide the field.
	switch {
	case titleHit:
		return SearchMatch{Field: "title", Snippet: makeSearchSnippet(title, rawQuery)}, true
	case bodyHit:
		return SearchMatch{Field: "body", Snippet: makeSearchSnippet(body, rawQuery)}, true
	case commentHit:
		return SearchMatch{Field: "comment", Snippet: makeSearchSnippet(comments, rawQuery)}, true
	default:
		// Overall MATCH hit without a per-column signal — omit rather than guess.
		return SearchMatch{}, false
	}
}

// plainFromFTSSnippet strips FTS highlight markers and normalizes whitespace.
func plainFromFTSSnippet(s string) string {
	s = strings.ReplaceAll(s, "\x01", "")
	s = strings.ReplaceAll(s, "\x02", "")
	return normalizeWhitespace(s)
}

// makeSearchSnippet returns a ~120-rune plain window around the first query
// token found in text, or the front of the text when no token hits.
func makeSearchSnippet(text, rawQuery string) string {
	text = normalizeWhitespace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= searchSnippetRunes {
		return text
	}
	lower := strings.ToLower(text)
	bestByte := -1
	bestEndByte := 0
	for _, tok := range snippetTokens(rawQuery) {
		i := strings.Index(lower, strings.ToLower(tok))
		if i < 0 {
			continue
		}
		if bestByte < 0 || i < bestByte {
			bestByte = i
			bestEndByte = i + len(tok)
		}
	}
	if bestByte < 0 {
		return string(runes[:searchSnippetRunes])
	}
	startRune := utf8.RuneCountInString(text[:bestByte])
	matchRunes := utf8.RuneCountInString(text[bestByte:bestEndByte])
	pad := (searchSnippetRunes - matchRunes) / 2
	if pad < 0 {
		pad = 0
	}
	start := startRune - pad
	if start < 0 {
		start = 0
	}
	end := start + searchSnippetRunes
	if end > len(runes) {
		end = len(runes)
		start = end - searchSnippetRunes
		if start < 0 {
			start = 0
		}
	}
	snip := string(runes[start:end])
	if start > 0 {
		snip = "…" + snip
	}
	if end < len(runes) {
		snip = snip + "…"
	}
	return snip
}

// ftsColumnPrefixHit reports whether text would satisfy a column-filtered
// prefix MATCH for rawQuery (AND of snippet tokens, each as a token prefix).
// Used instead of EXISTS(... MATCH 'title : …') so field attribution does
// not re-scan the FTS prefix posting list per returned row.
func ftsColumnPrefixHit(text, rawQuery string) bool {
	toks := snippetTokens(rawQuery)
	if len(toks) == 0 || text == "" {
		return false
	}
	for _, tok := range toks {
		if !ftsHasTokenPrefix(text, tok) {
			return false
		}
	}
	return true
}

// ftsHasTokenPrefix is a unicode61-shaped scan: hyphen and other
// non-token runes split both sides, then the query tokens must appear
// consecutively (each as a prefix). "REL-140" therefore hits "See REL-140."
// (tokens rel+140) and "p" hits "payment" but not mid-token "Aperture".
func ftsHasTokenPrefix(text, prefix string) bool {
	qToks := ftsTokens(prefix)
	if len(qToks) == 0 {
		return false
	}
	textToks := ftsTokens(text)
	for i := 0; i+len(qToks) <= len(textToks); i++ {
		ok := true
		for j, q := range qToks {
			if !strings.HasPrefix(textToks[i+j], q) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func ftsTokens(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		if isFTSTokenRune(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func isFTSTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// snippetTokens pulls plain search words out of a raw user query for windowing.
func snippetTokens(q string) []string {
	var out []string
	for _, t := range strings.Fields(q) {
		t = strings.Trim(t, `"()*`)
		if t == "" {
			continue
		}
		switch t {
		case "AND", "OR", "NOT", "NEAR":
			continue
		}
		out = append(out, t)
	}
	return out
}

// HasSource reports whether a sources row exists for id.
func (db *DB) HasSource(ctx context.Context, id string) (bool, error) {
	var n int
	err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM sources WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

// each is the query-rows-and-scan boilerplate, once.
func each(ctx context.Context, db *sql.DB, query string, scan func(*sql.Rows) error, args ...any) error {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
