package store

import (
	"context"
	"time"
)

// CycleP85MinSamples is the fewest finished issues before a p85 cycle time is
// a distribution rather than a coincidence. The server omits the flow block
// below it (read.go flowFields), and the client ignores a carried flow below
// it, so a mocked or hand-written payload cannot lower the bar either.
//
// Eleven, not ten (2026-09-06, second literature round): nearest-rank p85
// returns the maximum observation at n ≤ 6 and the second-largest at n ≤ 13,
// so a thin sample quotes an outlier as a percentile. The flow canon's own
// landmarks are five points for a credible median, eleven before the range
// says anything, and about twenty for useful limits (ProKanban, "How much
// data do I need to start forecasting?"); eleven is the first of those that
// this number can honestly stand on.
const CycleP85MinSamples = 11

// CycleTimeP85Hours is the learned half of the stale threshold: how long the
// slowest 15% of recently finished work actually took, in hours. Since v43 it
// reads the stored cycle_hours column (resolved_at − started_at, both Derive
// columns kept current by every upsert and backfilled at migration) instead
// of walking the changelog: `resolved_at >= since` selects the same window
// the walk's "done entry at or after since" did, and cycle_hours is NULL
// exactly for the issues the walk skipped — never in progress, finished
// before the window, or reopened and not re-finished (a non-cycle must not
// drag the percentile down; this deliberately diverges from Durations'
// span(), which clamps).
//
// The changelog walk this replaced lives on in cycle_time_test.go as the
// oracle the column read is proven equal against — on a seeded fixture and
// on a migrated copy of examples/demo.db. If the two ever disagree, the
// columns drifted from Derive's rules and that is the bug to fix, not the
// test.
//
// The percentile is nearest-rank with integer arithmetic, rank
// (85n+99)/100 — the formula internal/retro P85 owns (it moved out of
// cmd/gadak with the whole compute). Keep the two in lockstep.
//
// `since` is compared as a string in the stamps' own format
// (millisecond RFC3339, UTC): every column this reads is written by Derive
// in that one format, so the comparison never crosses precisions.
func (db *DB) CycleTimeP85Hours(ctx context.Context, since time.Time) (float64, int, error) {
	sinceStr := since.UTC().Format("2006-01-02T15:04:05.000") + "Z"
	rows, err := db.sql.QueryContext(ctx, `
		SELECT cycle_hours FROM issues_raw
		WHERE resolved_at >= ? AND cycle_hours IS NOT NULL
		ORDER BY cycle_hours`, sinceStr)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	cycles := []float64{}
	for rows.Next() {
		var h float64
		if err := rows.Scan(&h); err != nil {
			return 0, 0, err
		}
		cycles = append(cycles, h)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	if len(cycles) == 0 {
		return 0, 0, nil
	}
	// rows arrive ORDER BY cycle_hours; the rank index is the percentile.
	rank := (85*len(cycles) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	return cycles[rank-1], len(cycles), nil
}
