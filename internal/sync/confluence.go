package sync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/atlhttp"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// ConfluenceSourceID is the slug the wiki connector owns in sources / sync_state.
const ConfluenceSourceID = "confluence"

// confluenceOverlap covers CQL's minute granularity on lastModified.
const confluenceOverlap = 5 * time.Minute

// pageBatchSize is how many PageRecords are committed per store transaction.
const pageBatchSize = 50

// RunConfluence does one Confluence mirror pass: full or incremental. Page
// attachments ride the shared attachments table (GDK-1541); an origin that
// refuses the listing (issuetap: 501) degrades to "no attachments" for the
// pass. Version-history stamps (page_versions; never bodies) are collected
// when a mirrored page's current version number is not already stored. A
// failed history fetch is logged and does not fail the pass. Every
// successful pass prunes pages whose space is outside the current
// config/listing scope; memory.space always joins that scope (GDK-1079,
// joinMemorySpace). Full and scheduled reconcile passes also compare complete
// page listings with the mirror and delete pages gone within a kept space.
//
// Incremental floors are per-space (spaces.watermark), but the queries are
// chunked: many spaces share one type=page and one type=comment CQL round
// trip, floored at the chunk's oldest member watermark (GDK-1074 — one CQL
// pair per space made a quiet 80-space tick cost 160 sequential round trips,
// 105 s measured). A failed chunk does not move any member's watermark, and no
// member ever advances past history the pass has not enumerated. A floor
// selects candidates, it does not decide fetches: pageFetchGate does, and an
// incremental tick over an unchanged space reads zero page bodies.
//
// A failure leaves already-committed batches in place.
func RunConfluence(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) (Result, error) {
	if err := refuseIfFrozen(cfg); err != nil {
		return Result{}, err
	}
	var c *confluence.Client
	// Acquire the wiki client before runSource so a failure can skip this
	// pass without becoming the caller's error. Issue sync is a different
	// function (Run); returning err here used to look like "the sync failed"
	// even when only the wiki side could not start.
	if opts.ConfluenceClient == nil && cfg != nil && cfg.Confluence != nil {
		w, err := origin.Wiki(cfg)
		if err != nil {
			opts.logf("confluence: skip wiki pass: %v", err)
			return Result{}, nil
		}
		opts.ConfluenceClient = w
	}
	return runSource(ctx, cfg, db, opts,
		sourceIdent{ID: ConfluenceSourceID, Kind: KindConfluence},
		// The Confluence reconcile is space-scope prune plus the page-listing
		// comparison (GDK-1884). The flag only suffixes SyncRun.Kind; both
		// bodies are called from runConfluencePass.
		true,
		"confluence ",
		func() (string, usageTaker, error) {
			if cfg.Confluence == nil {
				return "", nil, errors.New("sync: confluence is not configured")
			}
			c = opts.ConfluenceClient
			if c == nil {
				var err error
				c, err = origin.Wiki(cfg)
				if err != nil {
					// Pre-acquire above already skipped this case. Keep the
					// same skip if a caller reaches here without it.
					opts.logf("confluence: skip wiki pass: %v", err)
					return "", nil, err
				}
			}
			return c.BaseURL(), c, nil
		},
		func(state store.SyncState, res *Result) error {
			return runConfluencePass(ctx, c, cfg, db, opts, state, res)
		},
	)
}

// pageFetch is one pooled fetch's whole product: the mapped page record, the
// space name it carries, its freshest stamp, and the version-history rows the
// worker read alongside the body (nil when none were needed — the same rule
// collectPageVersions applies). gone marks an ErrNotFound body — the page
// left between listing and fetch — and carries that error for the skip log;
// it is not a failure. Any other fetch error comes back as the fetch's error
// proper and fails the pass (fetchOrdered's errgroup rule). commitBatch
// consumes these in listing order.
type pageFetch struct {
	rec       store.PageRecord
	spaceName string
	when      string
	versions  []store.PageVersion
	gone      bool
	goneErr   error
}

// runConfluencePass is the Confluence-specific body inside the shared runSource
// skeleton. Usage flush is registered by runSource on the client from setup.
// The pass's working halves — the pool worker, the serial committer, the
// backfill and chunk walkers, the reconcile scan — are methods on
// confluencePass (GDK-1920), built below once the scope is known; this body
// owns pass-wide setup and the spine that orders the halves.
func runConfluencePass(ctx context.Context, c *confluence.Client, cfg *config.Config, db *store.DB, opts Options, state store.SyncState, res *Result) error {
	// The fetch pool (GDK-1673): the per-item GETs — body, comments, version
	// stamps — fan out over a bounded worker set while the store writes stay
	// serial and in listing order through the single commitBatch below. One
	// Throttle for the whole pass, across backfills and chunks, so AIMD state
	// earned in one chunk holds in the next. Width is the --concurrency knob
	// clamped to [1, MaxFetchConcurrency]; 1 is exactly the pre-1673 serial
	// pass, PauseBetween included.
	thr := newThrottle(clampFetchWidth(FetchConcurrency))
	// The throttle tally covers the whole pass, space listing included — the
	// meter snapshot is taken before any request this pass makes.
	throttleBefore := c.Usage().Throttled
	// PauseBetween is the serial pass's politeness gap; at a wider effective
	// width the requests already overlap and the pause would only stack
	// latency on top. Restored on exit: test clients are shared across passes.
	prevPauseDecide := c.PauseDecide
	c.PauseDecide = func() bool { return thr.Effective() <= 1 }
	defer func() { c.PauseDecide = prevPauseDecide }()
	// Workers log degrade lines from their own goroutines (fetchPageVersions);
	// test Log sinks are plain appends, so worker-side logging goes through
	// this serialized wrapper. Coordinator-side logging stays opts.logf.
	var logMu sync.Mutex
	poolLogf := func(format string, args ...any) {
		logMu.Lock()
		defer logMu.Unlock()
		opts.logf(format, args...)
	}
	// atts is the pass's attachment-listing memory: a 501 from
	// child/attachment stands for the origin, so it is learned once and the
	// rest of the pass skips the request (see attachmentSupport).
	atts := &attachmentSupport{}

	// Upgrade path for GDK-344: built-in wiki mirrors written before the
	// page id namespace existed hold `confluence:N` rows whose keys the pass
	// is about to re-insert as `standalone-confluence:N` — same (source_id,
	// key), different id, which UNIQUE(source_id, key) rejects.
	// A purge deletes rows the per-space watermarks would then skip
	// (GDK-1609) — same wiring as the issue side.
	if cfg.HasBuiltInOrigin() {
		if n, err := db.PurgePageIDsOutsideNamespace(ctx, ConfluenceSourceID, pageNS(cfg)); err != nil {
			return record(ctx, cfg, db, ConfluenceSourceID, err)
		} else if n > 0 {
			res.Full = true
			opts.logf("purged %d pre-namespace built-in pages: one full pass — the watermarks hide what was just deleted (GDK-344, GDK-1609)", n)
		}
	}

	spaces, verifiedSpaces, err := resolveSpaceScope(ctx, c, cfg, db, opts)
	if err != nil {
		return err
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

	// The pass's shared state — the running maxima, the listing guards, the
	// heartbeat — lives on confluencePass so every extracted half reads the
	// same fields it used to capture (GDK-1920).
	p := &confluencePass{
		ctx: ctx, c: c, cfg: cfg, db: db, opts: opts, res: res,
		thr: thr, atts: atts, poolLogf: poolLogf,
		heartbeat:      &progressHeartbeat{db: db, sourceID: ConfluenceSourceID},
		wms:            wms,
		spaces:         spaces,
		verifiedSpaces: verifiedSpaces,
		fullyListed:    map[string]map[string]bool{},
		beforeListings: map[string]map[string]store.PageStamp{},
	}

	if res.Full {
		for _, key := range spaces {
			if err := p.syncBackfill(key); err != nil {
				return err
			}
		}
	} else {
		// Incremental chunks first: keeping the mirrored spaces fresh is cheap
		// and must not wait behind (or be lost to) an expensive or failing
		// backfill of a newly scoped space.
		var backfills []string
		var incremental []string
		for _, key := range spaces {
			if wms[key] == "" {
				backfills = append(backfills, key)
			} else {
				incremental = append(incremental, key)
			}
		}
		for _, chunk := range chunkConfluenceSpaces(incremental, wms) {
			if err := p.syncChunk(chunk); err != nil {
				return err
			}
		}
		for _, key := range backfills {
			if err := p.syncBackfill(key); err != nil {
				return err
			}
		}
	}
	// A scheduled reconcile reads page IDs, not bodies — the fetch gate decides
	// any exception (a page moved between scoped spaces, or changed since the
	// incremental pass). Full/backfill spaces already paid for a complete
	// listing above and need no second request.
	if opts.Reconcile && !res.Full {
		if err := p.reconcileScan(); err != nil {
			return err
		}
	}
	if len(p.fullyListed) > 0 {
		deleted, err := deleteAbsentConfluencePages(ctx, c, db, spaces, p.fullyListed, p.beforeListings, opts.logf)
		if err != nil {
			return record(ctx, cfg, db, ConfluenceSourceID, err)
		}
		res.Deleted += deleted
		if deleted > 0 {
			opts.logf("confluence: reconciled %d deleted pages", deleted)
		}
	}

	pruned, err := db.PruneConfluenceSpaces(ctx, ConfluenceSourceID, spaces)
	if err != nil {
		return err
	}
	res.Deleted += pruned
	if pruned > 0 {
		opts.logf("confluence: pruned %d out-of-scope pages (kept: %s)", pruned, strings.Join(spaces, ", "))
	} else {
		// The kept list is only interesting when something left scope; a quiet
		// tick folds it to one short line (GDK-1074 waste ②).
		opts.logf("confluence: pruned 0 out-of-scope pages (%d spaces in scope)", len(spaces))
	}

	// The fetch pool's one-line summary (GDK-1673): configured width, the
	// lowest effective width AIMD sank to this pass, and — only when the
	// origin throttled — the 429 count. This pass's own line; runSource's
	// done line belongs to the shared skeleton.
	sum := fmt.Sprintf("confluence: concurrency=%d/%d", thr.Configured(), thr.MinEffective())
	if n := c.Usage().Throttled - throttleBefore; n > 0 {
		sum += fmt.Sprintf(" throttled=%d", n)
	}
	opts.logf("%s", sum)

	// The attachment degrade, when it fired: one line per pass, not one per
	// page (issuetap has no child/attachment route yet — the gap is the
	// origin's, reported by the round that measured it).
	if line := atts.summary(); line != "" {
		opts.logf("%s", line)
	}

	res.Watermark = p.maxRaw
	if err := db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{Watermark: p.maxRaw, FullSync: res.Full}); err != nil {
		return err
	}
	return nil
}

