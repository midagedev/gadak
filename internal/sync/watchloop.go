package sync

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// WatchRestartPause is how long WatchLoop idles after a return (fatal auth,
// unexpected error) before re-entering. Frozen workspaces skip inside Watch
// itself (GDK-541). Tests may shrink this.
var WatchRestartPause = 30 * time.Second

// watchFn is WatchLoop's inner call. Production is Watch; tests replace it.
// Unexported so callers cannot inject a different Watch.
var watchFn = Watch

// WatchLoop re-enters Watch until ctx is done. A rejected credential still
// ends Watch (no hot-loop on 401); this wait is what spaces retries until
// Reload sees a new credential (GDK-541).
//
// Stop and Reload-failure messages go to opts.Log when set, otherwise to
// log.Printf — MCP stdout is the protocol, so those lines must use mcp.Logf
// via opts.Log, and workspace mounts want a profile prefix on the same hook.
func WatchLoop(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) {
	for ctx.Err() == nil {
		err := watchFn(ctx, cfg, db, opts)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			watchLoopLog(opts, "sync loop stopped: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(WatchRestartPause):
		}
		if opts.Reload != nil {
			next, rerr := opts.Reload()
			if rerr != nil {
				watchLoopLog(opts, "sync loop: load config: %v", rerr)
			} else if next != nil {
				cfg = next
			}
		}
	}
}

func watchLoopLog(opts Options, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if opts.Log != nil {
		opts.Log(msg)
		return
	}
	log.Printf("%s", msg)
}
