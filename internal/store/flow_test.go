package store

// flow_test.go — the v43 flow layer's store half: Derive's three new
// columns, the migration backfill, and open_blockers / link_types.
//
// Contract ↔ assertion map (r3 flow layer):
//   C1  recomputed from mirrored data only — every test seeds the mirror
//       (rows or batches) and asserts stored values; no origin anywhere
//   C2  blocking type by catalog with documented 'Blocks' fallback —
//       TestRecomputeOpenBlockersResolvesCatalogNames (all three branches:
//       catalog name/outward, catalog-without-blocks, empty-catalog
//       fallback). Categories key on status ids, never display names —
//       every seed uses fixtureCategories
//   C4  open_blockers moves when a blocker's status changes without the
//       blocked issue in the batch — TestUpsertWidensOpenBlockers
//       (FAIL-first against pre-v43 code: the column does not exist, the
//       SELECT fails with "no such column")
//   migration — TestMigrateV43BackfillsFlow (FAIL-first: seeded at v42,
//       the same SELECTs fail before schemaV43 runs; Open is what applies
//       it). The fixture ships no status_catalog, so this test is also the
//       answer to "which fallback fed the backfill": the issue-row
//       reconstruction. TestBackfillFlowWithStatusCatalog is the path a
//       real synced mirror takes (catalog rows present).
//   C8  no new imports beyond the package's own surface (testing + the
//       already-blank-imported sqlite driver)

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// flowNow freezes the stamps like write_test.go's fixtureNow: seeded on one
// side of a second boundary, asserted on the other.
var flowNow = time.Now().UTC().Truncate(time.Second)

func flowAgo(days int) string {
	return flowNow.AddDate(0, 0, -days).Format("2006-01-02T15:04:05Z")
}

// TestDeriveFlowColumns covers the three Derive columns in isolation:
// started_at (first inprogress transition), cycle_hours (done now, positive
// span), last_activity_at (string max over changelog, item stamp, comments).
// FAIL-first: against pre-v43 derive.go none of these fields exist — the
// struct literals do not compile.
func TestDeriveFlowColumns(t *testing.T) {
	cats := fixtureCategories

	// Started, finished, then a later comment and item stamp: cycle is the
	// resolved−started span, last activity is the newest of all four stamps.
	d := Derive(DeriveInput{
		Changelog: []ChangeEntry{
			{ID: "h1", At: flowAgo(40), Field: "status", FromID: "1", ToID: "3"},
			{ID: "h2", At: flowAgo(30), Field: "status", FromID: "3", ToID: "5"},
			{ID: "h3", At: flowAgo(35), Field: "priority"}, // older entry: must not win
		},
		Categories:      cats,
		CurrentCategory: "done",
		Comments:        []Comment{{CreatedAt: flowAgo(20)}},
		UpdatedAt:       flowAgo(10),
	})
	if d.StartedAt == nil || *d.StartedAt != flowAgo(40) {
		t.Fatalf("started_at = %v, want the 40d-ago inprogress transition", d.StartedAt)
	}
	if d.CycleHours == nil {
		t.Fatal("cycle_hours is nil for a done issue with a positive span")
	}
	want := 10 * 24.0 // resolved 30d ago minus started 40d ago
	if *d.CycleHours < want-1 || *d.CycleHours > want+1 {
		t.Fatalf("cycle_hours = %v, want ~%v", *d.CycleHours, want)
	}
	if d.LastActivityAt == nil || *d.LastActivityAt != flowAgo(10) {
		t.Fatalf("last_activity_at = %v, want the 10d-ago item stamp to win", d.LastActivityAt)
	}

	// A comment newer than everything else wins the activity max.
	d = Derive(DeriveInput{
		Changelog:       []ChangeEntry{{ID: "h1", At: flowAgo(5), Field: "status", FromID: "1", ToID: "3"}},
		Categories:      cats,
		CurrentCategory: "inprogress",
		Comments:        []Comment{{CreatedAt: flowAgo(2)}},
	})
	if d.LastActivityAt == nil || *d.LastActivityAt != flowAgo(2) {
		t.Fatalf("last_activity_at = %v, want the 2d-ago comment", d.LastActivityAt)
	}

	// Never in progress: no started_at, and cycle_hours stays nil with it.
	d = Derive(DeriveInput{
		Changelog:       []ChangeEntry{{ID: "h1", At: flowAgo(5), Field: "status", FromID: "1", ToID: "5"}},
		Categories:      cats,
		CurrentCategory: "done",
	})
	if d.StartedAt != nil || d.CycleHours != nil {
		t.Fatalf("never-inprogress issue derived started=%v cycle=%v, want nil/nil", d.StartedAt, d.CycleHours)
	}

	// Reopened and not re-finished: resolved_at is nulled, so cycle_hours is
	// nil — a reopened span must not drag a percentile down.
	d = Derive(DeriveInput{
		Changelog: []ChangeEntry{
			{ID: "h1", At: flowAgo(40), Field: "status", FromID: "1", ToID: "3"},
			{ID: "h2", At: flowAgo(30), Field: "status", FromID: "3", ToID: "5"},
			{ID: "h3", At: flowAgo(3), Field: "status", FromID: "5", ToID: "3"},
		},
		Categories:      cats,
		CurrentCategory: "inprogress",
	})
	if d.CycleHours != nil {
		t.Fatalf("cycle_hours = %v on a reopened, unfinished issue, want nil", *d.CycleHours)
	}

	// Nothing observed at all: last_activity_at is nil, not an empty string.
	d = Derive(DeriveInput{CurrentCategory: "new"})
	if d.LastActivityAt != nil {
		t.Fatalf("last_activity_at = %v with no entries, comments or stamp, want nil", *d.LastActivityAt)
	}
}

