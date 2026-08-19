package sync

import (
	"context"
	"errors"
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

// linearPriorities is Linear's fixed priority vocabulary, most urgent first
// (integer 1..4; 0 is "No priority" and deliberately absent so it ranks 0).
// This is API vocabulary, not display configuration — Linear has no priority
// admin screen — so pinning it here keeps store.Derive's name-based
// priorityRank working with rank == the Linear integer.
var linearPriorities = []string{"Urgent", "High", "Medium", "Low"}

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
// constitution — the client has no mutations. Not mirrored this round, on
// purpose: state history (status_changed_at / reopen_count stay NULL until
// the history query shape is verified against the live API), attachments,
// and archived/deleted reconcile.
func RunLinear(ctx context.Context, cfg *config.Config, db *store.DB, opts Options) (Result, error) {
	var c *linear.Client
	return runSource(ctx, cfg, db, opts,
		sourceIdent{ID: LinearSourceID, Kind: "linear"},
		false, // no reconcile pass yet: absence is not proven this round
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
	var commentsTruncated, labelsTruncated int

	page := func(issues []linear.Issue) error {
		batch := store.Batch{
			Categories: map[string]string{},
			Priorities: linearPriorities,
			Records:    make([]store.IssueRecord, 0, len(issues)),
		}
		for _, iss := range issues {
			cat, known := linearCategory(iss.State.Type)
			if !known {
				unknownTypes[iss.State.Type]++
			}
			if iss.State.ID != "" {
				batch.Categories[iss.State.ID] = cat
			}
			if iss.Comments.PageInfo.HasNextPage {
				commentsTruncated++
			}
			if iss.Labels.PageInfo.HasNextPage {
				labelsTruncated++
			}
			batch.Records = append(batch.Records, buildLinearRecord(iss, cat))
		}
		changed, err := db.UpsertIssues(ctx, batch)
		if err != nil {
			return err
		}
		res.Fetched += len(issues)
		res.Changed += changed
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
			return record(ctx, db, LinearSourceID, err)
		}
	}

	// Truncation and unknown enum values are degradations, not failures —
	// but they must be loud, never silent.
	if commentsTruncated > 0 {
		opts.logf("linear: %d issues have more than %d comments — extra comments are not mirrored yet", commentsTruncated, linear.CommentsPageSize)
	}
	if labelsTruncated > 0 {
		opts.logf("linear: %d issues have more than %d labels — extra labels are not mirrored", labelsTruncated, linear.LabelsPageSize)
	}
	for _, typ := range sortedKeys(unknownTypes) {
		opts.logf("linear: unknown workflow state type %q on %d issues — mirrored as status_category new", typ, unknownTypes[typ])
	}

	res.Watermark = maxUTC
	return db.RecordSync(ctx, LinearSourceID, store.SyncResult{Watermark: maxUTC, FullSync: res.Full})
}

// buildLinearRecord maps one Linear issue onto the store's source-neutral
// record. Honest absences (MAPPING.md): issue_type / components / fixVersions
// / resolution / epic stay empty — Linear has no such concepts and synthetic
// constants are forbidden; description and comments are markdown and land in
// body_text only, never in the ADF columns; no changelog is supplied, so
// status_changed_at / resolved_at / reopen_count derive to NULL/0.
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
		// PriorityLabel verbatim; the integer is the stable id ("0".."4").
		// "No priority" is absent from linearPriorities, so it ranks 0.
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
	return rec
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
