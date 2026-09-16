package migrate

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/atomicfile"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fsperm"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// seedSprintMirror builds the source-side half of the sprint defect (GDK-1961):
// a mirror whose issues reference three sprints through issues_full.sprint_id —
// active, future, closed — with the closed one holding one done and one
// not-done member (the pair that separates "carried" from "swept"). The board
// row carries no project key, the demo mirror's shape, so the pass cannot map
// sprints through boards.project_key and must derive the project from the
// members.
func seedSprintMirror(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	sprint := func(id int64) *int64 { return &id }
	rec := func(n int, statusID, category string, sprintID *int64, sprintName, sprintState string) store.IssueRecord {
		key := "NMB-" + string(rune('0'+n))
		return store.IssueRecord{
			Item: store.Item{ID: "jira:" + key, SourceID: "jira", Kind: "issue", ExternalID: string(rune('0' + n)),
				Key: key, Title: "issue " + key,
				CreatedAt: "2026-07-20T00:00:00.000Z", UpdatedAt: "2026-07-21T00:00:00.000Z"},
			Issue: store.Issue{ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10001",
				Status: category, StatusID: statusID, StatusCategory: category,
				SprintID: sprintID, SprintName: sprintName, SprintState: sprintState},
		}
	}
	_, err = db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress", "10001": "done"},
		Records: []store.IssueRecord{
			rec(1, "3", "inprogress", sprint(10), "Sprint A", "active"),
			rec(2, "1", "new", sprint(10), "Sprint A", "active"),
			rec(3, "10001", "done", sprint(12), "Sprint C", "closed"),
			rec(4, "3", "inprogress", sprint(12), "Sprint C", "closed"),
			rec(5, "1", "new", sprint(11), "Sprint B", "future"),
			rec(6, "1", "new", nil, "", ""),
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.ReplaceAgile(ctx, "jira",
		[]store.BoardRow{{ID: 7, Name: "NMB board", Type: "scrum"}},
		[]store.SprintRow{
			{ID: 10, BoardID: 7, Name: "Sprint A", Goal: "close the pass", State: "active",
				StartAt: "2026-08-01T00:00:00.000Z", EndAt: "2026-08-14T00:00:00.000Z"},
			{ID: 11, BoardID: 7, Name: "Sprint B", State: "future",
				StartAt: "2026-09-01T00:00:00.000Z", EndAt: "2026-09-14T00:00:00.000Z"},
			{ID: 12, BoardID: 7, Name: "Sprint C", Goal: "done work", State: "closed",
				StartAt: "2026-07-01T00:00:00.000Z", EndAt: "2026-07-14T00:00:00.000Z"},
		}); err != nil {
		t.Fatalf("agile: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSprintPassCarriesSprintsAndMembership is the round-trip gate for the
// sprint pass (GDK-1961): a hand-built source mirror carrying three sprints
// migrates into a real built-in workspace, and afterwards the target's sprints
// table holds all three with their goal, dates and state, every issue that had
// a sprint still has one — except the not-done members of the closed sprint,
// which the Agile API's close sweep moves to the backlog — and the active
// sprint selects the same issues it did on the source.
//
// The mirror tail matters as much as the writes: the pass runs between the
// seed and origin.Close, then one incremental sync re-mirrors what it wrote,
// so the assertions read the target the way any later `gadak sync` would.
func TestSprintPassCarriesSprintsAndMembership(t *testing.T) {
	ctx := context.Background()
	srcDB, err := store.OpenReadOnly(seedSprintMirror(t))
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	t.Cleanup(func() { _ = srcDB.Close() })
	doc, stats, err := Build(ctx, srcDB, Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if stats.SprintIssues != 5 {
		t.Fatalf("export counted %d sprinted issues, want 5", stats.SprintIssues)
	}

	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	// The target must resolve as a credential-free built-in workspace, not
	// inherit whatever this shell points at.
	t.Setenv("GADAK_SITE", "")
	t.Setenv("GADAK_EMAIL", "")
	t.Setenv("GADAK_TOKEN", "")
	t.Setenv("GADAK_PROJECTS", "")
	const target = "sprints-target"
	config.SetProfile(target)
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	targetDir, err := config.DirFor(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := fsperm.EnsurePrivateDir(targetDir); err != nil {
		t.Fatal(err)
	}
	if err := fsperm.EnsurePrivateDir(filepath.Join(targetDir, filepath.Dir(filepath.FromSlash(origin.LegacyYAMLRel)))); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(targetDir, filepath.FromSlash(origin.LegacyYAMLRel))
	if err := atomicfile.WriteStream(yamlPath, "issuetap-*.yaml", func(w io.Writer) error {
		return WriteDoc(ctx, w, doc, nil, stats)
	}); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	tcfg, err := config.LoadFor(target)
	if err != nil {
		t.Fatal(err)
	}
	fillErr, err := originbind.SeedBuiltIn(tcfg, strings.Join(stats.Projects, ","), &config.ConfluenceConfig{Spaces: stats.Spaces},
		func() (*store.DB, func() error, error) {
			p, err := config.DBPathFor(target)
			if err != nil {
				return nil, nil, err
			}
			db, err := store.Open(p)
			if err != nil {
				return nil, nil, err
			}
			return db, db.Close, nil
		})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if fillErr != nil {
		t.Fatalf("first fill: %v", fillErr)
	}

	// ── the sprint pass under test, then the incremental refill ─────────
	c, err := origin.Client(tcfg)
	if err != nil {
		t.Fatalf("origin client: %v", err)
	}
	if err := MigrateSprints(ctx, srcDB, agileTestOrigin{c}, stats); err != nil {
		t.Fatalf("sprint pass: %v", err)
	}
	p, err := config.DBPathFor(target)
	if err != nil {
		t.Fatal(err)
	}
	pdb, err := store.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := syncer.Run(ctx, tcfg, pdb, syncer.Options{Client: c}); err != nil {
		t.Fatalf("refill: %v", err)
	}
	if err := pdb.Close(); err != nil {
		t.Fatal(err)
	}
	if err := origin.Close(); err != nil {
		t.Fatalf("flush origin persist: %v", err)
	}

	// ── target assertions, read the way a later sync would leave it ──────
	tdbPath, err := config.DBPathFor(target)
	if err != nil {
		t.Fatal(err)
	}
	tdb, err := store.OpenReadOnly(tdbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.Close()

	var n int
	if err := tdb.QueryRowContext(ctx, `SELECT COUNT(*) FROM sprints`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("target holds %d sprints, want 3", n)
	}
	rows, err := tdb.QueryContext(ctx, `SELECT name, COALESCE(goal,''), state, start_at IS NOT NULL, end_at IS NOT NULL FROM sprints ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	// The target mints its own sprint ids; identity here is the name.
	want := map[string][3]string{
		"Sprint A": {"close the pass", "active", ""},
		"Sprint B": {"", "future", ""},
		"Sprint C": {"done work", "closed", ""},
	}
	for rows.Next() {
		var name, goal, state string
		var hasStart, hasEnd bool
		if err := rows.Scan(&name, &goal, &state, &hasStart, &hasEnd); err != nil {
			t.Fatal(err)
		}
		w, ok := want[name]
		if !ok {
			t.Fatalf("unexpected sprint %q", name)
		}
		if goal != w[0] || state != w[1] || !hasStart || !hasEnd {
			t.Fatalf("sprint %q = (%q, %s, dates %v/%v), want goal %q state %s with dates", name, goal, state, hasStart, hasEnd, w[0], w[1])
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	// Membership: NMB-4 is the not-done member of the closed sprint — the
	// Agile API cannot hold it there (moves into a closed sprint are refused,
	// and closing sweeps every not-done member to the backlog), so the target
	// honestly reports it in the backlog and the stats carry the sweep.
	type member struct{ name, state string }
	got := map[string]member{}
	irows, err := tdb.QueryContext(ctx, `SELECT key, COALESCE(sprint_name,''), COALESCE(sprint_state,'') FROM issues_full ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	defer irows.Close()
	for irows.Next() {
		var k string
		var m member
		if err := irows.Scan(&k, &m.name, &m.state); err != nil {
			t.Fatal(err)
		}
		got[k] = m
	}
	if err := irows.Err(); err != nil {
		t.Fatal(err)
	}
	for k, m := range map[string]member{
		"NMB-1": {"Sprint A", "active"},
		"NMB-2": {"Sprint A", "active"},
		"NMB-3": {"Sprint C", "closed"},
		"NMB-4": {"", ""},
		"NMB-5": {"Sprint B", "future"},
		"NMB-6": {"", ""},
	} {
		if got[k] != m {
			t.Fatalf("%s = %+v, want %+v (all: %v)", k, got[k], m, got)
		}
	}
	if stats.SprintSwept != 1 {
		t.Fatalf("stats report %d swept issues, want 1 (NMB-4)", stats.SprintSwept)
	}

	// The defect's own probe: `sprint_state = 'active'` selects the same
	// issues as on the source.
	var active int
	if err := tdb.QueryRowContext(ctx, `SELECT COUNT(*) FROM issues_full WHERE sprint_state = 'active'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 2 {
		t.Fatalf("active sprint selects %d issues on the target, want 2 (NMB-1, NMB-2)", active)
	}

	// The count table carries the sprint axes and they agree — 4 members
	// survived (5 referenced minus the 1 swept), 3 sprints landed.
	verify, err := VerifyMirror(ctx, tdb, stats)
	if err != nil {
		t.Fatal(err)
	}
	rowsByMetric := map[string]VerifyRow{}
	for _, r := range verify {
		rowsByMetric[r.Metric] = r
	}
	for metric, wantSource := range map[string]int{"sprints": 3, "sprint issues": 4} {
		r, ok := rowsByMetric[metric]
		if !ok {
			t.Fatalf("verify table has no %q row: %+v", metric, verify)
		}
		if r.Source != wantSource || r.Migrated != wantSource {
			t.Fatalf("verify %q = %d/%d, want %d/%d (all: %+v)", metric, r.Source, r.Migrated, wantSource, wantSource, verify)
		}
	}
}

// agileTestOrigin adapts the Jira Agile client onto SprintOrigin for this
// package's tests (the product adapter lives in cmd/gadak). It adds nothing:
// same verbs, same order.
type agileTestOrigin struct{ c *jira.Client }

func (a agileTestOrigin) BoardForProject(ctx context.Context, projectKey string) (int64, error) {
	boards, err := a.c.BoardsForProject(ctx, projectKey)
	if err != nil {
		return 0, err
	}
	if len(boards) == 0 {
		return 0, fmt.Errorf("no board for project %s", projectKey)
	}
	return boards[0].ID, nil
}

func (a agileTestOrigin) CreateSprint(ctx context.Context, boardID int64, name, goal string) (int64, error) {
	s, err := a.c.CreateSprint(ctx, boardID, name, goal)
	return s.ID, err
}

func (a agileTestOrigin) UpdateSprint(ctx context.Context, sprintID int64, fields map[string]any) error {
	_, err := a.c.UpdateSprint(ctx, sprintID, fields)
	return err
}

func (a agileTestOrigin) MoveToSprint(ctx context.Context, sprintID int64, keys []string) error {
	return a.c.MoveToSprint(ctx, sprintID, keys)
}

// fakeSprintOrigin records the pass's write sequence and hands out its own
// ids. It is how the ordering contract — create while future, move before
// any transition, one call for active, activate-then-close for closed —
// stays pinned even without a live origin.
type fakeSprintOrigin struct {
	log  []string
	next int64
}

func formatFields(fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, fields[k]))
	}
	return strings.Join(parts, " ")
}

func (f *fakeSprintOrigin) BoardForProject(_ context.Context, projectKey string) (int64, error) {
	f.log = append(f.log, "board "+projectKey)
	return 1, nil
}

func (f *fakeSprintOrigin) CreateSprint(_ context.Context, boardID int64, name, goal string) (int64, error) {
	f.next++
	id := 500 + f.next
	f.log = append(f.log, fmt.Sprintf("create board=%d %q goal=%q -> %d", boardID, name, goal, id))
	return id, nil
}

func (f *fakeSprintOrigin) UpdateSprint(_ context.Context, sprintID int64, fields map[string]any) error {
	f.log = append(f.log, fmt.Sprintf("update %d %s", sprintID, formatFields(fields)))
	return nil
}

func (f *fakeSprintOrigin) MoveToSprint(_ context.Context, sprintID int64, keys []string) error {
	f.log = append(f.log, fmt.Sprintf("move %d %v", sprintID, keys))
	return nil
}

// TestSprintPassOrderStatsAndStuck pins the write sequence and the stats
// with a recording fake: sprints apply in source-id order, members move
// while the sprint is future (never into a closed one), an active sprint
// transitions in one call carrying its dates, a closed one activates then
// closes, a closed sprint without dates is created and reported stuck
// instead of silently mis-stated, and an orphaned sprint reference is
// counted rather than attempted.
//
// No FAIL-first run exists for this gate: it pins the contract of an API
// this round introduces, so the pre-change tree has no symbols to compile
// it against. The round-trip gate above is the one that failed first.
func TestSprintPassOrderStatsAndStuck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	sprint := func(id int64) *int64 { return &id }
	rec := func(n int, category string, sprintID *int64) store.IssueRecord {
		key := "NMB-" + string(rune('0'+n))
		status := map[string]string{"new": "1", "inprogress": "3", "done": "10001"}[category]
		return store.IssueRecord{
			Item: store.Item{ID: "jira:" + key, SourceID: "jira", Kind: "issue", ExternalID: string(rune('0' + n)),
				Key: key, Title: "issue " + key,
				CreatedAt: "2026-07-20T00:00:00.000Z", UpdatedAt: "2026-07-21T00:00:00.000Z"},
			Issue: store.Issue{ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10001",
				Status: category, StatusID: status, StatusCategory: category, SprintID: sprintID},
		}
	}
	_, err = db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress", "10001": "done"},
		Records: []store.IssueRecord{
			rec(1, "done", sprint(20)),
			rec(2, "inprogress", sprint(20)),
			rec(3, "done", sprint(21)),
			rec(4, "inprogress", sprint(21)),
			rec(5, "new", sprint(22)),
			rec(6, "new", sprint(23)),
			rec(7, "new", sprint(99)), // orphan: no sprints row
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.ReplaceAgile(ctx, "jira",
		[]store.BoardRow{{ID: 7, Name: "NMB board", Type: "scrum"}},
		[]store.SprintRow{
			{ID: 20, BoardID: 7, Name: "Sprint ZA", Goal: "goal A", State: "active",
				StartAt: "2026-08-01T00:00:00.000Z", EndAt: "2026-08-14T00:00:00.000Z"},
			{ID: 21, BoardID: 7, Name: "Sprint ZB", State: "closed",
				StartAt: "2026-07-01T00:00:00.000Z", EndAt: "2026-07-14T00:00:00.000Z"},
			// 22: closed with no dates — the stuck case.
			{ID: 22, BoardID: 7, Name: "Sprint ZC", State: "closed"},
			{ID: 23, BoardID: 7, Name: "Sprint ZD", State: "future",
				StartAt: "2026-09-01T00:00:00.000Z", EndAt: "2026-09-14T00:00:00.000Z"},
		}); err != nil {
		t.Fatalf("agile: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer ro.Close()

	st := &Stats{Projects: []string{"NMB"}}
	fake := &fakeSprintOrigin{}
	if err := MigrateSprints(ctx, ro, fake, st); err != nil {
		t.Fatalf("pass: %v", err)
	}

	want := []string{
		"board NMB",
		// 20 active: move while future, then one call — dates ride the
		// transition.
		`create board=1 "Sprint ZA" goal="goal A" -> 501`,
		`move 501 [NMB-1 NMB-2]`,
		`update 501 endDate=2026-08-14T00:00:00.000Z startDate=2026-08-01T00:00:00.000Z state=active`,
		// 21 closed: activate, then close.
		`create board=1 "Sprint ZB" goal="" -> 502`,
		`move 502 [NMB-3 NMB-4]`,
		`update 502 endDate=2026-07-14T00:00:00.000Z startDate=2026-07-01T00:00:00.000Z state=active`,
		`update 502 state=closed`,
		// 22 closed without dates: created, never transitioned.
		`create board=1 "Sprint ZC" goal="" -> 503`,
		`move 503 [NMB-5]`,
		// 23 future: dates are a plain set, no transition follows.
		`create board=1 "Sprint ZD" goal="" -> 504`,
		`update 504 endDate=2026-09-14T00:00:00.000Z startDate=2026-09-01T00:00:00.000Z`,
		`move 504 [NMB-6]`,
	}
	if len(fake.log) != len(want) {
		t.Fatalf("write sequence:\n%s\nwant %d calls, got %d", strings.Join(fake.log, "\n"), len(want), len(fake.log))
	}
	for i, w := range want {
		if fake.log[i] != w {
			t.Fatalf("write sequence step %d:\n  got  %s\n  want %s\n(full log:\n%s)", i, fake.log[i], w, strings.Join(fake.log, "\n"))
		}
	}

	if st.Sprints != 4 || st.SprintsCreated != 4 {
		t.Fatalf("sprint counts = %d referenced / %d created, want 4/4", st.Sprints, st.SprintsCreated)
	}
	if st.SprintSwept != 1 {
		t.Fatalf("swept = %d, want 1 (NMB-4, not done in closed Sprint ZB)", st.SprintSwept)
	}
	if st.SprintOrphans != 1 {
		t.Fatalf("orphans = %d, want 1 (NMB-7 references sprint 99)", st.SprintOrphans)
	}
	if len(st.SprintStuck) != 1 || !strings.Contains(st.SprintStuck[0], "Sprint ZC") {
		t.Fatalf("stuck = %v, want one entry naming Sprint ZC", st.SprintStuck)
	}
	if len(st.SprintErrors) != 0 {
		t.Fatalf("errors = %v, want none", st.SprintErrors)
	}
}
