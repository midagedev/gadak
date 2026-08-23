package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	err := db.mutate(ctx, func(tx *sql.Tx) ([]string, error) {
		// Reset per attempt: write() retries SQLITE_BUSY, which re-runs this
		// callback from the start, and a counter captured out here would count
		// the abandoned attempt's rows too (GDK-305).
		changed = 0
		sources := map[string]bool{}
		for _, r := range b.Records {
			ok, err := upsertRecord(tx, b, r)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", r.Item.Key, err)
			}
			if ok {
				changed++
				sources[r.Item.SourceID] = true
			}
		}
		if b.Force {
			for id := range sources {
				// Write-through (SyncIssue Force) is a successful origin
				// round-trip that does not call RecordSync — a single issue
				// must not become the watermark (internal/sync/one.go).
				// Clearing last_error here is that path's counterpart of
				// RecordSync's "nil clears it" (GDK-453).
				if _, err := tx.Exec(`UPDATE sync_state SET last_error = NULL WHERE source_id = ?`, id); err != nil {
					return nil, err
				}
			}
		}
		// Parent chains can resolve only after later pages arrive. Recompute
		// epic_key for the batch plus children/grandchildren whose parent
		// chain this page may have completed (GDK-755). A full-table UPDATE
		// still runs once at the end of a full sync (RecomputeEpicKeys).
		batchKeys := make([]string, 0, len(b.Records))
		for _, r := range b.Records {
			if r.Item.Key != "" {
				batchKeys = append(batchKeys, r.Item.Key)
			}
		}
		if err := recomputeEpicKeys(tx, batchKeys); err != nil {
			return nil, err
		}
		if err := cacheStatusCatalog(tx, b); err != nil {
			return nil, err
		}
		if err := cacheUserCatalog(tx, b); err != nil {
			return nil, err
		}
		return mapKeys(sources), nil
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

// epicKeySelect is the two-hop parent walk: parent if it is hierarchy_level
// 1, else grandparent, else NULL. Shared by the scoped UPDATE and the
// full-table sweep so the two paths cannot drift.
const epicKeySelect = `
			SELECT CASE
				WHEN p.hierarchy_level = 1 THEN p.key
				WHEN gp.hierarchy_level = 1 THEN gp.key
			END
			FROM issues p
			LEFT JOIN issues gp ON gp.key = p.parent_key
			WHERE p.key = issues.parent_key
`

// recomputeEpicKeys sets issues.epic_key to the nearest hierarchy_level==1
// ancestor via parent_key (direct parent, else grandparent). NULL when none.
//
// keys, when non-empty, limits the UPDATE to those keys plus issues whose
// parent or grandparent is in that set — the reverse-arriving child and the
// children of a parent that just landed. An empty/nil keys recomputes the
// whole table: full-sync final pass (RecomputeEpicKeys) and the v11
// backfill shape in schema.go.
func recomputeEpicKeys(tx *sql.Tx, keys []string) error {
	if len(keys) == 0 {
		_, err := tx.Exec(`UPDATE issues SET epic_key = (` + epicKeySelect + `)`)
		return err
	}
	affected, err := affectedEpicKeys(tx, keys)
	if err != nil {
		return err
	}
	if len(affected) == 0 {
		return nil
	}
	payload, err := json.Marshal(affected)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		UPDATE issues SET epic_key = (`+epicKeySelect+`)
		WHERE key IN (SELECT value FROM json_each(?))`, string(payload))
	return err
}

// affectedEpicKeys is the batch plus its children and grandchildren. The
// batch keys themselves come from json_each so a just-deleted epic still
// selects the rows that pointed at it.
func affectedEpicKeys(tx *sql.Tx, batchKeys []string) ([]string, error) {
	if len(batchKeys) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(batchKeys)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(`
		WITH batch AS (SELECT value AS k FROM json_each(?))
		SELECT k FROM batch
		UNION
		SELECT key FROM issues WHERE parent_key IN (SELECT k FROM batch)
		UNION
		SELECT key FROM issues WHERE parent_key IN (
			SELECT key FROM issues WHERE parent_key IN (SELECT k FROM batch)
		)`, string(payload))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		if key != "" {
			out = append(out, key)
		}
	}
	return out, rows.Err()
}

// RecomputeEpicKeys rewrites issues.epic_key for the whole table. Full sync
// calls this once after every page has been upserted; per-batch upserts
// only recompute the batch and its parent-chain dependents (GDK-755).
func (db *DB) RecomputeEpicKeys(ctx context.Context) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		return recomputeEpicKeys(tx, nil)
	})
}

// cacheStatusCatalog persists the batch's id -> category map (the origin's
// status catalog, fetched by every sync pass) into status_catalog. Without
// it the map evaporates when Derive returns, and the changelog's bare status
// ids are resolvable only for statuses that are some issue's *current*
// status — a claimed-then-finished issue's in-progress id goes dark and its
// wait/progress spans compute as absent (GDK-591). Origin reference data,
// not time-in-status values: a wipe costs one re-sync. All records in a
// batch come from one source, so the first record's id scopes the rows.
func cacheStatusCatalog(tx *sql.Tx, b Batch) error {
	if len(b.Categories) == 0 || len(b.Records) == 0 {
		return nil
	}
	src := b.Records[0].Item.SourceID
	for id, cat := range b.Categories {
		if id == "" || cat == "" {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO status_catalog (source_id, status_id, category) VALUES (?,?,?)
			ON CONFLICT(source_id, status_id) DO UPDATE SET category = excluded.category`,
			src, id, cat); err != nil {
			return err
		}
	}
	return nil
}

