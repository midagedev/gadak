package main

// gadak sql — read-only SQL against the mirror.

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/sqlhint"
	"github.com/midagedev/gadak/internal/store"
)

// openReadOnly gives sql/status a connection that cannot write, so a typo'd
// UPDATE cannot corrupt the mirror while the server holds the single writer.
func openReadOnly() (*sql.DB, error) {
	if err := rejectUnknownProfile(); err != nil {
		return nil, err
	}
	path, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no mirror at %s — run `gadak sync` first", path)
	}
	// OpenReadOnly ATTACHes local.db (creates an empty one if the profile
	// predates history) so `SELECT … FROM local.visits` works without ATTACH.
	return store.OpenReadOnly(path)
}

func cmdSQL(args []string) error {
	// Flags are matched by name wherever they appear instead of with
	// flag.Parse, because a query legitimately starts with `--` — a SQL comment,
	// which flag.Parse reads as an undefined flag and refuses. That is exactly what
	// happens when an agent pastes a commented query out of docs/MIRROR.md.
	if wantsHelp(args) {
		printHelp("sql")
		return nil
	}
	var asJSON, asCSV, noHeader bool
	var words []string
	for i, a := range args {
		switch a {
		case "--json", "-json":
			asJSON = true
		case "--csv", "-csv":
			asCSV = true
		case "--no-header", "-no-header":
			noHeader = true
		default:
			// A flag never contains whitespace. A single argv that starts
			// with `--` and contains a space is a quoted query (SQL comment
			// plus the statement), not a flag candidate — flag.Parse cannot
			// tell those apart, which is why this command matches flags by name.
			if sqlFlagCandidate(a) && sqlQueryFollows(args, i+1) {
				return usageError("sql", fmt.Sprintf("unknown flag %s", a))
			}
			words = append(words, a)
		}
	}
	query := strings.TrimSpace(strings.Join(words, " "))
	if query == "" {
		return usageError("sql", `usage: gadak sql [--json|--csv] [--no-header] "select ..."`)
	}
	return runReadOnlySQL(query, sqlOutput{JSON: asJSON, CSV: asCSV, NoHeader: noHeader})
}

// sqlOutput is the stdout contract `gadak sql` and `gadak recipes run` share.
type sqlOutput struct {
	JSON     bool
	CSV      bool
	NoHeader bool
}

// runReadOnlySQL is the single owner of the sql/recipes-run path: openReadOnly
// (writes are refused by SQLite), warnIfStale, column-suggestion on error,
// and the --json/--csv/--no-header row format.
func runReadOnlySQL(query string, out sqlOutput) error {
	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)
	return writeSQLQuery(db, query, out)
}

func writeSQLQuery(db *sql.DB, query string, out sqlOutput) error {
	rows, err := db.Query(query)
	if err != nil {
		return sqlhint.WithColumnSuggestion(db, query, err)
	}
	defer rows.Close()
	n, err := formatSQLRows(rows, query, out)
	if err != nil {
		return err
	}
	if n == 0 {
		if hint := zeroRowScopeHint(db, "zero rows", query); hint != "" {
			fmt.Fprintln(os.Stderr, hint)
		}
	}
	return nil
}

