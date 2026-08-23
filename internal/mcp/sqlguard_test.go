package mcp

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRejectNonSelectCommentEdges(t *testing.T) {
	if err := rejectNonSelect("SELECT/*x*/1"); err != nil {
		t.Errorf("SELECT/*x*/1 must stay a SELECT, got %v", err)
	}
	if err := rejectNonSelect("SELECT/*x*/key FROM issues_full"); err != nil {
		t.Errorf("SELECT/*x*/key must stay a SELECT, got %v", err)
	}
	q := `SELECT 1 FROM "col--name"; DELETE FROM issues`
	if err := rejectNonSelect(q); err == nil {
		t.Errorf("double-quoted -- must not hide a second statement: %q", q)
	}
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

// TestOpenReadOnlySetsBusyTimeout is FAIL-first for GDK-757: sqlguard's
// mode=ro DSN had no busy_timeout, so PRAGMA busy_timeout was 0 while
// store.Open / OpenReadOnly / EnsureLocal all pin 5000.
func TestOpenReadOnlySetsBusyTimeout(t *testing.T) {
	path := demoDB(t)
	db, err := openReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var got string
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "5000" {
		t.Fatalf("sqlguard busy_timeout = %s, want 5000", got)
	}
}

// TestRunQueryWaitsForWriterThenSucceeds is FAIL-first for GDK-757: a
// connection holding a write lock on a DELETE-journal file makes SELECT
// wait. Without busy_timeout the reader fails immediately with SQLITE_BUSY;
// with 5000ms it waits for the holder to commit (80ms here) and succeeds.
// WAL readers are a different path (store.TestWALReaderNotBlockedByWriter);
// busy_timeout covers checkpoint, attached DELETE local.db, and non-WAL files.
func TestRunQueryWaitsForWriterThenSucceeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gadak.db")
	// DELETE journal (the default) + _txlock=exclusive: BEGIN takes EXCLUSIVE
	// and blocks readers. BEGIN IMMEDIATE only takes RESERVED, which still
	// allows SELECT — that path cannot demonstrate a missing busy_timeout.
	w, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_txlock=exclusive")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	w.SetMaxOpenConns(1)
	if _, err := w.Exec(`CREATE TABLE issues (key TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO issues VALUES ('NMB-1')`); err != nil {
		t.Fatal(err)
	}
	tx, err := w.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO issues VALUES ('HOLD')`); err != nil {
		t.Fatal(err)
	}

	committed := make(chan error, 1)
	go func() {
		time.Sleep(80 * time.Millisecond)
		committed <- tx.Commit()
	}()

	start := time.Now()
	res, err := runQuery(path, `SELECT key FROM issues`, 10)
	elapsed := time.Since(start)
	if cerr := <-committed; cerr != nil {
		t.Fatalf("holder commit: %v", cerr)
	}
	if err != nil {
		t.Fatalf("runQuery under held writer: %v (elapsed %v)", err, elapsed)
	}
	if res.Count < 1 {
		t.Fatalf("count=%d, want rows after holder committed", res.Count)
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("runQuery returned in %v; expected to wait for the holder (~80ms)", elapsed)
	}
}
