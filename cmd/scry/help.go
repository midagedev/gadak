package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// commandNames is the canonical list of subcommands main dispatches.
// helps has one entry per name; tests assert the two sets stay equal so a
// new command without help (or a leftover help for a removed command) fails CI.
var commandNames = []string{
	"api",
	"assign",
	"comment",
	"demo",
	"doctor",
	"export-static",
	"fields",
	"init",
	"install-cli",
	"install-service",
	"issue",
	"mcp",
	"open",
	"profiles",
	"search",
	"serve",
	"snapshot",
	"sql",
	"status",
	"sync",
	"team",
	"transition",
	"tui",
	"version",
}

// cmdHelp is the prose around a command. Flag names and descriptions for
// flag.FlagSet commands are taken from the FlagSet via VisitAll at render
// time so they cannot drift from the registration site. options is only
// filled for commands that parse flags by hand.
type cmdHelp struct {
	summary  string
	usage    string
	options  []helpOption // manual Options lines; ignored when a FlagSet is given
	examples []string
	seeAlso  []string
}

type helpOption struct {
	name string // without leading dashes, e.g. "full" or "json"
	desc string
}

// helps is the per-command help table. Summaries recycle the top-level usage
// constant; positionals and examples match the real cmdXxx implementations.
var helps = map[string]cmdHelp{
	"init": {
		summary: "configure site and credentials (projects optional)",
		usage:   "scry [--profile <name>] init [--site URL] [--email ADDR] [--projects A,B] [--token-file PATH | --token-stdin] [--json]",
		// FlagSet VisitAll supplies Options when `scry init --help` runs; this
		// list covers formatHelp(nil) and documents the env-only token path.
		options: []helpOption{
			{name: "site", desc: "Jira site URL (https://your-site.atlassian.net); env SCRY_SITE"},
			{name: "email", desc: "account email; env SCRY_EMAIL"},
			{name: "projects", desc: "project keys, comma-separated (optional — blank syncs every project you can see); env SCRY_PROJECTS"},
			{name: "token-file", desc: "read API token from this file"},
			{name: "token-stdin", desc: "read API token from stdin"},
			{name: "token", desc: "not accepted; use SCRY_TOKEN, --token-file, or --token-stdin"},
			{name: "json", desc: "emit one JSON object on success"},
		},
		examples: []string{
			"scry init",
			"scry --profile demo init",
			"SCRY_TOKEN=$(cat token) scry init --site https://x.atlassian.net --email you@example.com --json",
			"scry init --site https://x.atlassian.net --email you@example.com --projects ABC --token-file ./token",
		},
		seeAlso: []string{"scry sync", "scry profiles"},
	},
	"sync": {
		summary: "mirror Jira into the local SQLite database",
		usage:   "scry [--profile <name>] sync [--full] [--watch]",
		examples: []string{
			"scry sync                 # incremental, what a serve loop does",
			"scry sync --full          # after changing projects or a mapping",
			"scry --profile demo sync  # against another site",
		},
		seeAlso: []string{"scry serve", "scry status"},
	},
	"serve": {
		summary: "web UI and API on loopback (syncs by default when a credential is configured)",
		usage:   "scry [--profile <name>] serve [options]",
		examples: []string{
			"scry serve",
			"scry serve --addr 127.0.0.1:7778 --no-open",
			"scry --profile demo serve --no-sync",
		},
		seeAlso: []string{"scry sync", "scry demo", "scry install-service"},
	},
	"install-service": {
		summary: "keep serve running across reboots (launchd / systemd user)",
		usage:   "scry [--profile <name>] install-service [--uninstall]",
		examples: []string{
			"scry install-service",
			"scry install-service --uninstall",
			"scry --profile work install-service",
		},
		seeAlso: []string{"scry serve"},
	},
	"install-cli": {
		summary: "put this binary on PATH via a symlink (default ~/.local/bin)",
		usage:   "scry install-cli [--dir <path>] [--force] [--print]",
		examples: []string{
			"scry install-cli",
			"/Applications/Scry.app/Contents/Resources/bin/scry install-cli",
			"scry install-cli --dir /usr/local/bin",
			"scry install-cli --print",
			"scry install-cli --force",
		},
		seeAlso: []string{"scry mcp install claude", "scry version"},
	},
	"status": {
		summary: "print sync state and row counts",
		usage:   "scry [--profile <name>] status [--json]",
		examples: []string{
			"scry status",
			"scry status --json",
			"scry --profile demo status",
		},
		seeAlso: []string{"scry sync", "scry sql", "scry doctor"},
	},
	"doctor": {
		summary: "print redacted diagnostics safe to paste into a bug report",
		usage:   "scry [--profile <name>] doctor [--json]",
		examples: []string{
			"scry doctor",
			"scry doctor --json",
			"scry --profile demo doctor",
		},
		seeAlso: []string{"scry status", "scry version"},
	},
	"demo": {
		summary: "serve the bundled snapshot; no Jira account needed",
		usage:   "scry demo [options]",
		examples: []string{
			"scry demo",
			"scry demo --addr 127.0.0.1:7879 --no-open",
			"scry demo --db examples/demo.db",
		},
		seeAlso: []string{"scry serve", "scry snapshot"},
	},
	"export-static": {
		summary: "freeze a snapshot database into static JSON for a hosted demo",
		usage:   "scry export-static [options] <outdir>",
		examples: []string{
			"scry export-static dist/static-demo",
			"scry export-static --db examples/demo.db out/",
			"scry export-static --api-base /scry/api/v1/issues/ dist/demo",
		},
		seeAlso: []string{"scry demo", "scry snapshot"},
	},
	"tui": {
		summary: "terminal issue navigator against the local mirror",
		usage:   "scry [--profile <name>] tui",
		examples: []string{
			"scry tui",
			"scry --profile demo tui",
		},
		seeAlso: []string{"scry serve", "scry issue"},
	},
	"profiles": {
		summary: "list configured profiles",
		usage:   "scry profiles",
		examples: []string{
			"scry profiles",
		},
		seeAlso: []string{"scry init"},
	},
	"sql": {
		summary: "run a read-only SQL query against the local mirror",
		usage:   "scry [--profile <name>] sql [--json|--csv] \"select ...\"",
		options: []helpOption{
			{name: "json", desc: "emit one JSON object per row"},
			{name: "csv", desc: "emit CSV with a header row"},
		},
		examples: []string{
			"scry sql \"select count(*) from issues\"",
			"scry sql --json \"select key, status from issues_full limit 5\"",
			"scry sql --csv \"select key from issues where status_category = 'done'\"",
		},
		seeAlso: []string{"scry issue", "scry search", "scry status"},
	},
	"api": {
		summary: "call Atlassian REST with the stored credential (escape hatch for endpoints the mirror does not cover)",
		usage:   "scry [--profile <name>] api [METHOD] <PATH> [--query k=v]... [--data <val|@file|->] [--write] [--status]",
		examples: []string{
			"scry api /rest/api/3/myself",
			"scry api GET /rest/api/3/issue/ABC-1/watchers",
			"scry api GET /wiki/api/v2/spaces --query limit=5",
			"scry api POST /rest/api/3/issue/ABC-1/worklog --data @wl.json --write",
		},
		seeAlso: []string{"scry issue", "scry comment", "scry fields"},
	},
	"issue": {
		summary: "print full detail for one issue from the local mirror",
		usage:   "scry [--profile <name>] issue <KEY> [--json]",
		examples: []string{
			"scry issue NMB-140",
			"scry issue NMB-140 --json",
		},
		seeAlso: []string{"scry search", "scry open", "scry sql"},
	},
	"open": {
		summary: "open the issue on your Jira site in the browser",
		usage:   "scry [--profile <name>] open <KEY>",
		examples: []string{
			"scry open NMB-140",
		},
		seeAlso: []string{"scry issue"},
	},
	"search": {
		summary: "full-text search of titles, bodies, and comments in the mirror",
		usage:   "scry [--profile <name>] search [--limit N] [--json] \"text\"",
		examples: []string{
			"scry search \"flaky upload\" --limit 5",
			"scry search \"idempotency\" --json",
		},
		seeAlso: []string{"scry issue", "scry sql"},
	},
	"comment": {
		summary: "add a comment on Jira (needs a credential; write-through to the mirror)",
		usage:   "scry [--profile <name>] comment <KEY> -m <text|-> [--json]",
		examples: []string{
			"scry comment NMB-140 -m \"Reproduced on staging.\"",
			"scry comment NMB-140 -m -          # body from stdin",
			"scry comment NMB-140 -m \"done\" --json",
		},
		seeAlso: []string{"scry transition", "scry assign", "scry issue"},
	},
	"transition": {
		summary: "change issue status on Jira (needs a credential; accepts transition id, name, or target status name)",
		usage:   "scry [--profile <name>] transition <KEY> <status-or-id> [--json]",
		examples: []string{
			"scry transition NMB-140 \"In Review\"",
			"scry transition NMB-140 31",
			"scry transition NMB-140 Done --json",
		},
		seeAlso: []string{"scry comment", "scry assign", "scry issue"},
	},
	"assign": {
		summary: "set the assignee on Jira (needs a credential; pass - to unassign)",
		usage:   "scry [--profile <name>] assign <KEY> <email|-> [--json]",
		examples: []string{
			"scry assign NMB-140 dana@example.com",
			"scry assign NMB-140 -                 # unassign",
			"scry assign NMB-140 dana@example.com --json",
		},
		seeAlso: []string{"scry comment", "scry transition", "scry issue"},
	},
	"fields": {
		summary: "report which custom fields are populated (samples the mirror; queries Jira)",
		usage:   "scry [--profile <name>] fields [--sample N] [--json] [--all] [--project KEY] [--apply]",
		options: []helpOption{
			{name: "sample", desc: "number of mirrored issues to sample (default 200)"},
			{name: "json", desc: "emit JSON"},
			{name: "all", desc: "include system fields (default: custom only)"},
			{name: "project", desc: "limit the sample to one project key"},
			{name: "apply", desc: "discover in-use custom fields from the mirror, save specs, and backfill (no re-download)"},
		},
		examples: []string{
			"scry fields",
			"scry fields --sample 100 --project NMB",
			"scry fields --all --json",
			"scry fields --apply",
		},
		seeAlso: []string{"scry status", "scry sync", "scry sql"},
	},
	"mcp": {
		summary: "MCP server on stdio for clients without a shell; install pins the profile",
		usage:   "scry [--profile <name>] mcp [install <client>]",
		examples: []string{
			"scry mcp",
			"scry --profile demo mcp",
			"scry mcp install claude",
			"scry --profile demo mcp install claude --dry-run",
			"scry mcp install json",
		},
		seeAlso: []string{"scry sql", "scry issue", "scry status", "scry profiles"},
	},
	"snapshot": {
		summary: "write a shareable copy of the mirror (no personal tables, no credentials)",
		usage:   "scry [--profile <name>] snapshot <out.db> [options]",
		options: []helpOption{
			{name: "from", desc: "source database path (default: this profile's mirror)"},
			{name: "spread", desc: "restate timestamps across this window, keeping every issue's own order (e.g. 90d)"},
			{name: "scale", desc: "clone issues onto new keys until the snapshot holds this many"},
			{name: "seed", desc: "reserved for --scale determinism (default 1)"},
			{name: "now", desc: "pin the clock to an RFC3339 timestamp for reproducible builds"},
			{name: "force", desc: "overwrite out.db if it already exists"},
		},
		examples: []string{
			"scry snapshot out.db",
			"scry snapshot out.db --spread 90d --force        # spread a seeded set over 3 months",
			"scry snapshot bench.db --scale 10000             # benchmark fixture, no 10k-issue site",
		},
		seeAlso: []string{"scry demo", "scry export-static", "scry sync"},
	},
	"team": {
		summary: "export or import shareable team settings and saved views (no credentials)",
		usage:   "scry [--profile <name>] team export|import …",
		options: []helpOption{
			{name: "out", desc: "export: write to this file instead of stdout"},
			{name: "with-members", desc: "export: include members (emails)"},
			{name: "dry-run", desc: "import: print the merge plan without writing"},
			{name: "overwrite", desc: "import: replace conflicting settings and same-named views"},
		},
		examples: []string{
			"scry team export --out scry-team.json",
			"scry team import scry-team.json --dry-run",
			"scry team import scry-team.json",
			"scry team import scry-team.json --overwrite",
		},
		seeAlso: []string{"scry init", "scry status"},
	},
	"version": {
		summary: "print the scry version",
		usage:   "scry version",
		examples: []string{
			"scry version",
		},
	},
}

