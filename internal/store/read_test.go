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

// TestSprintsSplitCountsByCategory — the strip draws progress from the
// server's split, not from the issues the client happens to have loaded
// (GDK-1709): a filtered or paginated board would otherwise report a subset
// as the sprint's whole shape. Todo is the remainder, so a category outside
// the three lands in "to do" instead of dropping off the bar.
func TestSprintsSplitCountsByCategory(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO sources (id, kind) VALUES ('jira','jira');
			INSERT INTO boards (source_id, id, name, type, project_key) VALUES ('jira',1,'Team board','scrum','');
			INSERT INTO sprints (source_id, id, board_id, name, goal, state, start_at, end_at, external_id)
			VALUES ('jira',42,1,'Sprint 42','','active','2026-09-01T00:00:00Z','2026-09-15T00:00:00Z','42');
			INSERT INTO items (id, source_id, kind, key, title) VALUES
			  ('i1','jira','issue','ENG-1','one'), ('i2','jira','issue','ENG-2','two'),
			  ('i3','jira','issue','ENG-3','three'), ('i4','jira','issue','ENG-4','four'),
			  ('i5','jira','issue','ENG-5','five');
			INSERT INTO issues_raw (item_id, key, project_key, status_category, sprint_id, priority_rank, reopen_count, comment_count) VALUES
			  ('i1','ENG-1','ENG','done',42,0,0,0),
			  ('i2','ENG-2','ENG','done',42,0,0,0),
			  ('i3','ENG-3','ENG','inprogress',42,0,0,0),
			  ('i4','ENG-4','ENG','new',42,0,0,0),
			  ('i5','ENG-5','ENG','triage',42,0,0,0)`)
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
	got := rows[0]
	if got.IssueCount != 5 {
		t.Errorf("issue_count = %d, want 5", got.IssueCount)
	}
	if got.Done != 2 || got.InProgress != 1 || got.Todo != 2 {
		t.Errorf("done/in_progress/todo = %d/%d/%d, want 2/1/2 (the unknown category counts as to do)",
			got.Done, got.InProgress, got.Todo)
	}
	if got.Done+got.InProgress+got.Todo != got.IssueCount {
		t.Errorf("the three segments (%d+%d+%d) must add up to issue_count %d",
			got.Done, got.InProgress, got.Todo, got.IssueCount)
	}
	// No story_points alias anywhere in this mirror: nil, not zero.
	if got.Points != nil || got.DonePoints != nil {
		t.Errorf("points/done_points = %v/%v, want nil on a mirror with no story_points alias", got.Points, got.DonePoints)
	}
}

// TestSprintsSumStoryPointsWhenMapped — the second unit on the strip. The
// demo fixture maps no story_points alias (measured: zero rows carry one), so
// this is where that contract is held.
func TestSprintsSumStoryPointsWhenMapped(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO sources (id, kind) VALUES ('jira','jira');
			INSERT INTO boards (source_id, id, name, type, project_key) VALUES ('jira',1,'Team board','scrum','');
			INSERT INTO sprints (source_id, id, board_id, name, goal, state, start_at, end_at, external_id)
			VALUES ('jira',42,1,'Sprint 42','','active','2026-09-01T00:00:00Z','2026-09-15T00:00:00Z','42');
			INSERT INTO items (id, source_id, kind, key, title) VALUES
			  ('i1','jira','issue','ENG-1','one'), ('i2','jira','issue','ENG-2','two'),
			  ('i3','jira','issue','ENG-3','three');
			INSERT INTO issues_raw (item_id, key, project_key, status_category, sprint_id, custom, priority_rank, reopen_count, comment_count) VALUES
			  ('i1','ENG-1','ENG','done',42,'{"story_points":5}',0,0,0),
			  ('i2','ENG-2','ENG','new',42,'{"story_points":8}',0,0,0),
			  ('i3','ENG-3','ENG','new',42,'{}',0,0,0)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Sprints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Points == nil || *rows[0].Points != 13 {
		t.Errorf("points = %v, want 13 (the unestimated issue adds nothing)", rows[0].Points)
	}
	if rows[0].DonePoints == nil || *rows[0].DonePoints != 5 {
		t.Errorf("done_points = %v, want 5", rows[0].DonePoints)
	}
}

// TestIssueLiteCarriesCarryoverCount — the column has been derived since v48
// and no read surface handed it over (GDK-1711). Nil and 0 are different
// answers and both have to survive the wire.
func TestIssueLiteCarriesCarryoverCount(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO sources (id, kind) VALUES ('jira','jira');
			INSERT INTO items (id, source_id, kind, key, title) VALUES
			  ('i1','jira','issue','ENG-1','carried'), ('i2','jira','issue','ENG-2','never'),
			  ('i3','jira','issue','ENG-3','unreadable');
			INSERT INTO issues_raw (item_id, key, project_key, status_category, carryover_count, priority_rank, reopen_count, comment_count) VALUES
			  ('i1','ENG-1','ENG','new',2,0,0,0),
			  ('i2','ENG-2','ENG','new',0,0,0,0),
			  ('i3','ENG-3','ENG','new',NULL,0,0,0)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.IssueLitesByKeys(ctx, []string{"ENG-1", "ENG-2", "ENG-3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].CarryoverCount == nil || *rows[0].CarryoverCount != 2 {
		t.Errorf("ENG-1 carryover_count = %v, want 2", rows[0].CarryoverCount)
	}
	if rows[1].CarryoverCount == nil || *rows[1].CarryoverCount != 0 {
		t.Errorf("ENG-2 carryover_count = %v, want 0", rows[1].CarryoverCount)
	}
	if rows[2].CarryoverCount != nil {
		t.Errorf("ENG-3 carryover_count = %v, want nil — an origin with no changelog cannot answer", rows[2].CarryoverCount)
	}
}

// TestBoardsCarryTheSprintFlag — the retro's board picker has to offer
// exactly the boards a sprint-cut report would accept (GDK-1713), so the
// flag is the report's own predicate: a sprint with a start date. A board
// whose only sprint has no start is not a choice.
func TestBoardsCarryTheSprintFlag(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO sources (id, kind) VALUES ('jira','jira');
			INSERT INTO boards (source_id, id, name, type, project_key) VALUES
			  ('jira',1,'Team board','scrum',''),
			  ('jira',2,'Kanban board','kanban',''),
			  ('jira',3,'Unscheduled','scrum','');
			INSERT INTO sprints (source_id, id, board_id, name, goal, state, start_at, end_at, external_id)
			VALUES ('jira',42,1,'Sprint 42','','active','2026-09-01T00:00:00Z','2026-09-15T00:00:00Z','42'),
			       ('jira',43,3,'Someday','','future',NULL,NULL,'43')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Boards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("boards = %d, want 3", len(rows))
	}
	want := map[int64]bool{1: true, 2: false, 3: false}
	for _, b := range rows {
		if b.HasSprints != want[b.ID] {
			t.Errorf("board %d (%s): has_sprints = %v, want %v", b.ID, b.Name, b.HasSprints, want[b.ID])
		}
	}
	if rows[0].Type != "scrum" {
		t.Errorf("board 1 type = %q, want scrum", rows[0].Type)
	}
}
