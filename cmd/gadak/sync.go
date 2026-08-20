package main

// gadak sync — mirrors Jira into SQLite; Confluence runs when configured.

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/selfupdate"
	syncer "github.com/midagedev/gadak/internal/sync"
)

func cmdSync(args []string) error {
	fs := newFlagSet("sync")
	full := fs.Bool("full", false, "force a full sync")
	watch := fs.Bool("watch", false, "keep syncing on an interval")
	source := fs.String("source", "all", "which source to sync: jira, linear, confluence, or all")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch *source {
	case "jira", "linear", "confluence", "all":
	default:
		return fmt.Errorf("unknown --source %q (want jira, linear, confluence, or all)", *source)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// The credential gate is per-source: `sync --source linear` needs the
	// Linear key, not a Jira token (a Linear-only profile is legitimate).
	if !cfg.HasCredential() && !(cfg.Linear != nil) {
		return config.ErrNotConfigured
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	opts := syncer.Options{Full: *full, Log: func(s string) { log.Print(s) }}
	if *watch {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		// Same reload seam as serve: a `sync --watch` left running all day must
		// notice a settings edit, not mirror yesterday's scope until restarted.
		opts.Reload = config.Load
		return syncer.Watch(ctx, cfg, db, opts)
	}
	runJira := *source == "all" || *source == "jira"
	runLinear := (*source == "all" || *source == "linear") && cfg.Linear != nil
	runConf := (*source == "all" || *source == "confluence") && cfg.Confluence != nil
	if !cfg.HasCredential() {
		// Linear-only profile: the gate above admitted it for its own
		// source; the Jira/Confluence passes still need the Jira token.
		runJira, runConf = false, false
	}
	if *source == "linear" && cfg.Linear == nil {
		return fmt.Errorf("linear is off for this profile — add a \"linear\" block (apiKey, optional teamIds) to config.json")
	}
	if *source == "confluence" && cfg.Confluence == nil {
		return fmt.Errorf("confluence is off for this profile — turn it on with `gadak init --spaces ENG,PROD`\n(or `--spaces all`), or in Settings → Sources")
	}
	if runJira {
		res, err := syncer.Run(context.Background(), cfg, db, opts)
		if err != nil {
			// GDK-485: the stored last_error is already folded (sync.record);
			// the printed line gets the same first sentence.
			return origin.FoldPairedError(cfg, err)
		}
		kind := "incremental"
		if res.Full {
			kind = "full"
		}
		fmt.Printf("%s sync: fetched %d, changed %d, deleted %d, watermark %s\n",
			kind, res.Fetched, res.Changed, res.Deleted, res.Watermark)
	}
	if runLinear {
		lres, err := syncer.RunLinear(context.Background(), cfg, db, opts)
		if err != nil {
			if runJira {
				log.Printf("linear sync failed: %v", err)
			}
			return err
		}
		kind := "incremental"
		if lres.Full {
			kind = "full"
		}
		fmt.Printf("linear %s sync: fetched %d, changed %d, watermark %s\n",
			kind, lres.Fetched, lres.Changed, lres.Watermark)
	}
	if runConf {
		cres, err := syncer.RunConfluence(context.Background(), cfg, db, opts)
		if err != nil {
			if runJira {
				// Jira already succeeded; log confluence failure but still exit non-zero.
				log.Printf("confluence sync failed: %v", err)
			}
			return err
		}
		kind := "incremental"
		if cres.Full {
			kind = "full"
		}
		// bodies/unchanged is the tick's body-read tally: a quiet tick over an
		// unchanged wiki must read 0 bodies. It is printed so "how many bodies
		// did that tick fetch?" is answerable from the command (GDK-113).
		fmt.Printf("confluence %s sync: fetched %d pages (%d bodies read, %d unchanged), changed %d, watermark %s\n",
			kind, cres.Fetched, cres.PageBodies, cres.PageSkips, cres.Changed, cres.Watermark)
	}
	printUpdateNotice(cfg, false)
	return nil
}

// printUpdateNotice prints a one-line brew upgrade hint when a newer release
// is known. withURL adds the release page on a second line (status). Failures
// and opt-out are silent — this is courtesy, not a feature path.
func printUpdateNotice(cfg *config.Config, withURL bool) {
	if cfg == nil || !cfg.UpdateCheckEnabled() {
		return
	}
	dir, err := config.Dir()
	if err != nil {
		return
	}
	info, ok := selfupdate.Check(context.Background(), dir, version, true)
	if !ok || !selfupdate.Newer(version, info.Latest) {
		return
	}
	fmt.Printf("update: v%s available (running v%s) — brew upgrade midagedev/tap/gadak\n",
		info.Latest, version)
	if withURL && info.URL != "" {
		fmt.Println(info.URL)
	}
}
