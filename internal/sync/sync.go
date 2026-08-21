// Package sync fills the mirror from the configured sources (Jira, and
// Confluence when enabled). It only ever writes to the mirror, never to Jira
// or Confluence, and the rules it implements are in
// specs/000-product/contracts/sync.md.
package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/jirafields"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// SourceID is the slug the Jira connector owns in `sources` and `sync_state`.
const SourceID = "jira"

// Phase source names for Options.Phase. Callers report what the mirror is
// fetching; Progress carries how much. Keep these string values stable — the
// web client keys UI copy on them.
const (
	PhaseIssues    = "issues"
	PhaseDocuments = "documents"
	PhaseIdle      = ""
)

// overlap covers JQL's minute granularity on `updated`: an exact >= boundary
// drops issues updated in the same minute as the watermark.
const overlap = 2 * time.Minute

// baseFields is the explicit field list. `*all` is slow on a large site and
// pulls custom fields nobody mapped.
var baseFields = []string{
	"summary", "description", "environment", "issuetype", "status", "priority",
	"assignee", "reporter", "creator", "project", "parent", "labels", "components",
	"fixVersions", "versions", "duedate", "resolution", "created", "updated",
	"comment", "attachment", "issuelinks",
}

// Options tunes one Run or Watch cycle. A nil Client or ConfluenceClient is
// built from cfg; a nil Notifier uses OSNotifier and never aborts the loop; a
// Reload error keeps the previous config so a momentarily unreadable file
// cannot stop the mirror.
type Options struct {
	Full      bool
	Reconcile bool
	// Log, when set, receives one line per committed page and per pass.
	Log func(string)
	// Progress, when set, is called once per committed page with the running
	// totals. It exists so a caller can report progress without parsing Log.
	Progress func(fetched, changed int)
	// Phase, when set, is called with the connector a pass is about to run
	// ("issues", "documents") and with "" when the cycle has nothing in flight.
	// It exists so a caller can report *what* is being fetched; Progress carries
	// how much. Watch calls it around each source; one-shot callers set it too.
	Phase func(source string)
	// Client is for tests and for a server that wants to share one; nil builds
	// one from cfg.
	Client *jira.Client
	// ConfluenceClient is for tests; nil builds one from cfg when Confluence is configured.
	ConfluenceClient *confluence.Client
	// LinearClient is for tests; nil builds one from cfg when Linear is configured.
	LinearClient *linear.Client
	// Notifier delivers OS desktop alerts for new personal-feed events after
	// each successful Watch cycle. Nil uses OSNotifier. Never aborts the loop.
	Notifier Notifier
	// Reload re-reads the config at the top of each watch cycle. Nil keeps the
	// config Watch was called with. A reload error is logged and the previous
	// config stays in use: a momentarily unreadable file must not stop the mirror.
	Reload func() (*config.Config, error)
	// Tick, when > 0, is the Watch interval. The zero value derives from
	// cfg.EffectiveSyncIntervalSec() (an integer number of seconds), which is
	// the production path. Tests set a sub-second Tick so they do not sit on
	// that 1s floor.
	Tick time.Duration
	// sprintField caches the gh-sprint custom field id for one Watch (or one
	// Run). Nil means this call has no cache yet; runJiraPass allocates one.
	sprintField *sprintFieldCache
}

// Result is the tally of one source pass (Run or RunConfluence).
// Fetched/Changed/Deleted count what this pass touched. Full is true when
// the pass had no watermark to increment from, or the caller asked for one.
// Watermark is the newest upstream timestamp recorded on success.
type Result struct {
	Full      bool
	Fetched   int
	Changed   int
	Deleted   int
	Watermark string
	// PageBodies and PageSkips are the Confluence pass's body-read tally: how
	// many page bodies this pass went to the source for (a hit that turned out
	// to be deleted still counts — the request was spent), and how many search
	// hits pageFetchGate answered from the mirror instead. They are the answer
	// to "how many bodies did that tick fetch?" — a quiet tick over an
	// unchanged corpus must report PageBodies 0. Jira leaves both at 0.
	PageBodies int
	PageSkips  int
}

func (o Options) logf(format string, args ...any) {
	if o.Log != nil {
		o.Log(fmt.Sprintf(format, args...))
	}
}

func (o Options) phasef(source string) {
	if o.Phase != nil {
		o.Phase(source)
	}
}

// ErrFrozen means this workspace refuses pulls from origin (config "frozen").
var ErrFrozen = errors.New("workspace is frozen: no sync into this mirror (config \"frozen\": true)")

// refuseIfFrozen is the gate every exported entry point calls first, so a new
// caller cannot reintroduce GDK-181 by forgetting a check of its own.
func refuseIfFrozen(cfg *config.Config) error {
	if cfg.SyncFrozen() {
		return ErrFrozen
	}
	return nil
}

