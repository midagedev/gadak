package snapshot

import (
	"testing"
	"time"
)

func TestParseWindowComposite(t *testing.T) {
	// Go's parser handles compound units when no custom d/w suffix is used.
	d, err := ParseWindow("1h30m")
	if err != nil {
		t.Fatal(err)
	}
	if d != 90*time.Minute {
		t.Errorf("got %v", d)
	}
}

// flowOf reads the beats off a changelog slice that loadTableMaps ordered by
// row id, not by time — a mirrored history routinely arrives with its ids in a
// different order than its stamps. Reading "the last Done row" off the slice
// hung resolved_at on the wrong transition, which store.Derive (which sorts on
// At) then disagreed with. GDK-1739.
func TestFlowOfPicksByTimeNotSlicePosition(t *testing.T) {
	cats := map[string]string{"3": "inprogress", "10018": "done", "10016": "new"}
	// Slice order is id order: the late Done row sorts first, the early one
	// last, and the In Progress row sits between two In Progress transitions.
	rows := []map[string]any{
		{"id": "a", "field": "status", "at": "2026-03-10T00:00:00.000Z", "to_id": "10018"},
		{"id": "b", "field": "status", "at": "2026-03-01T00:00:00.000Z", "to_id": "10018"},
		{"id": "c", "field": "status", "at": "2026-02-20T00:00:00.000Z", "to_id": "3"},
		{"id": "d", "field": "status", "at": "2026-02-10T00:00:00.000Z", "to_id": "3"},
		{"id": "e", "field": "assignee", "at": "2026-02-05T00:00:00.000Z", "to_id": "3"},
		{"id": "f", "field": "status", "at": "", "to_id": "3"},
	}
	f := flowOf(rows, cats)
	if f.firstIP != 3 {
		t.Errorf("firstIP = %d, want 3 (the 02-10 row, not the first in slice order)", f.firstIP)
	}
	if f.firstDone != 1 {
		t.Errorf("firstDone = %d, want 1 (03-01)", f.firstDone)
	}
	if f.lastDone != 0 {
		t.Errorf("lastDone = %d, want 0 (03-10)", f.lastDone)
	}

	// A history with no in-progress transition is what ensureStartTransitions
	// is looking for, and an unstamped row is not one.
	only := []map[string]any{
		{"id": "a", "field": "status", "at": "2026-03-10T00:00:00.000Z", "to_id": "10018"},
		{"id": "b", "field": "status", "at": "", "to_id": "3"},
	}
	if g := flowOf(only, cats); g.firstIP != -1 || g.firstDone != 0 || g.lastDone != 0 {
		t.Errorf("flowOf(only) = %+v, want firstIP -1 and both done beats 0", g)
	}
}
