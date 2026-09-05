package sync

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/adf"
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

// RunConfluence does one Confluence mirror pass: full or incremental.
// Attachments stay out of scope. Version-history stamps (page_versions; never
// bodies) are collected when a mirrored page's current version number is not
// already stored. A failed history fetch is logged and does not fail the pass.
// Every successful pass prunes pages whose space is outside the current
// config/listing scope; memory.space always joins that scope (GDK-1079,
// joinMemorySpace).
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

// runConfluencePass is the Confluence-specific body inside the shared runSource
// skeleton. Usage flush is registered by runSource on the client from setup.
func runConfluencePass(ctx context.Context, c *confluence.Client, cfg *config.Config, db *store.DB, opts Options, state store.SyncState, res *Result) error {
	// Upgrade path for GDK-344: local-origin wiki mirrors written before the
	// page id namespace existed hold `confluence:N` rows whose keys the pass
	// is about to re-insert as `local-origin-confluence:N` — same (source_id,
	// key), different id, which UNIQUE(source_id, key) rejects.
	if cfg.HasLocalOrigin() {
		if n, err := db.PurgePageIDsOutsideNamespace(ctx, ConfluenceSourceID, pageNS(cfg)); err != nil {
			return record(ctx, cfg, db, ConfluenceSourceID, err)
		} else if n > 0 {
			opts.logf("purged %d pre-namespace local-origin pages (GDK-344)", n)
		}
	}

	spaces := cfg.Confluence.Spaces
	if len(spaces) == 0 {
		listed, err := c.Spaces(ctx)
		if err != nil {
			return record(ctx, cfg, db, ConfluenceSourceID, err)
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
					return record(ctx, cfg, db, ConfluenceSourceID, err)
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
	// GDK-1079: memory.space joins the pass's scope whichever path built it,
	// and behind path ①'s global filter — a personal memory space must not be
	// dropped by a filter that exists to drop exactly those.
	spaces, joined, err := joinMemorySpace(ctx, c, cfg, opts, spaces)
	if err != nil {
		return record(ctx, cfg, db, ConfluenceSourceID, err)
	}
	if joined != nil {
		if err := db.UpsertSpaces(ctx, ConfluenceSourceID, []store.SpaceRow{*joined}); err != nil {
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
	// Watermarks are committed per chunk after that chunk finishes — never
	// from a mid-chunk page batch (a later chunk's failure must not inherit
	// this chunk's floor, and this chunk's floors must not move until its
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
		for _, rec := range batch {
			if err := collectPageVersions(ctx, c, db, opts, rec.Item.ID, rec.Item.ExternalID, rec.Page.Version); err != nil {
				return err
			}
		}
		res.Fetched += len(batch)
		res.Changed += changed
		if opts.Progress != nil {
			opts.Progress(res.Fetched, res.Changed)
		}
		opts.logf("  confluence: %s pages", formatCount(res.Fetched))
		return nil
	}

	processHits := func(cp *chunkPass) func([]confluence.Page) error {
		return func(hits []confluence.Page) error {
			batch := make([]store.PageRecord, 0, pageBatchSize)
			spaceByKey := map[string]store.SpaceRow{}
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
					opts.logf("confluence: skip %s (space %q outside this pass)", hit.ID, sk)
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
					res.PageSkips++
					noteStamp(hit.Version.When, &cp.maxUTC, &cp.maxRaw)
					noteStamp(hit.Version.When, &maxUTC, &maxRaw)
					continue
				}
				if why != "" {
					if _, isContainer := cp.containers[hit.ID]; isContainer {
						why = "comment-container"
					}
					cp.reasons[why]++
				}
				gate.markFetched(hit.ID)
				res.PageBodies++
				cp.bodies[sk]++
				rec, spaceName, when, err := fetchPageRecord(ctx, c, cfg, hit)
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
				noteStamp(when, &cp.maxUTC, &cp.maxRaw)
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

	// syncBackfill fully re-reads one space: the mirror's repair path (new or
	// restored spaces, and every space on --full). The nil gate re-reads every
	// body regardless of what the local rows claim; comments arrive with each
	// body, so there is no comments-only pass.
	syncBackfill := func(key string) error {
		cp := newChunkPass(map[string]*pageFetchGate{key: nil})
		cql := fmt.Sprintf(`space=%s AND type=page order by lastmodified asc`, cqlSpace(key))
		before := res.PageBodies
		if err := c.SearchPages(ctx, cql, processHits(cp)); err != nil {
			return record(ctx, cfg, db, ConfluenceSourceID, err)
		}
		opts.logf("confluence: space %s floor=full-backfill fetched=%d", key, res.PageBodies-before)
		if cp.maxRaw == "" {
			return nil
		}
		if err := db.SetSpaceWatermark(ctx, ConfluenceSourceID, key, cp.maxRaw); err != nil {
			return err
		}
		wms[key] = cp.maxRaw
		// Compatibility: sync_state.watermark stays the max across spaces so
		// status/doctor/freshness keep working. It is not an incremental floor.
		return db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{Watermark: cp.maxRaw})
	}

	// syncChunk runs one incremental chunk: one type=page CQL, one type=comment
	// CQL, floored at the chunk's oldest member watermark. Gates keep re-hits
	// inside the widened window from costing body reads.
	syncChunk := func(chunk spaceChunk) error {
		gates := make(map[string]*pageFetchGate, len(chunk.keys))
		for _, key := range chunk.keys {
			gate, err := newPageFetchGate(ctx, db, key, false)
			if err != nil {
				return err
			}
			gates[key] = gate
		}
		cp := newChunkPass(gates)
		bodiesBefore, skipsBefore := res.PageBodies, res.PageSkips
		cql := fmt.Sprintf(`%s AND type=page AND lastModified >= "%s" order by lastmodified asc`,
			cqlSpaceSet(chunk.keys), cqlTime(chunk.floorRaw))
		if err := c.SearchPages(ctx, cql, processHits(cp)); err != nil {
			return record(ctx, cfg, db, ConfluenceSourceID, err)
		}
		// comments-only pass: one type=comment CQL per chunk. Pages already
		// fetched above are skipped via the gates.
		if err := commentsOnlyPass(ctx, c, opts, chunk, cp, &maxUTC, &maxRaw, processHits(cp)); err != nil {
			return record(ctx, cfg, db, ConfluenceSourceID, err)
		}
		opts.logf("confluence: %d spaces floor=%s fetched=%d unchanged=%d", len(chunk.keys), chunk.floorRaw,
			res.PageBodies-bodiesBefore, res.PageSkips-skipsBefore)
		for _, key := range sortedKeys(cp.bodies) {
			opts.logf("confluence: space %s fetched=%d", key, cp.bodies[key])
		}
		if len(cp.reasons) > 0 {
			// Why the gate said fetch — the debug surface for "an unchanged tick
			// keeps re-reading the same N bodies" (GDK-1074 waste ①).
			opts.logf("confluence: refetch reasons %s", formatReasons(cp.reasons))
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
			if err := db.SetSpaceWatermark(ctx, ConfluenceSourceID, key, cp.maxRaw); err != nil {
				return err
			}
			wms[key] = cp.maxRaw
		}
		// Compatibility: sync_state.watermark stays the max across spaces so
		// status/doctor/freshness keep working. It is not an incremental floor.
		return db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{Watermark: cp.maxRaw})
	}

	if res.Full {
		for _, key := range spaces {
			if err := syncBackfill(key); err != nil {
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
			if err := syncChunk(chunk); err != nil {
				return err
			}
		}
		for _, key := range backfills {
			if err := syncBackfill(key); err != nil {
				return err
			}
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

	res.Watermark = maxRaw
	if err := db.RecordSync(ctx, ConfluenceSourceID, store.SyncResult{Watermark: maxRaw, FullSync: res.Full}); err != nil {
		return err
	}
	return nil
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
// (localOrigin wikis inside released binaries, paired home serves) parse only
// that form, and nearly every local-origin/paired workspace has exactly one
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
}

func newChunkPass(gates map[string]*pageFetchGate) *chunkPass {
	return &chunkPass{gates: gates, bodies: map[string]int{}, reasons: map[string]int{}, containers: map[string]struct{}{}}
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
	// haveComments is the mirror's stamp per source comment id. The
	// comments-only pass consults it so a comment hit inside cqlTime's overlap
	// window — already mirrored at exactly this stamp — does not re-read its
	// container page body on every tick.
	haveComments map[string]string
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
// hit's version number *and* the hit's lastModified: a number alone can be
// reused after a restore, and a stamp alone is minute-coarse in CQL. Anything
// missing, zero or different is fetched — the mirror is a disposable cache, so
// the safe answer is always "fetch". The reason is a log tally: a page that is
// re-read on every unchanged tick names its own cause in the sync output.
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
	if st.Version != hit.Version.Number {
		return true, "version"
	}
	if st.UpdatedAt == "" {
		return true, "no-stamp"
	}
	if st.UpdatedAt != jira.ISOTime(hit.Version.When) {
		return true, "stamp"
	}
	return false, ""
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
func fetchPageRecord(ctx context.Context, c *confluence.Client, cfg *config.Config, hit confluence.Page) (store.PageRecord, string, string, error) {
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
		rec.Comments = append(rec.Comments, store.Comment{
			ID:         pageNS(cfg) + ":" + cm.ID,
			ExternalID: cm.ID,
			Author:     cm.Version.By.DisplayName,
			AuthorID:   cm.Version.By.AccountID,
			BodyADF:    cmADF,
			BodyText:   adf.PlainText(cmADF),
			CreatedAt:  cmWhen,
			UpdatedAt:  cmWhen,
		})
	}
	return rec, spaceName, when, nil
}

// collectPageVersions fetches history stamps for one page and writes them.
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
	if itemID == "" || pageID == "" {
		return nil
	}
	has, err := db.HasPageVersion(ctx, itemID, incomingVer)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		opts.logf("confluence: page versions %s: read stored stamps: %v", pageID, err)
		return nil
	}
	if has {
		return nil
	}
	vers, err := c.PageVersions(ctx, pageID)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		opts.logf("confluence: page versions %s: %v", pageID, err)
		return nil
	}
	if len(vers) == 0 {
		return nil
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
	if len(rows) == 0 {
		return nil
	}
	if err := db.ReplacePageVersions(ctx, itemID, rows); err != nil {
		if ctx.Err() != nil {
			return err
		}
		// GDK-1307: the item can be gone by now — a second sync process on
		// the same mirror (an older app beside a newer CLI, with a different
		// space scope) prunes it between UpsertPages and this write, and the
		// page_versions→items FOREIGN KEY refuses. That is a missing stamp,
		// not a broken pass; the next pass over the page re-fetches it.
		opts.logf("confluence: page versions %s: write stamps: %v", pageID, err)
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
