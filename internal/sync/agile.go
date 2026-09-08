package sync

import (
	"context"
	"errors"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// importAgile fills the boards and sprints tables from the Agile API
// (GDK-1654). A site without Jira Software has no Agile API at all and says
// so once, quietly — that is not a failed sync. As with filters, a failure
// leaves the previous rows: a board-list 500 must not undo an issue pass.
func importAgile(ctx context.Context, c *jira.Client, db *store.DB, opts Options) {
	boards, err := c.Boards(ctx)
	if err != nil {
		if errors.Is(err, jira.ErrNoAgile) {
			return
		}
		opts.logf("boards: skipped (%v)", err)
		return
	}
	rows := make([]store.BoardRow, 0, len(boards))
	sprints := make([]store.SprintRow, 0, len(boards))
	seen := map[int64]bool{}
	for _, b := range boards {
		rows = append(rows, store.BoardRow{ID: b.ID, Name: b.Name, Type: b.Type, ProjectKey: b.ProjectKey})
		got, err := c.Sprints(ctx, b.ID)
		if err != nil {
			if errors.Is(err, jira.ErrNoAgile) {
				// A kanban board has no sprints; the route 400s or 404s.
				continue
			}
			opts.logf("sprints: board %d skipped (%v)", b.ID, err)
			continue
		}
		for _, s := range got {
			if seen[s.ID] {
				continue
			}
			seen[s.ID] = true
			board := s.OriginBoardID
			if board == 0 {
				board = b.ID
			}
			sprints = append(sprints, store.SprintRow{
				ID: s.ID, BoardID: board, Name: s.Name, Goal: s.Goal,
				State:       s.State,
				StartAt:     s.StartDate,
				EndAt:       s.EndDate,
				CompleteAt:  s.CompleteDate,
				ActivatedAt: s.ActivatedDate,
			})
		}
	}
	if err := db.ReplaceAgile(ctx, SourceID, rows, sprints); err != nil {
		opts.logf("boards: store failed (%v)", err)
		return
	}
	if len(rows) > 0 {
		opts.logf("agile: %d boards, %d sprints", len(rows), len(sprints))
	}
}

// RefreshAgile re-reads boards and sprints only. A sprint write changes no
// issue, so the pass that follows it would otherwise be a quiet tick that
// skips the agile listing and leaves the mirror stating the old state —
// gadak reporting what it wrote instead of what the origin now holds
// (GDK-1655, the GDK-1192 rule).
func RefreshAgile(ctx context.Context, cfg *config.Config, db *store.DB) error {
	c, err := origin.Client(cfg)
	if err != nil {
		return err
	}
	importAgile(ctx, c, db, Options{})
	return nil
}

// SprintIssueKeys are the mirrored keys sitting in one sprint — what a state
// change has to re-read so their sprint_state column stops lying.
func SprintIssueKeys(ctx context.Context, db *store.DB, sprintID int64) ([]string, error) {
	return db.KeysInSprint(ctx, sprintID)
}