// Run does one sync pass: full or incremental, plus a reconcile pass when asked
// for or after a full sync.
//
// A failure leaves the pages already committed in place and does not advance the
// watermark, so the next run re-reads from the last known-good point
// (contracts/sync.md invariants 2 and 3).
//
// An empty cfg.Projects means no project filter: the account's full visible
// issue set is the scope (one Search, not one per project).
func Run(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) (Result, error) {
	if err := refuseIfFrozen(cfg); err != nil {
		return Result{}, err
	}
	var c *jira.Client
	return runSource(ctx, cfg, db, opts,
		sourceIdent{ID: SourceID, Kind: "jira"},
		true, // SupportsReconcile: full and opts.Reconcile both run reconcile
		"",
		func() (string, usageTaker, error) {
			c = opts.Client
			if c == nil {
				var err error
				c, err = origin.Client(cfg)
				if err != nil {
					return "", nil, err
				}
			}
			return c.BaseURL(), c, nil
		},
		func(state store.SyncState, res *Result) error {
			return runJiraPass(ctx, c, cfg, db, opts, state, res)
		},
	)
}

// runJiraPass is the Jira-specific body inside the shared runSource skeleton.
func runJiraPass(ctx context.Context, c *jira.Client, cfg *config.Config, db *store.DB, opts Options, state store.SyncState, res *Result) error {
	// Discovery mode: no configured custom fields yet — first full sync pulls
	// *all so raw carries every custom value for auto-configuration.
	discoveryMode := len(cfg.Fields) == 0 && len(cfg.FieldMap) == 0

	// Upgrade path for GDK-241: standalone mirrors written before the id
	// namespace existed hold `jira:N` rows whose keys the pass is about to
	// re-insert as `standalone-jira:N` — same (source_id, key), different id,
	// which the UNIQUE(source_id, key) index rejects. The mirror is a
	// disposable cache: drop the legacy rows and let this pass re-mirror them
	// under the new namespace. No tombstones — the keys come right back.
	if cfg.IsStandalone() {
		if n, err := db.PurgeIssueIDsOutsideNamespace(ctx, SourceID, itemNS(cfg)); err != nil {
			return record(ctx, cfg, db, SourceID, err)
		} else if n > 0 {
			opts.logf("purged %d pre-namespace standalone rows (GDK-241)", n)
		}
	}

	cats, err := c.Statuses(ctx)
	if err != nil {
		return record(ctx, cfg, db, SourceID, err)
	}
	prios, err := c.Priorities(ctx)
	if err != nil {
		return record(ctx, cfg, db, SourceID, err)
	}

	if opts.sprintField == nil {
		opts.sprintField = &sprintFieldCache{}
	}
	if opts.Reconcile {
		opts.sprintField.reset()
	}
	sprintFieldID := opts.sprintField.resolve(ctx, c, opts)
	fieldIDs := appendSprintField(fieldList(cfg, res.Full), sprintFieldID)
	var maxUTC, maxRaw string
	// pageBase / unitDenom drive per-Search progress lines. unitDenom < 0 means
	// the approximate count failed and the line has no denominator.
	pageBase := 0
	unitDenom := -1
	page := func(issues []jira.Issue) error {
		batch := store.Batch{Categories: cats, Priorities: prios, Records: make([]store.IssueRecord, 0, len(issues))}
		for _, iss := range issues {
			// A status the site list did not cover is still known from the issue
			// itself, and a missing category can only lose a reopen.
			if id := iss.Fields.Status.ID; id != "" {
				if _, ok := cats[id]; !ok {
					cats[id] = jira.Category(iss.Fields.Status.StatusCategory.Key)
				}
			}
			r, err := build(ctx, c, cfg, iss, sprintFieldID)
			if err != nil {
				return err
			}
			batch.Records = append(batch.Records, r)
		}
		changed, err := db.UpsertIssues(ctx, batch)
		if err != nil {
			return err
		}
		res.Fetched += len(issues)
		res.Changed += changed
		// The watermark moves only for a page that is committed, which is this
		// line being after the upsert rather than before it.
		for _, iss := range issues {
			if u := jira.ISOTime(iss.Fields.Updated); u > maxUTC {
				maxUTC, maxRaw = u, iss.Fields.Updated
			}
		}
		local := res.Fetched - pageBase
		if unitDenom >= 0 {
			pct := 0
			if unitDenom > 0 {
				pct = local * 100 / unitDenom
				if pct > 100 {
					pct = 100
				}
			}
			opts.logf("  %s / %s  (%d%%)", formatCount(local), formatCount(unitDenom), pct)
		} else {
			opts.logf("  %s issues", formatCount(local))
		}
		if opts.Progress != nil {
			opts.Progress(res.Fetched, res.Changed)
		}
		return nil
	}

	// beginSearch resets per-page progress counters and optionally asks Jira for
	// a denominator. Count failures are silent (unitDenom stays -1). withAbout
	// puts " — about N issues" on the start line (full sync only; incremental
	// start lines carry the since-clock instead).
	beginSearch := func(startLine, countJQL string, withAbout bool) {
		pageBase = res.Fetched
		unitDenom = -1
		if opts.Log == nil {
			// The denominator exists only for the log lines — Progress carries
			// running totals, not a total. Watch loops run with no Log, and an
			// unread count would be one extra request every cycle.
			return
		}
		n, ok := approxCount(ctx, c, countJQL)
		if ok {
			unitDenom = n
		}
		if withAbout && ok {
			opts.logf("%s — about %s issues", startLine, formatCount(n))
			return
		}
		opts.logf("%s", startLine)
	}

	if res.Full {
		if len(cfg.Projects) == 0 {
			jql := fullJQL("")
			// Newest activity first: the mirror is usable the moment the first
			// page lands, instead of after every historical issue. The watermark
			// is max(updated) over fetched pages, so ordering does not affect it.
			beginSearch("full sync: "+scopeLabel(cfg), jql, true)
			if err := c.Search(ctx, jql, fieldIDs, true, page); err != nil {
				return record(ctx, cfg, db, SourceID, err)
			}
		} else {
			for _, p := range cfg.Projects {
				jql := fullJQL(p)
				// Newest activity first: the mirror is usable the moment the first
				// page lands, instead of after every historical issue. The watermark
				// is max(updated) over fetched pages, so ordering does not affect it.
				beginSearch("full sync: "+p, jql, true)
				if err := c.Search(ctx, jql, fieldIDs, true, page); err != nil {
					return record(ctx, cfg, db, SourceID, err)
				}
			}
		}
	} else {
		jql := incrementalJQL(cfg.Projects, state.Watermark)
		beginSearch("incremental: "+scopeLabel(cfg)+" — changes since "+sinceLabel(state.Watermark), jql, false)
		if discoveryMode && !cfg.IsStandalone() {
			opts.logf("tip: run `gadak sync --full` once to auto-configure custom fields")
		}
		if err := c.Search(ctx, jql, fieldIDs, true, page); err != nil {
			return record(ctx, cfg, db, SourceID, err)
		}
	}

	res.Watermark = maxRaw
	if skips := devStatusSkips.Swap(0); skips > 0 {
		opts.logf("dev-status: skipped on %d issues (the panel read is best-effort — Cloud marks the API internal)", skips)
	}
	if err := db.RecordSync(ctx, SourceID, store.SyncResult{Watermark: maxRaw, FullSync: res.Full}); err != nil {
		return err
	}

	if opts.Reconcile || res.Full {
		// Version catalog is a cache of GET /project/{key}/versions. Full and
		// reconcile refresh it; an incremental tick must not (GDK-532).
		syncVersionCatalog(ctx, c, db, cfg, opts)
		deleted, err := reconcile(ctx, c, db, cfg.Projects, opts)
		res.Deleted = deleted
		if err != nil {
			return record(ctx, cfg, db, SourceID, err)
		}
	}

	// Custom-field discovery / field_usage refresh (before the done line).
	if discoveryMode && res.Full {
		if err := runDiscovery(ctx, c, cfg, db, opts); err != nil {
			// cfg.Save failure propagates; other discovery errors are warnings.
			return err
		}
	} else if (res.Full || opts.Reconcile) && len(cfg.FieldSpecs()) > 0 {
		if err := refreshFieldUsage(ctx, db, cfg); err != nil {
			opts.logf("fields: usage refresh skipped: %v", err)
		}
	}

	// Owned + starred filters. Failure must not undo the issue pass.
	importFilters(ctx, c, cfg, db, opts)
	return nil
}

