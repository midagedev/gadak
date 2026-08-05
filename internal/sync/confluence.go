package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/confluence"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
)

// ConfluenceSourceID is the slug the wiki connector owns in sources / sync_state.
const ConfluenceSourceID = "confluence"

// confluenceOverlap covers CQL's minute granularity on lastModified.
const confluenceOverlap = 5 * time.Minute

// pageBatchSize is how many PageRecords are committed per store transaction.
const pageBatchSize = 50

// RunConfluence does one Confluence mirror pass: full or incremental.
// Attachments, delete reconcile, and changelog are out of scope for R1.
//
// A failure leaves already-committed batches in place and does not advance the
// watermark past the last successful batch.
func RunConfluence(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) (res Result, err error) {
	started := time.Now()
	defer func() {
		if err == nil && !res.Full && res.Changed == 0 && res.Deleted == 0 {
			return
		}
		kind := "incremental"
		if res.Full {
			kind = "full"
		}
		run := store.SyncRun{
			Kind:       kind,
			StartedAt:  started.UTC().Format(time.RFC3339),
			FinishedAt: time.Now().UTC().Format(time.RFC3339),
			Fetched:    res.Fetched,
			Changed:    res.Changed,
			Deleted:    res.Deleted,
		}
		if err != nil {
			run.Error = err.Error()
		}
		_ = db.AppendSyncRun(ConfluenceSourceID, run)
	}()

	if cfg == nil || !cfg.HasCredential() {
		return res, errors.New("sync: site, email and token are required")
	}
	if cfg.Confluence == nil {
		return res, errors.New("sync: confluence is not configured")
	}

	c := opts.ConfluenceClient
	if c == nil {
		c = confluence.New(cfg.Site, cfg.Email, cfg.Token)
	}
	baseURL := c.BaseURL()
	if err := db.UpsertSource(store.Source{ID: ConfluenceSourceID, Kind: "confluence", BaseURL: baseURL}); err != nil {
		return res, err
	}
	state, err := db.SyncState(ConfluenceSourceID)
	if err != nil {
		return res, err
	}
	res.Full = opts.Full || state.Watermark == ""

	spaces := cfg.Confluence.Spaces
	if len(spaces) == 0 {
		listed, err := c.Spaces(ctx)
		if err != nil {
			return res, recordConfluence(db, err)
		}
		for _, s := range listed {
			if s.Key != "" {
				spaces = append(spaces, s.Key)
			}
		}
	}
	if len(spaces) == 0 {
		opts.logf("confluence: no spaces in scope")
		if err := db.RecordSync(ConfluenceSourceID, store.SyncResult{FullSync: res.Full}); err != nil {
			return res, err
		}
		return res, nil
	}

	var maxUTC, maxRaw string
	// Per-batch watermark advance so a crash mid-run resumes. FullSync stamp
	// is applied only on the final RecordSync after the whole pass.
	commitBatch := func(batch []store.PageRecord) error {
		if len(batch) == 0 {
			return nil
		}
		changed, err := db.UpsertPages(batch)
		if err != nil {
			return err
		}
		res.Fetched += len(batch)
		res.Changed += changed
		if maxRaw != "" {
			if err := db.RecordSync(ConfluenceSourceID, store.SyncResult{Watermark: maxRaw}); err != nil {
				return err
			}
		}
		if opts.Progress != nil {
			opts.Progress(res.Fetched, res.Changed)
		}
		opts.logf("  confluence: %s pages", formatCount(res.Fetched))
		return nil
	}

	processHits := func(hits []confluence.Page) error {
		batch := make([]store.PageRecord, 0, pageBatchSize)
		for _, hit := range hits {
			rec, when, err := fetchPageRecord(ctx, c, hit)
			if err != nil {
				return err
			}
			batch = append(batch, rec)
			if when != "" {
				iso := jira.ISOTime(when)
				if iso > maxUTC {
					maxUTC, maxRaw = iso, when
				}
			}
			if len(batch) >= pageBatchSize {
				if err := commitBatch(batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
		}
		return commitBatch(batch)
	}

	if res.Full {
		for _, key := range spaces {
			cql := fmt.Sprintf(`space=%s AND type=page order by lastmodified asc`, cqlSpace(key))
			opts.logf("confluence full sync: space %s", key)
			if err := c.SearchPages(ctx, cql, processHits); err != nil {
				return res, recordConfluence(db, err)
			}
		}
	} else {
		floor := cqlTime(state.Watermark)
		// Space filter: one CQL covering configured spaces, or per-space if many.
		// CQL allows space in (AAA, BBB).
		cql := fmt.Sprintf(`space in (%s) AND type=page AND lastModified >= "%s" order by lastmodified asc`,
			cqlSpaceList(spaces), floor)
		opts.logf("confluence incremental: since %s", floor)
		if err := c.SearchPages(ctx, cql, processHits); err != nil {
			return res, recordConfluence(db, err)
		}
	}

	res.Watermark = maxRaw
	if err := db.RecordSync(ConfluenceSourceID, store.SyncResult{Watermark: maxRaw, FullSync: res.Full}); err != nil {
		return res, err
	}
	elapsed := time.Since(started).Round(time.Second)
	opts.logf("confluence done: %s fetched, %s changed in %s",
		formatCount(res.Fetched), formatCount(res.Changed), elapsed)
	return res, nil
}

func recordConfluence(db *store.DB, err error) error {
	_ = db.RecordSync(ConfluenceSourceID, store.SyncResult{Err: err})
	return err
}

// fetchPageRecord loads full body + comments for a search hit and maps to store.
// Comments are always re-fetched even when the page version is unchanged
// (comments do not bump page version — the comments-only trap).
func fetchPageRecord(ctx context.Context, c *confluence.Client, hit confluence.Page) (store.PageRecord, string, error) {
	full, err := c.Page(ctx, hit.ID)
	if err != nil {
		return store.PageRecord{}, "", err
	}
	// Prefer full fetch fields; fall back to search hit.
	if full.ID == "" {
		full = hit
	}
	cms, err := c.Comments(ctx, full.ID)
	if err != nil {
		return store.PageRecord{}, "", err
	}

	when := full.Version.When
	if when == "" {
		when = hit.Version.When
	}
	iso := jira.ISOTime(when)
	// R1: history.createdDate needs a separate expand; use version.when for both
	// CreatedAt and UpdatedAt until R2+ adds history expansion.
	spaceKey := full.Space.Key
	if spaceKey == "" {
		spaceKey = hit.Space.Key
	}
	parentID := ""
	if n := len(full.Ancestors); n > 0 {
		parentID = full.Ancestors[n-1].ID
	}
	status := full.Status
	if status == "" {
		status = "current"
	}
	ver := full.Version.Number
	if ver <= 0 {
		ver = hit.Version.Number
	}
	if ver <= 0 {
		ver = 1
	}
	title := full.Title
	if title == "" {
		title = hit.Title
	}
	adf := full.Body.ADFRaw()
	bodyText := jira.PlainText(adf)

	item := store.Item{
		ID:         ConfluenceSourceID + ":" + full.ID,
		SourceID:   ConfluenceSourceID,
		Kind:       "page",
		ExternalID: full.ID,
		Key:        full.ID,
		Title:      title,
		BodyText:   bodyText,
		Author:     full.Version.By.DisplayName,
		AuthorID:   full.Version.By.AccountID,
		URL:        pageURL(c, spaceKey, full.ID),
		CreatedAt:  iso,
		UpdatedAt:  iso,
	}

	rec := store.PageRecord{
		Item: item,
		Page: store.Page{
			SpaceKey: spaceKey,
			ParentID: parentID,
			Version:  ver,
			Status:   status,
		},
	}
	for _, cm := range cms {
		cmADF := cm.Body.ADFRaw()
		cmWhen := jira.ISOTime(cm.Version.When)
		rec.Comments = append(rec.Comments, store.Comment{
			ID:         ConfluenceSourceID + ":" + cm.ID,
			ExternalID: cm.ID,
			Author:     cm.Version.By.DisplayName,
			AuthorID:   cm.Version.By.AccountID,
			BodyADF:    cmADF,
			BodyText:   jira.PlainText(cmADF),
			CreatedAt:  cmWhen,
			UpdatedAt:  cmWhen,
		})
	}
	return rec, when, nil
}

func pageURL(c *confluence.Client, spaceKey, pageID string) string {
	// <site>/wiki/spaces/<KEY>/pages/<id>
	return fmt.Sprintf("%s/spaces/%s/pages/%s", c.BaseURL(), spaceKey, pageID)
}

// cqlSpace quotes a space key for CQL (keys are usually bare alphanumerics).
func cqlSpace(key string) string {
	if key == "" {
		return `""`
	}
	// Bare keys are fine; quote when they contain non-identifier chars.
	for _, r := range key {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return fmt.Sprintf("%q", key)
		}
	}
	return key
}

func cqlSpaceList(keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, cqlSpace(k))
	}
	return strings.Join(parts, ", ")
}

// cqlTime renders watermark minus overlap as CQL lastModified accepts:
// "2006-01-02 15:04". Stored watermark remains the source's raw ISO stamp.
func cqlTime(watermark string) string {
	t, err := time.Parse(jira.Layout, watermark)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, watermark); err != nil {
			if t, err = time.Parse("2006-01-02T15:04:05.000Z", watermark); err != nil {
				t = time.Now().Add(-24 * time.Hour)
			}
		}
	}
	return t.Add(-confluenceOverlap).Format("2006-01-02 15:04")
}
