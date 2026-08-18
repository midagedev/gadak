package secretscan

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	// Fixtures use documented prefixes plus filler only. Never a live token.
	atlassian := func(prefix string) string { return prefix + strings.Repeat("A", 20) }
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Positives — one row per pattern the package declares.
		{"atlassian user token", atlassian("ATATT"), "atlassian_api_token"},
		{"atlassian org token", atlassian("ATCTT"), "atlassian_api_token"},
		{"atlassian token in prose", "leaked " + atlassian("ATATT") + " here", "atlassian_api_token"},
		{"http basic auth", "Authorization: Basic " + strings.Repeat("Q", 8), "http_basic_auth"},
		{"http basic auth case", "AUTHORIZATION:\tbasic " + strings.Repeat("Q", 8), "http_basic_auth"},
		{"http bearer token", "Authorization: Bearer " + strings.Repeat("t", 20), "http_bearer_token"},
		{"http bearer jwt charset", "Authorization: Bearer " + strings.Repeat("a", 10) + "." + strings.Repeat("b", 10), "http_bearer_token"},
		{"slack bot token", "xoxb-" + strings.Repeat("1", 10), "slack_token"},
		{"slack app token", "xoxa-" + strings.Repeat("2", 10), "slack_token"},
		{"slack user token", "xoxp-" + strings.Repeat("3", 10), "slack_token"},
		{"slack refresh token", "xoxr-" + strings.Repeat("4", 10), "slack_token"},
		{"slack config token", "xoxs-" + strings.Repeat("5", 10), "slack_token"},
		{"github classic pat", "ghp_" + strings.Repeat("a", 20), "github_token"},
		{"github oauth token", "gho_" + strings.Repeat("b", 20), "github_token"},
		{"github fine-grained pat", "github_pat_" + strings.Repeat("c", 20), "github_token"},
		{"linear personal api key", "lin_api_" + strings.Repeat("d", 32), "linear_api_key"},
		{"pem private key", "-----BEGIN PRIVATE KEY-----", "private_key_pem"},
		{"pem rsa private key", "-----BEGIN RSA PRIVATE KEY-----", "private_key_pem"},
		{"pem ec private key", "-----BEGIN EC PRIVATE KEY-----", "private_key_pem"},
		{"pem openssh private key", "-----BEGIN OPENSSH PRIVATE KEY-----", "private_key_pem"},
		{"pem encrypted private key", "-----BEGIN ENCRYPTED PRIVATE KEY-----", "private_key_pem"},

		// Below-threshold shapes that the regex requires a minimum payload for.
		{"atlassian too short", "ATATT" + strings.Repeat("A", 19), ""},
		{"basic too short", "Authorization: Basic ABCDEFG", ""},
		{"bearer too short", "Authorization: Bearer " + strings.Repeat("t", 19), ""},
		{"github too short", "ghp_" + strings.Repeat("a", 19), ""},
		{"linear too short", "lin_api_" + strings.Repeat("d", 19), ""},
		{"slack too short", "xoxb-" + strings.Repeat("1", 9), ""},

		// Negatives that must not block a legitimate export.
		{"assignee email", "assignee dana@example.com", ""},
		{"jira issue key", "See NMB-140 for the write-through path", ""},
		{"account id", "5b10a2c8e8d4f01234567890abcdef12", ""},
		{"base64 avatar url", "https://avatar.example/user?data=data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", ""},
		{"ordinary prose", "The webhook retry dropped the last page on staging.", ""},
		{"public key pem", "-----BEGIN PUBLIC KEY-----", ""},
		{"lowercase atlassian prefix", "atatt" + strings.Repeat("A", 20), ""},
		{"basic without header name", "Basic " + strings.Repeat("Q", 16), ""},
		{"bearer without header name", "Bearer " + strings.Repeat("t", 24), ""},
		{"slack class not in set", "xoxc-" + strings.Repeat("1", 10), ""},
		{"github server token not listed", "ghs_" + strings.Repeat("a", 20), ""},
		{"github prefix glued to word", "xghp_" + strings.Repeat("a", 20), ""},
		{"empty", "", ""},

		// First matching *pattern* wins (table order), not first occurrence in s.
		{"table order beats string order", "ghp_" + strings.Repeat("b", 20) + " " + atlassian("ATATT"), "atlassian_api_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Match(tc.in)
			if got != tc.want {
				t.Errorf("Match(...) = %q, want %q", got, tc.want)
			}
			if got != "" && strings.Contains(got, tc.in) {
				t.Errorf("Match returned the input value instead of a pattern name")
			}
		})
	}
}

func TestMatchNamesAreOnlyDeclaredPatterns(t *testing.T) {
	allowed := map[string]bool{
		"atlassian_api_token": true,
		"http_basic_auth":     true,
		"http_bearer_token":   true,
		"slack_token":         true,
		"github_token":        true,
		"linear_api_key":      true,
		"private_key_pem":     true,
	}
	for _, p := range patterns {
		if !allowed[p.name] {
			t.Errorf("undeclared pattern name %q", p.name)
		}
		delete(allowed, p.name)
	}
	for name := range allowed {
		t.Errorf("declared pattern %q missing from package table", name)
	}
}

func TestAtlassianPatternAgreesWithRepoScanner(t *testing.T) {
	assertScriptVarMatchesPattern(t, "PAT_TOKEN", "atlassian_api_token")
}

func TestLinearPatternAgreesWithRepoScanner(t *testing.T) {
	assertScriptVarMatchesPattern(t, "PAT_LINEAR", "linear_api_key")
}

// assertScriptVarMatchesPattern is the shared body of the two agreement tests:
// the package comment promises these regexes do not disagree with
// scripts/scan-internal.sh, and a promise with no assertion is a comment.
func assertScriptVarMatchesPattern(t *testing.T, scriptVar, patternName string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "scripts", "scan-internal.sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^` + scriptVar + `='([^']+)'`).FindSubmatch(body)
	if m == nil {
		t.Fatalf("%s assignment not found in scripts/scan-internal.sh", scriptVar)
	}
	script := string(m[1])
	var goPat string
	for _, p := range patterns {
		if p.name == patternName {
			goPat = p.re.String()
		}
	}
	if goPat == "" {
		t.Fatalf("%s missing from the package table", patternName)
	}
	if script != goPat {
		t.Errorf("scan-internal.sh %s=%q\nsecretscan regex=%q", scriptVar, script, goPat)
	}
}
