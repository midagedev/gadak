package retro

// internal/retro/materials.go — contract ↔ assertion map. Every field of the
// wire contract gets two cases: one where a hand-planted row makes the value
// appear, and one where its absence makes the field empty rather than wrong.
// The demo fixture is deliberately not the instrument here: its status
// transitions sit one second apart with order reversals (GDK-1720), so a
// contract measured on it would be measuring that bug.
//
//	M1  aging items + p85, oldest first     → TestAgingIsTheInProgressTail
//	M1b aging empty when nothing in progress → TestAgingEmptyWithNothingInProgress
//	M2  events: every kind, at asc           → TestBucketEventsCarryEveryKind
//	M2b events cap + events_truncated        → TestBucketEventsCap
//	M3  surprises: reopened + reversal       → TestSurprisesReopenedAndReversal
//	M3b sprint-only kinds silent under weeks → TestSprintOnlySurprisesAreWeekSilent
//	M4  closed_by_type / _by_epic / unplanned
//	    / cycle_points all partition closed  → TestClosedDecompositionsPartitionClosed
//	M4b same on the demo fixture (the shipped
//	    sample, sums only)                   → TestClosedDecompositionsSumOnDemoFixture
//	M5  seen_not_moved / moved_not_seen      → TestSeenAndMovedSplitTheBucket
//	M5b no visits → both empty + a note      → TestSeenAndMovedEmptyWithoutVisits
//	M6  actions: metric parse, then/now      → TestActionsCarryTheirMetric
//	M6b no retro-action label → empty        → TestActionsEmptyWithoutLabel
//	M7  every material array is present in
//	    JSON, never null                     → TestMaterialArraysAreNeverNull
//	M8  definitions name every new field     → TestDefinitionsCoverTheMaterials
//
// FAIL-first for all of them: before materials.go the fields did not exist,
// so the file did not compile against the previous source.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// matNow is the pinned clock every fixture below is built around: a
// Wednesday noon, so Buckets(now, 7d) lays exactly two columns — the full
// week [03-02, 03-09) and the partial [03-09, now).
var matNow = time.Date(2026, 3, 11, 12, 0, 0, 0, time.Local)

// ts formats a fixture stamp the way the mirror stores them (UTC ISOMilli),
// from a local wall time — the same conversion a sync does.
func ts(t time.Time) string { return t.UTC().Format(config.ISOMilli) }

// day is a local instant inside March 2026, the fixture month.
func day(d, h int) time.Time { return time.Date(2026, 3, d, h, 0, 0, 0, time.Local) }

// matIssue is one hand-planted issue: the columns the materials read, with
// everything else left at its default.
type matIssue struct {
	key         string
	summary     string
	typeID      string
	typeName    string
	epicKey     string
	category    string
	created     time.Time
	updated     time.Time
	changedAt   time.Time
	resolvedAt  time.Time
	reopenReas  string
	cycleHours  float64
	reopenCount int
	labels      string
	body        string
	sprintID    int64
	carryover   int
	hierarchy   int
}

// matStatus is one status changelog row, by category rather than by id: the
// fixture names the categories it means and the helper maps them through the
// same status_catalog the report reads, so nothing here keys on a display
// name.
type matStatus struct {
	key      string
	at       time.Time
	from, to string // "" | "new" | "inprogress" | "done"
}

type matSprintLog struct {
	key            string
	at             time.Time
	fromID, toID   int64
	fromName, toNa string
}

type matComment struct {
	key    string
	at     time.Time
	author string
	body   string
}

type matVisit struct {
	key string
	at  time.Time
}

type matSprint struct {
	id, board  int64
	name       string
	start, end time.Time
}

type matFixture struct {
	issues    []matIssue
	statuses  []matStatus
	sprintLog []matSprintLog
	comments  []matComment
	visits    []matVisit
	sprints   []matSprint
}

// statusIDs are the fixture's three statuses, one per category. The report
// resolves them through status_catalog, which is exactly the point: a
// display name never enters this test.
var statusIDs = map[string]string{"new": "1", "inprogress": "3", "done": "10001"}

