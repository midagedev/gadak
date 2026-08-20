package store

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"testing"
)

// liveTables is every real table in both schemas, read from the database rather
// than from the source, so a table added by a migration shows up here whether
// or not anyone remembered this test exists.
func liveTables(t *testing.T, db *DB) []string {
	t.Helper()
	var out []string
	for _, q := range []string{
		`SELECT name FROM sqlite_master WHERE type = 'table'`,
		`SELECT name FROM local.sqlite_master WHERE type = 'table'`,
	} {
		rows, err := db.sql.QueryContext(context.Background(), q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			switch {
			case strings.HasPrefix(n, "sqlite_"):
				// SQLite's own bookkeeping (sqlite_sequence for AUTOINCREMENT).
			case strings.HasPrefix(n, "items_fts_"):
				// fts5 shadow tables; the virtual table items_fts owns them.
			default:
				out = append(out, n)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
	}
	sort.Strings(out)
	return out
}

// The recurrence layer for GDK-418. The defect was not that four tables were
// handled wrongly — it was that the set of tables conversion must consider was
// a literal list one file away from the migrations, so enrichments (schemaV2),
// feed_reads (schemaV4), field_usage (schemaV7) and sync_runs (schemaV8) each
// arrived without anyone being told a decision was due. This test is the thing
// that tells them: add a table to either schema without classifying it and the
// suite fails, naming the table and the four choices.
//
// FAIL-first, measured 2026-08-20: dropping the `feed_reads` rule from
// originScopedTables fails with
//
//	classified in origin_scope.go but absent from the schema: []
//	in the schema but not classified in origin_scope.go: [feed_reads]
func TestEveryTableIsOriginScoped(t *testing.T) {
	db := openTemp(t)
	live := liveTables(t, db)

	classified := map[string]bool{}
	for _, r := range originScopedTables {
		if classified[r.table] {
			t.Errorf("originScopedTables lists %q twice", r.table)
		}
		classified[r.table] = true
	}

	var missing, stale []string
	for _, n := range live {
		if !classified[n] {
			missing = append(missing, n)
		}
	}
	for n := range classified {
		found := false
		for _, l := range live {
			if l == n {
				found = true
				break
			}
		}
		if !found {
			stale = append(stale, n)
		}
	}
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Errorf("in the schema but not classified in origin_scope.go: %v\n"+
			"classified in origin_scope.go but absent from the schema: %v\n"+
			"every table must answer \"the workspace's origin is being replaced\": "+
			"scopeMirror, scopeDerived, scopeAuthored or scopeLocal",
			missing, stale)
	}
}

// A table that keeps its rows through a conversion has to say why. Without
// this, "we decided this survives" and "we forgot this one" are the same diff —
// which is the shape the original defect had.
func TestSurvivingTablesRecordWhy(t *testing.T) {
	for _, r := range originScopedTables {
		if r.dropForSource != "" || r.dropForOrigin != "" {
			continue
		}
		if strings.TrimSpace(r.why) == "" {
			t.Errorf("%s keeps its rows through a conversion and gives no reason", r.table)
		}
	}
}

// Each statement is executed with the source id as its only bind parameter, so
// a rule with two placeholders would fail at runtime on a path users reach once
// and cannot retry.
func TestDropStatementsTakeExactlyOneParameter(t *testing.T) {
	for _, r := range originScopedTables {
		if n := strings.Count(r.dropForSource, "?"); r.dropForSource != "" && n != 1 {
			t.Errorf("%s dropForSource has %d placeholders, want 1", r.table, n)
		}
		if n := strings.Count(r.dropForOrigin, "?"); n != 0 {
			t.Errorf("%s dropForOrigin has %d placeholders, want 0", r.table, n)
		}
	}
}

// Every statement must parse and address a table that exists — including the
// `local.` ones, which only resolve because the driver attaches local.db on
// every connection. A typo here is invisible until a user converts.
func TestDropStatementsRunAgainstTheRealSchema(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	for _, r := range originScopedTables {
		for _, q := range []string{r.dropForSource, r.dropForOrigin} {
			if q == "" {
				continue
			}
			args := []any{}
			if strings.Contains(q, "?") {
				args = append(args, "jira")
			}
			if err := db.write(ctx, func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, q, args...)
				return err
			}); err != nil {
				t.Errorf("%s: %q: %v", r.table, q, err)
			}
		}
	}
}

// The epoch subquery is embedded in inserts and in the History filter, so it
// has to be a valid scalar expression against an untouched local.db — where the
// local_meta row does not exist yet and COALESCE is what keeps it 0 rather than
// NULL (a NULL would make `origin_epoch = NULL` match nothing and empty the
// timeline for every existing install).
func TestCurrentEpochIsZeroBeforeAnyConversion(t *testing.T) {
	db := openTemp(t)
	var epoch int
	if err := db.sql.QueryRowContext(context.Background(), `SELECT `+currentEpochSQL).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	if epoch != 0 {
		t.Errorf("epoch on a fresh workspace = %d, want 0", epoch)
	}
}
