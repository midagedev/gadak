package main

// gadak sql — read-only SQL against the mirror.

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
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
	// happens when an agent pastes a commented query out of AGENTS.md.
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
		return sqlhint.WithColumnSuggestion(db, err)
	}
	defer rows.Close()
	return formatSQLRows(rows, query, out)
}

// drainSQLQuery executes query and discards rows. Used by recipes save to
// refuse a statement that cannot run, without printing it. Zero rows is
// success — a stale mirror can be empty.
func drainSQLQuery(db *sql.DB, query string) error {
	rows, err := db.Query(query)
	if err != nil {
		return sqlhint.WithColumnSuggestion(db, err)
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err()
}

func formatSQLRows(rows *sql.Rows, query string, out sqlOutput) error {
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	var csvOut *csv.Writer
	switch {
	case out.CSV:
		csvOut = csv.NewWriter(os.Stdout)
		if !out.NoHeader {
			if err := csvOut.Write(cols); err != nil {
				return err
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
			return err
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
				return err
			}
			continue
		}
		fmt.Println(strings.Join(line, "\t"))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hint := sqlhint.ZeroRowDisplayNameWarning(query, n); hint != "" {
		fmt.Fprintln(os.Stderr, hint)
	}
	if csvOut != nil {
		csvOut.Flush()
		return csvOut.Error()
	}
	return nil
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
