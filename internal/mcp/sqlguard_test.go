package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// The agent-facing contract is the JSON wire shape, so these tests assert on
// the marshaled result rather than on struct fields. That also lets them
// compile and fail against a queryResult with no warning support (FAIL-first
// for GDK-90).

// queryJSON runs q and returns the marshaled result alongside the struct.
func queryJSON(t *testing.T, dbPath, q string) (string, *queryResult) {
	t.Helper()
	res, err := runQuery(dbPath, q, 0)
	if err != nil {
		t.Fatalf("runQuery(%q): %v", q, err)
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return string(b), res
}

func TestQueryWarnsOnZeroRowsWithDisplayNameFilter(t *testing.T) {
	db := demoDB(t)
	// demo.db is an English-locale mirror, so the localized spellings below
	// match nothing — the exact silent-zero-rows trap the warning exists for.
	for _, q := range []string{
		`SELECT key FROM issues WHERE status = '진행 중'`,
		`SELECT key FROM issues WHERE STATUS = 'Nonexistent'`,
		`SELECT key FROM issues WHERE status='Nonexistent'`,
		`SELECT key FROM issues WHERE issue_type = '버그'`,
		`SELECT key FROM issues WHERE priority = '높음'`,
	} {
		t.Run(q, func(t *testing.T) {
			b, res := queryJSON(t, db, q)
			if res.Count != 0 {
				t.Fatalf("fixture premise: want 0 rows, got %d", res.Count)
			}
			if !strings.Contains(b, `"warning"`) {
				t.Fatalf("zero rows with a display-name filter must carry a warning, got %s", b)
			}
			for _, col := range []string{"status_category", "priority_rank", "issue_type_id"} {
				if !strings.Contains(b, col) {
					t.Fatalf("warning must point at %s, got %s", col, b)
				}
			}
		})
	}
}

func TestQueryNoWarningOnSafeColumns(t *testing.T) {
	db := demoDB(t)
	for _, q := range []string{
		`SELECT key FROM issues WHERE status_category = 'nonexistent'`,
		`SELECT key FROM issues WHERE status_id = 'nonexistent'`,
		`SELECT key FROM issues WHERE issue_type_id = 'nonexistent'`,
		`SELECT key FROM issues WHERE priority_rank = 99`,
		`SELECT key FROM issues WHERE priority_rank = 99 AND status_category = 'nonexistent'`,
		// A display name inside a stripped comment is not a filter.
		"-- status = 'In Progress'\nSELECT key FROM issues WHERE priority_rank = 99",
	} {
		t.Run(q, func(t *testing.T) {
			b, res := queryJSON(t, db, q)
			if res.Count != 0 {
				t.Fatalf("fixture premise: want 0 rows, got %d", res.Count)
			}
			if strings.Contains(b, `"warning"`) {
				t.Fatalf("safe id/category filter must not warn, got %s", b)
			}
		})
	}
}

func TestQueryNoWarningWhenRowsExist(t *testing.T) {
	db := demoDB(t)
	b, res := queryJSON(t, db, `SELECT key FROM issues WHERE status = 'In Progress' LIMIT 2`)
	if res.Count != 2 {
		t.Fatalf("fixture premise: want 2 rows, got %d (%s)", res.Count, b)
	}
	if strings.Contains(b, `"warning"`) {
		t.Fatalf("rows exist, so the display-name filter must not warn, got %s", b)
	}
}