// newFlagSet builds a FlagSet whose -h/--help prints formatHelp to stdout and
// exits 0 (flag.ExitOnError). Unknown flags still exit 2 after the same help.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.Usage = func() {
		fmt.Print(formatHelp(name, fs))
	}
	return fs
}

// wantsHelp reports whether args contain -h or --help (for commands that do
// not use flag.FlagSet).
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// usageError keeps the existing one-line usage text and points at --help for
// examples. The first line is intentionally unchanged for callers that match it.
func usageError(cmd, line string) error {
	return fmt.Errorf("%s\nrun \"scry %s --help\" for examples", line, cmd)
}

// formatHelp renders the help text for name. When fs is non-nil, Options come
// from VisitAll (double-dash names). When fs is nil, Options come from the
// manual options slice on the help entry.
func formatHelp(name string, fs *flag.FlagSet) string {
	h, ok := helps[name]
	if !ok {
		return fmt.Sprintf("scry: no help for %q\n", name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "scry %s — %s\n\n", name, h.summary)
	fmt.Fprintf(&b, "Usage:\n  %s\n", h.usage)

	optLines := optionLines(fs, h.options)
	if len(optLines) > 0 {
		b.WriteString("\nOptions:\n")
		for _, line := range optLines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	if len(h.examples) > 0 {
		b.WriteString("\nExamples:\n")
		for _, ex := range h.examples {
			fmt.Fprintf(&b, "  %s\n", ex)
		}
	}

	if len(h.seeAlso) > 0 {
		fmt.Fprintf(&b, "\nSee also: %s\n", strings.Join(h.seeAlso, ", "))
	}
	return b.String()
}

// dashed renders a flag the way it is actually written. Go's flag package
// accepts one or two dashes for any name, but a single-letter flag reads as
// -m everywhere in this project's docs, and "--m" looks like a typo.
func dashed(name string) string {
	if len(name) == 1 {
		return "-" + name
	}
	return "--" + name
}

func optionLines(fs *flag.FlagSet, manual []helpOption) []string {
	type item struct {
		name, desc, def string
	}
	var items []item
	if fs != nil {
		fs.VisitAll(func(f *flag.Flag) {
			items = append(items, item{
				name: dashed(f.Name),
				desc: f.Usage,
				def:  f.DefValue,
			})
		})
	} else {
		for _, o := range manual {
			items = append(items, item{name: dashed(o.name), desc: o.desc})
		}
	}
	if len(items) == 0 {
		return nil
	}
	width := 0
	for _, it := range items {
		if n := len(it.name); n > width {
			width = n
		}
	}
	lines := make([]string, 0, len(items))
	for _, it := range items {
		desc := it.desc
		if showDefault(it.def) {
			desc = fmt.Sprintf("%s (default %s)", desc, it.def)
		}
		lines = append(lines, fmt.Sprintf("  %-*s  %s", width, it.name, desc))
	}
	return lines
}

// showDefault omits zero-ish defaults that flag.PrintDefaults also hides.
func showDefault(def string) bool {
	switch def {
	case "", "false", "0":
		return false
	default:
		return true
	}
}

// printHelp writes formatHelp to stdout. Used by non-FlagSet commands.
func printHelp(name string) {
	fmt.Fprint(os.Stdout, formatHelp(name, nil))
}