// runDiscovery configures custom fields from the site catalog + mirror raw
// after a discovery-mode full sync. Network failures log a warning and leave
// the sync successful; Save failures propagate.
func runDiscovery(ctx context.Context, c *jira.Client, cfg *config.Config, db *store.DB, opts Options) error {
	catalog, err := c.Fields(ctx)
	if err != nil {
		opts.logf("fields: discovery skipped: %v", err)
		return nil
	}
	fill, err := scanMirrorFill(ctx, db)
	if err != nil {
		opts.logf("fields: discovery skipped: %v", err)
		return nil
	}
	specs := jirafields.Discover(catalog, fill, cfg.Fields)
	cfg.Fields = specs
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("fields: save discovered specs: %w", err)
	}
	n, err := reingestFromConfig(ctx, db, cfg)
	if err != nil {
		opts.logf("fields: discovery skipped: %v", err)
		return nil
	}
	if err := refreshFieldUsage(ctx, db, cfg); err != nil {
		opts.logf("fields: usage refresh skipped: %v", err)
	}
	if len(specs) == 0 {
		opts.logf("fields: no custom fields in use")
		return nil
	}
	opts.logf("fields: discovered %s custom fields in use — labels, filters, and editors configured", formatCount(len(specs)))
	opts.logf("fields: backfilled %s issues from the mirror (version/synced_at bumped, no re-download)", formatCount(n))
	return nil
}

// scanMirrorFill counts filled field ids across the mirror's stored raw JSON.
func scanMirrorFill(ctx context.Context, db *store.DB) (map[string]int, error) {
	fill := map[string]int{}
	err := db.ScanFieldFill(ctx, func(_ string, fieldVals map[string]json.RawMessage) error {
		for id, raw := range fieldVals {
			if fields.IsFilled(raw) {
				fill[id]++
			}
		}
		return nil
	})
	return fill, err
}

// reingestFromConfig rewrites issues.custom and FTS body from raw using specs.
func reingestFromConfig(ctx context.Context, db *store.DB, cfg *config.Config) (int, error) {
	specs := cfg.FieldSpecs()
	bodyIDs := fields.BodyFieldIDs(cfg.BodyFields, specs)
	return db.ReingestCustom(ctx, fields.SpecIDsFrom(specs), bodyIDs)
}

