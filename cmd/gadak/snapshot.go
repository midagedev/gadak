package main

// gadak snapshot — writes a shareable mirror copy (no personal tables, no credentials).

import (
	"fmt"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/snapshot"
)

// cmdSnapshot writes a shareable mirror copy (no personal tables, no credentials).
// See specs/000-product/contracts/sync.md "Snapshot generation".
//
// Flags may appear before or after <out.db> (same ergonomics as `gadak sql`).
func cmdSnapshot(args []string) error {
	const snapshotUsage = "usage: gadak snapshot <out.db> [--from db] [--spread 90d] [--scale N] [--seed N] [--now RFC3339] [--force]"
	var from, spread, nowArg string
	scale := 0
	seed := int64(1)
	force := false
	var positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--from" || a == "-from":
			if i+1 >= len(args) {
				return fmt.Errorf("--from needs a path")
			}
			i++
			from = args[i]
		case strings.HasPrefix(a, "--from="):
			from = strings.TrimPrefix(a, "--from=")
		case a == "--spread" || a == "-spread":
			if i+1 >= len(args) {
				return fmt.Errorf("--spread needs a duration")
			}
			i++
			spread = args[i]
		case strings.HasPrefix(a, "--spread="):
			spread = strings.TrimPrefix(a, "--spread=")
		case a == "--scale" || a == "-scale":
			if i+1 >= len(args) {
				return fmt.Errorf("--scale needs an integer")
			}
			i++
			n, err := atoiArg(args[i], "--scale")
			if err != nil {
				return err
			}
			scale = n
		case strings.HasPrefix(a, "--scale="):
			n, err := atoiArg(strings.TrimPrefix(a, "--scale="), "--scale")
			if err != nil {
				return err
			}
			scale = n
		case a == "--seed" || a == "-seed":
			if i+1 >= len(args) {
				return fmt.Errorf("--seed needs an integer")
			}
			i++
			n, err := atoi64Arg(args[i], "--seed")
			if err != nil {
				return err
			}
			seed = n
		case strings.HasPrefix(a, "--seed="):
			n, err := atoi64Arg(strings.TrimPrefix(a, "--seed="), "--seed")
			if err != nil {
				return err
			}
			seed = n
		case a == "--now" || a == "-now":
			if i+1 >= len(args) {
				return fmt.Errorf("--now needs an RFC3339 timestamp")
			}
			i++
			nowArg = args[i]
		case strings.HasPrefix(a, "--now="):
			nowArg = strings.TrimPrefix(a, "--now=")
		case a == "--force" || a == "-force":
			force = true
		case a == "-h" || a == "--help":
			printHelp("snapshot")
			return nil
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %s", a)
		default:
			positionals = append(positionals, a)
		}
	}
	if len(positionals) != 1 {
		return usageError("snapshot", snapshotUsage)
	}
	// --now pins the clock the spread window ends at, so a snapshot can be
	// rebuilt byte-for-byte. Left unset, a snapshot is dated now, which is what
	// keeps a regenerated demo from looking abandoned.
	var now time.Time
	if nowArg != "" {
		t, err := time.Parse(time.RFC3339, nowArg)
		if err != nil {
			return fmt.Errorf("--now %q: want RFC3339 (e.g. 2026-08-05T00:00:00Z)", nowArg)
		}
		now = t
	}
	out := positionals[0]
	src := from
	if src == "" {
		path, err := config.DBPath()
		if err != nil {
			return err
		}
		src = path
	}
	var window time.Duration
	if spread != "" {
		d, err := snapshot.ParseWindow(spread)
		if err != nil {
			return err
		}
		window = d
	}
	res, err := snapshot.Build(snapshot.Options{
		From:   src,
		Out:    out,
		Spread: window,
		Scale:  scale,
		Seed:   seed,
		Force:  force,
		Now:    now,
	})
	if err != nil {
		return err
	}
	extra := ""
	if res.Spread > 0 {
		extra = fmt.Sprintf(", spread %s", res.Spread)
	}
	if scale > 0 {
		extra += fmt.Sprintf(", scale %d", scale)
	}
	fmt.Printf("snapshot %s: %d issues, %d comments, %d changelog%s (%s)\n",
		res.Path, res.Issues, res.Comments, res.Changelog, extra, formatBytes(res.Bytes))
	return nil
}

func atoiArg(s, name string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", name, s)
	}
	return n, nil
}

func atoi64Arg(s, name string) (int64, error) {
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", name, s)
	}
	return n, nil
}

func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