// cacheUserCatalog merges the records' collected accounts into the users
// table (GDK-590). It runs on every batch — including ones whose rows were
// skipped as unchanged — because the catalog is fed by payloads the change
// detector never sees (a comment's author rides on an unchanged issue's
// child fetch, and every batch re-reads the users the records mention).
// Merge, not replace: an account whose payload this time carries no name or
// account_type (a dev-panel actor, a trimmed listing) keeps what the catalog
// already knows. Empty account ids are skipped — there is nothing to key on.
func cacheUserCatalog(tx *sql.Tx, b Batch) error {
	for _, r := range b.Records {
		for _, u := range r.Users {
			if u.AccountID == "" {
				continue
			}
			if _, err := tx.Exec(`
				INSERT INTO users (source_id, account_id, name, email, account_type) VALUES (?,?,?,?,?)
				ON CONFLICT(source_id, account_id) DO UPDATE SET
				  name = CASE WHEN excluded.name != '' THEN excluded.name ELSE users.name END,
				  email = CASE WHEN excluded.email != '' THEN excluded.email ELSE users.email END,
				  account_type = CASE WHEN excluded.account_type != '' THEN excluded.account_type ELSE users.account_type END`,
				r.Item.SourceID, u.AccountID, u.Name, u.Email, u.AccountType); err != nil {
				return err
			}
		}
	}
	return nil
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
			status, status_id, status_category, priority, priority_id, priority_rank,
			assignee, assignee_id, assignee_email, reporter, reporter_id, reporter_email, parent_key,
			labels, components, fix_versions, fix_version_ids, affects_versions, environment_text,
			duedate, resolution, resolution_id, created_at, updated_at,
			status_changed_at, resolved_at, reopen_count, reopened_at, reopen_reason,
			assignee_changed_at, comment_count, description_adf, custom, raw, cloned_from,
			hierarchy_level, sprint_id, sprint_name, sprint_state,
			security_level_id, security_level)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		it.ID, it.Key, nz(is.ProjectKey), nz(is.IssueType), nz(is.IssueTypeID),
		nz(is.Status), nz(is.StatusID), nz(is.StatusCategory), nz(is.Priority), is.PriorityID, d.PriorityRank,
		nz(is.Assignee), nz(is.AssigneeID), nz(is.AssigneeEmail), nz(is.Reporter),
		nz(is.ReporterID), nz(is.ReporterEmail), nz(is.ParentKey),
		jsonArray(is.Labels), jsonArray(is.Components), jsonArray(is.FixVersions), jsonArray(is.FixVersionIDs),
		jsonArray(is.AffectsVersions), nz(is.EnvironmentText),
		nz(is.Duedate), nz(is.Resolution), is.ResolutionID, nz(it.CreatedAt), nz(it.UpdatedAt),
		d.StatusChangedAt, d.ResolvedAt, d.ReopenCount, d.ReopenedAt, d.ReopenReason,
		d.AssigneeChangedAt, d.CommentCount, jsonRaw(is.DescriptionADF),
		jsonObject(is.Custom), jsonRaw(is.Raw), d.ClonedFrom,
		is.HierarchyLevel, nzInt64(is.SprintID), nz(is.SprintName), nz(is.SprintState),
		nz(is.SecurityLevelID), nz(is.SecurityLevel),
	); err != nil {
		return false, err
	}

	// Child lists arrive complete, so replacing them is both correct and the
	// only way a removed comment or link leaves the mirror. dev_links is
	// skipped when the origin answer was not observed (GDK-536 / GDK-580):
	// a fetch error cannot construct DevLinksUpdate, so existing rows stay.
	// A dev_links answer enumerates pull requests only — it is built from
	// the summary's pullrequest count — so its rewrite touches the
	// pullrequest rows and leaves the deployment/build rows `gadak dev
	// deploy`/`dev build` wrote (GDK-592; their detail vocabulary is
	// uncaptured, so no origin answer can enumerate them yet).
	childTables := []string{"comments", "attachments", "changelog", "links"}
	if r.DevLinks != nil {
		if _, err := tx.Exec(`DELETE FROM dev_links WHERE item_id = ? AND kind = 'pullrequest'`, it.ID); err != nil {
			return false, err
		}
	}
	for _, t := range childTables {
		if _, err := tx.Exec(`DELETE FROM `+t+` WHERE item_id = ?`, it.ID); err != nil {
			return false, err
		}
	}
	bodies := make([]string, 0, len(r.Comments))
	for _, c := range r.Comments {
		if err := insertComment(tx, it.ID, c); err != nil {
			return false, err
		}
		if c.BodyText != "" {
			bodies = append(bodies, c.BodyText)
		}
	}
	for _, a := range r.Attachments {
		if _, err := tx.Exec(`
			INSERT INTO attachments (id, item_id, external_id, filename, mime_type, size, author, author_id, created_at, url)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			a.ID, it.ID, nz(a.ExternalID), nz(a.Filename), nz(a.MimeType), a.Size,
			nz(a.Author), nz(a.AuthorID), nz(a.CreatedAt), nz(a.URL),
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
	if r.DevLinks != nil {
		if err := insertDevLinks(tx, it.ID, r.DevLinks.Links); err != nil {
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

// ReplaceProjectVersions upserts one project's version catalog and deletes
// rows for that project whose id is no longer in the catalog. The catalog is
// the origin; this table is a cache (GDK-532). An empty list clears the
// project. Rows with an empty id are skipped (no join key).
func (db *DB) ReplaceProjectVersions(ctx context.Context, projectKey string, rows []VersionRow) error {
	if projectKey == "" {
		return nil
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		keep := make([]string, 0, len(rows))
		for _, r := range rows {
			if r.ID == "" {
				continue
			}
			keep = append(keep, r.ID)
			if _, err := tx.Exec(`
				INSERT INTO versions (id, project_key, name, released, archived, release_date)
				VALUES (?,?,?,?,?,?)
				ON CONFLICT(id) DO UPDATE SET
					project_key = excluded.project_key,
					name = excluded.name,
					released = excluded.released,
					archived = excluded.archived,
					release_date = excluded.release_date`,
				r.ID, projectKey, r.Name, boolInt(r.Released), boolInt(r.Archived), nz(r.ReleaseDate),
			); err != nil {
				return fmt.Errorf("version %s: %w", r.ID, err)
			}
		}
		if len(keep) == 0 {
			_, err := tx.Exec(`DELETE FROM versions WHERE project_key = ?`, projectKey)
			return err
		}
		keepJSON, err := json.Marshal(keep)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM versions WHERE project_key = ? AND id NOT IN (SELECT value FROM json_each(?))`,
			projectKey, string(keepJSON))
		return err
	})
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
	err = db.mutate(ctx, func(tx *sql.Tx) ([]string, error) {
		srows, err := tx.QueryContext(ctx, `SELECT key FROM spaces WHERE source_id = ?`, sourceID)
		if err != nil {
			return nil, err
		}
		var drop []string
		for srows.Next() {
			var key string
			if err := srows.Scan(&key); err != nil {
				srows.Close()
				return nil, err
			}
			if _, ok := keep[key]; !ok {
				drop = append(drop, key)
			}
		}
		if err := srows.Err(); err != nil {
			srows.Close()
			return nil, err
		}
		srows.Close()
		for _, key := range drop {
			if _, err := tx.Exec(`DELETE FROM spaces WHERE source_id = ? AND key = ?`, sourceID, key); err != nil {
				return nil, err
			}
		}
		if len(drop) == 0 || n > 0 {
			// DeleteItems already bumped version when pages were removed.
			return nil, nil
		}
		return []string{sourceID}, nil
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
	err := db.mutate(ctx, func(tx *sql.Tx) ([]string, error) {
		// Reset per attempt: write() retries SQLITE_BUSY, which re-runs this
		// callback from the start, and a counter captured out here would count
		// the abandoned attempt's rows too (GDK-305).
		changed = 0
		// known project keys once per batch for bare-text issue-key filtering
		known, err := loadKnownProjectKeys(tx)
		if err != nil {
			return nil, err
		}
		sources := map[string]bool{}
		for _, r := range records {
			ok, err := upsertPageRecord(tx, r, known)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", r.Item.Key, err)
			}
			if ok {
				changed++
				sources[r.Item.SourceID] = true
			}
		}
		return mapKeys(sources), nil
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
		if err := insertComment(tx, it.ID, c); err != nil {
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
// update path, so delete-then-insert is the whole story. The fourth column is
// the CJK bigram text (GDK-259): it is what mid-compound Korean matches.
func writeFTS(tx *sql.Tx, rowid int64, title, body, comments string) error {
	if _, err := tx.Exec(`DELETE FROM items_fts WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	_, err := tx.Exec(
		`INSERT INTO items_fts (rowid, title, body_text, comments_text, cjk_bigram) VALUES (?,?,?,?,?)`,
		rowid, title, body, comments, FTSCJKBigramColumn(title, body, comments))
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
	err := db.mutate(ctx, func(tx *sql.Tx) ([]string, error) {
		// Reset per attempt: write() retries SQLITE_BUSY, which re-runs this
		// callback from the start, and a counter captured out here would count
		// the abandoned attempt's rows too (GDK-305).
		deleted = 0
		for _, key := range keys {
			var id string
			var rowid int64
			err := tx.QueryRow(`SELECT id, rowid FROM items WHERE source_id = ? AND key = ?`, sourceID, key).Scan(&id, &rowid)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if _, err := tx.Exec(`DELETE FROM items_fts WHERE rowid = ?`, rowid); err != nil {
				return nil, err
			}
			// issues, comments, attachments, changelog and links go with it via
			// ON DELETE CASCADE, which is why foreign_keys=ON is not optional.
			if _, err := tx.Exec(`DELETE FROM items WHERE id = ?`, id); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(`
				INSERT INTO deleted_items (key, source_id, deleted_at) VALUES (?,?,?)
				ON CONFLICT(key) DO UPDATE SET deleted_at = excluded.deleted_at`,
				key, sourceID, at); err != nil {
				return nil, err
			}
			deleted++
		}
		if deleted == 0 {
			return nil, nil
		}
		// ponytail: tombstones expire on a fixed 90-day window. A client offline
		// longer than that needs a full bootstrap anyway, which it gets from the
		// version mismatch.
		if _, err := tx.Exec(`DELETE FROM deleted_items WHERE deleted_at < datetime('now', '-90 days')`); err != nil {
			return nil, err
		}
		// Children still carry parent_key of a deleted epic; scoped
		// recompute walks those keys (json_each, not the missing row).
		if err := recomputeEpicKeys(tx, keys); err != nil {
			return nil, err
		}
		return []string{sourceID}, nil
	})
	if err != nil {
		return 0, err
	}
	// Same pass as tombstone expiry: drop visit/search rows older than the
	// raw window. A missing/unattached local.db must not fail the deletion.
	if deleted > 0 {
		if err := db.PruneLocalHistory(ctx); err != nil {
			log.Printf("store: prune local history: %v", err)
		}
	}
	return deleted, nil
}

// PurgeIssueIDsOutsideNamespace deletes one source's issue rows whose item id
// does not start with ns+":". Upgrade path for GDK-241: a standalone mirror
// written before ids were namespaced holds `jira:N` rows whose keys the next
// sync re-inserts as `standalone-jira:N` — same (source_id, key), new id —
// which UNIQUE(source_id, key) rejects. The rows re-mirror immediately under
// the new namespace, so no tombstones are written. Children go via
// ON DELETE CASCADE; items_fts is contentless and needs the explicit delete.
func (db *DB) PurgeIssueIDsOutsideNamespace(ctx context.Context, sourceID, ns string) (int, error) {
	return db.purgeIDsOutsideNamespace(ctx, sourceID, ns, "issue")
}

// PurgePageIDsOutsideNamespace is the wiki sibling of
// PurgeIssueIDsOutsideNamespace (GDK-344). A standalone page's key is its
// numeric external id, so a pre-namespace `confluence:N` row and the
// namespaced `standalone-confluence:N` insert share UNIQUE(source_id, key).
func (db *DB) PurgePageIDsOutsideNamespace(ctx context.Context, sourceID, ns string) (int, error) {
	return db.purgeIDsOutsideNamespace(ctx, sourceID, ns, "page")
}

func (db *DB) purgeIDsOutsideNamespace(ctx context.Context, sourceID, ns, kind string) (int, error) {
	purged := 0
	err := db.mutate(ctx, func(tx *sql.Tx) ([]string, error) {
		purged = 0
		rows, err := tx.Query(`SELECT id, rowid FROM items WHERE source_id = ? AND kind = ? AND id NOT LIKE ? ESCAPE '\'`,
			sourceID, kind, likeEscape(ns+":")+"%")
		if err != nil {
			return nil, err
		}
		type row struct {
			id    string
			rowid int64
		}
		var stale []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.rowid); err != nil {
				rows.Close()
				return nil, err
			}
			stale = append(stale, r)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		for _, r := range stale {
			if _, err := tx.Exec(`DELETE FROM items_fts WHERE rowid = ?`, r.rowid); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(`DELETE FROM items WHERE id = ?`, r.id); err != nil {
				return nil, err
			}
			purged++
		}
		if purged == 0 {
			return nil, nil
		}
		return []string{sourceID}, nil
	})
	return purged, err
}

// What conversion drops now lives in origin_scope.go: the statements are
// derived from a classification every table must appear in, because the literal
// list that used to be here is what let four later tables opt out of it
// silently (GDK-418).

// insertComment writes one comments row. visibility_type/value are NOT NULL
// and store the empty string (unrestricted); jsd_public is NULL when the
// origin omitted the marker.
func insertComment(tx *sql.Tx, itemID string, c Comment) error {
	_, err := tx.Exec(`
		INSERT INTO comments (id, item_id, external_id, author, author_id,
		                      body_adf, body_text, created_at, updated_at,
		                      visibility_type, visibility_value, jsd_public)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, itemID, nz(c.ExternalID), nz(c.Author), nz(c.AuthorID),
		jsonRaw(c.BodyADF), nz(c.BodyText), nz(c.CreatedAt), nz(c.UpdatedAt),
		c.VisibilityType, c.VisibilityValue, jsdPublicSQL(c.JsdPublic),
	)
	return err
}

func jsdPublicSQL(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return 1
	}
	return 0
}

func jsdPublicFromSQL(n sql.NullInt64) *bool {
	if !n.Valid {
		return nil
	}
	b := n.Int64 != 0
	return &b
}

// likeEscape escapes LIKE metacharacters so a namespace prefix matches
// literally (ESCAPE '\').
func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// bumpVersion advances the counter the read API turns into an ETag. It moves on
// every write that changed mirrored rows and on nothing else, so an unchanged
// sync leaves it alone. Called only from mutate (GDK-579).
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
	// SchemaVersion is the mirror's migration level (PRAGMA user_version).
	// Diagnostic surfaces (status --json, MCP gadak_status, doctor) all
	// publish this field. The sync_state.schema_version column can lag
	// when Open migrates after the last sync wrote the row (GDK-526), so
	// SyncState overwrites the scanned column with the live PRAGMA rather
	// than introducing a second JSON name for the same fact.
	SchemaVersion int `json:"schema_version"`
	// SchemaVersionRow is the stored column. json omitted so it cannot
	// collide with schema_version on the wire; doctor uses it to name a lag.
	SchemaVersionRow int `json:"-"`
	// FirstSyncAt is the first successful sync for this source (retention).
	FirstSyncAt *string `json:"first_sync_at,omitempty"`
	// SyncCount is the number of successful sync runs (retention).
	SyncCount int64 `json:"sync_count"`
	// LastNotifiedAt is the OS-notification watermark. Independent of
	// feed_reads: delivering a desktop alert must not mark the feed read.
	LastNotifiedAt *string `json:"last_notified_at,omitempty"`
	// Locale is the origin locale the jira source's display names were
	// fetched under (GDK-597). NULL (pre-v35 mirror) reads as "" = English.
	Locale string `json:"locale,omitempty"`
}

// SyncState reads the state for one source. A source that has never synced
// returns a zero state, not an error.
func (db *DB) SyncState(ctx context.Context, sourceID string) (SyncState, error) {
	s := SyncState{SourceID: sourceID, SchemaVersion: db.schemaVersion}
	var wm *string
	var loc *string
	err := db.sql.QueryRowContext(ctx, `
		SELECT st.watermark, st.version, st.last_full_sync_at, st.last_error,
		       st.schema_version, src.synced_at,
		       st.first_sync_at, st.sync_count, st.last_notified_at, st.locale
		FROM sync_state st LEFT JOIN sources src ON src.id = st.source_id
		WHERE st.source_id = ?`, sourceID).
		Scan(&wm, &s.Version, &s.LastFullSyncAt, &s.LastError, &s.SchemaVersionRow, &s.SyncedAt,
			&s.FirstSyncAt, &s.SyncCount, &s.LastNotifiedAt, &loc)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if wm != nil {
		s.Watermark = *wm
	}
	if loc != nil {
		s.Locale = *loc
	}
	// The column is the schema at last sync/migration write; PRAGMA
	// user_version is the mirror. Publishing the column under
	// schema_version made status and doctor disagree (GDK-526).
	s.SchemaVersion = db.schemaVersion
	return s, nil
}

// syncStateHasIssueActivity is true when this source has actually run (or
// failed) as an issue origin. An empty jira row on a Linear-only mirror
// must not win status --json.
func syncStateHasIssueActivity(s SyncState) bool {
	if s.Watermark != "" || s.SyncCount > 0 {
		return true
	}
	if s.LastError != nil && *s.LastError != "" {
		return true
	}
	return s.SyncedAt != nil && *s.SyncedAt != ""
}

// IssueSyncState is the issue-origin freshness row status --json and
// gadak_status publish as top-level watermark / sync_count / last_error.
// Jira wins when it has activity so dual-source workspaces keep the
// historical shape; Linear wins when it is the only issue source that has
// run.
func (db *DB) IssueSyncState(ctx context.Context) (SyncState, error) {
	jira, err := db.SyncState(ctx, "jira")
	if err != nil {
		return jira, err
	}
	linear, err := db.SyncState(ctx, "linear")
	if err != nil {
		return jira, err
	}
	if !syncStateHasIssueActivity(jira) && syncStateHasIssueActivity(linear) {
		return linear, nil
	}
	return jira, nil
}

// SyncResult is what a finished sync run reports.
type SyncResult struct {
	Watermark string // ignored when empty or not greater than the stored one
	FullSync  bool   // stamps last_full_sync_at
	Err       error  // recorded as last_error; nil clears it
	// Locale records the origin locale this pass fetched display names
	// under (GDK-597). Empty leaves the stored marker alone — an error
	// path or a source that does not localize must not clobber it.
	Locale string
}

// RecordSync stores the run's bookkeeping. It does not bump `version`: a run
// that changed no rows must leave the ETag alone. A successful run advances
// first_sync_at (once) and sync_count.
func (db *DB) RecordSync(ctx context.Context, sourceID string, r SyncResult) error {
	if sqliteBusy(r.Err) {
		// GDK-754: last_error is itself a write(). A holder that just
		// failed the caller will fail this too, stacking a second
		// busy_timeout cycle (~10s with two attempts at 5s). Skip; the
		// returned error already carries the holder hint from write() /
		// WithBusyHint. Production busy_timeout stays 5000ms.
		return nil
	}
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
			                       first_sync_at, sync_count, locale)
			VALUES (?,?,?,?,?,?,?,?)
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
				sync_count = sync_state.sync_count + excluded.sync_count,
				locale = COALESCE(excluded.locale, sync_state.locale)`,
			sourceID, nz(r.Watermark), fullAt, errText, db.schemaVersion, firstAt, inc, nz(r.Locale)); err != nil {
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
// has to dump (Constitution Article 1). Since GDK-105 it lives in local.db,
// which survives `rm gadak.db`.
type SavedView struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

func (db *DB) SavedViews(ctx context.Context) ([]SavedView, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT id, name, config, created_at, updated_at FROM local.saved_views ORDER BY name`)
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
			INSERT INTO local.saved_views (id, name, config, created_at, updated_at) VALUES (?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name, config = excluded.config, updated_at = excluded.updated_at`,
			v.ID, v.Name, string(v.Config), v.CreatedAt, now)
		return err
	})
}

