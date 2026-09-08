package config

import "testing"

// A Jira Server workspace is its own origin type, not Cloud with a flag
// (GDK-1635) — and an existing config, which has no such Kind, is still
// Cloud.
func TestJiraServerIsItsOwnOriginType(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want string
	}{
		{OriginJiraServer, OriginJiraServer},
		{OriginJira, OriginJira},
		{"", OriginJira}, // pre-split config.json: connected means Cloud
		{KindConnected, OriginJira},
	} {
		cfg := &Config{Kind: tc.kind, Site: "https://example.atlassian.net", Email: "a@b.c", Token: "t"}
		if got := cfg.OriginType(); got != tc.want {
			t.Fatalf("Kind %q: OriginType = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

// Server is remote like Cloud: only the built-in tracker runs in-process.
func TestJiraServerIsARemoteTransport(t *testing.T) {
	cfg := &Config{Kind: OriginJiraServer, Site: "http://jira.example/jira", Token: "pat"}
	if got := cfg.Transport(); got != TransportRemote {
		t.Fatalf("Transport = %q, want %q", got, TransportRemote)
	}
}

func TestJiraFamilyCoversBothJirasAndNothingElse(t *testing.T) {
	for _, ok := range []string{OriginJira, OriginJiraServer} {
		if !JiraFamily(ok) {
			t.Fatalf("JiraFamily(%q) = false", ok)
		}
	}
	for _, no := range []string{OriginLinear, OriginGadak, ""} {
		if JiraFamily(no) {
			t.Fatalf("JiraFamily(%q) = true", no)
		}
	}
}
