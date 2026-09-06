package store

import (
	"context"
	"database/sql"
	"sort"
	"time"
)

// CycleP85MinSamples is the fewest finished issues before a p85 cycle time is
// a distribution rather than a coincidence. The server omits the flow block
// below it (read.go flowFields), and the client ignores a carried flow below
// it, so a mocked or hand-written payload cannot lower the bar either.
const CycleP85MinSamples = 10

// CycleTimeP85Hours is the learned half of the stale threshold: how long the
// slowest 15% of recently finished work actually took, in hours. For each
// issue whose changelog has an entry INTO a done status at or after `since`,
// the cycle is that done entry (the latest one — matching Derive's
// last-write ResolvedAt) minus the FIRST entry into an in-progress status;
// issues with no in-progress entry have no measurable cycle and are skipped,
// as are cycles of zero or negative length (reopened, never re-finished —
// a non-cycle must not drag the percentile down; this deliberately diverges
// from Durations' span(), which clamps).
//
// Categories resolve by status id, never by display name (ids are stable,
// names localize): status_catalog per source first — two sources may reuse
// one id — and, when the catalog table is empty (the shipped demo fixture
// ships empty; a sync fills it), StatusCategories() reconstructs the map from
// issue rows. An id neither maps reads as not-done, same as Derive.
//
// The percentile is nearest-rank with integer arithmetic, rank
// (85n+99)/100 — the formula cmd/gadak/retro.go retroP85 owns. retroP85
// lives in package main, so it cannot be imported; keep the two in lockstep.
func (db *DB) CycleTimeP85Hours(ctx context.Context, since time.Time) (float64, int, error) {
	// status_catalog: (source, status id) -> category, scoped per source the
	// way retro.go scopes its walk.
	catalog := map[string]string{}
	crows, err := db.sql.QueryContext(ctx,
		`SELECT COALESCE(source_id,''), COALESCE(status_id,''), COALESCE(category,'') FROM status_catalog`)
	if err != nil {
		return 0, 0, err
	}
	for crows.Next() {
		var source, id, category string
		if err := crows.Scan(&source, &id, &category); err != nil {
			crows.Close()
			return 0, 0, err
		}
		catalog[source+"\x00"+id] = category
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return 0, 0, err
	}
	crows.Close()

	// Empty catalog: issue rows carry status_id + status_category, which is
	// the reconstruction StatusCategories() already owns (plus a catalog
	// overlay that is empty here by construction).
	fallback := map[string]string{}
	if len(catalog) == 0 {
		if fallback, err = db.StatusCategories(ctx); err != nil {
			return 0, 0, err
		}
	}
	categoryOf := func(source, id string) string {
		if len(catalog) > 0 {
			return catalog[source+"\x00"+id]
		}
		return fallback[id]
	}

	// One pass over the status changelog in time order: rows arrive
	// oldest-first, so the first in-progress entry and the latest done entry
	// are each a single compare.
	type cycleWalk struct {
		firstInprogress time.Time
		hasStart        bool
		latestDone      time.Time
		hasDone         bool
	}
	walks := map[string]*cycleWalk{}
	if err := each(ctx, db.sql, `
		SELECT c.item_id, COALESCE(c.at,''), COALESCE(c.to_id,''), COALESCE(it.source_id,'')
		FROM changelog c
		JOIN items it ON it.id = c.item_id
		WHERE it.kind = 'issue' AND c.field = 'status'
		  AND c.at IS NOT NULL AND c.at <> ''
		ORDER BY c.at, c.id`,
		func(rows *sql.Rows) error {
			var item, at, toID, source string
			if err := rows.Scan(&item, &at, &toID, &source); err != nil {
				return err
			}
			t, ok := parseStamp(at)
			if !ok {
				return nil // unparseable stamp drops the row, never errors the walk
			}
			w := walks[item]
			if w == nil {
				w = &cycleWalk{}
				walks[item] = w
			}
			switch categoryOf(source, toID) {
			case CategoryInProgress:
				if !w.hasStart {
					w.firstInprogress = t
					w.hasStart = true
				}
			case CategoryDone:
				if !t.Before(since) {
					w.latestDone = t
					w.hasDone = true
				}
			}
			return nil
		}); err != nil {
		return 0, 0, err
	}

	cycles := make([]float64, 0, len(walks))
	for _, w := range walks {
		if !w.hasStart || !w.hasDone || !w.latestDone.After(w.firstInprogress) {
			continue
		}
		cycles = append(cycles, w.latestDone.Sub(w.firstInprogress).Hours())
	}
	if len(cycles) == 0 {
		return 0, 0, nil
	}
	sort.Float64s(cycles)
	rank := (85*len(cycles) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	return cycles[rank-1], len(cycles), nil
}