// buildMirror plants the fixture into a real mirror — store.Open runs the
// shipped migrations, so the schema under test is the schema that ships, not
// a hand-typed approximation of it — and returns a read-only handle with
// local.db attached, the same view Compute gets from the CLI.
func buildMirror(t *testing.T, f matFixture) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gadak.db")
	sdb, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	sdb.Close()

	w, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := w.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT OR REPLACE INTO sources (id, kind, base_url) VALUES ('jira','jira','https://example.invalid')`)
	for cat, id := range statusIDs {
		exec(`INSERT OR REPLACE INTO status_catalog (source_id, status_id, category) VALUES ('jira',?,?)`, id, cat)
	}
	itemID := func(key string) string { return "jira:" + key }
	for _, is := range f.issues {
		labels := is.labels
		if labels == "" {
			labels = "[]"
		}
		exec(`INSERT INTO items (id, source_id, kind, external_id, key, title, body_text)
			VALUES (?, 'jira', 'issue', ?, ?, ?, ?)`,
			itemID(is.key), is.key, is.key, is.summary, is.body)
		var resolved, changed any
		if !is.resolvedAt.IsZero() {
			resolved = ts(is.resolvedAt)
		}
		if !is.changedAt.IsZero() {
			changed = ts(is.changedAt)
		}
		var cycle any
		if is.cycleHours != 0 {
			cycle = is.cycleHours
		}
		var sprint any
		if is.sprintID != 0 {
			sprint = is.sprintID
		}
		exec(`INSERT INTO issues_raw (item_id, key, project_key, issue_type, issue_type_id,
			status_id, status_category, epic_key, created_at, updated_at, status_changed_at,
			resolved_at, reopen_count, reopen_reason, cycle_hours, labels, sprint_id,
			carryover_count, hierarchy_level)
			VALUES (?,?,'T',?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			itemID(is.key), is.key, is.typeName, is.typeID,
			statusIDs[is.category], is.category, is.epicKey,
			ts(is.created), ts(is.updated), changed, resolved,
			is.reopenCount, is.reopenReas, cycle, labels, sprint,
			is.carryover, is.hierarchy)
	}
	for i, s := range f.statuses {
		exec(`INSERT INTO changelog (id, item_id, at, author, author_id, field, from_id, to_id, from_value, to_value)
			VALUES (?,?,?,'Ada','ada','status',?,?,'','')`,
			fmt.Sprintf("s%03d", i), itemID(s.key), ts(s.at), statusIDs[s.from], statusIDs[s.to])
	}
	for i, s := range f.sprintLog {
		var from, to any
		if s.fromID != 0 {
			from = fmt.Sprint(s.fromID)
		}
		if s.toID != 0 {
			to = fmt.Sprint(s.toID)
		}
		exec(`INSERT INTO changelog (id, item_id, at, author, author_id, field, from_id, to_id, from_value, to_value)
			VALUES (?,?,?,'Ada','ada','sprint',?,?,?,?)`,
			fmt.Sprintf("p%03d", i), itemID(s.key), ts(s.at), from, to, s.fromName, s.toNa)
	}
	for i, c := range f.comments {
		exec(`INSERT INTO comments (id, item_id, author, author_id, body_text, created_at)
			VALUES (?,?,?,'ada',?,?)`,
			fmt.Sprintf("c%03d", i), itemID(c.key), c.author, c.body, ts(c.at))
	}
	for _, s := range f.sprints {
		exec(`INSERT OR REPLACE INTO boards (source_id, id, name, type, project_key)
			VALUES ('jira',?,?,'scrum','T')`, s.board, fmt.Sprintf("Board %d", s.board))
		var end any
		if !s.end.IsZero() {
			end = ts(s.end)
		}
		exec(`INSERT INTO sprints (source_id, id, board_id, name, state, start_at, end_at)
			VALUES ('jira',?,?,?,'active',?,?)`, s.id, s.board, s.name, ts(s.start), end)
	}
	w.Close()

	if err := store.EnsureLocal(path); err != nil {
		t.Fatalf("EnsureLocal: %v", err)
	}
	if len(f.visits) > 0 {
		l, err := sql.Open("sqlite", "file:"+filepath.Join(filepath.Dir(path), "local.db"))
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range f.visits {
			if _, err := l.Exec(`INSERT INTO visits (kind, key, viewed_at, origin_epoch, source)
				VALUES ('issue', ?, ?, 0, 'ui')`, v.key, ts(v.at)); err != nil {
				l.Close()
				t.Fatalf("insert visit: %v", err)
			}
		}
		l.Close()
	}
	db, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// baseFixture is the shared scenario: two week columns around matNow, with