// refreshFieldUsage recomputes the field_usage table from issues.custom.
func refreshFieldUsage(ctx context.Context, db *store.DB, cfg *config.Config) error {
	specs := cfg.FieldSpecs()
	if len(specs) == 0 {
		return db.ReplaceFieldUsage(ctx, nil)
	}
	aliases := make([]string, 0, len(specs))
	for _, s := range specs {
		if s.Alias != "" {
			aliases = append(aliases, s.Alias)
		}
	}
	rows, err := db.ComputeFieldUsage(ctx, aliases)
	if err != nil {
		return err
	}
	return db.ReplaceFieldUsage(ctx, rows)
}

// approxCount asks Jira for a progress denominator. Any failure or timeout is
// reported as ok=false; the caller continues without a denominator.
func approxCount(ctx context.Context, c *jira.Client, jql string) (int, bool) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	n, err := c.Count(cctx, jql)
	if err != nil {
		return 0, false
	}
	return n, true
}

// Watch runs incremental sync on an interval and reconcile on a longer one. A
// transport failure is logged and retried on the next tick; a rejected
// credential (IsRejectedCredential — atlhttp.ErrAuth or any error implementing
// atlhttp.RejectedCredential) records last_error, logs
// once, and stops retrying that source, because every further request would
// only burn rate budget. Jira is fatal (the loop ends). Confluence is not:
// Jira mirroring keeps going when only the wiki side is rejected. `gadak
// doctor` (sync.<id>.last_error) and sync_health read that last_error.
// `gadak status --json` last_error is the Jira row only. A later one-shot
// Run / RunConfluence with a new token still clears it. After each successful
// Jira cycle, new personal-feed events may
// produce one OS desktop notification (see notifyAfterSync); notification
// failures never stop the loop.
//
// When opts.Reload is set, each cycle re-reads config so a settings edit
// (projects, Confluence, intervals) takes effect without restarting the process.
func Watch(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) error {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if err := refuseIfFrozen(cfg); err != nil {
		return err
	}
	every := time.Duration(cfg.EffectiveSyncIntervalSec()) * time.Second
	if opts.Tick > 0 {
		every = opts.Tick
	}
	reconcileEvery := time.Duration(cfg.EffectiveReconcileIntervalSec()) * time.Second
	scope := syncScope(cfg)

	tick := time.NewTicker(every)
	defer tick.Stop()
	rtick := time.NewTicker(reconcileEvery)
	defer rtick.Stop()

	o := opts
	if o.sprintField == nil {
		o.sprintField = &sprintFieldCache{}
	}
	defer o.phasef(PhaseIdle)
	dead := map[string]bool{}
	sources := defaultWatchSources()
	cred := watchCredential(cfg)
	for {
		skip := false
		if opts.Reload != nil {
			next, err := opts.Reload()
			if err != nil {
				opts.logf("sync loop: reload config: %v", err)
			} else if next != nil {
				cfg = next
				if err := refuseIfFrozen(cfg); err != nil {
					// GDK-541: a freeze must skip this tick, not end Watch.
					skip = true
				} else {
					if nextCred := watchCredential(cfg); nextCred != cred {
						// Skip is per-credential: a settings token rotation must
						// retry a previously-rejected source. Same credential
						// keeps the skip so we do not 401 every tick. Sprint
						// field ids are per-site, so a new credential must
						// rediscover.
						dead = map[string]bool{}
						cred = nextCred
						o.sprintField.reset()
					}
					newEvery := time.Duration(cfg.EffectiveSyncIntervalSec()) * time.Second
					if opts.Tick > 0 {
						newEvery = opts.Tick
					}
					newReconcile := time.Duration(cfg.EffectiveReconcileIntervalSec()) * time.Second
					if newEvery != every {
						every = newEvery
						tick.Reset(every)
					}
					if newReconcile != reconcileEvery {
						reconcileEvery = newReconcile
						rtick.Reset(reconcileEvery)
					}
					if newScope := syncScope(cfg); newScope != scope {
						opts.logf("sync scope changed: %s -> %s", scope, newScope)
						scope = newScope
					}
				}
			}
		}
		if !skip {
			for _, src := range sources {
				if !src.enabled(cfg) || dead[src.id] {
					continue
				}
				o.phasef(src.phase)
				_, runErr := src.run(ctx, cfg, db, o)
				if runErr == nil && src.notify {
					if nerr := notifyAfterSync(db, cfg, o.Notifier); nerr != nil {
						// Desktop notify is best-effort: never abort the watch loop.
						opts.logf("notify: %v", nerr)
					}
				}
				if err := applyWatchErr(ctx, cfg, db, src, runErr, opts.logf, dead); err != nil {
					return err
				}
			}
			o.Full, o.Reconcile = false, false
		}
		o.phasef(PhaseIdle)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		case <-rtick.C:
			o.Reconcile = true
		}
	}
}

// watchCredential is the identity the Watch skip map is keyed on. Site,
// email, or token changing is a new credential; a previously-rejected
// source must be retried (TestWatchConfluenceResumesAfterCredentialReload).
func watchCredential(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	id := cfg.Site + "\x00" + cfg.Email + "\x00" + cfg.Token
	if cfg.Linear != nil {
		// The Linear key is its own credential: rotating it must clear a
		// previously-dead linear source, same as an Atlassian token swap.
		id += "\x00" + cfg.Linear.APIKey
	}
	return id
}

