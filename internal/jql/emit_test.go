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
