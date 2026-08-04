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
	IssueKey        string   `json:"issue_key"`
	Summary         string   `json:"summary"`
	ProjectKey      string   `json:"project_key"`
	IssueType       string   `json:"issue_type"`
	IssueTypeID     string   `json:"issue_type_id"`
	Status          string   `json:"status"`
	StatusID        string   `json:"status_id"`
	StatusCategory  string   `json:"status_category"`
	Priority        *string  `json:"priority"`
	PriorityRank    int      `json:"priority_rank"`
	Assignee        *string  `json:"assignee"`
	AssigneeID      *string  `json:"assignee_id"`
	AssigneeEmail   *string  `json:"assignee_email"`
	Reporter        *string  `json:"reporter"`
	ReporterEmail   *string  `json:"reporter_email"`
	EpicKey         *string  `json:"epic_key"`
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
	CommentCount    int      `json:"comment_count"`
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
	       i.assignee, i.assignee_id, i.assignee_email, i.reporter, i.reporter_email, i.parent_key,
	       COALESCE(i.labels, '[]'), COALESCE(i.components, '[]'), COALESCE(i.fix_versions, '[]'),
	       i.duedate, i.resolution, i.created_at, i.updated_at,
	       i.status_changed_at, i.resolved_at, i.reopen_count, i.reopened_at, i.comment_count,
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
		if err := rows.Scan(&v.IssueKey, &v.Summary, &v.ProjectKey, &v.IssueType, &v.IssueTypeID,
			&v.Status, &v.StatusID, &v.StatusCategory, &v.Priority, &v.PriorityRank,
			&v.Assignee, &v.AssigneeID, &v.AssigneeEmail, &v.Reporter, &v.ReporterEmail, &v.EpicKey,
			&labels, &components, &fixVersions,
			&v.Duedate, &v.Resolution, &v.CreatedAt, &v.UpdatedAt,
			&v.StatusChangedAt, &v.ResolvedAt, &v.ReopenCount, &v.ReopenedAt, &v.CommentCount,
			&custom,
		); err != nil {
			return nil, err
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
}

// Detail assembles one issue. An unknown key returns sql.ErrNoRows so the
// handler can answer 404.
func (db *DB) Detail(key string) (*Detail, error) {
	var itemID string
	var adf *string
	if err := db.sql.QueryRow(`SELECT item_id, description_adf FROM issues WHERE key = ?`, key).
		Scan(&itemID, &adf); err != nil {
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

// Search runs an FTS5 query over titles, bodies and comment text and returns
// matching issue keys, best match first. A query FTS5 cannot parse is retried as
// a literal phrase rather than surfaced as an error, because this is fed raw
// user input.
func (db *DB) Search(query string, limit int) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []string{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	keys, err := db.search(query, limit)
	if err != nil {
		return db.search(`"`+strings.ReplaceAll(query, `"`, `""`)+`"`, limit)
	}
	return keys, nil
}

func (db *DB) search(match string, limit int) ([]string, error) {
	out := []string{}
	err := each(db.sql, `
		SELECT i.key
		FROM items_fts f
		JOIN items it ON it.rowid = f.rowid
		JOIN issues i ON i.item_id = it.id
		WHERE items_fts MATCH ?
		ORDER BY rank
		LIMIT ?`,
		func(rows *sql.Rows) error {
			var k string
			if err := rows.Scan(&k); err != nil {
				return err
			}
			out = append(out, k)
			return nil
		}, match, limit)
	return out, err
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
