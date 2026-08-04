package tui

import (
	"testing"
	"time"

	"github.com/midagedev/scry/internal/store"
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
		{TabOpen, 3},      // non-done
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

func TestBuildRowsPreLowercases(t *testing.T) {
	rows := buildRows([]store.IssueLite{
		{IssueKey: "X-1", Summary: "Hello World", Assignee: strp("Alice")},
	})
	if rows[0].search != "x-1 hello world alice" {
		t.Fatalf("search haystack = %q", rows[0].search)
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
