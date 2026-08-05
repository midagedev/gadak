// Package sync fills the mirror from Jira. It only ever writes to the mirror,
// never to Jira, and the rules it implements are in
// specs/000-product/contracts/sync.md.
package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/fields"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
)

// SourceID is the slug the Jira connector owns in `sources` and `sync_state`.
const SourceID = "jira"

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

type Options struct {
	Full      bool
	Reconcile bool
	// Log, when set, receives one line per committed page and per pass.
	Log func(string)
	// Progress, when set, is called once per committed page with the running
	// totals. It exists so a caller can report progress without parsing Log.
	Progress func(fetched, changed int)
	// Client is for tests and for a server that wants to share one; nil builds
	// one from cfg.
	Client *jira.Client
	// Notifier delivers OS desktop alerts for new personal-feed events after
	// each successful Watch cycle. Nil uses OSNotifier. Never aborts the loop.
	Notifier Notifier
}

type Result struct {
	Full      bool
	Fetched   int
	Changed   int
	Deleted   int
	Watermark string
}

func (o Options) logf(format string, args ...any) {
	if o.Log != nil {
		o.Log(fmt.Sprintf(format, args...))
	}
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
	var res Result
	started := time.Now()
	if !cfg.HasCredential() {
		return res, errors.New("sync: site, email and token are required")
	}
	c := opts.Client
	if c == nil {
		c = jira.New(cfg.Site, cfg.Email, cfg.Token)
	}
	// Flush call-volume counters into the mirror for every exit path of this
	// pass (success, transport failure, auth failure). Instrumentation must
	// never fail the sync itself — see flushAPIUsage. Watch also lands here
	// once per cycle, so one-shot and watch share this single flush point.
	defer flushAPIUsage(db, c, opts.logf)
	if err := db.UpsertSource(store.Source{ID: SourceID, Kind: "jira", BaseURL: c.BaseURL()}); err != nil {
		return res, err
	}
	state, err := db.SyncState(SourceID)
	if err != nil {
		return res, err
	}
	// No watermark means nothing has been mirrored yet, so incremental has no
	// floor to start from.
	res.Full = opts.Full || state.Watermark == ""

	// Discovery mode: no configured custom fields yet — first full sync pulls
	// *all so raw carries every custom value for auto-configuration.
	discoveryMode := len(cfg.Fields) == 0 && len(cfg.FieldMap) == 0

	cats, err := c.Statuses(ctx)
	if err != nil {
		return res, record(db, res, err)
	}
	prios, err := c.Priorities(ctx)
	if err != nil {
		return res, record(db, res, err)
	}

	fieldIDs := fieldList(cfg, res.Full)
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
			r, err := build(ctx, c, cfg, iss)
			if err != nil {
				return err
			}
			batch.Records = append(batch.Records, r)
		}
		changed, err := db.UpsertIssues(batch)
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
			beginSearch("full sync: every project this account can see", jql, true)
			if err := c.Search(ctx, jql, fieldIDs, true, page); err != nil {
				return res, record(db, res, err)
			}
		} else {
			for _, p := range cfg.Projects {
				jql := fullJQL(p)
				// Newest activity first: the mirror is usable the moment the first
				// page lands, instead of after every historical issue. The watermark
				// is max(updated) over fetched pages, so ordering does not affect it.
				beginSearch("full sync: "+p, jql, true)
				if err := c.Search(ctx, jql, fieldIDs, true, page); err != nil {
					return res, record(db, res, err)
				}
			}
		}
	} else {
		jql := incrementalJQL(cfg.Projects, state.Watermark)
		beginSearch("incremental: "+scopeLabel(cfg.Projects)+" — changes since "+sinceLabel(state.Watermark), jql, false)
		if discoveryMode {
			opts.logf("tip: run `scry sync --full` once to auto-configure custom fields")
		}
		if err := c.Search(ctx, jql, fieldIDs, true, page); err != nil {
			return res, record(db, res, err)
		}
	}

	res.Watermark = maxRaw
	if err := db.RecordSync(SourceID, store.SyncResult{Watermark: maxRaw, FullSync: res.Full}); err != nil {
		return res, err
	}

	if opts.Reconcile || res.Full {
		deleted, err := reconcile(ctx, c, db, cfg.Projects, opts)
		res.Deleted = deleted
		if err != nil {
			return res, record(db, res, err)
		}
	}

	// Custom-field discovery / field_usage refresh (before the done line).
	if discoveryMode && res.Full {
		if err := runDiscovery(ctx, c, cfg, db, opts); err != nil {
			// cfg.Save failure propagates; other discovery errors are warnings.
			return res, err
		}
	} else if (res.Full || opts.Reconcile) && len(cfg.FieldSpecs()) > 0 {
		if err := refreshFieldUsage(db, cfg); err != nil {
			opts.logf("fields: usage refresh skipped: %v", err)
		}
	}

	elapsed := time.Since(started).Round(time.Second)
	opts.logf("done: %s fetched, %s changed, %s deleted in %s",
		formatCount(res.Fetched), formatCount(res.Changed), formatCount(res.Deleted), elapsed)
	return res, nil
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
	fill, err := scanMirrorFill(db)
	if err != nil {
		opts.logf("fields: discovery skipped: %v", err)
		return nil
	}
	specs := fields.Discover(catalog, fill, cfg.Fields)
	cfg.Fields = specs
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("fields: save discovered specs: %w", err)
	}
	n, err := reingestFromConfig(db, cfg)
	if err != nil {
		opts.logf("fields: discovery skipped: %v", err)
		return nil
	}
	if err := refreshFieldUsage(db, cfg); err != nil {
		opts.logf("fields: usage refresh skipped: %v", err)
	}
	if len(specs) == 0 {
		opts.logf("fields: no custom fields in use")
		return nil
	}
	opts.logf("fields: discovered %s custom fields in use — labels, filters, and editors configured", formatCount(len(specs)))
	opts.logf("fields: backfilled %s issues from the mirror (no re-download)", formatCount(n))
	return nil
}

