package migrate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/statuscat"
)

// ToJira is the third destination of `gadak migrate` (GDK-378) and the one
// that closes the built-in tracker's exit: a workspace that grew up here
// leaves for a real Jira project instead of being deleted by
// `init --replace-local`. It is the Linear sibling's shape
// (internal/migrate/linear.go) aimed at the Jira write verbs, and it maps
// by contract axis rather than display name — status_category picks the
// transition, priority_rank indexes the target catalog, the issue type is
// matched by the name createmeta itself reports.
//
// Idempotency is the sibling's: every created description ends with a
// `gadak-migrate: <KEY>` footer, and a run first asks the target project
// for issues carrying one. Like the Linear path, a matched issue is skipped
// whole — a run that died between its create and its comments is not
// repaired by a re-run.
//
// What Jira cannot receive on this path (change history, wiki pages, real
// authorship, dev links, custom fields, sprints) is reported, never dropped
// in silence.

// --- helpers shared with the Linear destination -------------------------
//
// These live here rather than in linear.go because linear.go is the older
// path and this one is written against it; if a third destination arrives
// they want a file of their own.

// scopeIssues applies opt.Limit and re-cuts parents and links to the kept
// set. Build already cut to the project set, so this only bites under
// --limit. The returned slice is a copy: the caller's Doc is not edited.
func scopeIssues(all []Issue, limit int) (issues []Issue, keyset map[string]bool, droppedParents, droppedLinks int) {
	issues = all
	if limit > 0 && limit < len(issues) {
		issues = issues[:limit]
	}
	issues = append(issues[:0:0], issues...)
	keyset = map[string]bool{}
	for _, is := range issues {
		keyset[is.Key] = true
	}
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
	return issues, keyset, droppedParents, droppedLinks
}

// sourceTally is the network-free half of both destinations' count table.
type sourceTally struct {
	comments    int
	parents     int
	attachments int
	assigned    int
	history     int
}

func tallyIssues(issues []Issue) sourceTally {
	var t sourceTally
	for _, is := range issues {
		t.comments += len(is.Comments)
		t.attachments += len(is.Attachments)
		t.history += len(is.History)
		if is.Parent != "" {
			t.parents++
		}
		if is.Assignee != "" {
			t.assigned++
		}
	}
	return t
}

