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
// cfg's project list scopes the listing the way it scopes the issue pass
// (GDK-2069, measured on a public Jira 10.3 Server with projects=[CALCITE]:
// 265 boards visible, none of them the configured project's, 559 agile
// requests and 115 s per quiet tick): configured projects list boards per
// project and never read the site-wide list; nil — or no non-blank entry —
// is a workspace with no configured projects and keeps the site-wide listing
// with its per-board project backfill (GDK-1665).
func importAgile(ctx context.Context, c *jira.Client, cfg *config.Config, db *store.DB, opts Options) {
	boards, ok := listBoards(ctx, c, cfg, opts)
	if !ok {
		return
	}
	rows := make([]store.BoardRow, 0, len(boards))
	sprints := make([]store.SprintRow, 0, len(boards))
	seen := map[int64]bool{}
	for _, b := range boards {
		rows = append(rows, store.BoardRow{ID: b.ID, Name: b.Name, Type: b.Type, ProjectKey: b.ProjectKey})
		if strings.EqualFold(b.Type, "kanban") {
			// A kanban board has no sprint backing: the ask is a guaranteed
			// 400, one per board per tick (GDK-2069 measured a public server
			// answering exactly that for every kanban board). The row is
			// still stored; only the sprint read is skipped. scrum and
			// simple stay asked — a Cloud team-managed board of type simple
			// can hold sprints.
			continue
		}
		got, err := c.Sprints(ctx, b.ID)
		if err != nil {
			if errors.Is(err, jira.ErrNoAgile) {
				// A board without a sprint backing 400s or 404s; kanban is
				// skipped before the ask, so this guards the rest.
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

// listBoards answers the boards this workspace mirrors, both listing shapes
// behind one call. ok=false ends the pass with the previous rows intact
// (importAgile's header): a failed listing, an origin too old for the Agile
// API, or a site without Jira Software at all.
func listBoards(ctx context.Context, c *jira.Client, cfg *config.Config, opts Options) ([]jira.Board, bool) {
	projects := configuredProjects(cfg)
	if len(projects) == 0 {
		boards, err := c.Boards(ctx)
		if err != nil {
			switch {
			case errors.Is(err, jira.ErrNoAgile):
				// No Jira Software on the site: nothing to mirror, and the
				// previous rows stand.
			case errors.Is(err, jira.ErrAgileUnimplemented):
				logAgileUnimplemented(opts)
			default:
				opts.logf("boards: skipped (%v)", err)
			}
			return nil, false
		}
		backfillBoardProjects(ctx, c, boards, opts)
		return boards, true
	}
	// One listing per configured project (GDK-2069) — the same route the
	// backfill already read (GDK-1665), promoted from key-mapper to listing:
	// the boards of projects the workspace does not configure were the
	// site-wide list's only extra contribution, at one /board/{id}/project
	// read each.
	var (
		boards  []jira.Board
		byID    = map[int64]bool{}
		failed  bool
		noAgile int
	)
	for _, project := range projects {
		scoped, err := c.BoardsForProject(ctx, project)
		if err != nil {
			switch {
			case errors.Is(err, jira.ErrNoAgile):
				// The project has no boards on this site — not a failure.
				noAgile++
				continue
			case errors.Is(err, jira.ErrAgileUnimplemented):
				logAgileUnimplemented(opts)
				return nil, false
			default:
				// ReplaceAgile is wholesale, so a partial union must not
				// reach it: one failed listing keeps every project's rows
				// this pass.
				opts.logf("boards: project %s listing skipped (%v)", project, err)
				failed = true
				continue
			}
		}
		for _, b := range scoped {
			if byID[b.ID] {
				continue
			}
			byID[b.ID] = true
			if b.ProjectKey == "" {
				// Server's scoped listing carries no location (GDK-1665);
				// the project asked for is the answer, and the first listing
				// to return the board wins it.
				b.ProjectKey = project
			}
			boards = append(boards, b)
		}
	}
	if failed {
		return nil, false
	}
	if noAgile == len(projects) {
		// Every listing said "no Agile here": the same answer the site-wide
		// listing gives on a site without Jira Software, so the same rule —
		// the previous rows stand rather than being replaced by nothing.
		return nil, false
	}
	return boards, true
}

// configuredProjects is the workspace's project scope, trimmed and
// blank-dropped: one entry per project the issue pass reads. nil means the
// workspace mirrors the whole site.
func configuredProjects(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	var out []string
	for _, project := range cfg.Projects {
		if project = strings.TrimSpace(project); project != "" {
			out = append(out, project)
		}
	}
	return out
}

// logAgileUnimplemented says a 501 the way GDK-1691 measured it needing to
// be said: whose limitation it is (the serve's, not the mirror's), that the
// issue rows are unaffected, and the one action that closes it. Both listing
// shapes can hit it, so the sentence has one owner.
func logAgileUnimplemented(opts Options) {
	opts.logf("boards: skipped — the origin's server predates sprints (Agile API answered 501); issue rows are unaffected. Upgrade the paired gadak serve, then run `gadak sync --full`")
}

// backfillBoardProjects fills ProjectKey on the boards the listing left
// empty. Cloud's /board carries location.projectKey on every row; Jira
// Server carries none, so without this every boards.project_key on a Server
// workspace is the empty string and the board→project mapping is gone
// (GDK-1665, measured on Jira Software 11.3.11 DC).
//
// Only the unscoped listing reaches here: a workspace with configured
// projects lists per project and that listing names the project itself
// (GDK-2069), so the one-request-per-board read below is the price of a
// whole-site workspace alone.
func backfillBoardProjects(ctx context.Context, c *jira.Client, boards []jira.Board, opts Options) {
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
