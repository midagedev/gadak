package sync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// LinearSourceID is the slug the Linear connector owns in sources / sync_state.
// Mirrored item ids are "linear:<uuid>" — no standalone namespace variant:
// issuetap has no Linear surface, and Linear ids are uuids that cannot collide
// with anything numeric (the GDK-241 rationale does not apply).
const LinearSourceID = "linear"

// linearRankList is the Priorities list store.Derive's name lookup consumes
// so the resulting rank equals Linear's integer id (1=Urgent … 4=Low, 0=unset).
// The issue's display label is placed at index id-1; other slots are bytes
// that cannot match a real label. Rank keys on the id, never the label
// (Linear's 3 is "Normal" or "Medium" depending on the workspace).
func linearRankList(id int, label string) []string {
	if id < 1 {
		return nil
	}
	out := make([]string, id)
	for i := 0; i < id-1; i++ {
		out[i] = "\x00" + strconv.Itoa(i+1)
	}
	out[id-1] = label
	return out
}

// linearCategory collapses a WorkflowState.type onto the mirror's
// status_category contract (new | inprogress | done). The enum is open —
// Linear added "duplicate" after the original six — so ok reports whether the
// type was known; unknown collapses to new (an issue can only be misread as
// open, never as silently done) and the caller reports it loudly.
// Never key on state names: they are display text ("진행 중").
func linearCategory(stateType string) (cat string, ok bool) {
	switch stateType {
	case "started":
		return "inprogress", true
	case "unstarted", "backlog", "triage":
		return "new", true
	case "completed", "canceled", "duplicate":
		return "done", true
	}
	return "new", false
}

// RunLinear does one Linear mirror pass: full or incremental. Read-only by
// constitution — the client has no mutations. stateHistory / history are
// still not mirrored (status_changed_at / reopen_count stay NULL).
func RunLinear(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) (Result, error) {
	if err := refuseIfFrozen(cfg); err != nil {
		return Result{}, err
	}
	var c *linear.Client
	return runSource(ctx, cfg, db, opts,
		sourceIdent{ID: LinearSourceID, Kind: KindLinear},
		true, // SupportsReconcile: full and opts.Reconcile, same as Jira
		"linear ",
		func() (string, usageTaker, error) {
			if cfg.Linear == nil {
				return "", nil, errors.New("sync: linear is not configured")
			}
			c = opts.LinearClient
			if c == nil {
				var err error
				c, err = origin.Linear(cfg)
				if err != nil {
					return "", nil, err
				}
			}
			// Display base, not the API endpoint: sources.base_url is what
			// link-building surfaces would join keys onto.
			return "https://linear.app", c, nil
		},
		func(state store.SyncState, res *Result) error {
			return runLinearPass(ctx, c, cfg, db, opts, state, res)
		},
	)
}

