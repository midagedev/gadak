package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/store"
)

func strp(s string) *string { return &s }

func sampleRows() []row {
	return buildRows([]store.IssueLite{
		{IssueKey: "AAA-1", Summary: "Fix login timeout", Status: "Backlog", StatusCategory: "new", Assignee: strp("Ada"), UpdatedAt: strp("2026-08-01T10:00:00Z")},
		{IssueKey: "AAA-2", Summary: "Ship dashboard", Status: "In Progress", StatusCategory: "inprogress", Assignee: strp("Bob"), UpdatedAt: strp("2026-08-02T10:00:00Z")},
		{IssueKey: "BBB-3", Summary: "Login polish", Status: "Done", StatusCategory: "done", Assignee: strp("Ada"), UpdatedAt: strp("2026-08-03T10:00:00Z")},
		{IssueKey: "CCC-4", Summary: "Unassigned chore", Status: "To Do", StatusCategory: "new"},
	})
}

func TestApplyFilterTab(t *testing.T) {
	all := sampleRows()
	cases := []struct {
		tab  Tab
		want int
	}{
		{TabAll, 4},
		{TabOpen, 3}, // non-done
		{TabInProgress, 1},
		{TabDone, 1},
	}
	for _, tc := range cases {
		got := applyFilter(all, tc.tab, "")
		if len(got) != tc.want {
			t.Errorf("tab %s: got %d want %d", tc.tab.Label(), len(got), tc.want)
		}
	}
}

