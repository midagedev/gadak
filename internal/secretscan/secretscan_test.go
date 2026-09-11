package secretscan

import (
	"os"
	"os/exec"
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
		if !allowed[p.Name] {
			t.Errorf("undeclared pattern name %q", p.Name)
		}
		delete(allowed, p.Name)
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
		if p.Name == patternName {
			goPat = p.Re.String()
		}
	}
	if goPat == "" {
		t.Fatalf("%s missing from the package table", patternName)
	}
	if script != goPat {
		t.Errorf("scan-internal.sh %s=%q\nsecretscan regex=%q", scriptVar, script, goPat)
	}
}

func TestSlackPatternAgreesWithRepoScanner(t *testing.T) {
	assertScriptVarMatchesPattern(t, "PAT_SLACK", "slack_token")
}

func TestPEMPatternAgreesWithRepoScanner(t *testing.T) {
	assertScriptVarMatchesPattern(t, "PAT_PEM", "private_key_pem")
}

// treeScanned records, for every shape in the package table, whether
// scripts/scan-internal.sh greps the repository for it, and a synthetic vector
// to prove it either way. The map is the single answer to "which shapes does
// the repo scanner cover" — adding a pattern to the package without deciding
// its tree coverage fails TestRepoScannerTreeCoverageIsExhaustive below.
//
// private_key_pem turned true in GDK-1797: every shape in the table is now
// scanned over the tree. Its legitimate hits — the drift-gate corpora that
// exist to contain a PEM header — are exempted per file in pem-exemptions.txt
// (this package's directory), the single home of that list: the script drops
// exempted hits and fails stale entries from it, and the tests below enforce
// the same discipline from the Go side, where the canonical shape lives.
//
// Vectors are synthetic filler behind a documented prefix. Never a live token.
var treeScanned = map[string]struct {
	vector  string
	scanned bool
}{
	"atlassian_api_token": {"ATATT" + strings.Repeat("A", 24), true},
	"linear_api_key":      {"lin_api_" + strings.Repeat("d", 32), true},
	"slack_token":         {"xoxb-" + strings.Repeat("1", 14), true},
	"github_token":        {"ghp_" + strings.Repeat("a", 24), true},
	"http_basic_auth":     {"Authorization: Basic " + strings.Repeat("Q", 12), true},
	"http_bearer_token":   {"Authorization: Bearer " + strings.Repeat("t", 24), true},
	"private_key_pem":     {"-----BEGIN PRIVATE KEY-----", true},
}

func TestRepoScannerTreeCoverageIsExhaustive(t *testing.T) {
	for _, p := range patterns {
		if _, ok := treeScanned[p.Name]; !ok {
			t.Errorf("pattern %q has no tree-coverage decision in treeScanned", p.Name)
		}
	}
	for name := range treeScanned {
		found := false
		for _, p := range patterns {
			if p.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("treeScanned lists %q, which is not a package pattern", name)
		}
	}
}

// TestRepoScannerMatchesTreeCoverage runs the actual script over a directory
// holding one synthetic vector, once per shape. Behaviour and not bytes,
// because three of the shapes cannot be spelled identically in POSIX ERE
// ((?i), (?:...), \b, \s) — a byte pin for those would force the script to
// carry a regex grep does not understand.
func TestRepoScannerMatchesTreeCoverage(t *testing.T) {
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "scan-internal.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatal(err)
	}
	for name, tc := range treeScanned {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "vector.txt"), []byte(tc.vector+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command("bash", script, "--dir", dir).CombinedOutput()
			failed := err != nil
			if failed != tc.scanned {
				t.Fatalf("scan-internal.sh over a %s vector: failed=%v, want %v\n%s", name, failed, tc.scanned, out)
			}
			if tc.scanned && !strings.Contains(string(out), "["+name+"]") {
				t.Errorf("hit was not labelled with the shape that matched (%q):\n%s", name, out)
			}
		})
	}
}

// pemExemptionsFile is the single home of the per-file exemption list for the
// private_key_pem tree scan. scripts/scan-internal.sh reads it to drop
// exempted hits and to fail stale entries; the tests below read the same
// file to enforce the discipline. Neither side keeps a copy — a second list
// is exactly the drift this package exists to prevent.
const pemExemptionsFile = "pem-exemptions.txt"

// readPEMExemptions parses pem-exemptions.txt into path → reason, rejecting
// entries that carry no reason. The rule is the NO_PALETTE_ROW precedent
// (web/src/lib/palette-coverage.test.ts): the exemption map's type is
// "exempted thing → why", which is what makes an undocumented exemption
// impossible to add. Here the parser is that type.
func readPEMExemptions(t *testing.T) map[string]string {
	t.Helper()
	body, err := os.ReadFile(pemExemptionsFile)
	if err != nil {
		t.Fatalf("%s unreadable: %v", pemExemptionsFile, err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, reason, ok := strings.Cut(line, "|")
		if !ok {
			t.Errorf("%s: entry %q has no '|' — an exemption is 'path|reason'", pemExemptionsFile, line)
			continue
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s: exemption for %q must say why — an undocumented exemption is how the list grows to everything", pemExemptionsFile, path)
			continue
		}
		if _, dup := out[path]; dup {
			t.Errorf("%s: duplicate entry for %q", pemExemptionsFile, path)
		}
		out[path] = reason
	}
	if len(out) == 0 {
		t.Errorf("%s holds no entries — either the paths went missing, or the last vector is gone and the exemption machinery should be retired with it", pemExemptionsFile)
	}
	return out
}

// TestPEMExemptionsCarryReasons is the reason half of the discipline; the
// parser above reports every violation it finds.
func TestPEMExemptionsCarryReasons(t *testing.T) {
	readPEMExemptions(t)
}

// TestPEMExemptionsAreNotStale is the deletion half: an entry whose file no
// longer matches the shape is a decision about a tree that is gone, and the
// entry must be removed. Staleness is judged with this package's own regexp
// — the canonical shape — so the ERE spelling in the script cannot hide it.
// The scanner enforces the same at scan time; this pins it where the shape
// is owned.
func TestPEMExemptionsAreNotStale(t *testing.T) {
	exempt := readPEMExemptions(t)
	var pemRe *regexp.Regexp
	for _, p := range patterns {
		if p.Name == "private_key_pem" {
			pemRe = p.Re
		}
	}
	if pemRe == nil {
		t.Fatal("private_key_pem missing from the package table")
	}
	for path := range exempt {
		body, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Errorf("%s: %q is not readable — delete the entry or fix the path: %v", pemExemptionsFile, path, err)
			continue
		}
		if !pemRe.MatchString(string(body)) {
			t.Errorf("%s: stale exemption — %q no longer carries a private-key header; delete the entry", pemExemptionsFile, path)
		}
	}
}

// TestPEMExemptionListIsTheScannersList pins the two readers to one home: if
// the script stops pointing at this file (a rename, or an inline copy grown
// beside it), the exemption list and the scanner stop being the same
// decision.
func TestPEMExemptionListIsTheScannersList(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "scripts", "scan-internal.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "internal/secretscan/"+pemExemptionsFile) {
		t.Errorf("scripts/scan-internal.sh no longer reads internal/secretscan/%s — the exemption list must have exactly one home, referenced by path", pemExemptionsFile)
	}
}