// syncScope is the part of the config that decides what gets mirrored. Watch
// logs a line when it changes so a settings edit is visible in the log the
// user is looking at. The string carries counts only — never project or space keys.
func syncScope(cfg *config.Config) string {
	if cfg == nil {
		cfg = &config.Config{}
	}
	var proj string
	if len(cfg.Projects) == 0 {
		proj = "all projects"
	} else {
		proj = fmt.Sprintf("%d projects", len(cfg.Projects))
	}
	if cfg.Linear != nil {
		// Empty TeamIDs is every team the key can see (see runLinearPass).
		if n := len(cfg.Linear.TeamIDs); n == 0 {
			proj += ", linear on (all teams)"
		} else {
			proj += fmt.Sprintf(", linear on (%d teams)", n)
		}
	}
	if cfg.Confluence == nil {
		return proj + ", confluence off"
	}
	switch n := len(cfg.Confluence.Spaces); {
	case n == 0:
		// Empty is not "nothing selected": it is every global space
		// (see RunConfluence). Saying "0 spaces" would read as the opposite.
		return proj + ", confluence on (all global spaces)"
	case n == 1:
		return proj + ", confluence on (1 space)"
	default:
		return fmt.Sprintf("%s, confluence on (%d spaces)", proj, n)
	}
}

// reconcile proves absence, which a search over an `updated >=` window cannot.
// It is a separate pass because its cost scales with total issue count.
// Empty projects means the whole visible site is in scope: every mirrored key
// not returned upstream is a candidate for deletion.
func reconcile(ctx context.Context, c *jira.Client, db *store.DB, projects []string, opts Options) (int, error) {
	upstream := map[string]bool{}
	jql := reconcileJQL(projects)
	// summary rather than an empty list: the key comes back regardless, and an
	// empty `fields` is the one thing Jira may answer with every field.
	if err := c.Search(ctx, jql, []string{"summary"}, false, func(issues []jira.Issue) error {
		for _, iss := range issues {
			upstream[iss.Key] = true
		}
		return nil
	}); err != nil {
		return 0, err
	}

	lites, err := db.IssueLites(ctx)
	if err != nil {
		return 0, err
	}
	allProjects := len(projects) == 0
	inScope := map[string]bool{}
	for _, p := range projects {
		inScope[strings.ToUpper(p)] = true
	}
	gone := []string{}
	for _, l := range lites {
		if (allProjects || inScope[strings.ToUpper(l.ProjectKey)]) && !upstream[l.IssueKey] {
			gone = append(gone, l.IssueKey)
		}
	}
	if len(gone) == 0 {
		return 0, nil
	}
	if len(upstream) == 0 {
		return 0, fmt.Errorf("reconcile: upstream reported no issues in scope while the mirror holds %d; refusing to empty it", len(gone))
	}
	opts.logf("reconcile: %d keys vanished upstream", len(gone))
	return db.DeleteItems(ctx, SourceID, gone)
}

// sourceNS is the id-namespace prefix for one connector. Standalone
// (issuetap) numeric ids overlap the numbers a real Atlassian site uses, so
// mirrored rows get a distinct prefix; source_id stays the connector slug
// (ids are opaque, never parsed back).
func sourceNS(cfg *config.Config, sourceID string) string {
	if cfg != nil && cfg.IsStandalone() {
		return "standalone-" + sourceID
	}
	return sourceID
}

// itemNS is the id namespace for mirrored issue rows (GDK-241).
func itemNS(cfg *config.Config) string {
	return sourceNS(cfg, SourceID)
}

// pageNS is the id namespace for mirrored wiki rows — pages and their
// comments (GDK-344). issuetap issues page ids from 20000 and page-comment
// ids from 30000. A page's key stays the numeric external id, so a
// pre-namespace row must be purged before the namespaced id is inserted
// (UNIQUE(source_id, key)).
func pageNS(cfg *config.Config) string {
	return sourceNS(cfg, ConfluenceSourceID)
}

// devStatusSkips counts dev-status fetches that failed this pass (GDK-496):
// the panel read is best-effort — a failure skips the issue's dev links, and
// the pass reports one line instead of failing the sync. Reset per pass.
var devStatusSkips atomic.Int64

