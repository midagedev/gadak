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
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/retro"
	"github.com/midagedev/gadak/internal/store"
)

const retroUsageLine = "usage: gadak retro [--since 14d|<N>d|<N>w | --by-sprint [--board <id>]] [--session-gap " + config.DefaultRetroSessionGap + "] [--json] [--explain] [--open closed|in-progress|mismatch|cycle|aging|unplanned|surprises|seen-not-moved|moved-not-seen [--week N]] [--no-open]"

// retroDefaultSince is two ISO weeks, enough for a "this week against last
// week" read without paging.
const retroDefaultSince = "14d"

// retroOpenMetrics are the --open values, in help order. Each names a cell
// of the table by its row.
//
// The last five name the material lists under the table rather than a cell
// of it (materials.go). `aging` is the one that is not per bucket — it is
// measured at now — so --week does not apply to it and saying so is better
// than quietly ignoring the flag.
var retroOpenMetrics = []string{"closed", "in-progress", "mismatch", "cycle",
	"aging", "unplanned", "surprises", "seen-not-moved", "moved-not-seen"}

// retroReportMetrics are the --open values answered by the report rather
// than by one bucket.
var retroReportMetrics = []string{"aging"}

// retroNow is the one clock a retro run reads. A test pins it so that a
// hand query it compares against can bind the same instant instead of
// asking SQLite for a second "now" — two readings of now round to
// different tenths at an x.x5 day boundary (GDK-1594, CI 2026-09-08).
var retroNow = time.Now

// retroBucketKeys is the key set behind one cell.
func retroBucketKeys(b retro.Bucket, metric string) []string {
	switch metric {
	case "closed":
		return b.ClosedKeys
	case "in-progress":
		return b.InProgressKeys
	case "mismatch":
		return b.MismatchKeys
	case "cycle":
		return b.CycleKeys
	case "unplanned":
		return b.Unplanned.Keys
	case "surprises":
		keys := make([]string, 0, len(b.Surprises))
		seen := map[string]bool{}
		for _, s := range b.Surprises {
			if !seen[s.Key] {
				seen[s.Key] = true
				keys = append(keys, s.Key)
			}
		}
		sort.Strings(keys)
		return keys
	case "seen-not-moved":
		return b.SeenNotMoved.Keys
	case "moved-not-seen":
		return b.MovedNotSeen.Keys
	}
	return nil
}

// retroReportKeys is the key set behind a report-level --open name: aging is
// measured at now, not inside a bucket, so it has no week.
func retroReportKeys(rep retro.Report, metric string) ([]string, bool) {
	report := false
	for _, m := range retroReportMetrics {
		if m == metric {
			report = true
			break
		}
	}
	if !report {
		return nil, false
	}
	keys := make([]string, 0, len(rep.Aging.Items))
	for _, it := range rep.Aging.Items {
		keys = append(keys, it.Key)
	}
	sort.Strings(keys)
	return keys, true
}

func cmdRetro(args []string) error {
	fs := newFlagSet("retro")
	sinceFlag := fs.String("since", retroDefaultSince, "how far back the table reaches: 14d, 30d, 4w (1 to 365 days)")
	sessionGapFlag := fs.String("session-gap", config.DefaultRetroSessionGap, "split sessions where the gap to the previous read exceeds this: a Go duration, 5m to 24h ("+config.DefaultRetroSessionGap+", 1h30m; default: retro.sessionGap or "+config.DefaultRetroSessionGap+")")
	bySprintFlag := fs.Bool("by-sprint", false, "one column per sprint instead of per ISO week — the unit a scrum team actually retrospects on")
	boardFlag := fs.Int64("board", 0, "with --by-sprint: which board's sprints are the columns (only needed when more than one board has sprints)")
	asJSON := fs.Bool("json", false, "emit the same numbers as one JSON document")
	explainFlag := fs.Bool("explain", false, "print a paragraph under each section below the table: what it is, why it is here, how to read it")
	openFlag := fs.String("open", "", "open the issues behind one cell or list in the running app: "+joinRetroMetrics())
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
	sinceSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "since" {
			sinceSet = true
		}
	})
	// The two are different bucket sources, and honouring both would silently
	// drop one: --since would name a window the sprints do not fill.
	if *bySprintFlag && sinceSet {
		return usageError("retro", "--since and --by-sprint choose the columns two different ways; pass one")
	}
	if *boardFlag != 0 && !*bySprintFlag {
		return usageError("retro", "--board only applies to --by-sprint")
	}
	// Config is loaded here (once — identity below reuses it) because the
	// session gap's default is config-owned now: an unset flag falls back to
	// retro.sessionGap, a given flag still wins.
	cfg, cfgErr := config.Load()
	gapRaw := *sessionGapFlag
	gapFromFlag := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "session-gap" {
			gapFromFlag = true
		}
	})
	if !gapFromFlag {
		gapRaw = cfg.EffectiveRetroSessionGap()
	}
	sessionGap, err := retro.ParseSessionGap(gapRaw)
	if err != nil {
		if !gapFromFlag {
			err = fmt.Errorf("config retro.sessionGap: %w", err)
		}
		return usageError("retro", err.Error())
	}
	weekSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "week" {
			weekSet = true
		}
	})
	if *explainFlag && *asJSON {
		return usageError("retro", "--explain writes the sections under the table; --json already carries the definitions")
	}
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

	// Self identity comes from the same config the session gap read (loaded
	// above the flag handling). Missing config is a warning, not a failure:
	// resume degrades to any-author writes on visited issues and the footer
	// names the branch.
	me := store.FeedIdentityOf(cfg)
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not read the workspace config; resume counts any author on visited issues: %v\n", cfgErr)
		me = store.FeedIdentity{}
	}
	rep, err := retro.Compute(context.Background(), db, me, since, retroNow(), retro.Options{
		SessionGap: sessionGap,
		BySprint:   *bySprintFlag,
		BoardID:    *boardFlag,
	})
	if err != nil {
		// The two --by-sprint refusals are the reader's problem to fix, not a
		// stack trace: say the sentence the error already carries.
		var amb *retro.ErrAmbiguousBoard
		if errors.Is(err, retro.ErrNoSprints) || errors.As(err, &amb) {
			return usageError("retro", err.Error())
		}
		return err
	}
	if *openFlag == "" {
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(rep.JSON())
		}
		fmt.Print(rep.Table())
		fmt.Print(rep.Sections(*explainFlag))
		return nil
	}

	// A report-level list has no week: --open aging reads now, so a --week
	// beside it is a request the report cannot honour, said rather than
	// dropped.
	if keys, ok := retroReportKeys(rep, *openFlag); ok {
		if weekSet {
			return usageError("retro", fmt.Sprintf("--open %s is measured now, not per %s; drop --week", *openFlag, rep.BucketNoun()))
		}
		if len(keys) == 0 {
			fmt.Fprintf(os.Stderr, "retro: %s has no issues\n", *openFlag)
			return nil
		}
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(struct {
				Metric string   `json:"metric"`
				Keys   []string `json:"keys"`
			}{Metric: *openFlag, Keys: keys})
		}
		return openKeysView(keys, "", *noOpenFlag, false)
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
