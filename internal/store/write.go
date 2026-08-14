package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// UpsertSource records a connector instance. Credentials never come near it
// (Constitution Article 8).
func (db *DB) UpsertSource(ctx context.Context, s Source) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO sources (id, kind, base_url) VALUES (?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET kind = excluded.kind, base_url = excluded.base_url`,
			s.ID, s.Kind, nz(s.BaseURL))
		return err
	})
}

// UpsertIssues writes one page of sync output in a single transaction and
// returns how many items it actually changed. Derived fields are recomputed from
// the batch's changelog, never carried over (contracts/sync.md invariant 5).
//
// An item whose updated_at is unchanged is skipped whole — children, FTS and the
// version counter included — unless Batch.Force is set. That is what makes an
// incremental re-run over the watermark overlap window a no-op.
func (db *DB) UpsertIssues(ctx context.Context, b Batch) (int, error) {
	if len(b.Records) == 0 {
		return 0, nil
	}
	changed := 0
	err := db.write(ctx, func(tx *sql.Tx) error {
		sources := map[string]bool{}
		for _, r := range b.Records {
			ok, err := upsertRecord(tx, b, r)
			if err != nil {
				return fmt.Errorf("%s: %w", r.Item.Key, err)
			}
			if ok {
				changed++
				sources[r.Item.SourceID] = true
			}
		}
		for id := range sources {
			if err := bumpVersion(tx, id, db.schemaVersion); err != nil {
				return err
			}
		}
		// Parent chains can resolve only after later pages arrive, so recompute
		// epic_key for the whole table after every batch (cheap two-hop join).
		if err := recomputeEpicKeys(tx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

// recomputeEpicKeys sets issues.epic_key to the nearest hierarchy_level==1
// ancestor via parent_key (direct parent, else grandparent). NULL when none.
// Full-table UPDATE is intentional: reverse batch order and children of
// unchanged parents both need a second look, and the join is two hops.
func recomputeEpicKeys(tx *sql.Tx) error {
	_, err := tx.Exec(`
		UPDATE issues SET epic_key = (
			SELECT CASE
				WHEN p.hierarchy_level = 1 THEN p.key
				WHEN gp.hierarchy_level = 1 THEN gp.key
			END
			FROM issues p
			LEFT JOIN issues gp ON gp.key = p.parent_key
			WHERE p.key = issues.parent_key
		)`)
	return err
}

func upsertRecord(tx *sql.Tx, b Batch, r IssueRecord) (bool, error) {
	it := r.Item
	if it.ID == "" || it.SourceID == "" {
		return false, errors.New("item id and source_id are required")
	}
	if it.Kind == "" {
		it.Kind = "issue"
	}
	syncedAt := Now()

	// The conditional DO UPDATE is the change detector: no RETURNING row means
	// the source reported no new `updated`, so nothing below needs to run.
	var rowid int64
	err := tx.QueryRow(`
		INSERT INTO items (id, source_id, kind, external_id, key, title, body_text,
		                   author, author_id, url, created_at, updated_at, synced_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			source_id = excluded.source_id, kind = excluded.kind,
			external_id = excluded.external_id, key = excluded.key,
			title = excluded.title, body_text = excluded.body_text,
			author = excluded.author, author_id = excluded.author_id,
			url = excluded.url, created_at = excluded.created_at,
			updated_at = excluded.updated_at, synced_at = excluded.synced_at
		WHERE ? OR excluded.updated_at IS NOT items.updated_at
		RETURNING rowid`,
		it.ID, it.SourceID, it.Kind, nz(it.ExternalID), nz(it.Key), nz(it.Title),
		nz(it.BodyText), nz(it.Author), nz(it.AuthorID), nz(it.URL),
		nz(it.CreatedAt), nz(it.UpdatedAt), syncedAt,
		b.Force,
	).Scan(&rowid)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	d := Derive(DeriveInput{
		Changelog:       r.Changelog,
		Categories:      b.Categories,
		CurrentCategory: r.Issue.StatusCategory,
		Priority:        r.Issue.Priority,
		Priorities:      b.Priorities,
		Comments:        r.Comments,
		Links:           r.Links,
	})

	is := r.Issue
	if _, err := tx.Exec(`DELETE FROM issues WHERE item_id = ?`, it.ID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`
		INSERT INTO issues (item_id, key, project_key, issue_type, issue_type_id,
			status, status_id, status_category, priority, priority_rank,
			assignee, assignee_id, assignee_email, reporter, reporter_id, reporter_email, parent_key,
			labels, components, fix_versions, affects_versions, environment_text,
			duedate, resolution, created_at, updated_at,
			status_changed_at, resolved_at, reopen_count, reopened_at, reopen_reason,
			assignee_changed_at, comment_count, description_adf, custom, raw, cloned_from,
			hierarchy_level)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		it.ID, it.Key, nz(is.ProjectKey), nz(is.IssueType), nz(is.IssueTypeID),
		nz(is.Status), nz(is.StatusID), nz(is.StatusCategory), nz(is.Priority), d.PriorityRank,
		nz(is.Assignee), nz(is.AssigneeID), nz(is.AssigneeEmail), nz(is.Reporter),
		nz(is.ReporterID), nz(is.ReporterEmail), nz(is.ParentKey),
		jsonArray(is.Labels), jsonArray(is.Components), jsonArray(is.FixVersions),
		jsonArray(is.AffectsVersions), nz(is.EnvironmentText),
		nz(is.Duedate), nz(is.Resolution), nz(it.CreatedAt), nz(it.UpdatedAt),
		d.StatusChangedAt, d.ResolvedAt, d.ReopenCount, d.ReopenedAt, d.ReopenReason,
		d.AssigneeChangedAt, d.CommentCount, jsonRaw(is.DescriptionADF),
		jsonObject(is.Custom), jsonRaw(is.Raw), d.ClonedFrom,
		is.HierarchyLevel,
	); err != nil {
		return false, err
	}

	// Child lists arrive complete, so replacing them is both correct and the
	// only way a removed comment or link leaves the mirror.
	for _, t := range []string{"comments", "attachments", "changelog", "links"} {
		if _, err := tx.Exec(`DELETE FROM `+t+` WHERE item_id = ?`, it.ID); err != nil {
			return false, err
		}
	}
	bodies := make([]string, 0, len(r.Comments))
	for _, c := range r.Comments {
		if _, err := tx.Exec(`
			INSERT INTO comments (id, item_id, external_id, author, author_id,
			                      body_adf, body_text, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			c.ID, it.ID, nz(c.ExternalID), nz(c.Author), nz(c.AuthorID),
			jsonRaw(c.BodyADF), nz(c.BodyText), nz(c.CreatedAt), nz(c.UpdatedAt),
		); err != nil {
			return false, err
		}
		if c.BodyText != "" {
			bodies = append(bodies, c.BodyText)
		}
	}
	for _, a := range r.Attachments {
		if _, err := tx.Exec(`
			INSERT INTO attachments (id, item_id, external_id, filename, mime_type, size, author, author_id, created_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			a.ID, it.ID, nz(a.ExternalID), nz(a.Filename), nz(a.MimeType), a.Size,
			nz(a.Author), nz(a.AuthorID), nz(a.CreatedAt),
		); err != nil {
			return false, err
		}
	}
	for i, e := range r.Changelog {
		id := e.ID
		if id == "" {
			id = fmt.Sprintf("%s:%d", it.ID, i)
		}
		if _, err := tx.Exec(`
			INSERT INTO changelog (id, item_id, at, author, author_id, field, from_value, from_id, to_value, to_id)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			id, it.ID, nz(e.At), nz(e.Author), nz(e.AuthorID), nz(e.Field),
			nz(e.FromValue), nz(e.FromID), nz(e.ToValue), nz(e.ToID),
		); err != nil {
			return false, err
		}
	}
	for _, l := range r.Links {
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO links (item_id, type, direction, target_key) VALUES (?,?,?,?)`,
			it.ID, l.Type, l.Direction, l.TargetKey,
		); err != nil {
			return false, err
		}
	}

	if err := writeFTS(tx, rowid, it.Title, it.BodyText, strings.Join(bodies, "\n")); err != nil {
		return false, err
	}

	// Text-derived page refs from raw ADF + flattened text (after comments written).
	commentBlobs := make([]string, 0, len(r.Comments)*2)
	for _, c := range r.Comments {
		if len(c.BodyADF) > 0 {
			commentBlobs = append(commentBlobs, string(c.BodyADF))
		}
		if c.BodyText != "" {
			commentBlobs = append(commentBlobs, c.BodyText)
		}
	}
	pageRefs := filterSelfRef(ExtractPageRefsFromIssue(string(is.DescriptionADF), it.BodyText, commentBlobs), it.Key)
	if err := replaceItemRefs(tx, it.ID, pageRefs); err != nil {
		return false, err
	}

	// An item that came back is no longer deleted.
	_, err = tx.Exec(`DELETE FROM deleted_items WHERE source_id = ? AND key = ?`, it.SourceID, it.Key)
	return true, err
}

// UpsertSpaces writes wiki space rows (key → name/kind/homepage_id) for a
// source. Empty name/kind/homepage_id on conflict keeps the previous value so
// page-hit upserts (name only) do not wipe kind or homepage filled by a full
// space listing or per-space GET.
func (db *DB) UpsertSpaces(ctx context.Context, sourceID string, rows []SpaceRow) error {
	if sourceID == "" || len(rows) == 0 {
		return nil
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		for _, r := range rows {
			if r.Key == "" {
				continue
			}
			if _, err := tx.Exec(`
				INSERT INTO spaces (source_id, key, name, kind, homepage_id) VALUES (?,?,?,?,?)
				ON CONFLICT(source_id, key) DO UPDATE SET
					name = CASE WHEN excluded.name != '' THEN excluded.name ELSE spaces.name END,
					kind = CASE WHEN excluded.kind != '' THEN excluded.kind ELSE spaces.kind END,
					homepage_id = CASE WHEN excluded.homepage_id != '' THEN excluded.homepage_id ELSE spaces.homepage_id END`,
				sourceID, r.Key, r.Name, r.Kind, r.HomepageID,
			); err != nil {
				return fmt.Errorf("space %s: %w", r.Key, err)
			}
		}
		return nil
	})
}

