package mcp

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
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
	s := stripSQLComments(query)
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("empty SQL")
	}
	// Allow one trailing semicolon; anything else is multi-statement.
	body := strings.TrimRight(s, " \t\n\r;")
	if strings.Contains(body, ";") {
		return errors.New("multiple statements are not allowed; send one SELECT or WITH")
	}
	kw := firstKeyword(body)
	switch strings.ToUpper(kw) {
	case "SELECT", "WITH":
		return nil
	case "":
		return errors.New("empty SQL")
	default:
		return fmt.Errorf("only SELECT or WITH statements are allowed (got %q)", kw)
	}
}

// stripSQLComments removes -- line comments and /* */ block comments outside of
// string literals so a leading comment does not hide the real keyword.
func stripSQLComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		// Single-quoted string (SQL escapes '' as one quote).
		if s[i] == '\'' {
			b.WriteByte('\'')
			i++
			for i < len(s) {
				if s[i] == '\'' {
					b.WriteByte('\'')
					i++
					if i < len(s) && s[i] == '\'' {
						b.WriteByte('\'')
						i++
						continue
					}
					break
				}
				b.WriteByte(s[i])
				i++
			}
			continue
		}
		// Double-quoted identifier.
		if s[i] == '"' {
			b.WriteByte('"')
			i++
			for i < len(s) {
				c := s[i]
				b.WriteByte(c)
				i++
				if c == '"' {
					if i < len(s) && s[i] == '"' {
						b.WriteByte('"')
						i++
						continue
					}
					break
				}
			}
			continue
		}
		// Line comment.
		if s[i] == '-' && i+1 < len(s) && s[i+1] == '-' {
			i += 2
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		// Block comment.
		if s[i] == '/' && i+1 < len(s) && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			if i+1 < len(s) {
				i += 2
			}
			// Leave a space so "SELECT/*x*/1" stays tokenised.
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func firstKeyword(s string) string {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	if s == "" {
		return ""
	}
	end := 0
	for end < len(s) {
		r := rune(s[end])
		// Keywords are ASCII letters; stop at anything else.
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' {
			end++
			continue
		}
		break
	}
	return s[:end]
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
		return nil, err
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