func TestApplyFilterText(t *testing.T) {
	all := sampleRows()

	// Key partial match
	got := applyFilter(all, TabAll, "aaa")
	if len(got) != 2 {
		t.Fatalf("key filter: got %d want 2", len(got))
	}

	// Summary partial match
	got = applyFilter(all, TabAll, "login")
	if len(got) != 2 {
		t.Fatalf("summary filter: got %d want 2", len(got))
	}

	// Assignee match
	got = applyFilter(all, TabAll, "bob")
	if len(got) != 1 || all[got[0]].lite.IssueKey != "AAA-2" {
		t.Fatalf("assignee filter: %#v", got)
	}

	// Tab + text
	got = applyFilter(all, TabDone, "login")
	if len(got) != 1 || all[got[0]].lite.IssueKey != "BBB-3" {
		t.Fatalf("tab+text: %#v", got)
	}

	// No match
	got = applyFilter(all, TabAll, "zzzz")
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

// TUI / filter is substring, so Korean particles already match; keep a regression
// case so a future FTS-shaped change cannot break list narrowing.
func TestApplyListFilterKoreanSubstring(t *testing.T) {
	all := buildRows([]store.IssueLite{
		{IssueKey: "KR-1", Summary: "로그인이 안됨", StatusCategory: "new"},
		{IssueKey: "KR-2", Summary: "결제 오류", StatusCategory: "new"},
	})
	got := applyListFilter(all, listFilter{tab: TabAll, text: "로그인"})
	if len(got) != 1 || all[got[0]].lite.IssueKey != "KR-1" {
		t.Fatalf("korean substring filter: %#v", got)
	}
}

func TestBuildRowsPreLowercases(t *testing.T) {
	rows := buildRows([]store.IssueLite{
		{IssueKey: "X-1", Summary: "Hello World", Assignee: strp("Alice")},
	})
	if rows[0].search != "x-1 hello world alice" {
		t.Fatalf("search haystack = %q", rows[0].search)
	}
}

func TestApplyListFilterUnassignedAndEmail(t *testing.T) {
	all := buildRows([]store.IssueLite{
		{IssueKey: "A-1", Summary: "one", StatusCategory: "new", Assignee: strp("Ada"), AssigneeEmail: strp("ada@x.com")},
		{IssueKey: "A-2", Summary: "two", StatusCategory: "new"},
		{IssueKey: "A-3", Summary: "three", StatusCategory: "done", AssigneeEmail: strp("bob@x.com")},
	})
	got := applyListFilter(all, listFilter{tab: TabAll, unassigned: true})
	if len(got) != 1 || all[got[0]].lite.IssueKey != "A-2" {
		t.Fatalf("unassigned: %#v", got)
	}
	got = applyListFilter(all, listFilter{tab: TabAll, assigneeEmail: "ada@x.com"})
	if len(got) != 1 || all[got[0]].lite.IssueKey != "A-1" {
		t.Fatalf("email: %#v", got)
	}
}

func TestParseSavedViewConfig(t *testing.T) {
	av := parseSavedViewConfig("Open mine", []byte(`{
		"filters": {
			"status_category": ["new","inprogress"],
			"q": "login",
			"labels": ["batch"],
			"unassigned": false
		},
		"display": {"sort":"updated","group_by":"status"}
	}`))
	if av.filter.text != "login" {
		t.Fatalf("text=%q", av.filter.text)
	}
	if len(av.filter.statusCategories) != 2 {
		t.Fatalf("cats=%v", av.filter.statusCategories)
	}
	if av.filter.tab != TabOpen {
		t.Fatalf("tab=%v", av.filter.tab)
	}
	if av.sort.key != "updated" || av.sort.dir != "desc" {
		t.Fatalf("sort=%+v", av.sort)
	}
	if av.groupBy != "status" {
		t.Fatalf("groupBy=%q", av.groupBy)
	}
	joined := strings.Join(av.unsupported, ",")
	if !strings.Contains(joined, "labels") {
		t.Fatalf("unsupported=%v want labels", av.unsupported)
	}
	if strings.Contains(joined, "display") || strings.Contains(joined, "sort") || strings.Contains(joined, "group") {
		t.Fatalf("supported display keys should not be unsupported: %v", av.unsupported)
	}

	// Flat simplified shape from store tests.
	av = parseSavedViewConfig("Mine", []byte(`{"assignee":"ada@x.com"}`))
	if av.filter.assigneeEmail != "ada@x.com" {
		t.Fatalf("flat assignee email=%q", av.filter.assigneeEmail)
	}
}

func TestParseSavedViewConfigDisplay(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantSort    string
		wantDir     string
		wantGroup   string
		wantUnsup   []string
		forbidUnsup []string
	}{
		{
			name:      "created asc + assignee group",
			raw:       `{"filters":{},"display":{"sort":"created","dir":"asc","group_by":"assignee"}}`,
			wantSort:  "created",
			wantDir:   "asc",
			wantGroup: "assignee",
		},
		{
			name:      "priority desc default dir",
			raw:       `{"display":{"sort":"priority","group_by":"priority"}}`,
			wantSort:  "priority",
			wantDir:   "desc",
			wantGroup: "priority",
		},
		{
			name:        "relevance unsupported",
			raw:         `{"display":{"sort":"relevance","dir":"desc","group_by":"status_category"}}`,
			wantSort:    "",
			wantDir:     "",
			wantGroup:   "status_category",
			wantUnsup:   []string{"sort=relevance"},
			forbidUnsup: []string{"group_by"},
		},
		{
			name:        "epic group_by supported",
			raw:         `{"display":{"sort":"updated","group_by":"epic"}}`,
			wantSort:    "updated",
			wantDir:     "desc",
			wantGroup:   "epic",
			forbidUnsup: []string{"group_by"},
		},
		{
			name:        "columns reported",
			raw:         `{"display":{"sort":"reopen_count","dir":"asc","group_by":"none","columns":["assignee"]}}`,
			wantSort:    "reopen_count",
			wantDir:     "asc",
			wantGroup:   "",
			wantUnsup:   []string{"columns"},
			forbidUnsup: []string{"sort", "group_by"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			av := parseSavedViewConfig(tc.name, []byte(tc.raw))
			if av.sort.key != tc.wantSort || av.sort.dir != tc.wantDir {
				t.Fatalf("sort=%+v want key=%q dir=%q", av.sort, tc.wantSort, tc.wantDir)
			}
			if av.groupBy != tc.wantGroup {
				t.Fatalf("groupBy=%q want %q", av.groupBy, tc.wantGroup)
			}
			joined := strings.Join(av.unsupported, ",")
			for _, u := range tc.wantUnsup {
				if !strings.Contains(joined, u) {
					t.Fatalf("unsupported=%v missing %q", av.unsupported, u)
				}
			}
			for _, u := range tc.forbidUnsup {
				if strings.Contains(joined, u) {
					t.Fatalf("unsupported=%v should not contain %q", av.unsupported, u)
				}
			}
		})
	}
}

