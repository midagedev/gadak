package mcp

// gadak_recents — "which keys was I just touching", for a host without a
// shell.
//
// The trail these rows come from is written next door in visits.go: every
// gadak_issue and gadak_search call appends the same personal-history row the
// CLI and the UI append. Reading it back had no MCP surface at all, so an
// agent whose context was compacted could write the trail and never walk it
// (GDK-631) — and for a shell-less host a tool is the only form the first
// command after a compaction can take.
//
// Thin on purpose, like gadak_retro: store.DB.RecentVisits is the owner
// `gadak recents` already calls, the query is not restated here, and the one
// argument is the CLI's own flag name. --json has no counterpart: this
// surface answers JSON always.

import (
	"context"
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/store"
)

const toolRecents = "gadak_recents"

// recentsDefaultLimit is `gadak recents`' own --limit default.
const recentsDefaultLimit = 20

// recentsKinds is the kind vocabulary, taken from the store constants that
// RecordVisit validates against rather than retyped: internal/mcp tool
// descriptions are the one surface in this repo with no gate, so a value
// taught here must come from the code that parses it.
var recentsKinds = []string{store.VisitKindIssue, store.VisitKindPage}

func recentsDescription() string {
	return `The keys this workspace was reading, newest first — the same list ` + "`gadak recents`" + ` prints.

Call this first after a context compaction, or when starting on a workspace
you have worked in before: it answers which keys were in play when you do not
remember them and have nothing to search for.

Rows come from this machine's local read history, which gadak_issue and
gadak_search append to as you call them (so does the CLI, and so does the
desktop window). One row per distinct (kind, key), folded to its newest read:
{kind, key, viewed_at}. kind is ` + `"` + strings.Join(recentsKinds, `" | "`) + `"` + `. Keys read under a
previous origin never resurface.

Arguments (optional; the name is the CLI's flag):
- limit: how many rows, 1 or more (default ` + fmt.Sprint(recentsDefaultLimit) + `).

This is your own reading history, not a ranking and not a query: it says
nothing about what changed or what is assigned. Follow a key with gadak_issue,
ask anything countable with gadak_query, or put keys on the human's screen
with gadak_show.

Read-only: it writes nothing, here or at the origin.`
}

func recentsToolDefinition() Tool {
	return Tool{
		Name:        toolRecents,
		Description: recentsDescription(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": fmt.Sprintf("How many rows, newest first (default %d).", recentsDefaultLimit),
					"minimum":     1,
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
	}
}

func (s *Server) toolRecents(args map[string]any) ([]contentItem, error) {
	// callTool has already refused any argument this tool does not publish
	// (GDK-1812) — its InputSchema is the allowlist.
	limit := intArg(args, "limit", recentsDefaultLimit)
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be 1 or more")
	}
	rows, err := s.db.RecentVisits(context.Background(), limit)
	if err != nil {
		return nil, err
	}
	return s.marshalResult(map[string]any{"recents": rows})
}
