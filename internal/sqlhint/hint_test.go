package sqlhint

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestZeroRowDisplayNameWarning(t *testing.T) {
	q := `SELECT key FROM issues WHERE status = 'In Progress'`
	if got := ZeroRowDisplayNameWarning(q, 0); got != zeroRowWarning {
		t.Fatalf("0-row display-name: %q", got)
	}
	if got := ZeroRowDisplayNameWarning(q, 2); got != "" {
		t.Fatalf("rows exist must stay silent, got %q", got)
	}
	safe := `SELECT key FROM issues WHERE status_category = 'inprogress'`
	if got := ZeroRowDisplayNameWarning(safe, 0); got != "" {
		t.Fatalf("status_category must not warn, got %q", got)
	}
	commented := "-- status = 'In Progress'\nSELECT key FROM issues WHERE priority_rank = 99"
	if got := ZeroRowDisplayNameWarning(commented, 0); got != "" {
		t.Fatalf("comment-only display name must not warn, got %q", got)
	}
}

func TestSuggestColumnIssueKey(t *testing.T) {
	cols := []string{"key", "issue_type", "issue_type_id", "status", "status_id", "summary", "id"}
	if got := suggestColumn("issue_key", cols); got != "key" {
		t.Fatalf("issue_key → %q, want key", got)
	}
	if got := suggestColumn("keey", cols); got != "key" {
		t.Fatalf("keey → %q, want key", got)
	}
	if got := suggestColumn("zzqx", cols); got != "" {
		t.Fatalf("zzqx → %q, want omit", got)
	}
	if got := suggestColumn("issue_type", cols); got != "" {
		t.Fatalf("exact match must not suggest, got %q", got)
	}
}

func TestWithColumnSuggestionWrongTable(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE issues (key TEXT, summary TEXT, status TEXT);
		CREATE TABLE items (id INTEGER, title TEXT);
		CREATE TABLE changelog (item_id INTEGER, at TEXT)`); err != nil {
		t.Fatal(err)
	}
	queryErr := func(q string) error {
		rows, err := db.Query(q)
		if err == nil {
			rows.Close()
			t.Fatalf("query %q unexpectedly succeeded", q)
		}
		return err
	}

	// Wrong table: the name exists verbatim on a hint table — name it,
	// instead of the old silence (GDK-974).
	got := WithColumnSuggestion(db, queryErr(`SELECT summary FROM changelog`))
	want := `column "summary" exists on issues — query issues`
	if !strings.Contains(got.Error(), want) {
		t.Fatalf("wrong-table hint missing: %q", got)
	}
	got = WithColumnSuggestion(db, queryErr(`SELECT title FROM changelog`))
	if !strings.Contains(got.Error(), `exists on items — query items`) {
		t.Fatalf("items ownership missing: %q", got)
	}

	// Typo path is unchanged.
	got = WithColumnSuggestion(db, queryErr(`SELECT sumary FROM issues`))
	if !strings.Contains(got.Error(), `did you mean "summary"?`) {
		t.Fatalf("typo hint missing: %q", got)
	}

	// A name nowhere near any column stays unadorned.
	base := queryErr(`SELECT zzqx FROM issues`)
	if got := WithColumnSuggestion(db, base); got.Error() != base.Error() {
		t.Fatalf("distant name must stay unadorned: %q", got)
	}
}

func TestStripCommentsCommentEdges(t *testing.T) {
	got := StripComments("SELECT/*x*/1")
	if got != "SELECT 1" {
		t.Errorf("SELECT/*x*/1 → %q, want \"SELECT 1\"", got)
	}
	got = StripComments("SELECT/*x*/key FROM issues_full")
	if got != "SELECT key FROM issues_full" {
		t.Errorf("SELECT/*x*/key → %q, want spaced SELECT", got)
	}
	got = StripComments(`SELECT "col--name"`)
	if got != `SELECT "col--name"` {
		t.Errorf("double-quoted -- must be preserved, got %q", got)
	}
}

func TestParseNoSuchColumn(t *testing.T) {
	name, ok := parseNoSuchColumn("SQL logic error: no such column: issue_key (1)")
	if !ok || name != "issue_key" {
		t.Fatalf("got %q ok=%v", name, ok)
	}
	if _, ok := parseNoSuchColumn("syntax error"); ok {
		t.Fatal("non-column error must not parse")
	}
}
