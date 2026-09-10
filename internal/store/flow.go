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
	"strings"
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
	if err := txEach(tx, `
		SELECT COALESCE(status_id,''), COALESCE(category,'')
		FROM status_catalog WHERE source_id = ?`,
		func(rows *sql.Rows) error {
			var id, cat string
			if err := rows.Scan(&id, &cat); err != nil {
				return err
			}
			cats[id] = cat
			return nil
		}, sourceID); err != nil {
		return nil, err
	}
	if len(cats) > 0 {
		return cats, nil
	}
	if err := txEach(tx, `
		SELECT COALESCE(i.status_id,''), COALESCE(i.status_category,'')
		FROM issues_raw i JOIN items it ON it.id = i.item_id
		WHERE it.source_id = ?
		  AND i.status_id IS NOT NULL AND i.status_id != ''
		  AND i.status_category IS NOT NULL AND i.status_category != ''`,
		func(rows *sql.Rows) error {
			var id, cat string
			if err := rows.Scan(&id, &cat); err != nil {
				return err
			}
			cats[id] = cat
			return nil
		}, sourceID); err != nil {
		return nil, err
	}
	return cats, nil
}

// LinkTypePhrases reads the cached link-type catalog as a name → (inward,
// outward) map, lowercased by name. The one owner of that SQL (GDK-1215):
// the detail response's phrase field and the CLI's human link line both
// render through origin.LinkPhrase and neither writes its own query any
// more. An unreadable or empty catalog answers nil — callers fall back to
// the wire pair wording, the contract the CLI had before the catalog
// existed (GDK-1734). Sources are not distinguished: a name two sources
// share is one entry, last row wins — same semantics the CLI's copy had.
func (db *DB) LinkTypePhrases() map[string][2]string {
	rows, err := db.Query(`SELECT name, inward, outward FROM link_types`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string][2]string{}
	for rows.Next() {
		var name, inward, outward string
		if err := rows.Scan(&name, &inward, &outward); err != nil {
			return nil
		}
		out[strings.ToLower(strings.TrimSpace(name))] = [2]string{inward, outward}
	}
	return out
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
	var out []string
	if err := txEach(tx, `
		SELECT name FROM link_types
		WHERE source_id = ? AND (lower(name) = 'blocks' OR lower(outward) LIKE 'block%')`,
		func(rows *sql.Rows) error {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			if name != "" {
				out = append(out, name)
			}
			return nil
		}, sourceID); err != nil {
		return nil, err
	}
	return out, nil
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
// added to an issue no page carried. The walk itself is
// recomputeOpenBlockersAllSources — the same function the v43 migration
// hook's closing sweep runs, so the two cannot disagree (GDK-1576).
func (db *DB) RecomputeOpenBlockers(ctx context.Context) error {
	return db.write(ctx, recomputeOpenBlockersAllSources)
}

// backfillFlow is the v43 migration hook (the v15/v16 shape): derive
// started_at / cycle_hours / last_activity_at for every mirrored issue
// through Derive — the single owner of those rules, so the backfill and the
// sync path cannot disagree — then sweep open_blockers whole-table.
//
// Categories resolve through categoriesForSource per source (status_catalog
// first, issue-row reconstruction when a source holds no catalog rows — the
// shipped fixture's state). cloned_from/priority_rank are NOT rewritten:
// those columns already hold their values and Derive's inputs here are
// deliberately lean (no links, no priority list). resolved_at and reopened_at
// ARE rewritten since GDK-1720 — they name transitions, and Derive is their
// single owner on the sync path too (write.go), so the backfill and a sync
// agree on them. The reopen triple is rewritten with them too
// (count, stamp, reason): the rule grew an inprogress→new axis, and a reason
// keyed to the pre-migration stamp would describe a reopen the count no
// longer names — so comment bodies joined the lean inputs.
func backfillFlow(tx *sql.Tx) error {
	type issueRow struct {
		itemID, source, category, updated, created, kind string
	}
	var issues []issueRow
	if err := txEach(tx, `
		SELECT ir.item_id, COALESCE(it.source_id,''), COALESCE(ir.status_category,''), COALESCE(it.updated_at,''),
		       COALESCE(it.created_at,''), COALESCE(s.kind,'')
		FROM issues_raw ir JOIN items it ON it.id = ir.item_id
		LEFT JOIN sources s ON s.id = it.source_id`,
		func(rows *sql.Rows) error {
			var r issueRow
			if err := rows.Scan(&r.itemID, &r.source, &r.category, &r.updated, &r.created, &r.kind); err != nil {
				return err
			}
			issues = append(issues, r)
			return nil
		}); err != nil {
		return err
	}

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
		if err := txEach(tx, `
			SELECT COALESCE(field,''), COALESCE(at,''), COALESCE(from_id,''), COALESCE(to_id,'')
			FROM changelog WHERE item_id = ?`,
			func(rows *sql.Rows) error {
				var e ChangeEntry
				if err := rows.Scan(&e.Field, &e.At, &e.FromID, &e.ToID); err != nil {
					return err
				}
				entries = append(entries, e)
				return nil
			}, r.itemID); err != nil {
			return err
		}

		comments := []Comment{}
		if err := txEach(tx, `SELECT COALESCE(created_at,''), COALESCE(body_text,'') FROM comments WHERE item_id = ?`,
			func(rows *sql.Rows) error {
				var at, body string
				if err := rows.Scan(&at, &body); err != nil {
					return err
				}
				comments = append(comments, Comment{CreatedAt: at, BodyText: body})
				return nil
			}, r.itemID); err != nil {
			return err
		}

		d := Derive(DeriveInput{
			Changelog:       entries,
			Categories:      cats[r.source],
			CurrentCategory: r.category,
			CreatedAt:       r.created,
			// Linear supplies no changelog (internal/sync/linear.go sets
			// Batch.NoHistory); the backfill reads the same fact off the source row.
			NoHistory: r.kind == "linear",
			Comments:  comments,
			UpdatedAt: r.updated,
		})
		if _, err := tx.Exec(`
			UPDATE issues_raw SET started_at = ?, cycle_hours = ?, last_activity_at = ?
			WHERE item_id = ?`,
			d.StartedAt, d.CycleHours, d.LastActivityAt, r.itemID); err != nil {
			return fmt.Errorf("backfill flow %s: %w", r.itemID, err)
		}
		// status_changed_at is derived from the same changelog this loop just
		// read, and Derive already computed it — it was simply dropped here
		// (GDK-1684). In a snapshot that is not cosmetic: the column-bag mover
		// carries the source's stamp while the spread re-times the changelog
		// rows independently, so the two drift and the column points at an
		// instant no transition happened. Measured on a regenerated
		// examples/demo.db: jira:10094 read 21:38:18 between changelog rows at
		// 21:36:35 and 21:39:01. Only written when the history produced one —
		// an issue whose changelog never reached the mirror keeps whatever it
		// had rather than losing it to a nil.
		if d.StatusChangedAt != nil {
			if _, err := tx.Exec(
				`UPDATE issues_raw SET status_changed_at = ? WHERE item_id = ?`,
				*d.StatusChangedAt, r.itemID); err != nil {
				return fmt.Errorf("backfill status_changed_at %s: %w", r.itemID, err)
			}
		}
		// resolved_at and reopened_at name transitions too, and rode the same
		// drift for the same reason (GDK-1720): the column-bag mover carries
		// the source's stamps while the spread re-times the changelog, so
		// resolved_at pointed at an instant no Done entry happened and the
		// retro's "closed" rows disagreed with the history view beside them.
		// Guarded like status_changed_at — an issue whose changelog never
		// reached the mirror keeps what it had rather than losing it to a nil.
		if d.ResolvedAt != nil {
			if _, err := tx.Exec(
				`UPDATE issues_raw SET resolved_at = ? WHERE item_id = ?`,
				*d.ResolvedAt, r.itemID); err != nil {
				return fmt.Errorf("backfill resolved_at %s: %w", r.itemID, err)
			}
		}
		// The reopen triple rides the same guard and the same drift fix
		// as resolved_at: the rule now counts inprogress→new moves, so a mirror
		// backfilled before it holds counts the changelog disagrees with, and
		// a reason keyed to the old reopened_at would name a comment the new
		// stamp no longer points at. Count and reason are written only here,
		// under the same "history produced one" guard — an issue whose
		// changelog never reached the mirror keeps all three as they were.
		if d.ReopenedAt != nil {
			if _, err := tx.Exec(
				`UPDATE issues_raw SET reopen_count = ?, reopened_at = ?, reopen_reason = ? WHERE item_id = ?`,
				d.ReopenCount, *d.ReopenedAt, d.ReopenReason, r.itemID); err != nil {
				return fmt.Errorf("backfill reopened_at %s: %w", r.itemID, err)
			}
		}
	}
	return recomputeOpenBlockersAllSources(tx)
}

