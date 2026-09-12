package mcp

// gadak_sprint — the daily burn-up for a host without a shell.
//
// GDK-1826, from the leverage-residue axis: v0.22's second theme is "a sprint
// is an object, and a retrospective is a screen", and of that theme's 45 keys
// the MCP surface carried almost none. The retrospective got here already —
// gadak_retro even cuts its columns by sprint — but the series `gadak sprint
// show <id>` prints did not, and gadak_query cannot stand in for it: the
// daily scope/started/completed is a state replay over the changelog, not a
// stored column, exactly as data-model.md keeps time-in-status computed.
//
// Thin on purpose, like gadak_retro next door. store.SprintBurnup is already
// the single owner with two callers — the CLI verb (cmd/gadak/sprint.go) and
// GET /api/v1/sprints/{id}/burnup/ (internal/server/personal.go) — and this
// is the third, not a third reconstruction. The "this origin keeps no change
// history" judgement has one owner too (retro.OriginSuppliesChangelog), and
// the result is shaped like `gadak sprint show --json` so an agent reading
// both surfaces sees one answer.
//
// Why the no-history case returns a sentence and no days: a flat line of
// zeros reads as "a sprint where nothing happened", which is a different and
// false claim (GDK-1679). The CLI made that choice; this surface does not
// get to make a quieter one.

import (
	"context"
	"errors"
	"fmt"

	"github.com/midagedev/gadak/internal/retro"
	"github.com/midagedev/gadak/internal/store"
)

const toolSprint = "gadak_sprint"

// sprintIDsRecourse is spelled once and used by both the description and the
// not-found error. An agent with no shell cannot run `gadak sprint list`, so
// pointing at the CLI the way the CLI does would be advice it cannot take —
// the ids are in the mirror and gadak_query is the door it already has.
const sprintIDsRecourse = "the ids are in the mirror: gadak_query " +
	`"select id, name, state, board_id from sprints order by start_at desc"`

func sprintDescription() string {
	return `One sprint's daily burn-up — the same series ` + "`gadak sprint show <id> --json`" + ` prints.

Scope, started and completed as they stood at each day boundary, reconstructed
from the sprint and status changelog rows. These are not running totals: scope
can dip when work leaves the sprint, and completed can hold while scope moves.

Not a query. The series is computed, not stored, so gadak_query cannot answer
it — ` + sprintIDsRecourse + `.

An origin that keeps no change history (Linear) cannot answer this at all. Such
a sprint comes back with has_history false and no days, rather than a row of
zeros that would read as a sprint where nothing happened.`
}

func sprintToolDefinition() Tool {
	return Tool{
		Name:        toolSprint,
		Description: sprintDescription(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "integer",
					"description": "The sprint id — " + sprintIDsRecourse + ".",
					"minimum":     1,
				},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
	}
}

func (s *Server) toolSprint(args map[string]any) ([]contentItem, error) {
	// callTool has already refused any argument this tool does not publish
	// (GDK-1812) — its InputSchema is the allowlist.
	id := int64(intArg(args, "id", 0))
	if id <= 0 {
		return nil, fmt.Errorf("id must be a sprint id of 1 or more — %s", sprintIDsRecourse)
	}
	doc, err := s.db.SprintBurnup(context.Background(), id, retroNow())
	if errors.Is(err, store.ErrSprintNotFound) {
		return nil, fmt.Errorf("no sprint %d in the mirror — %s", id, sprintIDsRecourse)
	}
	if err != nil {
		return nil, err
	}
	hasHistory := retro.OriginSuppliesChangelog(doc.SourceKind)
	if !hasHistory {
		doc.Days = nil
	}
	return s.marshalResult(map[string]any{"burnup": doc, "has_history": hasHistory})
}