// confluencePass is one wiki pass's shared state (GDK-1920): everything the
// extracted halves — the pool worker (fetchOne), the serial committer
// (commitBatch, emit), the backfill and chunk walkers (syncBackfill,
// syncChunk) and the reconcile scan (reconcileScan) — used to reach into
// runConfluencePass through closure captures now has one owner. The maps are
// the pass's timeline, not convenience handles: maxUTC/maxRaw only move
// forward, and fullyListed/beforeListings guard the absence-delete until
// every fetch and search in the pass has succeeded.
type confluencePass struct {
	ctx  context.Context
	c    *confluence.Client
	cfg  *config.Config
	db   *store.DB
	opts Options
	res  *Result

	// thr is the pass-wide AIMD controller: one Throttle across backfills
	// and chunks, so width earned in one chunk holds in the next.
	thr *Throttle
	// atts is the pass's attachment-listing memory: a 501 from
	// child/attachment stands for the origin, so it is learned once and the
	// rest of the pass skips the request (see attachmentSupport).
	atts *attachmentSupport
	// heartbeat is the first-sync progress row handle (GDK-1677), same
	// contract as the issue pass.
	heartbeat *progressHeartbeat
	// poolLogf serializes worker-side logging through opts.logf (test Log
	// sinks are plain appends; workers log from their own goroutines).
	poolLogf func(string, ...any)

	// maxUTC/maxRaw are the newest lastModified observed anywhere in the
	// pass; wms is the per-space incremental floor map, rewritten as chunks
	// and backfills commit their floors.
	maxUTC, maxRaw string
	wms            map[string]string
	spaces         []string
	// verifiedSpaces marks the keys this pass verified against the origin
	// (path ①'s listing, path ②'s resolved GETs, the memory.space join).
	verifiedSpaces map[string]bool
	// A page can be declared absent only after its space's unfiltered listing
	// completed. fullyListed keeps those listings until every fetch/search in
	// the pass has succeeded; a partial run must not turn absence into a
	// tombstone. beforeListings is each space's stamps before its listing —
	// what deleteAbsentConfluencePages compares the survivors against.
	fullyListed    map[string]map[string]bool
	beforeListings map[string]map[string]store.PageStamp
}

// fetchOne is one pool worker's whole item: the serial per-hit fetch set —
// body, comments, then the version-stamp read (GDK-1673 moved that read
// from commitBatch-time into the worker; its write stays behind the
// upsert) — plus this fetch window's AIMD note. A 429 was already waited
// out inside the transport (Retry-After); the meter delta is how the
// worker sees it happened at all.
func (p *confluencePass) fetchOne(ctx context.Context, hit confluence.Page) (pageFetch, error) {
	before := p.c.Usage().Throttled
	var pf pageFetch
	rec, spaceName, when, err := fetchPageRecord(ctx, p.c, p.cfg, hit, p.atts)
	if err == nil {
		pf.versions, err = fetchPageVersions(ctx, p.c, p.db, p.poolLogf, rec.Item.ID, rec.Item.ExternalID, rec.Page.Version)
	}
	if errors.Is(err, confluence.ErrNotFound) {
		// Deleted or view-restricted between listing and fetch — not a
		// failure: emit logs the skip.
		pf.gone, pf.goneErr = true, err
		err = nil
	}
	pf.rec, pf.spaceName, pf.when = rec, spaceName, when
	if p.c.Usage().Throttled > before {
		p.thr.NoteThrottle()
	} else {
		p.thr.NoteClean()
	}
	return pf, err
}

// commitBatch lands one pageBatchSize (or trailing) batch in one store
// transaction window: spaces first, then the items upsert, then version
// stamps. batchSpaces: path ② (config listed spaces) collects names from
// page hits; also a harmless refresh when path ① already wrote spaces from
// Spaces(). Watermarks are committed per chunk after that chunk finishes —
// never from a mid-chunk page batch (a later chunk's failure must not
// inherit this chunk's floor, and this chunk's floors must not move until
// its fetch completed).
func (p *confluencePass) commitBatch(batch []pageFetch, batchSpaces []store.SpaceRow) error {
	if len(batch) == 0 {
		return nil
	}
	if len(batchSpaces) > 0 {
		if err := p.db.UpsertSpaces(p.ctx, ConfluenceSourceID, batchSpaces); err != nil {
			return err
		}
	}
	recs := make([]store.PageRecord, len(batch))
	for i, pf := range batch {
		recs[i] = pf.rec
	}
	changed, err := p.db.UpsertPages(p.ctx, recs)
	if err != nil {
		return err
	}
	// Version stamps write after the items rows exist (page_versions→items
	// FK) — the same spot the serial pass collected them. The pooled worker
	// already did the read; this is only the write, in batch order.
	for _, pf := range batch {
		if len(pf.versions) == 0 {
			continue
		}
		if err := writePageVersions(p.ctx, p.db, p.opts.logf, pf.rec.Item.ID, pf.rec.Item.ExternalID, pf.versions); err != nil {
			return err
		}
	}
	p.res.Fetched += len(batch)
	p.res.Changed += changed
	if p.res.Full {
		// GDK-1677: same heartbeat as the issue pass. No denominator —
		// Confluence exposes no page count per space, and inventing one
		// (an extra CQL count per space) is a request this pass must not
		// spend. total stays NULL; readers omit the "/ total" fragment.
		p.heartbeat.touch(p.ctx, p.opts, p.res.Fetched, -1)
	}
	if p.opts.Progress != nil {
		p.opts.Progress(p.res.Fetched, p.res.Changed)
	}
	p.opts.logf("  confluence: %s pages", formatCount(p.res.Fetched))
	return nil
}

// hitBatch is the serial half of one processHits call: the state the emit
// callback accumulates between hits until the next flush. A struct so emit
// is a method (GDK-1920) instead of a closure over four locals.
type hitBatch struct {
	cp      *chunkPass
	kept    []confluence.Page
	fetches []pageFetch
	// spaces collects spaceKey → row for the batch's commitBatch call.
	spaces map[string]store.SpaceRow
}

