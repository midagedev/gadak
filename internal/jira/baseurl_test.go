package jira

import "testing"

// TestNewBaseURLTrailingSlash pins the origin New stores. Callers build
// browse links as BaseURL()+"/browse/"+key (internal/sync/sync.go); a leftover
// slash produces https://site//browse/KEY, which the UI then renders as a
// broken avatar/link.
//
// Avatar() lives on User in types.go (not client.go:61-62 — that is only
// BaseURL). It returns the 48px URL as Jira sent it, or "" when missing.
// There is no BaseURL join.
func TestNewBaseURLTrailingSlash(t *testing.T) {
	cases := []struct {
		site string
		want string
	}{
		{"https://example.atlassian.net", "https://example.atlassian.net"},
		{"https://example.atlassian.net/", "https://example.atlassian.net"},
		{"https://example.atlassian.net///", "https://example.atlassian.net"},
		{"", ""},
		{"/", ""},
	}
	for _, tc := range cases {
		got := New(tc.site, "dev@example.com", "token").BaseURL()
		if got != tc.want {
			t.Errorf("New(%q).BaseURL() = %q, want %q", tc.site, got, tc.want)
		}
	}
}

func TestUserAvatarEmptyAnd48px(t *testing.T) {
	// Blocks: a member row rendering a broken <img src> when Jira omitted
	// avatars, or picking the 16/24px URL when 48px is present.
	if got := (User{}).Avatar(); got != "" {
		t.Errorf("zero User.Avatar() = %q, want empty", got)
	}
	if got := (User{AvatarURLs: map[string]string{}}).Avatar(); got != "" {
		t.Errorf("empty map Avatar() = %q, want empty", got)
	}
	if got := (User{AvatarURLs: map[string]string{"24x24": "https://avatars.example/24.png"}}).Avatar(); got != "" {
		t.Errorf("24px-only Avatar() = %q, want empty (only 48x48 is used)", got)
	}
	const want = "https://avatars.example/48.png"
	if got := (User{AvatarURLs: map[string]string{"48x48": want, "24x24": "https://avatars.example/24.png"}}).Avatar(); got != want {
		t.Errorf("Avatar() = %q, want %q", got, want)
	}
}
