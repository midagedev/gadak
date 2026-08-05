package store

import (
	"database/sql"
	"encoding/json"
	"strings"
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
	PriorityRank   int     `json:"priority_rank"`
	Assignee       *string `json:"assignee"`
	AssigneeID     *string `json:"assignee_id"`
	AssigneeEmail  *string `json:"assignee_email"`
	Reporter       *string `json:"reporter"`
	ReporterEmail  *string `json:"reporter_email"`
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
}

const issueLiteSelect = `
	SELECT i.key, COALESCE(it.title, ''), COALESCE(i.project_key, ''),
	       COALESCE(i.issue_type, ''), COALESCE(i.issue_type_id, ''),
	       COALESCE(i.status, ''), COALESCE(i.status_id, ''), COALESCE(i.status_category, ''),
	       i.priority, i.priority_rank,
	       i.assignee, i.assignee_id, i.assignee_email, i.reporter, i.reporter_email,
	       i.epic_key, i.parent_key,
	       COALESCE(i.labels, '[]'), COALESCE(i.components, '[]'), COALESCE(i.fix_versions, '[]'),
	       i.duedate, i.resolution, i.created_at, i.updated_at,
	       i.status_changed_at, i.resolved_at, i.reopen_count, i.reopened_at,
	       COALESCE(i.reopen_reason, ''), COALESCE(i.cloned_from, ''), i.comment_count,
	       COALESCE(i.custom, '{}')
	FROM issues i JOIN items it ON it.id = i.item_id`

// IssueLites returns the whole mirror, which is what `bootstrap` sends.
func (db *DB) IssueLites() ([]IssueLite, error) {
	return db.issueLites(issueLiteSelect + ` ORDER BY it.updated_at DESC`)
}

// IssueLitesSince returns rows written at or after the given cursor. Only a row
// whose content actually changed has a newer synced_at, so an idle poll returns
// none. The bound is inclusive on purpose: re-sending a row the client already
// has is a harmless upsert, while missing one leaves it stale forever.
func (db *DB) IssueLitesSince(since string) ([]IssueLite, error) {
	return db.issueLites(issueLiteSelect+` WHERE it.synced_at >= ? ORDER BY it.updated_at DESC`, since)
}

