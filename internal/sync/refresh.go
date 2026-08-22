package sync

import (
	"context"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// RefreshIssue is the single owner of the write-through tail (CLI, including
// batch and create, and REST mutate / resync / attach). Both surfaces call
// RefreshIssue so the src routing cannot exist on one path and not the other:
// LinearSourceID goes to origin.Linear + SyncLinearIssue; every other src,
// including empty, goes to SyncIssue with default Options.
//
// Errors pass through unchanged. The CLI wraps them as "write applied, but
// the mirror did not refresh"; REST maps them to write_applied_mirror_stale
// (or failJira on resync, which is re-read only). This function does not
// wrap, log, or name those surfaces.
func RefreshIssue(ctx context.Context, cfg *config.Config, db *store.DB, key, src string) error {
	return refreshIssue(ctx, cfg, db, key, src, refreshPaths{
		linear:     origin.Linear,
		syncLinear: SyncLinearIssue,
		syncIssue:  SyncIssue,
	})
}

// refreshPaths are the two origin re-read paths RefreshIssue routes
// between. Tests pass stubs; production wires origin.Linear / SyncLinearIssue
// / SyncIssue.
type refreshPaths struct {
	linear     func(*config.Config) (*linear.Client, error)
	syncLinear func(context.Context, *store.DB, *linear.Client, string) error
	syncIssue  func(context.Context, *config.Config, *store.DB, string, Options) error
}

func refreshIssue(ctx context.Context, cfg *config.Config, db *store.DB, key, src string, paths refreshPaths) error {
	if src == LinearSourceID {
		lc, err := paths.linear(cfg)
		if err != nil {
			return err
		}
		return paths.syncLinear(ctx, db, lc, key)
	}
	return paths.syncIssue(ctx, cfg, db, key, Options{})
}