// recomputeOpenBlockersAllSources is the whole-table open_blockers sweep,
// on an existing transaction: RecomputeOpenBlockers and the v43 migration
// hook's closing sweep (backfillFlow) both call this one function, so the
// public recompute and the backfill cannot land different counts
// (GDK-1576).
func recomputeOpenBlockersAllSources(tx *sql.Tx) error {
	var srcs []string
	if err := txEach(tx, `SELECT DISTINCT source_id FROM items WHERE kind = 'issue'`,
		func(rows *sql.Rows) error {
			var src string
			if err := rows.Scan(&src); err != nil {
				return err
			}
			srcs = append(srcs, src)
			return nil
		}); err != nil {
		return err
	}
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

// normalizeSprintChangelog rewrites the pre-v48 changelog rows that carry the
// sprint history under the site's own custom field id.
//
// The field id is per-site (customfield_10020 here, another number there), so
// a migration cannot know it from a constant and has no network to ask. The
// mirror answers instead: a changelog row whose value names a sprint this
// mirror already has — by id or by name — is a sprint row, and the field it
// sits on is that site's sprint field. That is a join, not a guess. A mirror
// with no sprints yet changes nothing and is filled by the next sync.
func normalizeSprintChangelog(tx *sql.Tx) error {
	_, err := tx.Exec(`
		UPDATE changelog SET field = 'sprint'
		WHERE field LIKE 'customfield_%'
		  AND field IN (
			SELECT DISTINCT c.field
			FROM changelog c
			JOIN items it ON it.id = c.item_id
			JOIN sprints s ON s.source_id = it.source_id
			                AND (CAST(s.id AS TEXT) = c.to_id OR s.name = c.to_value)
			WHERE c.field LIKE 'customfield_%' AND c.to_id != ''
		  )`)
	return err
}

// backfillCarryover is the v48 migration hook: after the sprint changelog
// rows can be recognised (normalizeSprintChangelog), run every mirrored issue
// back through Derive so the stored carry-over columns and the sync path
// cannot disagree. Only the three new columns are written back — the rest of
// Derive's output already holds its values and the inputs here are lean.
func backfillCarryover(tx *sql.Tx) error {
	if err := normalizeSprintChangelog(tx); err != nil {
		return fmt.Errorf("normalize sprint changelog: %w", err)
	}
	type row struct{ itemID, kind string }
	var issues []row
	if err := txEach(tx, `
		SELECT ir.item_id, COALESCE(s.kind,'')
		FROM issues_raw ir JOIN items it ON it.id = ir.item_id
		LEFT JOIN sources s ON s.id = it.source_id`,
		func(rows *sql.Rows) error {
			var r row
			if err := rows.Scan(&r.itemID, &r.kind); err != nil {
				return err
			}
			issues = append(issues, r)
			return nil
		}); err != nil {
		return err
	}
	for _, r := range issues {
		entries := []ChangeEntry{}
		if err := txEach(tx, `
			SELECT COALESCE(field,''), COALESCE(at,''), COALESCE(to_id,'')
			FROM changelog WHERE item_id = ? AND field = 'sprint'`,
			func(rows *sql.Rows) error {
				var e ChangeEntry
				if err := rows.Scan(&e.Field, &e.At, &e.ToID); err != nil {
					return err
				}
				entries = append(entries, e)
				return nil
			}, r.itemID); err != nil {
			return err
		}
		d := Derive(DeriveInput{Changelog: entries, NoHistory: r.kind == "linear"})
		if _, err := tx.Exec(`
			UPDATE issues_raw SET carryover_count = ?, first_sprint_id = ?, first_sprint_at = ?
			WHERE item_id = ?`,
			d.CarryoverCount, d.FirstSprintID, d.FirstSprintAt, r.itemID); err != nil {
			return fmt.Errorf("backfill carryover %s: %w", r.itemID, err)
		}
	}
	return nil
}

// BackfillCarryoverTx is backfillCarryover on a caller's transaction. The
// snapshot pipeline builds its own fixture and never goes through the sync
// write path, so it derives the sprint history itself and then asks this
// package — the single owner of the rule — for the columns. Exported for
// that one caller, the same reason BackfillFlow is.
func BackfillCarryoverTx(tx *sql.Tx) error { return backfillCarryover(tx) }
