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

// demoIssuesColumns is the real `issues` column set, copied verbatim from
// `sqlite3 examples/demo.db "select name from pragma_table_info('issues')"`
// on 2026-09-15 — prefix misses are an agent-vs-schema shape, so the list
// must be the schema agents actually hit, not a curated sample.
var demoIssuesColumns = []string{
	"summary", "item_id", "key", "project_key", "issue_type", "issue_type_id",
	"status", "status_id", "status_category", "priority", "priority_rank",
	"assignee", "assignee_id", "assignee_email", "reporter", "reporter_id",
	"reporter_email", "parent_key", "labels", "components", "fix_versions",
	"affects_versions", "environment_text", "duedate", "resolution",
	"created_at", "updated_at", "status_changed_at", "resolved_at",
	"reopen_count", "reopened_at", "assignee_changed_at", "comment_count",
	"description_adf", "custom", "raw", "reopen_reason", "cloned_from",
	"hierarchy_level", "epic_key", "priority_id", "resolution_id", "sprint_id",
	"sprint_name", "sprint_state", "fix_version_ids", "security_level_id",
	"security_level", "started_at", "cycle_hours", "last_activity_at",
	"open_blockers", "carryover_count", "first_sprint_id", "first_sprint_at",
	"blocked_hours", "blocked_since", "description_text",
}

