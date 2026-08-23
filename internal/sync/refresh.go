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
// A failure is wrapped with ErrMirrorStale so every surface classifies the
// same class with errors.Is. Error() stays the inner re-read sentence
// verbatim (SQLITE_BUSY remains recoverable). handleResync still maps the
// wrapped error through failJira — that path is re-read only, not a landed
// write. CLI/REST write-through sites name the class; this function does
// not log or emit surface copy.
func RefreshIssue(ctx context.Context, cfg *config.Config, db *store.DB, key, src string) error {
	return MirrorStale(refreshIssue(ctx, cfg, db, key, src, refreshPaths{
		linear:     origin.Linear,
		syncLinear: SyncLinearIssue,
		syncIssue:  SyncIssue,
	}))
}

// RefreshPage is the wiki write-through tail: SyncPage, then ErrMirrorStale.
// handlePageResync stays on SyncPage + failJira (re-read only).
func RefreshPage(ctx context.Context, cfg *config.Config, db *store.DB, id string) error {
	return MirrorStale(SyncPage(ctx, cfg, db, id))
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
