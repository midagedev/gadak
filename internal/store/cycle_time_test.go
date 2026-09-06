package store

/*
 * CycleTimeP85Hours — the learned half of the stale threshold.
 *
 * C1–C6 ↔ assertion table (task contract):
 *   C1 exact p85 on a known fixture, samples count .... TestExactNearestRankP85
 *                                                     TestEdgeCasesSkippedIssues
 *   C1 sample count below ten is visible to the
 *      caller (the server's flowFields gate is a
 *      pure function of it) .......................... TestFewerThanTenSamples
 *   C1/C6 catalog empty → StatusCategories fallback
 *      resolves by id ................................ TestEmptyCatalogFallsBackToIssueRows
 *   C6 categories by id through status_catalog,
 *      never a status display name ................... every test seeds ids 1/3/5;
 *      the display names ("할 일" etc.) never match
 *   C2/C3/C4/C5 are client-side (view-config.test.ts,
 *      e2e/stale-threshold.spec.ts).
 *
 * FAIL-first: every test here failed before cycle_time.go existed
 * (undefined: db.CycleTimeP85Hours — compile error captured in the round
 * report), and TestEmptyCatalogFallsBackToIssueRows additionally failed
 * against the first draft that read status_catalog only, because the
 * shipped demo fixture seeds no catalog rows at all.
 */

import (
	"context"
	"testing"
	"time"
)

// cycleBase is a fixed anchor; cycle fixtures stamp changelog entries as
// RFC3339 seconds, which parseStamp accepts (its ISOMilli fast path misses
// and the RFC3339 fallback catches).
var cycleBase = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func cycleStamp(offset time.Duration) string {
	return cycleBase.Add(offset).Format(time.RFC3339)
}

// seedCycleIssue writes one issue whose changelog went 1(new)→3(inprogress)
// at start, then 3→5(done) at start+cycle. Status display names are Korean
// on purpose: nothing below may resolve a category through them (C6).
func seedCycleIssue(t *testing.T, db *DB, n int, start time.Duration, cycle time.Duration, withCatalog bool) {
	t.Helper()
	started := cycleStamp(start)
	entries := []ChangeEntry{
		{ID: "h1", At: started, Author: "Kim", Field: "status", FromValue: "할 일", FromID: "1", ToValue: "진행 중", ToID: "3"},
		{ID: "h2", At: cycleStamp(start + cycle), Author: "Kim", Field: "status", FromValue: "진행 중", FromID: "3", ToValue: "완료", ToID: "5"},
	}
	if cycle < 0 {
		// Negative cycle = done before `since`; drop the in-progress leg so the
		// entry order still reads chronologically.
		entries = []ChangeEntry{
			{ID: "h1", At: cycleStamp(start + cycle), Author: "Kim", Field: "status", FromValue: "할 일", FromID: "1", ToValue: "완료", ToID: "5"},
		}
	}
	batch := Batch{
		Priorities: []string{"Highest", "High", "Medium", "Low"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:1" + itoa(n), SourceID: "jira", Kind: "issue", ExternalID: itoa(n),
				Key: "NMB-1" + itoa(n), Title: "cycle " + itoa(n),
				CreatedAt: started, UpdatedAt: started,
			},
			Issue:     Issue{ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10001", Status: "완료", StatusID: "5", StatusCategory: "done"},
			Changelog: entries,
		}},
	}
	if withCatalog {
		batch.Categories = fixtureCategories
	}
	if _, err := db.UpsertIssues(context.Background(), batch); err != nil {
		t.Fatalf("seed NMB-1%d: %v", n, err)
	}
}

// (itoa is epic_test.go's helper — same package, reused as-is.)

