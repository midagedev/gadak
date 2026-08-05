package main

import (
	"flag"
	"strings"
	"testing"
)

// TestHelpCoversAllCommands keeps the help table and the canonical command list
// in lockstep: add a command without help (or leave a stale help entry) and CI
// fails. main's switch must dispatch every name in commandNames — see the
// comment on commandNames and the switch cases in main.
func TestHelpCoversAllCommands(t *testing.T) {
	if len(commandNames) == 0 {
		t.Fatal("commandNames is empty")
	}
	seen := make(map[string]bool, len(commandNames))
	for _, name := range commandNames {
		if seen[name] {
			t.Errorf("commandNames: duplicate %q", name)
		}
		seen[name] = true
		if _, ok := helps[name]; !ok {
			t.Errorf("command %q has no helps entry", name)
		}
	}
	for name := range helps {
		if !seen[name] {
			t.Errorf("helps has %q but it is not in commandNames", name)
		}
	}
	if len(helps) != len(commandNames) {
		t.Errorf("helps has %d entries, commandNames has %d", len(helps), len(commandNames))
	}
}

// TestRenderHelpShape checks the documented layout on a representative
// FlagSet-backed command (sync).
func TestRenderHelpShape(t *testing.T) {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.Bool("full", false, "force a full sync")
	fs.Bool("watch", false, "keep syncing on an interval")
	out := formatHelp("sync", fs)

	if !strings.HasPrefix(out, "scry sync — ") {
		t.Errorf("first line: want prefix %q, got %q", "scry sync — ", firstLine(out))
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
	if !strings.HasPrefix(out, "scry sql — ") {
		t.Errorf("first line: want prefix %q, got %q", "scry sql — ", firstLine(out))
	}
	if !strings.Contains(out, "--json") || !strings.Contains(out, "--csv") {
		t.Errorf("sql Options missing --json/--csv:\n%s", out)
	}
	if strings.Contains(out, "Usage of ") {
		t.Errorf("default flag header in sql help:\n%s", out)
	}
}

// TestUsageErrorPointsAtHelp keeps the original usage line and adds a pointer.
func TestUsageErrorPointsAtHelp(t *testing.T) {
	err := usageError("issue", "usage: scry issue <KEY> [--json]")
	s := err.Error()
	if !strings.HasPrefix(s, "usage: scry issue <KEY> [--json]") {
		t.Errorf("first line changed: %q", s)
	}
	if !strings.Contains(s, `run "scry issue --help" for examples`) {
		t.Errorf("missing help pointer: %q", s)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
