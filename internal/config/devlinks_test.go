package config

import "testing"

// GDK-1496: dev_links was empty across an 11,792-issue Cloud mirror and no
// surface said why — the fetch is off by default on a connected Cloud
// workspace, so "no pull requests" and "never asked" looked identical. The
// fix is one owner for that question, consulted by sync, doctor and status
// alike; this pins the answer per origin.
//
// FAIL-first: MirrorsDevLinks did not exist before this change, so the
// package did not compile — build failure is this test's red.
func TestMirrorsDevLinks(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"nil workspace", nil, false},
		// gadak's own tracker, in-process: the panel is local and the flag
		// must not drain it (GDK-536).
		{"built-in", &Config{Kind: OriginGadak}, true},
		{"built-in legacy kind", &Config{Kind: KindStandalone}, true},
		// Connected Cloud is opt-in — the GDK-1496 field case.
		{"cloud default", &Config{Kind: OriginJira, Site: "https://x.atlassian.net"}, false},
		{"cloud opted in", &Config{Kind: OriginJira, Site: "https://x.atlassian.net", DevStatus: true}, true},
		{"jira server default", &Config{Kind: OriginJiraServer}, false},
		{"jira server opted in", &Config{Kind: OriginJiraServer, DevStatus: true}, true},
		// Linear has no development panel at all; the flag cannot conjure
		// one, and reporting "synced" there would be a false statement.
		{"linear", &Config{Kind: OriginLinear}, false},
		{"linear with the flag set", &Config{Kind: OriginLinear, DevStatus: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.MirrorsDevLinks(); got != tc.want {
				t.Fatalf("MirrorsDevLinks() = %v, want %v", got, tc.want)
			}
		})
	}
}
