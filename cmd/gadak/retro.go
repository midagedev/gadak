package main

// gadak retro — the CLI face of the weekly retrospective internal/retro
// computes: flags in, one table (or one JSON document) out, definitions
// printed under the numbers every time. --open follows one cell to the
// issues behind it through the same path `views open --keys` takes, so a
// count is never a dead end (THEORY.md G4: arrangement beats sentences;
// G9: progress is visible where the user goes on purpose).

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/retro"
	"github.com/midagedev/gadak/internal/store"
)

const retroUsageLine = "usage: gadak retro [--since 14d|<N>d|<N>w] [--json] [--open closed|in-progress|mismatch [--week N]] [--no-open]"

// retroDefaultSince is two ISO weeks, enough for a "this week against last
// week" read without paging.
const retroDefaultSince = "14d"

// retroOpenMetrics are the --open values, in help order. Each names a cell
// of the table by its row.
var retroOpenMetrics = []string{"closed", "in-progress", "mismatch"}

// retroBucketKeys is the key set behind one cell.
func retroBucketKeys(b retro.Bucket, metric string) []string {
	switch metric {
	case "closed":
		return b.ClosedKeys
	case "in-progress":
		return b.InProgressKeys
	case "mismatch":
		return b.MismatchKeys
	}
	return nil
}

func cmdRetro(args []string) error {
	fs := newFlagSet("retro")
	sinceFlag := fs.String("since", retroDefaultSince, "how far back the table reaches: 14d, 30d, 4w (1 to 365 days)")
	asJSON := fs.Bool("json", false, "emit the same numbers as one JSON document")
	openFlag := fs.String("open", "", "open the issues behind one cell in the running app: closed|in-progress|mismatch")
	weekFlag := fs.Int("week", 0, "which week --open reads: 0 = the current partial week, 1 = the last full week")
	noOpenFlag := fs.Bool("no-open", false, "with --open: write the hash only; do not open a window")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("retro", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usageError("retro", retroUsageLine)
	}
	since, err := retro.ParseSince(*sinceFlag)
	if err != nil {
		return usageError("retro", err.Error())
	}
	weekSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "week" {
			weekSet = true
		}
	})
	if *openFlag == "" {
		if weekSet {
			return usageError("retro", "--week only applies to --open")
		}
		if *noOpenFlag {
			return usageError("retro", "--no-open only applies to --open")
		}
	} else {
		ok := false
		for _, m := range retroOpenMetrics {
			if *openFlag == m {
				ok = true
				break
			}
		}
		if !ok {
			return usageError("retro", fmt.Sprintf("--open wants %s (got %q)", joinRetroMetrics(), *openFlag))
		}
	}
	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)

	// Self identity comes from the same config the feed uses. Missing config
	// is a warning, not a failure: resume degrades to any-author writes on
	// visited issues and the footer names the branch.
	cfg, cfgErr := config.Load()
	me := store.FeedIdentityOf(cfg)
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not read the workspace config; resume counts any author on visited issues: %v\n", cfgErr)
		me = store.FeedIdentity{}
	}
	rep, err := retro.Compute(context.Background(), db, me, since, time.Now())
	if err != nil {
		return err
	}
	if *openFlag == "" {
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(rep.JSON())
		}
		fmt.Print(rep.Table())
		return nil
	}

	// --open: week 0 is the current partial bucket, 1 the last full week.
	if *weekFlag < 0 || *weekFlag >= len(rep.Buckets) {
		return usageError("retro", fmt.Sprintf("--week %d is out of range 0..%d for --since %s (0 is the current partial week)",
			*weekFlag, len(rep.Buckets)-1, *sinceFlag))
	}
	b := rep.Buckets[len(rep.Buckets)-1-*weekFlag]
	keys := retroBucketKeys(b, *openFlag)
	if len(keys) == 0 {
		fmt.Fprintf(os.Stderr, "retro: %s in week %s has no issues\n", *openFlag, b.Label())
		return nil
	}
	// Same rule as `views open --json`: the document instead of the window.
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(struct {
			Metric string   `json:"metric"`
			Week   int      `json:"week"`
			Keys   []string `json:"keys"`
		}{Metric: *openFlag, Week: *weekFlag, Keys: keys})
	}
	return openKeysView(keys, "", *noOpenFlag, false)
}

// joinRetroMetrics is the --open value list for its usage error.
func joinRetroMetrics() string {
	out := ""
	for i, m := range retroOpenMetrics {
		if i > 0 {
			out += ", "
		}
		out += m
	}
	return out
}
