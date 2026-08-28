// Package secretscan holds the credential-shaped string patterns that every
// outbound artifact is checked against before it is written.
//
// It lives on its own because more than one surface produces something a user
// may hand to someone else — a snapshot database, a team config file — and
// those surfaces have nothing else in common. Whichever one had owned the
// patterns would have become an accidental dependency of the others.
//
// The Atlassian and Linear token patterns must not disagree with
// scripts/scan-internal.sh, which guards the repository itself.
package secretscan

import "regexp"

// Pattern is one row of the credential-shape table: the name Match reports
// and the regexp that recognizes it.
type Pattern struct {
	Name string
	Re   *regexp.Regexp
}

var patterns = []Pattern{
	{"atlassian_api_token", regexp.MustCompile(`ATATT[A-Za-z0-9+/=_-]{20,}|ATCTT[A-Za-z0-9+/=_-]{20,}`)},
	{"http_basic_auth", regexp.MustCompile(`(?i)Authorization:\s*Basic\s+[A-Za-z0-9+/=_-]{8,}`)},
	{"http_bearer_token", regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+[A-Za-z0-9._-]{20,}`)},
	{"slack_token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)},
	{"github_token", regexp.MustCompile(`\b(?:ghp_|gho_|github_pat_)[A-Za-z0-9_]{20,}`)},
	// Linear personal API key (Settings > Security & access). Added when a real
	// one first entered this machine for the GDK-263 connector work: a tracker
	// gadak may one day mirror is exactly the kind of credential that ends up
	// pasted into a fixture, and the scanners had no shape for it.
	{"linear_api_key", regexp.MustCompile(`lin_api_[A-Za-z0-9]{20,}`)},
	{"private_key_pem", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
}

// Match reports the name of the first credential-shaped pattern found in s, or
// "" when none match. Callers report the name and never the value: a diagnostic
// that quotes the secret has published it a second time.
//
// Email addresses are deliberately not a pattern here. A mirror legitimately
// carries assignee emails, and treating them as secrets would refuse every
// real snapshot.
func Match(s string) string {
	for _, p := range patterns {
		if p.Re.MatchString(s) {
			return p.Name
		}
	}
	return ""
}

// Patterns returns the pattern table for surfaces that erase rather than
// refuse — internal/applog composes its log scrubber over it, so the two
// lists cannot drift apart silently (the applog corpus test fails until a
// new pattern name is covered there). The slice is shared with Match;
// treat it as read-only.
func Patterns() []Pattern {
	return patterns
}
