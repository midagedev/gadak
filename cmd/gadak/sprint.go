package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/retro"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

const sprintUsage = `usage: gadak sprint <subcommand>

  gadak sprint list                       sprints in the mirror, active first (id, state, board, issues, name, goal)
  gadak sprint show <sprint-id>           the sprint's daily burn-up (scope, started, done)
  gadak sprint add <sprint-id> <KEY>...   put issues into a sprint
  gadak sprint remove <KEY>...            take issues back to the backlog
  gadak sprint create <board-id> <name>   open a future sprint
  gadak sprint start <sprint-id>          start it (default 14 days)
  gadak sprint close <sprint-id>          complete it

On a Jira origin these are Jira Software sprints. On a Linear origin they
are cycles — one board per team, and start/close refuse: a cycle begins and
ends by its dates, so edit the cycle's dates in Linear instead. The built-in
tracker serves the same Agile surface as Jira (GDK-1666), so every verb here
works there too.`

func cmdSprint(args []string) error {
	if wantsHelp(args) || len(args) == 0 {
		fmt.Fprintln(os.Stdout, sprintUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return sprintList(rest)
	case "show":
		return sprintShow(rest)
	case "add":
		return sprintAdd(rest)
	case "remove", "rm":
		return sprintRemove(rest)
	case "create":
		return sprintCreate(rest)
	case "start":
		return sprintSetState(rest, "active")
	case "close":
		return sprintSetState(rest, "closed")
	}
	return usageError("sprint", sprintUsage)
}

// sprintList reads the mirror, not the origin — that is the whole point of
// having sprints as rows (GDK-1654).
func sprintList(args []string) error {
	fs := newFlagSet("sprint list")
	asJSON := fs.Bool("json", false, "emit JSON")
	if _, err := parseAround(fs, args); err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()
	rows, err := db.Sprints(ctx)
	if err != nil {
		return err
	}
	{
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(rows)
		}
		if len(rows) == 0 {
			fmt.Fprintln(os.Stdout, "no sprints in the mirror — run `gadak sync`, or this origin has none")
			return nil
		}
		// goal last, and always present: the origin has carried it since
		// sprints became rows and nothing ever showed it (GDK-1695). It is a
		// sentence, so it goes at the end where a long one cannot push the
		// columns around; empty is the honest cell for a sprint with no goal.
		fmt.Fprintln(os.Stdout, "id\tstate\tboard\tissues\tname\tgoal")
		for _, s := range rows {
			fmt.Fprintf(os.Stdout, "%d\t%s\t%d\t%d\t%s\t%s\n", s.ID, s.State, s.BoardID, s.IssueCount, s.Name, s.Goal)
		}
		return nil
	}
}

func sprintBoardFor(c origin.Writer) (origin.SprintBoard, error) {
	return origin.AsSprintBoard(c)
}

func sprintAdd(args []string) error {
	if len(args) < 2 {
		return usageError("sprint", "usage: gadak sprint add <sprint-id> <KEY>...")
	}
	id, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
	if err != nil {
		return usageError("sprint", fmt.Sprintf("sprint id must be a number, got %q — `gadak sprint list` has the ids", args[0]))
	}
	keys := normalizeKeys(args[1:])
	return withKeyWriteSession(keys[0], func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		sb, err := sprintBoardFor(c)
		if err != nil {
			return err
		}
		if err := sb.MoveToSprint(ctx, id, keys); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "%s\tmoved to sprint %d\n", strings.Join(keys, ","), id)
		if err := syncer.RefreshAgile(ctx, cfg, db, src); err != nil {
			return err
		}
		return refreshKeys(ctx, cfg, db, keys, src)
	})
}

func sprintRemove(args []string) error {
	if len(args) < 1 {
		return usageError("sprint", "usage: gadak sprint remove <KEY>...")
	}
	keys := normalizeKeys(args)
	return withKeyWriteSession(keys[0], func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		sb, err := sprintBoardFor(c)
		if err != nil {
			return err
		}
		if err := sb.MoveToBacklog(ctx, keys); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "%s\tmoved to the backlog\n", strings.Join(keys, ","))
		if err := syncer.RefreshAgile(ctx, cfg, db, src); err != nil {
			return err
		}
		return refreshKeys(ctx, cfg, db, keys, src)
	})
}

func sprintCreate(args []string) error {
	fs := newFlagSet("sprint create")
	goal := fs.String("goal", "", "the sprint goal")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return usageError("sprint", "usage: gadak sprint create <board-id> <name> [--goal ...]")
	}
	board, err := strconv.ParseInt(strings.TrimSpace(pos[0]), 10, 64)
	if err != nil {
		return usageError("sprint", fmt.Sprintf("board id must be a number, got %q — `gadak sql \"select id, name from boards\"` has them", pos[0]))
	}
	name := strings.TrimSpace(strings.Join(pos[1:], " "))
	return withCreateSession("", func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		sb, err := sprintBoardFor(c)
		if err != nil {
			return err
		}
		s, err := sb.CreateSprint(ctx, board, name, *goal)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "%d\t%s\t%s\n", s.ID, s.State, s.Name)
		return syncer.RefreshAgile(ctx, cfg, db, src)
	})
}

