package main

// gadak list — the default open-issues read: status_category != 'done',
// priority_rank first with updated_at desc breaking ties, 30 rows. Priority
// appears twice on purpose — the display name for reading, priority_rank so
// the sort can be checked against the output itself. age_days (days in the
// current status, computed from status_changed_at on issues_full — time in
// status is never a stored column) rides after status in every row, so an
// answer to "what next" can carry age as a fact about the issue
// (docs/project/THEORY.md T4/G10). `ready` (the --ready
// flag, or the top-level alias) narrows the same list to issues no open
// blocker holds back; the blocking link type resolves through the origin's
// link-type catalog, never a hardcoded 'Blocks' — link-type names are
// per-account like status names, and a renamed type would silently
// misfilter.

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

const (
	listUsage  = `usage: gadak list [--limit N] [--all] [--ready] [--json|--csv|--no-header]`
	readyUsage = `usage: gadak ready [--limit N] [--json|--csv|--no-header]`
	// ageDaysColumn is the age signal every open-issues read carries: days
	// in the current status, rounded to one decimal, NULL (an empty TSV
	// cell) when the mirror has no status_changed_at. Computed from
	// status_changed_at on issues_full — never stored. Single owner of the
	// expression: listColumns and nextRecipeSQL (recipes.go) both build on
	// it, the same idiom docs/MIRROR.md and internal/mcp/tools.go teach.
	ageDaysColumn = `round(julianday('now') - julianday(status_changed_at), 1) as age_days`

	defaultListLimit = 30
	listColumns      = "key, priority, priority_rank, status, " + ageDaysColumn + ", updated_at, summary"
)

func cmdList(args []string) error  { return runList("list", args) }
func cmdReady(args []string) error { return runList("ready", args) }

func runList(name string, args []string) error {
	fs := newFlagSet(name)
	limit := fs.Int("limit", defaultListLimit, "maximum rows to list")
	asJSON := fs.Bool("json", false, "emit one JSON object per row")
	asCSV := fs.Bool("csv", false, "emit CSV with a header row")
	noHeader := fs.Bool("no-header", false, "omit the TSV/CSV header row (no-op with --json)")
	all := fs.Bool("all", false, "include done issues (default hides them)")
	var ready *bool
	if name != "ready" {
		ready = fs.Bool("ready", false, "only issues no open blocker holds back (one origin link-type read)")
	}
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp(name, fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usageError(name, listUsageLine(name))
	}
	wantReady := name == "ready" || (ready != nil && *ready)
	if wantReady && *all {
		return usageError(name, "--ready and --all cannot be combined: ready is the open list minus blocked issues")
	}
	if *limit <= 0 {
		return usageError(name, "--limit must be 1 or more")
	}
	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)
	blocker := ""
	if wantReady {
		blocker = readyBlockerFilter()
	}
	return writeSQLQuery(db, listSQL(*limit, *all, blocker), sqlOutput{JSON: *asJSON, CSV: *asCSV, NoHeader: *noHeader})
}

func listUsageLine(name string) string {
	if name == "ready" {
		return readyUsage
	}
	return listUsage
}

// listSQL is the single owner of the built-in open-issues query. blocker,
// when non-empty, is ready's NOT EXISTS clause (" and not exists (…)") and
// only ever arrives with all=false — runList rejects --ready --all first.
// listDefaultSQL (open, no blocker) is what next/pick fall back to when no
// recipe is saved — one query, one column list, one order, so the verbs
// cannot drift apart.
func listSQL(limit int, all bool, blocker string) string {
	q := "select " + listColumns + " from issues_full f"
	if !all {
		q += " where status_category != 'done'" + blocker
	}
	return fmt.Sprintf("%s order by priority_rank, updated_at desc limit %d", q, limit)
}

func listDefaultSQL(limit int) string { return listSQL(limit, false, "") }

// readyBlockerFilter resolves the blocking link type the way `gadak link
// --type blocks` does — against the origin's link-type catalog
// (origin.ResolveLinkType), because the mirror stores link types by name
// only. When no catalog can answer (no credential, offline, Linear, or no
// blocks-like type in the account), ready degrades on purpose: one stderr
// line, then the plain open list. Returning empty instead would read as
// "nothing is ready" — a stronger and wronger claim than "blockers not
// filtered"; silently dropping the filter would make ready a hidden alias
// of list. The announcement keeps the degradation honest.
func readyBlockerFilter() string {
	warn := func(err error) {
		fmt.Fprintf(os.Stderr, "ready: blocking link type unresolved (%v) — blockers not filtered, showing all open issues\n", err)
	}
	cfg, err := config.Load()
	if err != nil {
		warn(err)
		return ""
	}
	c, err := origin.Client(cfg)
	if err != nil {
		warn(err)
		return ""
	}
	catalog, err := c.IssueLinkTypes(context.Background())
	if err != nil {
		warn(err)
		return ""
	}
	lt, _, err := origin.ResolveLinkType("Blocks", catalog)
	if err != nil {
		warn(err)
		return ""
	}
	return fmt.Sprintf(
		" and not exists (select 1 from links l join issues_full b on b.key = l.target_key"+
			" where l.item_id = f.item_id and l.type = %s and l.direction = 'inward'"+
			" and b.status_category != 'done')",
		sqlLiteral(lt.Name),
	)
}

// sqlLiteral quotes s as a SQLite string literal. The value comes from the
// origin catalog rather than the user, but a renamed link type can still
// carry an apostrophe.
func sqlLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
