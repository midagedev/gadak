package sync

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
func RunConfluence(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) (Result, error) {
	var c *confluence.Client
	return runSource(ctx, cfg, db, opts,
		sourceIdent{ID: ConfluenceSourceID, Kind: "confluence"},
		// No reconcile pass yet — kind stays full|incremental (no +reconcile).
		// The label rule lives in runSource; flip this when delete-reconcile ships.
		false,
		"confluence ",
		func() (string, usageTaker, error) {
			if cfg.Confluence == nil {
				return "", nil, errors.New("sync: confluence is not configured")
			}
			c = opts.ConfluenceClient
			if c == nil {
				c = confluence.New(cfg.Site, cfg.Email, cfg.Token)
			}
			return c.BaseURL(), c, nil
		},
		func(state store.SyncState, res *Result) error {
			return runConfluencePass(ctx, c, cfg, db, opts, state, res)
		},
	)
}

// runConfluencePass is the Confluence-specific body inside the shared runSource
// skeleton. Usage flush is registered by runSource on the client from setup.
func runConfluencePass(ctx context.Context, c *confluence.Client, cfg *config.Config, db *store.DB, opts Options, state store.SyncState, res *Result) error {
	spaces := cfg.Confluence.Spaces
	if len(spaces) == 0 {
		listed, err := c.Spaces(ctx)
		if err != nil {
			return record(ctx, db, ConfluenceSourceID, err)
		}
		// Path ①: empty config → Spaces() listing carries key/name/type/homepage.
		var spaceRows []store.SpaceRow
		for _, s := range listed {
			if s.Key == "" {
				continue
			}
			row := store.SpaceRow{Key: s.Key, Name: s.Name, Kind: s.Type}
			if s.Homepage != nil {
				row.HomepageID = s.Homepage.ID
			}
			spaceRows = append(spaceRows, row)
			// An empty config means "the team's wiki", not "every space I can
			// see": Cloud gives each user a personal space, so an unfiltered
			// listing is mostly ~accountid noise that also blows up CQL URLs.
			// Personal spaces stay reachable by naming them in config.spaces.
			if s.Type == "global" {
				spaces = append(spaces, s.Key)
			}
		}
		if err := db.UpsertSpaces(ctx, ConfluenceSourceID, spaceRows); err != nil {
			return err
		}
	} else {
		// Path ②: config lists spaces explicitly — no Spaces() listing, so
		// fetch each space once per run for name/kind/homepage. A bad key or
		// permission error is logged and skipped; the page pass still runs.
		var spaceRows []store.SpaceRow
		for _, key := range spaces {
			if key == "" {
				continue
			}
			s, err := c.Space(ctx, key)
			if err != nil {
				opts.logf("confluence: space %s: %v", key, err)
				continue
			}
			row := store.SpaceRow{Key: s.Key, Name: s.Name, Kind: s.Type}
			if row.Key == "" {
				row.Key = key
			}
			if s.Homepage != nil {
				row.HomepageID = s.Homepage.ID
			}
			spaceRows = append(spaceRows, row)
		}
		if err := db.UpsertSpaces(ctx, ConfluenceSourceID, spaceRows); err != nil {
			return err
		}
	}
	if len(spaces) == 0 {
		opts.logf("confluence: no spaces in scope")
		if err := db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{FullSync: res.Full}); err != nil {
			return err
		}
		return nil
	}

	var maxUTC, maxRaw string
	// Per-batch watermark advance so a crash mid-run resumes. FullSync stamp
	// is applied only on the final RecordSync after the whole pass.
	// batchSpaces: path ② (config listed spaces) collects names from page hits;
	// also a harmless refresh when path ① already wrote spaces from Spaces().
	commitBatch := func(batch []store.PageRecord, batchSpaces []store.SpaceRow) error {
		if len(batch) == 0 {
			return nil
		}
		if len(batchSpaces) > 0 {
			if err := db.UpsertSpaces(ctx, ConfluenceSourceID, batchSpaces); err != nil {
				return err
			}
		}
		changed, err := db.UpsertPages(ctx, batch)
		if err != nil {
			return err
		}
		res.Fetched += len(batch)
		res.Changed += changed
		if maxRaw != "" {
			if err := db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{Watermark: maxRaw}); err != nil {
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
		spaceByKey := map[string]store.SpaceRow{}
		for _, hit := range hits {
			rec, spaceName, when, err := fetchPageRecord(ctx, c, hit)
			if errors.Is(err, confluence.ErrNotFound) {
				// Deleted or view-restricted between the listing and the fetch —
				// routine on a busy site; the reconcile of a later full sync
				// removes any stale mirror row.
				opts.logf("confluence: skip %s (gone: %v)", hit.ID, err)
				continue
			}
			if err != nil {
				return err
			}
			if sk := rec.Page.SpaceKey; sk != "" {
				spaceByKey[sk] = store.SpaceRow{Key: sk, Name: spaceName}
			}
			batch = append(batch, rec)
			if when != "" {
				iso := jira.ISOTime(when)
				if iso > maxUTC {
					maxUTC, maxRaw = iso, when
				}
			}
			if len(batch) >= pageBatchSize {
				if err := commitBatch(batch, spaceRowsFromMap(spaceByKey)); err != nil {
					return err
				}
				batch = batch[:0]
				spaceByKey = map[string]store.SpaceRow{}
			}
		}
		return commitBatch(batch, spaceRowsFromMap(spaceByKey))
	}

	if res.Full {
		for _, key := range spaces {
			cql := fmt.Sprintf(`space=%s AND type=page order by lastmodified asc`, cqlSpace(key))
			opts.logf("confluence full sync: space %s", key)
			if err := c.SearchPages(ctx, cql, processHits); err != nil {
				return record(ctx, db, ConfluenceSourceID, err)
			}
		}
	} else {
		floor := cqlTime(state.Watermark)
		opts.logf("confluence incremental: since %s", floor)
		// Space filter in chunks: quoted keys are ~15+ chars each on real Cloud
		// sites, and one `space in (…)` clause over every visible space blew
		// past URL limits (observed 414 with the CDN in front of *.atlassian.net).
		for _, group := range chunkStrings(spaces, cqlSpaceChunk) {
			cql := fmt.Sprintf(`space in (%s) AND type=page AND lastModified >= "%s" order by lastmodified asc`,
				cqlSpaceList(group), floor)
			if err := c.SearchPages(ctx, cql, processHits); err != nil {
				return record(ctx, db, ConfluenceSourceID, err)
			}
		}
	}

	res.Watermark = maxRaw
	if err := db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{Watermark: maxRaw, FullSync: res.Full}); err != nil {
		return err
	}
	return nil
}