// one issue for each shape the materials name.
//
//	B0 = [03-02, 03-09)   B1 = [03-09, 03-11 12:00) partial
//
//	T-1  created 03-03, started 03-04, resolved 03-05 in B0 — unplanned,
//	     a cycle sample (48h), type Bug under epic E-1
//	T-2  created 02-20, resolved 03-06 in B0 — planned, type Task, no epic
//	T-3  resolved 03-03 then reopened 03-07 in B0 — a reopened surprise
//	T-4  four status moves inside B0 — a reversal
//	T-5  in progress since 03-01, visited 03-04, no changelog in B0 —
//	     seen_not_moved, and the oldest aging row
//	T-6  in progress with no status_changed_at, updated 03-09 — aging falls
//	     back to updated_at
//	A-1  labelled retro-action, created 03-04 in B0, names the closed row
func baseFixture() matFixture {
	return matFixture{
		issues: []matIssue{
			{key: "T-1", summary: "one", typeID: "10004", typeName: "Bug", epicKey: "E-1",
				category: "done", created: day(3, 9), updated: day(5, 9),
				changedAt: day(5, 9), resolvedAt: day(5, 9), cycleHours: 48},
			{key: "T-2", summary: "two", typeID: "10001", typeName: "Task",
				category: "done", created: time.Date(2026, 2, 20, 9, 0, 0, 0, time.Local),
				updated: day(6, 9), changedAt: day(6, 9), resolvedAt: day(6, 9)},
			{key: "T-3", summary: "three", typeID: "10004", typeName: "Bug", epicKey: "E-1",
				category: "inprogress", created: day(1, 9), updated: day(7, 9),
				changedAt: day(7, 9), reopenCount: 1, reopenReas: "regressed in staging"},
			{key: "T-4", summary: "four", typeID: "10001", typeName: "Task",
				category: "new", created: day(2, 9), updated: day(6, 9), changedAt: day(6, 9)},
			{key: "T-5", summary: "five", typeID: "10001", typeName: "Task",
				category: "inprogress", created: day(1, 0), updated: day(1, 0), changedAt: day(1, 0)},
			{key: "T-6", summary: "six", typeID: "10001", typeName: "Task",
				category: "inprogress", created: day(1, 0), updated: day(9, 0)},
			{key: "E-1", summary: "the epic", typeID: "10000", typeName: "Epic",
				category: "new", created: day(1, 0), updated: day(1, 0), hierarchy: 1},
			{key: "A-1", summary: "shorten the tail", typeID: "10001", typeName: "Task",
				category: "new", created: day(4, 9), updated: day(4, 9),
				labels: `["retro-action"]`, body: "metric: closed\n\ncut the queue"},
		},
		statuses: []matStatus{
			{key: "T-1", at: day(4, 9), from: "new", to: "inprogress"},
			{key: "T-1", at: day(5, 9), from: "inprogress", to: "done"},
			{key: "T-2", at: day(6, 9), from: "inprogress", to: "done"},
			{key: "T-3", at: day(3, 9), from: "inprogress", to: "done"},
			{key: "T-3", at: day(7, 9), from: "done", to: "inprogress"},
			{key: "T-4", at: day(3, 1), from: "new", to: "inprogress"},
			{key: "T-4", at: day(3, 2), from: "inprogress", to: "new"},
			{key: "T-4", at: day(3, 3), from: "new", to: "inprogress"},
			{key: "T-4", at: day(3, 4), from: "inprogress", to: "new"},
		},
		comments: []matComment{{key: "T-1", at: day(4, 10), author: "Ada", body: "looking"}},
		visits:   []matVisit{{key: "T-5", at: day(4, 8)}},
	}
}

// matReport computes the pinned report over the fixture's two week columns.
func matReport(t *testing.T, f matFixture) Report {
	t.Helper()
	db := buildMirror(t, f)
	rep, err := Compute(context.Background(), db, store.FeedIdentity{}, 7*24*time.Hour, matNow, Options{})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if len(rep.Buckets) != 2 {
		t.Fatalf("want two buckets around the pinned clock, got %d", len(rep.Buckets))
	}
	return rep
}

