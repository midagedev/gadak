package migrate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
// counts, the mapping applied, and what did not travel.
type JiraReport struct {
	Project     string      `json:"project"`
	DryRun      bool        `json:"dry_run"`
	Counts      []VerifyRow `json:"counts"`
	Mapping     []string    `json:"mapping"`
	NotMigrated []string    `json:"not_migrated"`
	Warnings    []string    `json:"warnings,omitempty"`
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
func ToJira(ctx context.Context, client *jira.Client, doc *Doc, st *Stats, opt JiraOptions) (*JiraReport, error) {
	if opt.ProjectKey == "" {
		return nil, fmt.Errorf("migrate: --project <KEY> is required for --to jira")
	}
	rep := &JiraReport{Project: opt.ProjectKey, DryRun: opt.DryRun}
	progress := opt.Progress
	if progress == nil {
		progress = io.Discard
	}

	issues, keyset, droppedParents, droppedLinks := scopeIssues(doc.Issues, opt.Limit)
	tally := tallyIssues(issues)
	relations := foldRelations(issues, jiraLinkName, jiraSymmetricLink)

	names := map[string]string{}
	for _, u := range doc.Users {
		names[u.AccountID] = u.DisplayName
	}
	typeName := map[string]string{}
	for _, t := range doc.IssueTypes {
		typeName[t.ID] = t.Name
	}
	cats := map[string]bool{}
	ranks := map[int]bool{}
	labelSet := map[string]bool{}
	linkNames := map[string]bool{}
	for _, is := range issues {
		cats[is.StatusCategory] = true
		ranks[is.PriorityRank] = true
		for _, l := range is.Labels {
			labelSet[l] = true
		}
		for _, l := range is.Links {
			linkNames[jiraLinkName(l.Type)] = true
		}
	}

	rankList := make([]int, 0, len(ranks))
	for r := range ranks {
		rankList = append(rankList, r)
	}
	sort.Ints(rankList)

	for _, c := range sortedKeys(cats) {
		rep.Mapping = append(rep.Mapping, fmt.Sprintf("status_category %-10s → one transition into a target status of the same category (names are never matched)", c))
	}
	rep.Mapping = append(rep.Mapping,
		"priority_rank   → the same position in the target site's priority catalog (proportional when the two catalogs differ in length)",
		"issue type      → the target project's type of the same name; otherwise the project's default type",
		"link types      → the target site's link type of the same name; otherwise Relates",
		"comments        → body prefixed with a bold `author · time` line (Jira posts as the credential's user)",
		"description     → the origin's ADF plus a `gadak-migrate: <KEY>` footer (the idempotency key)",
		fmt.Sprintf("labels          → carried as-is (%d distinct)", len(labelSet)))

	rep.NotMigrated = append(rep.NotMigrated,
		fmt.Sprintf("history %d (Jira has no changelog write API — reopen counts and time-in-status start over at the target)", tally.history),
		fmt.Sprintf("authorship: the reporter and the authors of %d comments are text in the body; the credential's own user is the creator, and created/updated are the migration's own", tally.comments),
		fmt.Sprintf("assignees %d (the source's account ids do not exist on the target site; assign after the move)", tally.assigned))
	if opt.SkipAttachments && tally.attachments > 0 {
		rep.NotMigrated = append(rep.NotMigrated, fmt.Sprintf("attachment bytes %d (--skip-attachments: the count table shows them as not uploaded)", tally.attachments))
	}
	if st != nil {
		if st.Pages > 0 {
			rep.NotMigrated = append(rep.NotMigrated, fmt.Sprintf("wiki pages %d (this verb writes issues; a Confluence space is not a migrate destination)", st.Pages))
		}
		if st.DevLinks+st.CustomIssues+st.SprintIssues > 0 {
			rep.NotMigrated = append(rep.NotMigrated, fmt.Sprintf("dev links %d, issues with custom fields %d, issues with sprints %d", st.DevLinks, st.CustomIssues, st.SprintIssues))
		}
	}
	if droppedParents+droppedLinks > 0 {
		rep.NotMigrated = append(rep.NotMigrated, fmt.Sprintf("parents %d and links %d pointing outside the migrated set", droppedParents, droppedLinks))
	}

	attachSource := tally.attachments
	counts := func(created, skipped map[string]int) {
		row := func(metric string, source int) {
			rep.Counts = append(rep.Counts, VerifyRow{Metric: metric, Source: source,
				Migrated: created[metric] + skipped[metric], Skipped: skipped[metric]})
		}
		row("issues", len(issues))
		row("comments", tally.comments)
		row("parents", tally.parents)
		row("links", len(relations))
		row("labels", len(labelSet))
		row("attachments", attachSource)
	}
	if opt.DryRun {
		counts(nil, nil)
		return rep, nil
	}

	// --- network from here on ---
	metas, err := client.CreateMeta(ctx, []string{opt.ProjectKey})
	if err != nil {
		return nil, fmt.Errorf("createmeta for project %s: %w", opt.ProjectKey, err)
	}
	var types []jira.CreateMetaIssueType
	for _, m := range metas {
		if strings.EqualFold(m.Key, opt.ProjectKey) {
			types = m.IssueTypes
		}
	}
	if len(types) == 0 {
		return nil, fmt.Errorf("migrate: project %s has no creatable issue type for this credential", opt.ProjectKey)
	}
	// The default is the first non-subtask type: a sub-task cannot be
	// created without a parent, so it can never be the fallback.
	defaultType := types[0]
	for _, t := range types {
		if !t.Subtask {
			defaultType = t
			break
		}
	}
	typeIDFor := func(sourceType string) (string, string) {
		want := typeName[sourceType]
		if want != "" {
			for _, t := range types {
				if strings.EqualFold(t.Name, want) || strings.EqualFold(t.UntranslatedName, want) {
					return t.ID, t.Name
				}
			}
		}
		return defaultType.ID, defaultType.Name
	}
	typeMapped := map[string]string{}
	for _, is := range issues {
		if _, seen := typeMapped[is.Type]; seen {
			continue
		}
		_, to := typeIDFor(is.Type)
		from := typeName[is.Type]
		if from == "" {
			from = "(none)"
		}
		typeMapped[is.Type] = to
		rep.Mapping = append(rep.Mapping, fmt.Sprintf("issue type %-14s → %q", from, to))
	}

	priorities, err := client.PriorityCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("priority catalog: %w", err)
	}
	srcPriorities := len(doc.Priorities)
	if srcPriorities == 0 {
		for _, r := range rankList {
			if r > srcPriorities {
				srcPriorities = r
			}
		}
	}
	priorityIDFor := func(rank int) (string, string) {
		i := jiraPriorityIndex(rank, srcPriorities, len(priorities))
		if i < 0 {
			return "", ""
		}
		return priorities[i].ID, priorities[i].Name
	}
	for _, r := range rankList {
		_, name := priorityIDFor(r)
		if name == "" {
			rep.Mapping = append(rep.Mapping, fmt.Sprintf("priority_rank %d → (unset — the project's default)", r))
			continue
		}
		rep.Mapping = append(rep.Mapping, fmt.Sprintf("priority_rank %d → %q", r, name))
	}

	linkTypes, err := client.IssueLinkTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("issue link types: %w", err)
	}
	linkTypeID := map[string]string{}
	for _, lt := range linkTypes {
		linkTypeID[jiraLinkName(lt.Name)] = lt.ID
	}
	relatesID := linkTypeID["relates"]

	// Existing rows by footer — the idempotency scan.
	existing := map[string]string{} // source key → target key
	err = client.Search(ctx, jiraMigrateJQL(opt.ProjectKey), []string{"description"}, false, func(page []jira.Issue) error {
		for _, ji := range page {
			if k := parseMigrateFooter(adf.PlainText(ji.Fields.Description)); k != "" {
				existing[k] = ji.Key
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s for already-migrated issues: %w", opt.ProjectKey, err)
	}

	created := map[string]int{}
	skipped := map[string]int{}
	newLabels := map[string]bool{}
	oldLabels := map[string]bool{}
	target := map[string]string{} // source key → target key
	for k, tk := range existing {
		if keyset[k] {
			target[k] = tk
		}
	}

	var pending []*Issue
	for i := range issues {
		is := &issues[i]
		if _, ok := existing[is.Key]; ok {
			// Counted as "already there": the whole issue and everything
			// under it. An earlier run's own misses stay visible in the
			// mismatch column rather than being re-attempted.
			skipped["issues"]++
			skipped["comments"] += len(is.Comments)
			skipped["attachments"] += len(is.Attachments)
			if is.Parent != "" {
				skipped["parents"]++
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
			if is.Parent != "" && target[is.Parent] == "" {
				rest = append(rest, is)
				continue
			}
			typeID, _ := typeIDFor(is.Type)
			fields := map[string]any{
				"project":     map[string]any{"key": opt.ProjectKey},
				"summary":     is.Summary,
				"description": jiraDescription(*is),
				"issuetype":   map[string]any{"id": typeID},
			}
			if id, _ := priorityIDFor(is.PriorityRank); id != "" {
				fields["priority"] = map[string]any{"id": id}
			}
			if len(is.Labels) > 0 {
				fields["labels"] = is.Labels
			}
			if is.Duedate != "" {
				fields["duedate"] = is.Duedate
			}
			if is.Parent != "" {
				fields["parent"] = map[string]any{"key": target[is.Parent]}
			}
			key, err := client.CreateIssue(ctx, fields)
			if err != nil {
				return nil, fmt.Errorf("create %s: %w", is.Key, err)
			}
			target[is.Key] = key
			created["issues"]++
			if is.Parent != "" {
				created["parents"]++
			}
			fmt.Fprintf(progress, "%s → %s\n", is.Key, key)
			progressed = true
		}
		if !progressed {
			for _, is := range rest {
				droppedParents++
				is.Parent = ""
			}
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d parents could not be ordered and were dropped", len(rest)))
		}
		pending = rest
	}

	isNew := func(k string) bool { _, was := existing[k]; return !was && target[k] != "" }

	// Labels are a field on the create call, so they exist the moment the
	// issue does. A label counts as created only when it rode an issue this
	// run made; one that appears solely on already-migrated issues is
	// "already there", which is what a partial re-run has to show.
	for _, is := range issues {
		for _, l := range is.Labels {
			if isNew(is.Key) {
				if !newLabels[l] {
					newLabels[l] = true
					created["labels"]++
				}
			} else if !oldLabels[l] {
				oldLabels[l] = true
			}
		}
	}
	for l := range oldLabels {
		if !newLabels[l] {
			skipped["labels"]++
		}
	}

	for i := range issues {
		is := &issues[i]
		if !isNew(is.Key) {
			continue
		}
		key := target[is.Key]

		// Status: one transition into a status of the same category. The
		// name is never compared — it is localized per account, and this is
		// exactly the trap data-model.md names.
		if cat := statuscat.Category(is.StatusCategory); cat != "new" {
			trs, err := client.Transitions(ctx, key)
			if err != nil {
				return nil, fmt.Errorf("transitions for %s: %w", key, err)
			}
			moved := false
			for _, tr := range trs {
				if statuscat.Category(tr.To.StatusCategory.Key) != cat {
					continue
				}
				if err := client.Transition(ctx, key, tr.ID, nil, nil); err != nil {
					return nil, fmt.Errorf("transition %s to %s: %w", key, cat, err)
				}
				moved = true
				break
			}
			if !moved {
				rep.Warnings = append(rep.Warnings,
					fmt.Sprintf("%s (%s) has no transition into status_category %q from the project's initial status — it stays where it landed", key, is.Key, cat))
			}
		}

		for _, c := range is.Comments {
			who := names[c.Author]
			if who == "" {
				who = c.Author
			}
			if _, err := client.AddComment(ctx, key, jiraCommentBody(strings.TrimSpace(who+" · "+c.Created), c), nil, false); err != nil {
				return nil, fmt.Errorf("comment on %s: %w", is.Key, err)
			}
			created["comments"]++
		}

		if opt.SkipAttachments {
			continue
		}
		for _, a := range is.Attachments {
			src, warn, err := jiraAttachmentSource(ctx, opt.Fetch, a)
			if err != nil {
				return nil, fmt.Errorf("attachment %q on %s: %w", a.Filename, is.Key, err)
			}
			if warn != "" {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("attachment %q on %s: %s", a.Filename, is.Key, warn))
			}
			if src == nil {
				continue
			}
			_, err = client.Upload(ctx, key, a.Filename, src)
			src.Close()
			if err != nil {
				return nil, fmt.Errorf("attachment %q on %s: %w", a.Filename, is.Key, err)
			}
			created["attachments"]++
		}
	}

	// A pair whose both ends pre-existed was linked by the run that created
	// them; only pairs with a new end are new.
	unlinkable := map[string]bool{}
	fellBack := map[string]bool{}
	for _, r := range relations {
		if !isNew(r.from) && !isNew(r.to) {
			skipped["links"]++
			continue
		}
		id := linkTypeID[r.typ]
		note := ""
		if id == "" {
			id, note = relatesID, r.typ
		}
		if id == "" {
			unlinkable[r.typ] = true
			continue
		}
		if err := client.LinkIssues(ctx, id, target[r.from], target[r.to]); err != nil {
			return nil, fmt.Errorf("link %s %s %s: %w", r.from, r.typ, r.to, err)
		}
		if note != "" && !fellBack[note] {
			fellBack[note] = true
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("link type %q has no counterpart on the target site — linked as Relates", note))
		}
		created["links"]++
	}
	if len(unlinkable) > 0 {
		rep.NotMigrated = append(rep.NotMigrated,
			fmt.Sprintf("links of %d type(s) absent from the target site, which also has no Relates type: %s",
				len(unlinkable), strings.Join(sortedKeys(unlinkable), ", ")))
	}

	counts(created, skipped)
	return rep, nil
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