// emit is fetchOrdered's committer callback: one fetched page, in listing
// order, on the caller's goroutine. A gone page is logged and marked for
// the absence bookkeeping; a live one joins the batch, flushing at
// pageBatchSize.
func (p *confluencePass) emit(b *hitBatch, i int, pf pageFetch) error {
	if pf.gone {
		// Deleted or view-restricted between the listing and the fetch.
		// A full listing excludes this ID from seen; candidate verification
		// below decides whether its old mirror row can be deleted now.
		b.cp.gone[b.kept[i].ID] = true
		p.opts.logf("confluence: skip %s (gone: %v)", b.kept[i].ID, pf.goneErr)
		return nil
	}
	if sk := pf.rec.Page.SpaceKey; sk != "" {
		b.spaces[sk] = store.SpaceRow{Key: sk, Name: pf.spaceName}
	}
	noteStamp(pf.when, &b.cp.maxUTC, &b.cp.maxRaw)
	noteStamp(pf.when, &p.maxUTC, &p.maxRaw)
	b.fetches = append(b.fetches, pf)
	if len(b.fetches) >= pageBatchSize {
		if err := p.commitBatch(b.fetches, spaceRowsFromMap(b.spaces)); err != nil {
			return err
		}
		b.fetches = b.fetches[:0]
		b.spaces = map[string]store.SpaceRow{}
	}
	return nil
}

// processHits is the gate-side entry both listing paths share. The gate
// pass stays serial: it reads the mirror and decides, per hit, whether a
// fetch is warranted at all. Only the network half fans out.
func (p *confluencePass) processHits(cp *chunkPass) func([]confluence.Page) error {
	return func(hits []confluence.Page) error {
		b := &hitBatch{cp: cp, kept: make([]confluence.Page, 0, len(hits)),
			fetches: make([]pageFetch, 0, pageBatchSize), spaces: map[string]store.SpaceRow{}}
		for _, hit := range hits {
			sk := hit.Space.Key
			gate, ok := cp.gates[sk]
			if !ok && sk == "" && len(cp.gates) == 1 {
				// A server that omits space on hits (single-space query is
				// unambiguous): route to the one member.
				for k, g := range cp.gates {
					sk, gate, ok = k, g, true
				}
			}
			if !ok {
				// A hit outside the queried set (page moved mid-listing, or a
				// server ignoring the space filter): never mirror it into a
				// space this pass does not own.
				p.opts.logf("confluence: skip %s (space %q outside this pass)", hit.ID, sk)
				continue
			}
			need, why := gate.needsBody(hit)
			if !need {
				// The mirror already holds this page at this exact version and
				// stamp: the hit is inside cqlTime's floor window, not a change.
				// Its stamp still counts toward the watermark — we have just
				// verified the mirror is current through it — and the page stays
				// OUT of the gate's fetched set so the comments-only pass can
				// still reach it if only a comment moved.
				//
				// The no-fetch verdicts that carry a reason are the
				// unchecked ones (GDK-1888): a children.attachment or
				// children.comment expansion that came back truncated at the
				// search limit, so the scan could not compare it — the two
				// names join with a + when both were truncated. They ride
				// the same reasons tally as the fetch reasons so the gap is
				// visible in the log.
				if why != "" {
					cp.reasons[why]++
				}
				p.res.PageSkips++
				noteStamp(hit.Version.When, &cp.maxUTC, &cp.maxRaw)
				noteStamp(hit.Version.When, &p.maxUTC, &p.maxRaw)
				continue
			}
			if why != "" {
				if _, isContainer := cp.containers[hit.ID]; isContainer {
					why = "comment-container"
				}
				cp.reasons[why]++
			}
			gate.markFetched(hit.ID)
			p.res.PageBodies++
			cp.bodies[sk]++
			b.kept = append(b.kept, hit)
		}
		// The pooled half: workers run fetchOne (body + comments + version
		// read) concurrently, bounded by thr; emit lands back here in
		// listing order on this goroutine, so the batch, the spaces map,
		// the tallies and every commit still see one writer.
		if err := fetchOrdered(p.ctx, p.thr, b.kept, p.fetchOne, func(i int, pf pageFetch) error {
			return p.emit(b, i, pf)
		}); err != nil {
			return err
		}
		// The trailing sub-batch commit, exactly where the serial pass
		// had it: emit only flushes at pageBatchSize.
		return p.commitBatch(b.fetches, spaceRowsFromMap(b.spaces))
	}
}

// syncBackfill fully re-reads one space: the mirror's repair path (new or
// restored spaces, and every space on --full). The nil gate re-reads every
// body regardless of what the local rows claim; comments arrive with each
// body, so there is no comments-only pass.
func (p *confluencePass) syncBackfill(key string) error {
	cp := newChunkPass(map[string]*pageFetchGate{key: nil})
	if p.verifiedSpaces[key] {
		before, err := p.db.PageStamps(p.ctx, ConfluenceSourceID, key)
		if err != nil {
			return err
		}
		p.beforeListings[key] = before
	}
	cql := fmt.Sprintf(`space=%s AND type=page order by lastmodified asc`, cqlSpace(key))
	before := p.res.PageBodies
	seen := map[string]bool{}
	if err := p.c.SearchPages(p.ctx, cql, func(hits []confluence.Page) error {
		for _, hit := range hits {
			if hit.ID == "" || (hit.Space.Key != "" && hit.Space.Key != key) {
				return fmt.Errorf("confluence: invalid page listing for space %s (page %q, space %q)", key, hit.ID, hit.Space.Key)
			}
			seen[hit.ID] = true
		}
		return p.processHits(cp)(hits)
	}); err != nil {
		return record(p.ctx, p.cfg, p.db, ConfluenceSourceID, err)
	}
	for id := range cp.gone {
		delete(seen, id)
	}
	if p.verifiedSpaces[key] {
		p.fullyListed[key] = seen
	}
	p.opts.logf("confluence: space %s floor=full-backfill fetched=%d", key, p.res.PageBodies-before)
	if cp.maxRaw == "" {
		return nil
	}
	if err := p.db.SetSpaceWatermark(p.ctx, ConfluenceSourceID, key, cp.maxRaw); err != nil {
		return err
	}
	p.wms[key] = cp.maxRaw
	// Compatibility: sync_state.watermark stays the max across spaces so
	// status/doctor/freshness keep working. It is not an incremental floor.
	return p.db.RecordSync(p.ctx, ConfluenceSourceID, store.SyncResult{Watermark: cp.maxRaw})
}

// syncChunk runs one incremental chunk: one type=page CQL, one type=comment
// CQL, floored at the chunk's oldest member watermark. Gates keep re-hits
// inside the widened window from costing body reads.
func (p *confluencePass) syncChunk(chunk spaceChunk) error {
	gates := make(map[string]*pageFetchGate, len(chunk.keys))
	for _, key := range chunk.keys {
		gate, err := newPageFetchGate(p.ctx, p.db, key, false)
		if err != nil {
			return err
		}
		gates[key] = gate
	}
	cp := newChunkPass(gates)
	bodiesBefore, skipsBefore := p.res.PageBodies, p.res.PageSkips
	cql := fmt.Sprintf(`%s AND type=page AND lastModified >= "%s" order by lastmodified asc`,
		cqlSpaceSet(chunk.keys), cqlTime(chunk.floorRaw))
	if err := p.c.SearchPages(p.ctx, cql, p.processHits(cp)); err != nil {
		return record(p.ctx, p.cfg, p.db, ConfluenceSourceID, err)
	}
	// comments-only pass: one type=comment CQL per chunk. Pages already
	// fetched above are skipped via the gates.
	if err := commentsOnlyPass(p.ctx, p.c, p.opts, chunk, cp, &p.maxUTC, &p.maxRaw, p.processHits(cp)); err != nil {
		return record(p.ctx, p.cfg, p.db, ConfluenceSourceID, err)
	}
	p.opts.logf("confluence: %d spaces floor=%s fetched=%d unchanged=%d", len(chunk.keys), chunk.floorRaw,
		p.res.PageBodies-bodiesBefore, p.res.PageSkips-skipsBefore)
	for _, key := range sortedKeys(cp.bodies) {
		p.opts.logf("confluence: space %s fetched=%d", key, cp.bodies[key])
	}
	if len(cp.reasons) > 0 {
		// Why the gate said fetch — the debug surface for "an unchanged tick
		// keeps re-reading the same N bodies" (GDK-1074 waste ①).
		p.opts.logf("confluence: refetch reasons %s", formatReasons(cp.reasons))
	}
	if cp.maxRaw == "" {
		// Nothing observed anywhere in the chunk: floors stay put. The next
		// tick re-runs the same cheap zero-hit queries.
		return nil
	}
	// Every member advances to the chunk max, the quiet ones included: the
	// query enumerated all changes in these spaces since the chunk floor
	// (≤ every member's own floor), so each member is verified current
	// through the newest stamp observed. A quiet member that kept its old
	// floor would drag this chunk's window wider on every future tick.
	for _, key := range chunk.keys {
		if err := p.db.SetSpaceWatermark(p.ctx, ConfluenceSourceID, key, cp.maxRaw); err != nil {
			return err
		}
		p.wms[key] = cp.maxRaw
	}
	// Compatibility: sync_state.watermark stays the max across spaces so
	// status/doctor/freshness keep working. It is not an incremental floor.
	return p.db.RecordSync(p.ctx, ConfluenceSourceID, store.SyncResult{Watermark: cp.maxRaw})
}

