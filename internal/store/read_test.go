package store

// Reader-side contracts that have no home in the bigger test files.

import (
	"context"
	"database/sql"
	"testing"
)

// TestSprintsCarryTheGoal — the goal has been stored since sprints became
// rows, and every read surface dropped it (GDK-1695). FAIL-first is the CLI
// column, but the reader has to hand it over first.
func TestSprintsCarryTheGoal(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO sources (id, kind) VALUES ('jira','jira');
			INSERT INTO boards (source_id, id, name, type, project_key) VALUES ('jira',1,'Team board','scrum','');
			INSERT INTO sprints (source_id, id, board_id, name, goal, state, start_at, end_at, external_id)
			VALUES ('jira',42,1,'Sprint 42','Ship the uploader','active','2026-09-01T00:00:00Z','2026-09-15T00:00:00Z','42')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Sprints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("sprints = %d, want 1", len(rows))
	}
	if rows[0].Goal != "Ship the uploader" {
		t.Errorf("goal = %q, want the stored goal", rows[0].Goal)
	}
}
