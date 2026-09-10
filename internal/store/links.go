package store

// links.go owns the one rule the child-list replacement in write.go cannot
// state on its own.
//
// A relationship is stored on both ends. The blocker carries `outward Blocks
// → blocked` and the blocked issue carries `inward Blocks → blocker`, because
// that is how both connectors ship it: a Jira issuelinks payload names the far
// end from each side, and the Linear pass splits relations / inverseRelations
// the same way (internal/sync/linear.go). write.go replaces an issue's own
// child lists wholesale, which is correct and complete for the issue in hand
// — and blind to the other end. An incremental window that carries only one
// of the two ends therefore left the counterpart row behind forever: the
// mirror still believed a link the origin no longer has,
// issues_raw.open_blockers stayed high, and `gadak ready` dropped an issue
// nothing blocks until the next full sync healed it (GDK-1507, measured by the
// GDK-1299 review round: "FIX-42 inward Blocks -> FIX-41" surviving, and
// FIX-42 open_blockers = 1).
//
// What this file does NOT do is assume the two ends agree. A link stored on
// one end only is a shape this mirror holds legitimately — a far end outside
// the synced scope, a fixture, an origin answer that named the relationship
// once — and openBlockersSelect is written for it (a target outside the
// mirror is not blocking). So the removal is driven by evidence, never by
// symmetry: the far end's row is deleted only when the issue being written
// *had* that exact link a moment ago and its fresh payload no longer does.
// That is the origin saying the relationship is gone, in the one place the
// mirror can hear it.
//
// The rule holds only when the fresh list is complete. Every connector's is,
// except a Linear relations connection the origin paged
// (IssueRecord.LinksPartial) — a truncated list cannot prove a link absent,
// so a partial record deletes nothing.

import (
	"context"
	"database/sql"
	"sort"
)

// mirrorLinkDirection is the direction the far end stores for the same
// relationship. An unknown direction has no counterpart and is skipped:
// links.direction is written from the connectors' own split and is one of
// these two, but a value this function does not understand must not be
// guessed into a deletion.
func mirrorLinkDirection(d string) string {
	switch d {
	case "outward":
		return "inward"
	case "inward":
		return "outward"
	}
	return ""
}

// storedLinks reads one item's link rows. Called before the child-list
// replacement, it is the "what this issue used to say" half of the evidence
// reconcileLinkCounterparts needs.
func storedLinks(tx *sql.Tx, itemID string) ([]Link, error) {
	var out []Link
	err := txEach(tx, `SELECT type, direction, target_key FROM links WHERE item_id = ?`,
		func(rows *sql.Rows) error {
			var l Link
			if err := rows.Scan(&l.Type, &l.Direction, &l.TargetKey); err != nil {
				return err
			}
			out = append(out, l)
			return nil
		}, itemID)
	return out, err
}

func linkKey(l Link) string { return l.Type + "\x00" + l.Direction + "\x00" + l.TargetKey }

// reconcileLinkCounterparts deletes the far-end row of every link this issue
// just lost, and returns the keys it touched so the caller can recompute
// their open_blockers — the deleted row is gone by then, so the caller's own
// "issues holding an inward link at a batch key" clause could no longer find
// them.
//
// old is the issue's link rows as they stood before this write, fresh is the
// list the origin just reported. Scope is one source: keys are unique within
// a source and mean nothing across two.
func reconcileLinkCounterparts(tx *sql.Tx, sourceID, itemID, key string, old, fresh []Link, partial bool) ([]string, error) {
	if sourceID == "" || itemID == "" || key == "" || partial || len(old) == 0 {
		return nil, nil
	}
	keep := make(map[string]bool, len(fresh))
	for _, l := range fresh {
		keep[linkKey(l)] = true
	}
	touched := map[string]bool{}
	for _, l := range old {
		if keep[linkKey(l)] {
			continue
		}
		d := mirrorLinkDirection(l.Direction)
		// A self-link has no far end to reconcile, and this issue's own rows
		// were already replaced by the caller.
		if d == "" || l.Type == "" || l.TargetKey == "" || l.TargetKey == key {
			continue
		}
		if _, err := tx.Exec(`
			DELETE FROM links
			WHERE type = ? AND direction = ? AND target_key = ?
			  AND item_id IN (SELECT id FROM items WHERE source_id = ? AND key = ? AND kind = 'issue')`,
			l.Type, d, key, sourceID, l.TargetKey); err != nil {
			return nil, err
		}
		touched[l.TargetKey] = true
	}
	out := make([]string, 0, len(touched))
	for k := range touched {
		out = append(out, k)
	}
	sort.Strings(out) // deterministic recompute key order
	return out, nil
}

// CountOneSidedLinks counts stored link rows whose far end is in the mirror
// and holds no counterpart row. Both ends must be mirrored for a row to
// count: a link pointing outside the synced scope has no counterpart to
// expect, and counting those would report every cross-project link as damage.
//
// It is a diagnostic, not a verdict (GDK-1507). Jira and Linear both report a
// relationship from both sides, so on a mirror synced from either, a non-zero
// number is the standing symptom of an incremental window that carried one
// end and not the other — one `gadak sync --full` levels it. A mirror seeded
// by other means can hold one-sided rows by construction, which is why
// doctor prints the number and draws no conclusion from it.
func (db *DB) CountOneSidedLinks(ctx context.Context) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM links l
		JOIN items o ON o.id = l.item_id
		JOIN items f ON f.source_id = o.source_id AND f.key = l.target_key AND f.kind = 'issue'
		WHERE l.direction IN ('inward','outward')
		  AND NOT EXISTS (
			SELECT 1 FROM links b
			WHERE b.item_id = f.id AND b.type = l.type
			  AND b.target_key = o.key
			  AND b.direction = CASE l.direction WHEN 'inward' THEN 'outward' ELSE 'inward' END)`).Scan(&n)
	return n, err
}
