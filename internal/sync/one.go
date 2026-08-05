package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
)

// ErrNotFound means Jira answered but no issue with that key came back: it is
// outside the credential's permissions, or it was just deleted.
var ErrNotFound = errors.New("sync: issue not found upstream")

// SyncIssue re-reads one issue and writes it to the mirror. Every write-through
// endpoint ends here, which is what makes a row that came back from a write
// identical to one a scheduled sync produced — same field mapping, same derived
// fields, no second code path to keep in step.
func SyncIssue(ctx context.Context, cfg *config.Config, db *store.DB, key string, opts Options) error {
	if !cfg.HasCredential() {
		return errors.New("sync: site, email and token are required")
	}
	c := opts.Client
	if c == nil {
		c = jira.New(cfg.Site, cfg.Email, cfg.Token)
	}
	// ponytail: two metadata calls per write. They are what keeps reopen_count and
	// priority_rank identical to a scheduled sync's; give them a TTL cache if
	// writes ever get chatty enough to matter.
	cats, err := c.Statuses(ctx)
	if err != nil {
		return err
	}
	prios, err := c.Priorities(ctx)
	if err != nil {
		return err
	}

	found := false
	err = c.Search(ctx, fmt.Sprintf("key = %q", key), fieldList(cfg, false), true, func(issues []jira.Issue) error {
		// Force rewrites the row even when `updated` looks unchanged. That is the
		// point: the rewrite is what moves synced_at and bumps sync_state.version,
		// so the client's next delta carries the row and its ETag no longer matches.
		batch := store.Batch{Categories: cats, Priorities: prios, Force: true}
		for _, iss := range issues {
			if id := iss.Fields.Status.ID; id != "" {
				if _, ok := cats[id]; !ok {
					cats[id] = jira.Category(iss.Fields.Status.StatusCategory.Key)
				}
			}
			r, err := build(ctx, c, cfg, iss)
			if err != nil {
				return err
			}
			batch.Records = append(batch.Records, r)
			found = true
		}
		_, err := db.UpsertIssues(batch)
		return err
	})
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%s: %w", key, ErrNotFound)
	}
	return nil
}