// fetchPageRecord loads full body + comments for a search hit and maps to store.
// Comments are always re-fetched even when the page version is unchanged
// (comments do not bump page version — the comments-only trap).
// spaceName is the human space title from the full page (fallback: search hit).
func fetchPageRecord(ctx context.Context, c *confluence.Client, hit confluence.Page) (store.PageRecord, string, string, error) {
	full, err := c.Page(ctx, hit.ID)
	if err != nil {
		return store.PageRecord{}, "", "", err
	}
	// Prefer full fetch fields; fall back to search hit.
	if full.ID == "" {
		full = hit
	}
	cms, err := c.Comments(ctx, full.ID)
	if errors.Is(err, confluence.ErrNotFound) {
		// The page itself fetched fine but its comment container 404s (seen
		// live: restricted child content). Keep the page, drop the comments.
		cms = nil
	} else if err != nil {
		return store.PageRecord{}, "", "", err
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
	spaceName := full.Space.Name
	if spaceName == "" {
		spaceName = hit.Space.Name
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

	// Labels: first expand page only (≤25); sorted for deterministic store rows.
	labels := full.LabelNames()
	if labels == nil {
		labels = []string{}
	}
	sort.Strings(labels)

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
			Labels:   labels,
			BodyADF:  adf,
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
	return rec, spaceName, when, nil
}

func pageURL(c *confluence.Client, spaceKey, pageID string) string {
	// <site>/wiki/spaces/<KEY>/pages/<id>
	return fmt.Sprintf("%s/spaces/%s/pages/%s", c.BaseURL(), spaceKey, pageID)
}

func spaceRowsFromMap(m map[string]store.SpaceRow) []store.SpaceRow {
	if len(m) == 0 {
		return nil
	}
	out := make([]store.SpaceRow, 0, len(m))
	for _, r := range m {
		out = append(out, r)
	}
	return out
}

// cqlSpace quotes a space key for CQL — always. A bare key that starts with a
// digit (real Cloud sites generate keys like "3dvBrsa61dIo") is a CQL parse
// error, and quoting a key that didn't need it is harmless.
func cqlSpace(key string) string {
	return fmt.Sprintf("%q", key)
}

// cqlSpaceChunk bounds how many space keys ride one `space in (…)` query.
// Keys travel URL-encoded with quotes (~20 bytes each); 15 keeps the whole
// request a few hundred bytes under even conservative proxy URL limits.
const cqlSpaceChunk = 15

func chunkStrings(all []string, size int) [][]string {
	var out [][]string
	for i := 0; i < len(all); i += size {
		end := min(i+size, len(all))
		out = append(out, all[i:end])
	}
	return out
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
