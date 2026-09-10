package store

import (
	"context"
	"os"
)

// The doctor-facing facts (GDK-307, GDK-1413). doctor (cmd/gadak) owns the
// report fields; the store owns the file and schema knowledge those fields
// quote — which sidecars a WAL mirror has, and what "open issues by
// priority" counts. These helpers are facts only: no migration, no write,
// no PRAGMA that moves a byte.

// MirrorSidecarBytes reports the byte sizes of the mirror's WAL sidecars —
// gadak.db-wal and gadak.db-shm next to the mirror at path. An absent file
// is 0, not an error: a mirror with no -wal is a legitimate state (never
// opened, or checkpointed on close), and the diagnostic the caller prints
// is "wal=0" rather than a missing-file error — MirrorVersion's rule, one
// file over.
//
// This is the GDK-307 observability seam. store.Open sets journal_mode=WAL
// and no journal_size_limit caps the sidecar, so the -wal file is bounded
// only by checkpointing — and a serve process that holds the mirror open
// never gets the checkpoint-on-close. Whether that ever starves in practice
// is not this function's claim; the claim is that without these bytes being
// visible, "the -wal file grew" and "SQLite is broken" are indistinguishable
// from outside the machine. Callers report them before store.Open (opening
// mints the sidecars when absent — doctor's existing mirror Stat has the
// same ordering note).
func MirrorSidecarBytes(path string) (wal, shm int64) {
	for _, s := range [...]struct {
		name string
		out  *int64
	}{
		{path + "-wal", &wal},
		{path + "-shm", &shm},
	} {
		if fi, err := os.Stat(s.name); err == nil {
			*s.out = fi.Size()
		}
	}
	return wal, shm
}

// PriorityCount is one row of the open-issue priority census (GDK-1413):
// how many open issues carry this priority_rank. Rank 0 is "unset" and is a
// row of its own on purpose — "72% 미설정" and "72%가 rank 1" are the same
// fact about an axis carrying no information, and folding unset into the
// total would hide it.
type PriorityCount struct {
	Rank  int `json:"rank"`
	Count int `json:"count"`
}

// PriorityDistribution is the whole census: one row per distinct
// priority_rank among open issues, plus the total they sum to. Open keys on
// status_category (new|inprogress are open, done is not) — the category,
// never a status display name, which localizes per account and silently
// returns zero rows (my_open's rule).
type PriorityDistribution struct {
	Rows []PriorityCount `json:"rows"`
	Open int             `json:"open"`
}

// DominantSharePct is the doctor-warning threshold of GDK-1413: a single
// priority value holding this share of the open issues. Attribution: the
// number is the issue's own ("한 값이 70% 이상이면"), pinned 2026-09-10,
// round w11-server — not derived from a measurement, so moving it is a
// product decision to argue, not a tuning round. The comparison that uses
// it is exact integer math (100*count >= 70*open) so a boundary case never
// rides a float rounding.
const DominantSharePct = 70

// Concentrated names the largest row when one priority value holds
// DominantSharePct or more of the open issues — the fact the doctor line
// states ("72%가 rank 1"), which is a census sentence, not a verdict on the
// workspace. ok=false when there are no open issues or the values are
// spread out, the healthy shapes both.
func (d PriorityDistribution) Concentrated() (row PriorityCount, pct int, ok bool) {
	for _, r := range d.Rows {
		if r.Count > row.Count {
			row = r
		}
	}
	if row.Count > 0 && 100*row.Count >= DominantSharePct*d.Open {
		return row, 100 * row.Count / d.Open, true
	}
	return PriorityCount{}, 0, false
}

// OpenPriorityDistribution is the census query: one GROUP BY over
// issues_raw, the physical table every issue writer touches — the same
// population doctor's issues count already walks, so the two numbers cannot
// disagree about what an issue is. COALESCE keeps a NULL status_category in
// the open side (it is certainly not done); done rows are excluded wherever
// they carry the category.
func (db *DB) OpenPriorityDistribution(ctx context.Context) (PriorityDistribution, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT priority_rank, COUNT(*) FROM issues_raw
		WHERE COALESCE(status_category, '') <> 'done'
		GROUP BY priority_rank
		ORDER BY priority_rank`)
	if err != nil {
		return PriorityDistribution{}, err
	}
	defer rows.Close()
	out := PriorityDistribution{Rows: []PriorityCount{}}
	for rows.Next() {
		var r PriorityCount
		if err := rows.Scan(&r.Rank, &r.Count); err != nil {
			return PriorityDistribution{}, err
		}
		out.Rows = append(out.Rows, r)
		out.Open += r.Count
	}
	return out, rows.Err()
}