// TestExactNearestRankP85: twenty issues with cycle times 1h..20h. The rank
// is the retro integer form (85*20+99)/100 = 17, so the p85 is the 17th
// smallest cycle — 17h — not the interpolated 17.85 a float percentile
// would report.
func TestExactNearestRankP85(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 20; i++ {
		seedCycleIssue(t, db, i, 0, time.Duration(i)*time.Hour, true)
	}
	p85, samples, err := db.CycleTimeP85Hours(ctx, cycleBase.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if samples != 20 {
		t.Fatalf("samples = %d, want 20", samples)
	}
	if p85 != 17 {
		t.Fatalf("p85 = %v, want 17 (nearest rank (85*20+99)/100 = 17)", p85)
	}
}

// TestFewerThanTenSamples: nine finished issues still answer with their own
// p85 and the true sample count; the "absent below ten" gate is the server's
// flowFields, a pure function of this count (read.go).
func TestFewerThanTenSamples(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 9; i++ {
		seedCycleIssue(t, db, i, 0, time.Duration(i)*time.Hour, true)
	}
	p85, samples, err := db.CycleTimeP85Hours(ctx, cycleBase.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if samples != 9 {
		t.Fatalf("samples = %d, want 9", samples)
	}
	if p85 != 8 {
		t.Fatalf("p85 = %v, want 8 (nearest rank (85*9+99)/100 = 8)", p85)
	}
}

// TestEmptyCatalogFallsBackToIssueRows: same twenty-issue seed, but the
// batch carries no Categories so status_catalog stays empty — the state the
// shipped demo fixture is in. The walk must resolve ids through
// StatusCategories(), whose reconstruction only sees ids some current issue
// row holds — so the fixture needs one in-progress row carrying status id 3
// for the starts to resolve at all, exactly as a real workspace would have.
// That row has no done entry, so it joins the walk but not the sample set.
func TestEmptyCatalogFallsBackToIssueRows(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 20; i++ {
		seedCycleIssue(t, db, i, 0, time.Duration(i)*time.Hour, false)
	}
	started := cycleStamp(time.Hour)
	if _, err := db.UpsertIssues(ctx, Batch{
		Priorities: []string{"Highest", "High", "Medium", "Low"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:991", SourceID: "jira", Kind: "issue", ExternalID: "991",
				Key: "NMB-991", Title: "still going", CreatedAt: started, UpdatedAt: started,
			},
			Issue:     Issue{ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10001", Status: "진행 중", StatusID: "3", StatusCategory: "inprogress"},
			Changelog: []ChangeEntry{{ID: "h1", At: started, Author: "Kim", Field: "status", FromID: "1", ToID: "3"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	var catalogRows int
	if err := db.sql.QueryRowContext(ctx, `SELECT count(*) FROM status_catalog`).Scan(&catalogRows); err != nil {
		t.Fatal(err)
	}
	if catalogRows != 0 {
		t.Fatalf("status_catalog has %d rows; the fallback path is not what this test exercised", catalogRows)
	}
	p85, samples, err := db.CycleTimeP85Hours(ctx, cycleBase.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if samples != 20 || p85 != 17 {
		t.Fatalf("fallback path: samples=%d p85=%v, want 20/17 (same as the catalog path)", samples, p85)
	}
}

// TestEdgeCasesSkippedIssues pins which issues have no measurable cycle:
//   - done at or after `since` but never in progress            → skipped
//   - finished before `since`                                   → skipped
//   - reopened after finishing and never re-finished (the only
//     done entry precedes the first in-progress entry)          → skipped
//   - done, reopened, done again: the FIRST in-progress entry to
//     the LATEST done entry is the cycle (whole history)        → 30h
//   - an ordinary control issue                                 → 2h
//
// Two samples {2h, 30h}: rank (85*2+99)/100 = 2 → p85 = 30h.
func TestEdgeCasesSkippedIssues(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	seed := func(n int, entries []ChangeEntry) {
		t.Helper()
		at := entries[0].At
		if _, err := db.UpsertIssues(ctx, Batch{
			Categories: fixtureCategories,
			Priorities: []string{"Highest", "High", "Medium", "Low"},
			Records: []IssueRecord{{
				Item: Item{
					ID: "jira:2" + itoa(n), SourceID: "jira", Kind: "issue", ExternalID: itoa(n),
					Key: "NMB-2" + itoa(n), Title: "edge " + itoa(n), CreatedAt: at, UpdatedAt: at,
				},
				Issue:     Issue{ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10001", Status: "완료", StatusID: "5", StatusCategory: "done"},
				Changelog: entries,
			}},
		}); err != nil {
			t.Fatalf("seed NMB-2%d: %v", n, err)
		}
	}
	st := func(key, at string, fromID, toID string) ChangeEntry {
		return ChangeEntry{ID: key, At: at, Author: "Kim", Field: "status", FromID: fromID, ToID: toID}
	}
	// 1: new → done directly, never in progress.
	seed(1, []ChangeEntry{st("e1", cycleStamp(time.Hour), "1", "5")})
	// 2: finished two days before `since`.
	seed(2, []ChangeEntry{
		st("e1", cycleStamp(-48*time.Hour), "1", "3"),
		st("e2", cycleStamp(-47*time.Hour), "3", "5"),
	})
	// 3: straight to done at 2h, reopened into progress at 5h, never
	// re-finished — the done entry precedes the first in-progress entry, so
	// the "cycle" runs backwards and the issue is skipped.
	seed(3, []ChangeEntry{
		st("e1", cycleStamp(2*time.Hour), "1", "5"),
		st("e2", cycleStamp(5*time.Hour), "5", "3"),
	})
	// 4: in progress at 0h, done at 10h, reopened at 20h, done again at 30h.
	seed(4, []ChangeEntry{
		st("e1", cycleStamp(0), "1", "3"),
		st("e2", cycleStamp(10*time.Hour), "3", "5"),
		st("e3", cycleStamp(20*time.Hour), "5", "3"),
		st("e4", cycleStamp(30*time.Hour), "3", "5"),
	})
	// 5: control — in progress at 0h, done at 2h.
	seed(5, []ChangeEntry{
		st("e1", cycleStamp(0), "1", "3"),
		st("e2", cycleStamp(2*time.Hour), "3", "5"),
	})
	p85, samples, err := db.CycleTimeP85Hours(ctx, cycleBase)
	if err != nil {
		t.Fatal(err)
	}
	if samples != 2 {
		t.Fatalf("samples = %d, want 2 (issues 1-3 have no measurable cycle)", samples)
	}
	if p85 != 30 {
		t.Fatalf("p85 = %v, want 30 (rank (85*2+99)/100 = 2 over {2h, 30h})", p85)
	}
}
