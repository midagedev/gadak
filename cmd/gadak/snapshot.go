package main

// gadak snapshot — writes a shareable mirror copy (no personal tables, no credentials).

import (
	"fmt"
	"os"
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
	fs := newFlagSet("snapshot")
	from := fs.String("from", "", "source database path (default: this workspace's mirror)")
	spread := fs.String("spread", "", "restate timestamps across this window, keeping every issue's own order (e.g. 90d)")
	scale := fs.Int("scale", 0, "clone issues onto new keys until the snapshot holds this many")
	seed := fs.Int64("seed", 1, "reserved for --scale determinism")
	nowArg := fs.String("now", "", "pin the clock to an RFC3339 timestamp for reproducible builds")
	force := fs.Bool("force", false, "overwrite out.db if it already exists")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("snapshot", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("snapshot", snapshotUsage)
	}
	// --now pins the clock the spread window ends at, so a snapshot can be
	// rebuilt byte-for-byte. Left unset, a snapshot is dated now, which is what
	// keeps a regenerated demo from looking abandoned.
	var now time.Time
	if *nowArg != "" {
		t, err := time.Parse(time.RFC3339, *nowArg)
		if err != nil {
			return fmt.Errorf("--now %q: want RFC3339 (e.g. 2026-08-05T00:00:00Z)", *nowArg)
		}
		now = t
	}
	out := pos[0]
	src := *from
	if src == "" {
		path, err := config.DBPath()
		if err != nil {
			return err
		}
		src = path
	}
	var window time.Duration
	if *spread != "" {
		d, err := snapshot.ParseWindow(*spread)
		if err != nil {
			return err
		}
		window = d
	}
	res, err := snapshot.Build(snapshot.Options{
		From:   src,
		Out:    out,
		Spread: window,
		Scale:  *scale,
		Seed:   *seed,
		Force:  *force,
		Now:    now,
	})
	if err != nil {
		return err
	}
	extra := ""
	if res.Spread > 0 {
		extra = fmt.Sprintf(", spread %s", res.Spread)
	}
	if *scale > 0 {
		extra += fmt.Sprintf(", scale %d", *scale)
	}
	fmt.Printf("snapshot %s: %d issues, %d comments, %d changelog%s (%s)\n",
		res.Path, res.Issues, res.Comments, res.Changelog, extra, formatBytes(res.Bytes))
	return nil
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