// GDK-1899: agents type the Jira field name, which is a prefix of the real
// column — created → created_at, description → both description_adf and
// description_text (both are right answers; the hint must not pick for the
// caller). Ordered shortest first, then alphabetical.
func TestSuggestColumnPrefixMisses(t *testing.T) {
	if got := suggestColumn("created", demoIssuesColumns); got != "created_at" {
		t.Errorf("created → %q, want created_at", got)
	}
	if got := suggestColumn("updated", demoIssuesColumns); got != "updated_at" {
		t.Errorf("updated → %q, want updated_at", got)
	}
	want := []string{"description_adf", "description_text"}
	got := suggestColumns("description", demoIssuesColumns)
	if len(got) != len(want) || got[0] != want[0] || got[len(got)-1] != want[len(want)-1] {
		t.Errorf("description → %v, want %v", got, want)
	}
	// A prefix of nothing, edit-distant: stays silent.
	if got := suggestColumns("foo", demoIssuesColumns); len(got) != 0 {
		t.Errorf("foo → %v, want none", got)
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
	hintFor := func(q string) error {
		rows, err := db.Query(q)
		if err == nil {
			rows.Close()
			t.Fatalf("query %q unexpectedly succeeded", q)
		}
		return WithColumnSuggestion(db, q, err)
	}

	// Wrong table: the name exists verbatim on a hint table — name it,
	// instead of the old silence (GDK-974).
	got := hintFor(`SELECT summary FROM changelog`)
	want := `column "summary" exists on issues — query issues`
	if !strings.Contains(got.Error(), want) {
		t.Fatalf("wrong-table hint missing: %q", got)
	}
	got = hintFor(`SELECT title FROM changelog`)
	if !strings.Contains(got.Error(), `exists on items — query items`) {
		t.Fatalf("items ownership missing: %q", got)
	}

	// Typo path is unchanged.
	got = hintFor(`SELECT sumary FROM issues`)
	if !strings.Contains(got.Error(), `did you mean "summary"?`) {
		t.Fatalf("typo hint missing: %q", got)
	}

	// A name nowhere near any column stays unadorned.
	q := `SELECT zzqx FROM issues`
	rows, baseErr := db.Query(q)
	if baseErr == nil {
		rows.Close()
		t.Fatalf("query %q unexpectedly succeeded", q)
	}
	if got := WithColumnSuggestion(db, q, baseErr); got.Error() != baseErr.Error() {
		t.Fatalf("distant name must stay unadorned: %q", got)
	}
}

// GDK-1899: a prefix miss with two continuations names both in one
// did-you-mean — `did you mean "a" or "b"?`.
func TestWithColumnSuggestionPrefixTwoCandidates(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE issues (description_adf TEXT, description_text TEXT)`); err != nil {
		t.Fatal(err)
	}
	q := `SELECT description FROM issues`
	rows, baseErr := db.Query(q)
	if baseErr == nil {
		rows.Close()
		t.Fatalf("query %q unexpectedly succeeded", q)
	}
	got := WithColumnSuggestion(db, q, baseErr)
	if !strings.Contains(got.Error(), `did you mean "description_adf" or "description_text"?`) {
		t.Fatalf("two-candidate hint missing: %q", got)
	}
}

// GDK-595: json_each in the FROM list makes `key`/`value` ambiguous, and the
// bare SQLite sentence does not say the function brought its own columns.
// The hint is scoped to json_each queries — an ambiguity between two ordinary
// tables keeps the plain error.
func TestWithColumnSuggestionJSONEachAmbiguity(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE issues (key TEXT, labels TEXT);
		CREATE TABLE items (key TEXT)`); err != nil {
		t.Fatal(err)
	}
	hintFor := func(q string) error {
		rows, err := db.Query(q)
		if err == nil {
			rows.Close()
			t.Fatalf("query %q unexpectedly succeeded", q)
		}
		return WithColumnSuggestion(db, q, err)
	}

	got := hintFor(`SELECT key FROM issues, json_each(labels) WHERE json_each.value='batch'`)
	for _, want := range []string{"ambiguous column name: key", "json_each exposes", "issues_full.key"} {
		if !strings.Contains(got.Error(), want) {
			t.Errorf("hint missing %q: %q", want, got)
		}
	}

	// Same ambiguity, no json_each: the plain SQLite error, no hint.
	q := `SELECT key FROM issues, items`
	plain := hintFor(q)
	if !strings.Contains(plain.Error(), "ambiguous column name: key") {
		t.Fatalf("plain ambiguity lost its error: %q", plain)
	}
	if strings.Contains(plain.Error(), "hint:") {
		t.Fatalf("ordinary ambiguity got the json_each hint: %q", plain)
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

// TestParseAmbiguousColumn pins the twin of parseNoSuchColumn: the same
// unquote-and-strip-qualifier rule, read off SQLite's "ambiguous column
// name" sentence instead. The qualified forms matter because SQLite names
// the alias in the message (`issues.key`) while the json_each hint compares
// the bare column.
func TestParseAmbiguousColumn(t *testing.T) {
	name, ok := parseAmbiguousColumn("SQL logic error: ambiguous column name: key (1)")
	if !ok || name != "key" {
		t.Fatalf("got %q ok=%v", name, ok)
	}
	if name, ok := parseAmbiguousColumn(`ambiguous column name: "issues".key`); !ok || name != "key" {
		t.Fatalf("qualified name: got %q ok=%v, want key", name, ok)
	}
	if _, ok := parseAmbiguousColumn("syntax error"); ok {
		t.Fatal("non-column error must not parse")
	}
}

// GDK-2029: the mirror image of the prefix miss. An agent that knows Jira's
// API vocabulary types the real column with a suffix glued on —
// assignee_name, reporter_display_name, status_name — and the hint was
// silent on all of them, which is the second wall for an agent whose skill
// file is missing or stale (the reproduction was one such session going off
// to read sqlite_master by hand). The longest real column that the unknown
// name starts with is the answer: reporter_display_name is reporter, not a
// Levenshtein neighbour.
func TestSuggestColumnSuffixMisses(t *testing.T) {
	for _, tc := range []struct{ unknown, want string }{
		{"assignee_name", "assignee"},
		{"reporter_display_name", "reporter"},
		{"status_name", "status"},
		{"priority_name", "priority"},
		{"assignee_email_address", "assignee_email"},
	} {
		if got := suggestColumn(tc.unknown, demoIssuesColumns); got != tc.want {
			t.Errorf("%s → %q, want %q", tc.unknown, got, tc.want)
		}
	}
	// The rules that were already there keep their answers: a prefix glued
	// in front still resolves to the column it names, and a name that is a
	// prefix of real columns still offers those.
	if got := suggestColumn("issue_key", demoIssuesColumns); got != "key" {
		t.Errorf("issue_key → %q, want key", got)
	}
	if got := suggestColumn("created", demoIssuesColumns); got != "created_at" {
		t.Errorf("created → %q, want created_at", got)
	}
}
