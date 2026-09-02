package migrate

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/linear"
)

// ToLinear is the second destination of `gadak migrate` (GDK-1265): the
// same Doc that seeds the built-in tracker, emitted through the Linear
// write verbs instead. Mapping is by contract axis, never display name —
// status_category → WorkflowState.type, priority_rank → 0-4, link type →
// IssueRelationType. What Linear cannot receive (changelog, wiki pages,
// attachment bytes, real authorship) is reported, not dropped silently.
//
// Idempotency: every created issue ends with a `gadak-migrate: <KEY>`
// footer; a run first pages the team's issues and treats a footer match as
// already migrated. ponytail: a matched issue is skipped whole — a run that
// died between its create and its comments is not repaired by a re-run.
// Add per-child repair (compare comment counts from the scan) if that
// happens in practice.

// MigrateLabel marks every issue this path creates — the workspace-side
// signal that a row came from a migration, independent of the footer.
const MigrateLabel = "gadak-migrate"

// LinearOptions scopes ToLinear.
type LinearOptions struct {
	TeamKey string
	// Limit keeps the first N issues by key (0 = all); references to the
	// cut-off rest are dropped and counted like out-of-set ones.
	Limit int
	// DryRun computes the mapping and counts without one network call.
	DryRun bool
	// Progress receives one line per created issue ("NMS-1 → MID-42").
	Progress io.Writer
	// AttachmentURL turns a Jira attachment content id into the URL a
	// browser session at the source can open; nil or "" skips the link.
	AttachmentURL func(contentID string) string
}

// LinearReport is the run's honest half: counts, the mapping applied, and
// what did not travel. Counts reuse VerifyRow with Skipped = already
// present at the target before this run.
type LinearReport struct {
	Team        string      `json:"team"`
	DryRun      bool        `json:"dry_run"`
	Counts      []VerifyRow `json:"counts"`
	Mapping     []string    `json:"mapping"`
	NotMigrated []string    `json:"not_migrated"`
	Warnings    []string    `json:"warnings,omitempty"`
}

var footerRe = regexp.MustCompile(`(?m)^gadak-migrate: (\S+)\s*$`)

func migrateFooter(key string) string { return "\n\n---\ngadak-migrate: " + key }

func parseMigrateFooter(desc string) string {
	if m := footerRe.FindStringSubmatch(desc); m != nil {
		return m[1]
	}
	return ""
}

// stateTypesFor is the category → WorkflowState.type preference list
// (MAPPING.md "status_category", read in reverse). Unknown categories are
// filed as new — the same bucket the mirror gives an unknown Jira status.
func stateTypesFor(category string) []string {
	switch category {
	case "inprogress":
		return []string{"started"}
	case "done":
		return []string{"completed"}
	}
	return []string{"backlog", "unstarted", "triage"}
}

// pickLinearState returns the lowest-positioned state of the first
// preferred type present; the zero WorkflowState when none is.
func pickLinearState(states []linear.WorkflowState, category string) linear.WorkflowState {
	for _, typ := range stateTypesFor(category) {
		var best *linear.WorkflowState
		for i := range states {
			s := &states[i]
			if s.Type == typ && (best == nil || s.Position < best.Position) {
				best = s
			}
		}
		if best != nil {
			return *best
		}
	}
	return linear.WorkflowState{}
}

// linearPriority maps priority_rank (1 = most urgent, 0 = unset) onto
// Linear's 0-4. Ranks past 4 collapse to Low and are reported.
func linearPriority(rank int) (p int, collapsed bool) {
	if rank <= 0 {
		return 0, false
	}
	if rank > 4 {
		return 4, true
	}
	return rank, false
}

// linearRelationType keys on the source link-type name lowercased —
// Jira's link types are named, not categorized, so the name is the only
// axis there is (the display-name ban is about status/priority/type
// filters). Anything unrecognized is "related".
func linearRelationType(linkType string) string {
	switch strings.ToLower(linkType) {
	case "blocks":
		return "blocks"
	case "duplicate", "duplicates":
		return "duplicate"
	}
	return "related"
}

type relation struct{ from, to, typ string }

// linearRelations folds each issue's link rows into one relation per pair:
// outward on A = A→target, inward = target→A; symmetric types are
// order-normalized so a pair stored on both ends emits once.
func linearRelations(issues []Issue) []relation {
	seen := map[relation]bool{}
	var out []relation
	for _, is := range issues {
		for _, l := range is.Links {
			r := relation{typ: linearRelationType(l.Type)}
			if l.Outward != "" {
				r.from, r.to = is.Key, l.Outward
			} else {
				r.from, r.to = l.Inward, is.Key
			}
			if r.typ == "related" && r.from > r.to {
				r.from, r.to = r.to, r.from
			}
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	return out
}

// linearTime normalizes a mirror timestamp (UTC ms, or Jira's ±hhmm
// offset form) to the ISO-8601 UTC string Linear's DateTime takes; ""
// when unparsable, which omits the field.
func linearTime(s string) string {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000-0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05.000Z")
		}
	}
	return ""
}