func keysOf[T any](xs []T, f func(T) string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, f(x))
	}
	return out
}

/* ── M1 aging ── */

func TestAgingIsTheInProgressTail(t *testing.T) {
	rep := matReport(t, baseFixture())
	got := keysOf(rep.Aging.Items, func(a AgingItem) string { return a.Key })
	// T-5 (since 03-01 00:00), T-3 (since 03-07 09:00), T-6 (no status
	// change: updated_at 03-09 00:00) — days descending.
	want := []string{"T-5", "T-3", "T-6"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("aging items = %v, want %v (oldest first)", got, want)
	}
	if d := rep.Aging.Items[0].Days; d < 10.4 || d > 10.6 {
		t.Errorf("T-5 age = %v days, want ~10.5", d)
	}
	if rep.Aging.Items[0].Summary != "five" || rep.Aging.Items[0].IssueTypeID != "10001" {
		t.Errorf("aging row carries summary/type: %+v", rep.Aging.Items[0])
	}
	// The fallback row: no status_changed_at, so updated_at answers.
	if d := rep.Aging.Items[2].Days; d < 2.4 || d > 2.6 {
		t.Errorf("T-6 age = %v days, want ~2.5 from updated_at", d)
	}
	if rep.Aging.P85 == nil {
		t.Fatal("p85_days must exist when anything is in progress")
	}
	// Nearest-rank p85 of three samples is the largest.
	if *rep.Aging.P85 != rep.Aging.Items[0].Days {
		t.Errorf("p85 = %v, want the largest of three samples (%v)", *rep.Aging.P85, rep.Aging.Items[0].Days)
	}
}

func TestAgingEmptyWithNothingInProgress(t *testing.T) {
	f := baseFixture()
	for i := range f.issues {
		if f.issues[i].category == "inprogress" {
			f.issues[i].category = "done"
		}
	}
	rep := matReport(t, f)
	if rep.Aging.P85 != nil {
		t.Errorf("p85_days = %v, want null with nothing in progress", *rep.Aging.P85)
	}
	if len(rep.Aging.Items) != 0 {
		t.Errorf("aging items = %v, want none", rep.Aging.Items)
	}
	if got := rep.JSON().Aging.Items; got == nil {
		t.Error("JSON aging.items must be an empty array, never null")
	}
}

/* ── M2 events ── */

func TestBucketEventsCarryEveryKind(t *testing.T) {
	f := baseFixture()
	f.sprintLog = []matSprintLog{
		{key: "T-2", at: day(4, 11), toID: 7, toNa: "Sprint 7"},
		{key: "T-2", at: day(6, 11), fromID: 7, fromName: "Sprint 7"},
	}
	rep := matReport(t, f)
	b := rep.Buckets[0]

	seen := map[string]bool{}
	for _, e := range b.Events {
		seen[e.Kind] = true
	}
	for _, want := range []string{EventCreated, EventStarted, EventResolved, EventReopened,
		EventSprintIn, EventSprintOut, EventComment} {
		if !seen[want] {
			t.Errorf("no %q event in the full week: %v", want, b.Events)
		}
	}
	for i := 1; i < len(b.Events); i++ {
		if b.Events[i-1].At > b.Events[i].At {
			t.Fatalf("events out of order at %d: %v", i, b.Events)
		}
	}
	// Every event is inside the bucket's own window.
	for _, e := range b.Events {
		at, ok := parseTime(e.At)
		if !ok || at.Before(b.From) || !at.Before(b.To) {
			t.Fatalf("event outside [%s, %s): %+v", b.From, b.To, e)
		}
	}
	// The details the contract names: a comment carries its author, a sprint
	// move its sprint name.
	for _, e := range b.Events {
		switch e.Kind {
		case EventComment:
			if e.Detail != "Ada" {
				t.Errorf("comment event detail = %q, want the author", e.Detail)
			}
		case EventSprintIn, EventSprintOut:
			if e.Detail != "Sprint 7" {
				t.Errorf("%s detail = %q, want the sprint name", e.Kind, e.Detail)
			}
		}
	}
	if b.EventsTruncated {
		t.Error("a nine-row week is not truncated")
	}
	// The empty case: a fixture with no activity in the window has an empty
	// array, not a missing one.
	empty := matReport(t, matFixture{issues: []matIssue{
		{key: "Z-1", summary: "z", typeID: "10001", typeName: "Task", category: "new",
			created: time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local),
			updated: time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)},
	}})
	if len(empty.Buckets[0].Events) != 0 {
		t.Errorf("a quiet week has events: %v", empty.Buckets[0].Events)
	}
	if empty.JSON().Buckets[0].Events == nil {
		t.Error("JSON events must be an empty array, never null")
	}
}