// runLinearPass is the Linear-specific body inside the shared runSource
// skeleton. One Issues call per configured team id (or one unscoped call),
// committed page by page; the watermark moves only over committed pages.
func runLinearPass(ctx context.Context, c *linear.Client, cfg *config.Config, db *store.DB, opts Options, state store.SyncState, res *Result) error {
	var maxUTC string // Linear stamps are ISO-8601 UTC ms: lexicographic max is chronological
	unknownTypes := map[string]int{}
	var commentsTruncated, labelsTruncated, attachmentsTruncated int
	seen := map[string]bool{}

	page := func(issues []linear.Issue) error {
		for i := range issues {
			if err := c.CompleteComments(ctx, &issues[i]); err != nil {
				return err
			}
			seen[issues[i].Identifier] = true
		}
		cats := map[string]string{}
		type recGroup struct {
			id    int
			label string
			recs  []store.IssueRecord
		}
		groups := map[string]*recGroup{}
		var order []string
		for _, iss := range issues {
			cat, known := linearCategory(iss.State.Type)
			if !known {
				unknownTypes[iss.State.Type]++
			}
			if iss.State.ID != "" {
				cats[iss.State.ID] = cat
			}
			if iss.Comments.PageInfo.HasNextPage {
				commentsTruncated++
			}
			if iss.Labels.PageInfo.HasNextPage {
				labelsTruncated++
			}
			if iss.Attachments.PageInfo.HasNextPage {
				attachmentsTruncated++
			}
			gk := strconv.Itoa(iss.Priority) + "\x00" + iss.PriorityLabel
			g, ok := groups[gk]
			if !ok {
				g = &recGroup{id: iss.Priority, label: iss.PriorityLabel}
				groups[gk] = g
				order = append(order, gk)
			}
			g.recs = append(g.recs, buildLinearRecord(iss, cat))
		}
		for _, gk := range order {
			g := groups[gk]
			changed, err := db.UpsertIssues(ctx, store.Batch{
				Categories: cats,
				Priorities: linearRankList(g.id, g.label),
				Records:    g.recs,
			})
			if err != nil {
				return err
			}
			res.Changed += changed
		}
		res.Fetched += len(issues)
		// Watermark only after the commit above, same rule as the Jira pass.
		for _, iss := range issues {
			if iss.UpdatedAt > maxUTC {
				maxUTC = iss.UpdatedAt
			}
		}
		opts.logf("  linear: %s issues", formatCount(res.Fetched))
		if opts.Progress != nil {
			opts.Progress(res.Fetched, res.Changed)
		}
		return nil
	}

	issueOpts := linear.IssueOpts{}
	if !res.Full {
		// gte re-reads the boundary issue; the upsert is a no-op. No overlap
		// window needed: Linear timestamps carry milliseconds, unlike JQL's
		// minute floor.
		issueOpts.UpdatedAfter = state.Watermark
	}
	teams := cfg.Linear.TeamIDs
	if len(teams) == 0 {
		teams = []string{""} // one unscoped pass: every team the key can see
	}
	for _, teamID := range teams {
		o := issueOpts
		o.TeamID = teamID
		if err := c.Issues(ctx, o, page); err != nil {
			return record(ctx, cfg, db, LinearSourceID, err)
		}
	}

	// Truncation and unknown enum values are degradations, not failures —
	// but they must be loud, never silent.
	if commentsTruncated > 0 {
		opts.logf("linear: %d issues still have more comments than were fetched", commentsTruncated)
	}
	if labelsTruncated > 0 {
		opts.logf("linear: %d issues have more than %d labels — extra labels are not mirrored", labelsTruncated, linear.LabelsPageSize)
	}
	if attachmentsTruncated > 0 {
		opts.logf("linear: %d issues have more than %d attachments — extra attachments are not mirrored", attachmentsTruncated, linear.AttachmentsPageSize)
	}
	for _, typ := range sortedKeys(unknownTypes) {
		opts.logf("linear: unknown workflow state type %q on %d issues — mirrored as status_category new", typ, unknownTypes[typ])
	}

	res.Watermark = maxUTC
	if err := db.RecordSync(ctx, LinearSourceID, store.SyncResult{Watermark: maxUTC, FullSync: res.Full}); err != nil {
		return err
	}
	if opts.Reconcile || res.Full {
		upstream := seen
		if !res.Full {
			listed, err := listLinearIdentifiers(ctx, c, cfg)
			if err != nil {
				return record(ctx, cfg, db, LinearSourceID, err)
			}
			upstream = listed
		}
		deleted, err := reconcileLinear(ctx, c, db, cfg, upstream, opts)
		res.Deleted = deleted
		if err != nil {
			return record(ctx, cfg, db, LinearSourceID, err)
		}
	}
	return nil
}

// buildLinearRecord maps one Linear issue onto the store's source-neutral
// record. Honest absences (MAPPING.md): issue_type / components / fixVersions
// / resolution / resolution_id / epic / security_level_id / security_level
// stay empty — Linear has no such concepts and synthetic constants are
// forbidden; description and comments
// are markdown and land in body_text only, never in the ADF columns; no
// changelog is supplied, so status_changed_at / resolved_at / reopen_count
// derive to NULL/0.
func buildLinearRecord(iss linear.Issue, cat string) store.IssueRecord {
	item := store.Item{
		ID:         LinearSourceID + ":" + iss.ID,
		SourceID:   LinearSourceID,
		Kind:       "issue",
		ExternalID: iss.ID,
		Key:        iss.Identifier,
		Title:      iss.Title,
		BodyText:   iss.Description,
		URL:        iss.URL,
		CreatedAt:  iss.CreatedAt,
		UpdatedAt:  iss.UpdatedAt,
	}
	if iss.Creator != nil {
		item.Author, item.AuthorID = iss.Creator.DisplayName, iss.Creator.ID
	}

	labels := make([]string, 0, len(iss.Labels.Nodes))
	for _, l := range iss.Labels.Nodes {
		if l.Name != "" {
			labels = append(labels, l.Name)
		}
	}
	sort.Strings(labels)

	issue := store.Issue{
		ProjectKey:     iss.Team.Key,
		Status:         iss.State.Name,
		StatusID:       iss.State.ID,
		StatusCategory: cat,
		// PriorityLabel is display-only; rank comes from the integer id via
		// linearRankList (id 0 → rank 0; id 1..4 → that rank).
		Priority:   iss.PriorityLabel,
		PriorityID: strconv.Itoa(iss.Priority),
		Labels:     labels,
		Duedate:    iss.DueDate,
	}
	if iss.Assignee != nil {
		issue.Assignee, issue.AssigneeID, issue.AssigneeEmail = iss.Assignee.DisplayName, iss.Assignee.ID, iss.Assignee.Email
	}
	if iss.Creator != nil {
		issue.Reporter, issue.ReporterID = iss.Creator.DisplayName, iss.Creator.ID
	}
	if iss.Parent != nil {
		issue.ParentKey = iss.Parent.Identifier
	}

	rec := store.IssueRecord{Item: item, Issue: issue}
	for _, cm := range iss.Comments.Nodes {
		sc := store.Comment{
			ID:         LinearSourceID + ":" + cm.ID,
			ExternalID: cm.ID,
			BodyText:   cm.Body, // markdown — BodyADF stays empty on purpose
			CreatedAt:  cm.CreatedAt,
			UpdatedAt:  cm.UpdatedAt,
		}
		if cm.User != nil {
			sc.Author, sc.AuthorID = cm.User.DisplayName, cm.User.ID
		}
		rec.Comments = append(rec.Comments, sc)
	}
	for _, at := range iss.Attachments.Nodes {
		size, mime := attachmentSizeMime(at.Metadata)
		rec.Attachments = append(rec.Attachments, store.Attachment{
			ID:         LinearSourceID + ":" + at.ID,
			ExternalID: at.ID,
			Filename:   at.Title,
			MimeType:   mime,
			Size:       size,
			CreatedAt:  at.CreatedAt,
			URL:        at.URL,
		})
	}
	return rec
}

