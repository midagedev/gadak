package store

/*
 * SprintBurnup — the daily scope/started/completed reconstruction
 * (GDK-1710). The assertion table below is the contract the sparkline and
 * `gadak sprint show` both read:
 *
 *   scope grows on arrival and dips on departure  ... the 03-04 and 03-09
 *                                                     columns (NMB-2 added,
 *                                                     NMB-4 pulled out)
 *   completed is a day's state, not a running
 *   tally — a reopen takes it back the next day  ... NMB-3: 4 on 03-05,
 *                                                     3 from 03-06 on
 *   a departure removes the issue from every
 *   line, completed included                       ... NMB-4 leaves 03-09
 *   the Cloud whole-membership shape ("7" → "7, 9")
 *   is not a departure                             ... NMB-5 stays all days
 *   an event counts from the day its stamp lands
 *   in, 23:59 included                             ... NMB-2 completes 03-04
 *   a trimmed history still counts via the
 *   current row                                    ... NMB-6 (no changelog,
 *                                                     sprint_id set, born done)
 *   categories resolve by status id, display
 *   names are Korean and never consulted           ... every status seed
 *
 * FAIL-first on the pre-change tree: the file does not compile —
 * undefined: db.SprintBurnup — the same form cycle_time_test.go records.
 */

import (
	"context"
	"testing"
	"time"
)

// burnupNow sits after the sprint's complete_at so the series is bounded by
// the window, not by today — the frozen-clock half of the contract.
var burnupNow = time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

func burnupStamp(day, clock string) string {
	return "2026-03-" + day + "T" + clock + "Z"
}

func seedBurnupIssue(t *testing.T, db *DB, n, key string, sprintNow bool, entries ...ChangeEntry) {
	t.Helper()
	seven := int64(7)
	issue := Issue{ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10001",
		Status: "완료", StatusID: "5", StatusCategory: "done"}
	if sprintNow {
		// The trimmed-history row: sitting in the sprint now, no event that
		// says when it arrived.
		issue.SprintID = &seven
	}
	rec := IssueRecord{
		Item: Item{
			ID: "jira:" + n, SourceID: "jira", Kind: "issue", ExternalID: n,
			Key: key, Title: "burnup " + key,
			CreatedAt: "2026-02-20T09:00:00Z",
			UpdatedAt: "2026-03-10T09:00:00Z",
		},
		Issue:     issue,
		Changelog: entries,
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Categories: fixtureCategories, Records: []IssueRecord{rec}}); err != nil {
		t.Fatalf("seed %s: %v", key, err)
	}
}