// build maps one Jira issue onto the store's record, fetching the children the
// search response truncated.
func build(ctx context.Context, c *jira.Client, cfg *config.Config, iss jira.Issue, sprintFieldID string) (store.IssueRecord, error) {
	f := iss.Fields

	comments := f.Comment.Comments
	if f.Comment.Total > len(comments) {
		full, err := c.Comments(ctx, iss.Key)
		if err != nil {
			return store.IssueRecord{}, err
		}
		comments = full
	}

	var histories []jira.History
	if iss.Changelog != nil {
		histories = iss.Changelog.Histories
		if iss.Changelog.Total > len(histories) {
			full, err := c.Changelog(ctx, iss.Key)
			if err != nil {
				return store.IssueRecord{}, err
			}
			histories = full
		}
	}

	body := []string{jira.PlainText(f.Description)}
	for _, id := range fields.BodyFieldIDs(cfg.BodyFields, cfg.FieldSpecs()) {
		if text := jira.PlainText(iss.Extra[id]); text != "" {
			body = append(body, text)
		}
	}

	author := f.Creator
	if author == nil {
		author = f.Reporter
	}
	item := store.Item{
		ID:         itemNS(cfg) + ":" + iss.ID,
		SourceID:   SourceID,
		Kind:       "issue",
		ExternalID: iss.ID,
		Key:        iss.Key,
		Title:      f.Summary,
		BodyText:   strings.TrimSpace(strings.Join(body, "\n\n")),
		URL:        c.BaseURL() + "/browse/" + iss.Key,
		CreatedAt:  jira.ISOTime(f.Created),
		UpdatedAt:  jira.ISOTime(f.Updated),
	}
	if author != nil {
		item.Author, item.AuthorID = author.DisplayName, author.AccountID
	}

	projectKey := f.Project.Key
	if projectKey == "" {
		projectKey, _, _ = strings.Cut(iss.Key, "-")
	}
	issue := store.Issue{
		ProjectKey:      projectKey,
		IssueType:       f.IssueType.Name,
		IssueTypeID:     f.IssueType.ID,
		Status:          f.Status.Name,
		StatusID:        f.Status.ID,
		StatusCategory:  jira.Category(f.Status.StatusCategory.Key),
		Labels:          f.Labels,
		Components:      names(f.Components),
		FixVersions:     names(f.FixVersions),
		FixVersionIDs:   ids(f.FixVersions),
		AffectsVersions: names(f.Versions),
		EnvironmentText: jira.PlainText(f.Environment),
		Duedate:         f.Duedate,
		DescriptionADF:  f.Description,
		Custom:          custom(cfg.FieldSpecs(), iss.Extra),
		Raw:             iss.Raw,
	}
	if f.Priority != nil {
		issue.Priority = f.Priority.Name
		issue.PriorityID = f.Priority.ID
	}
	if f.Assignee != nil {
		issue.Assignee, issue.AssigneeID, issue.AssigneeEmail = f.Assignee.DisplayName, f.Assignee.AccountID, f.Assignee.Email
	}
	if f.Reporter != nil {
		issue.Reporter, issue.ReporterID, issue.ReporterEmail = f.Reporter.DisplayName, f.Reporter.AccountID, f.Reporter.Email
	}
	if f.Parent != nil {
		issue.ParentKey = f.Parent.Key
	}
	// hierarchyLevel lives on issuetype; NamedID does not carry it, so read from
	// the raw fields map (source-neutral once stored as HierarchyLevel).
	if raw, ok := iss.Extra["issuetype"]; ok {
		var it struct {
			HierarchyLevel int `json:"hierarchyLevel"`
		}
		if err := json.Unmarshal(raw, &it); err == nil {
			issue.HierarchyLevel = it.HierarchyLevel
		}
	}
	if f.Resolution != nil {
		issue.Resolution = f.Resolution.Name
		issue.ResolutionID = f.Resolution.ID
	}
	applySprint(&issue, iss.Extra, sprintFieldID)

	rec := store.IssueRecord{Item: item, Issue: issue}
	if shouldFetchDevLinks(cfg, c) {
		rec.DevLinks, rec.DevLinksValid = devLinksFor(ctx, c, iss.ID)
	} else {
		// Cloud opt-out: a successful "we are not collecting these" drains.
		rec.DevLinksValid = true
	}
	for _, cm := range comments {
		sc := store.Comment{
			ID:         itemNS(cfg) + ":" + cm.ID,
			ExternalID: cm.ID,
			Author:     cm.Author.DisplayName,
			AuthorID:   cm.Author.AccountID,
			BodyADF:    cm.Body,
			BodyText:   jira.PlainText(cm.Body),
			CreatedAt:  jira.ISOTime(cm.Created),
			UpdatedAt:  jira.ISOTime(cm.Updated),
			JsdPublic:  cm.JsdPublic,
		}
		if cm.Visibility != nil {
			sc.VisibilityType = cm.Visibility.Type
			sc.VisibilityValue = cm.Visibility.Value
		}
		rec.Comments = append(rec.Comments, sc)
	}
	for _, at := range f.Attachment {
		rec.Attachments = append(rec.Attachments, store.Attachment{
			ID:         itemNS(cfg) + ":" + at.ID,
			ExternalID: at.ID,
			Filename:   at.Filename,
			MimeType:   at.MimeType,
			Size:       at.Size,
			Author:     at.Author.DisplayName,
			AuthorID:   at.Author.AccountID,
			CreatedAt:  jira.ISOTime(at.Created),
		})
	}
	for _, h := range histories {
		for i, it := range h.Items {
			field := changelogField(it)
			rec.Changelog = append(rec.Changelog, store.ChangeEntry{
				ID:        fmt.Sprintf("%s:%s:%d", itemNS(cfg), h.ID, i),
				At:        jira.ISOTime(h.Created),
				Author:    h.Author.DisplayName,
				AuthorID:  h.Author.AccountID,
				Field:     field,
				FromValue: it.FromString,
				FromID:    it.From,
				ToValue:   it.ToString,
				ToID:      it.To,
			})
		}
	}
	for _, l := range f.IssueLinks {
		switch {
		case l.OutwardIssue != nil:
			rec.Links = append(rec.Links, store.Link{Type: l.Type.Name, Direction: "outward", TargetKey: l.OutwardIssue.Key})
		case l.InwardIssue != nil:
			rec.Links = append(rec.Links, store.Link{Type: l.Type.Name, Direction: "inward", TargetKey: l.InwardIssue.Key})
		}
	}
	return rec, nil
}