// reconcileScan is the scheduled reconcile's listing comparison (GDK-1886,
// GDK-1888): the full page listing of every verified-but-unlisted space,
// compared against the mirror through the same gate the incremental pass
// uses, with the children.* expansions that let needsBody compare
// attachment and comment id sets. It reads IDs, not bodies — the fetch gate
// decides any exception.
func (p *confluencePass) reconcileScan() error {
	var scan []string
	for _, key := range p.spaces {
		if p.verifiedSpaces[key] && p.fullyListed[key] == nil {
			scan = append(scan, key)
		}
	}
	for _, chunk := range chunkConfluenceSpaces(scan, nil) {
		seen := map[string]map[string]bool{}
		// One gate covering every space in the chunk, so the scan's hits
		// go through the same single owner of "does this hit need a body?"
		// as the incremental pass (GDK-1886). The scan used to record the
		// hit and move on; a page moved between two scoped spaces without
		// a version bump then kept its old space_key forever — the
		// incremental CQL floor never lists it, and the prune below kept
		// the row because the confirm GET reported a kept space. With the
		// gate asked, the union's spaceOf answers "filed under another
		// space" (reason "space") and the fetch rewrites space_key through
		// the normal upsert path. Rejected alternative: writing space_key
		// inside deleteAbsentConfluencePages — a write hidden in a prune
		// path. Documented gap, out of scope here: an incremental
		// (CQL lastModified) pass cannot list an unbumped moved page at
		// all — a Full or Reconcile pass is what catches it.
		gate := &pageFetchGate{have: map[string]store.PageStamp{}, spaceOf: map[string]string{}, fetched: map[string]struct{}{},
			attachments: map[string]map[string]bool{}, comments: map[string]map[string]bool{}, commentsUnknown: map[string]bool{}}
		gates := map[string]*pageFetchGate{}
		for _, key := range chunk.keys {
			seen[key] = map[string]bool{}
			gates[key] = gate
			before, err := p.db.PageStamps(p.ctx, ConfluenceSourceID, key)
			if err != nil {
				return err
			}
			p.beforeListings[key] = before
			for id, st := range before {
				gate.have[id] = st
				gate.spaceOf[id] = key
			}
			// GDK-1888: the scan's attachment comparison base — the
			// mirror's per-page attachment id set, one space at a time
			// into the shared gate.
			held, err := p.db.PageAttachmentIDs(p.ctx, ConfluenceSourceID, key)
			if err != nil {
				return err
			}
			for id, set := range held {
				gate.attachments[id] = set
			}
			// GDK-1888 comment half: the same base for top-level comment
			// ids, plus the pages whose comment rows predate parent_id
			// (NULL — unknown). needsBody heals those with one
			// comments-backfill fetch instead of comparing a set it
			// cannot trust.
			tops, unknownParents, err := p.db.PageTopCommentIDs(p.ctx, ConfluenceSourceID, key)
			if err != nil {
				return err
			}
			for id, set := range tops {
				gate.comments[id] = set
			}
			for id := range unknownParents {
				gate.commentsUnknown[id] = true
			}
		}
		cp := newChunkPass(gates)
		// listedSpace remembers the space each hit was listed under, so a
		// page whose body GET came back gone can be pulled out of seen the
		// way syncBackfill does — a mid-scan deletion must not hide behind
		// the listing that predates it.
		listedSpace := map[string]string{}
		cql := fmt.Sprintf(`%s AND type=page order by lastmodified asc`, cqlSpaceSet(chunk.keys))
		// The scan expands children.attachment and children.comment so
		// needsBody can compare the listed ids against the mirror
		// (GDK-1888): an attachment or comment added or removed bumps no
		// page version, so without this every stamp matches and the cache
		// keeps a deleted attachment or comment indefinitely. The
		// expansions ride the scan only — the incremental and full passes
		// keep the default payload.
		if err := p.c.SearchPagesExpand(p.ctx, cql, "version,space,children.attachment,children.comment", func(hits []confluence.Page) error {
			for _, hit := range hits {
				key := hit.Space.Key
				if key == "" && len(chunk.keys) == 1 {
					key = chunk.keys[0]
				}
				if seen[key] == nil {
					return fmt.Errorf("confluence: page %s has space %q outside reconcile scope", hit.ID, key)
				}
				if hit.ID == "" {
					return fmt.Errorf("confluence: page listing in space %s has no id", key)
				}
				seen[key][hit.ID] = true
				listedSpace[hit.ID] = key
			}
			return p.processHits(cp)(hits)
		}); err != nil {
			return record(p.ctx, p.cfg, p.db, ConfluenceSourceID, err)
		}
		for id := range cp.gone {
			if key := listedSpace[id]; key != "" {
				delete(seen[key], id)
			}
		}
		if len(cp.reasons) > 0 {
			// The scan's own tally, same shape as syncChunk's: space= is a
			// move between scoped spaces, new= a page no scanned space had
			// ever held, version/stamp= a change the incremental pass had
			// not yet seen, attachments=/comments= an id set the mirror
			// holds differently — an add or delete that bumped no version
			// (GDK-1888) — comments-backfill= comment rows whose parent_id
			// predates schemaV52, healed by the fetch itself — and
			// attachments-unchecked=/comments-unchecked= (alone or joined
			// with a +) a listing truncated at the search limit, where no
			// comparison was possible.
			p.opts.logf("confluence: reconcile scan refetch reasons %s", formatReasons(cp.reasons))
		}
		for key, ids := range seen {
			p.fullyListed[key] = ids
		}
	}
	return nil
}

// resolveSpaceScope interprets cfg.Confluence.Spaces into the pass's space
// key scope, upserting the space rows the interpretation learned. Path ①
// (empty config) builds the scope from the origin's own Spaces() listing, so
// it cannot name a space the origin does not have; path ② (config lists
// keys) fetches each space once for name/kind/homepage and accounts the keys
// the origin does not have. Either way memory.space joins the scope
// (GDK-1079), and a configured-but-missing key that leaves NOTHING in scope
// fails the pass instead of reporting a zero-page success (GDK-1484).
//
// Errors come back exactly as the inline code returned them: origin failures
// and the fatal scope error already went through record (last_error
// written, watermark untouched), store failures are bare.
func resolveSpaceScope(ctx context.Context, c *confluence.Client, cfg *config.Config, db *store.DB, opts Options) ([]string, map[string]bool, error) {
	spaces := cfg.Confluence.Spaces
	verified := map[string]bool{}
	var configured, resolved int
	var missing []string
	if len(spaces) == 0 {
		listed, err := c.Spaces(ctx)
		if err != nil {
			return nil, nil, record(ctx, cfg, db, ConfluenceSourceID, err)
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
			// GDK-1302: the exclusion is personal, not "anything but global" —
			// Cloud later added team space types (collaboration,
			// knowledge_base) and an allowlist dropped whole team spaces.
			if s.Type == "personal" {
				continue
			}
			row := store.SpaceRow{Key: s.Key, Name: s.Name, Kind: s.Type}
			if s.Homepage != nil {
				row.HomepageID = s.Homepage.ID
			}
			spaceRows = append(spaceRows, row)
			spaces = append(spaces, s.Key)
			verified[s.Key] = true
		}
		if err := db.UpsertSpaces(ctx, ConfluenceSourceID, spaceRows); err != nil {
			return nil, nil, err
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
			configured++
			s, err := c.Space(ctx, key)
			if err != nil {
				// A bad/restricted key is skippable; a rejected credential is
				// not — continuing would 401 again on SearchPages.
				if IsRejectedCredential(err) {
					return nil, nil, record(ctx, cfg, db, ConfluenceSourceID, err)
				}
				opts.logf("confluence: space %s: %v", key, err)
				missing = append(missing, key)
				continue
			}
			resolved++
			verified[key] = true
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
			return nil, nil, err
		}
	}
	// GDK-1079: memory.space joins the pass's scope whichever path built it,
	// and behind path ①'s global filter — a personal memory space must not be
	// dropped by a filter that exists to drop exactly those.
	spaces, joined, err := joinMemorySpace(ctx, c, cfg, opts, spaces)
	if err != nil {
		return nil, nil, record(ctx, cfg, db, ConfluenceSourceID, err)
	}
	if joined != nil {
		verified[joined.Key] = true
		if err := db.UpsertSpaces(ctx, ConfluenceSourceID, []store.SpaceRow{*joined}); err != nil {
			return nil, nil, err
		}
	}
	if len(missing) > 0 {
		inScope := resolved
		if joined != nil {
			// memory.space is a real, resolvable member of the scope
			// (joinMemorySpace) — the pass still has something to mirror.
			inScope++
		}
		head := fmt.Sprintf("confluence: %d of %d configured spaces exist upstream (%s)",
			resolved, configured, strings.Join(missing, ", "))
		if inScope > 0 {
			opts.logf("%s — those keys mirror nothing", head)
		} else {
			opts.logf(`%s — no page mirrored; run `+"`"+`gadak config set confluence.spaces "[]"`+"`"+` to mirror every space the origin has`, head)
			return nil, nil, record(ctx, cfg, db, ConfluenceSourceID, fmt.Errorf(
				`sync: %d of %d configured confluence spaces exist upstream (%s) — no page mirrored; run: gadak config set confluence.spaces "[]"`,
				resolved, configured, strings.Join(missing, ", ")))
		}
	}
	return spaces, verified, nil
}

