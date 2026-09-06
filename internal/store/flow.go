package store

// flow.go owns the v43 flow layer's stored half: open_blockers and the
// link_types catalog it resolves its vocabulary from. started_at /
// cycle_hours / last_activity_at are Derive columns (derive.go); this file
// backfills all three at migration (backfillFlow) and recomputes
// open_blockers wherever links or target statuses move.
//
// Which link types block is site configuration: Jira's default type is named
// "Blocks" in English and something else on a localized account, so the
// names come from the origin's own catalog (cached in link_types, the
// status_catalog contract) — matched by lower(name) = 'blocks' OR
// lower(outward) LIKE 'block%', the same vocabulary `gadak link --type`
// resolves through origin.ResolveLinkType. When the catalog has no rows for
// a source, the literal 'Blocks' stands in: a mirror that has not yet run a
// post-v43 sync (and the shipped fixture) still answer, and the next sync
// replaces the guess with the site's real catalog.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// categoriesForSource is the single owner of "how a status id becomes a
// category" wherever a walk or a write needs one: status_catalog rows for
// that source first, and when the catalog holds none (the shipped fixture
// ships empty; a sync fills it), reconstruction from that source's own issue
// rows carrying both a status_id and a status_category. The migration
// backfill and the upsert path share it, so a column written at sync time
// and the same column recomputed at migration cannot disagree.
func categoriesForSource(tx *sql.Tx, sourceID string) (map[string]string, error) {
	cats := map[string]string{}
	rows, err := tx.Query(`
		SELECT COALESCE(status_id,''), COALESCE(category,'')
		FROM status_catalog WHERE source_id = ?`, sourceID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, cat string
		if err := rows.Scan(&id, &cat); err != nil {
			rows.Close()
			return nil, err
		}
		cats[id] = cat
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(cats) > 0 {
		return cats, nil
	}
	rows, err = tx.Query(`
		SELECT COALESCE(i.status_id,''), COALESCE(i.status_category,'')
		FROM issues_raw i JOIN items it ON it.id = i.item_id
		WHERE it.source_id = ?
		  AND i.status_id IS NOT NULL AND i.status_id != ''
		  AND i.status_category IS NOT NULL AND i.status_category != ''`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, cat string
		if err := rows.Scan(&id, &cat); err != nil {
			return nil, err
		}
		cats[id] = cat
	}
	return cats, rows.Err()
}

// cacheLinkTypeCatalog merges the batch's link-type rows into link_types,
// scoped by the first record's source (all records in a batch come from one
// source — the cacheStatusCatalog rule). Rows with an empty id are skipped:
// no join key.
func cacheLinkTypeCatalog(tx *sql.Tx, b Batch) error {
	if len(b.LinkTypes) == 0 || len(b.Records) == 0 {
		return nil
	}
	src := b.Records[0].Item.SourceID
	for _, lt := range b.LinkTypes {
		if lt.ID == "" {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO link_types (source_id, id, name, inward, outward) VALUES (?,?,?,?,?)
			ON CONFLICT(source_id, id) DO UPDATE SET
			  name = excluded.name, inward = excluded.inward, outward = excluded.outward`,
			src, lt.ID, lt.Name, lt.Inward, lt.Outward); err != nil {
			return err
		}
	}
	return nil
}

// blockingLinkTypeNames returns the catalog names of sourceID's blocking
// link types. A source with no catalog rows falls back to the literal
// 'Blocks' (documented caveat above); a source whose catalog exists but
// holds no blocking type genuinely blocks nothing — an empty list, which
// callers must treat as "no blockers", never as "cannot answer".
func blockingLinkTypeNames(tx *sql.Tx, sourceID string) ([]string, error) {
	var have int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM link_types WHERE source_id = ?`, sourceID).Scan(&have); err != nil {
		return nil, err
	}
	if have == 0 {
		return []string{"Blocks"}, nil
	}
	rows, err := tx.Query(`
		SELECT name FROM link_types
		WHERE source_id = ? AND (lower(name) = 'blocks' OR lower(outward) LIKE 'block%')`,
		sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if name != "" {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

// openBlockersSelect is the count behind issues_raw.open_blockers: inward
// links of a blocking type whose target issue is in the mirror and not done.
// A target outside the mirror is NOT blocking — the mirror cannot prove the
// far side unfinished, and "unknown" must not hold work back. The blocking
// names arrive as a json_each parameter, never interpolated: type names are
// site configuration strings.
const openBlockersSelect = `
  SELECT COUNT(*) FROM links l
  WHERE l.item_id = issues_raw.item_id
    AND l.direction = 'inward'
    AND l.type IN (SELECT value FROM json_each(?))
    AND EXISTS (SELECT 1 FROM issues_raw t
                WHERE t.key = l.target_key AND t.status_category != 'done')`

// recomputeOpenBlockers rewrites issues_raw.open_blockers for one source.
// keys, when non-empty, limits the rewrite to those keys plus every issue
// holding an inward link at one of them — the batch's own rows and the
// issues a batch page's status change un-blocks, which is the whole point:
// a page that carries the blocker but not the blocked issue (C4) must still
// move the blocked row's count. An empty keys recomputes every issue of the
// source.
func recomputeOpenBlockers(tx *sql.Tx, sourceID string, keys []string) error {
	names, err := blockingLinkTypeNames(tx, sourceID)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	namesJSON, err := json.Marshal(names)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		_, err = tx.Exec(`
			UPDATE issues_raw SET open_blockers = (`+openBlockersSelect+`)
			WHERE item_id IN (SELECT id FROM items WHERE source_id = ? AND kind = 'issue')`,
			string(namesJSON), sourceID)
		return err
	}
	keysJSON, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		UPDATE issues_raw SET open_blockers = (`+openBlockersSelect+`)
		WHERE key IN (SELECT value FROM json_each(?))
		   OR item_id IN (SELECT item_id FROM links
		                  WHERE direction = 'inward'
		                    AND target_key IN (SELECT value FROM json_each(?)))`,
		string(namesJSON), string(keysJSON), string(keysJSON))
	return err
}

// RecomputeOpenBlockers rewrites open_blockers for every issue of every
// source. A full sync calls this once after the last page, beside
// RecomputeEpicKeys: page-scoped recomputes cannot see a link the origin
// added to an issue no page carried.
func (db *DB) RecomputeOpenBlockers(ctx context.Context) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT DISTINCT source_id FROM items WHERE kind = 'issue'`)
		if err != nil {
			return err
		}
		var srcs []string
		for rows.Next() {
			var src string
			if err := rows.Scan(&src); err != nil {
				rows.Close()
				return err
			}
			srcs = append(srcs, src)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, src := range srcs {
			if err := recomputeOpenBlockers(tx, src, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

// backfillFlow is the v43 migration hook (the v15/v16 shape): derive
// started_at / cycle_hours / last_activity_at for every mirrored issue
// through Derive — the single owner of those rules, so the backfill and the
// sync path cannot disagree — then sweep open_blockers whole-table.
//
// Categories resolve through categoriesForSource per source (status_catalog
// first, issue-row reconstruction when a source holds no catalog rows — the
// shipped fixture's state). ReopenedAt/reopen_reason/cloned_from/
// priority_rank are NOT rewritten: those columns already hold their values
// and Derive's inputs here are deliberately lean (no comment bodies, no
// links, no priority list), so only the three new columns are written back.
func backfillFlow(tx *sql.Tx) error {
	irows, err := tx.Query(`
		SELECT ir.item_id, COALESCE(it.source_id,''), COALESCE(ir.status_category,''), COALESCE(it.updated_at,'')
		FROM issues_raw ir JOIN items it ON it.id = ir.item_id`)
	if err != nil {
		return err
	}
	type issueRow struct {
		itemID, source, category, updated string
	}
	var issues []issueRow
	for irows.Next() {
		var r issueRow
		if err := irows.Scan(&r.itemID, &r.source, &r.category, &r.updated); err != nil {
			irows.Close()
			return err
		}
		issues = append(issues, r)
	}
	if err := irows.Err(); err != nil {
		irows.Close()
		return err
	}
	irows.Close()

	cats := map[string]map[string]string{}
	catSources := map[string]bool{}
	for _, r := range issues {
		if catSources[r.source] {
			continue
		}
		catSources[r.source] = true
		m, err := categoriesForSource(tx, r.source)
		if err != nil {
			return err
		}
		cats[r.source] = m
	}

	for _, r := range issues {
		entries := []ChangeEntry{}
		lrows, err := tx.Query(`
			SELECT COALESCE(field,''), COALESCE(at,''), COALESCE(from_id,''), COALESCE(to_id,'')
			FROM changelog WHERE item_id = ?`, r.itemID)
		if err != nil {
			return err
		}
		for lrows.Next() {
			var e ChangeEntry
			if err := lrows.Scan(&e.Field, &e.At, &e.FromID, &e.ToID); err != nil {
				lrows.Close()
				return err
			}
			entries = append(entries, e)
		}
		if err := lrows.Err(); err != nil {
			lrows.Close()
			return err
		}
		lrows.Close()

		comments := []Comment{}
		mrows, err := tx.Query(`SELECT COALESCE(created_at,'') FROM comments WHERE item_id = ?`, r.itemID)
		if err != nil {
			return err
		}
		for mrows.Next() {
			var at string
			if err := mrows.Scan(&at); err != nil {
				mrows.Close()
				return err
			}
			comments = append(comments, Comment{CreatedAt: at})
		}
		if err := mrows.Err(); err != nil {
			mrows.Close()
			return err
		}
		mrows.Close()

		d := Derive(DeriveInput{
			Changelog:       entries,
			Categories:      cats[r.source],
			CurrentCategory: r.category,
			Comments:        comments,
			UpdatedAt:       r.updated,
		})
		if _, err := tx.Exec(`
			UPDATE issues_raw SET started_at = ?, cycle_hours = ?, last_activity_at = ?
			WHERE item_id = ?`,
			d.StartedAt, d.CycleHours, d.LastActivityAt, r.itemID); err != nil {
			return fmt.Errorf("backfill flow %s: %w", r.itemID, err)
		}
	}
	return recomputeOpenBlockersAllSources(tx)
}

// recomputeOpenBlockersAllSources is RecomputeOpenBlockers's loop on an
// existing transaction, for callers already inside one (the migration hook).
func recomputeOpenBlockersAllSources(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT DISTINCT source_id FROM items WHERE kind = 'issue'`)
	if err != nil {
		return err
	}
	var srcs []string
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			rows.Close()
			return err
		}
		srcs = append(srcs, src)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, src := range srcs {
		if err := recomputeOpenBlockers(tx, src, nil); err != nil {
			return err
		}
	}
	return nil
}

// BackfillFlow is backfillFlow on an open handle's own transaction. The
// snapshot builder calls it after copying rows into a fresh database: the
// copy lands the flow columns at their DEFAULT (the column-bag mover does
// not know them), and this recomputes them from the destination's own rows
// so they agree with whatever timestamps the snapshot spread.
func (db *DB) BackfillFlow(ctx context.Context) error {
	return db.write(ctx, func(tx *sql.Tx) error {
		return backfillFlow(tx)
	})
}