func attachmentSizeMime(meta map[string]any) (int64, string) {
	if meta == nil {
		return 0, ""
	}
	var size int64
	switch v := meta["size"].(type) {
	case float64:
		size = int64(v)
	case int64:
		size = v
	case int:
		size = int64(v)
	}
	mime, _ := meta["mimeType"].(string)
	if mime == "" {
		mime, _ = meta["mimetype"].(string)
	}
	if mime == "" {
		mime, _ = meta["contentType"].(string)
	}
	return size, mime
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SyncLinearIssue is the write-through tail for one Linear issue: re-read it
// from the origin and commit the row, the Linear counterpart of SyncIssue.
// Force is on for the same reason as the Jira path — the caller just wrote,
// so an unchanged updatedAt must not skip the refresh.
func SyncLinearIssue(ctx context.Context, db *store.DB, c *linear.Client, key string) error {
	iss, err := c.Issue(ctx, key)
	if err != nil {
		return err
	}
	if err := c.CompleteComments(ctx, &iss); err != nil {
		return err
	}
	cat, _ := linearCategory(iss.State.Type)
	batch := store.Batch{
		Categories: map[string]string{},
		Priorities: linearRankList(iss.Priority, iss.PriorityLabel),
		Records:    []store.IssueRecord{buildLinearRecord(iss, cat)},
		Force:      true,
	}
	if iss.State.ID != "" {
		batch.Categories[iss.State.ID] = cat
	}
	_, err = db.UpsertIssues(ctx, batch)
	return err
}

// listLinearIdentifiers is a complete listing (no watermark) for incremental
// reconcile. Full sync uses the identifiers collected during the pass.
func listLinearIdentifiers(ctx context.Context, c *linear.Client, cfg *config.Config) (map[string]bool, error) {
	out := map[string]bool{}
	teams := cfg.Linear.TeamIDs
	if len(teams) == 0 {
		teams = []string{""}
	}
	for _, teamID := range teams {
		err := c.Issues(ctx, linear.IssueOpts{TeamID: teamID}, func(issues []linear.Issue) error {
			for _, iss := range issues {
				out[iss.Identifier] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// reconcileLinear deletes linear-source rows whose keys are not in upstream.
// Same policy as Jira: incremental does not reconcile unless opts.Reconcile;
// an empty upstream refuses to wipe the mirror. Linear's default listing
// omits archived issues, so archived is treated as deleted.
func reconcileLinear(ctx context.Context, c *linear.Client, db *store.DB, cfg *config.Config, upstream map[string]bool, opts Options) (int, error) {
	keys, err := db.KeysBySource(ctx, LinearSourceID)
	if err != nil {
		return 0, err
	}
	scope, err := linearTeamScope(ctx, c, cfg)
	if err != nil {
		return 0, err
	}
	lites, err := db.IssueLites(ctx)
	if err != nil {
		return 0, err
	}
	proj := map[string]string{}
	for _, l := range lites {
		proj[l.IssueKey] = l.ProjectKey
	}
	gone := []string{}
	for _, k := range keys {
		if scope != nil && !scope[proj[k]] {
			continue
		}
		if !upstream[k] {
			gone = append(gone, k)
		}
	}
	if len(gone) == 0 {
		return 0, nil
	}
	if len(upstream) == 0 {
		return 0, fmt.Errorf("reconcile: upstream reported no issues in scope while the mirror holds %d; refusing to empty it", len(gone))
	}
	opts.logf("linear reconcile: %d keys vanished upstream", len(gone))
	return db.DeleteItems(ctx, LinearSourceID, gone)
}

// linearTeamScope returns the team keys in cfg.Linear.TeamIDs. nil means
// unscoped (every linear row is a candidate).
func linearTeamScope(ctx context.Context, c *linear.Client, cfg *config.Config) (map[string]bool, error) {
	if cfg == nil || cfg.Linear == nil || len(cfg.Linear.TeamIDs) == 0 {
		return nil, nil
	}
	want := map[string]bool{}
	for _, id := range cfg.Linear.TeamIDs {
		want[id] = true
	}
	teams, err := c.Teams(ctx)
	if err != nil {
		return nil, err
	}
	scope := map[string]bool{}
	for _, t := range teams {
		if want[t.ID] {
			scope[t.Key] = true
		}
	}
	return scope, nil
}