// confirmVerdict is deleteAbsentConfluencePages's reading of one candidate's
// confirm GET (c.Page). One pure classifier owns the verdict so every status
// has exactly one owner and tests can pin each row (GDK-1887). Before it,
// any confirm error that was not ErrNotFound returned from the whole pass —
// one undecidable candidate (a 400) silently froze every other deletion,
// every pass, forever.
type confirmVerdict int

const (
	confirmCheckPage confirmVerdict = iota // err == nil: apply today's status/space check
	confirmGone                            // the page left this credential's view: delete (guarded)
	confirmKeep                            // this candidate is undecidable this pass: keep the row, continue
	confirmAbort                           // the pass cannot trust any answer: return the error
)

// classifyReconcileConfirm sorts one confirm-GET error. The readings follow
// the fetch path's existing attitude (fetchPageRecord): ErrNotFound is
// "deleted or view-restricted between listing and fetch — not a failure",
// and 410 joins it. A forbidden confirm GET keeps the candidate and lets
// the pass continue: 403 cannot prove deletion. A dead credential
// (IsRejectedCredential), a cancelled context, a throttle (429) or a server
// error (≥500) aborts the pass loudly: those answers say nothing about the
// page, and the next pass retries. Any other 4xx is an answer about this
// request only — the candidate is kept for the next pass and the rest of
// the pass proceeds. Anything unrecognized (network, decode) aborts.
//
// atlhttp.Do retains the status alongside the rejected-credential identity.
// Only this missing-page confirmation treats 403 as undecidable rather than
// fatal; listing, body fetches and writes keep their existing auth policy.
func classifyReconcileConfirm(err error) confirmVerdict {
	switch {
	case err == nil:
		return confirmCheckPage
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return confirmAbort
	case confirmErrStatus(err) == http.StatusForbidden:
		return confirmKeep
	case IsRejectedCredential(err):
		return confirmAbort
	case errors.Is(err, confluence.ErrNotFound):
		return confirmGone
	}
	var apiErr *confluence.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Status == http.StatusGone:
			return confirmGone
		case apiErr.Status == http.StatusTooManyRequests || apiErr.Status >= 500:
			return confirmAbort
		case apiErr.Status >= 400 && apiErr.Status < 500:
			return confirmKeep
		}
	}
	return confirmAbort
}

// confirmErrStatus reports the HTTP status inside a confirm error, 0 when
// there is none. Auth responses carry atlhttp.StatusError; other non-2xx
// responses carry confluence.APIError.
func confirmErrStatus(err error) int {
	var statusErr *atlhttp.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.Status
	}
	var apiErr *confluence.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}

// deleteAbsentConfluencePages compares only spaces whose complete, unfiltered
// page listing succeeded. It considers only rows present before that listing,
// confirms each missing candidate by direct GET (search pagination may omit a
// live page), then conditionally deletes unchanged rows with delta tombstones.
// A candidate whose confirm GET is undecidable (classifyReconcileConfirm →
// confirmKeep) is kept and logged — it costs only itself, not the pass
// (GDK-1887).
func deleteAbsentConfluencePages(ctx context.Context, c *confluence.Client, db *store.DB, scope []string, seen map[string]map[string]bool, before map[string]map[string]store.PageStamp, logf func(string, ...any)) (int, error) {
	guards := map[string]store.PageStamp{}
	kept := map[string]bool{}
	for _, space := range scope {
		kept[space] = true
	}
	spaces := make([]string, 0, len(seen))
	for space := range seen {
		spaces = append(spaces, space)
	}
	sort.Strings(spaces)
	var keptPairs []string
	for _, space := range spaces {
		for id, stamp := range before[space] {
			if !seen[space][id] {
				page, err := c.Page(ctx, id)
				switch classifyReconcileConfirm(err) {
				case confirmAbort:
					return 0, err
				case confirmKeep:
					// An answer about this request, not about the page: keep
					// the row for the next pass and let the other candidates
					// decide.
					keptPairs = append(keptPairs, fmt.Sprintf("%s=%d", id, confirmErrStatus(err)))
					continue
				case confirmGone:
					// Search pagination can omit a still-current page under
					// concurrent edits. A direct read must confirm it really
					// left the live set; ErrNotFound and 410 both mean it
					// left this credential's view.
					guards[id] = stamp
				case confirmCheckPage:
					if (page.Status == "" || page.Status == "current") && (page.Space.Key == "" || kept[page.Space.Key]) {
						continue
					}
					guards[id] = stamp
				}
			}
		}
	}
	if len(keptPairs) > 0 {
		sort.Strings(keptPairs)
		shown := keptPairs
		if len(shown) > 5 {
			shown = append(shown[:5:5], "…")
		}
		logf("confluence: reconcile kept %d candidate(s) it could not confirm: %s", len(keptPairs), strings.Join(shown, ", "))
	}
	return db.DeletePagesIfUnchanged(ctx, ConfluenceSourceID, guards)
}

// joinMemorySpace appends cfg.MemorySpace() to the pass's scope keys when it
// is set and not already a member (case-insensitive, the same tolerance the
// memory verbs apply to space keys), so one setting cannot be quietly voided
// by another: a full pass's prune used to delete the memory pages whenever
// memory.space sat outside confluence.spaces, because `memory add` mirrors
// through RefreshPage, which has no say in the scope (GDK-1079). This is the
// scope's only consumption point, so the guarantee holds no matter when or
// how either setting was written.
//
// The scope is rebuilt from config on every pass, so the join re-fires (and
// its one GET re-costs) every pass — the same per-space GET path ② already
// spends, and zero when memory.space is unset or already inside the scope.
//
// The SpaceRow follows path ②'s attitude: a bad or restricted key is logged
// and its row skipped while the key itself still joins — dropping the key
// would hand the pass's prune exactly the pages this join exists to keep.
// Only a rejected credential fails the pass, as in path ②.
func joinMemorySpace(ctx context.Context, c *confluence.Client, cfg *config.Config, opts Options, keys []string) ([]string, *store.SpaceRow, error) {
	mem := cfg.MemorySpace()
	if mem == "" {
		return keys, nil, nil
	}
	for _, k := range keys {
		if strings.EqualFold(k, mem) {
			return keys, nil, nil
		}
	}
	opts.logf("confluence: memory.space %s joined the sync scope", mem)
	s, err := c.Space(ctx, mem)
	if err != nil {
		if IsRejectedCredential(err) {
			return nil, nil, err
		}
		opts.logf("confluence: space %s: %v", mem, err)
		return append(keys, mem), nil, nil
	}
	// Mirror pages carry the server's canonical key (fetchPageRecord reads
	// full.Space.Key) and prune compares exactly, so the key joins in the
	// canonical form whenever the GET returned one.
	key := s.Key
	if key == "" {
		key = mem
	}
	row := store.SpaceRow{Key: key, Name: s.Name, Kind: s.Type}
	if s.Homepage != nil {
		row.HomepageID = s.Homepage.ID
	}
	return append(keys, key), &row, nil
}

