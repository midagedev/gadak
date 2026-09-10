package sync

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// importAgile fills the boards and sprints tables from the Agile API
// (GDK-1654). A site without Jira Software has no Agile API at all and says
// so once, quietly — that is not a failed sync. As with filters, a failure
// leaves the previous rows: a board-list 500 must not undo an issue pass.
// cfg is read only for its project list, which scopes the board→project
// backfill (GDK-1665); nil is a workspace with no configured projects and
// takes the per-board fallback for every unmapped board.
func importAgile(ctx context.Context, c *jira.Client, cfg *config.Config, db *store.DB, opts Options) {
	boards, err := c.Boards(ctx)
	if err != nil {
		if errors.Is(err, jira.ErrNoAgile) {
			return
		}
		if errors.Is(err, jira.ErrAgileUnimplemented) {
			// GDK-1691: a 501 here is the origin's age, not a failed sync.
			// The sentence must say all three true things: whose limitation
			// it is (the serve's, not the mirror's), that the issue rows are
			// unaffected, and the one action that closes it.
			opts.logf("boards: skipped — the origin's server predates sprints (Agile API answered 501); issue rows are unaffected. Upgrade the paired gadak serve, then run `gadak sync --full`")
			return
		}
		opts.logf("boards: skipped (%v)", err)
		return
	}
	backfillBoardProjects(ctx, c, cfg, boards, opts)
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
			if errors.Is(err, jira.ErrAgileUnimplemented) {
				// The board list answered, so this is the same old-server
				// shape one route deeper (GDK-1691); the remedy line stays
				// on the boards-level log so it is said once, not per board.
				opts.logf("sprints: board %d skipped — the origin's server predates sprints (Agile API answered 501)", b.ID)
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
				// Lowercase so the two sprint_state owners agree by
				// construction: pickSprint lowercases the issue-side
				// projection, and the ReplaceAgile derive copies this
				// column onto the issue rows (GDK-1661). Measured wire is
				// lowercase on both Jiras; this removes the assumption.
				State: strings.ToLower(strings.TrimSpace(s.State)),
				// The origin's own id verbatim (v46): on Jira the integer
				// already is the id, stored as its string.
				ExternalID:  strconv.FormatInt(s.ID, 10),
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
// (GDK-1655, the GDK-1192 rule). The sprint_state derive inside
// ReplaceAgile is the same call, so `gadak sprint start|close` heals the
// issue rows here too — there is no second code path (GDK-1661).
//
// src routes the re-read the way refreshIssue routes an issue re-read
// (GDK-1667): a Linear workspace re-lists cycles, not the Jira Agile API.
// The verb is not forked; the refresh is.
func RefreshAgile(ctx context.Context, cfg *config.Config, db *store.DB, src string) error {
	if src == LinearSourceID {
		c, err := origin.Linear(cfg)
		if err != nil {
			return err
		}
		importLinearCycles(ctx, c, cfg, db, Options{}, time.Now().UTC())
		return nil
	}
	c, err := origin.Client(cfg)
	if err != nil {
		return err
	}
	importAgile(ctx, c, cfg, db, Options{})
	return nil
}

// backfillBoardProjects fills ProjectKey on the boards the listing left
// empty. Cloud's /board carries location.projectKey on every row; Jira
// Server carries none, so without this every boards.project_key on a Server
// workspace is the empty string and the board→project mapping is gone
// (GDK-1665, measured on Jira Software 11.3.11 DC).
//
// Two reads, cheapest first, because importAgile runs every tick
// (GDK-1661): ② one project-scoped listing per *configured* project, which
// maps every board of the projects the workspace actually watches; then ①
// one /board/{id}/project per board still unmapped — boards of projects the
// config does not name, and every board on a workspace configuring none. A
// listing that already answered (Cloud) leaves both loops with nothing to
// do and costs no request.
func backfillBoardProjects(ctx context.Context, c *jira.Client, cfg *config.Config, boards []jira.Board, opts Options) {
	unmapped := func() bool {
		for i := range boards {
			if boards[i].ProjectKey == "" {
				return true
			}
		}
		return false
	}
	if !unmapped() {
		return
	}
	byID := map[int64]int{}
	for i := range boards {
		byID[boards[i].ID] = i
	}
	if cfg != nil {
		for _, project := range cfg.Projects {
			project = strings.TrimSpace(project)
			if project == "" || !unmapped() {
				continue
			}
			scoped, err := c.BoardsForProject(ctx, project)
			if err != nil {
				if !errors.Is(err, jira.ErrNoAgile) {
					opts.logf("boards: project %s listing skipped (%v)", project, err)
				}
				continue
			}
			for _, b := range scoped {
				if i, ok := byID[b.ID]; ok && boards[i].ProjectKey == "" {
					boards[i].ProjectKey = project
				}
			}
		}
	}
	for i := range boards {
		if boards[i].ProjectKey != "" {
			continue
		}
		key, err := c.BoardProject(ctx, boards[i].ID)
		if err != nil {
			if !errors.Is(err, jira.ErrNoAgile) {
				opts.logf("boards: board %d project skipped (%v)", boards[i].ID, err)
			}
			continue
		}
		boards[i].ProjectKey = key
	}
}

// SprintIssueKeys are the mirrored keys sitting in one sprint — what a state
// change has to re-read so their sprint_state column stops lying.
func SprintIssueKeys(ctx context.Context, db *store.DB, sprintID int64) ([]string, error) {
	return db.KeysInSprint(ctx, sprintID)
}
