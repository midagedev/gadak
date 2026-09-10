package store

import (
	"context"
	"testing"
)

/*
 * GDK-1451: a visit stamps what it saw. visits.seen_updated_at carries the
 * issue's updated_at at the moment of the read, so "rows that changed since
 * I last opened them" is a local join — no origin call, no new sync. The
 * recipe in docs/RECIPES.md ("Mine") is the user-facing copy of the query
 * asserted here; cmd/gadak/recipes_gate_test.go runs that copy against the
 * demo mirror.
 *
 * FAIL-first: against the pre-fix source this file's SELECT dies with
 * "no such column: seen_updated_at" — the column is the deliverable.
 *
 * Clause table:
 *
 *   S1 the stamp is the issue's updated_at at visit time —
 *      TestRecordVisitStampsSeenUpdatedAt ①
 *   S1 a key the mirror does not know stamps '' (never a false "changed") —
 *      TestRecordVisitStampsSeenUpdatedAt ②
 *   S1 the changed-since-seen join answers — TestChangedSinceSeenJoin
 *        ③ visited, then updated → caught
 *        ④ visited after the change → not caught
 *        ⑤ never-visited rows and ''-stamped rows → never caught
 *   S1 person reads only: the recipe filters source to ui or the pre-V7
 *      empty source, matching LastVisits' boundary rule — ⑥
 */
func TestRecordVisitStampsSeenUpdatedAt(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	ctx := context.Background()

	// NMB-3 is the fixture's unassigned issue; its updated_at is the stamp.
	var before string
	if err := db.sql.QueryRowContext(ctx,
		`SELECT updated_at FROM issues_full WHERE key = 'NMB-3'`).Scan(&before); err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	v, err := db.RecordVisit(ctx, "issue", "NMB-3", VisitSourceUI)
	if err != nil {
		t.Fatal(err)
	}
	if v.SeenUpdatedAt != before {
		t.Fatalf("visit stamp = %q, want the issue's updated_at %q", v.SeenUpdatedAt, before)
	}
	var stored string
	if err := db.sql.QueryRowContext(ctx,
		`SELECT seen_updated_at FROM local.visits WHERE id = ?`, v.ID).Scan(&stored); err != nil {
		t.Fatalf("read back seen_updated_at: %v", err)
	}
	if stored != before {
		t.Fatalf("stored stamp = %q, want %q", stored, before)
	}

	// ② A key outside the mirror (or a page the mirror has no stamp for)
	// records the visit anyway, with an empty stamp — an unknown "seen"
	// must never read as "changed since seen".
	unknown, err := db.RecordVisit(ctx, "issue", "NMB-404", VisitSourceUI)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.SeenUpdatedAt != "" {
		t.Fatalf("unknown key stamp = %q, want empty", unknown.SeenUpdatedAt)
	}
}

// changedSinceSeenSQL is the recipe from docs/RECIPES.md's Mine section,
// verbatim in shape: newest person read per issue, rows whose updated_at
// moved after the stamp that read recorded.
const changedSinceSeenSQL = `
SELECT i.key
FROM issues_full i
JOIN (
  SELECT key, MAX(viewed_at) AS viewed_at, seen_updated_at
  FROM local.visits
  WHERE kind = 'issue' AND source IN ('ui','') AND seen_updated_at <> ''
  GROUP BY key
) v ON v.key = i.key
WHERE i.updated_at > v.seen_updated_at
ORDER BY i.updated_at DESC`

func TestChangedSinceSeenJoin(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	ctx := context.Background()

	changedKeys := func(t *testing.T) map[string]bool {
		t.Helper()
		rows, err := db.sql.QueryContext(ctx, changedSinceSeenSQL)
		if err != nil {
			t.Fatalf("changed-since-seen query: %v", err)
		}
		defer rows.Close()
		out := map[string]bool{}
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				t.Fatal(err)
			}
			out[k] = true
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// The fixture's three issues all carry updated_at stamps.
	var stale string
	if err := db.sql.QueryRowContext(ctx,
		`SELECT updated_at FROM issues_full WHERE key = 'NMB-3'`).Scan(&stale); err != nil {
		t.Fatal(err)
	}

	// ③ Visit NMB-3, then move its updated_at past the visit's stamp. The
	// column the recipe compares is the projection's (issues_full.updated_at
	// is issues_raw's), so that is the one the fixture moves.
	if _, err := db.RecordVisit(ctx, "issue", "NMB-3", VisitSourceUI); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.ExecContext(ctx,
		`UPDATE issues_raw SET updated_at = '2027-01-01T00:00:00.000Z'
		 WHERE item_id = (SELECT item_id FROM issues_full WHERE key = 'NMB-3')`); err != nil {
		t.Fatal(err)
	}
	got := changedKeys(t)
	if !got["NMB-3"] {
		t.Fatalf("NMB-3 changed after the newest visit but was not caught: %v", got)
	}

	// ④ Visit again after the change: the newest read now carries the new
	// stamp, and the row stops answering.
	if _, err := db.RecordVisit(ctx, "issue", "NMB-3", VisitSourceUI); err != nil {
		t.Fatal(err)
	}
	if got = changedKeys(t); got["NMB-3"] {
		t.Fatalf("NMB-3 still answered after a visit that saw the new stamp: %v", got)
	}

	// ⑤ NMB-1/NMB-2 were never visited and must never answer.
	if got["NMB-1"] || got["NMB-2"] {
		t.Fatalf("never-visited rows answered the changed-since-seen join: %v", got)
	}

	// ⑥ A CLI read is not the person returning (LastVisits' rule): recording
	// one after the change leaves the older person read as the newest seen.
	if _, err := db.sql.ExecContext(ctx,
		`UPDATE issues_raw SET updated_at = '2027-02-01T00:00:00.000Z'
		 WHERE item_id = (SELECT item_id FROM issues_full WHERE key = 'NMB-3')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVisit(ctx, "issue", "NMB-3", VisitSourceCLI); err != nil {
		t.Fatal(err)
	}
	if got = changedKeys(t); !got["NMB-3"] {
		t.Fatalf("a cli read must not clear the person's changed-since-seen answer: %v", got)
	}
}