func TestBucketEventsCap(t *testing.T) {
	f := matFixture{}
	// MaxEvents+20 creations inside the full week, all at distinct instants.
	for i := 0; i < MaxEvents+20; i++ {
		at := day(3, 0).Add(time.Duration(i) * time.Minute)
		f.issues = append(f.issues, matIssue{
			key: fmt.Sprintf("C-%04d", i), summary: "c", typeID: "10001", typeName: "Task",
			category: "new", created: at, updated: at,
		})
	}
	rep := matReport(t, f)
	b := rep.Buckets[0]
	if len(b.Events) != MaxEvents || !b.EventsTruncated {
		t.Fatalf("events = %d (truncated %v), want %d and true", len(b.Events), b.EventsTruncated, MaxEvents)
	}
	if !rep.JSON().Buckets[0].EventsTruncated {
		t.Error("events_truncated must reach the document")
	}
}

/* ── M3 surprises ── */

func TestSurprisesReopenedAndReversal(t *testing.T) {
	rep := matReport(t, baseFixture())
	b := rep.Buckets[0]
	byKind := map[string][]Surprise{}
	for _, s := range b.Surprises {
		byKind[s.Kind] = append(byKind[s.Kind], s)
	}
	re := byKind[SurpriseReopened]
	if len(re) != 1 || re[0].Key != "T-3" {
		t.Fatalf("reopened surprises = %v, want just T-3", re)
	}
	if re[0].Detail != "regressed in staging" {
		t.Errorf("reopened detail = %q, want the recorded reopen reason", re[0].Detail)
	}
	rv := byKind[SurpriseReversal]
	if len(rv) != 1 || rv[0].Key != "T-4" {
		t.Fatalf("reversal surprises = %v, want just T-4 (four moves)", rv)
	}
	if rv[0].Detail != "4" {
		t.Errorf("reversal detail = %q, want the move count", rv[0].Detail)
	}
	// The empty case: the partial week has none of either.
	if len(rep.Buckets[1].Surprises) != 0 {
		t.Errorf("the quiet partial week has surprises: %v", rep.Buckets[1].Surprises)
	}
	// Three moves is not a reversal — the threshold is a contract, not a mood.
	f := baseFixture()
	f.statuses = f.statuses[:len(f.statuses)-1]
	if got := matReport(t, f).Buckets[0].Surprises; len(got) != 1 || got[0].Kind != SurpriseReopened {
		t.Errorf("three moves counted as a reversal: %v", got)
	}
}

func TestSprintOnlySurprisesAreWeekSilent(t *testing.T) {
	f := baseFixture()
	f.sprints = []matSprint{{id: 7, board: 1, name: "Sprint 7", start: day(2, 0), end: day(16, 0)}}
	f.sprintLog = []matSprintLog{{key: "T-2", at: day(4, 11), toID: 7, toNa: "Sprint 7"}}
	for i := range f.issues {
		if f.issues[i].key == "T-2" {
			f.issues[i].sprintID = 7
			f.issues[i].carryover = 2
		}
	}
	// Week columns: the two sprint kinds are structurally absent.
	week := matReport(t, f)
	for _, b := range week.Buckets {
		for _, s := range b.Surprises {
			if s.Kind == SurpriseAddedAfterStart || s.Kind == SurpriseCarried {
				t.Errorf("week column carries a sprint-only surprise: %+v", s)
			}
		}
	}
	// Sprint columns: both appear, on the sprint that owns them.
	db := buildMirror(t, f)
	rep, err := Compute(context.Background(), db, store.FeedIdentity{}, 7*24*time.Hour, matNow, Options{BySprint: true})
	if err != nil {
		t.Fatalf("Compute --by-sprint: %v", err)
	}
	if len(rep.Buckets) != 1 || rep.Buckets[0].SprintID != 7 {
		t.Fatalf("want one bucket for sprint 7, got %+v", rep.Buckets)
	}
	var added, carried []Surprise
	for _, s := range rep.Buckets[0].Surprises {
		switch s.Kind {
		case SurpriseAddedAfterStart:
			added = append(added, s)
		case SurpriseCarried:
			carried = append(carried, s)
		}
	}
	if len(added) != 1 || added[0].Key != "T-2" {
		t.Errorf("added_after_start = %v, want T-2", added)
	}
	if len(carried) != 1 || carried[0].Key != "T-2" || carried[0].Detail != "2" {
		t.Errorf("carried = %v, want T-2 with detail 2", carried)
	}
}