// scanMirrorFill counts filled field ids across the mirror's stored raw JSON.
func scanMirrorFill(db *store.DB) (map[string]int, error) {
	fill := map[string]int{}
	err := db.ScanFieldFill(func(_ string, fieldVals map[string]json.RawMessage) error {
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
func reingestFromConfig(db *store.DB, cfg *config.Config) (int, error) {
	specs := cfg.FieldSpecs()
	bodyIDs := fields.BodyFieldIDs(cfg.BodyFields, specs)
	return db.ReingestCustom(fields.SpecIDsFrom(specs), bodyIDs)
}

// refreshFieldUsage recomputes the field_usage table from issues.custom.
func refreshFieldUsage(db *store.DB, cfg *config.Config) error {
	specs := cfg.FieldSpecs()
	if len(specs) == 0 {
		return db.ReplaceFieldUsage(nil)
	}
	aliases := make([]string, 0, len(specs))
	for _, s := range specs {
		if s.Alias != "" {
			aliases = append(aliases, s.Alias)
		}
	}
	rows, err := db.ComputeFieldUsage(aliases)
	if err != nil {
		return err
	}
	return db.ReplaceFieldUsage(rows)
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
// credential ends the loop, because every further request would only burn rate
// budget. After each successful cycle, new personal-feed events may produce one
// OS desktop notification (see notifyAfterSync); notification failures never
// stop the loop.
func Watch(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) error {
	every := time.Duration(cfg.EffectiveSyncIntervalSec()) * time.Second
	reconcileEvery := time.Duration(cfg.EffectiveReconcileIntervalSec()) * time.Second

	tick := time.NewTicker(every)
	defer tick.Stop()
	rtick := time.NewTicker(reconcileEvery)
	defer rtick.Stop()

	o := opts
	for {
		if _, err := Run(ctx, cfg, db, o); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, jira.ErrAuth) {
				return err
			}
			opts.logf("sync failed: %v", err)
		} else if err := notifyAfterSync(db, cfg, o.Notifier); err != nil {
			// Desktop notify is best-effort: never abort the watch loop.
			opts.logf("notify: %v", err)
		}
		o.Full, o.Reconcile = false, false
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		case <-rtick.C:
			o.Reconcile = true
		}
	}
}

// record stores last_error and returns the error unchanged. It passes no
// watermark: a failed run must not advance it.
func record(db *store.DB, res Result, err error) error {
	_ = db.RecordSync(SourceID, store.SyncResult{Err: err})
	return err
}

// flushAPIUsage takes the client's process-local counters and accumulates them
// into api_usage for the current UTC day. One-shot `scry sync` and each Watch
// cycle both go through Run, so this is the single flush point.
//
// A flush failure is logged and swallowed: rate-limit visibility must not break
// the sync that produced the traffic.
func flushAPIUsage(db *store.DB, c *jira.Client, logf func(string, ...any)) {
	if db == nil || c == nil {
		return
	}
	u := c.TakeUsage()
	if u.Requests == 0 && u.Throttled == 0 && u.ServerErrors == 0 && u.Retries == 0 && u.WaitMS == 0 {
		return
	}
	delta := store.APIUsageDelta{
		Requests:     u.Requests,
		Throttled:    u.Throttled,
		ServerErrors: u.ServerErrors,
		Retries:      u.Retries,
		WaitMS:       u.WaitMS,
	}
	if !u.LastThrottledAt.IsZero() {
		delta.LastThrottledAt = u.LastThrottledAt.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	day := time.Now().UTC().Format("2006-01-02")
	if err := db.AddAPIUsage(day, delta); err != nil {
		if logf != nil {
			logf("api usage flush: %v", err)
		}
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

	lites, err := db.IssueLites()
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
	return db.DeleteItems(SourceID, gone)
}

// build maps one Jira issue onto the store's record, fetching the children the
// search response truncated.
func build(ctx context.Context, c *jira.Client, cfg *config.Config, iss jira.Issue) (store.IssueRecord, error) {
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
		ID:         SourceID + ":" + iss.ID,
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
		AffectsVersions: names(f.Versions),
		EnvironmentText: jira.PlainText(f.Environment),
		Duedate:         f.Duedate,
		DescriptionADF:  f.Description,
		Custom:          custom(cfg.FieldSpecs(), iss.Extra),
		Raw:             iss.Raw,
	}
	if f.Priority != nil {
		issue.Priority = f.Priority.Name
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
	if f.Resolution != nil {
		issue.Resolution = f.Resolution.Name
	}

	rec := store.IssueRecord{Item: item, Issue: issue}
	for _, cm := range comments {
		rec.Comments = append(rec.Comments, store.Comment{
			ID:         SourceID + ":" + cm.ID,
			ExternalID: cm.ID,
			Author:     cm.Author.DisplayName,
			AuthorID:   cm.Author.AccountID,
			BodyADF:    cm.Body,
			BodyText:   jira.PlainText(cm.Body),
			CreatedAt:  jira.ISOTime(cm.Created),
			UpdatedAt:  jira.ISOTime(cm.Updated),
		})
	}
	for _, at := range f.Attachment {
		rec.Attachments = append(rec.Attachments, store.Attachment{
			ID:         SourceID + ":" + at.ID,
			ExternalID: at.ID,
			Filename:   at.Filename,
			MimeType:   at.MimeType,
			Size:       at.Size,
			Author:     at.Author.DisplayName,
			CreatedAt:  jira.ISOTime(at.Created),
		})
	}
	for _, h := range histories {
		for i, it := range h.Items {
			field := it.FieldID
			if field == "" {
				field = strings.ToLower(it.Field)
			}
			rec.Changelog = append(rec.Changelog, store.ChangeEntry{
				ID:        fmt.Sprintf("%s:%s:%d", SourceID, h.ID, i),
				At:        jira.ISOTime(h.Created),
				Author:    h.Author.DisplayName,
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

func names(list []jira.NamedID) []string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		out = append(out, n.Name)
	}
	return out
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

// scopeLabel is the human scope fragment on incremental start lines.
func scopeLabel(projects []string) string {
	if len(projects) == 0 {
		return "every project this account can see"
	}
	return strings.Join(projects, ", ")
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
