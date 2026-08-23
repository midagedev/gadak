package apprun

import (
	"context"
	"log"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// StartWatch starts the optional update check and the incremental sync
// loops (primary profile + workspace mounts). Order and log strings match
// the previous inlined cmdServe / desktop ApplicationStarted body.
//
// Watch re-entry is syncer.WatchLoop, not Watch (GDK-663): Watch returns
// on fatal auth and would leave the process alive with sync permanently
// stopped.
//
// noSync skips the watch loops but still starts the update check (serve
// --no-sync). Desktop always passes false.
func (rt *Runtime) StartWatch(ctx context.Context, noSync bool) {
	if rt == nil || rt.API == nil {
		return
	}
	// Optional once-a-day GitHub release check (opt-out via updateCheck: false).
	// Independent of Jira credentials; silent on failure. Always called:
	// StartUpdateCheck records cacheDir even when disabled so CheckNow knows
	// where the file lives (serve already did this; desktop had an extra
	// UpdateCheckEnabled guard that the method already applies internally).
	if dir, err := config.Dir(); err == nil {
		rt.API.StartUpdateCheck(ctx, dir)
	}

	if noSync {
		return
	}

	startWatch := func() {
		go func() {
			// Reload so a late setup does not capture a stale empty config.
			cur, err := config.Load()
			if err != nil {
				log.Printf("sync loop: load config: %v", err)
				return
			}
			phase, progress := rt.API.SyncActivityHooks()
			syncer.WatchLoop(ctx, cur, rt.DB, syncer.Options{
				Log:      rt.log,
				Reload:   config.Load,
				Phase:    phase,
				Progress: progress,
			})
		}()
	}
	if rt.Cfg != nil && rt.Cfg.HasCredential() {
		startWatch()
	} else {
		rt.API.SetSyncStarter(startWatch)
	}
	if rt.Reg != nil {
		watched := rt.Reg.WatchAll(ctx, config.Profile(), rt.log)
		if len(watched) > 0 {
			log.Printf("syncing %d workspace mirrors: %s", len(watched), strings.Join(watched, ", "))
		}
	}
	note("watch")
	rt.stage("apprun: watch started")
}