/* ── M4 closed decompositions ── */

func TestClosedDecompositionsPartitionClosed(t *testing.T) {
	rep := matReport(t, baseFixture())
	b := rep.Buckets[0]
	// T-3 entered done on 03-03 and left it again on 03-07: it closed during
	// the week and also reopened during it, so it is counted by both rows.
	if b.Closed == nil || *b.Closed != 3 {
		t.Fatalf("closed = %v, want 3 (T-1, T-2, T-3)", derefInt(b.Closed))
	}
	byType := 0
	for _, tc := range b.ClosedByType {
		byType += tc.Count
		if tc.Count != len(tc.Keys) {
			t.Errorf("closed_by_type row %+v: count is not its key count", tc)
		}
	}
	if byType != *b.Closed {
		t.Errorf("closed_by_type sums to %d, closed is %d", byType, *b.Closed)
	}
	byEpic := 0
	for _, ec := range b.ClosedByEpic {
		byEpic += ec.Count
		if ec.Count != len(ec.Keys) {
			t.Errorf("closed_by_epic row %+v: count is not its key count", ec)
		}
	}
	if byEpic != *b.Closed {
		t.Errorf("closed_by_epic sums to %d, closed is %d", byEpic, *b.Closed)
	}
	// The groups themselves: T-1 is a Bug under E-1, T-2 a Task under none.
	seenType := map[string][]string{}
	for _, tc := range b.ClosedByType {
		seenType[tc.IssueTypeID] = tc.Keys
	}
	if strings.Join(seenType["10004"], ",") != "T-1,T-3" || strings.Join(seenType["10001"], ",") != "T-2" {
		t.Errorf("closed_by_type groups = %+v", b.ClosedByType)
	}
	seenEpic := map[string][]string{}
	for _, ec := range b.ClosedByEpic {
		seenEpic[ec.EpicKey] = ec.Keys
		if ec.EpicKey == "E-1" && ec.Title != "the epic" {
			t.Errorf("epic row carries no title: %+v", ec)
		}
	}
	if strings.Join(seenEpic["E-1"], ",") != "T-1,T-3" || strings.Join(seenEpic[""], ",") != "T-2" {
		t.Errorf("closed_by_epic groups = %+v (the empty key is the no-epic row)", b.ClosedByEpic)
	}
	// unplanned is a subset of closed: T-1 was created and closed in the
	// same week, T-2 arrived before it.
	if b.Unplanned.Count != 1 || strings.Join(b.Unplanned.Keys, ",") != "T-1" {
		t.Errorf("unplanned = %+v, want just T-1", b.Unplanned)
	}
	// cycle_points names the same sample CycleKeys does, with its value.
	if strings.Join(b.CycleKeys, ",") != "T-1" {
		t.Fatalf("cycle sample = %v, want T-1", b.CycleKeys)
	}
	if len(b.CyclePoints) != 1 || b.CyclePoints[0].Key != "T-1" || b.CyclePoints[0].Days != 2 {
		t.Errorf("cycle_points = %+v, want T-1 at 2 days (48h)", b.CyclePoints)
	}
	if b.CyclePoints[0].ResolvedAt == "" {
		t.Error("a cycle point carries when it resolved")
	}
	// The empty case: the partial week closed nothing.
	q := rep.Buckets[1]
	if len(q.ClosedByType) != 0 || len(q.ClosedByEpic) != 0 || q.Unplanned.Count != 0 || len(q.CyclePoints) != 0 {
		t.Errorf("the quiet partial week carries closures: %+v", q)
	}
}

