package main

// gadak list — the default open-issues read: status_category != 'done',
// priority_rank first with updated_at desc breaking ties, 30 rows. Priority
// appears twice on purpose — the display name for reading, priority_rank so
// the sort can be checked against the output itself. age_days (days in the
// current status, computed from status_changed_at on issues_full — time in
// status is never a stored column) rides after status in every row, so an
// answer to "what next" can carry age as a fact about the issue
// (docs/project/THEORY.md T4/G10). `ready` (the --ready flag, or the
// top-level alias) narrows the same list to issues no open blocker holds
// back: the open_blockers column (v43), recomputed by the store whenever
// links or target statuses move — a pure mirror read, no origin call.

import (
	"database/sql"
	"fmt"
	"os"
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
		ready = fs.Bool("ready", false, "only issues no recorded open blocker holds back (block links are typed far less consistently than epic links)")
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
		blocker = readyBlockerFilter(db)
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

// readyBlockerFilter is ready's whole filter in one clause: the
// open_blockers column (v43), which the store recomputes whenever links or
// target statuses move (internal/store/flow.go) and which resolves the
// blocking link types through the mirror's own link_types catalog — never a
// hardcoded display name. Before v43 this verb did a live origin read per
// invocation (one GET /issueLinkType per `ready`) to resolve the type name;
// the column made that path dead and it is gone.
//
// The one case the column cannot answer is a mirror whose schema predates
// v43 — a mirror no post-upgrade sync has migrated, so the column is not
// just stale but absent. That keeps the old degradation posture: one stderr
// line, then the plain open list, because an empty "nothing ready" would be
// a stronger and wronger claim than "blockers not filtered", and silently
// dropping the filter would make ready a hidden alias of list. The next
// `gadak sync` migrates the mirror and the notice disappears.
func readyBlockerFilter(db *sql.DB) string {
	var have int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&have); err != nil || have < openBlockersVersion {
		fmt.Fprintln(os.Stderr, "ready: mirror schema predates open_blockers — run `gadak sync` to migrate; blockers not filtered, showing all open issues")
		return ""
	}
	return " and f.open_blockers = 0"
}

// openBlockersVersion is the migration that added issues_raw.open_blockers.
const openBlockersVersion = 43