func (db *DB) DeleteSavedView(ctx context.Context, id string) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM local.saved_views WHERE id = ?`, id)
		return err
	})
}

// freshViewID backs AbsorbViews' id-collision guard. Same shape as the server
// package's newID (16 hex chars) — personal ids carry a "p-" prefix, so a
// collision only happens when the same browser row is re-absorbed under a new
// name; the row then keeps its identity elsewhere, this one starts fresh.
func freshViewID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return Now() // unique enough for a local single-user store
	}
	return hex.EncodeToString(b)
}

// AbsorbViews merges browser-local (localStorage) views into local.saved_views
// — the GDK-437 promotion day must not look like data loss. Same rules as
// AbsorbRecents: the server is already the owner, so an incoming view whose
// name exists there is dropped (server row wins), the rest are inserted.
// Reads order by name, so "server rows in front" reduces to that conflict
// rule. Kept ids stay stable — the client hides its absorbed rows by id — and
// an id that already exists gets a fresh one so an insert can never overwrite
// a server row (PutSavedView's upsert is deliberately not used here).
func (db *DB) AbsorbViews(ctx context.Context, incoming []SavedView) error {
	if len(incoming) == 0 {
		return nil
	}
	return db.write(ctx, func(tx *sql.Tx) error {
		for _, v := range incoming {
			v.Name = strings.TrimSpace(v.Name)
			if v.ID == "" || v.Name == "" {
				continue
			}
			var one int
			err := tx.QueryRowContext(ctx, `SELECT 1 FROM local.saved_views WHERE name = ? LIMIT 1`, v.Name).Scan(&one)
			if err == nil {
				continue // a server row already owns this name
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			id := v.ID
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM local.saved_views WHERE id = ? LIMIT 1`, id).Scan(&one)
			if err == nil {
				id = freshViewID()
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			cfg := v.Config
			if len(cfg) == 0 {
				cfg = json.RawMessage("{}")
			}
			created := v.CreatedAt
			if created == "" {
				created = Now()
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO local.saved_views (id, name, config, created_at, updated_at) VALUES (?,?,?,?,?)`,
				id, v.Name, string(cfg), created, Now()); err != nil {
				return err
			}
		}
		return nil
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

// keys/setKey address local.<table>: watches and favorites are personal state
// and live in local.db (GDK-105). The mirror-side tables of the same name are
// frozen leftovers of the schemaV26 copy — keep new readers on local.*.
func (db *DB) keys(ctx context.Context, table string) ([]string, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT key FROM local.`+table+` ORDER BY created_at, key`)
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
			_, err := tx.Exec(`DELETE FROM local.`+table+` WHERE key = ?`, key)
			return err
		}
		_, err := tx.Exec(`INSERT OR IGNORE INTO local.`+table+` (key, created_at) VALUES (?, ?)`, key, Now())
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