// custom lands the configured aliases in issues.custom. Specs coalesce the
// first filled id; body-role values are included so the web can render them.
// Ids are configuration, never code (contracts/sync.md, "Field mapping").
func custom(specs []config.FieldSpec, extra map[string]json.RawMessage) map[string]any {
	return fields.CoalesceSpecs(specs, extra)
}

// fieldList is the Search fields= argument. Discovery-mode full sync uses
// *all so every custom value lands in raw; otherwise base fields plus every
// id from FieldSpecs and BodyFields (base first, de-duplicated).
func fieldList(cfg *config.Config, full bool) []string {
	if cfg != nil && full && len(cfg.Fields) == 0 && len(cfg.FieldMap) == 0 {
		return []string{"*all"}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, f := range baseFields {
		if !seen[f] {
			seen[f], out = true, append(out, f)
		}
	}
	if cfg == nil {
		return out
	}
	for _, s := range cfg.FieldSpecs() {
		for _, id := range s.IDs {
			if id != "" && !seen[id] {
				seen[id], out = true, append(out, id)
			}
		}
	}
	for _, id := range cfg.BodyFields {
		if id != "" && !seen[id] {
			seen[id], out = true, append(out, id)
		}
	}
	return out
}

// changelogFieldNames maps localized (and English) changelog display names to
// Jira fieldIds. I searched for an existing axis (priority_rank, FieldIDSlug,
// Derive field switch) and found none that translates a missing fieldId.
var changelogFieldNames = map[string]string{
	"status":           "status",
	"assignee":         "assignee",
	"reporter":         "reporter",
	"priority":         "priority",
	"summary":          "summary",
	"description":      "description",
	"issuetype":        "issuetype",
	"issue type":       "issuetype",
	"resolution":       "resolution",
	"labels":           "labels",
	"components":       "components",
	"fix version":      "fixVersions",
	"fix versions":     "fixVersions",
	"affects version":  "versions",
	"affects versions": "versions",
	"due date":         "duedate",
	"environment":      "environment",
	"상태":               "status",
	"담당자":              "assignee",
	"보고자":              "reporter",
	"우선순위":             "priority",
	"우선 순위":            "priority",
	"요약":               "summary",
	"설명":               "description",
	"이슈 유형":            "issuetype",
	"이슈 타입":            "issuetype",
	"해결":               "resolution",
	"해결책":              "resolution",
	"레이블":              "labels",
	"구성 요소":            "components",
	"수정 버전":            "fixVersions",
	"ステータス":            "status",
	"担当者":              "assignee",
}

// changelogField is the locale-stable changelog field key Derive and the feed
// switch on. fieldId wins; without it a stable name map is used; last resort
// is lowercase (English system fields).
func changelogField(it jira.HistoryItem) string {
	if it.FieldID != "" {
		return it.FieldID
	}
	if it.Field == "" {
		return ""
	}
	if id, ok := changelogFieldNames[it.Field]; ok {
		return id
	}
	lower := strings.ToLower(it.Field)
	if id, ok := changelogFieldNames[lower]; ok {
		return id
	}
	return lower
}

func names(list []jira.NamedID) []string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		out = append(out, n.Name)
	}
	return out
}

func ids(list []jira.NamedID) []string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		out = append(out, n.ID)
	}
	return out
}

// syncVersionCatalog refreshes the project version catalog for the sync
// scope. Failure is a warning: the issue pass already committed.
func syncVersionCatalog(ctx context.Context, c *jira.Client, db *store.DB, cfg *config.Config, opts Options) {
	keys, err := catalogProjects(ctx, db, cfg)
	if err != nil {
		opts.logf("versions: catalog skipped: %v", err)
		return
	}
	for _, pk := range keys {
		list, err := c.ProjectVersions(ctx, pk)
		if err != nil {
			opts.logf("versions: catalog for %s skipped: %v", pk, err)
			continue
		}
		rows := make([]store.VersionRow, 0, len(list))
		for _, v := range list {
			rows = append(rows, store.VersionRow{
				ID:          v.ID,
				ProjectKey:  pk,
				Name:        v.Name,
				Released:    v.Released,
				Archived:    v.Archived,
				ReleaseDate: v.ReleaseDate,
			})
		}
		if err := db.ReplaceProjectVersions(ctx, pk, rows); err != nil {
			opts.logf("versions: catalog for %s write skipped: %v", pk, err)
		}
	}
}

// catalogProjects is the version-catalog scope: configured projects when
// set, otherwise distinct project_key values already in the mirror (the
// same grain as fullJQL / reconcileJQL).
func catalogProjects(ctx context.Context, db *store.DB, cfg *config.Config) ([]string, error) {
	if cfg != nil && len(cfg.Projects) > 0 {
		out := make([]string, 0, len(cfg.Projects))
		seen := map[string]bool{}
		for _, pk := range cfg.Projects {
			pk = strings.TrimSpace(pk)
			if pk == "" || seen[pk] {
				continue
			}
			seen[pk] = true
			out = append(out, pk)
		}
		return out, nil
	}
	lites, err := db.IssueLites(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, l := range lites {
		pk := strings.TrimSpace(l.ProjectKey)
		if pk == "" || seen[pk] {
			continue
		}
		seen[pk] = true
		out = append(out, pk)
	}
	sort.Strings(out)
	return out, nil
}

func quoteList(keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%q", k))
	}
	return strings.Join(parts, ", ")
}