func (db *DB) issueLites(query string, args ...any) ([]IssueLite, error) {
	rows, err := db.sql.Query(query, args...)
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
			&v.Status, &v.StatusID, &v.StatusCategory, &v.Priority, &v.PriorityRank,
			&v.Assignee, &v.AssigneeID, &v.AssigneeEmail, &v.Reporter, &v.ReporterEmail,
			&v.EpicKey, &v.ParentKey,
			&labels, &components, &fixVersions,
			&v.Duedate, &v.Resolution, &v.CreatedAt, &v.UpdatedAt,
			&v.StatusChangedAt, &v.ResolvedAt, &v.ReopenCount, &v.ReopenedAt,
			&reopenReason, &clonedFrom, &v.CommentCount,
			&custom,
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
func (db *DB) DeletedKeysSince(since string) ([]string, error) {
	rows, err := db.sql.Query(`SELECT key FROM deleted_items WHERE deleted_at >= ? ORDER BY deleted_at`, since)
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
// content proxy path.
type DetailAttachment struct {
	ID         string `json:"id"`
	ExternalID string `json:"external_id"`
	Filename   string `json:"filename"`
	MimeType   string `json:"mime_type"`
	Size       int64  `json:"size"`
	CreatedAt  string `json:"created_at"`
}

// DetailChange is one history row. The ids come along because display values are
// localized: a caller that wants the `from_category` / `to_category` the detail
// contract allows has to resolve them from an id, never from a name.
type DetailChange struct {
	At        string `json:"at"`
	Author    string `json:"author"`
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
	IssueKey       string             `json:"issue_key"`
	DescriptionADF json.RawMessage    `json:"description_adf"`
	Comments       []DetailComment    `json:"comments"`
	Attachments    []DetailAttachment `json:"attachments"`
	History        []DetailChange     `json:"history"`
	LinkedIssues   []DetailLink       `json:"linked_issues"`
	// Custom is the issue's full alias→value map. List rows strip body-role
	// values (they can be document-sized); detail is where they surface.
	Custom map[string]any `json:"-"`
}

// Detail assembles one issue. An unknown key returns sql.ErrNoRows so the
// handler can answer 404.
func (db *DB) Detail(key string) (*Detail, error) {
	var itemID string
	var adf *string
	var customJSON string
	if err := db.sql.QueryRow(`SELECT item_id, description_adf, COALESCE(custom, '{}') FROM issues WHERE key = ?`, key).
		Scan(&itemID, &adf, &customJSON); err != nil {
		return nil, err
	}
	d := &Detail{
		IssueKey:       key,
		DescriptionADF: rawOrNull(adf),
		Comments:       []DetailComment{},
		Attachments:    []DetailAttachment{},
		History:        []DetailChange{},
		LinkedIssues:   []DetailLink{},
	}
	_ = json.Unmarshal([]byte(customJSON), &d.Custom)

	if err := each(db.sql, `
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

	if err := each(db.sql, `
		SELECT id, COALESCE(external_id,''), COALESCE(filename,''), COALESCE(mime_type,''),
		       COALESCE(size,0), COALESCE(created_at,'')
		FROM attachments WHERE item_id = ? ORDER BY created_at, id`,
		func(rows *sql.Rows) error {
			var a DetailAttachment
			if err := rows.Scan(&a.ID, &a.ExternalID, &a.Filename, &a.MimeType, &a.Size, &a.CreatedAt); err != nil {
				return err
			}
			d.Attachments = append(d.Attachments, a)
			return nil
		}, itemID); err != nil {
		return nil, err
	}

	if err := each(db.sql, `
		SELECT COALESCE(at,''), COALESCE(author,''), COALESCE(field,''),
		       COALESCE(from_value,''), COALESCE(from_id,''),
		       COALESCE(to_value,''), COALESCE(to_id,'')
		FROM changelog WHERE item_id = ? ORDER BY at, id`,
		func(rows *sql.Rows) error {
			var c DetailChange
			if err := rows.Scan(&c.At, &c.Author, &c.Field, &c.FromValue, &c.FromID, &c.ToValue, &c.ToID); err != nil {
				return err
			}
			d.History = append(d.History, c)
			return nil
		}, itemID); err != nil {
		return nil, err
	}

	if err := each(db.sql, `
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
	return d, nil
}

// PageLite is the list/search row for a mirrored wiki page. Field names are
// snake_case JSON to match IssueLite and the read API contract.
type PageLite struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	SpaceKey  string `json:"space_key"`
	ParentID  string `json:"parent_id"`
	Author    string `json:"author"`
	UpdatedAt string `json:"updated_at"`
	Version   int    `json:"version"`
	URL       string `json:"url"`
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
}

// PageLites returns every mirrored page, ordered by space then title.
func (db *DB) PageLites() ([]PageLite, error) {
	out := []PageLite{}
	err := each(db.sql, `
		SELECT COALESCE(it.key, ''), COALESCE(it.title, ''), COALESCE(p.space_key, ''),
		       COALESCE(p.parent_id, ''), COALESCE(it.author, ''), COALESCE(it.updated_at, ''),
		       COALESCE(p.version, 0), COALESCE(it.url, '')
		FROM pages p
		JOIN items it ON it.id = p.item_id
		WHERE it.kind = 'page'
		ORDER BY p.space_key, it.title`,
		func(rows *sql.Rows) error {
			var v PageLite
			if err := rows.Scan(&v.Key, &v.Title, &v.SpaceKey, &v.ParentID,
				&v.Author, &v.UpdatedAt, &v.Version, &v.URL); err != nil {
				return err
			}
			out = append(out, v)
			return nil
		})
	return out, err
}

// PageDetail assembles one page. An unknown key returns (nil, nil).
func (db *DB) PageDetail(key string) (*PageDetail, error) {
	var itemID string
	var bodyADF *string
	var d PageDetail
	err := db.sql.QueryRow(`
		SELECT it.id, COALESCE(it.key, ''), COALESCE(it.title, ''), COALESCE(p.space_key, ''),
		       COALESCE(p.parent_id, ''), COALESCE(it.author, ''), COALESCE(it.updated_at, ''),
		       COALESCE(p.version, 0), COALESCE(it.url, ''), p.body_adf
		FROM pages p
		JOIN items it ON it.id = p.item_id
		WHERE it.kind = 'page' AND it.key = ?`, key).
		Scan(&itemID, &d.Key, &d.Title, &d.SpaceKey, &d.ParentID,
			&d.Author, &d.UpdatedAt, &d.Version, &d.URL, &bodyADF)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.BodyADF = rawOrNull(bodyADF)
	d.Comments = []PageComment{}
	if err := each(db.sql, `
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
	return &d, nil
}

// SearchResult is a kind-aware FTS hit list. Keys are issue keys (best match
// first among issues in the ranked window); Pages are page hits in the same
// ranked window. Total is the number of hits returned (keys + pages), matching
// the pre-R2 meaning of total = len(results after limit).
type SearchResult struct {
	Keys  []string
	Pages []PageLite
	Total int
}

// Search runs an FTS5 query over titles, bodies and comment text and returns
// matching issues and pages, best match first. Bare terms are rewritten as
// quoted prefix queries (see ftsPrefixQuery). A query FTS5 cannot parse is
// retried as a literal phrase rather than surfaced as an error, because this is
// fed raw user input. limit applies to the combined FTS result set.
func (db *DB) Search(query string, limit int) (SearchResult, error) {
	empty := SearchResult{Keys: []string{}, Pages: []PageLite{}}
	query = strings.TrimSpace(query)
	if query == "" {
		return empty, nil
	}
	if limit <= 0 {
		limit = 50
	}
	match := ftsPrefixQuery(query)
	res, err := db.search(match, limit)
	if err != nil {
		return db.search(`"`+strings.ReplaceAll(query, `"`, `""`)+`"`, limit)
	}
	return res, nil
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

func (db *DB) search(match string, limit int) (SearchResult, error) {
	res := SearchResult{Keys: []string{}, Pages: []PageLite{}}
	err := each(db.sql, `
		SELECT it.kind, COALESCE(it.key, ''), COALESCE(it.title, ''),
		       COALESCE(it.author, ''), COALESCE(it.updated_at, ''), COALESCE(it.url, ''),
		       COALESCE(p.space_key, ''), COALESCE(p.parent_id, ''), COALESCE(p.version, 0)
		FROM items_fts f
		JOIN items it ON it.rowid = f.rowid
		LEFT JOIN pages p ON p.item_id = it.id
		WHERE items_fts MATCH ?
		ORDER BY rank
		LIMIT ?`,
		func(rows *sql.Rows) error {
			var kind, key, title, author, updatedAt, url, spaceKey, parentID string
			var version int
			if err := rows.Scan(&kind, &key, &title, &author, &updatedAt, &url,
				&spaceKey, &parentID, &version); err != nil {
				return err
			}
			switch kind {
			case "issue":
				if key != "" {
					res.Keys = append(res.Keys, key)
				}
			case "page":
				res.Pages = append(res.Pages, PageLite{
					Key: key, Title: title, SpaceKey: spaceKey, ParentID: parentID,
					Author: author, UpdatedAt: updatedAt, Version: version, URL: url,
				})
			}
			return nil
		}, match, limit)
	res.Total = len(res.Keys) + len(res.Pages)
	return res, err
}

// HasSource reports whether a sources row exists for id.
func (db *DB) HasSource(id string) (bool, error) {
	var n int
	err := db.sql.QueryRow(`SELECT COUNT(*) FROM sources WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

// each is the query-rows-and-scan boilerplate, once.
func each(db *sql.DB, query string, scan func(*sql.Rows) error, args ...any) error {
	rows, err := db.Query(query, args...)
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
