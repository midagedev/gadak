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
