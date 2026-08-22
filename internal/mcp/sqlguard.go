package mcp

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/sqlhint"
)

// Row and response caps protect agent context windows. limit defaults and hard
// ceilings are part of the agent contract (contracts/agent.md).
const (
	defaultRowLimit = 200
	hardRowLimit    = 1000
	maxResultBytes  = 256 * 1024
)

// clampLimit applies the default (200) and hard ceiling (1000).
func clampLimit(n int) int {
	if n <= 0 {
		return defaultRowLimit
	}
	if n > hardRowLimit {
		return hardRowLimit
	}
	return n
}

// rejectNonSelect refuses anything that is not a single SELECT or WITH
// statement. mode=ro already blocks writes at the connection, but PRAGMA,
// ATTACH, and multi-statement payloads still need an explicit check so the
// agent gets a clear error instead of a surprising empty result.
func rejectNonSelect(query string) error {
	s := sqlhint.StripComments(query)
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("empty SQL")
	}
	// Allow one trailing semicolon; anything else is multi-statement.
	body := strings.TrimRight(s, " \t\n\r;")
	if strings.Contains(body, ";") {
		return errors.New("multiple statements are not allowed; send one SELECT or WITH")
	}
	kw := sqlhint.FirstKeyword(body)
	switch strings.ToUpper(kw) {
	case "SELECT", "WITH":
		return nil
	case "":
		return errors.New("empty SQL")
	default:
		return fmt.Errorf("only SELECT or WITH statements are allowed (got %q)", kw)
	}
}

// queryResult is the JSON shape returned by gadak_query.
type queryResult struct {
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	Count     int              `json:"count"`
	Truncated bool             `json:"truncated,omitempty"`
	// TruncationReason is set when rows or bytes were cut; agents re-query with
	// a tighter LIMIT or fewer columns when they see it.
	TruncationReason string `json:"truncation_reason,omitempty"`
	RowLimit         int    `json:"row_limit"`
	// Warning is set only when the query returned zero rows while comparing a
	// display-name column: Jira localizes status/priority/type names per
	// account, so the empty result is usually the locale trap, not "nothing
	// matches". omitempty keeps the field absent everywhere else.
	Warning string `json:"warning,omitempty"`
}

// runQuery opens the mirror read-only, enforces SELECT/WITH, and caps rows.
func runQuery(dbPath string, query string, limit int) (*queryResult, error) {
	if err := rejectNonSelect(query); err != nil {
		return nil, err
	}
	limit = clampLimit(limit)

	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		return nil, sqlhint.WithColumnSuggestion(db, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := &queryResult{
		Columns:  cols,
		Rows:     make([]map[string]any, 0),
		RowLimit: limit,
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	for rows.Next() {
		if len(out.Rows) >= limit {
			out.Truncated = true
			out.TruncationReason = fmt.Sprintf("row limit %d reached; re-query with a smaller LIMIT or pass a lower limit argument", limit)
			break
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = cellValue(vals[i])
		}
		out.Rows = append(out.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out.Count = len(out.Rows)
	out.Warning = sqlhint.ZeroRowDisplayNameWarning(query, out.Count)
	return out, nil
}

func cellValue(v any) any {
	if v == nil {
		return nil
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}