// TestClosedDecompositionsSumOnDemoFixture repeats the sum invariant on the
// shipped sample: the hand fixture proves the values, this proves the
// partition holds on real-shaped data with many types and epics.
func TestClosedDecompositionsSumOnDemoFixture(t *testing.T) {
	_, db := demoFixture(t, true)
	rep, err := Compute(context.Background(), db, store.FeedIdentity{}, 8*7*24*time.Hour, time.Now(), Options{})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	total := 0
	for bi, b := range rep.Buckets {
		if b.Closed == nil {
			continue
		}
		total += *b.Closed
		byType, byEpic := 0, 0
		for _, tc := range b.ClosedByType {
			byType += tc.Count
		}
		for _, ec := range b.ClosedByEpic {
			byEpic += ec.Count
		}
		if byType != *b.Closed || byEpic != *b.Closed {
			t.Errorf("bucket %d: closed %d, by type %d, by epic %d", bi, *b.Closed, byType, byEpic)
		}
		if b.Unplanned.Count > *b.Closed {
			t.Errorf("bucket %d: unplanned %d exceeds closed %d", bi, b.Unplanned.Count, *b.Closed)
		}
		if len(b.CyclePoints) != len(b.CycleKeys) {
			t.Errorf("bucket %d: %d cycle points for %d cycle keys", bi, len(b.CyclePoints), len(b.CycleKeys))
		}
	}
	if total == 0 {
		t.Fatal("the demo fixture should close something over eight weeks")
	}
}

/* ── M5 seen / moved ── */

func TestSeenAndMovedSplitTheBucket(t *testing.T) {
	rep := matReport(t, baseFixture())
	b := rep.Buckets[0]
	if strings.Join(b.SeenNotMoved.Keys, ",") != "T-5" {
		t.Errorf("seen_not_moved = %v, want T-5 (visited, never touched)", b.SeenNotMoved.Keys)
	}
	if len(b.MovedNotSeen.Keys) == 0 {
		t.Fatal("moved_not_seen is empty though the week has changelog rows")
	}
	seen := map[string]bool{}
	for _, k := range b.SeenNotMoved.Keys {
		seen[k] = true
	}
	for _, k := range b.MovedNotSeen.Keys {
		if seen[k] {
			t.Errorf("%s is in both halves — they are complements", k)
		}
		if k == "T-5" {
			t.Error("T-5 was visited; it cannot be moved_not_seen")
		}
	}
	if !contains(b.MovedNotSeen.Keys, "T-1") {
		t.Errorf("moved_not_seen = %v, want T-1 among them", b.MovedNotSeen.Keys)
	}
}