// confluenceChunkSize caps how many spaces share one incremental CQL round
// trip. A var so tests can force chunk boundaries.
var confluenceChunkSize = 25

// confluenceChunkKeyBudget caps the quoted-key characters per chunk so the
// search URL stays comfortably under any proxy/edge limit even with long
// generated space keys.
const confluenceChunkKeyBudget = 1500

// spaceChunk is one incremental CQL round trip's worth of spaces. floorRaw is
// the oldest member watermark — the CQL floor for the whole chunk.
type spaceChunk struct {
	keys     []string
	floorRaw string
}

// chunkConfluenceSpaces groups incremental spaces (all with a watermark) into
// CQL-sized chunks, sorted by floor, newest first, so spaces with nearby
// floors share a chunk. Proximity matters: mixing one dormant floor into an
// active chunk would re-list the active members' history back to that dormant
// floor. The quiet-member advance in syncChunk then converges every member of
// a touched chunk onto the same recent floor.
func chunkConfluenceSpaces(keys []string, wms map[string]string) []spaceChunk {
	sorted := append([]string(nil), keys...)
	sort.SliceStable(sorted, func(i, j int) bool {
		wi, wj := jira.ISOTime(wms[sorted[i]]), jira.ISOTime(wms[sorted[j]])
		if wi != wj {
			return wi > wj
		}
		return sorted[i] < sorted[j]
	})
	var chunks []spaceChunk
	var cur spaceChunk
	budget := 0
	flush := func() {
		if len(cur.keys) > 0 {
			chunks = append(chunks, cur)
			cur, budget = spaceChunk{}, 0
		}
	}
	for _, k := range sorted {
		cost := len(cqlSpace(k)) + 1
		if len(cur.keys) >= confluenceChunkSize || (len(cur.keys) > 0 && budget+cost > confluenceChunkKeyBudget) {
			flush()
		}
		cur.keys = append(cur.keys, k)
		budget += cost
		// Sorted newest-first, so the last member appended holds the minimum.
		cur.floorRaw = wms[k]
	}
	flush()
	return chunks
}

// cqlSpaceSet renders the space filter for a chunk. One space keeps the
// space="KEY" form: issuetap servers older than its space-IN support
// (builtIn wikis inside released binaries, paired home serves) parse only
// that form, and nearly every built-in/paired workspace has exactly one
// space. A multi-space chunk against such a server fails loudly with a CQL
// parse error — never silently.
func cqlSpaceSet(keys []string) string {
	if len(keys) == 1 {
		return "space=" + cqlSpace(keys[0])
	}
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = cqlSpace(k)
	}
	return "space IN (" + strings.Join(quoted, ",") + ")"
}

// chunkPass is one chunk's in-flight state: the per-space gates, the newest
// stamp observed anywhere in the chunk (page or comment), the per-space body
// tally, and why the gate said fetch.
type chunkPass struct {
	gates          map[string]*pageFetchGate
	maxUTC, maxRaw string
	bodies         map[string]int
	reasons        map[string]int
	// containers marks page ids the comments-only pass re-reads for their
	// comments — by construction version-less, so the reason tally names them
	// comment-container instead of miscounting them as unversioned hits.
	containers map[string]struct{}
	gone       map[string]bool
}

func newChunkPass(gates map[string]*pageFetchGate) *chunkPass {
	return &chunkPass{gates: gates, bodies: map[string]int{}, reasons: map[string]int{}, containers: map[string]struct{}{}, gone: map[string]bool{}}
}

func formatReasons(m map[string]int) string {
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
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
	// spaceOf, when set, maps page id → the space the mirror files it under,
	// across every space this gate covers. A gate built for one space leaves
	// it nil: its have map is already that space's stamps, so a page held
	// under another space is simply absent and needsBody answers "new". The
	// reconcile scan builds one gate per chunk covering all its spaces; there
	// absence cannot be told from a move, so the space the row is filed under
	// has to ride along (GDK-1886).
	spaceOf map[string]string
	// haveComments is the mirror's stamp per source comment id. The
	// comments-only pass consults it so a comment hit inside cqlTime's overlap
	// window — already mirrored at exactly this stamp — does not re-read its
	// container page body on every tick.
	haveComments map[string]string
	// fetched is every page id this pass has already pulled (or attempted) a
	// body for. The comments-only pass consults it so a page touched by both
	// passes costs one GET, not two.
	fetched map[string]struct{}
	// attachments is the mirror's per-page attachment id set (page external
	// id → attachment external ids), loaded per space by the reconcile scan
	// only — nil on every other gate, which keeps the attachment question out
	// of the incremental and full passes. With it, needsBody can compare a
	// hit's children.attachment expansion against the mirror: an attachment
	// added or removed bumps no page version, so every stamp check passes and
	// the cache would keep a deleted attachment (and miss a new one)
	// indefinitely (GDK-1888). Every mirrored page of the covered spaces has
	// an entry (empty set = no attachments); a page missing here is one the
	// mirror does not hold — "new" above has already answered it.
	attachments map[string]map[string]bool
	// comments is the same comparison base for top-level comment external ids
	// (page external id → top-level comment external ids, replies excluded),
	// and commentsUnknown holds the pages with any pre-schemaV52 comment row
	// (parent_id NULL — the set cannot be trusted). Both are loaded per space
	// by the reconcile scan only, nil on every other gate. needsBody answers
	// comments-backfill for an unknown page — one fetch rewrites the rows with
	// real parents — and otherwise compares the hit's children.comment
	// expansion (top-level ids only, the shape the search returns) against the
	// set (GDK-1888 comment half).
	comments        map[string]map[string]bool
	commentsUnknown map[string]bool
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
	haveComments, err := db.PageCommentStamps(ctx, ConfluenceSourceID, spaceKey)
	if err != nil {
		return nil, err
	}
	return &pageFetchGate{have: have, haveComments: haveComments, fetched: map[string]struct{}{}}, nil
}

// commentCurrent reports whether the mirror already holds this comment search
// hit at exactly its stamp — the hit is the overlap window echoing, not a new
// or edited comment. Anything missing is not current: the safe answer is
// always "fetch the container".
func (g *pageFetchGate) commentCurrent(hit confluence.Page) bool {
	if g == nil || hit.ID == "" || hit.Version.When == "" {
		return false
	}
	at, ok := g.haveComments[hit.ID]
	return ok && at != "" && at == jira.ISOTime(hit.Version.When)
}

// needsBody reports whether hit's body must be pulled, and — when the gate had
// a say — why. It says no only when the mirror holds that page at exactly the
// hit's version number *and* the hit's lastModified, in the space the hit
// claims (a gate covering several spaces also knows which one it files the id
// under): a number alone can be reused after a restore, a stamp alone is
// minute-coarse in CQL, and a space move bumps neither. When the gate carries
// the reconcile scan's comparison bases and the hit carries the children.*
// expansions, a set difference in either direction also says fetch —
// attachments and comments bump no version (GDK-1888). Anything missing, zero
// or different is fetched — the mirror is a disposable cache, so the safe
// answer is always "fetch". The reason is a log tally: a page that is re-read
// on every unchanged tick names its own cause in the sync output.
//
// The no-fetch verdicts that still carry a reason are the unchecked ones: a
// children.* expansion truncated at the search limit proves neither set
// direction, so the scan refuses to compare and returns the name (the two
// names join with a + when both listings were truncated) — the caller tallies
// it so the gap rides the log instead of aging silently. A truncated listing
// no longer ends the question: attachments truncated still falls through to
// the comment check, on purpose — one truncated axis must not blind the
// other.
//
// Documented gaps, on purpose and visible in the reasons tally: a deleted
// reply is not detected (the expansion carries top-level ids only — the
// parent's id still lists, so the set compares equal); and a page whose
// comment container 404s on fetch (restricted child content, the ErrNotFound
// branch in fetchPageRecord) while the listing still shows ids re-fetches on
// every reconcile — that page is the comments= line in the tally.
// childSetVerdict is compareChildSet's reading of one children.* expansion
// against the mirror (GDK-1888). The attachments half and the comments half
// of needsBody ask the same question with different literals, so one enum —
// the confirmVerdict pattern — owns the answers instead of two hand-copied
// branch pairs drifting apart.
type childSetVerdict int