func TestSprintBurnupReplaysStatePerDay(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceAgile(ctx, "jira",
		[]BoardRow{{ID: 1, Name: "NMB board", Type: "scrum"}},
		[]SprintRow{{ID: 7, BoardID: 1, Name: "Sprint 7", State: "closed",
			StartAt: "2026-03-02T10:00:00.000Z", EndAt: "2026-03-13T10:00:00.000Z",
			CompleteAt: "2026-03-13T18:00:00.000Z"}}); err != nil {
		t.Fatal(err)
	}

	seedBurnupIssue(t, db, "101", "NMB-1", false,
		ChangeEntry{ID: "s1", At: burnupStamp("01", "12:00:00"), Field: "sprint", ToID: "7", ToValue: "Sprint 7"},
		ChangeEntry{ID: "h1", At: burnupStamp("03", "09:00:00"), Field: "status", FromID: "1", ToID: "3", FromValue: "할 일", ToValue: "진행 중"},
		ChangeEntry{ID: "h2", At: burnupStamp("05", "09:00:00"), Field: "status", FromID: "3", ToID: "5", FromValue: "진행 중", ToValue: "완료"},
	)
	// Added mid-sprint and finished the same day at 23:59 — the day-bucket
	// boundary case: both count from 03-04, not 03-05.
	seedBurnupIssue(t, db, "102", "NMB-2", false,
		ChangeEntry{ID: "s2", At: burnupStamp("04", "08:00:00"), Field: "sprint", ToID: "7", ToValue: "Sprint 7"},
		ChangeEntry{ID: "h3", At: burnupStamp("04", "09:00:00"), Field: "status", FromID: "1", ToID: "3", FromValue: "할 일", ToValue: "진행 중"},
		ChangeEntry{ID: "h4", At: burnupStamp("04", "23:59:00"), Field: "status", FromID: "3", ToID: "5", FromValue: "진행 중", ToValue: "완료"},
	)
	// Reopened inside the sprint: completed is a state, so it comes back off
	// the line the day of the reopen.
	seedBurnupIssue(t, db, "103", "NMB-3", false,
		ChangeEntry{ID: "s3", At: "2026-02-28T09:00:00Z", Field: "sprint", ToID: "7", ToValue: "Sprint 7"},
		ChangeEntry{ID: "h5", At: burnupStamp("02", "10:00:00"), Field: "status", FromID: "1", ToID: "3", FromValue: "할 일", ToValue: "진행 중"},
		ChangeEntry{ID: "h6", At: burnupStamp("05", "10:00:00"), Field: "status", FromID: "3", ToID: "5", FromValue: "진행 중", ToValue: "완료"},
		ChangeEntry{ID: "h7", At: burnupStamp("06", "10:00:00"), Field: "status", FromID: "5", ToID: "3", FromValue: "완료", ToValue: "진행 중"},
	)
	// Pulled out of the sprint after finishing: scope and completed both
	// drop on the departure day.
	seedBurnupIssue(t, db, "104", "NMB-4", false,
		ChangeEntry{ID: "s4", At: "2026-02-28T09:00:00Z", Field: "sprint", ToID: "7", ToValue: "Sprint 7"},
		ChangeEntry{ID: "s5", At: burnupStamp("09", "10:00:00"), Field: "sprint", FromID: "7", ToID: "", FromValue: "Sprint 7"},
		ChangeEntry{ID: "h8", At: burnupStamp("06", "09:00:00"), Field: "status", FromID: "1", ToID: "3", FromValue: "할 일", ToValue: "진행 중"},
		ChangeEntry{ID: "h9", At: burnupStamp("07", "09:00:00"), Field: "status", FromID: "3", ToID: "5", FromValue: "진행 중", ToValue: "완료"},
	)
	// Cloud's whole-membership shape: "7" → "7, 9" adds another sprint and
	// is not a departure from this one.
	seedBurnupIssue(t, db, "105", "NMB-5", false,
		ChangeEntry{ID: "s6", At: burnupStamp("01", "12:00:00"), Field: "sprint", ToID: "7", ToValue: "Sprint 7"},
		ChangeEntry{ID: "s7", At: burnupStamp("08", "10:00:00"), Field: "sprint", FromID: "7", ToID: "7, 9", FromValue: "Sprint 7", ToValue: "Sprint 7, Sprint 9"},
		ChangeEntry{ID: "h10", At: burnupStamp("03", "11:00:00"), Field: "status", FromID: "1", ToID: "3", FromValue: "할 일", ToValue: "진행 중"},
		ChangeEntry{ID: "h11", At: burnupStamp("10", "11:00:00"), Field: "status", FromID: "3", ToID: "5", FromValue: "진행 중", ToValue: "완료"},
	)
	// Trimmed history: in the sprint by the current row alone, done since
	// before the window, never had a transition the replay could point at.
	seedBurnupIssue(t, db, "106", "NMB-6", true)

	doc, err := db.SprintBurnup(ctx, 7, burnupNow)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "Sprint 7" || doc.State != "closed" || doc.SourceKind != "jira" {
		t.Fatalf("doc header = %q/%q/%q", doc.Name, doc.State, doc.SourceKind)
	}
	want := []BurnupDay{
		{"2026-03-02", 5, 2, 1},
		{"2026-03-03", 5, 4, 1},
		{"2026-03-04", 6, 5, 2},
		{"2026-03-05", 6, 5, 4},
		{"2026-03-06", 6, 6, 3},
		{"2026-03-07", 6, 6, 4},
		{"2026-03-08", 6, 6, 4},
		{"2026-03-09", 5, 5, 3},
		{"2026-03-10", 5, 5, 4},
		{"2026-03-11", 5, 5, 4},
		{"2026-03-12", 5, 5, 4},
		{"2026-03-13", 5, 5, 4},
	}
	if len(doc.Days) != len(want) {
		t.Fatalf("days = %d, want %d: %+v", len(doc.Days), len(want), doc.Days)
	}
	for i, d := range doc.Days {
		if d != want[i] {
			t.Errorf("day %d = %+v, want %+v", i, d, want[i])
		}
	}
	// The stacking invariant the chart's bands rely on: completed rides on
	// started rides on scope, every day, or the stacked areas invert.
	for _, d := range doc.Days {
		if d.Completed > d.Started || d.Started > d.Scope {
			t.Errorf("%s breaks the stack: %+v", d.Date, d)
		}
	}
}

// A Linear cycle has no changelog behind it, so the series is not a worse
// estimate — it is a question this origin cannot answer. The doc says which
// kind the sprint's own source is; the sentence ("no history") is the
// caller's, and retro.OriginSuppliesChangelog owns the judgement.
func TestSprintBurnupNamesTheLinearSourceKind(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "lin", Kind: "linear", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceAgile(ctx, "lin", nil,
		[]SprintRow{{ID: 3, BoardID: 1, Name: "Cycle 3", State: "active",
			StartAt: "2026-03-02T10:00:00.000Z"}}); err != nil {
		t.Fatal(err)
	}
	doc, err := db.SprintBurnup(ctx, 3, burnupNow)
	if err != nil {
		t.Fatal(err)
	}
	if doc.SourceKind != "linear" {
		t.Fatalf("source_kind = %q, want linear — the caller cannot ask a history-less origin for a burn-up", doc.SourceKind)
	}
	for _, d := range doc.Days {
		if d.Scope != 0 || d.Started != 0 || d.Completed != 0 {
			t.Errorf("a cycle with no changelog produced numbers: %+v", d)
		}
	}
}

func TestSprintBurnupNotFound(t *testing.T) {
	db := openTemp(t)
	if _, err := db.SprintBurnup(context.Background(), 999, burnupNow); err != ErrSprintNotFound {
		t.Fatalf("want ErrSprintNotFound, got %v", err)
	}
}

// A future sprint has no lived day yet: the honest series is empty, and the
// caller must not draw a chart of zeroes that reads as "nothing happened".
func TestSprintBurnupFutureSprintHasNoDays(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceAgile(ctx, "jira", nil,
		[]SprintRow{{ID: 9, BoardID: 1, Name: "Sprint 9", State: "future",
			StartAt: "2026-04-06T10:00:00.000Z", EndAt: "2026-04-17T10:00:00.000Z"}}); err != nil {
		t.Fatal(err)
	}
	doc, err := db.SprintBurnup(ctx, 9, burnupNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Days) != 0 {
		t.Fatalf("a future sprint answered %d days, want 0", len(doc.Days))
	}
}