// foldRelations folds each issue's link rows into one relation per pair:
// outward on A = A→target, inward = target→A. normalize maps the source
// link-type name onto the destination's vocabulary; symmetric names the
// types whose two ends are interchangeable, so a pair stored on both ends
// with opposite directions still emits once.
func foldRelations(issues []Issue, normalize func(string) string, symmetric func(string) bool) []relation {
	seen := map[relation]bool{}
	var out []relation
	for _, is := range issues {
		for _, l := range is.Links {
			r := relation{typ: normalize(l.Type)}
			if l.Outward != "" {
				r.from, r.to = is.Key, l.Outward
			} else {
				r.from, r.to = l.Inward, is.Key
			}
			if symmetric(r.typ) && r.from > r.to {
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

// --- Jira destination ---------------------------------------------------

// JiraOptions scopes ToJira.
type JiraOptions struct {
	// ProjectKey is the target project in the Jira workspace this command
	// runs in. Required — Jira has no "default project".
	ProjectKey string
	// Limit keeps the first N issues by key (0 = all); references to the
	// cut-off rest are dropped and counted like out-of-set ones.
	Limit int
	// DryRun computes the mapping and counts without one network call.
	DryRun bool
	// SkipAttachments keeps attachment metadata in the count table but
	// uploads no bytes.
	SkipAttachments bool
	// Progress receives one line per created issue ("NMS-1 → ABC-42").
	Progress io.Writer
	// Fetch opens one attachment's bytes at the source origin; they are
	// streamed straight into the upload, never held (GDK-1618 shape). nil
	// falls back to bytes already carried in the export (DataBase64 / Text
	// — fixtures and tests), and an attachment with neither is skipped.
	Fetch StreamFetch
}

// JiraReport is the run's honest half, in the Linear report's shape:
// counts, the mapping applied, and what did not travel (reportCore).
type JiraReport struct {
	Project string `json:"project"`
	DryRun  bool   `json:"dry_run"`
	reportCore
}

// jiraLinkName normalizes a source link-type name for matching against the
// target site's catalog. Jira's link types are named, not categorized, so
// the name is the only axis there is on either end (the display-name ban is
// about status/priority/type filters, which do have ids).
func jiraLinkName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// jiraSymmetricLink names the link families whose two ends are
// interchangeable, so the same pair declared from both sides emits once.
func jiraSymmetricLink(name string) bool {
	switch name {
	case "relates", "related", "duplicate", "duplicates":
		return true
	}
	return false
}

// jiraPriorityIndex maps a source priority_rank (1 = most urgent, 0 =
// unset) onto a 0-based index into the target site's priority catalog,
// which is also ordered most urgent first (jira.PriorityCatalog). -1 means
// "leave unset, land on the project default".
//
// Same-size catalogs are position-for-position. Different sizes are
// proportional over the closed interval, so the two ends stay pinned: the
// most urgent rank lands on the most urgent priority and the least urgent
// on the least urgent, whichever catalog is longer.
func jiraPriorityIndex(rank, srcN, dstN int) int {
	if rank <= 0 || dstN <= 0 {
		return -1
	}
	if srcN <= 1 {
		if rank-1 < dstN {
			return rank - 1
		}
		return dstN - 1
	}
	if srcN == dstN {
		if rank-1 < dstN {
			return rank - 1
		}
		return dstN - 1
	}
	idx := int(math.Round(float64(rank-1) * float64(dstN-1) / float64(srcN-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= dstN {
		idx = dstN - 1
	}
	return idx
}

// adfNodes returns a document's top-level content array, or nil when raw is
// not an ADF document (empty, a bare string, unparsable).
func adfNodes(raw string) []any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var doc struct {
		Type    string `json:"type"`
		Content []any  `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil || doc.Type != "doc" {
		return nil
	}
	return doc.Content
}

// adfDoc wraps top-level nodes as an ADF document.
func adfDoc(nodes []any) json.RawMessage {
	b, err := json.Marshal(map[string]any{"type": "doc", "version": 1, "content": nodes})
	if err != nil {
		return jira.Doc("", nil)
	}
	return b
}

// jiraBodyNodes is migrate.go's body rule on the write side: the origin's
// ADF when it is a document, otherwise the text wrapped as paragraphs (what
// jira.Doc builds).
func jiraBodyNodes(text, bodyADF string) []any {
	if nodes := adfNodes(bodyADF); nodes != nil {
		return nodes
	}
	return adfNodes(string(jira.Doc(text, nil)))
}

// jiraDescription is the body plus the idempotency footer, as its own
// paragraph after a rule so a reader can see where the migration's mark
// starts. The footer's plain text is what parseMigrateFooter reads back off
// the scan, so it must be a line of its own — a paragraph is (adf.PlainText
// ends every block node with a newline).
func jiraDescription(is Issue) json.RawMessage {
	nodes := jiraBodyNodes(is.Description, is.DescriptionADF)
	nodes = append(append([]any{}, nodes...),
		map[string]any{"type": "rule"},
		map[string]any{"type": "paragraph", "content": []any{
			map[string]any{"type": "text", "text": "gadak-migrate: " + is.Key},
		}})
	return adfDoc(nodes)
}

// jiraCommentBody carries the source author and time as a bold first line —
// the Linear sibling's `**author · time**` header, built as an ADF strong
// mark because Jira stores a document, not markdown, and the asterisks
// would otherwise arrive as literal text.
func jiraCommentBody(header string, c Comment) json.RawMessage {
	nodes := []any{map[string]any{"type": "paragraph", "content": []any{
		map[string]any{"type": "text", "text": header,
			"marks": []any{map[string]any{"type": "strong"}}},
	}}}
	return adfDoc(append(nodes, jiraBodyNodes(c.Body, c.BodyADF)...))
}

// jiraMigrateJQL is the idempotency scan: everything in the target project
// whose description mentions the footer marker. `~` is Jira's text match,
// so it over-selects (a body that merely says the word) — parseMigrateFooter
// is what decides, and an over-selecting query is the safe direction.
func jiraMigrateJQL(project string) string {
	return fmt.Sprintf("project = %q AND description ~ %q ORDER BY created ASC", project, migrateLabel)
}

// ToJira emits doc into project opt.ProjectKey through client. st is the
// Build stats (for the not-migrated rows). Only doc.Issues travel; wiki
// pages would need a Confluence space and are reported instead.
//
// The run is a seven-stage pipeline on jiraRun: loadSource and sourceReport
// are the network-free dry-run product, and past the dry-run gate
// preflightTarget, scanExisting, createIssues, completeIssues and createLinks
// run in that order. counts closes the report either way. The stage methods
// carry the per-step rationale.
func ToJira(ctx context.Context, client *jira.Client, doc *Doc, st *Stats, opt JiraOptions) (*JiraReport, error) {
	if opt.ProjectKey == "" {
		return nil, fmt.Errorf("migrate: --project <KEY> is required for --to jira")
	}
	rep := &JiraReport{Project: opt.ProjectKey, DryRun: opt.DryRun}
	progress := opt.Progress
	if progress == nil {
		progress = io.Discard
	}
	r := &jiraRun{ctx: ctx, client: client, doc: doc, st: st, opt: opt, rep: rep, progress: progress}
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
	if err := r.createIssues(); err != nil {
		return nil, err
	}
	if err := r.completeIssues(); err != nil {
		return nil, err
	}
	if err := r.createLinks(); err != nil {
		return nil, err
	}
	r.counts(r.created, r.skipped)
	return rep, nil
}

// jiraRun is one ToJira execution: the inputs it started from, the report it
// fills, and the state one pipeline stage hands the next. See ToJira for the
// stage order.
type jiraRun struct {
	ctx      context.Context
	client   *jira.Client
	doc      *Doc
	st       *Stats
	opt      JiraOptions
	rep      *JiraReport
	progress io.Writer

	// loadSource: the migrated set and the source-side numbers.
	issues         []Issue
	keyset         map[string]bool
	droppedParents int
	droppedLinks   int
	tally          sourceTally
	relations      []relation
	names          map[string]string
	typeName       map[string]string
	cats           map[string]bool
	labelSet       map[string]bool
	rankList       []int

	// preflightTarget: the target's catalogs and what was mapped onto them.
	types         []jira.CreateMetaIssueType
	defaultType   jira.CreateMetaIssueType
	priorities    []jira.NamedID
	srcPriorities int
	linkTypeID    map[string]string
	relatesID     string

	// the write stages' tallies.
	existing  map[string]string // source key → target key
	created   map[string]int
	skipped   map[string]int
	newLabels map[string]bool
	oldLabels map[string]bool
	target    map[string]string
}

// loadSource is stage 1: cut the doc to the migrated set and tally every
// source-side number the report and the count table need. No network.
func (r *jiraRun) loadSource() {
	r.issues, r.keyset, r.droppedParents, r.droppedLinks = scopeIssues(r.doc.Issues, r.opt.Limit)
	r.tally = tallyIssues(r.issues)
	r.relations = foldRelations(r.issues, jiraLinkName, jiraSymmetricLink)

	r.names = map[string]string{}
	for _, u := range r.doc.Users {
		r.names[u.AccountID] = u.DisplayName
	}
	r.typeName = map[string]string{}
	for _, t := range r.doc.IssueTypes {
		r.typeName[t.ID] = t.Name
	}
	r.cats = map[string]bool{}
	ranks := map[int]bool{}
	r.labelSet = map[string]bool{}
	for _, is := range r.issues {
		r.cats[is.StatusCategory] = true
		ranks[is.PriorityRank] = true
		for _, l := range is.Labels {
			r.labelSet[l] = true
		}
	}
	r.rankList = make([]int, 0, len(ranks))
	for rk := range ranks {
		r.rankList = append(r.rankList, rk)
	}
	sort.Ints(r.rankList)
}

// sourceReport is stage 2: the network-free half of the report — the
// mapping lines that need no target catalog, and everything that did not
// travel.
func (r *jiraRun) sourceReport() {
	for _, c := range slices.Sorted(maps.Keys(r.cats)) {
		r.rep.Mapping = append(r.rep.Mapping, fmt.Sprintf("status_category %-10s → one transition into a target status of the same category (names are never matched)", c))
	}
	r.rep.Mapping = append(r.rep.Mapping,
		"priority_rank   → the same position in the target site's priority catalog (proportional when the two catalogs differ in length)",
		"issue type      → the target project's type of the same name; otherwise the project's default type",
		"link types      → the target site's link type of the same name; otherwise Relates",
		"comments        → body prefixed with a bold `author · time` line (Jira posts as the credential's user)",
		"description     → the origin's ADF plus a `gadak-migrate: <KEY>` footer (the idempotency key)",
		fmt.Sprintf("labels          → carried as-is (%d distinct)", len(r.labelSet)))

	r.rep.NotMigrated = append(r.rep.NotMigrated,
		fmt.Sprintf("history %d (Jira has no changelog write API — reopen counts and time-in-status start over at the target)", r.tally.history),
		fmt.Sprintf("authorship: the reporter and the authors of %d comments are text in the body; the credential's own user is the creator, and created/updated are the migration's own", r.tally.comments),
		fmt.Sprintf("assignees %d (the source's account ids do not exist on the target site; assign after the move)", r.tally.assigned))
	if r.opt.SkipAttachments && r.tally.attachments > 0 {
		r.rep.NotMigrated = append(r.rep.NotMigrated, fmt.Sprintf("attachment bytes %d (--skip-attachments: the count table shows them as not uploaded)", r.tally.attachments))
	}
	if r.st != nil {
		if r.st.Pages > 0 {
			r.rep.NotMigrated = append(r.rep.NotMigrated, fmt.Sprintf("wiki pages %d (this verb writes issues; a Confluence space is not a migrate destination)", r.st.Pages))
		}
		if r.st.DevLinks+r.st.CustomIssues+r.st.SprintIssues > 0 {
			r.rep.NotMigrated = append(r.rep.NotMigrated, fmt.Sprintf("dev links %d, issues with custom fields %d, issues with sprints %d", r.st.DevLinks, r.st.CustomIssues, r.st.SprintIssues))
		}
	}
	if r.droppedParents+r.droppedLinks > 0 {
		r.rep.NotMigrated = append(r.rep.NotMigrated, fmt.Sprintf("parents %d and links %d pointing outside the migrated set", r.droppedParents, r.droppedLinks))
	}
}

// counts appends the count table, the report's last rows in a dry run and
// in a real one (nil maps in the dry run: nothing was created or skipped).
func (r *jiraRun) counts(created, skipped map[string]int) {
	r.rep.addCountRow("issues", len(r.issues), created, skipped)
	r.rep.addCountRow("comments", r.tally.comments, created, skipped)
	r.rep.addCountRow("parents", r.tally.parents, created, skipped)
	r.rep.addCountRow("links", len(r.relations), created, skipped)
	r.rep.addCountRow("labels", len(r.labelSet), created, skipped)
	r.rep.addCountRow("attachments", r.tally.attachments, created, skipped)
}

// preflightTarget is stage 3: read the target's catalogs before the first
// write — creatable issue types, the priority catalog, link types — and
// append the mapping lines that could only be written once those were read.
func (r *jiraRun) preflightTarget() error {
	metas, err := r.client.CreateMeta(r.ctx, []string{r.opt.ProjectKey})
	if err != nil {
		return fmt.Errorf("createmeta for project %s: %w", r.opt.ProjectKey, err)
	}
	for _, m := range metas {
		if strings.EqualFold(m.Key, r.opt.ProjectKey) {
			r.types = m.IssueTypes
		}
	}
	if len(r.types) == 0 {
		return fmt.Errorf("migrate: project %s has no creatable issue type for this credential", r.opt.ProjectKey)
	}
	// The default is the first non-subtask type: a sub-task cannot be
	// created without a parent, so it can never be the fallback.
	r.defaultType = r.types[0]
	for _, t := range r.types {
		if !t.Subtask {
			r.defaultType = t
			break
		}
	}
	typeMapped := map[string]string{}
	for _, is := range r.issues {
		if _, seen := typeMapped[is.Type]; seen {
			continue
		}
		_, to := r.typeIDFor(is.Type)
		from := r.typeName[is.Type]
		if from == "" {
			from = "(none)"
		}
		typeMapped[is.Type] = to
		r.rep.Mapping = append(r.rep.Mapping, fmt.Sprintf("issue type %-14s → %q", from, to))
	}

	priorities, err := r.client.PriorityCatalog(r.ctx)
	if err != nil {
		return fmt.Errorf("priority catalog: %w", err)
	}
	r.priorities = priorities
	r.srcPriorities = len(r.doc.Priorities)
	if r.srcPriorities == 0 {
		for _, rk := range r.rankList {
			if rk > r.srcPriorities {
				r.srcPriorities = rk
			}
		}
	}
	for _, rk := range r.rankList {
		_, name := r.priorityIDFor(rk)
		if name == "" {
			r.rep.Mapping = append(r.rep.Mapping, fmt.Sprintf("priority_rank %d → (unset — the project's default)", rk))
			continue
		}
		r.rep.Mapping = append(r.rep.Mapping, fmt.Sprintf("priority_rank %d → %q", rk, name))
	}

	linkTypes, err := r.client.IssueLinkTypes(r.ctx)
	if err != nil {
		return fmt.Errorf("issue link types: %w", err)
	}
	r.linkTypeID = map[string]string{}
	for _, lt := range linkTypes {
		r.linkTypeID[jiraLinkName(lt.Name)] = lt.ID
	}
	r.relatesID = r.linkTypeID["relates"]
	return nil
}

// typeIDFor maps a source issue type onto a creatable target type: the
// project's type of the same name (createmeta's own spelling, translated or
// untranslated), otherwise the project's default type.
func (r *jiraRun) typeIDFor(sourceType string) (string, string) {
	want := r.typeName[sourceType]
	if want != "" {
		for _, t := range r.types {
			if strings.EqualFold(t.Name, want) || strings.EqualFold(t.UntranslatedName, want) {
				return t.ID, t.Name
			}
		}
	}
	return r.defaultType.ID, r.defaultType.Name
}

// priorityIDFor maps a source priority_rank onto the target site's priority
// catalog (jiraPriorityIndex); "" means leave unset and land on the default.
func (r *jiraRun) priorityIDFor(rank int) (string, string) {
	i := jiraPriorityIndex(rank, r.srcPriorities, len(r.priorities))
	if i < 0 {
		return "", ""
	}
	return r.priorities[i].ID, r.priorities[i].Name
}

// scanExisting is stage 4: the idempotency scan — every issue in the target
// project whose description carries the migrate footer, keyed by source key.
func (r *jiraRun) scanExisting() error {
	r.existing = map[string]string{} // source key → target key
	err := r.client.Search(r.ctx, jiraMigrateJQL(r.opt.ProjectKey), []string{"description"}, false, func(page []jira.Issue) error {
		for _, ji := range page {
			if k := parseMigrateFooter(adf.PlainText(ji.Fields.Description)); k != "" {
				r.existing[k] = ji.Key
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan %s for already-migrated issues: %w", r.opt.ProjectKey, err)
	}
	return nil
}

// createIssues is stage 5: create the not-yet-migrated issues parent-first
// (the parent field rides the create call), then account the labels against
// what this run's creates actually carried.
func (r *jiraRun) createIssues() error {
	r.created = map[string]int{}
	r.skipped = map[string]int{}
	r.newLabels = map[string]bool{}
	r.oldLabels = map[string]bool{}
	r.target = map[string]string{} // source key → target key
	for k, tk := range r.existing {
		if r.keyset[k] {
			r.target[k] = tk
		}
	}

	var pending []*Issue
	for i := range r.issues {
		is := &r.issues[i]
		if _, ok := r.existing[is.Key]; ok {
			// Counted as "already there": the whole issue and everything
			// under it. An earlier run's own misses stay visible in the
			// mismatch column rather than being re-attempted.
			r.skipped["issues"]++
			r.skipped["comments"] += len(is.Comments)
			r.skipped["attachments"] += len(is.Attachments)
			if is.Parent != "" {
				r.skipped["parents"]++
			}
			continue
		}
		pending = append(pending, is)
	}

	// Create parent-first so the parent field rides the create call.
	for len(pending) > 0 {
		var rest []*Issue
		progressed := false
		for _, is := range pending {
			if is.Parent != "" && r.target[is.Parent] == "" {
				rest = append(rest, is)
				continue
			}
			typeID, _ := r.typeIDFor(is.Type)
			fields := map[string]any{
				"project":     map[string]any{"key": r.opt.ProjectKey},
				"summary":     is.Summary,
				"description": jiraDescription(*is),
				"issuetype":   map[string]any{"id": typeID},
			}
			if id, _ := r.priorityIDFor(is.PriorityRank); id != "" {
				fields["priority"] = map[string]any{"id": id}
			}
			if len(is.Labels) > 0 {
				fields["labels"] = is.Labels
			}
			if is.Duedate != "" {
				fields["duedate"] = is.Duedate
			}
			if is.Parent != "" {
				fields["parent"] = map[string]any{"key": r.target[is.Parent]}
			}
			key, err := r.client.CreateIssue(r.ctx, fields)
			if err != nil {
				return fmt.Errorf("create %s: %w", is.Key, err)
			}
			r.target[is.Key] = key
			r.created["issues"]++
			if is.Parent != "" {
				r.created["parents"]++
			}
			fmt.Fprintf(r.progress, "%s → %s\n", is.Key, key)
			progressed = true
		}
		if !progressed {
			for _, is := range rest {
				r.droppedParents++
				is.Parent = ""
			}
			r.rep.Warnings = append(r.rep.Warnings, fmt.Sprintf("%d parents could not be ordered and were dropped", len(rest)))
		}
		pending = rest
	}

	// Labels are a field on the create call, so they exist the moment the
	// issue does. A label counts as created only when it rode an issue this
	// run made; one that appears solely on already-migrated issues is
	// "already there", which is what a partial re-run has to show.
	for _, is := range r.issues {
		for _, l := range is.Labels {
			if r.isNew(is.Key) {
				if !r.newLabels[l] {
					r.newLabels[l] = true
					r.created["labels"]++
				}
			} else if !r.oldLabels[l] {
				r.oldLabels[l] = true
			}
		}
	}
	for l := range r.oldLabels {
		if !r.newLabels[l] {
			r.skipped["labels"]++
		}
	}
	return nil
}

// isNew says an issue was created by this run: not there at the scan, and
// holding a target key now.
func (r *jiraRun) isNew(k string) bool {
	_, was := r.existing[k]
	return !was && r.target[k] != ""
}

// completeIssues is stage 6: the per-issue writes that need the issue to
// exist — the status transition, the comments, the attachments.
func (r *jiraRun) completeIssues() error {
	for i := range r.issues {
		is := &r.issues[i]
		if !r.isNew(is.Key) {
			continue
		}
		key := r.target[is.Key]

		// Status: one transition into a status of the same category. The
		// name is never compared — it is localized per account, and this is
		// exactly the trap data-model.md names.
		if cat := statuscat.Category(is.StatusCategory); cat != "new" {
			trs, err := r.client.Transitions(r.ctx, key)
			if err != nil {
				return fmt.Errorf("transitions for %s: %w", key, err)
			}
			moved := false
			for _, tr := range trs {
				if statuscat.Category(tr.To.StatusCategory.Key) != cat {
					continue
				}
				if err := r.client.Transition(r.ctx, key, tr.ID, nil, nil); err != nil {
					return fmt.Errorf("transition %s to %s: %w", key, cat, err)
				}
				moved = true
				break
			}
			if !moved {
				r.rep.Warnings = append(r.rep.Warnings,
					fmt.Sprintf("%s (%s) has no transition into status_category %q from the project's initial status — it stays where it landed", key, is.Key, cat))
			}
		}

		for _, c := range is.Comments {
			who := r.names[c.Author]
			if who == "" {
				who = c.Author
			}
			if _, err := r.client.AddComment(r.ctx, key, jiraCommentBody(strings.TrimSpace(who+" · "+c.Created), c), nil, false); err != nil {
				return fmt.Errorf("comment on %s: %w", is.Key, err)
			}
			r.created["comments"]++
		}

		if r.opt.SkipAttachments {
			continue
		}
		for _, a := range is.Attachments {
			src, warn, err := jiraAttachmentSource(r.ctx, r.opt.Fetch, a)
			if err != nil {
				return fmt.Errorf("attachment %q on %s: %w", a.Filename, is.Key, err)
			}
			if warn != "" {
				r.rep.Warnings = append(r.rep.Warnings, fmt.Sprintf("attachment %q on %s: %s", a.Filename, is.Key, warn))
			}
			if src == nil {
				continue
			}
			_, err = r.client.Upload(r.ctx, key, a.Filename, src)
			src.Close()
			if err != nil {
				return fmt.Errorf("attachment %q on %s: %w", a.Filename, is.Key, err)
			}
			r.created["attachments"]++
		}
	}
	return nil
}

// createLinks is stage 7: the relations, typed against the target site's
// link catalog with Relates as the fallback.
func (r *jiraRun) createLinks() error {
	// A pair whose both ends pre-existed was linked by the run that created
	// them; only pairs with a new end are new.
	unlinkable := map[string]bool{}
	fellBack := map[string]bool{}
	for _, rel := range r.relations {
		if !r.isNew(rel.from) && !r.isNew(rel.to) {
			r.skipped["links"]++
			continue
		}
		id := r.linkTypeID[rel.typ]
		note := ""
		if id == "" {
			id, note = r.relatesID, rel.typ
		}
		if id == "" {
			unlinkable[rel.typ] = true
			continue
		}
		if err := r.client.LinkIssues(r.ctx, id, r.target[rel.from], r.target[rel.to]); err != nil {
			return fmt.Errorf("link %s %s %s: %w", rel.from, rel.typ, rel.to, err)
		}
		if note != "" && !fellBack[note] {
			fellBack[note] = true
			r.rep.Warnings = append(r.rep.Warnings, fmt.Sprintf("link type %q has no counterpart on the target site — linked as Relates", note))
		}
		r.created["links"]++
	}
	if len(unlinkable) > 0 {
		r.rep.NotMigrated = append(r.rep.NotMigrated,
			fmt.Sprintf("links of %d type(s) absent from the target site, which also has no Relates type: %s",
				len(unlinkable), strings.Join(slices.Sorted(maps.Keys(unlinkable)), ", ")))
	}
	return nil
}

// jiraAttachmentSource opens the bytes one attachment uploads: the source
// origin when fetch is set (streamed, not held), else what the export
// carries. nil src with an empty warning is a plain skip (metadata-only
// row); a warning names why bytes that should exist could not be read.
func jiraAttachmentSource(ctx context.Context, fetch StreamFetch, a Attachment) (src io.ReadCloser, warn string, err error) {
	switch {
	case fetch != nil && a.SourceURL == "" && a.ContentID != "":
		status, _, body, ferr := fetch(ctx, a.ContentID)
		switch {
		case ferr != nil:
			return nil, "", ferr
		case status == 404:
			return nil, "missing at the source (404)", nil
		case status != 200 || body == nil:
			return nil, fmt.Sprintf("source answered status %d", status), nil
		}
		return body, "", nil
	case a.DataBase64 != "":
		raw, derr := base64.StdEncoding.DecodeString(a.DataBase64)
		if derr != nil {
			return nil, "unreadable bytes in the export", nil
		}
		return io.NopCloser(bytes.NewReader(raw)), "", nil
	case a.Text != "":
		return io.NopCloser(strings.NewReader(a.Text)), "", nil
	}
	return nil, "", nil
}