const (
	childSetSame      childSetVerdict = iota // listed ids match the mirror exactly: no fetch
	childSetDiffers                          // a difference in either direction: fetch
	childSetUnchecked                        // listing truncated at the search limit: no comparison was possible
)

// compareChildSet folds needsBody's two duplicate halves into the one rule
// they always shared: build the listed-id map from a children.* expansion,
// compare it with the stored set via sameIDSet, and refuse the truncated
// shape — Size >= Limit proves neither set direction (GDK-1888). Only the
// reason literals differ between the attachments and comments call sites.
func compareChildSet(listing confluence.ChildList, stored map[string]bool) childSetVerdict {
	if listing.Size >= listing.Limit {
		return childSetUnchecked
	}
	listed := make(map[string]bool, len(listing.Results))
	for _, r := range listing.Results {
		if r.ID != "" {
			listed[r.ID] = true
		}
	}
	if sameIDSet(listed, stored) {
		return childSetSame
	}
	return childSetDiffers
}

func (g *pageFetchGate) needsBody(hit confluence.Page) (bool, string) {
	if g == nil {
		return true, "" // backfill: every body, not a gate decision
	}
	if hit.ID == "" {
		return true, "no-id"
	}
	if _, done := g.fetched[hit.ID]; done {
		return false, ""
	}
	if hit.Version.Number <= 0 || hit.Version.When == "" {
		return true, "unversioned-hit"
	}
	st, ok := g.have[hit.ID]
	if !ok {
		return true, "new"
	}
	if hit.Space.Key != "" {
		if held := g.spaceOf[hit.ID]; held != "" && held != hit.Space.Key {
			// GDK-1886: the mirror files this id under a different space —
			// the page moved between scoped spaces without a version bump, so
			// every stamp below still matches. Only a body fetch rewrites
			// pages.space_key, through the normal upsert (which compares it);
			// writing it from the prune path instead was rejected — see the
			// reconcile scan. Note the incremental pass alone cannot catch
			// this: its CQL lastModified floor never lists an unbumped moved
			// page. A Full or Reconcile pass is what does.
			return true, "space"
		}
	}
	if st.Version != hit.Version.Number {
		return true, "version"
	}
	if st.UpdatedAt == "" {
		return true, "no-stamp"
	}
	if st.UpdatedAt != jira.ISOTime(hit.Version.When) {
		return true, "stamp"
	}
	// GDK-1888: every stamp matches, but attachments and comments do not bump
	// the page version — the last word belongs to the hit's own child
	// listings, which only the reconcile scan's expansions carry (the
	// comparison maps are nil on every other gate, and a hit with no
	// expansion — issuetap — changes nothing). A complete listing
	// (size < limit) is compared as a set, both directions: an origin add the
	// cache misses and a cache row the origin dropped both fetch. A truncated
	// listing (size >= limit) proves neither direction, so it does not fetch
	// — the name is remembered below and both unchecked names ride the no
	// verdict joined with a +.
	var unchecked []string
	if g.attachments != nil && hit.Children.Attachment != nil {
		switch compareChildSet(*hit.Children.Attachment, g.attachments[hit.ID]) {
		case childSetDiffers:
			return true, "attachments"
		case childSetUnchecked:
			unchecked = append(unchecked, "attachments-unchecked")
		}
	}
	// The comment half, top-level ids only (the shape the expansion carries):
	// a pre-schemaV52 page (any NULL parent_id row) is healed by one fetch
	// rather than compared against a set that cannot be trusted — checked
	// before truncation, because a truncated listing with unknown parents
	// still wants the heal. On the comments= difference itself, the store's
	// unchanged-compare reads parent_id (NULL ≠ ''), so the backfill fetch
	// commits instead of skipping as unchanged.
	if g.comments != nil && hit.Children.Comment != nil {
		if g.commentsUnknown[hit.ID] {
			return true, "comments-backfill"
		}
		switch compareChildSet(*hit.Children.Comment, g.comments[hit.ID]) {
		case childSetDiffers:
			return true, "comments"
		case childSetUnchecked:
			unchecked = append(unchecked, "comments-unchecked")
		}
	}
	if len(unchecked) > 0 {
		return false, strings.Join(unchecked, "+")
	}
	return false, ""
}

// sameIDSet reports whether two id sets hold exactly the same members — the
// set compare behind compareChildSet's attachments and comments checks
// (GDK-1888). A nil set reads as the empty set, so a page the mirror holds
// with none of that child kind compares equal to an empty origin listing.
func sameIDSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for id := range a {
		if !b[id] {
			return false
		}
	}
	return true
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

// fetchPageRecord loads full body + comments + attachments for a search hit
// and maps to store. Comments are always re-fetched even when the page version
// is unchanged (comments do not bump page version — the comments-only trap).
// For a page the fetch gate skipped this is not reached at all;
// commentsOnlyPass is what keeps that page's comments current.
// spaceName is the human space title from the full page (fallback: search hit).
// atts is the pass's attachment-listing memory: an origin that refuses the
// child/attachment endpoint (issuetap answers 501) is learned once, and the
// rest of the pass skips the request instead of collecting the same refusal
// per page. Nil degrades the same way with nobody to tell.
func fetchPageRecord(ctx context.Context, c *confluence.Client, cfg *config.Config, hit confluence.Page, atts *attachmentSupport) (store.PageRecord, string, string, error) {
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
	attRows := []confluence.Attachment(nil)
	if !atts.skipListing() {
		listed, err := c.Attachments(ctx, full.ID)
		switch {
		case err == nil:
			attRows = listed
		case errors.Is(err, confluence.ErrNotFound):
			// Same restricted-child shape comments tolerate.
		case isUnsupportedEndpoint(err):
			// issuetap's wiki has no child/attachment route yet: it answers
			// 501 unsupported_endpoint. That gap is the origin's, not the
			// page's — the page keeps what it has and the pass stops asking
			// (one refusal is measured, not one per page).
			atts.noteRefused()
		default:
			return store.PageRecord{}, "", "", err
		}
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
	bodyADF := full.Body.ADFRaw()
	bodyText := adf.PlainText(bodyADF)

	// Labels: first expand page only (≤25); sorted for deterministic store rows.
	labels := full.LabelNames()
	if labels == nil {
		labels = []string{}
	}
	sort.Strings(labels)

	item := store.Item{
		ID:         pageNS(cfg) + ":" + full.ID,
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
			BodyADF:  bodyADF,
		},
	}
	for _, cm := range cms {
		cmADF := cm.Body.ADFRaw()
		cmWhen := jira.ISOTime(cm.Version.When)
		// ParentID always has a value here — a pointer to it, so a top-level
		// comment writes '' (known no-parent) and a reply writes its parent's
		// external id; NULL stays reserved for rows that predate schemaV52
		// and the origins with no thread parent at all (GDK-1888).
		parent := cm.ParentID
		rec.Comments = append(rec.Comments, store.Comment{
			ID:         pageNS(cfg) + ":" + cm.ID,
			ExternalID: cm.ID,
			Author:     cm.Version.By.DisplayName,
			AuthorID:   cm.Version.By.AccountID,
			BodyADF:    cmADF,
			BodyText:   adf.PlainText(cmADF),
			CreatedAt:  cmWhen,
			UpdatedAt:  cmWhen,
			ParentID:   &parent,
		})
	}
	for _, at := range attRows {
		rec.Attachments = append(rec.Attachments, store.Attachment{
			ID:         pageNS(cfg) + ":" + at.ID,
			ExternalID: at.ID,
			Filename:   at.Title,
			MimeType:   at.MIMEType(),
			Size:       at.Size(),
			Author:     at.Version.By.DisplayName,
			AuthorID:   at.Version.By.AccountID,
			CreatedAt:  jira.ISOTime(at.Version.When),
			// Empty for the same reason issue attachments leave it empty on
			// Cloud and built-in: the proxy builds content/{id}/download
			// from the id, and a second stored address would be two things
			// to keep true (GDK-1639).
		})
	}
	return rec, spaceName, when, nil
}