// ConfluenceSpaceWatermarks returns each space key's incremental floor for
// sourceID. A missing or NULL watermark is "". Callers treat empty as
// "not yet backfilled" and run a full fetch for that space.
func (db *DB) ConfluenceSpaceWatermarks(ctx context.Context, sourceID string) (map[string]string, error) {
	out := map[string]string{}
	if sourceID == "" {
		return out, nil
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT key, watermark FROM spaces WHERE source_id = ?`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var wm sql.NullString
		if err := rows.Scan(&key, &wm); err != nil {
			return nil, err
		}
		if wm.Valid {
			out[key] = wm.String
		} else {
			out[key] = ""
		}
	}
	return out, rows.Err()
}

// SetSpaceWatermark writes one space's incremental floor. An unknown key is
// inserted so a just-scoped space can be stamped after its first successful
// pass without a separate UpsertSpaces.
func (db *DB) SetSpaceWatermark(ctx context.Context, sourceID, key, watermark string) error {
	if sourceID == "" || key == "" {
		return nil
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO spaces (source_id, key, name, kind, homepage_id, watermark)
			VALUES (?, ?, '', '', '', ?)
			ON CONFLICT(source_id, key) DO UPDATE SET watermark = excluded.watermark`,
			sourceID, key, nz(watermark))
		return err
	})
}

