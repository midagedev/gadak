package jql

import "testing"

// GDK-1216: every sprint state emitted openSprints(), so a `closed` filter
// asked for the opposite set. The mapping is measured, not assumed — Jira
// 11.3.11, one active sprint and one future one: openSprints() returned only
// the active one's issue.
func TestSprintStateEmitsItsOwnFunction(t *testing.T) {
	for _, tc := range []struct {
		states []string
		want   string
	}{
		{[]string{"active"}, "sprint in openSprints()"},
		{[]string{"future"}, "sprint in futureSprints()"},
		{[]string{"closed"}, "sprint in closedSprints()"},
		{[]string{"active", "future"}, "(sprint in openSprints() OR sprint in futureSprints())"},
		{[]string{"ACTIVE", "active"}, "sprint in openSprints()"},
		{[]string{"nonsense"}, ""},
		{nil, ""},
	} {
		if got := sprintStateClause(tc.states, false); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.states, got, tc.want)
		}
	}
}

// GDK-1862: five of the nine sort keys have no Jira field to be, and the
// emitter replaced them with `updated` without saying so. `omitted` is the
// toast's whole promise — "the Jira link cannot carry {omitted}" — so a loss
// missing from it is a toast that lies by omission.
//
// The judgement the issue left open is whether order is part of a view's
// identity to whoever opens the link. It is. A view named for its order is
// not that view under another one: "the quietest first" opened newest-first
// is the same set and the wrong list, which is the argument GDK-1992 settled
// on the phone the same week — the delegation ledger came out newest-first
// and the view's whole point was gone.
//
// FAIL-first (2026-09-18, with the omitted line taken out again):
//
//	sort status_changed: omitted [], named=false want true
//	sort started:        omitted [], named=false want true
//	sort reopen_count:   omitted [], named=false want true
//	sort relevance:      omitted [], named=false want true
//	sort keys:           omitted [], named=false want true
func TestEmitNamesASortJiraCannotTake(t *testing.T) {
	for _, tc := range []struct {
		sort  string
		field string
		named bool
	}{
		// The four that survive, named by the Jira field they become.
		{"updated", "updated", false},
		{"created", "created", false},
		{"priority", "priority", false},
		{"due", "duedate", false},
		// The five that do not. Every one of them used to become `updated`
		// in silence.
		{"status_changed", "updated", true},
		{"started", "updated", true},
		{"reopen_count", "updated", true},
		{"relevance", "updated", true},
		{"keys", "updated", true},
	} {
		jql, omitted := Emit(Filter{}, Display{Sort: tc.sort, Dir: "asc"}, EmitOpts{})
		if want := "ORDER BY " + tc.field + " ASC"; jql != want {
			t.Errorf("sort %s: jql %q, want %q", tc.sort, jql, want)
		}
		named := false
		for _, o := range omitted {
			if o == "sort by "+tc.sort {
				named = true
			}
		}
		if named != tc.named {
			t.Errorf("sort %s: omitted %v, named=%v want %v", tc.sort, omitted, named, tc.named)
		}
	}
}
