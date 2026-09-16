package main

// The built-in destination's sprint pass (GDK-1961). Sprints do not fit the
// issuetap fixture document — the seed format has no sprint slot, and that
// format lives in a repo gadak only pins, not one it edits — so the sprint
// graph travels as a second pass after the seed, through the Agile write API
// on the origin (the product invariant: writes go through origin, never a
// mirror). The verbs are the ones `gadak sprint` already writes with, so this
// file is wiring, not a second write path.

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/migrate"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// agileOrigin adapts the Jira Agile client onto the pass's narrow surface.
type agileOrigin struct{ c *jira.Client }

func (a agileOrigin) BoardForProject(ctx context.Context, projectKey string) (int64, error) {
	boards, err := a.c.BoardsForProject(ctx, projectKey)
	if err != nil {
		return 0, err
	}
	if len(boards) == 0 {
		return 0, fmt.Errorf("no board for project %s", projectKey)
	}
	// The built-in tracker materializes exactly one scrum board per project;
	// a later multi-board origin takes the first scrum board.
	return boards[0].ID, nil
}

func (a agileOrigin) CreateSprint(ctx context.Context, boardID int64, name, goal string) (int64, error) {
	s, err := a.c.CreateSprint(ctx, boardID, name, goal)
	return s.ID, err
}

func (a agileOrigin) UpdateSprint(ctx context.Context, sprintID int64, fields map[string]any) error {
	_, err := a.c.UpdateSprint(ctx, sprintID, fields)
	return err
}

func (a agileOrigin) MoveToSprint(ctx context.Context, sprintID int64, keys []string) error {
	return a.c.MoveToSprint(ctx, sprintID, keys)
}

// migrateSprints runs the sprint half of the built-in migrate between the
// seed and origin.Close: every sprint the export's issues reference is
// created on the origin and filled, then one incremental sync re-mirrors
// what the pass wrote — sprint moves bump the issues' `updated`, which is
// exactly the window an incremental pass re-reads, and the agile listing
// that lands the sprints rows runs on every tick. Nothing here is fatal:
// the workspace already exists at this point, and a partially carried
// sprint graph is repairable by hand (`gadak sprint add`) while an abort
// would leave no path forward, since migrate refuses existing targets.
// Failures land in st.SprintErrors and surface in the report.
func migrateSprints(ctx context.Context, target string, tcfg *config.Config, srcDB *sql.DB, st *migrate.Stats) {
	if st.SprintIssues == 0 {
		return
	}
	c, err := origin.Client(tcfg)
	if err != nil {
		st.SprintErrors = append(st.SprintErrors, fmt.Sprintf("origin client: %v", err))
		return
	}
	if err := migrate.MigrateSprints(ctx, srcDB, agileOrigin{c}, st); err != nil {
		st.SprintErrors = append(st.SprintErrors, err.Error())
		return
	}
	p, err := config.DBPathFor(target)
	if err != nil {
		st.SprintErrors = append(st.SprintErrors, fmt.Sprintf("refill: %v", err))
		return
	}
	db, err := store.Open(p)
	if err != nil {
		st.SprintErrors = append(st.SprintErrors, fmt.Sprintf("refill: %v", err))
		return
	}
	if _, err := syncer.Run(ctx, tcfg, db, syncer.Options{Client: c}); err != nil {
		st.SprintErrors = append(st.SprintErrors, fmt.Sprintf("refill: %v", err))
	}
	if err := db.Close(); err != nil {
		st.SprintErrors = append(st.SprintErrors, fmt.Sprintf("refill close: %v", err))
	}
}

// printSprintNotes reports the sprint pass's honest half: what was created,
// what the Agile API's own close rule swept away, and what could not travel.
// The count table above it already carries the sprints and sprint-issues
// rows; these lines are the reasons behind any difference.
func printSprintNotes(w *os.File, st *migrate.Stats) {
	if st.SprintsCreated > 0 {
		fmt.Fprintf(w, "sprints: %d created", st.SprintsCreated)
		if st.SprintSwept > 0 {
			fmt.Fprintf(w, "; closing swept %d not-done issues to the backlog (the Agile API cannot hold a not-done issue in a closed sprint)", st.SprintSwept)
		}
		fmt.Fprintln(w)
	}
	for _, s := range st.SprintStuck {
		fmt.Fprintf(w, "sprints: %s\n", s)
	}
	for _, e := range st.SprintErrors {
		fmt.Fprintf(w, "  ! sprints: %s\n", e)
	}
	if st.SprintOrphans > 0 {
		fmt.Fprintf(w, "issues referencing sprints the source mirror no longer lists: %d (nothing to create them from)\n", st.SprintOrphans)
	}
}