// PruneConfluenceSpaces deletes mirrored wiki pages (and their items, FTS
// rows, and cascaded children) whose space_key is not in keepKeys, then
// drops the matching spaces rows. keepKeys empty is a no-op so a zero-scope
// call cannot wipe the mirror. The returned count is the number of pages
// removed (space-row-only cleanup is not counted).
func (db *DB) PruneConfluenceSpaces(ctx context.Context, sourceID string, keepKeys []string) (int, error) {
	if sourceID == "" || len(keepKeys) == 0 {
		return 0, nil
	}
	keep := make(map[string]struct{}, len(keepKeys))
	args := []any{sourceID}
	for _, k := range keepKeys {
		if k == "" {
			continue
		}
		if _, ok := keep[k]; ok {
			continue
		}
		keep[k] = struct{}{}
		args = append(args, k)
	}
	if len(keep) == 0 {
		return 0, nil
	}
	qs := strings.Repeat("?,", len(keep)-1) + "?"
	rows, err := db.sql.QueryContext(ctx, `
		SELECT i.key FROM items i
		JOIN pages p ON p.item_id = i.id
		WHERE i.source_id = ? AND p.space_key NOT IN (`+qs+`)`, args...)
	if err != nil {
		return 0, err
	}
	var gone []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return 0, err
		}
		gone = append(gone, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	n, err := db.DeleteItems(ctx, sourceID, gone)
	if err != nil {
		return 0, err
	}
	err = db.write(ctx, func(tx *sql.Tx) error {
		srows, err := tx.QueryContext(ctx, `SELECT key FROM spaces WHERE source_id = ?`, sourceID)
		if err != nil {
			return err
		}
		var drop []string
		for srows.Next() {
			var key string
			if err := srows.Scan(&key); err != nil {
				srows.Close()
				return err
			}
			if _, ok := keep[key]; !ok {
				drop = append(drop, key)
			}
		}
		if err := srows.Err(); err != nil {
			srows.Close()
			return err
		}
		srows.Close()
		for _, key := range drop {
			if _, err := tx.Exec(`DELETE FROM spaces WHERE source_id = ? AND key = ?`, sourceID, key); err != nil {
				return err
			}
		}
		if len(drop) == 0 || n > 0 {
			// DeleteItems already bumped version when pages were removed.
			return nil
		}
		return bumpVersion(tx, sourceID, db.schemaVersion)
	})
	if err != nil {
		return n, err
	}
	return n, nil
}

