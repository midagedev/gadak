package retro

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

/*
 * GDK-1690: the reopen surfaces are decided by the origin's capability, never
 * by the count.
 *
 * reopen_count, reopened_at and reopen_reason are derived from the changelog,
 * so on an origin that supplies none they stay 0 forever. The screen showed
 * "reopened 0" there, which is not the same sentence as "cannot be counted" —
 * on a Linear workspace it reads as "this team has no regressions".
 *
 * FAIL-first: against the pre-fix source `OriginSuppliesChangelog` did not
 * exist, Report carried no ReopenUnavailable, and the summary printed the
 * reopened clause on every origin.
 */
func TestOriginSuppliesChangelog(t *testing.T) {
	cases := []struct {
		origin string
		want   bool
	}{
		{config.OriginJira, true},
		{config.OriginJiraServer, true},
		{config.OriginGadak, true},
		{config.OriginLinear, false},
		// No config is the CLI's own default and every test's zero value: the
		// surfaces stay, which is the pre-GDK-1690 behaviour.
		{"", true},
	}
	for _, c := range cases {
		if got := OriginSuppliesChangelog(c.origin); got != c.want {
			t.Errorf("OriginSuppliesChangelog(%q) = %v, want %v", c.origin, got, c.want)
		}
	}
}

// TestOriginChangelogMatchesSupportMatrix keeps the code and the matrix from
// drifting. docs/SUPPORT_MATRIX.md is the single owner of the origin columns
// (CLAUDE.md), and the history row is the one this capability is: a ✅ means
// the origin feeds reopen_count, a ◐ or ✗ means it cannot. If that row is
// re-marked, this fails and OriginSuppliesChangelog is what has to move.
func TestOriginChangelogMatchesSupportMatrix(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "SUPPORT_MATRIX.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("support matrix absent: %v", err)
	}
	var row string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, "`reopen_count`") && strings.HasPrefix(strings.TrimSpace(line), "|") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("%s no longer has a history row naming reopen_count — update this citation", path)
	}
	cells := strings.Split(strings.Trim(strings.TrimSpace(row), "|"), "|")
	if len(cells) < 5 {
		t.Fatalf("history row has %d cells, want the label plus four origins: %q", len(cells), row)
	}
	// Column order is the table's header: Jira · Jira Server · Linear · Built-in.
	origins := []string{config.OriginJira, config.OriginJiraServer, config.OriginLinear, config.OriginGadak}
	footnote := regexp.MustCompile(`\[\^\d+\]`)
	for i, origin := range origins {
		mark := strings.TrimSpace(footnote.ReplaceAllString(cells[i+1], ""))
		want := mark == "✅"
		if got := OriginSuppliesChangelog(origin); got != want {
			t.Errorf("SUPPORT_MATRIX history cell for %s is %q (supplies=%v) but OriginSuppliesChangelog says %v",
				origin, mark, want, got)
		}
	}
}

// TestReopenUnavailableSilencesTheSummaryClause: the CLI sentence leaves out
// what it cannot say rather than printing a zero.
func TestReopenUnavailableSilencesTheSummaryClause(t *testing.T) {
	rep := matReport(t, baseFixture())
	if rep.ReopenUnavailable {
		t.Fatal("a report with no origin set must keep the reopen surfaces")
	}
	// Summary speaks about the last bucket, and the fixture's reopens are in
	// the first one, so that is the bucket this assertion needs.
	rep.Buckets = rep.Buckets[:1]
	with := rep.Summary()
	if !strings.Contains(with, "reopened ") {
		t.Fatalf("the fixture's summary has no reopened clause to silence: %q", with)
	}

	rep.ReopenUnavailable = true
	without := rep.Summary()
	if strings.Contains(without, "reopened ") {
		t.Errorf("summary still counts reopens on a changelog-less origin: %q", without)
	}
	// Only that clause goes: the rest of the sentence is unaffected.
	if !strings.Contains(without, "closed ") {
		t.Errorf("silencing the reopen clause took the rest of the sentence with it: %q", without)
	}

	// And the reason is said rather than left to be inferred from an absence.
	var note string
	for _, n := range rep.Notes() {
		if n[0] == "reopened" {
			note = n[1]
		}
	}
	if note == "" {
		t.Error("no note explains why the reopen rows are gone")
	}
	if !strings.Contains(note, "changelog") {
		t.Errorf("the note does not name the reason: %q", note)
	}
	// The wire carries it, because the web surface decides the same way.
	if !rep.JSON().ReopenUnavailable {
		t.Error("Doc.reopen_unavailable does not follow the report")
	}
}