// devKind defaults an empty dev-link kind to pullrequest — the only kind v29 stores.
func devKind(k string) string {
	if k == "" {
		return "pullrequest"
	}
	return k
}

// ReplaceDevLinks swaps one issue's dev_links rows for a successful origin
// answer — the mirror-refresh half of a dev-link write-through (GDK-497).
// Never a source of truth: the same rows are rebuilt by any later sync.
// update is a complete pull-request answer (empty Links drains the PR rows
// — GDK-580); deployment/build rows are not part of a dev-status answer
// and survive, written only by `gadak dev deploy`/`dev build` (GDK-592).
func (db *DB) ReplaceDevLinks(ctx context.Context, key string, update DevLinksUpdate) error {
	return db.mutate(ctx, func(tx *sql.Tx) ([]string, error) {
		var itemID, sourceID string
		if err := tx.QueryRow(`
			SELECT i.item_id, it.source_id
			FROM issues i JOIN items it ON it.id = i.item_id
			WHERE i.key = ?`, key).Scan(&itemID, &sourceID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`DELETE FROM dev_links WHERE item_id = ? AND kind = 'pullrequest'`, itemID); err != nil {
			return nil, err
		}
		if err := insertDevLinks(tx, itemID, update.Links); err != nil {
			return nil, err
		}
		return []string{sourceID}, nil
	})
}

func insertDevLinks(tx *sql.Tx, itemID string, links []DevLink) error {
	for _, dl := range links {
		if _, err := tx.Exec(`
			INSERT INTO dev_links (item_id, kind, external_id, url, title, status,
			                       author, actor, actor_name, branch, environment, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(item_id, url) DO UPDATE SET
			  kind=excluded.kind, external_id=excluded.external_id,
			  title=excluded.title, status=excluded.status,
			  author=excluded.author, actor=excluded.actor,
			  actor_name=excluded.actor_name, branch=excluded.branch,
			  environment=excluded.environment, updated_at=excluded.updated_at`,
			itemID, devKind(dl.Kind), dl.ExternalID, dl.URL, dl.Title, dl.Status,
			dl.Author, dl.Actor, dl.ActorName, dl.Branch, dl.Environment, dl.UpdatedAt,
		); err != nil {
			return err
		}
	}
	return nil
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}