// fullJQL is the Search clause for a full pass. project empty → no project
// filter (the account's full visible set). Never emits `project in ()`.
func fullJQL(project string) string {
	if project == "" {
		return "ORDER BY updated DESC"
	}
	return fmt.Sprintf("project = %q ORDER BY updated DESC", project)
}

// incrementalJQL is the Search clause for an incremental pass. Empty projects
// drops the project filter; never emits `project in ()`.
func incrementalJQL(projects []string, watermark string) string {
	floor := jqlTime(watermark)
	if len(projects) == 0 {
		return fmt.Sprintf("updated >= %q ORDER BY updated ASC", floor)
	}
	return fmt.Sprintf("project in (%s) AND updated >= %q ORDER BY updated ASC",
		quoteList(projects), floor)
}

// reconcileJQL is the Search clause for the reconcile key scan. Empty projects
// drops the project filter; never emits `project in ()`.
func reconcileJQL(projects []string) string {
	if len(projects) == 0 {
		return "ORDER BY created ASC"
	}
	return fmt.Sprintf("project in (%s) ORDER BY created ASC", quoteList(projects))
}

// scopeLabel is the human scope fragment on sync start lines.
// GDK-464: kind=standalone has no account — name the seeded project, never
// "this account". cfg.IsStandalone() is the only discriminator.
func scopeLabel(cfg *config.Config) string {
	if cfg != nil && cfg.IsStandalone() {
		if len(cfg.Projects) > 0 {
			return strings.Join(cfg.Projects, ", ")
		}
		p := strings.TrimSpace(cfg.DefaultProject)
		if p == "" {
			p = origin.DefaultProjectKey
		}
		return p
	}
	if cfg == nil || len(cfg.Projects) == 0 {
		return "every project this account can see"
	}
	return strings.Join(cfg.Projects, ", ")
}

// sinceLabel is the human "changes since …" clock for incremental start lines
// (same floor jqlTime uses, dash-separated for readability).
func sinceLabel(watermark string) string {
	t, err := time.Parse(jira.Layout, watermark)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, watermark); err != nil {
			t = time.Now().Add(-24 * time.Hour)
		}
	}
	return t.Add(-overlap).Format("2006-01-02 15:04")
}

// formatCount renders n with ASCII thousands separators (6543 → "6,543").
// No external locale package — keep the binary dependency-free.
func formatCount(n int) string {
	if n < 0 {
		return "-" + formatCount(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	// Leading group may be shorter than 3; then groups of three.
	rem := len(s) % 3
	if rem == 0 {
		rem = 3
	}
	b.WriteString(s[:rem])
	for i := rem; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// jqlTime renders the watermark minus the overlap in JQL's minute-granularity
// form. It formats in the offset Jira itself stamped on `updated`, because a
// bare JQL timestamp is read in the account's timezone — formatting it as UTC
// would shift the floor forward on any account west of it and lose issues.
func jqlTime(watermark string) string {
	t, err := time.Parse(jira.Layout, watermark)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, watermark); err != nil {
			// Unreadable cursor: a day back is cheap and re-processing is a no-op.
			t = time.Now().Add(-24 * time.Hour)
		}
	}
	return t.Add(-overlap).Format("2006/01/02 15:04")
}

// shouldFetchDevLinks reports whether this pass should ask the origin for
// development-panel links. Cloud is opt-in (cfg.DevStatus). Standalone and
// paired-to-standalone (embedded / serve-passthrough issuetap) always fetch
// — the panel is local and the flag must not drain it (GDK-536).
func shouldFetchDevLinks(cfg *config.Config, c *jira.Client) bool {
	if cfg != nil && cfg.DevStatus {
		return true
	}
	if cfg != nil && cfg.IsStandalone() {
		return true
	}
	if c != nil && c.HTTP != nil {
		rt := c.HTTP.Transport
		if origin.TransportIsEmbedded(rt) || origin.TransportIsServe(rt) {
			return true
		}
	}
	return false
}

// devLinksFor reads the origin's development panel for one issue — summary
// first (one cheap call), detail only when it counts something. ok is false
// on fetch error so the rewrite preserves existing rows; a successful empty
// answer (n==0 or no PRs) is ok=true and drains.
func devLinksFor(ctx context.Context, c *jira.Client, issueID string) ([]store.DevLink, bool) {
	n, err := c.DevStatusPRCount(ctx, issueID)
	if err != nil {
		devStatusSkips.Add(1)
		return nil, false
	}
	if n == 0 {
		return nil, true
	}
	prs, err := c.DevStatusPRs(ctx, issueID)
	if err != nil {
		devStatusSkips.Add(1)
		return nil, false
	}
	return DevLinksFromPRs(prs), true
}

// DevLinksFromPRs maps dev-status pull requests onto mirror rows. Shared with
// `gadak dev link`'s post-write refresh so both paths store the same shape.
func DevLinksFromPRs(prs []jira.DevPR) []store.DevLink {
	out := make([]store.DevLink, 0, len(prs))
	for _, pr := range prs {
		if pr.URL == "" {
			continue
		}
		out = append(out, store.DevLink{
			Kind:       "pullrequest",
			ExternalID: pr.ID,
			URL:        pr.URL,
			Title:      pr.Name,
			Status:     strings.ToLower(pr.Status),
			UpdatedAt:  store.Now(),
		})
	}
	return out
}
