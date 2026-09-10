package migrate

import (
	"context"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
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

// migrateLabel marks every issue this path creates — the workspace-side
// signal that a row came from a migration, independent of the footer.
const migrateLabel = "gadak-migrate"

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
// what did not travel (reportCore). Counts reuse VerifyRow with Skipped =
// already present at the target before this run.
type LinearReport struct {
	Team   string `json:"team"`
	DryRun bool   `json:"dry_run"`
	reportCore
}

var footerRe = regexp.MustCompile(`(?m)^gadak-migrate: (\S+)\s*$`)

func migrateFooter(key string) string { return "\n\n---\ngadak-migrate: " + key }

func parseMigrateFooter(desc string) string {
	if m := footerRe.FindStringSubmatch(desc); m != nil {
		return m[1]
	}
	return ""
}

// pickLinearState returns the state a source status category lands on: the
// lowest-positioned workflow state whose type linear.StatusCategory files
// under that category — the same collapse the mirror applies when it reads
// the team back, so a migrated issue reports the category it left with.
//
// This used to keep its own type list (done → "completed" only), a second
// owner of the mapping that had already drifted: a team whose done states
// are all canceled-type failed with "no workflow state of type completed"
// while sync round-tripped those states fine (GDK-1314). Among the
// matching states, a "completed"-kind state still wins over a
// canceled/duplicate one when both exist: a migrated done issue should land
// on Done, not Canceled, and the cancel-ish types are the fallback for a
// team that has nothing else.
func pickLinearState(states []linear.WorkflowState, category string) linear.WorkflowState {
	switch category {
	case "inprogress", "done":
	default:
		// Unknown categories are filed as new — the same bucket the mirror
		// gives an unknown Jira status.
		category = "new"
	}
	var best *linear.WorkflowState
	for i := range states {
		s := &states[i]
		if cat, _ := linear.StatusCategory(s.Type); cat != category {
			continue
		}
		if best == nil || stateRank(*s) < stateRank(*best) ||
			(stateRank(*s) == stateRank(*best) && s.Position < best.Position) {
			best = s
		}
	}
	if best == nil {
		return linear.WorkflowState{}
	}
	return *best
}

// stateRank orders states of one category: the plain kinds first, the
// terminal-negative kinds (canceled, duplicate) last.
func stateRank(s linear.WorkflowState) int {
	switch s.Type {
	case "canceled", "duplicate":
		return 1
	}
	return 0
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

// linearRelations folds each issue's link rows into one relation per pair;
// the fold itself is shared with the Jira destination (jira.go,
// foldRelations) and only the vocabulary is Linear's.
func linearRelations(issues []Issue) []relation {
	return foldRelations(issues, linearRelationType, func(t string) bool { return t == "related" })
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
//
// The run is a pipeline on linearRun, the Linear sibling of ToJira's seven
// stages with one more write step: loadSource and sourceReport are the
// network-free dry-run product, and past the dry-run gate preflightTarget,
// scanExisting, ensureLabels, createIssues, completeIssues and
// createRelations run in that order. counts closes the report either way.
func ToLinear(ctx context.Context, client *linear.Client, doc *Doc, st *Stats, opt LinearOptions) (*LinearReport, error) {
	if opt.TeamKey == "" {
		return nil, fmt.Errorf("migrate: --team <KEY> is required for --to linear")
	}
	rep := &LinearReport{Team: opt.TeamKey, DryRun: opt.DryRun}
	progress := opt.Progress
	if progress == nil {
		progress = io.Discard
	}
	r := &linearRun{ctx: ctx, client: client, doc: doc, st: st, opt: opt, rep: rep, progress: progress}
	r.loadSource()
	r.sourceReport()
	if opt.DryRun {
		r.counts(nil, nil)
		return rep, nil
	}

	// --- network from here on ---
	if err := r.preflightTarget(); err != nil {
		return nil, err
	}
	if err := r.scanExisting(); err != nil {
		return nil, err
	}
	if err := r.ensureLabels(); err != nil {
		return nil, err
	}
	if err := r.createIssues(); err != nil {
		return nil, err
	}
	if err := r.completeIssues(); err != nil {
		return nil, err
	}
	if err := r.createRelations(); err != nil {
		return nil, err
	}
	// GDK-1318: the assignee skips the run could not verify are a report
	// row, not a silent gap — the assignees count row shows the mismatch
	// either way; this line says which part of it is "the lookup errored"
	// and not "the person is not on Linear".
	if len(r.userLookupFailed) > 0 {
		rep.NotMigrated = append(rep.NotMigrated,
			fmt.Sprintf("assignee lookups failed for %d accounts (%s) — their issues migrated unassigned; a lookup error, not a confirmed miss",
				len(r.userLookupFailed), strings.Join(slices.Sorted(maps.Keys(r.userLookupFailed)), ", ")))
	}
	r.counts(r.created, r.skipped)
	return rep, nil
}

// linearRun is one ToLinear execution: the inputs it started from, the
// report it fills, and the state one pipeline stage hands the next. See
// ToLinear for the stage order.
type linearRun struct {
	ctx      context.Context
	client   *linear.Client
	doc      *Doc
	st       *Stats
	opt      LinearOptions
	rep      *LinearReport
	progress io.Writer

	// loadSource: the migrated set and the source-side numbers.
	issues         []Issue
	keyset         map[string]bool
	droppedParents int
	droppedLinks   int
	tally          sourceTally
	relations      []relation
	names          map[string]string
	emails         map[string]string
	typeName       map[string]string
	labelSet       map[string]bool
	cats           map[string]bool
	rankList       []int
	collapsed      int

	// preflightTarget / ensureLabels: the team and what was mapped onto it.
	teamID   string
	stateFor map[string]string
	labelID  map[string]string

	// the write stages' state.
	existing         map[string]linear.Issue
	created          map[string]int
	skipped          map[string]int
	userID           map[string]string // account id → Linear user id ("" = miss or failed)
	userLookupFailed map[string]bool
	ids              map[string]string // source key → Linear issue id
	isNew            map[string]bool
}

// loadSource is stage 1: cut the doc to the migrated set and tally every
// source-side number the report and the count table need — the Jira
// destination's loadSource plus the axes only Linear has (emails for
// assignees, type names as labels). No network.
func (r *linearRun) loadSource() {
	r.issues, r.keyset, r.droppedParents, r.droppedLinks = scopeIssues(r.doc.Issues, r.opt.Limit)

	r.names = map[string]string{}
	r.emails = map[string]string{}
	for _, is := range r.issues {
		if is.Assignee != "" && is.AssigneeEmail != "" {
			r.emails[is.Assignee] = is.AssigneeEmail
		}
	}
	for _, u := range r.doc.Users {
		r.names[u.AccountID] = u.DisplayName
		if u.Email != "" {
			r.emails[u.AccountID] = u.Email
		}
	}
	r.typeName = map[string]string{}
	for _, t := range r.doc.IssueTypes {
		r.typeName[t.ID] = t.Name
	}
	r.tally = tallyIssues(r.issues)
	r.labelSet = map[string]bool{migrateLabel: true}
	r.cats = map[string]bool{}
	ranks := map[int]bool{}
	for _, is := range r.issues {
		if _, c := linearPriority(is.PriorityRank); c {
			r.collapsed++
		}
		r.cats[is.StatusCategory] = true
		ranks[is.PriorityRank] = true
		for _, l := range is.Labels {
			r.labelSet[l] = true
		}
		if n := r.typeName[is.Type]; n != "" {
			r.labelSet[n] = true
		}
	}
	r.relations = linearRelations(r.issues)
	r.rankList = make([]int, 0, len(ranks))
	for rk := range ranks {
		r.rankList = append(r.rankList, rk)
	}
	sort.Ints(r.rankList)
}

// sourceReport is stage 2: the network-free half of the report — the
// mapping lines that need no target catalog, and everything that did not
// travel.
func (r *linearRun) sourceReport() {
	for _, c := range slices.Sorted(maps.Keys(r.cats)) {
		r.rep.Mapping = append(r.rep.Mapping, fmt.Sprintf("status_category %-10s → the team's lowest workflow state in that category", c))
	}
	for _, rk := range r.rankList {
		p, c := linearPriority(rk)
		note := ""
		if c {
			note = "  (collapsed)"
		}
		r.rep.Mapping = append(r.rep.Mapping, fmt.Sprintf("priority_rank %d → priority %d%s", rk, p, note))
	}
	for _, t := range r.doc.IssueTypes {
		r.rep.Mapping = append(r.rep.Mapping, fmt.Sprintf("issue type %-12s → label %q", t.Name, t.Name))
	}
	r.rep.Mapping = append(r.rep.Mapping,
		"link types      → relation blocks|duplicate|related (others → related; Linear moves a duplicate into its Duplicate state)",
		"comments        → body prefixed with `author · time` (Linear cannot post as someone else)",
		"description     → plain text + footer `gadak-migrate: <KEY>` (the idempotency key)")

	r.rep.NotMigrated = append(r.rep.NotMigrated,
		fmt.Sprintf("history %d (Linear has no changelog write API — reopen counts and time-in-status start over)", r.tally.history),
		fmt.Sprintf("authorship: reporter and the authors of %d comments are text in the body; the API key's user is the creator (timestamps are backdated)", r.tally.comments),
		fmt.Sprintf("attachment bytes %d (linked by URL to the source when it has one; never uploaded)", r.tally.attachments))
	if r.st != nil {
		if r.st.Pages > 0 {
			r.rep.NotMigrated = append(r.rep.NotMigrated, fmt.Sprintf("wiki pages %d (Linear has no wiki)", r.st.Pages))
		}
		if r.st.DevLinks+r.st.CustomIssues+r.st.SprintIssues > 0 {
			r.rep.NotMigrated = append(r.rep.NotMigrated, fmt.Sprintf("dev links %d, issues with custom fields %d, issues with sprints %d", r.st.DevLinks, r.st.CustomIssues, r.st.SprintIssues))
		}
	}
	if r.collapsed > 0 {
		r.rep.NotMigrated = append(r.rep.NotMigrated, fmt.Sprintf("priority ranks past 4 on %d issues collapsed to Low", r.collapsed))
	}
	if r.droppedParents+r.droppedLinks > 0 {
		r.rep.NotMigrated = append(r.rep.NotMigrated, fmt.Sprintf("parents %d and links %d pointing outside the migrated set", r.droppedParents, r.droppedLinks))
	}
}

// counts appends the count table, the report's last rows in a dry run and
// in a real one (nil maps in the dry run: nothing was created or skipped).
func (r *linearRun) counts(created, skipped map[string]int) {
	r.rep.addCountRow("issues", len(r.issues), created, skipped)
	r.rep.addCountRow("comments", r.tally.comments, created, skipped)
	r.rep.addCountRow("parents", r.tally.parents, created, skipped)
	r.rep.addCountRow("relations", len(r.relations), created, skipped)
	r.rep.addCountRow("labels", len(r.labelSet), created, skipped)
	r.rep.addCountRow("attachments", r.tally.attachments, created, skipped)
	r.rep.addCountRow("assignees", r.tally.assigned, created, skipped)
}

// preflightTarget is stage 3: resolve the team and map each source
// status_category onto one of its workflow states — the target's catalogs
// before the first write.
func (r *linearRun) preflightTarget() error {
	teams, err := r.client.Teams(r.ctx)
	if err != nil {
		return err
	}
	for _, t := range teams {
		if t.Key == r.opt.TeamKey {
			r.teamID = t.ID
		}
	}
	if r.teamID == "" {
		return fmt.Errorf("migrate: no Linear team with key %q", r.opt.TeamKey)
	}
	states, err := r.client.WorkflowStates(r.ctx, r.teamID)
	if err != nil {
		return err
	}
	r.stateFor = map[string]string{}
	for c := range r.cats {
		s := pickLinearState(states, c)
		if s.ID == "" {
			return fmt.Errorf("migrate: team %s has no workflow state in status_category %q (no state whose type maps there)", r.opt.TeamKey, c)
		}
		r.stateFor[c] = s.ID
		r.rep.Mapping = append(r.rep.Mapping, fmt.Sprintf("status_category %-10s → %q (%s)", c, s.Name, s.Type))
	}
	return nil
}

// scanExisting is stage 4: the idempotency scan — every issue on the team
// whose description carries the migrate footer, keyed by source key.
func (r *linearRun) scanExisting() error {
	r.existing = map[string]linear.Issue{}
	err := r.client.Issues(r.ctx, linear.IssueOpts{TeamID: r.teamID, IncludeArchived: true, PageSize: 250}, func(page []linear.Issue) error {
		for _, li := range page {
			if k := parseMigrateFooter(li.Description); k != "" {
				r.existing[k] = li
			}
		}
		return nil
	})
	return err
}

// ensureLabels is stage 5, the first write: reuse the labels the team
// already has by name and create the rest on the team.
func (r *linearRun) ensureLabels() error {
	r.created = map[string]int{}
	r.skipped = map[string]int{}
	r.labelID = map[string]string{}
	all, err := r.client.Labels(r.ctx)
	if err != nil {
		return err
	}
	for _, l := range all {
		r.labelID[strings.ToLower(l.Name)] = l.ID
	}
	for _, name := range slices.Sorted(maps.Keys(r.labelSet)) {
		if r.labelID[strings.ToLower(name)] != "" {
			r.skipped["labels"]++
			continue
		}
		if err := pace(r.ctx, r.client); err != nil {
			return err
		}
		l, err := r.client.CreateLabel(r.ctx, r.teamID, name)
		if err != nil {
			return fmt.Errorf("create label %q: %w", name, err)
		}
		r.labelID[strings.ToLower(name)] = l.ID
		r.created["labels"]++
	}
	return nil
}

// resolveUser maps a source account id onto a Linear user id by email; a
// miss leaves the issue unassigned. A failed lookup used to be
// indistinguishable from a miss (any error skipped the assignment and the
// run reported success, GDK-1318) — the issue still migrates, because one
// person must not fail the whole run, but the account is remembered and the
// report names it, so a transport error never masquerades as "no such
// user" in the assignees row's mismatch.
func (r *linearRun) resolveUser(acct string) string {
	if id, ok := r.userID[acct]; ok {
		return id
	}
	id := ""
	if email := r.emails[acct]; email != "" {
		users, err := r.client.Users(r.ctx, email)
		if err != nil {
			r.userLookupFailed[acct] = true
		} else {
			for _, u := range users {
				if strings.EqualFold(u.Email, email) {
					id = u.ID
				}
			}
		}
	}
	r.userID[acct] = id
	return id
}

// createIssues is stage 6: create the not-yet-migrated issues in
// parent-first order so parentId rides the create call, assigning by email
// as it goes.
func (r *linearRun) createIssues() error {
	r.userID = map[string]string{}
	r.userLookupFailed = map[string]bool{}
	r.ids = map[string]string{} // source key → Linear issue id
	for k, li := range r.existing {
		if r.keyset[k] {
			r.ids[k] = li.ID
		}
	}
	var newKeys []string
	pending := make([]*Issue, 0, len(r.issues))
	for i := range r.issues {
		if li, ok := r.existing[r.issues[i].Key]; ok {
			// Counted from what the scan saw on the Linear row, not from
			// the source — an earlier run's misses stay visible.
			r.skipped["issues"]++
			r.skipped["comments"] += len(li.Comments.Nodes)
			r.skipped["attachments"] += len(li.Attachments.Nodes)
			if li.Parent != nil {
				r.skipped["parents"]++
			}
			if li.Assignee != nil {
				r.skipped["assignees"]++
			}
			continue
		}
		pending = append(pending, &r.issues[i])
	}
	createdAtChecked := false
	for len(pending) > 0 {
		var rest []*Issue
		progressed := false
		for _, is := range pending {
			if is.Parent != "" && r.ids[is.Parent] == "" {
				rest = append(rest, is)
				continue
			}
			if err := pace(r.ctx, r.client); err != nil {
				return err
			}
			in := linear.IssueCreate{
				TeamID:      r.teamID,
				Title:       is.Summary,
				Description: is.Description + migrateFooter(is.Key),
				StateID:     r.stateFor[is.StatusCategory],
				ParentID:    r.ids[is.Parent],
				CreatedAt:   linearTime(is.Created),
				DueDate:     is.Duedate,
			}
			p, _ := linearPriority(is.PriorityRank)
			in.Priority = &p
			in.LabelIDs = []string{r.labelID[strings.ToLower(migrateLabel)]}
			for _, l := range is.Labels {
				in.LabelIDs = append(in.LabelIDs, r.labelID[strings.ToLower(l)])
			}
			if n := r.typeName[is.Type]; n != "" {
				in.LabelIDs = append(in.LabelIDs, r.labelID[strings.ToLower(n)])
			}
			if is.Assignee != "" {
				if in.AssigneeID = r.resolveUser(is.Assignee); in.AssigneeID != "" {
					r.created["assignees"]++
				}
			}
			li, err := r.client.CreateIssue(r.ctx, in)
			if err != nil {
				return fmt.Errorf("create %s: %w", is.Key, err)
			}
			if !createdAtChecked && in.CreatedAt != "" {
				createdAtChecked = true
				if linearTime(li.CreatedAt) != in.CreatedAt {
					r.rep.Warnings = append(r.rep.Warnings, "Linear did not honor createdAt on issueCreate — issues carry the migration time")
				}
			}
			r.ids[is.Key] = li.ID
			newKeys = append(newKeys, is.Key)
			r.created["issues"]++
			if is.Parent != "" {
				r.created["parents"]++
			}
			fmt.Fprintf(r.progress, "%s → %s\n", is.Key, li.Identifier)
			progressed = true
		}
		if !progressed {
			// Parent chain cannot resolve (a cycle, or a parent whose
			// create failed silently); file the rest as roots and say so.
			for _, is := range rest {
				r.droppedParents++
				is.Parent = ""
			}
			r.rep.Warnings = append(r.rep.Warnings, fmt.Sprintf("%d parents could not be ordered and were dropped", len(rest)))
		}
		pending = rest
	}
	r.isNew = map[string]bool{}
	for _, k := range newKeys {
		r.isNew[k] = true
	}
	return nil
}

// completeIssues is stage 7: the per-issue writes that need the issue to
// exist — comments (backdated) and attachment links.
func (r *linearRun) completeIssues() error {
	for i := range r.issues {
		is := &r.issues[i]
		if !r.isNew[is.Key] {
			continue
		}
		for _, c := range is.Comments {
			if err := pace(r.ctx, r.client); err != nil {
				return err
			}
			who := r.names[c.Author]
			if who == "" {
				who = c.Author
			}
			body := fmt.Sprintf("**%s · %s**\n\n%s", who, c.Created, c.Body)
			if _, err := r.client.CreateCommentAt(r.ctx, r.ids[is.Key], body, linearTime(c.Created)); err != nil {
				return fmt.Errorf("comment on %s: %w", is.Key, err)
			}
			r.created["comments"]++
		}
		for _, a := range is.Attachments {
			url := a.SourceURL
			if url == "" && r.opt.AttachmentURL != nil {
				url = r.opt.AttachmentURL(a.ContentID)
			}
			if url == "" {
				continue
			}
			if err := pace(r.ctx, r.client); err != nil {
				return err
			}
			if _, err := r.client.CreateAttachment(r.ctx, r.ids[is.Key], url, a.Filename); err != nil {
				return fmt.Errorf("attachment on %s: %w", is.Key, err)
			}
			r.created["attachments"]++
		}
	}
	return nil
}

// createRelations is stage 8: the relations, typed
// blocks|duplicate|related.
func (r *linearRun) createRelations() error {
	// A relation whose both ends pre-existed was made by the run that
	// created them; only pairs with a new end are new.
	for _, rel := range r.relations {
		if !r.isNew[rel.from] && !r.isNew[rel.to] {
			r.skipped["relations"]++
			continue
		}
		if err := pace(r.ctx, r.client); err != nil {
			return err
		}
		if err := r.client.CreateRelation(r.ctx, r.ids[rel.from], r.ids[rel.to], rel.typ); err != nil {
			return fmt.Errorf("relation %s %s %s: %w", rel.from, rel.typ, rel.to, err)
		}
		r.created["relations"]++
	}
	return nil
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
