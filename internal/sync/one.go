package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// ErrNotFound means the upstream answered but no issue/page with that key came
// back: it is outside the credential's permissions, or it was just deleted.
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
		var err error
		c, err = origin.Client(cfg)
		if err != nil {
			return err
		}
	}
	// Write-through can run before the first full sync (standalone create is
	// the case that made this visible). items.source_id references sources.id.
	// Same GDK-241 upgrade purge as runJiraPass: a write-back to an existing
	// issue re-mirrors it under the namespaced id, which UNIQUE(source_id,
	// key) rejects while the pre-namespace row is still in the mirror.
	if cfg.IsStandalone() {
		if _, err := db.PurgeIssueIDsOutsideNamespace(ctx, SourceID, itemNS(cfg)); err != nil {
			return err
		}
	}
	if err := db.UpsertSource(ctx, store.Source{ID: SourceID, Kind: "jira", BaseURL: c.BaseURL()}); err != nil {
		return err
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
		_, err := db.UpsertIssues(ctx, batch)
		return err
	})
	if err != nil {
		return err
	}
	if !found {
		n, delErr := db.DeleteItems(ctx, SourceID, []string{key})
		if delErr != nil {
			return delErr
		}
		opts.logf("sync: tombstone %s (%d rows)", key, n)
		return fmt.Errorf("%s: %w", key, ErrNotFound)
	}
	return nil
}

// SyncPage re-reads one Confluence page and writes it to the mirror. Used when
// the desktop app closes an in-app browser tab after editing a wiki page: the
// rewrite moves synced_at and bumps sync_state.version so the client's next
// pages list / detail fetch sees the edit.
//
// Unlike a full Confluence pass (commitBatch in confluence.go), this must not
// call RecordSync. A single page's lastModified must not become the source
// watermark — that would jump incremental sync into the future and skip every
// page modified in between. UpsertPages alone is enough: it rewrites the row
// and bumps version without advancing the watermark.
func SyncPage(ctx context.Context, cfg *config.Config, db *store.DB, id string) error {
	if !cfg.HasCredential() {
		return errors.New("sync: site, email and token are required")
	}
	c, err := origin.Wiki(cfg)
	if err != nil {
		// SyncPage is wiki-only (desktop page resync). Returning the error
		// does not stop issue sync; Run never calls this.
		return err
	}
	// Same GDK-344 upgrade purge as runConfluencePass: a write-back to an
	// existing page re-mirrors it under the namespaced id, which
	// UNIQUE(source_id, key) rejects while the pre-namespace row is still
	// in the mirror.
	if cfg.IsStandalone() {
		if _, err := db.PurgePageIDsOutsideNamespace(ctx, ConfluenceSourceID, pageNS(cfg)); err != nil {
			return err
		}
	}
	rec, _, _, err := fetchPageRecord(ctx, c, cfg, confluence.Page{ID: id})
	if err != nil {
		if errors.Is(err, confluence.ErrNotFound) {
			// SyncPage has no Options logger; DeleteItems is the count surface.
			if _, delErr := db.DeleteItems(ctx, ConfluenceSourceID, []string{id}); delErr != nil {
				return delErr
			}
			return fmt.Errorf("%s: %w", id, ErrNotFound)
		}
		return err
	}
	_, err = db.UpsertPages(ctx, []store.PageRecord{rec})
	return err
}