// ToLinear emits doc into the team opt.TeamKey through client. st is the
// Build stats (for the not-migrated rows). Only doc.Issues travel; pages
// have no Linear counterpart.
func ToLinear(ctx context.Context, client *linear.Client, doc *Doc, st *Stats, opt LinearOptions) (*LinearReport, error) {
	if opt.TeamKey == "" {
		return nil, fmt.Errorf("migrate: --team <KEY> is required for --to linear")
	}
	rep := &LinearReport{Team: opt.TeamKey, DryRun: opt.DryRun}
	progress := opt.Progress
	if progress == nil {
		progress = io.Discard
	}

	issues := doc.Issues
	if opt.Limit > 0 && opt.Limit < len(issues) {
		issues = issues[:opt.Limit]
	}
	keyset := map[string]bool{}
	for _, is := range issues {
		keyset[is.Key] = true
	}
	// Re-cut parents and links to the (possibly limited) set; Build already
	// did this for the project set, so this only bites under --limit.
	droppedParents, droppedLinks := 0, 0
	for i := range issues {
		is := &issues[i]
		if is.Parent != "" && !keyset[is.Parent] {
			droppedParents++
			is.Parent = ""
		}
		kept := is.Links[:0:0]
		for _, l := range is.Links {
			if keyset[l.Inward+l.Outward] {
				kept = append(kept, l)
			} else {
				droppedLinks++
			}
		}
		is.Links = kept
	}

	// Source-side tallies and the mapping table (network-free).
	names := map[string]string{}
	emails := map[string]string{}
	for _, is := range issues {
		if is.Assignee != "" && is.AssigneeEmail != "" {
			emails[is.Assignee] = is.AssigneeEmail
		}
	}
	for _, u := range doc.Users {
		names[u.AccountID] = u.DisplayName
		if u.Email != "" {
			emails[u.AccountID] = u.Email
		}
	}
	typeName := map[string]string{}
	for _, t := range doc.IssueTypes {
		typeName[t.ID] = t.Name
	}
	var comments, parents, attachments, assigned, collapsed, historyRows int
	labelSet := map[string]bool{MigrateLabel: true}
	cats := map[string]bool{}
	ranks := map[int]bool{}
	for _, is := range issues {
		comments += len(is.Comments)
		attachments += len(is.Attachments)
		historyRows += len(is.History)
		if is.Parent != "" {
			parents++
		}
		if is.Assignee != "" {
			assigned++
		}
		if _, c := linearPriority(is.PriorityRank); c {
			collapsed++
		}
		cats[is.StatusCategory] = true
		ranks[is.PriorityRank] = true
		for _, l := range is.Labels {
			labelSet[l] = true
		}
		if n := typeName[is.Type]; n != "" {
			labelSet[n] = true
		}
	}
	relations := linearRelations(issues)

	for _, c := range sortedKeys(cats) {
		rep.Mapping = append(rep.Mapping, fmt.Sprintf("status_category %-10s → workflow state of type %s", c, strings.Join(stateTypesFor(c), "|")))
	}
	rankList := make([]int, 0, len(ranks))
	for r := range ranks {
		rankList = append(rankList, r)
	}
	sort.Ints(rankList)
	for _, r := range rankList {
		p, c := linearPriority(r)
		note := ""
		if c {
			note = "  (collapsed)"
		}
		rep.Mapping = append(rep.Mapping, fmt.Sprintf("priority_rank %d → priority %d%s", r, p, note))
	}
	for _, t := range doc.IssueTypes {
		rep.Mapping = append(rep.Mapping, fmt.Sprintf("issue type %-12s → label %q", t.Name, t.Name))
	}
	rep.Mapping = append(rep.Mapping,
		"link types      → relation blocks|duplicate|related (others → related; Linear moves a duplicate into its Duplicate state)",
		"comments        → body prefixed with `author · time` (Linear cannot post as someone else)",
		"description     → plain text + footer `gadak-migrate: <KEY>` (the idempotency key)")

	rep.NotMigrated = append(rep.NotMigrated,
		fmt.Sprintf("history %d (Linear has no changelog write API — reopen counts and time-in-status start over)", historyRows),
		fmt.Sprintf("authorship: reporter and the authors of %d comments are text in the body; the API key's user is the creator (timestamps are backdated)", comments),
		fmt.Sprintf("attachment bytes %d (linked by URL to the source when it has one; never uploaded)", attachments))
	if st != nil {
		if st.Pages > 0 {
			rep.NotMigrated = append(rep.NotMigrated, fmt.Sprintf("wiki pages %d (Linear has no wiki)", st.Pages))
		}
		if st.DevLinks+st.CustomIssues+st.SprintIssues > 0 {
			rep.NotMigrated = append(rep.NotMigrated, fmt.Sprintf("dev links %d, issues with custom fields %d, issues with sprints %d", st.DevLinks, st.CustomIssues, st.SprintIssues))
		}
	}
	if collapsed > 0 {
		rep.NotMigrated = append(rep.NotMigrated, fmt.Sprintf("priority ranks past 4 on %d issues collapsed to Low", collapsed))
	}
	if droppedParents+droppedLinks > 0 {
		rep.NotMigrated = append(rep.NotMigrated, fmt.Sprintf("parents %d and links %d pointing outside the migrated set", droppedParents, droppedLinks))
	}

	counts := func(created, skipped map[string]int) {
		row := func(metric string, source int) {
			rep.Counts = append(rep.Counts, VerifyRow{Metric: metric, Source: source,
				Migrated: created[metric] + skipped[metric], Skipped: skipped[metric]})
		}
		row("issues", len(issues))
		row("comments", comments)
		row("parents", parents)
		row("relations", len(relations))
		row("labels", len(labelSet))
		row("attachments", attachments)
		row("assignees", assigned)
	}
	if opt.DryRun {
		counts(nil, nil)
		return rep, nil
	}

	// --- network from here on ---
	teams, err := client.Teams(ctx)
	if err != nil {
		return nil, err
	}
	teamID := ""
	for _, t := range teams {
		if t.Key == opt.TeamKey {
			teamID = t.ID
		}
	}
	if teamID == "" {
		return nil, fmt.Errorf("migrate: no Linear team with key %q", opt.TeamKey)
	}
	states, err := client.WorkflowStates(ctx, teamID)
	if err != nil {
		return nil, err
	}
	stateFor := map[string]string{}
	for c := range cats {
		s := pickLinearState(states, c)
		if s.ID == "" {
			return nil, fmt.Errorf("migrate: team %s has no workflow state of type %s for status_category %q", opt.TeamKey, strings.Join(stateTypesFor(c), "|"), c)
		}
		stateFor[c] = s.ID
		rep.Mapping = append(rep.Mapping, fmt.Sprintf("status_category %-10s → %q (%s)", c, s.Name, s.Type))
	}

	// Existing rows by footer — the idempotency scan.
	existing := map[string]linear.Issue{}
	err = client.Issues(ctx, linear.IssueOpts{TeamID: teamID, IncludeArchived: true, PageSize: 250}, func(page []linear.Issue) error {
		for _, li := range page {
			if k := parseMigrateFooter(li.Description); k != "" {
				existing[k] = li
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Labels: reuse by name, create the rest on the team.
	created := map[string]int{}
	skipped := map[string]int{}
	labelID := map[string]string{}
	all, err := client.Labels(ctx)
	if err != nil {
		return nil, err
	}
	for _, l := range all {
		labelID[strings.ToLower(l.Name)] = l.ID
	}
	for _, name := range sortedKeys(labelSet) {
		if labelID[strings.ToLower(name)] != "" {
			skipped["labels"]++
			continue
		}
		if err := pace(ctx, client); err != nil {
			return nil, err
		}
		l, err := client.CreateLabel(ctx, teamID, name)
		if err != nil {
			return nil, fmt.Errorf("create label %q: %w", name, err)
		}
		labelID[strings.ToLower(name)] = l.ID
		created["labels"]++
	}

	// Assignees by email; a miss leaves the issue unassigned.
	userID := map[string]string{} // account id → Linear user id ("" = miss)
	resolveUser := func(acct string) string {
		if id, ok := userID[acct]; ok {
			return id
		}
		id := ""
		if email := emails[acct]; email != "" {
			if users, err := client.Users(ctx, email); err == nil {
				for _, u := range users {
					if strings.EqualFold(u.Email, email) {
						id = u.ID
					}
				}
			}
		}
		userID[acct] = id
		return id
	}

	// Create in parent-first order so parentId rides the create call.
	ids := map[string]string{} // source key → Linear issue id
	for k, li := range existing {
		if keyset[k] {
			ids[k] = li.ID
		}
	}
	var newKeys []string
	pending := make([]*Issue, 0, len(issues))
	for i := range issues {
		if li, ok := existing[issues[i].Key]; ok {
			// Counted from what the scan saw on the Linear row, not from
			// the source — an earlier run's misses stay visible.
			skipped["issues"]++
			skipped["comments"] += len(li.Comments.Nodes)
			skipped["attachments"] += len(li.Attachments.Nodes)
			if li.Parent != nil {
				skipped["parents"]++
			}
			if li.Assignee != nil {
				skipped["assignees"]++
			}
			continue
		}
		pending = append(pending, &issues[i])
	}
	createdAtChecked := false
	for len(pending) > 0 {
		var rest []*Issue
		progressed := false
		for _, is := range pending {
			if is.Parent != "" && ids[is.Parent] == "" {
				rest = append(rest, is)
				continue
			}
			if err := pace(ctx, client); err != nil {
				return nil, err
			}
			in := linear.IssueCreate{
				TeamID:      teamID,
				Title:       is.Summary,
				Description: is.Description + migrateFooter(is.Key),
				StateID:     stateFor[is.StatusCategory],
				ParentID:    ids[is.Parent],
				CreatedAt:   linearTime(is.Created),
				DueDate:     is.Duedate,
			}
			p, _ := linearPriority(is.PriorityRank)
			in.Priority = &p
			in.LabelIDs = []string{labelID[strings.ToLower(MigrateLabel)]}
			for _, l := range is.Labels {
				in.LabelIDs = append(in.LabelIDs, labelID[strings.ToLower(l)])
			}
			if n := typeName[is.Type]; n != "" {
				in.LabelIDs = append(in.LabelIDs, labelID[strings.ToLower(n)])
			}
			if is.Assignee != "" {
				if in.AssigneeID = resolveUser(is.Assignee); in.AssigneeID != "" {
					created["assignees"]++
				}
			}
			li, err := client.CreateIssue(ctx, in)
			if err != nil {
				return nil, fmt.Errorf("create %s: %w", is.Key, err)
			}
			if !createdAtChecked && in.CreatedAt != "" {
				createdAtChecked = true
				if linearTime(li.CreatedAt) != in.CreatedAt {
					rep.Warnings = append(rep.Warnings, "Linear did not honor createdAt on issueCreate — issues carry the migration time")
				}
			}
			ids[is.Key] = li.ID
			newKeys = append(newKeys, is.Key)
			created["issues"]++
			if is.Parent != "" {
				created["parents"]++
			}
			fmt.Fprintf(progress, "%s → %s\n", is.Key, li.Identifier)
			progressed = true
		}
		if !progressed {
			// Parent chain cannot resolve (a cycle, or a parent whose
			// create failed silently); file the rest as roots and say so.
			for _, is := range rest {
				droppedParents++
				is.Parent = ""
			}
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d parents could not be ordered and were dropped", len(rest)))
		}
		pending = rest
	}

	// Children of the issues created this run.
	isNew := map[string]bool{}
	for _, k := range newKeys {
		isNew[k] = true
	}
	for i := range issues {
		is := &issues[i]
		if !isNew[is.Key] {
			continue
		}
		for _, c := range is.Comments {
			if err := pace(ctx, client); err != nil {
				return nil, err
			}
			who := names[c.Author]
			if who == "" {
				who = c.Author
			}
			body := fmt.Sprintf("**%s · %s**\n\n%s", who, c.Created, c.Body)
			if _, err := client.CreateCommentAt(ctx, ids[is.Key], body, linearTime(c.Created)); err != nil {
				return nil, fmt.Errorf("comment on %s: %w", is.Key, err)
			}
			created["comments"]++
		}
		for _, a := range is.Attachments {
			url := a.SourceURL
			if url == "" && opt.AttachmentURL != nil {
				url = opt.AttachmentURL(a.ContentID)
			}
			if url == "" {
				continue
			}
			if err := pace(ctx, client); err != nil {
				return nil, err
			}
			if _, err := client.CreateAttachment(ctx, ids[is.Key], url, a.Filename); err != nil {
				return nil, fmt.Errorf("attachment on %s: %w", is.Key, err)
			}
			created["attachments"]++
		}
	}
	// A relation whose both ends pre-existed was made by the run that
	// created them; only pairs with a new end are new.
	for _, r := range relations {
		if !isNew[r.from] && !isNew[r.to] {
			skipped["relations"]++
			continue
		}
		if err := pace(ctx, client); err != nil {
			return nil, err
		}
		if err := client.CreateRelation(ctx, ids[r.from], ids[r.to], r.typ); err != nil {
			return nil, fmt.Errorf("relation %s %s %s: %w", r.from, r.typ, r.to, err)
		}
		created["relations"]++
	}

	counts(created, skipped)
	return rep, nil
}

// pace waits for the request window to reset when the server-stated budget
// is nearly spent (ratelimit.go). RATELIMITED replies are still retried by
// the client; this only keeps a long run from hitting them every call.
func pace(ctx context.Context, c *linear.Client) error {
	rl := c.LastRateLimit()
	if rl.RequestsLimit == 0 || rl.RequestsRemaining > 20 {
		return nil
	}
	wait := time.Until(time.UnixMilli(rl.RequestsResetMS))
	if wait <= 0 {
		return nil
	}
	if wait > time.Hour {
		wait = time.Hour
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}