func TestSortVisiblePriorityAndNullLast(t *testing.T) {
	// Priority names are deliberately Korean: Jira localizes them per account
	// language, so the sort must key on priority_rank (the issue's position in
	// the site's own list, 1 = most urgent) and never on the name. Rank 0 means
	// no priority, or one the site does not list.
	all := buildRows([]store.IssueLite{
		{IssueKey: "A", Priority: strp("낮음"), PriorityRank: 4, UpdatedAt: strp("2026-08-01T00:00:00Z")},
		{IssueKey: "B", Priority: strp("가장 높음"), PriorityRank: 1, UpdatedAt: strp("2026-08-02T00:00:00Z")},
		{IssueKey: "C", Priority: nil, UpdatedAt: strp("2026-08-03T00:00:00Z")},
		{IssueKey: "D", Priority: strp("보통"), PriorityRank: 3, UpdatedAt: strp("2026-08-04T00:00:00Z")},
		{IssueKey: "E", Priority: strp("Weird"), UpdatedAt: strp("2026-08-05T00:00:00Z")},
	})
	keysOf := func(vis []int) []string {
		out := make([]string, len(vis))
		for i, idx := range vis {
			out[i] = all[idx].lite.IssueKey
		}
		return out
	}

	// desc = most urgent first: rank 1 → 3 → 4, then the rank-0 rows. Among
	// those, the tiebreak is updated desc (E 08-05 before C 08-03), matching
	// how the web UI sorts the same saved view.
	vis := []int{0, 1, 2, 3, 4}
	sortVisible(all, vis, listSort{key: "priority", dir: "desc"})
	if got := keysOf(vis); strings.Join(got, ",") != "B,D,A,E,C" {
		t.Fatalf("priority desc: %v", got)
	}

	// asc = least urgent first: rank 4 → 3 → 1, rank-0 rows still last.
	vis = []int{0, 1, 2, 3, 4}
	sortVisible(all, vis, listSort{key: "priority", dir: "asc"})
	if got := keysOf(vis); strings.Join(got, ",") != "A,D,B,E,C" {
		t.Fatalf("priority asc: %v", got)
	}

	// updated desc: empty last regardless of direction
	all2 := buildRows([]store.IssueLite{
		{IssueKey: "X", UpdatedAt: strp("2026-08-01T00:00:00Z")},
		{IssueKey: "Y", UpdatedAt: nil},
		{IssueKey: "Z", UpdatedAt: strp("2026-08-03T00:00:00Z")},
	})
	vis = []int{0, 1, 2}
	sortVisible(all2, vis, listSort{key: "updated", dir: "asc"})
	if all2[vis[2]].lite.IssueKey != "Y" {
		t.Fatalf("null last asc: %v", keysOfVisible(all2, vis))
	}
	vis = []int{0, 1, 2}
	sortVisible(all2, vis, listSort{key: "updated", dir: "desc"})
	if all2[vis[2]].lite.IssueKey != "Y" {
		t.Fatalf("null last desc: %v", keysOfVisible(all2, vis))
	}
}

func keysOfVisible(all []row, vis []int) []string {
	out := make([]string, len(vis))
	for i, idx := range vis {
		out[i] = all[idx].lite.IssueKey
	}
	return out
}

