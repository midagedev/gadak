package sync

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// ConfluenceSourceID is the slug the wiki connector owns in sources / sync_state.
const ConfluenceSourceID = "confluence"

// confluenceOverlap covers CQL's minute granularity on lastModified.
const confluenceOverlap = 5 * time.Minute

// pageBatchSize is how many PageRecords are committed per store transaction.
const pageBatchSize = 50

// RunConfluence does one Confluence mirror pass: full or incremental.
// Attachments and changelog are out of scope for R1. Every successful pass
// prunes pages whose space is outside the current config/listing scope.
// Incremental floors are per-space (spaces.watermark); a failed space does
// not move its own watermark or any other space's. A floor selects candidates,
// it does not decide fetches: pageFetchGate does, and an incremental tick over
// an unchanged space reads zero page bodies.
//
// A failure leaves already-committed batches in place.
func RunConfluence(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) (Result, error) {
	var c *confluence.Client
	return runSource(ctx, cfg, db, opts,
		sourceIdent{ID: ConfluenceSourceID, Kind: "confluence"},
		// Space-scope prune is the Confluence reconcile. The flag only suffixes
		// SyncRun.Kind; prune itself is called from runConfluencePass.
		true,
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
			// An empty config means "the team's wiki", not "every space I can
			// see": Cloud gives each user a personal space, so an unfiltered
			// listing is mostly ~accountid noise that also blows up CQL URLs.
			// Personal spaces stay reachable by naming them in config.spaces
			// (path ② upserts those). Upserting them here just so prune can
			// delete them would bump version every Watch cycle.
			if s.Type != "global" {
				continue
			}
			row := store.SpaceRow{Key: s.Key, Name: s.Name, Kind: s.Type}
			if s.Homepage != nil {
				row.HomepageID = s.Homepage.ID
			}
			spaceRows = append(spaceRows, row)
			spaces = append(spaces, s.Key)
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
				// A bad/restricted key is skippable; a rejected credential is
				// not — continuing would 401 again on SearchPages.
				if IsRejectedCredential(err) {
					return record(ctx, db, ConfluenceSourceID, err)
				}
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

	wms, err := db.ConfluenceSpaceWatermarks(ctx, ConfluenceSourceID)
	if err != nil {
		return err
	}

	var maxUTC, maxRaw string
	// batchSpaces: path ② (config listed spaces) collects names from page hits;
	// also a harmless refresh when path ① already wrote spaces from Spaces().
	// Watermarks are committed per space after that space finishes — never
	// from a mid-space page batch (a later space's failure must not inherit
	// this space's floor, and this space's floor must not move until its
	// fetch completed).
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
		if opts.Progress != nil {
			opts.Progress(res.Fetched, res.Changed)
		}
		opts.logf("  confluence: %s pages", formatCount(res.Fetched))
		return nil
	}

	processHits := func(spaceMaxUTC, spaceMaxRaw *string, gate *pageFetchGate) func([]confluence.Page) error {
		return func(hits []confluence.Page) error {
			batch := make([]store.PageRecord, 0, pageBatchSize)
			spaceByKey := map[string]store.SpaceRow{}
			for _, hit := range hits {
				if !gate.needsBody(hit) {
					// The mirror already holds this page at this exact version and
					// stamp: the hit is inside cqlTime's floor window, not a change.
					// Its stamp still counts toward the watermark — we have just
					// verified the mirror is current through it — and the page stays
					// OUT of the gate's fetched set so the comments-only pass can
					// still reach it if only a comment moved.
					res.PageSkips++
					noteStamp(hit.Version.When, spaceMaxUTC, spaceMaxRaw)
					noteStamp(hit.Version.When, &maxUTC, &maxRaw)
					continue
				}
				gate.markFetched(hit.ID)
				res.PageBodies++
				rec, spaceName, when, err := fetchPageRecord(ctx, c, hit)
				if errors.Is(err, confluence.ErrNotFound) {
					// Deleted or view-restricted between the listing and the fetch —
					// routine on a busy site; the next successful prune removes any
					// stale mirror row that has left scope.
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
				noteStamp(when, spaceMaxUTC, spaceMaxRaw)
				noteStamp(when, &maxUTC, &maxRaw)
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
	}

	syncSpace := func(key string, backfill bool) error {
		var cql, floorLabel string
		if backfill {
			floorLabel = "full-backfill"
			cql = fmt.Sprintf(`space=%s AND type=page order by lastmodified asc`, cqlSpace(key))
		} else {
			floorLabel = wms[key]
			cql = fmt.Sprintf(`space=%s AND type=page AND lastModified >= "%s" order by lastmodified asc`,
				cqlSpace(key), cqlTime(wms[key]))
		}
		bodiesBefore, skipsBefore := res.PageBodies, res.PageSkips
		var spaceMaxUTC, spaceMaxRaw string
		// One gate per space, built from the mirror's own rows. Backfill gets a
		// nil gate: a full/backfill pass is the mirror's repair path and must
		// re-read every body regardless of what the local rows claim.
		gate, err := newPageFetchGate(ctx, db, key, backfill)
		if err != nil {
			return err
		}
		if err := c.SearchPages(ctx, cql, processHits(&spaceMaxUTC, &spaceMaxRaw, gate)); err != nil {
			return record(ctx, db, ConfluenceSourceID, err)
		}
		opts.logf("confluence: space %s floor=%s fetched=%d unchanged=%d", key, floorLabel,
			res.PageBodies-bodiesBefore, res.PageSkips-skipsBefore)

		// comments-only pass: one type=comment CQL per incremental space.
		// Pages already fetched above are skipped. Full/backfill already
		// loaded every page's comments, so they skip this pass.
		if !backfill {
			if err := commentsOnlyPass(ctx, c, opts, key, wms[key], gate,
				&spaceMaxUTC, &spaceMaxRaw, &maxUTC, &maxRaw,
				processHits(&spaceMaxUTC, &spaceMaxRaw, gate)); err != nil {
				return record(ctx, db, ConfluenceSourceID, err)
			}
		}

		if spaceMaxRaw == "" {
			return nil
		}
		if err := db.SetSpaceWatermark(ctx, ConfluenceSourceID, key, spaceMaxRaw); err != nil {
			return err
		}
		wms[key] = spaceMaxRaw
		// Compatibility: sync_state.watermark stays the max across spaces so
		// status/doctor/freshness keep working. It is not an incremental floor.
		if err := db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{Watermark: spaceMaxRaw}); err != nil {
			return err
		}
		return nil
	}

	if res.Full {
		for _, key := range spaces {
			if err := syncSpace(key, true); err != nil {
				return err
			}
		}
	} else {
		for _, key := range spaces {
			if err := syncSpace(key, wms[key] == ""); err != nil {
				return err
			}
		}
	}

	pruned, err := db.PruneConfluenceSpaces(ctx, ConfluenceSourceID, spaces)
	if err != nil {
		return err
	}
	res.Deleted += pruned
	opts.logf("confluence: pruned %d out-of-scope pages (kept: %s)", pruned, strings.Join(spaces, ", "))

	res.Watermark = maxRaw
	if err := db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{Watermark: maxRaw, FullSync: res.Full}); err != nil {
		return err
	}
	return nil
}

// pageFetchGate is the single owner of "does this search hit need a body
// fetch?" for one space pass. Both paths that can pull a page body — the
// type=page pass and the comments-only pass — go through it, so the
// already-mirrored decision, the already-fetched-this-tick decision and the
// tally live in one place instead of being spread over the two callers.
//
// Why it exists (GDK-113): cqlTime renders the floor at minute granularity and
// subtracts confluenceOverlap, so every page modified within that window of the
// space watermark comes back as a hit on every tick — forever, because the
// watermark can never advance past the newest page. Without this gate the pass
// spent a GET /content/{id} plus its comment paging on each of them, which was
// 19.4 s of a measured 21.4 s tick. Narrowing the window cannot close that (a
// zero overlap still returns the whole minute); only asking the mirror can.
//
// A nil gate always says yes. That is the backfill/full case: a full pass is
// the mirror's repair path and must re-read every body regardless of what the
// local rows claim.
type pageFetchGate struct {
	// have is the mirror's version stamp per source page id, loaded once per
	// space. Absent means unknown, which always means fetch.
	have map[string]store.PageStamp
	// fetched is every page id this pass has already pulled (or attempted) a
	// body for. The comments-only pass consults it so a page touched by both
	// passes costs one GET, not two.
	fetched map[string]struct{}
}

// newPageFetchGate loads one space's mirrored stamps. backfill returns a nil
// gate — see the type comment.
func newPageFetchGate(ctx context.Context, db *store.DB, spaceKey string, backfill bool) (*pageFetchGate, error) {
	if backfill {
		return nil, nil
	}
	have, err := db.PageStamps(ctx, ConfluenceSourceID, spaceKey)
	if err != nil {
		return nil, err
	}
	return &pageFetchGate{have: have, fetched: map[string]struct{}{}}, nil
}

// needsBody reports whether hit's body must be pulled. It says no only when the
// mirror holds that page at exactly the hit's version number *and* the hit's
// lastModified: a number alone can be reused after a restore, and a stamp alone
// is minute-coarse in CQL. Anything missing, zero or different is fetched —
// the mirror is a disposable cache, so the safe answer is always "fetch".
func (g *pageFetchGate) needsBody(hit confluence.Page) bool {
	if g == nil || hit.ID == "" {
		return true
	}
	if _, done := g.fetched[hit.ID]; done {
		return false
	}
	if hit.Version.Number <= 0 || hit.Version.When == "" {
		return true
	}
	st, ok := g.have[hit.ID]
	if !ok || st.Version != hit.Version.Number || st.UpdatedAt == "" {
		return true
	}
	return st.UpdatedAt != jira.ISOTime(hit.Version.When)
}

// markFetched records that this pass pulled (or tried to pull) id's body.
// A nil gate keeps no state: backfill fetches everything exactly once anyway,
// and it runs no comments-only pass to dedupe against.
func (g *pageFetchGate) markFetched(id string) {
	if g == nil || id == "" {
		return
	}
	g.fetched[id] = struct{}{}
}

// alreadyFetched reports whether this pass has already pulled id's body. It is
// how the comments-only pass avoids a second GET — and, just as importantly,
// why a gate-skipped page is NOT in the set: comments do not bump a page's
// version, so the comments-only pass must stay able to reach it.
func (g *pageFetchGate) alreadyFetched(id string) bool {
	if g == nil {
		return false
	}
	_, ok := g.fetched[id]
	return ok
}

// noteStamp folds one source lastModified into a running max pair (UTC-normalised
// for comparison, raw for storage). Empty is ignored.
func noteStamp(when string, maxUTC, maxRaw *string) {
	if when == "" {
		return
	}
	iso := jira.ISOTime(when)
	if iso > *maxUTC {
		*maxUTC, *maxRaw = iso, when
	}
}

// fetchPageRecord loads full body + comments for a search hit and maps to store.
// Comments are always re-fetched even when the page version is unchanged
// (comments do not bump page version — the comments-only trap). For a page the
// fetch gate skipped this is not reached at all; commentsOnlyPass is what keeps
// that page's comments current.
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

// commentsOnlyPass finds pages whose comments changed without a body edit.
// Cost cap: one CQL per incremental space (type=comment + lastModified floor).
// Hits are resolved to container page IDs; pages the page pass already fetched
// are skipped via the gate. Never a full-space refetch (that would undo C4).
//
// A page the gate skipped is deliberately *not* "already fetched": its body is
// unchanged but its comments may not be, and this pass is the only path left
// to them.
//
// Decision 0006 described a comments-only pass on a global watermark. The
// floor is now per-space (spaces.watermark) so a failed space cannot inherit
// another space's comment cursor. Watermark advances to max(page, comment)
// lastModified so a quiet wiki does not rescan every comment since the last
// body edit on every Watch tick.
func commentsOnlyPass(
	ctx context.Context,
	c *confluence.Client,
	opts Options,
	spaceKey, watermark string,
	gate *pageFetchGate,
	spaceMaxUTC, spaceMaxRaw, maxUTC, maxRaw *string,
	fetch func([]confluence.Page) error,
) error {
	cql := fmt.Sprintf(`space=%s AND type=comment AND lastModified >= "%s" order by lastmodified asc`,
		cqlSpace(spaceKey), cqlTime(watermark))
	var commentHits int
	need := map[string]struct{}{}
	err := c.SearchPages(ctx, cql, func(hits []confluence.Page) error {
		for _, hit := range hits {
			commentHits++
			noteStamp(hit.Version.When, spaceMaxUTC, spaceMaxRaw)
			noteStamp(hit.Version.When, maxUTC, maxRaw)
			pid, err := resolveCommentContainer(ctx, c, hit)
			if err != nil {
				opts.logf("confluence: comments-only skip %s: %v", hit.ID, err)
				continue
			}
			if pid == "" {
				opts.logf("confluence: comments-only skip %s (no container page)", hit.ID)
				continue
			}
			if gate.alreadyFetched(pid) {
				continue
			}
			need[pid] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return err
	}
	pages := make([]confluence.Page, 0, len(need))
	for id := range need {
		pages = append(pages, confluence.Page{ID: id})
	}
	if len(pages) > 0 {
		if err := fetch(pages); err != nil {
			return err
		}
	}
	opts.logf("confluence: comments-only space %s pages=%d comment_hits=%d",
		spaceKey, len(pages), commentHits)
	return nil
}

// reCommentPageID / reCommentWebUIPage extract the container page from a
// comment search hit's webui. store.reWikiPage requires /wiki/spaces/ which
// Cloud webui often omits (/spaces/KEY/pages/ID or pageId=).
var (
	reCommentPageID    = regexp.MustCompile(`(?i)pageId=(\d+)`)
	reCommentWebUIPage = regexp.MustCompile(`/pages/(\d+)`)
)

func commentContainerFromWebUI(webui string) string {
	if webui == "" {
		return ""
	}
	if m := reCommentPageID.FindStringSubmatch(webui); len(m) == 2 {
		return m[1]
	}
	if m := reCommentWebUIPage.FindStringSubmatch(webui); len(m) == 2 {
		return m[1]
	}
	return ""
}

// resolveCommentContainer maps a type=comment search hit to its page id.
// Prefer _links.webui (no extra GET). Fallback: GET the comment and take
// the first ancestor (page; replies list the page first, then the parent
// comment).
func resolveCommentContainer(ctx context.Context, c *confluence.Client, hit confluence.Page) (string, error) {
	if hit.Type == "page" && hit.ID != "" {
		return hit.ID, nil
	}
	if id := commentContainerFromWebUI(hit.Links.WebUI); id != "" {
		return id, nil
	}
	if hit.ID == "" {
		return "", nil
	}
	full, err := c.Page(ctx, hit.ID)
	if err != nil {
		return "", err
	}
	if n := len(full.Ancestors); n > 0 {
		return full.Ancestors[0].ID, nil
	}
	return "", nil
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

// cqlTime renders watermark minus overlap as CQL lastModified accepts:
// "2006-01-02 15:04". Stored watermark remains the source's raw ISO stamp.
func cqlTime(watermark string) string {
	t, err := time.Parse(jira.Layout, watermark)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, watermark); err != nil {
			if t, err = time.Parse(config.ISOMilli, watermark); err != nil {
				t = time.Now().Add(-24 * time.Hour)
			}
		}
	}
	return t.Add(-confluenceOverlap).Format("2006-01-02 15:04")
}