// UpsertPages writes document records in a single transaction and returns how
// many items it actually changed. A page whose stored body, meta and comments
// match the incoming record is skipped (no version bump). Comment-only edits
// still bump because comments are part of the compare — that is the comments-
// only trap the old always-rewrite path existed to close.
func (db *DB) UpsertPages(ctx context.Context, records []PageRecord) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}
	changed := 0
	err := db.write(ctx, func(tx *sql.Tx) error {
		// known project keys once per batch for bare-text issue-key filtering
		known, err := loadKnownProjectKeys(tx)
		if err != nil {
			return err
		}
		sources := map[string]bool{}
		for _, r := range records {
			ok, err := upsertPageRecord(tx, r, known)
			if err != nil {
				return fmt.Errorf("%s: %w", r.Item.Key, err)
			}
			if ok {
				changed++
				sources[r.Item.SourceID] = true
			}
		}
		for id := range sources {
			if err := bumpVersion(tx, id, db.schemaVersion); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

func upsertPageRecord(tx *sql.Tx, r PageRecord, knownProjects map[string]bool) (bool, error) {
	it := r.Item
	if it.ID == "" || it.SourceID == "" {
		return false, errors.New("item id and source_id are required")
	}
	if it.Kind == "" {
		it.Kind = "page"
	}
	r.Item.Kind = it.Kind
	if pg := &r.Page; pg.Status == "" {
		pg.Status = "current"
	}
	if r.Page.Version <= 0 {
		r.Page.Version = 1
	}

	unchanged, err := pageRecordUnchanged(tx, r)
	if err != nil {
		return false, err
	}
	if unchanged {
		return false, nil
	}

	syncedAt := Now()

	var rowid int64
	err = tx.QueryRow(`
		INSERT INTO items (id, source_id, kind, external_id, key, title, body_text,
		                   author, author_id, url, created_at, updated_at, synced_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			source_id = excluded.source_id, kind = excluded.kind,
			external_id = excluded.external_id, key = excluded.key,
			title = excluded.title, body_text = excluded.body_text,
			author = excluded.author, author_id = excluded.author_id,
			url = excluded.url, created_at = excluded.created_at,
			updated_at = excluded.updated_at, synced_at = excluded.synced_at
		RETURNING rowid`,
		it.ID, it.SourceID, it.Kind, nz(it.ExternalID), nz(it.Key), nz(it.Title),
		nz(it.BodyText), nz(it.Author), nz(it.AuthorID), nz(it.URL),
		nz(it.CreatedAt), nz(it.UpdatedAt), syncedAt,
	).Scan(&rowid)
	if err != nil {
		return false, err
	}

	pg := r.Page
	if pg.Status == "" {
		pg.Status = "current"
	}
	if pg.Version <= 0 {
		pg.Version = 1
	}
	if _, err := tx.Exec(`DELETE FROM pages WHERE item_id = ?`, it.ID); err != nil {
		return false, err
	}
	bodyADF := ""
	if len(pg.BodyADF) > 0 {
		bodyADF = string(pg.BodyADF)
	}
	excerpt := pageExcerptFromADF(bodyADF)
	if _, err := tx.Exec(`
		INSERT INTO pages (item_id, space_key, parent_id, version, status, body_adf, labels, excerpt)
		VALUES (?,?,?,?,?,?,?,?)`,
		// parent_id/space_key/status/body_adf/excerpt are NOT NULL — empty string, never nil.
		// labels is a JSON array string ("[]" when absent), matching issues.labels.
		it.ID, pg.SpaceKey, pg.ParentID, pg.Version, pg.Status, bodyADF, jsonArray(pg.Labels), excerpt,
	); err != nil {
		return false, err
	}

	if _, err := tx.Exec(`DELETE FROM comments WHERE item_id = ?`, it.ID); err != nil {
		return false, err
	}
	bodies := make([]string, 0, len(r.Comments))
	for _, c := range r.Comments {
		if _, err := tx.Exec(`
			INSERT INTO comments (id, item_id, external_id, author, author_id,
			                      body_adf, body_text, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			c.ID, it.ID, nz(c.ExternalID), nz(c.Author), nz(c.AuthorID),
			jsonRaw(c.BodyADF), nz(c.BodyText), nz(c.CreatedAt), nz(c.UpdatedAt),
		); err != nil {
			return false, err
		}
		if c.BodyText != "" {
			bodies = append(bodies, c.BodyText)
		}
	}

	if err := writeFTS(tx, rowid, it.Title, it.BodyText, strings.Join(bodies, "\n")); err != nil {
		return false, err
	}

	// Text-derived issue refs from ADF URLs + plain body (not comments).
	issueRefs := filterSelfRef(ExtractIssueRefsFromPage(bodyADF, it.BodyText, knownProjects), it.Key)
	if err := replaceItemRefs(tx, it.ID, issueRefs); err != nil {
		return false, err
	}

	_, err = tx.Exec(`DELETE FROM deleted_items WHERE source_id = ? AND key = ?`, it.SourceID, it.Key)
	return true, err
}

// pageRecordUnchanged reports whether the stored page (item + projection +
// comments) already matches r. Compared fields — keep this list in lockstep
// with the write below, or a silent skip will leave the client stale:
//
//	items: title, body_text, author, author_id, url, created_at, updated_at,
//	       external_id, key
//	pages: space_key, parent_id, version, status, body_adf, labels
//	comments: id, external_id, author, author_id, body_adf, body_text,
//	          created_at, updated_at (order-independent, keyed by id)
//
// excerpt is derived from body_adf and is not compared. synced_at is the
// write stamp and is not compared.
func pageRecordUnchanged(tx *sql.Tx, r PageRecord) (bool, error) {
	it := r.Item
	pg := r.Page
	var (
		title, body, author, authorID, url, created, updated string
		extID, key                                           string
		space, parent, status, adf, labels                   string
		version                                              int
	)
	err := tx.QueryRow(`
		SELECT COALESCE(it.title,''), COALESCE(it.body_text,''),
		       COALESCE(it.author,''), COALESCE(it.author_id,''), COALESCE(it.url,''),
		       COALESCE(it.created_at,''), COALESCE(it.updated_at,''),
		       COALESCE(it.external_id,''), COALESCE(it.key,''),
		       COALESCE(p.space_key,''), COALESCE(p.parent_id,''), p.version,
		       COALESCE(p.status,''), COALESCE(p.body_adf,''), COALESCE(p.labels,'[]')
		FROM items it JOIN pages p ON p.item_id = it.id
		WHERE it.id = ?`, it.ID).Scan(
		&title, &body, &author, &authorID, &url, &created, &updated,
		&extID, &key, &space, &parent, &version, &status, &adf, &labels,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	bodyADF := ""
	if len(pg.BodyADF) > 0 {
		bodyADF = string(pg.BodyADF)
	}
	if title != it.Title || body != it.BodyText ||
		author != it.Author || authorID != it.AuthorID || url != it.URL ||
		created != it.CreatedAt || updated != it.UpdatedAt ||
		extID != it.ExternalID || key != it.Key ||
		space != pg.SpaceKey || parent != pg.ParentID || version != pg.Version ||
		status != pg.Status || adf != bodyADF || labels != jsonArray(pg.Labels) {
		return false, nil
	}

	rows, err := tx.Query(`
		SELECT id, COALESCE(external_id,''), COALESCE(author,''), COALESCE(author_id,''),
		       COALESCE(body_adf,''), COALESCE(body_text,''),
		       COALESCE(created_at,''), COALESCE(updated_at,'')
		FROM comments WHERE item_id = ?`, it.ID)
	if err != nil {
		return false, err
	}
	type snap struct {
		id, ext, author, authorID, adf, body, created, updated string
	}
	var have []snap
	for rows.Next() {
		var s snap
		if err := rows.Scan(&s.id, &s.ext, &s.author, &s.authorID, &s.adf, &s.body, &s.created, &s.updated); err != nil {
			rows.Close()
			return false, err
		}
		have = append(have, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close()

	want := make([]snap, 0, len(r.Comments))
	for _, c := range r.Comments {
		adf := ""
		if len(c.BodyADF) > 0 {
			adf = string(c.BodyADF)
		}
		want = append(want, snap{
			id: c.ID, ext: c.ExternalID, author: c.Author, authorID: c.AuthorID,
			adf: adf, body: c.BodyText, created: c.CreatedAt, updated: c.UpdatedAt,
		})
	}
	if len(have) != len(want) {
		return false, nil
	}
	sort.Slice(have, func(i, j int) bool { return have[i].id < have[j].id })
	sort.Slice(want, func(i, j int) bool { return want[i].id < want[j].id })
	for i := range have {
		if have[i] != want[i] {
			return false, nil
		}
	}
	return true, nil
}

// writeFTS rebuilds one row of the contentless index. Contentless FTS5 has no
// update path, so delete-then-insert is the whole story.
func writeFTS(tx *sql.Tx, rowid int64, title, body, comments string) error {
	if _, err := tx.Exec(`DELETE FROM items_fts WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO items_fts (rowid, title, body_text, comments_text) VALUES (?,?,?,?)`,
		rowid, title, body, comments)
	return err
}

// DeleteItems removes items whose keys have left the source's scope and records
// a tombstone so `delta` can report the deletion to a client that missed it.
func (db *DB) DeleteItems(ctx context.Context, sourceID string, keys []string) (int, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	deleted := 0
	at := Now()
	err := db.write(ctx, func(tx *sql.Tx) error {
		for _, key := range keys {
			var id string
			var rowid int64
			err := tx.QueryRow(`SELECT id, rowid FROM items WHERE source_id = ? AND key = ?`, sourceID, key).Scan(&id, &rowid)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM items_fts WHERE rowid = ?`, rowid); err != nil {
				return err
			}
			// issues, comments, attachments, changelog and links go with it via
			// ON DELETE CASCADE, which is why foreign_keys=ON is not optional.
			if _, err := tx.Exec(`DELETE FROM items WHERE id = ?`, id); err != nil {
				return err
			}
			if _, err := tx.Exec(`
				INSERT INTO deleted_items (key, source_id, deleted_at) VALUES (?,?,?)
				ON CONFLICT(key) DO UPDATE SET deleted_at = excluded.deleted_at`,
				key, sourceID, at); err != nil {
				return err
			}
			deleted++
		}
		if deleted == 0 {
			return nil
		}
		// ponytail: tombstones expire on a fixed 90-day window. A client offline
		// longer than that needs a full bootstrap anyway, which it gets from the
		// version mismatch.
		if _, err := tx.Exec(`DELETE FROM deleted_items WHERE deleted_at < datetime('now', '-90 days')`); err != nil {
			return err
		}
		return bumpVersion(tx, sourceID, db.schemaVersion)
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

// bumpVersion advances the counter the read API turns into an ETag. It moves on
// every write that changed mirrored rows and on nothing else, so an unchanged
// sync leaves it alone.
func bumpVersion(tx *sql.Tx, sourceID string, schemaVersion int) error {
	_, err := tx.Exec(`
		INSERT INTO sync_state (source_id, version, schema_version) VALUES (?, 1, ?)
		ON CONFLICT(source_id) DO UPDATE SET version = sync_state.version + 1`,
		sourceID, schemaVersion)
	return err
}

// SyncState is the per-source sync bookkeeping.
type SyncState struct {
	SourceID  string `json:"source_id"`
	Watermark string `json:"watermark"`
	// SyncedAt is the last run that finished without an error — the only field
	// here that means "the mirror is fresh". A watermark stalls on its own
	// whenever the project is simply quiet.
	SyncedAt       *string `json:"synced_at"`
	Version        int64   `json:"version"`
	LastFullSyncAt *string `json:"last_full_sync_at"`
	LastError      *string `json:"last_error"`
	SchemaVersion  int     `json:"schema_version"`
	// FirstSyncAt is the first successful sync for this source (retention).
	FirstSyncAt *string `json:"first_sync_at,omitempty"`
	// SyncCount is the number of successful sync runs (retention).
	SyncCount int64 `json:"sync_count"`
	// LastNotifiedAt is the OS-notification watermark. Independent of
	// feed_reads: delivering a desktop alert must not mark the feed read.
	LastNotifiedAt *string `json:"last_notified_at,omitempty"`
}

// SyncState reads the state for one source. A source that has never synced
// returns a zero state, not an error.
func (db *DB) SyncState(ctx context.Context, sourceID string) (SyncState, error) {
	s := SyncState{SourceID: sourceID, SchemaVersion: db.schemaVersion}
	var wm *string
	err := db.sql.QueryRowContext(ctx, `
		SELECT st.watermark, st.version, st.last_full_sync_at, st.last_error,
		       st.schema_version, src.synced_at,
		       st.first_sync_at, st.sync_count, st.last_notified_at
		FROM sync_state st LEFT JOIN sources src ON src.id = st.source_id
		WHERE st.source_id = ?`, sourceID).
		Scan(&wm, &s.Version, &s.LastFullSyncAt, &s.LastError, &s.SchemaVersion, &s.SyncedAt,
			&s.FirstSyncAt, &s.SyncCount, &s.LastNotifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if wm != nil {
		s.Watermark = *wm
	}
	return s, nil
}

// SyncResult is what a finished sync run reports.
type SyncResult struct {
	Watermark string // ignored when empty or not greater than the stored one
	FullSync  bool   // stamps last_full_sync_at
	Err       error  // recorded as last_error; nil clears it
}

// RecordSync stores the run's bookkeeping. It does not bump `version`: a run
// that changed no rows must leave the ETag alone. A successful run advances
// first_sync_at (once) and sync_count.
func (db *DB) RecordSync(ctx context.Context, sourceID string, r SyncResult) error {
	var errText, fullAt, firstAt any
	inc := 0
	if r.Err != nil {
		errText = r.Err.Error()
	} else {
		inc = 1
		firstAt = Now()
	}
	if r.FullSync {
		fullAt = Now()
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`
			INSERT INTO sync_state (source_id, watermark, last_full_sync_at, last_error, schema_version,
			                       first_sync_at, sync_count)
			VALUES (?,?,?,?,?,?,?)
			ON CONFLICT(source_id) DO UPDATE SET
				watermark = CASE
					WHEN excluded.watermark IS NOT NULL
					 AND excluded.watermark > COALESCE(sync_state.watermark, '')
					THEN excluded.watermark ELSE sync_state.watermark END,
				last_full_sync_at = COALESCE(excluded.last_full_sync_at, sync_state.last_full_sync_at),
				last_error = excluded.last_error,
				schema_version = excluded.schema_version,
				first_sync_at = CASE
					WHEN excluded.first_sync_at IS NOT NULL AND sync_state.first_sync_at IS NULL
					THEN excluded.first_sync_at ELSE sync_state.first_sync_at END,
				sync_count = sync_state.sync_count + excluded.sync_count`,
			sourceID, nz(r.Watermark), fullAt, errText, db.schemaVersion, firstAt, inc); err != nil {
			return err
		}
		if r.Err != nil {
			return nil
		}
		_, err := tx.Exec(`UPDATE sources SET synced_at = ? WHERE id = ?`, Now(), sourceID)
		return err
	})
}

// SetLastNotifiedAt advances the OS-notification watermark. Never touches feed_reads.
func (db *DB) SetLastNotifiedAt(ctx context.Context, sourceID, at string) error {
	if at == "" {
		at = Now()
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO sync_state (source_id, last_notified_at, schema_version, version)
			VALUES (?, ?, ?, 0)
			ON CONFLICT(source_id) DO UPDATE SET last_notified_at = excluded.last_notified_at`,
			sourceID, at, db.schemaVersion)
		return err
	})
}

// SavedView is a user's stored filter set. Personal state is the only thing in
// this database a user would miss, so it is also the only thing `gadak export`
// has to dump (Constitution Article 1).
type SavedView struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

func (db *DB) SavedViews(ctx context.Context) ([]SavedView, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT id, name, config, created_at, updated_at FROM saved_views ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SavedView{}
	for rows.Next() {
		var v SavedView
		var cfg string
		var created, updated *string
		if err := rows.Scan(&v.ID, &v.Name, &cfg, &created, &updated); err != nil {
			return nil, err
		}
		v.Config = json.RawMessage(cfg)
		if created != nil {
			v.CreatedAt = *created
		}
		if updated != nil {
			v.UpdatedAt = *updated
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// PutSavedView inserts or replaces a view. The caller owns id generation.
func (db *DB) PutSavedView(ctx context.Context, v SavedView) error {
	if v.ID == "" || v.Name == "" {
		return errors.New("saved view needs an id and a name")
	}
	if len(v.Config) == 0 {
		v.Config = json.RawMessage("{}")
	}
	now := Now()
	if v.CreatedAt == "" {
		v.CreatedAt = now
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO saved_views (id, name, config, created_at, updated_at) VALUES (?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name, config = excluded.config, updated_at = excluded.updated_at`,
			v.ID, v.Name, string(v.Config), v.CreatedAt, now)
		return err
	})
}

func (db *DB) DeleteSavedView(ctx context.Context, id string) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM saved_views WHERE id = ?`, id)
		return err
	})
}

func (db *DB) Watches(ctx context.Context) ([]string, error)   { return db.keys(ctx, "watches") }
func (db *DB) Favorites(ctx context.Context) ([]string, error) { return db.keys(ctx, "favorites") }

// SetWatch adds or removes a watched issue key.
func (db *DB) SetWatch(ctx context.Context, key string, on bool) error {
	return db.setKey(ctx, "watches", key, on)
}

// SetFavorite adds or removes a favorite issue key.
func (db *DB) SetFavorite(ctx context.Context, key string, on bool) error {
	return db.setKey(ctx, "favorites", key, on)
}

func (db *DB) keys(ctx context.Context, table string) ([]string, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT key FROM `+table+` ORDER BY created_at, key`)
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

func (db *DB) setKey(ctx context.Context, table, key string, on bool) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		if !on {
			_, err := tx.Exec(`DELETE FROM `+table+` WHERE key = ?`, key)
			return err
		}
		_, err := tx.Exec(`INSERT OR IGNORE INTO `+table+` (key, created_at) VALUES (?, ?)`, key, Now())
		return err
	})
}

// FreshenSyncClock stamps every sync timestamp as now. It exists for throwaway
// fixtures — `gadak demo` and the demo recordings — where the snapshot's real age
// would surface as a stale-sync warning about data that is deliberately frozen.
// Never call this on a mirror that syncs for real: it would hide a stalled sync.
func (db *DB) FreshenSyncClock(ctx context.Context) error {
	now := Now()
	_, err := db.sql.ExecContext(ctx, `
		UPDATE sync_state SET watermark = ?, last_full_sync_at = ?, last_error = NULL;
		UPDATE sources    SET synced_at = ?;
		UPDATE items      SET synced_at = ?;`, now, now, now, now)
	return err
}
