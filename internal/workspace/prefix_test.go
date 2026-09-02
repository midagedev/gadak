package workspace

import "testing"

// These cases are the ones cmd/gadak/views_test.go pinned before the rule
// moved here; they are kept identical on purpose, because the reason for
// moving it was that two surfaces have to agree, and a rule that changed
// shape in the move would defeat that.
func TestPrefix(t *testing.T) {
	for _, tc := range []struct {
		name            string
		profile, served string
		want            string
	}{
		{"same named profile is already mounted", "work", "work", ""},
		{"a named profile on the primary server is a mount", "work", "", "/w/work"},
		// GitHub #85 / GDK-1309: /w/default is a mount that can never answer,
		// so the primary is / on every server.
		{"the primary profile on a named server is the root", "", "work", ""},
		{"default and empty are the same mirror", "default", "", ""},
		{"empty and default are the same mirror", "", "default", ""},
		{"default spelled out on a named server is the root", "default", "work", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Prefix(tc.profile, tc.served); got != tc.want {
				t.Fatalf("Prefix(%q, %q) = %q, want %q", tc.profile, tc.served, got, tc.want)
			}
		})
	}
}

func TestProfileEq(t *testing.T) {
	for _, tc := range []struct {
		got, want string
		equal     bool
	}{
		{"", "", true},
		{"default", "", true},
		{"", "default", true},
		{"default", "default", true},
		{"work", "work", true},
		{"work", "", false},
		{"", "work", false},
		{"work", "default", false},
		// Not case-folded: profile names are directory names, and the config
		// layer does not fold them either. If that ever changes, it changes
		// in one place now.
		{"Work", "work", false},
	} {
		if got := ProfileEq(tc.got, tc.want); got != tc.equal {
			t.Fatalf("ProfileEq(%q, %q) = %v, want %v", tc.got, tc.want, got, tc.equal)
		}
	}
}