// seedFlowV42 builds a v42 mirror by hand (the TestMigrateV40 pattern) with
// three issues exercising the backfill, and returns the path. status_catalog
// stays empty — the shipped fixture's state — so category resolution runs
// through the issue-row reconstruction.
func seedFlowV42(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gadak.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 42; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 42`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sources (id, kind) VALUES ('jira', 'jira')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	ins := func(ext, key, statusID, category, updated string) {
		t.Helper()
		itemID := "jira:" + ext
		if _, err := raw.Exec(`
			INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at)
			VALUES (?, 'jira', 'issue', ?, ?, ?, '2026-01-01', ?, '2026-01-02')`,
			itemID, ext, key, key, updated, updated); err != nil {
			raw.Close()
			t.Fatal(err)
		}
		if _, err := raw.Exec(`
			INSERT INTO issues_raw (item_id, key, project_key, status_id, status_category, priority_rank, reopen_count, comment_count, raw)
			VALUES (?, ?, 'STD', ?, ?, 0, 0, 0, '{}')`,
			itemID, key, statusID, category); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	log := func(ext, id, at, from, to string) {
		t.Helper()
		if _, err := raw.Exec(`
			INSERT INTO changelog (id, item_id, at, field, from_id, to_id)
			VALUES (?, ?, ?, 'status', ?, ?)`,
			"jira:"+id, "jira:"+ext, at, from, to); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	comment := func(ext, id, at string) {
		t.Helper()
		if _, err := raw.Exec(`
			INSERT INTO comments (id, item_id, created_at)
			VALUES (?, ?, ?)`, "jira:"+id, "jira:"+ext, at); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	link := func(ext, typ, dir, target string) {
		t.Helper()
		if _, err := raw.Exec(`
			INSERT INTO links (item_id, type, direction, target_key) VALUES (?, ?, ?, ?)`,
			"jira:"+ext, typ, dir, target); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}

	// STD-1: in progress, started 40d ago, last touched by its own transition.
	ins("1", "STD-1", "3", "inprogress", flowAgo(40))
	log("1", "m1", flowAgo(40), "1", "3")
	// STD-2: done — started 40d ago, resolved 2d ago, a comment 3d ago and
	// the item stamp 2d ago both outrank the transitions.
	ins("2", "STD-2", "5", "done", flowAgo(2))
	log("2", "m2", flowAgo(40), "1", "3")
	log("2", "m3", flowAgo(2), "3", "5")
	comment("2", "c1", flowAgo(3))
	// STD-3: never in progress — started_at/cycle_hours NULL, and
	// last_activity_at is its item stamp alone (UpdatedAt counts as activity).
	ins("3", "STD-3", "1", "new", "2026-01-01")

	// STD-3's inward links: a blocking one at an unfinished in-mirror target
	// (counts), one at a done target (does not), one at a target outside the
	// mirror (does not — unknown must not hold work back), and a Relates one
	// (not a blocking type).
	link("3", "Blocks", "inward", "STD-1")
	link("3", "Blocks", "inward", "STD-2")
	link("3", "Blocks", "inward", "MISSING-9")
	link("3", "Relates", "inward", "STD-1")

	raw.Close()
	return path
}

// flowRow reads one issue's flow columns.
func flowRow(t *testing.T, db *DB, key string) (started, activity string, cycle sql.NullFloat64, blockers int) {
	t.Helper()
	err := db.QueryRow(`
		SELECT COALESCE(started_at,''), COALESCE(last_activity_at,''), cycle_hours, open_blockers
		FROM issues_raw WHERE key = ?`, key).
		Scan(&started, &activity, &cycle, &blockers)
	if err != nil {
		t.Fatal(err)
	}
	return
}

// TestMigrateV43BackfillsFlow seeds a v42 mirror and asserts Open's
// migration derives every flow column from rows the mirror already holds.
// FAIL-first: seeded at v42, the same SELECT fails with "no such column:
// started_at" before schemaV43 runs (drop the migration from the slice and
// this test cannot even reach its assertions).
func TestMigrateV43BackfillsFlow(t *testing.T) {
	db, err := Open(seedFlowV42(t))
	if err != nil {
		t.Fatalf("migrate to v43: %v", err)
	}
	defer db.Close()

	started, activity, cycle, blockers := flowRow(t, db, "STD-1")
	if started != flowAgo(40) {
		t.Fatalf("STD-1 started_at = %q, want the 40d-ago transition", started)
	}
	if cycle.Valid && cycle.Float64 != 0 {
		t.Fatalf("STD-1 cycle_hours = %v, want NULL for an unfinished issue", cycle.Float64)
	}
	if activity != flowAgo(40) {
		t.Fatalf("STD-1 last_activity_at = %q, want its transition stamp", activity)
	}
	if blockers != 0 {
		t.Fatalf("STD-1 open_blockers = %d, want 0", blockers)
	}

	started, activity, cycle, blockers = flowRow(t, db, "STD-2")
	if started != flowAgo(40) {
		t.Fatalf("STD-2 started_at = %q, want the 40d-ago transition", started)
	}
	want := 38 * 24.0
	if !cycle.Valid || cycle.Float64 < want-1 || cycle.Float64 > want+1 {
		t.Fatalf("STD-2 cycle_hours = %+v, want ~%v", cycle, want)
	}
	if activity != flowAgo(2) {
		t.Fatalf("STD-2 last_activity_at = %q, want the 2d-ago item stamp", activity)
	}
	if blockers != 0 {
		t.Fatalf("STD-2 open_blockers = %d, want 0", blockers)
	}

	started, activity, cycle, blockers = flowRow(t, db, "STD-3")
	if started != "" {
		t.Fatalf("STD-3 started_at = %q, want empty (never in progress)", started)
	}
	if activity != "2026-01-01" {
		t.Fatalf("STD-3 last_activity_at = %q, want its item stamp — UpdatedAt counts as activity", activity)
	}
	// Exactly the STD-1 link counts: done target, missing target and Relates
	// all disqualify. The catalog is empty, so the literal-'Blocks' fallback
	// resolved the type — the shipped fixture's exact path.
	if blockers != 1 {
		t.Fatalf("STD-3 open_blockers = %d, want 1 (unfinished in-mirror Blocks target only)", blockers)
	}
}

// TestBackfillFlowWithStatusCatalog runs the same backfill with
// status_catalog filled — the path a real synced mirror takes (a sync fills
// the table). STD-2's done transition is rewritten to an id only the catalog
// maps (7 → done), so without the catalog the entry resolves to nothing and
// cycle_hours stays NULL: the assertion discriminates the two resolvers.
func TestBackfillFlowWithStatusCatalog(t *testing.T) {
	path := seedFlowV42(t)
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`UPDATE changelog SET to_id = '7' WHERE id = 'jira:m3'`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO status_catalog (source_id, status_id, category) VALUES
		('jira', '1', 'new'), ('jira', '3', 'inprogress'), ('jira', '7', 'done')`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	started, activity, cycle, _ := flowRow(t, db, "STD-2")
	if started != flowAgo(40) {
		t.Fatalf("STD-2 started_at = %q, want the 40d-ago transition", started)
	}
	want := 38 * 24.0
	if !cycle.Valid || cycle.Float64 < want-1 || cycle.Float64 > want+1 {
		t.Fatalf("STD-2 cycle_hours = %+v, want ~%v — the catalog did not resolve the done id", cycle, want)
	}
	if activity != flowAgo(2) {
		t.Fatalf("STD-2 last_activity_at = %q, want the 2d-ago item stamp", activity)
	}
}

