package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

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
		got := Derive(DeriveInput{Priority: c.priority, Priorities: priorities,
			Comments: []Comment{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}}})
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

func TestDeriveReopenReason(t *testing.T) {
	reopen := []ChangeEntry{
		statusChange("2026-07-05T00:00:00Z", "1", "5", englishNames),
		statusChange("2026-07-10T00:00:00Z", "5", "3", englishNames),
	}
	comments := []Comment{
		{ID: "c1", BodyText: "before the reopen", CreatedAt: "2026-07-06T00:00:00Z"},
		{ID: "c2", BodyText: "this is why it came back", CreatedAt: "2026-07-10T01:00:00Z"},
		{ID: "c3", BodyText: "a later comment", CreatedAt: "2026-07-11T00:00:00Z"},
	}
	got := Derive(DeriveInput{Changelog: reopen, Categories: categories,
		CurrentCategory: "inprogress", Comments: comments})
	if got.ReopenReason != "this is why it came back" {
		t.Fatalf("reopen_reason = %q", got.ReopenReason)
	}

	// No comment after the reopen -> empty, never a pre-reopen comment.
	got = Derive(DeriveInput{Changelog: reopen, Categories: categories,
		CurrentCategory: "inprogress", Comments: comments[:1]})
	if got.ReopenReason != "" {
		t.Fatalf("reopen_reason without post-reopen comment = %q", got.ReopenReason)
	}

	// Never reopened -> empty even with comments.
	got = Derive(DeriveInput{Categories: categories, CurrentCategory: "new", Comments: comments})
	if got.ReopenReason != "" {
		t.Fatalf("reopen_reason without reopen = %q", got.ReopenReason)
	}
}

func TestDeriveReopenReasonTruncatesOnRuneBoundary(t *testing.T) {
	long := ""
	for len(long) < 1100 {
		long += "가나다라마바사아자차" // 3 bytes per rune
	}
	got := Derive(DeriveInput{
		Changelog: []ChangeEntry{
			statusChange("2026-07-05T00:00:00Z", "1", "5", englishNames),
			statusChange("2026-07-10T00:00:00Z", "5", "3", englishNames),
		},
		Categories: categories, CurrentCategory: "inprogress",
		Comments: []Comment{{ID: "c", BodyText: long, CreatedAt: "2026-07-10T01:00:00Z"}},
	})
	if len(got.ReopenReason) == 0 || len(got.ReopenReason) > 1000 {
		t.Fatalf("truncated length = %d", len(got.ReopenReason))
	}
	for i, r := range got.ReopenReason {
		if r == '�' {
			t.Fatalf("invalid rune at byte %d — split mid-sequence", i)
		}
	}
}

// The clone displays the outward phrase ("clones <origin>"), so the origin
// sits behind the OUTWARD link. Inward ("is cloned by") marks the origin
// itself and must derive nothing (GDK-1214; inverted before it).
func TestDeriveClonedFrom(t *testing.T) {
	links := []Link{
		{Type: "Blocks", Direction: "inward", TargetKey: "NMA-1"},
		{Type: "Cloners", Direction: "outward", TargetKey: "NMS-42"},
	}
	if got := Derive(DeriveInput{Links: links}); got.ClonedFrom != "NMS-42" {
		t.Fatalf("cloned_from = %q", got.ClonedFrom)
	}
	// Inward clone links ("is cloned by X") sit on the origin, not the clone.
	inOnly := []Link{{Type: "Cloners", Direction: "inward", TargetKey: "NMS-42"}}
	if got := Derive(DeriveInput{Links: inOnly}); got.ClonedFrom != "" {
		t.Fatalf("the origin derived a cloned_from: %q", got.ClonedFrom)
	}
}

// TestMigrateV40RecomputesClonedFrom seeds a v39 mirror whose cloned_from was
// derived from the inward Cloners link (the pre-GDK-1214 inversion: the value
// sat on the origin, the clone had none) and asserts v40 moves it to the
// clone, straight from the links rows the mirror already holds.
func TestMigrateV40RecomputesClonedFrom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 39; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 39`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sources (id, kind) VALUES ('jira', 'jira')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	insert := func(id, key, clonedFrom string) {
		t.Helper()
		itemID := "jira:" + id
		if _, err := raw.Exec(`
			INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at)
			VALUES (?, 'jira', 'issue', ?, ?, ?, '2026-01-01', '2026-01-02', '2026-01-02')`,
			itemID, id, key, key); err != nil {
			t.Fatal(err)
		}
		if _, err := raw.Exec(`
			INSERT INTO issues (item_id, key, project_key, priority_rank, reopen_count, comment_count, raw, cloned_from)
			VALUES (?, ?, 'NMB', 0, 0, 0, '{}', ?)`,
			itemID, key, clonedFrom); err != nil {
			t.Fatal(err)
		}
	}
	// NMB-1 is the origin (inward "is cloned by"), NMB-2 the clone (outward
	// "clones NMB-1") — the inverted derive had stamped the origin.
	insert("1", "NMB-1", "NMB-2")
	insert("2", "NMB-2", "")
	if _, err := raw.Exec(`
		INSERT INTO links (item_id, type, direction, target_key) VALUES
		('jira:1', 'Cloners', 'inward',  'NMB-2'),
		('jira:2', 'Cloners', 'outward', 'NMB-1')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("migrate to v40: %v", err)
	}
	defer db.Close()
	got := map[string]string{}
	rows, err := db.sql.QueryContext(context.Background(), `SELECT key, cloned_from FROM issues ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var k, c string
		if err := rows.Scan(&k, &c); err != nil {
			t.Fatal(err)
		}
		got[k] = c
	}
	if got["NMB-1"] != "" || got["NMB-2"] != "NMB-1" {
		t.Fatalf("cloned_from after v40 = %v, want origin empty and clone → NMB-1", got)
	}
}
