package migrate

// The sprint pass (GDK-1961). Sprints do not fit the export document — the
// issuetap fixture format has no sprint slot — so an export that carried
// issues, comments, links and changelog still left the target's sprints table
// empty and every migrated issue's sprint behind. The Agile write API has
// every verb the graph needs: boards materialize one scrum board per project
// on first listing, POST /sprint creates a future sprint, POST /sprint/{id}
// runs the state machine, POST /sprint/{id}/issue moves members. So the pass
// runs after the seed, through the origin — never a mirror write — and one
// incremental sync afterwards re-mirrors what it wrote, because every move
// and sweep stamps the issue's `updated`, which is exactly the window an
// incremental pass re-reads.
//
// One ordering consequence is unavoidable and reported rather than hidden:
// a closed sprint can only hold done issues on the target. Moves into a
// closed sprint are refused, and closing sweeps every not-done member to the
// backlog. The pass therefore moves members while the sprint is still future
// and transitions afterwards; the not-done members of a closed source sprint
// are swept, counted in Stats.SprintSwept, and the verify table's membership
// row subtracts them so it stays a mismatch detector rather than a
// restatement of the sweep.

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// SprintOrigin is the slice of the Agile write surface the pass needs. The
// product adapter over *jira.Client lives in cmd/gadak; tests bring their
// own. It is deliberately the same shape `gadak sprint` writes through, so
// the pass introduces no second write path to keep in step.
type SprintOrigin interface {
	// BoardForProject returns the board a project's sprints live on. On the
	// built-in tracker exactly one scrum board exists per project and
	// materializes on first listing.
	BoardForProject(ctx context.Context, projectKey string) (int64, error)
	CreateSprint(ctx context.Context, boardID int64, name, goal string) (int64, error)
	// UpdateSprint applies a partial sprint patch; state transitions follow
	// the Agile API's machine (future to active needs both dates, closing
	// sweeps the not-done members).
	UpdateSprint(ctx context.Context, sprintID int64, fields map[string]any) error
	// MoveToSprint moves issues into a sprint that is not closed.
	MoveToSprint(ctx context.Context, sprintID int64, keys []string) error
}

// sprintMember is one migrated issue sitting in the source sprint. done is
// the source mirror's status category — what decides whether the member
// survives the close sweep on the target.
type sprintMember struct {
	key     string
	project string
	done    bool
}

// sprintSpec is one source sprint and the members the migrated set holds.
type sprintSpec struct {
	id    int64
	name  string
	goal  string
	state string
	start string
	end   string
	// members arrive key-ordered from the read; moves keep that order.
	members []sprintMember
}

// label names the sprint in stats lines: id plus name when there is one.
func (sp sprintSpec) label() string {
	if sp.name != "" {
		return fmt.Sprintf("%d %q", sp.id, sp.name)
	}
	return fmt.Sprintf("%d", sp.id)
}

// project picks the sprint's project: the one most of its members belong to
// (ties alphabetical). The boards table cannot answer this — the demo
// mirror's boards.project_key is empty, the Jira Server shape — and a board
// can span projects anyway, while the target's create call needs exactly one.
// Deterministic despite the map walk: the tie-break is total.
func (sp sprintSpec) project() string {
	counts := map[string]int{}
	for _, m := range sp.members {
		if m.project != "" {
			counts[m.project]++
		}
	}
	best, bestN := "", -1
	for p, n := range counts {
		if n > bestN || (n == bestN && p < best) {
			best, bestN = p, n
		}
	}
	return best
}

// MigrateSprints creates every sprint the migrated issues reference on the
// origin behind w and fills it with those issues. db is the source mirror,
// read only; the scope is st.Projects, the same list Build selected. The
// stats st carry the honest half: what was created, what the Agile API's own
// close sweep took, and what could not travel.
//
// Read failures propagate — a query that failed must not read as "no
// sprints" (the GDK-1318 rule). Per-sprint write failures are recorded in
// st.SprintErrors and the pass continues: at this point the workspace exists,
// and a partially carried sprint graph is repairable by hand (`gadak sprint
// add`) while aborting would leave the user no path forward, since migrate
// refuses existing targets.
func MigrateSprints(ctx context.Context, db *sql.DB, w SprintOrigin, st *Stats) error {
	specs, orphans, err := readSprints(ctx, db, st.Projects)
	if err != nil {
		return err
	}
	st.Sprints = len(specs)
	st.SprintOrphans = orphans
	if len(specs) == 0 {
		return nil
	}
	boards := map[string]int64{}
	for _, sp := range specs {
		project := sp.project()
		board, ok := boards[project]
		if !ok {
			b, err := w.BoardForProject(ctx, project)
			if err != nil {
				st.SprintErrors = append(st.SprintErrors,
					fmt.Sprintf("sprint %s: board for project %s: %v", sp.label(), project, err))
				continue
			}
			boards[project], board = b, b
		}
		applySprint(ctx, w, board, sp, st)
	}
	return nil
}