// attachmentSupport is one pass's memory of whether the origin lists page
// attachments at all. A 501 from child/attachment (issuetap: no endpoint
// yet) is an origin property, not a page property, so the first refusal
// stands for the pass — every later page skips the request. Nil-safe on
// purpose: SyncPage has no pass to remember anything for.
type attachmentSupport struct {
	refused atomic.Bool
	// measured counts the refusals actually seen. Width > 1 can race the
	// learning (two workers in flight before the first lands), so the
	// summary reports what happened rather than assuming one.
	measured atomic.Int64
	skipped  atomic.Int64
}

// skipListing reports whether this page's attachment listing should not be
// asked for at all (the origin already refused one).
func (a *attachmentSupport) skipListing() bool {
	if a == nil {
		return false
	}
	if !a.refused.Load() {
		return false
	}
	a.skipped.Add(1)
	return true
}

// noteRefused records the origin's refusal once; later pages are skipped.
func (a *attachmentSupport) noteRefused() {
	if a == nil {
		return
	}
	a.measured.Add(1)
	a.refused.Store(true)
}

// summary is the one log line a pass owes when it degraded: what was
// refused and how many pages were skipped after that. Empty when it did not.
func (a *attachmentSupport) summary() string {
	if a == nil || !a.refused.Load() {
		return ""
	}
	return fmt.Sprintf("confluence: origin refused the page-attachment listing (501) — %d measured, %d later pages skipped",
		a.measured.Load(), a.skipped.Load())
}

// isUnsupportedEndpoint reports whether err is the origin's "no such route"
// answer. issuetap returns 501 unsupported_endpoint for wiki paths it has
// not implemented; Confluence Cloud never answers 501 here.
func isUnsupportedEndpoint(err error) bool {
	var api *confluence.APIError
	if !errors.As(err, &api) {
		return false
	}
	return api.Status == http.StatusNotImplemented
}

// collectPageVersions fetches history stamps for one page and writes them.
// Since GDK-1673 it is the composition of the two pool halves — read in a
// worker (fetchPageVersions), write in the committer (writePageVersions) —
// and remains the definition of the whole operation for one-off callers.
//
// Incremental rule: refetch only when page_versions has no row for the
// incoming version number. An unchanged version cannot have grown new
// history, so comments-only rewrites and incremental ticks over a quiet
// page spend no extra GET. A missing stamp (first mirror, or a version
// bump) spends one GET.
//
// Nothing here fails the pass. Missing history degrades the mirror, it does
// not break it — and that has to hold for the store as much as the network,
// because a page whose stamps could not be written is in exactly the same
// state as one whose stamps could not be fetched. Only a cancelled context
// propagates: that is the caller stopping, not a mirror hiccup.
func collectPageVersions(ctx context.Context, c *confluence.Client, db *store.DB, opts Options, itemID, pageID string, incomingVer int) error {
	rows, err := fetchPageVersions(ctx, c, db, opts.logf, itemID, pageID, incomingVer)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return writePageVersions(ctx, db, opts.logf, itemID, pageID, rows)
}

// fetchPageVersions is collectPageVersions's read half, run inside a fetch
// pool worker (GDK-1673): the stored-stamp check is a WAL read and the
// history GET is one pooled request. logf is the serialized pool logger.
// The degrade contract is collectPageVersions's — nothing fails the pass
// except a cancelled context.
func fetchPageVersions(ctx context.Context, c *confluence.Client, db *store.DB, logf func(string, ...any), itemID, pageID string, incomingVer int) ([]store.PageVersion, error) {
	if itemID == "" || pageID == "" {
		return nil, nil
	}
	has, err := db.HasPageVersion(ctx, itemID, incomingVer)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		logf("confluence: page versions %s: read stored stamps: %v", pageID, err)
		return nil, nil
	}
	if has {
		return nil, nil
	}
	vers, err := c.PageVersions(ctx, pageID)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		logf("confluence: page versions %s: %v", pageID, err)
		return nil, nil
	}
	if len(vers) == 0 {
		return nil, nil
	}
	rows := make([]store.PageVersion, 0, len(vers))
	for _, v := range vers {
		if v.Number <= 0 {
			continue
		}
		rows = append(rows, store.PageVersion{
			Number:     v.Number,
			CreatedAt:  v.When,
			AuthorID:   v.By.AccountID,
			AuthorName: v.By.DisplayName,
			Message:    v.Message,
			MinorEdit:  v.MinorEdit,
		})
	}
	return rows, nil
}

// writePageVersions is collectPageVersions's write half, run in the
// committer after the batch's items rows exist (page_versions→items FK).
// Same degrade rule as the read half: a store failure logs and never fails
// the pass — the GDK-1307 race (the item pruned between upsert and stamp
// write by a second sync process) lands here.
func writePageVersions(ctx context.Context, db *store.DB, logf func(string, ...any), itemID, pageID string, rows []store.PageVersion) error {
	if len(rows) == 0 {
		return nil
	}
	if err := db.ReplacePageVersions(ctx, itemID, rows); err != nil {
		if ctx.Err() != nil {
			return err
		}
		logf("confluence: page versions %s: write stamps: %v", pageID, err)
		return nil
	}
	return nil
}

// commentsOnlyPass finds pages whose comments changed without a body edit.
// Cost cap: one CQL per incremental chunk (type=comment + lastModified floor).
// Hits are resolved to container page IDs; pages the page pass already fetched
// are skipped via that space's gate. Never a full-space refetch (that would
// undo C4).
//
// A page the gate skipped is deliberately *not* "already fetched": its body is
// unchanged but its comments may not be, and this pass is the only path left
// to them.
//
// Decision 0006 described a comments-only pass on a global watermark. The
// floor is per-space (spaces.watermark), queried per chunk at the oldest
// member floor. Watermark advances to max(page, comment) lastModified so a
// quiet wiki does not rescan every comment since the last body edit on every
// Watch tick.
func commentsOnlyPass(
	ctx context.Context,
	c *confluence.Client,
	opts Options,
	chunk spaceChunk,
	cp *chunkPass,
	maxUTC, maxRaw *string,
	fetch func([]confluence.Page) error,
) error {
	cql := fmt.Sprintf(`%s AND type=comment AND lastModified >= "%s" order by lastmodified asc`,
		cqlSpaceSet(chunk.keys), cqlTime(chunk.floorRaw))
	var commentHits int
	need := map[string]string{} // container page id → space key, for gate routing
	err := c.SearchPages(ctx, cql, func(hits []confluence.Page) error {
		for _, hit := range hits {
			commentHits++
			noteStamp(hit.Version.When, &cp.maxUTC, &cp.maxRaw)
			noteStamp(hit.Version.When, maxUTC, maxRaw)
			if cp.gates[hit.Space.Key].commentCurrent(hit) {
				// Already mirrored at this exact stamp: the overlap window
				// echoing, not a change. No container re-read.
				continue
			}
			pid, err := resolveCommentContainer(ctx, c, hit)
			if err != nil {
				opts.logf("confluence: comments-only skip %s: %v", hit.ID, err)
				continue
			}
			if pid == "" {
				opts.logf("confluence: comments-only skip %s (no container page)", hit.ID)
				continue
			}
			if cp.gates[hit.Space.Key].alreadyFetched(pid) {
				continue
			}
			need[pid] = hit.Space.Key
		}
		return nil
	})
	if err != nil {
		return err
	}
	pages := make([]confluence.Page, 0, len(need))
	for id, sk := range need {
		// Carry the comment hit's space key so processHits routes the container
		// fetch to the right gate.
		pages = append(pages, confluence.Page{ID: id, Space: confluence.SpaceRef{Key: sk}})
		cp.containers[id] = struct{}{}
	}
	if len(pages) > 0 {
		if err := fetch(pages); err != nil {
			return err
		}
	}
	if commentHits > 0 || len(pages) > 0 {
		// Zero-hit chunks stay silent (GDK-1074 waste ③).
		opts.logf("confluence: comments-only %d spaces pages=%d comment_hits=%d",
			len(chunk.keys), len(pages), commentHits)
	}
	return nil
}

// reCommentWebUIPage extracts the container page from a comment search
// hit's webui path form. The pageId= parameter form lives in
// store.PageIDFromQuery (GDK-1104): the case policy of that grammar has one
// owner — the same one refs extraction folds with — instead of a private
// (?i) copy here that could drift again. store.reWikiPage requires
// /wiki/spaces/ which Cloud webui often omits (/spaces/KEY/pages/ID).
var reCommentWebUIPage = regexp.MustCompile(`/pages/(\d+)`)

func commentContainerFromWebUI(webui string) string {
	if webui == "" {
		return ""
	}
	if id := store.PageIDFromQuery(webui); id != "" {
		return id
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