// TestRecomputeOpenBlockersResolvesCatalogNames covers all three branches of
// blockingLinkTypeNames: a localized catalog name matched by its outward
// phrase, a catalog with no blocking type (blocks nothing — empty answer,
// not the fallback), and an empty catalog (the documented 'Blocks' literal).
// FAIL-first: against pre-v43 code there is no link_types table and no
// recompute to call.
func TestRecomputeOpenBlockersResolvesCatalogNames(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	ins := func(src, ext, key, category string) {
		t.Helper()
		if err := db.UpsertSource(ctx, Source{ID: src, Kind: "jira"}); err != nil {
			t.Fatal(err)
		}
		if err := db.write(ctx, func(tx *sql.Tx) error {
			itemID := src + ":" + ext
			if _, err := tx.Exec(`
				INSERT INTO items (id, source_id, kind, external_id, key, created_at, updated_at, synced_at)
				VALUES (?, ?, 'issue', ?, ?, '2026-01-01', '2026-01-02', '2026-01-02')`,
				itemID, src, ext, key); err != nil {
				return err
			}
			_, err := tx.Exec(`
				INSERT INTO issues_raw (item_id, key, project_key, status_category, priority_rank, reopen_count, comment_count, raw)
				VALUES (?, ?, 'STD', ?, 0, 0, 0, '{}')`, itemID, key, category)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	link := func(src, ext, typ, target string) {
		t.Helper()
		if err := db.write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO links (item_id, type, direction, target_key) VALUES (?, ?, 'inward', ?)`,
				src+":"+ext, typ, target)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	catalog := func(src string, rows ...LinkType) {
		t.Helper()
		if err := db.write(ctx, func(tx *sql.Tx) error {
			return cacheLinkTypeCatalog(tx, Batch{Records: []IssueRecord{{Item: Item{SourceID: src}}}, LinkTypes: rows})
		}); err != nil {
			t.Fatal(err)
		}
	}
	blockers := func(key string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT open_blockers FROM issues_raw WHERE key = ?`, key).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Source one: a localized catalog. The blocking type's name is Korean;
	// only the outward phrase 'blocks' identifies it. A display-name match
	// would find nothing.
	ins("site1", "1", "LOC-1", "inprogress")
	ins("site1", "2", "LOC-2", "new")
	catalog("site1",
		LinkType{ID: "10000", Name: "차단", Inward: "차단됨", Outward: "blocks"},
		LinkType{ID: "10001", Name: "관련", Inward: "관련됨", Outward: "relates to"},
	)
	link("site1", "2", "차단", "LOC-1")
	link("site1", "2", "관련", "LOC-1")

	// Source two: a catalog with no blocking type at all. An inward link
	// literally named 'Blocks' must still block nothing — the catalog
	// answered, and its answer is "nothing blocks".
	ins("site2", "1", "NBL-1", "inprogress")
	ins("site2", "2", "NBL-2", "new")
	catalog("site2", LinkType{ID: "10001", Name: "Relates", Inward: "relates to", Outward: "relates to"})
	link("site2", "2", "Blocks", "NBL-1")

	// Source three: no catalog (a mirror that has not yet run a post-v43
	// sync). The documented fallback is the literal 'Blocks'.
	ins("site3", "1", "FALL-1", "inprogress")
	ins("site3", "2", "FALL-2", "new")
	link("site3", "2", "Blocks", "FALL-1")

	if err := db.RecomputeOpenBlockers(ctx); err != nil {
		t.Fatal(err)
	}
	if n := blockers("LOC-2"); n != 1 {
		t.Fatalf("LOC-2 open_blockers = %d, want 1 (localized type resolved by outward phrase)", n)
	}
	if n := blockers("NBL-2"); n != 0 {
		t.Fatalf("NBL-2 open_blockers = %d, want 0 (catalog present, no blocking type — never the fallback)", n)
	}
	if n := blockers("FALL-2"); n != 1 {
		t.Fatalf("FALL-2 open_blockers = %d, want 1 (empty catalog, documented 'Blocks' fallback)", n)
	}
}

// TestUpsertWidensOpenBlockers is C4 at the store level: a batch that
// carries the blocker but not the blocked issue must still move the blocked
// row's open_blockers. Batch 1 links STD-A (unfinished) as a blocker of
// STD-B; batch 2 carries only STD-A, now done — STD-B is not in it. The
// widening subquery (issues holding an inward link at a batch key) is the
// whole point; without it the blocked row's count would wait for a full
// sync. FAIL-first: pre-v43, `SELECT open_blockers` is "no such column".
func TestUpsertWidensOpenBlockers(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}

	rec := func(ext, key, category, updated string, links []Link) IssueRecord {
		return IssueRecord{
			Item: Item{
				ID: "jira:" + ext, SourceID: "jira", Kind: "issue", ExternalID: ext,
				Key: key, Title: key, CreatedAt: flowAgo(30), UpdatedAt: updated,
			},
			Issue: Issue{
				ProjectKey: "STD", StatusCategory: category,
			},
			Changelog: []ChangeEntry{
				{ID: ext + "-h1", At: flowAgo(20), Field: "status", FromID: "1", ToID: "3"},
			},
			Links: links,
		}
	}

	// Batch 1: both issues; STD-B carries its inward blocks link at STD-A.
	if _, err := db.UpsertIssues(ctx, Batch{
		Categories: fixtureCategories,
		Records: []IssueRecord{
			rec("1", "STD-A", "new", flowAgo(3), nil),
			rec("2", "STD-B", "new", flowAgo(3), []Link{{Type: "Blocks", Direction: "inward", TargetKey: "STD-A"}}),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if n := openBlockersOf(t, db, "STD-B"); n != 1 {
		t.Fatalf("STD-B open_blockers = %d after batch 1, want 1", n)
	}

	// Batch 2: only STD-A, now done. STD-B is not in the batch.
	if _, err := db.UpsertIssues(ctx, Batch{
		Categories: fixtureCategories,
		Records: []IssueRecord{
			rec("1", "STD-A", "done", flowAgo(1), nil),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if n := openBlockersOf(t, db, "STD-B"); n != 0 {
		t.Fatalf("STD-B open_blockers = %d after its blocker finished in a batch that did not carry it, want 0 — the widening failed (C4)", n)
	}

	// Deleting the blocker leaves the target outside the mirror, which is
	// not blocking either: the same widening runs on DeleteItems. The stamp
	// must differ from batch 2's — the change detector skips a record whose
	// updated_at did not move.
	if _, err := db.UpsertIssues(ctx, Batch{
		Categories: fixtureCategories,
		Records: []IssueRecord{
			rec("1", "STD-A", "new", flowAgo(0), nil),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if n := openBlockersOf(t, db, "STD-B"); n != 1 {
		t.Fatalf("STD-B open_blockers = %d after STD-A reopened, want 1", n)
	}
	if _, err := db.DeleteItems(ctx, "jira", []string{"STD-A"}); err != nil {
		t.Fatal(err)
	}
	if n := openBlockersOf(t, db, "STD-B"); n != 0 {
		t.Fatalf("STD-B open_blockers = %d after its blocker left the mirror, want 0 (a target outside the mirror is not blocking)", n)
	}
}

func openBlockersOf(t *testing.T, db *DB, key string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT open_blockers FROM issues_raw WHERE key = ?`, key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestCacheLinkTypeCatalog covers the catalog cache: rows merge by
// (source, id), an empty id is skipped (no join key), and the cache is
// scoped by the batch's first record. FAIL-first: no link_types table
// exists before v43.
func TestCacheLinkTypeCatalog(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	b := Batch{
		Records: []IssueRecord{{Item: Item{SourceID: "jira"}}},
		LinkTypes: []LinkType{
			{ID: "10000", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
			{ID: "", Name: "nameless", Outward: "phantom"}, // skipped: no join key
		},
	}
	if err := db.write(ctx, func(tx *sql.Tx) error { return cacheLinkTypeCatalog(tx, b) }); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM link_types`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("link_types holds %d rows, want 1 (empty id skipped)", n)
	}

	// A later run renames the type: the merge updates in place.
	b.LinkTypes = []LinkType{{ID: "10000", Name: "차단", Inward: "차단됨", Outward: "blocks"}}
	if err := db.write(ctx, func(tx *sql.Tx) error { return cacheLinkTypeCatalog(tx, b) }); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM link_types WHERE source_id = 'jira' AND id = '10000'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "차단" {
		t.Fatalf("merged name = %q, want the renamed value", name)
	}

	// Another source's catalog does not leak into this one's rows.
	b2 := Batch{
		Records:   []IssueRecord{{Item: Item{SourceID: "linear"}}},
		LinkTypes: []LinkType{{ID: "blocks", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"}},
	}
	if err := db.write(ctx, func(tx *sql.Tx) error { return cacheLinkTypeCatalog(tx, b2) }); err != nil {
		t.Fatal(err)
	}
	var jira, linear int
	if err := db.QueryRow(`SELECT COUNT(*) FROM link_types WHERE source_id = 'jira'`).Scan(&jira); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM link_types WHERE source_id = 'linear'`).Scan(&linear); err != nil {
		t.Fatal(err)
	}
	if jira != 1 || linear != 1 {
		t.Fatalf("catalog scoping broke: jira=%d linear=%d, want 1/1", jira, linear)
	}
}