func TestBuildListLinesGroupHeaders(t *testing.T) {
	all := buildRows([]store.IssueLite{
		{IssueKey: "A", StatusCategory: "done", Status: "Done"},
		{IssueKey: "B", StatusCategory: "new", Status: "Backlog"},
		{IssueKey: "C", StatusCategory: "inprogress", Status: "Doing"},
		{IssueKey: "D", StatusCategory: "new", Status: "To Do"},
	})
	// Pre-sorted as if by updated; grouping reorders groups only.
	vis := []int{0, 1, 2, 3}
	lines := buildListLines(all, vis, "status_category")
	// new (2) + inprogress (1) + done (1) = 3 headers + 4 issues = 7
	if len(lines) != 7 {
		t.Fatalf("lines=%d want 7", len(lines))
	}
	headers := 0
	var labels []string
	for _, ln := range lines {
		if ln.kind == lineKindHeader {
			headers++
			labels = append(labels, ln.label)
		}
	}
	if headers != 3 {
		t.Fatalf("headers=%d", headers)
	}
	if strings.Join(labels, ",") != "new,inprogress,done" {
		t.Fatalf("group order=%v", labels)
	}
	// No grouping: 1:1 with visible
	plain := buildListLines(all, vis, "")
	if len(plain) != 4 {
		t.Fatalf("ungrouped lines=%d", len(plain))
	}
	for i, ln := range plain {
		if ln.kind != lineKindIssue || ln.visIdx != i {
			t.Fatalf("plain[%d]=%+v", i, ln)
		}
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		iso  string
		want string
	}{
		{"2026-08-04T11:59:30Z", "just now"},
		{"2026-08-04T11:30:00Z", "30m ago"},
		{"2026-08-04T09:00:00Z", "3h ago"},
		{"2026-08-01T12:00:00Z", "3d ago"},
	}
	for _, tc := range cases {
		if got := relativeTime(tc.iso, now); got != tc.want {
			t.Errorf("relativeTime(%q)=%q want %q", tc.iso, got, tc.want)
		}
	}
}

func TestBuildListLinesEpicGroup(t *testing.T) {
	ep1, ep2 := "EP-1", "EP-2"
	all := buildRows([]store.IssueLite{
		{IssueKey: "EP-1", Summary: "Billing", EpicKey: &ep1}, // epic itself may carry no epic_key in real data
		{IssueKey: "ST-1", Summary: "Story A", EpicKey: &ep1},
		{IssueKey: "ST-2", Summary: "Story B", EpicKey: &ep2},
		{IssueKey: "OR-1", Summary: "Orphan", EpicKey: nil},
	})
	// Fix: epic row should have nil EpicKey (epics do not belong to themselves).
	all[0].lite.EpicKey = nil
	vis := []int{0, 1, 2, 3}
	lines := buildListLines(all, vis, "epic")
	var labels []string
	for _, ln := range lines {
		if ln.kind == lineKindHeader {
			labels = append(labels, ln.label)
		}
	}
	// Groups sorted by label asc; empty "(no epic)" last.
	// EP-1 has summary in pool → "EP-1 Billing"; EP-2 is not in pool as issue key with that summary...
	// EP-2 is not present as a row, so label is key only. EP-1 row is in pool with Summary Billing.
	joined := strings.Join(labels, "|")
	if !strings.Contains(joined, "EP-1 Billing") {
		t.Fatalf("want epic label with summary, got %v", labels)
	}
	if !strings.Contains(joined, "EP-2") {
		t.Fatalf("want EP-2 group, got %v", labels)
	}
	if labels[len(labels)-1] != "(no epic)" {
		t.Fatalf("empty epic group should be last, got %v", labels)
	}
	// Count: 3 headers + 4 issues = 7
	if len(lines) != 7 {
		t.Fatalf("lines=%d want 7", len(lines))
	}
}
