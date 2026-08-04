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
func Run(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) (Result, error) {
	var res Result
	if !cfg.HasCredential() {
		return res, errors.New("sync: site, email and token are required")
	}
	if len(cfg.Projects) == 0 {
		return res, errors.New("sync: no projects configured")
	}
	c := opts.Client
	if c == nil {
		c = jira.New(cfg.Site, cfg.Email, cfg.Token)
	}
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

	cats, err := c.Statuses(ctx)
	if err != nil {
		return res, record(db, res, err)
	}
	prios, err := c.Priorities(ctx)
	if err != nil {
		return res, record(db, res, err)
	}

	fields := fieldList(cfg)
	var maxUTC, maxRaw string
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
		opts.logf("page: %d issues, %d changed", len(issues), changed)
		if opts.Progress != nil {
			opts.Progress(res.Fetched, res.Changed)
		}
		return nil
	}

	if res.Full {
		for _, p := range cfg.Projects {
			opts.logf("full sync: %s", p)
			if err := c.Search(ctx, fmt.Sprintf("project = %q ORDER BY created ASC", p), fields, true, page); err != nil {
				return res, record(db, res, err)
			}
		}
	} else {
		jql := fmt.Sprintf("project in (%s) AND updated >= %q ORDER BY updated ASC",
			quoteList(cfg.Projects), jqlTime(state.Watermark))
		opts.logf("incremental: %s", jql)
		if err := c.Search(ctx, jql, fields, true, page); err != nil {
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
	opts.logf("done: %d fetched, %d changed, %d deleted", res.Fetched, res.Changed, res.Deleted)
	return res, nil
}

// Watch runs incremental sync on an interval and reconcile on a longer one. A
// transport failure is logged and retried on the next tick; a rejected
// credential ends the loop, because every further request would only burn rate
// budget.
func Watch(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) error {
	every := seconds(cfg.SyncIntervalSec, 60*time.Second)
	reconcileEvery := seconds(cfg.ReconcileIntervalSec, time.Hour)

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

func seconds(v int, fallback time.Duration) time.Duration {
	if v <= 0 {
		return fallback
	}
	return time.Duration(v) * time.Second
}

// record stores last_error and returns the error unchanged. It passes no
// watermark: a failed run must not advance it.
func record(db *store.DB, res Result, err error) error {
	_ = db.RecordSync(SourceID, store.SyncResult{Err: err})
	return err
}

// reconcile proves absence, which a search over an `updated >=` window cannot.
// It is a separate pass because its cost scales with total issue count.
func reconcile(ctx context.Context, c *jira.Client, db *store.DB, projects []string, opts Options) (int, error) {
	upstream := map[string]bool{}
	jql := fmt.Sprintf("project in (%s) ORDER BY created ASC", quoteList(projects))
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
	inScope := map[string]bool{}
	for _, p := range projects {
		inScope[strings.ToUpper(p)] = true
	}
	gone := []string{}
	for _, l := range lites {
		if inScope[strings.ToUpper(l.ProjectKey)] && !upstream[l.IssueKey] {
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
	for _, id := range cfg.BodyFields {
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
		Custom:          custom(cfg.FieldMap, iss.Extra),
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

// custom lands the configured aliases in issues.custom. Ids are configuration,
// never code (contracts/sync.md, "Field mapping").
func custom(fieldMap map[string]string, extra map[string]json.RawMessage) map[string]any {
	if len(fieldMap) == 0 {
		return nil
	}
	out := map[string]any{}
	for alias, id := range fieldMap {
		raw, ok := extra[id]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var v any
		if json.Unmarshal(raw, &v) != nil {
			continue
		}
		out[alias] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func fieldList(cfg *config.Config) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, f := range baseFields {
		if !seen[f] {
			seen[f], out = true, append(out, f)
		}
	}
	for _, id := range cfg.FieldMap {
		if id != "" && !seen[id] {
			seen[id], out = true, append(out, id)
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