// applySprint carries one sprint: create as future (the only state the API
// creates), keep the members' move ahead of any transition (moves into a
// closed sprint are refused), then walk the state machine — active in one
// call, since the store applies date fields before the transition check;
// closed by activating first, because future to closed is refused. Closing
// sweeps the sprint's not-done members to the backlog; they are counted, not
// silently dropped.
func applySprint(ctx context.Context, w SprintOrigin, board int64, sp sprintSpec, st *Stats) {
	name := sp.name
	if name == "" {
		name = fmt.Sprintf("Sprint %d", sp.id)
	}
	newID, err := w.CreateSprint(ctx, board, name, sp.goal)
	if err != nil {
		st.SprintErrors = append(st.SprintErrors, fmt.Sprintf("sprint %s: create: %v", sp.label(), err))
		return
	}
	st.SprintsCreated++

	transition := sp.state == "active" || sp.state == "closed"
	// Dates first, while the sprint is future and a date patch is a plain
	// set with no transition attached. A sprint about to transition gets its
	// dates in the transition call itself.
	if !transition && (sp.start != "" || sp.end != "") {
		fields := map[string]any{}
		if sp.start != "" {
			fields["startDate"] = sp.start
		}
		if sp.end != "" {
			fields["endDate"] = sp.end
		}
		if err := w.UpdateSprint(ctx, newID, fields); err != nil {
			st.SprintErrors = append(st.SprintErrors, fmt.Sprintf("sprint %s: dates: %v", sp.label(), err))
			return
		}
	}

	keys := make([]string, 0, len(sp.members))
	for _, m := range sp.members {
		keys = append(keys, m.key)
	}
	if len(keys) > 0 {
		if err := w.MoveToSprint(ctx, newID, keys); err != nil {
			st.SprintErrors = append(st.SprintErrors, fmt.Sprintf("sprint %s: move %d issues: %v", sp.label(), len(keys), err))
			return
		}
	}

	if !transition {
		return
	}
	if sp.start == "" || sp.end == "" {
		// The API will not take a sprint from future to active without both
		// dates, so this state cannot travel. The sprint stays future, named
		// and dated as close to the source as it got.
		st.SprintStuck = append(st.SprintStuck, fmt.Sprintf(
			"%s: %s on the source with no start/end dates — the Agile API refuses the transition without both, left future",
			sp.label(), sp.state))
		return
	}
	if err := w.UpdateSprint(ctx, newID, map[string]any{
		"state": "active", "startDate": sp.start, "endDate": sp.end}); err != nil {
		st.SprintErrors = append(st.SprintErrors, fmt.Sprintf("sprint %s: activate: %v", sp.label(), err))
		return
	}
	if sp.state != "closed" {
		return
	}
	if err := w.UpdateSprint(ctx, newID, map[string]any{"state": "closed"}); err != nil {
		st.SprintErrors = append(st.SprintErrors, fmt.Sprintf("sprint %s: close: %v", sp.label(), err))
		return
	}
	for _, m := range sp.members {
		if !m.done {
			st.SprintSwept++
		}
	}
}

// readSprints walks the migrated set's sprint references and joins each to
// its sprints row. A reference with no row — an issue whose sprint the
// mirror's agile listing no longer carries — is an orphan, counted and
// skipped: there is nothing to create on the target from a bare id.
func readSprints(ctx context.Context, db *sql.DB, projects []string) ([]sprintSpec, int, error) {
	marks, args := inClause(projects)
	rows, err := db.QueryContext(ctx, `
		SELECT i.sprint_id, i.key, i.project_key, COALESCE(i.status_category,''),
		       s.id, COALESCE(s.name,''), COALESCE(s.goal,''), COALESCE(s.state,''),
		       COALESCE(s.start_at,''), COALESCE(s.end_at,'')
		FROM issues_full i LEFT JOIN sprints s ON s.id = i.sprint_id
		WHERE i.project_key IN (`+marks+`) AND i.sprint_id IS NOT NULL
		ORDER BY i.key`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	byID := map[int64]*sprintSpec{}
	orphans := 0
	for rows.Next() {
		var sprintID int64
		var category string
		var m sprintMember
		var row sql.NullInt64
		var name, goal, state, start, end string
		if err := rows.Scan(&sprintID, &m.key, &m.project, &category, &row,
			&name, &goal, &state, &start, &end); err != nil {
			return nil, 0, err
		}
		if !row.Valid {
			orphans++
			continue
		}
		m.done = category == "done"
		sp, ok := byID[sprintID]
		if !ok {
			sp = &sprintSpec{id: sprintID, name: name, goal: goal, state: state, start: start, end: end}
			byID[sprintID] = sp
		}
		sp.members = append(sp.members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	ids := make([]int64, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	specs := make([]sprintSpec, 0, len(ids))
	for _, id := range ids {
		specs = append(specs, *byID[id])
	}
	return specs, orphans, nil
}
