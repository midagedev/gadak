package main

// gadak recents — the keys this workspace was reading, newest first, from
// local.visits. `gadak issue` appends those rows as it runs
// (recordVisitBestEffort in agent.go); `gadak search` records into
// local.searches (recordSearchBestEffort), which RecentVisits does not read —
// search history and read history are two lists. After a context compaction
// or a fresh session on an old workspace this is the one command that says
// which keys were in play. Same data domain as `gadak export`: openStore, no
// staleness warning — the answer comes from local.db, not from the mirror.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

const recentsUsageLine = "usage: gadak recents [--limit N] [--json]"

func cmdRecents(args []string) error {
	fs := newFlagSet("recents")
	limit := fs.Int("limit", 20, "maximum rows to list")
	asJSON := fs.Bool("json", false, "emit a JSON array of rows")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("recents", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usageError("recents", recentsUsageLine)
	}
	if *limit <= 0 {
		return usageError("recents", "--limit must be 1 or more")
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.RecentVisits(context.Background(), *limit)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(jsonList(rows))
	}
	// Header always, like `gadak sql`: the command's consumer is a person (or
	// agent) reading, not a `cut -f2` pipe — and --json exists for machines.
	fmt.Println("kind\tkey\tviewed_at")
	for _, r := range rows {
		fmt.Printf("%s\t%s\t%s\n", r.Kind, r.Key, r.ViewedAt)
	}
	return nil
}