func sprintSetState(args []string, state string) error {
	fs := newFlagSet("sprint " + state)
	days := fs.Int("days", 14, "sprint length in days (start only)")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("sprint", fmt.Sprintf("usage: gadak sprint %s <sprint-id>", stateVerb(state)))
	}
	id, err := strconv.ParseInt(strings.TrimSpace(pos[0]), 10, 64)
	if err != nil {
		return usageError("sprint", fmt.Sprintf("sprint id must be a number, got %q", pos[0]))
	}
	fields := map[string]any{"state": state}
	if state == "active" {
		// Jira refuses an active sprint with no dates, so the verb supplies
		// them rather than handing the user back the origin's 400.
		now := time.Now().UTC()
		fields["startDate"] = now.Format("2006-01-02T15:04:05.000-0700")
		fields["endDate"] = now.AddDate(0, 0, *days).Format("2006-01-02T15:04:05.000-0700")
	}
	return withCreateSession("", func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		sb, err := sprintBoardFor(c)
		if err != nil {
			return err
		}
		// Re-read the issues while the mirror still says which they are.
		keys, _ := syncer.SprintIssueKeys(ctx, db, id)
		s, err := sb.UpdateSprint(ctx, id, fields)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "%d\t%s\t%s\n", s.ID, s.State, s.Name)
		if err := syncer.RefreshAgile(ctx, cfg, db, src); err != nil {
			return err
		}
		return refreshKeys(ctx, cfg, db, keys, src)
	})
}

func stateVerb(state string) string {
	if state == "active" {
		return "start"
	}
	return "close"
}

func normalizeKeys(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
			if k := normalizeKey(part); k != "" {
				out = append(out, k)
			}
		}
	}
	return out
}

func refreshKeys(ctx context.Context, cfg *config.Config, db *store.DB, keys []string, src string) error {
	for _, k := range keys {
		if err := syncer.RefreshIssue(ctx, cfg, db, k, src); err != nil {
			return err
		}
	}
	return nil
}

// sprintShow prints the sprint's daily burn-up, the same series the board
// strip's sparkline draws (GDK-1710) — CLI-first parity, decisions/0008: the
// verb and the endpoint read one function (store.SprintBurnup) so there is
// no second reconstruction to drift.
//
// An origin that keeps no changelog cannot answer this at all, and the
// judgement has one owner (retro.OriginSuppliesChangelog). Such a sprint
// gets the sentence, not a table of zeros — a flat line reads as "a sprint
// where nothing happened", which is a different and false claim (GDK-1679).
func sprintShow(args []string) error {
	fs := newFlagSet("sprint show")
	asJSON := fs.Bool("json", false, "emit JSON")
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageError("sprint", "usage: gadak sprint show <sprint-id>")
	}
	id, err := strconv.ParseInt(strings.TrimSpace(rest[0]), 10, 64)
	if err != nil || id <= 0 {
		return usageError("sprint", fmt.Sprintf("sprint id must be a number, got %q — `gadak sprint list` has the ids", rest[0]))
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	doc, err := db.SprintBurnup(context.Background(), id, time.Now())
	if errors.Is(err, store.ErrSprintNotFound) {
		return fmt.Errorf("no sprint %d in the mirror — `gadak sprint list` has the ids", id)
	}
	if err != nil {
		return err
	}
	hasHistory := retro.OriginSuppliesChangelog(doc.SourceKind)
	if !hasHistory {
		doc.Days = nil
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"burnup":      doc,
			"has_history": hasHistory,
		})
	}
	fmt.Fprintf(os.Stdout, "sprint %d\t%s\t%s\n", doc.ID, doc.State, doc.Name)
	if !hasHistory {
		fmt.Fprintf(os.Stdout, "no burn-up: this origin (%s) keeps no change history, so the daily scope cannot be reconstructed\n", doc.SourceKind)
		return nil
	}
	if len(doc.Days) == 0 {
		fmt.Fprintln(os.Stdout, "no burn-up: this sprint has no placeable window yet (no start date and no sprint-field history)")
		return nil
	}
	fmt.Fprintln(os.Stdout, "date\tscope\tstarted\tdone")
	for _, d := range doc.Days {
		fmt.Fprintf(os.Stdout, "%s\t%d\t%d\t%d\n", d.Date, d.Scope, d.Started, d.Completed)
	}
	return nil
}