// drainSQLQuery executes query and discards rows. Used by recipes save to
// refuse a statement that cannot run, without printing it. Zero rows is
// success — a stale mirror can be empty.
func drainSQLQuery(db *sql.DB, query string) error {
	rows, err := db.Query(query)
	if err != nil {
		return sqlhint.WithColumnSuggestion(db, query, err)
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err()
}

// formatSQLRows prints the row set in the requested format and returns how
// many rows it printed. The count is writeSQLQuery's input for the 0-row
// scope hint (GDK-1612): the same query text that printed nothing needs a
// verdict on whether "nothing" means "no match" or "not this workspace".
func formatSQLRows(rows *sql.Rows, query string, out sqlOutput) (int, error) {
	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	var csvOut *csv.Writer
	switch {
	case out.CSV:
		csvOut = csv.NewWriter(os.Stdout)
		if !out.NoHeader {
			if err := csvOut.Write(cols); err != nil {
				return 0, err
			}
		}
	case !out.JSON:
		if !out.NoHeader {
			fmt.Println(strings.Join(cols, "\t"))
		}
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	enc := json.NewEncoder(os.Stdout)
	n := 0
	for rows.Next() {
		n++
		if err := rows.Scan(ptrs...); err != nil {
			return n, err
		}
		if out.JSON {
			obj := make(map[string]any, len(cols))
			for i, c := range cols {
				obj[c] = cell(vals[i])
			}
			_ = enc.Encode(obj)
			continue
		}
		line := make([]string, len(cols))
		for i := range vals {
			line[i] = text(vals[i])
		}
		if csvOut != nil {
			if err := csvOut.Write(line); err != nil {
				return n, err
			}
			continue
		}
		fmt.Println(strings.Join(line, "\t"))
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	if hint := sqlhint.ZeroRowDisplayNameWarning(query, n); hint != "" {
		fmt.Fprintln(os.Stderr, hint)
	}
	if csvOut != nil {
		csvOut.Flush()
		return n, csvOut.Error()
	}
	return n, nil
}

// scopeQuerier is the one method zeroRowScopeHint needs. `gadak sql` reads
// through a raw *sql.DB; the issue and search paths hold a *store.DB — both
// satisfy this, so the scope verdict has a single owner (GDK-1612) instead
// of one near-copy per surface.
type scopeQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// GDK-1612: 0 rows has two causes that need opposite next moves — nothing
// matched, or this workspace never mirrors that project at all. The incident:
// `gadak --workspace work sql "… WHERE item_id='jira:124963'"` was read as a
// sync defect while that workspace holds a single other project. Two token
// shapes name a foreign issue in a query: issue keys (D1-8228) and item ids
// (jira:124963 — the id agents copy out of JSON payloads). When the mirror
// holds nothing under that token, zeroRowScopeHint returns the one line that
// names the workspace that was read and the scope it holds; every caller
// prints it on stderr, because stdout is the contract on all three surfaces
// (sql TSV/JSON, search TSV, issue text).
var (
	// Uppercase prefix only, like a tracker key; lowercase words (e2e-2, a-1)
	// and dates (2026-09-10, which starts with a digit) do not match.
	scopeIssueKeyRe = regexp.MustCompile(`\b([A-Z][A-Z0-9]*)-\d+\b`)
	// Item ids are `source:external`: the source kinds the mirror carries.
	scopeItemIDRe = regexp.MustCompile(`\b(?:jira|gadak|confluence|linear):[0-9A-Za-z.-]+\b`)
)

// scopeTokenCap and scopeProjectCap keep the hint one line when a query names
// many foreign issues or the workspace holds many projects.
const (
	scopeTokenCap   = 3
	scopeProjectCap = 5
)

// zeroRowScopeHint classifies a 0-row (or 0-match, or not-found) answer
// against the mirror's project scope. zeroWord is the surface's own phrase
// for the empty result ("zero rows", "0 matches"); the issue paths pass "" so
// the hint reads as the error itself. Empty return means "no scope verdict":
// the text names no issue, everything it names is inside the scope, or the
// scope read failed — never a guess.
func zeroRowScopeHint(q scopeQuerier, zeroWord, text string) string {
	keys := scopeIssueKeyRe.FindAllStringSubmatch(text, -1)
	ids := scopeItemIDRe.FindAllStringSubmatch(text, -1)
	if len(keys) == 0 && len(ids) == 0 {
		return "" // nothing names an issue: 0 rows is the query's business
	}
	held, heldList := scopeHeldProjects(q)
	if held == nil {
		return "" // scope unreadable: a read failure is not a scope verdict
	}
	var foreign []string
	seen := map[string]bool{}
	add := func(tok string) {
		if !seen[tok] {
			seen[tok] = true
			foreign = append(foreign, tok)
		}
	}
	for _, m := range keys {
		if !held[m[1]] {
			add(m[0])
		}
	}
	for _, m := range ids {
		if !scopeItemPresent(q, m[0]) {
			add(m[0])
		}
	}
	if len(foreign) == 0 {
		return "" // every token is in scope: the miss is a key/filter miss
	}
	verb := "is"
	if len(foreign) > 1 {
		verb = "are"
	}
	if len(foreign) > scopeTokenCap {
		foreign = append(foreign[:scopeTokenCap], "…")
	}
	heldText := "nothing yet — run gadak sync"
	if len(heldList) > 0 {
		heldText = strings.Join(heldList, ", ")
		if len(heldList) > scopeProjectCap {
			heldText = strings.Join(heldList[:scopeProjectCap], ", ") + ", …"
		}
	}
	profile := config.Profile()
	if profile == "" {
		profile = "default"
	}
	body := fmt.Sprintf("%s %s outside this workspace's mirror — workspace %q holds %s; another profile may hold it",
		strings.Join(foreign, ", "), verb, profile, heldText)
	if zeroWord != "" {
		return zeroWord + ": " + body
	}
	return body
}

// scopeHeldProjects reads the mirror's project scope — what this workspace
// actually mirrors, which is the fact the hint names (the config's project
// list can lag or disagree; the mirror is what answered the query). Nil map
// means the read failed.
func scopeHeldProjects(q scopeQuerier) (map[string]bool, []string) {
	rows, err := q.Query(`SELECT DISTINCT project_key FROM issues WHERE project_key != '' ORDER BY 1`)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	held := map[string]bool{}
	var list []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, nil
		}
		held[k] = true
		list = append(list, k)
	}
	if err := rows.Err(); err != nil {
		return nil, nil
	}
	return held, list
}

// scopeItemPresent asks the items table whether one item id is in the mirror.
// An unreadable answer counts as present: the hint must never call a scope
// miss on a query it could not run.
func scopeItemPresent(q scopeQuerier, id string) bool {
	rows, err := q.Query(`SELECT 1 FROM items WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return true
	}
	defer rows.Close()
	return rows.Next()
}

func sqlKnownFlag(a string) bool {
	switch a {
	case "--json", "-json", "--csv", "-csv", "--no-header", "-no-header":
		return true
	}
	return false
}

// sqlFlagCandidate reports whether a is an unknown `--…` token. Flags have
// no whitespace; a quoted argv that starts with `--` and contains a space is
// a SQL comment plus a statement, not a flag.
func sqlFlagCandidate(a string) bool {
	if !strings.HasPrefix(a, "--") || sqlKnownFlag(a) {
		return false
	}
	return !strings.ContainsAny(a, " \t\n\r")
}

// sqlQueryFollows reports a later argv that looks like SQL, not another flag.
// Used so `gadak sql --pretty` (lone token) stays a comment-only query, while
// `gadak sql --pretty "select …"` is a typo'd flag in front of a real query.
func sqlQueryFollows(args []string, from int) bool {
	for _, a := range args[from:] {
		if sqlKnownFlag(a) {
			continue
		}
		if sqlFlagCandidate(a) {
			continue
		}
		if strings.TrimSpace(a) != "" {
			return true
		}
	}
	return false
}

func cell(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// text renders a cell for the row-oriented outputs. NULL prints as empty rather
// than as Go's "<nil>", which no consumer of a tab or CSV row wants to parse.
func text(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(cell(v))
}
