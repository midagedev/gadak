package applog

import (
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/secretscan"
)

// The corpus calls the internal scrub directly: it runs on every log write,
// and installing a logger per shape would test Install, not scrub.
//
// Fixtures are documented prefixes plus filler only — never a live token —
// and are built by concatenation so no source line carries a complete
// credential shape for scripts/scan-internal.sh to hit (same convention as
// the fixtures in secretscan_test.go).
var corpusFixtures = map[string]string{
	"atlassian_api_token": "ATATT" + strings.Repeat("A", 20),
	"http_basic_auth":     "Authorization: Basic " + strings.Repeat("Q", 8),
	"http_bearer_token":   "Authorization: Bearer " + strings.Repeat("t", 20),
	"slack_token":         "xoxb-" + strings.Repeat("1", 10),
	"github_token":        "ghp_" + strings.Repeat("a", 20),
	"linear_api_key":      "lin_api_" + strings.Repeat("d", 32),
	"private_key_pem":     "-----BEGIN PRIVATE KEY-----",
}

// TestScrubCoversSecretscanPatterns is the drift gate between the two
// surfaces: secretscan owns the credential shapes and scrub erases them, and
// nothing else forces the second fact to follow the first. A pattern name
// with no fixture here fails the test, so adding a shape to secretscan
// without proving the scrubber erases it cannot land green.
func TestScrubCoversSecretscanPatterns(t *testing.T) {
	for _, p := range secretscan.Patterns() {
		fixture, ok := corpusFixtures[p.Name]
		if !ok {
			t.Errorf("new secretscan pattern %q has no applog corpus fixture — add one and confirm scrub covers it", p.Name)
			continue
		}
		if !p.Re.MatchString(fixture) {
			t.Errorf("fixture for %q does not match its own pattern — fix the fixture, not the pattern", p.Name)
			continue
		}
		got := string(scrub([]byte("prose " + fixture + " prose")))
		if strings.Contains(got, fixture) {
			t.Errorf("scrub left the %q fixture intact: %q", p.Name, got)
		}
		if !strings.Contains(got, "<redacted>") {
			t.Errorf("scrub produced no <redacted> for %q: %q", p.Name, got)
		}
	}
	// The reverse drift: a pattern removed from secretscan would leave a
	// fixture behind that no longer proves anything.
	for name := range corpusFixtures {
		declared := false
		for _, p := range secretscan.Patterns() {
			if p.Name == name {
				declared = true
				break
			}
		}
		if !declared {
			t.Errorf("applog corpus fixture %q matches no secretscan pattern — stale entry, remove it", name)
		}
	}
}

// TestScrubBearerKeepsHeaderShape pins the composition order the scrubber
// comment argues for: the wide bearer runs before the secretscan table, so
// an Authorization-prefixed bearer keeps its header and prefix in the log
// exactly as it did before the table composition. If someone reorders the
// two, the narrow http_bearer_token row collapses the whole header to a bare
// <redacted> and this fails.
func TestScrubBearerKeepsHeaderShape(t *testing.T) {
	in := "Authorization: Bearer " + strings.Repeat("t", 20)
	want := "Authorization: Bearer <redacted>"
	if got := string(scrub([]byte(in))); got != want {
		t.Fatalf("scrub(bearer with header) = %q, want %q", got, want)
	}
}
