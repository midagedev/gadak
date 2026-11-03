package origin

import "testing"

// DecideAssign's contract table (GDK-2063 CLI, GDK-2073 web/phone) — every
// row of cmd/gadak/assign_stale_test.go's S1–S6 map that is the pure
// decision's to make, plus the shapes only the shared owner can lose:
// the no-op outranking a stale cache, and a target nobody holding it.
// The read-failure refusal (S6) is deliberately absent: a caller whose read
// failed refuses on its own and never consults DecideAssign — a nil current
// would read as "unassigned", which is not a fact that caller has.
func TestDecideAssign(t *testing.T) {
	dana := &User{AccountID: "acc-hc", DisplayName: "Dana Whitfield"}
	cases := []struct {
		name       string
		current    *User
		cacheID    string
		cacheKnown bool
		target     string
		force      bool
		want       AssignVerdict
	}{
		// S1 stale: mirror ≠ origin → refuse (the caller names the holder).
		{"stale: cache empty, origin holds", dana, "", true, "acc-mr", false, AssignStale},
		{"stale: cache holds someone else", dana, "acc-old", true, "acc-mr", false, AssignStale},
		// S2 stale + force → the write goes out.
		{"force writes past a stale cache", dana, "", true, "acc-mr", true, AssignWrite},
		// S3 fresh: mirror == origin → the deliberate, seen replace.
		{"fresh replace writes", dana, "acc-hc", true, "acc-mr", false, AssignWrite},
		// S4 unassign reports the same way: target "" vs a held issue.
		{"unassign over a fresh cache writes", dana, "acc-hc", true, "", false, AssignWrite},
		{"unassign over a stale cache refuses", dana, "acc-old", true, "", false, AssignStale},
		// S5 the target already holds it → no write, whatever the cache says
		// and whatever force says.
		{"target holds it: no-op over a stale cache", dana, "acc-old", true, "acc-hc", false, AssignNoop},
		{"target holds it: no-op even under force", dana, "acc-hc", true, "acc-hc", true, AssignNoop},
		{"unassign when nobody holds it: no-op", nil, "acc-old", true, "", false, AssignNoop},
		// A key the mirror has no row for has nothing to compare and writes
		// as it always did.
		{"no cache row: write", dana, "", false, "acc-mr", false, AssignWrite},
		{"no cache row under nil holder: write", nil, "", false, "acc-mr", false, AssignWrite},
		// A stale cache must not block the one replace it exists to allow
		// once force is paid.
		{"force unassign over a stale cache writes", dana, "acc-old", true, "", true, AssignWrite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DecideAssign(tc.current, tc.cacheID, tc.cacheKnown, tc.target, tc.force)
			if got != tc.want {
				t.Fatalf("DecideAssign(%+v, cache=%q known=%v, target=%q, force=%v) = %v, want %v",
					tc.current, tc.cacheID, tc.cacheKnown, tc.target, tc.force, got, tc.want)
			}
		})
	}
}

// The id and display-name spellings the verdict compares and the refusal
// reports — the CLI's assigneeID/assigneeDisplayName delegates here, so the
// nil and empty-name edges live once.
func TestAssigneeIDAndDisplayName(t *testing.T) {
	if got := AssigneeID(nil); got != "" {
		t.Errorf("AssigneeID(nil) = %q, want empty", got)
	}
	if got := AssigneeID(&User{}); got != "" {
		t.Errorf("AssigneeID(&User{}) = %q, want empty", got)
	}
	if got := AssigneeID(&User{AccountID: "acc-hc"}); got != "acc-hc" {
		t.Errorf("AssigneeID account = %q", got)
	}
	// A Jira Server user keys by name when accountId is absent (User.ID).
	if got := AssigneeID(&User{Name: "hc"}); got != "hc" {
		t.Errorf("AssigneeID server name = %q", got)
	}
	if got := AssigneeDisplayName(nil); got != "" {
		t.Errorf("AssigneeDisplayName(nil) = %q, want empty", got)
	}
	if got := AssigneeDisplayName(&User{AccountID: "acc-hc"}); got != "acc-hc" {
		t.Errorf("no display name must fall back to the id: %q", got)
	}
	if got := AssigneeDisplayName(&User{AccountID: "acc-hc", DisplayName: "  Dana  "}); got != "Dana" {
		t.Errorf("display name must trim: %q", got)
	}
}