func TestSeenAndMovedEmptyWithoutVisits(t *testing.T) {
	f := baseFixture()
	f.visits = nil
	rep := matReport(t, f)
	if !rep.VisitsEmpty {
		t.Fatal("a fixture with no visits must mark the report")
	}
	for bi, b := range rep.Buckets {
		if len(b.SeenNotMoved.Keys) != 0 || len(b.MovedNotSeen.Keys) != 0 {
			t.Errorf("bucket %d answers seen/moved with no visits: %+v %+v", bi, b.SeenNotMoved, b.MovedNotSeen)
		}
	}
	found := false
	for _, n := range rep.Notes() {
		if n[0] == "visits" && strings.Contains(n[1], "no issue reads recorded") {
			found = true
		}
	}
	if !found {
		t.Errorf("no note explains the empty rows: %v", rep.Notes())
	}
	// The document carries empty arrays, never null.
	jb := rep.JSON().Buckets[0]
	if jb.SeenNotMoved.Keys == nil || jb.MovedNotSeen.Keys == nil {
		t.Error("seen_not_moved/moved_not_seen keys must be empty arrays in JSON")
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

/* ── M6 actions ── */

func TestActionsCarryTheirMetric(t *testing.T) {
	rep := matReport(t, baseFixture())
	if len(rep.Actions) != 1 {
		t.Fatalf("actions = %+v, want the one retro-action issue", rep.Actions)
	}
	a := rep.Actions[0]
	if a.Key != "A-1" || a.Summary != "shorten the tail" || a.StatusCategory != "new" {
		t.Errorf("action row = %+v", a)
	}
	if a.ResolvedAt != nil {
		t.Errorf("resolved_at = %v, want null on an open action", *a.ResolvedAt)
	}
	if a.Metric != "closed" {
		t.Fatalf("metric = %q, want the row named on the first line", a.Metric)
	}
	// A-1 was created inside the full week, so "then" is that week's closed
	// (2) and "now" is the partial week's (0).
	if a.Then == nil || *a.Then != 3 {
		t.Errorf("then = %v, want the closed value of the bucket the action was created in (3)", a.Then)
	}
	if a.Now == nil || *a.Now != 0 {
		t.Errorf("now = %v, want the last bucket's closed value (0)", a.Now)
	}
	// An action with no metric line carries no numbers rather than zeros.
	f := baseFixture()
	for i := range f.issues {
		if f.issues[i].key == "A-1" {
			f.issues[i].body = "just do the thing"
		}
	}
	plain := matReport(t, f).Actions[0]
	if plain.Metric != "" || plain.Then != nil || plain.Now != nil {
		t.Errorf("an action naming no metric = %+v, want empty metric and null then/now", plain)
	}
}

func TestActionsEmptyWithoutLabel(t *testing.T) {
	f := baseFixture()
	for i := range f.issues {
		f.issues[i].labels = `["quick-win"]`
	}
	rep := matReport(t, f)
	if len(rep.Actions) != 0 {
		t.Errorf("actions = %+v, want none without the retro-action label", rep.Actions)
	}
	if rep.JSON().Actions == nil {
		t.Error("JSON actions must be an empty array, never null")
	}
}

/* ── M7 the document shape ── */

// TestMaterialArraysAreNeverNull walks the encoded document: the web track
// reads exactly this JSON, and a null where it expects an array is a crash
// in the renderer rather than an empty section.
func TestMaterialArraysAreNeverNull(t *testing.T) {
	rep := matReport(t, matFixture{issues: []matIssue{
		{key: "Z-1", summary: "z", typeID: "10001", typeName: "Task", category: "new",
			created: time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local),
			updated: time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)},
	}})
	raw, err := json.Marshal(rep.JSON())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Aging struct {
			P85   *float64 `json:"p85_days"`
			Items []any    `json:"items"`
		} `json:"aging"`
		Actions []any `json:"actions"`
		Buckets []map[string]json.RawMessage
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Aging.Items == nil || doc.Actions == nil {
		t.Fatal("aging.items and actions must be arrays")
	}
	for bi, b := range doc.Buckets {
		for _, field := range []string{"events", "surprises", "closed_by_type", "closed_by_epic", "cycle_points"} {
			v, ok := b[field]
			if !ok {
				t.Errorf("bucket %d has no %q", bi, field)
				continue
			}
			if string(v) == "null" {
				t.Errorf("bucket %d %q is null, want []", bi, field)
			}
		}
		for _, field := range []string{"unplanned", "seen_not_moved", "moved_not_seen"} {
			var obj struct {
				Keys []string `json:"keys"`
			}
			if err := json.Unmarshal(b[field], &obj); err != nil {
				t.Errorf("bucket %d %q: %v", bi, field, err)
				continue
			}
			if obj.Keys == nil {
				t.Errorf("bucket %d %q.keys is null, want []", bi, field)
			}
		}
	}
}

/* ── M8 definitions ── */

func TestDefinitionsCoverTheMaterials(t *testing.T) {
	for _, bySprint := range []bool{false, true} {
		r := Report{BySprint: bySprint}
		have := map[string]bool{}
		for _, d := range r.Definitions() {
			have[d[0]] = true
			if strings.TrimSpace(d[1]) == "" {
				t.Errorf("definition %q is empty", d[0])
			}
		}
		for _, name := range []string{"aging", "events", "surprises", "closed_by_type",
			"closed_by_epic", "unplanned", "cycle_points", "seen_not_moved",
			"moved_not_seen", "actions"} {
			if !have[name] {
				t.Errorf("bySprint=%v: no definition for %q", bySprint, name)
			}
		}
		doc := r.JSON()
		for _, name := range []string{"aging", "events", "surprises", "actions"} {
			if doc.Definitions[name] == "" {
				t.Errorf("bySprint=%v: the document carries no definition for %q", bySprint, name)
			}
		}
	}
}

// derefInt prints a *int in a failure message without the pointer address.
func derefInt(p *int) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprint(*p)
}
