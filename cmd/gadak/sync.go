package main

// gadak sync — mirrors Jira into SQLite; Confluence runs when configured.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/selfupdate"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

func cmdSync(args []string) error {
	fs := newFlagSet("sync")
	full := fs.Bool("full", false, "force a full sync")
	watch := fs.Bool("watch", false, "keep syncing on an interval")
	source := fs.String("source", "all", "which source to sync: jira, linear, confluence, or all")
	ifStale := fs.String("if-stale", "", "sync a source only when its last successful run is older than DUR (e.g. 15m, 1h) or its last run failed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch *source {
	case "jira", "linear", "confluence", "all":
	default:
		return fmt.Errorf("unknown --source %q (want jira, linear, confluence, or all)", *source)
	}
	if *watch && *ifStale != "" {
		return usageError("sync", "--watch and --if-stale cannot be combined")
	}
	var staleEvery time.Duration
	ifStaleRaw := strings.TrimSpace(*ifStale)
	if ifStaleRaw != "" {
		d, err := time.ParseDuration(ifStaleRaw)
		if err != nil || d <= 0 {
			return usageError("sync", fmt.Sprintf("invalid --if-stale %q (want a duration like 15m or 1h)", *ifStale))
		}
		staleEvery = d
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.SyncFrozen() {
		return frozenSyncError()
	}
	// HasCredential is the single configured-or-not owner and now counts a
	// Linear apiKey. A Linear-only profile is legitimate.
	if !cfg.HasCredential() {
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
		return translateFrozen(syncer.Watch(ctx, cfg, db, opts))
	}
	runJira := *source == "all" || *source == "jira"
	runLinear := (*source == "all" || *source == "linear") && cfg.Linear != nil
	runConf := (*source == "all" || *source == "confluence") && cfg.Confluence != nil
	if !cfg.HasAtlassianCredential() {
		// Linear-only profile: HasCredential admitted it; the Jira and
		// Confluence passes still need the Atlassian credential.
		runJira, runConf = false, false
	}
	if *source == "linear" && cfg.Linear == nil {
		return fmt.Errorf("linear is off for this workspace — add a \"linear\" block (apiKey, optional teamIds) to config.json")
	}
	if *source == "confluence" && cfg.Confluence == nil {
		return fmt.Errorf("confluence is off for this workspace — turn it on with `gadak init --spaces ENG,PROD`\n(or `--spaces all`), or in Settings → Sources")
	}
	if staleEvery > 0 {
		now := time.Now()
		ctx := context.Background()
		var fresh []string
		skipFresh := func(id string, run *bool) error {
			if !*run {
				return nil
			}
			st, err := db.SyncState(ctx, id)
			if err != nil {
				return err
			}
			if syncStale(deref(st.SyncedAt, ""), deref(st.LastError, ""), now, staleEvery) {
				return nil
			}
			*run = false
			fresh = append(fresh, id+" synced "+relativeAge(deref(st.SyncedAt, ""), now))
			return nil
		}
		if err := skipFresh(syncer.SourceID, &runJira); err != nil {
			return err
		}
		if err := skipFresh(syncer.LinearSourceID, &runLinear); err != nil {
			return err
		}
		if err := skipFresh(syncer.ConfluenceSourceID, &runConf); err != nil {
			return err
		}
		if !runJira && !runLinear && !runConf && len(fresh) > 0 {
			fmt.Printf("sync: fresh — %s (threshold %s)\n", strings.Join(fresh, ", "), ifStaleRaw)
			return nil
		}
	}
	if runJira {
		res, err := syncer.Run(context.Background(), cfg, db, opts)
		if err != nil {
			// GDK-485: the stored last_error is already folded (sync.record);
			// the printed line gets the same first sentence.
			return translateFrozen(origin.FoldPairedError(cfg, store.WithBusyHint(err)))
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
			return translateFrozen(store.WithBusyHint(err))
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
			return translateFrozen(store.WithBusyHint(err))
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
	printProjectScopeWarnings(cfg, db)
	printUpdateNotice(cfg, false)
	return nil
}

// syncStale reports whether --if-stale should run this source. last_error
// always retries, regardless of age; a missing or unparseable SyncedAt is
// never-synced; a successful run older than threshold is stale. Equal to
// threshold is fresh (the same > comparison warnIfStale uses).
func syncStale(syncedAt, lastErr string, now time.Time, threshold time.Duration) bool {
	if lastErr != "" {
		return true
	}
	if syncedAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, syncedAt)
	if err != nil {
		t, err = time.Parse(config.ISOMilli, syncedAt)
	}
	if err != nil {
		return true
	}
	return now.Sub(t) > threshold
}

// frozenSyncError is the CLI translation of sync.ErrFrozen: cause plus how
// to unfreeze. config.Path is the on-disk file (internal/config/config.go).
func frozenSyncError() error {
	path := "config.json"
	if p, err := config.Path(); err == nil && p != "" {
		path = p
	}
	return fmt.Errorf("this workspace is frozen — nothing goes to the origin, syncs or writes (config \"frozen\": true in %s); unfreeze with `gadak config set frozen false`", path)
}

func translateFrozen(err error) error {
	if errors.Is(err, syncer.ErrFrozen) {
		return frozenSyncError()
	}
	return err
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
