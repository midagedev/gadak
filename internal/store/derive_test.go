package store

import "testing"

// Status ids are the same on both sites; only the display names differ. Every
// case below is run twice, once per naming, and the results must be identical.
var (
	englishNames = map[string]string{"1": "To Do", "3": "In Progress", "5": "Done", "9": "Awaiting Release"}
	koreanNames  = map[string]string{"1": "해야 할 일", "3": "진행 중", "5": "완료", "9": "릴리즈 대기"}
	// The category map a connector builds from the site's status list.
	categories = map[string]string{"1": "new", "3": "inprogress", "5": "done"} // "9" deliberately absent
)

func statusChange(at, fromID, toID string, names map[string]string) ChangeEntry {
	return ChangeEntry{
		At: at, Field: "status",
		FromValue: names[fromID], FromID: fromID,
		ToValue: names[toID], ToID: toID,
	}
}

func TestDerive(t *testing.T) {
	cases := []struct {
		name            string
		changelog       func(names map[string]string) []ChangeEntry
		currentCategory string
		want            Derived
	}{
		{
			name:            "no transitions",
			changelog:       func(map[string]string) []ChangeEntry { return nil },
			currentCategory: "new",
			want:            Derived{},
		},
		{
			name: "resolved once",
			changelog: func(n map[string]string) []ChangeEntry {
				return []ChangeEntry{
					statusChange("2026-07-01T00:00:00Z", "1", "3", n),
					statusChange("2026-07-05T00:00:00Z", "3", "5", n),
				}
			},
			currentCategory: "done",
			want: Derived{
				StatusChangedAt: str("2026-07-05T00:00:00Z"),
				ResolvedAt:      str("2026-07-05T00:00:00Z"),
			},
		},
		{
			name: "done to todo is a reopen",
			changelog: func(n map[string]string) []ChangeEntry {
				return []ChangeEntry{
					statusChange("2026-07-05T00:00:00Z", "3", "5", n),
					statusChange("2026-07-09T00:00:00Z", "5", "1", n),
				}
			},
			currentCategory: "new",
			want: Derived{
				StatusChangedAt: str("2026-07-09T00:00:00Z"),
				ReopenCount:     1,
				ReopenedAt:      str("2026-07-09T00:00:00Z"),
				// resolved_at is cleared: the issue is not done any more.
			},
		},
		{
			name: "multiple reopens keep the newest",
			changelog: func(n map[string]string) []ChangeEntry {
				return []ChangeEntry{
					statusChange("2026-07-01T00:00:00Z", "1", "5", n),
					statusChange("2026-07-02T00:00:00Z", "5", "3", n),
					statusChange("2026-07-03T00:00:00Z", "3", "5", n),
					statusChange("2026-07-04T00:00:00Z", "5", "1", n),
					statusChange("2026-07-05T00:00:00Z", "1", "5", n),
				}
			},
			currentCategory: "done",
			want: Derived{
				StatusChangedAt: str("2026-07-05T00:00:00Z"),
				ResolvedAt:      str("2026-07-05T00:00:00Z"),
				ReopenCount:     2,
				ReopenedAt:      str("2026-07-04T00:00:00Z"),
			},
		},
		{
			name: "entries out of order are sorted by time",
			changelog: func(n map[string]string) []ChangeEntry {
				return []ChangeEntry{
					statusChange("2026-07-09T00:00:00Z", "5", "3", n),
					statusChange("2026-07-05T00:00:00Z", "3", "5", n),
				}
			},
			currentCategory: "inprogress",
			want: Derived{
				StatusChangedAt: str("2026-07-09T00:00:00Z"),
				ReopenCount:     1,
				ReopenedAt:      str("2026-07-09T00:00:00Z"),
			},
		},
		{
			name: "an unmapped status counts as not done",
			changelog: func(n map[string]string) []ChangeEntry {
				return []ChangeEntry{
					statusChange("2026-07-05T00:00:00Z", "3", "9", n),
					statusChange("2026-07-06T00:00:00Z", "9", "5", n),
					statusChange("2026-07-07T00:00:00Z", "5", "9", n),
				}
			},
			currentCategory: "inprogress",
			want: Derived{
				StatusChangedAt: str("2026-07-07T00:00:00Z"),
				ReopenCount:     1, // only 5 -> 9; nothing into or out of 9 counts otherwise
				ReopenedAt:      str("2026-07-07T00:00:00Z"),
			},
		},
		{
			name: "assignee change is tracked separately",
			changelog: func(n map[string]string) []ChangeEntry {
				return []ChangeEntry{
					{At: "2026-07-01T00:00:00Z", Field: "assignee", ToValue: "Ada"},
					statusChange("2026-07-02T00:00:00Z", "1", "3", n),
					{At: "2026-07-03T00:00:00Z", Field: "assignee", FromValue: "Ada", ToValue: "Grace"},
					{At: "2026-07-04T00:00:00Z", Field: "priority", FromValue: "Low", ToValue: "High"},
				}
			},
			currentCategory: "inprogress",
			want: Derived{
				StatusChangedAt:   str("2026-07-02T00:00:00Z"),
				AssigneeChangedAt: str("2026-07-03T00:00:00Z"),
			},
		},
	}

	for _, c := range cases {
		for _, site := range []struct {
			label string
			names map[string]string
		}{{"english", englishNames}, {"korean", koreanNames}} {
			t.Run(c.name+"/"+site.label, func(t *testing.T) {
				got := Derive(DeriveInput{
					Changelog:       c.changelog(site.names),
					Categories:      categories,
					CurrentCategory: c.currentCategory,
				})
				assertDerived(t, got, c.want)
			})
		}
	}
}

func TestDeriveCountsAndPriorityRank(t *testing.T) {
	priorities := []string{"Highest", "High", "Medium", "Low", "Lowest"}
	for _, c := range []struct {
		priority string
		want     int
	}{
		{"Highest", 1},
		{"Medium", 3},
		{"Lowest", 5},
		{"", 0},
		{"Not On This Site", 0},
	} {
		got := Derive(DeriveInput{Priority: c.priority, Priorities: priorities, CommentCount: 4})
		if got.PriorityRank != c.want {
			t.Errorf("priority %q rank = %d, want %d", c.priority, got.PriorityRank, c.want)
		}
		if got.CommentCount != 4 {
			t.Errorf("comment_count = %d, want 4", got.CommentCount)
		}
	}
}

func assertDerived(t *testing.T, got, want Derived) {
	t.Helper()
	eq := func(name string, g, w *string) {
		switch {
		case g == nil && w == nil:
		case g == nil || w == nil:
			t.Errorf("%s = %v, want %v", name, deref(g), deref(w))
		case *g != *w:
			t.Errorf("%s = %q, want %q", name, *g, *w)
		}
	}
	eq("status_changed_at", got.StatusChangedAt, want.StatusChangedAt)
	eq("resolved_at", got.ResolvedAt, want.ResolvedAt)
	eq("reopened_at", got.ReopenedAt, want.ReopenedAt)
	eq("assignee_changed_at", got.AssigneeChangedAt, want.AssigneeChangedAt)
	if got.ReopenCount != want.ReopenCount {
		t.Errorf("reopen_count = %d, want %d", got.ReopenCount, want.ReopenCount)
	}
}

func str(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
