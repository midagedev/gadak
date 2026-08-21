package sqlhint

import "testing"

func TestZeroRowDisplayNameWarning(t *testing.T) {
	q := `SELECT key FROM issues WHERE status = 'In Progress'`
	if got := ZeroRowDisplayNameWarning(q, 0); got != ZeroRowWarning {
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

func TestParseNoSuchColumn(t *testing.T) {
	name, ok := parseNoSuchColumn("SQL logic error: no such column: issue_key (1)")
	if !ok || name != "issue_key" {
		t.Fatalf("got %q ok=%v", name, ok)
	}
	if _, ok := parseNoSuchColumn("syntax error"); ok {
		t.Fatal("non-column error must not parse")
	}
}
