package main

import (
	"flag"
	"regexp"
	"strings"
	"testing"
)

// TestHelpCoversAllCommands keeps help prose, the commands registry, and
// dispatch in lockstep: add a command without help (or leave a stale help
// entry, or forget a run func) and CI fails. Every name in commands must have
// a non-nil run func so main's map lookup actually dispatches.
func TestHelpCoversAllCommands(t *testing.T) {
	names := commandNames()
	if len(names) == 0 {
		t.Fatal("commandNames is empty")
	}
	if len(names) != len(commands) {
		t.Errorf("commandNames has %d entries, commands has %d", len(names), len(commands))
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			t.Errorf("commandNames: duplicate %q", name)
		}
		seen[name] = true
		run, ok := commands[name]
		if !ok {
			t.Errorf("command %q is in commandNames but not in commands", name)
			continue
		}
		if run == nil {
			t.Errorf("command %q has nil run func — will not dispatch", name)
		}
		if _, ok := helps[name]; !ok {
			t.Errorf("command %q has no helps entry", name)
		}
	}
	for name := range helps {
		if !seen[name] {
			t.Errorf("helps has %q but it is not in commands", name)
		}
	}
	for name, run := range commands {
		if !seen[name] {
			t.Errorf("commands has %q but it is not in commandNames()", name)
		}
		if run == nil {
			t.Errorf("commands[%q] is nil", name)
		}
		if _, ok := helps[name]; !ok {
			t.Errorf("commands has %q but no helps entry", name)
		}
	}
	if len(helps) != len(commands) {
		t.Errorf("helps has %d entries, commands has %d", len(helps), len(commands))
	}
}

// TestRenderHelpShape checks the documented layout on a representative
// FlagSet-backed command (sync).
func TestRenderHelpShape(t *testing.T) {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.Bool("full", false, "force a full sync")
	fs.Bool("watch", false, "keep syncing on an interval")
	out := formatHelp("sync", fs)

	if !strings.HasPrefix(out, "gadak sync — ") {
		t.Errorf("first line: want prefix %q, got %q", "gadak sync — ", firstLine(out))
	}
	for _, want := range []string{"Usage:", "Options:", "Examples:", "See also:"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "Usage of ") {
		t.Errorf("default flag package header leaked into help:\n%s", out)
	}
	if !strings.Contains(out, "--full") || !strings.Contains(out, "--watch") {
		t.Errorf("Options must use double-dash flag names:\n%s", out)
	}
	// Single-dash flag listing from flag.PrintDefaults must not appear as the
	// primary form (a bare "-full" column would mean we regressed to defaults).
	for _, line := range strings.Split(out, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "-full") || strings.HasPrefix(trim, "-watch") {
			t.Errorf("Options line uses single-dash form: %q", line)
		}
	}
}

// TestRenderHelpManualCommand covers a non-FlagSet command (sql).
func TestRenderHelpManualCommand(t *testing.T) {
	out := formatHelp("sql", nil)
	if !strings.HasPrefix(out, "gadak sql — ") {
		t.Errorf("first line: want prefix %q, got %q", "gadak sql — ", firstLine(out))
	}
	if !strings.Contains(out, "--json") || !strings.Contains(out, "--csv") {
		t.Errorf("sql Options missing --json/--csv:\n%s", out)
	}
	if !strings.Contains(out, "--no-header") {
		t.Errorf("sql Options missing --no-header:\n%s", out)
	}
	if strings.Contains(out, "Usage of ") {
		t.Errorf("default flag header in sql help:\n%s", out)
	}
}

// TestUsageErrorPointsAtHelp keeps the original usage line and adds a pointer.
func TestUsageErrorPointsAtHelp(t *testing.T) {
	err := usageError("issue", "usage: gadak issue <KEY> [--json]")
	s := err.Error()
	if !strings.HasPrefix(s, "usage: gadak issue <KEY> [--json]") {
		t.Errorf("first line changed: %q", s)
	}
	if !strings.Contains(s, `run "gadak issue --help" for examples`) {
		t.Errorf("missing help pointer: %q", s)
	}
}

func TestParseAroundKeepsTrailingFlags(t *testing.T) {
	fs := newFlagSet("search")
	jql := fs.Bool("jql", false, "")
	asJSON := fs.Bool("json", false, "")
	limit := fs.Int("limit", 20, "")
	pos, err := parseAround(fs, []string{
		"--jql", `project = NMA AND statusCategory = "In Progress"`, "--json", "--limit", "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !*jql || !*asJSON || *limit != 3 {
		t.Fatalf("flags jql=%v json=%v limit=%d", *jql, *asJSON, *limit)
	}
	if got := strings.Join(pos, " "); got != `project = NMA AND statusCategory = "In Progress"` {
		t.Fatalf("query %q", got)
	}
}

func TestInitHelpSpacesAreGlobalOnly(t *testing.T) {
	// formatHelp(nil) is the `gadak help init` path (helps["init"].options).
	// confluence.go only syncs type=="global" when Spaces is empty.
	out := formatHelp("init", nil)
	if !strings.Contains(out, "global") {
		t.Errorf("init help must say global spaces, got:\n%s", out)
	}
	if strings.Contains(out, "every space you can see") {
		t.Errorf("init help still claims every visible space:\n%s", out)
	}
	// "all (every space)" was the dishonest form; "every global space" is ok.
	if strings.Contains(out, "all (every space)") {
		t.Errorf("init help still says all = every space:\n%s", out)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TestHelpDropsIssueNumbersAndLinearLead is GDK-469: pairing --help must not
// ship an issue key, and sync's first line must not advertise Linear to a
// user who has not configured it.
func TestHelpDropsIssueNumbersAndLinearLead(t *testing.T) {
	pairing := formatHelp("pairing", nil)
	if strings.Contains(pairing, "GDK-") {
		t.Errorf("pairing --help still names an issue:\n%s", pairing)
	}
	if strings.Contains(usage, "GDK-") {
		t.Errorf("top-level usage still names an issue:\n%s", usage)
	}
	syncFirst := firstLine(formatHelp("sync", nil))
	if strings.Contains(strings.ToLower(syncFirst), "linear") {
		t.Errorf("sync --help first line still names Linear: %q", syncFirst)
	}
}

// TestUsageListsEveryCommand pins the top-level usage const to the command
// table: a new verb that ships without a line there is invisible to
// `gadak help` (GDK-426 — page and project were both missing).
func TestUsageListsEveryCommand(t *testing.T) {
	for _, name := range commandNames() {
		if name == "view" {
			// alias of views; usage names it as "(alias: view)"
			continue
		}
		re := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(name) + `\b`)
		if !re.MatchString(usage) {
			t.Errorf("usage is missing a line for %q", name)
		}
	}
}
