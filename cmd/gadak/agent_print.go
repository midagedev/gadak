package main

// The print side of an agent issue answer: the human rendering of one
// issue (fields, links, comments, dev links) and the --derive explanation
// of what the status-derived fields imply. Split from agent.go
// (GDK-1771); the verbs that produce the data live in agent_issue.go and
// agent_write.go.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/jirafields"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
)

func printIssueDocs(docs []issueDoc, phrases map[string][2]string) {
	for i, doc := range docs {
		if i > 0 {
			fmt.Printf("--- %s ---\n", doc.Issue.IssueKey)
		}
		printIssue(doc.Issue, doc.Detail, doc.durations, phrases)
	}
}

type issueEditMetaOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type issueEditMetaField struct {
	Alias    string                `json:"alias"`
	ID       string                `json:"id"`
	Kind     string                `json:"kind"`
	Required bool                  `json:"required"`
	Options  []issueEditMetaOption `json:"options"`
}

// printIssueEditMeta is `gadak issue KEY --editmeta`: one origin GET
// editmeta, then the same allowlist ∩ ResolveEditable filter
// handleEditMeta uses (internal/server/write.go). The answer is not
// written to the mirror — origin is the source of truth for this verb.
func printIssueEditMeta(key string, asJSON bool) error {
	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, _ *store.DB, c origin.Writer, _ string) error {
		meta, err := c.EditMeta(ctx, key)
		if err != nil {
			return err
		}
		rows := editableFieldsForIssue(cfg, meta)
		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(struct {
				Key    string               `json:"key"`
				Fields []issueEditMetaField `json:"fields"`
			}{Key: key, Fields: jsonList(rows)})
		}
		printIssueEditMetaHuman(rows)
		return nil
	})
}

// editableFieldsForIssue is the web editmeta intersection: allowlist from
// fields.EditableAliases, presence+kind from jirafields.ResolveEditable,
// options from FieldMeta.AllowedValues (value, else name — same as
// handleEditMeta).
func editableFieldsForIssue(cfg *config.Config, meta map[string]jira.FieldMeta) []issueEditMetaField {
	allow := fields.EditableAliases(cfg)
	aliases := slices.Sorted(maps.Keys(allow))
	out := make([]issueEditMetaField, 0)
	for _, alias := range aliases {
		ea := allow[alias]
		id, kind, present := jirafields.ResolveEditable(ea.IDs, meta, ea.Kind)
		if !present {
			continue
		}
		m := meta[id]
		opts := make([]issueEditMetaOption, 0, len(m.AllowedValues))
		for _, v := range m.AllowedValues {
			name := v.Value
			if name == "" {
				name = v.Name
			}
			opts = append(opts, issueEditMetaOption{ID: v.ID, Name: name})
		}
		out = append(out, issueEditMetaField{
			Alias:    alias,
			ID:       id,
			Kind:     kind,
			Required: m.Required,
			Options:  jsonList(opts),
		})
	}
	return out
}

func printIssueEditMetaHuman(rows []issueEditMetaField) {
	if len(rows) == 0 {
		// An empty intersection means no configured custom aliases, not a
		// read-only issue — silence here reads as "nothing editable".
		fmt.Println("no configured custom fields are editable on this issue — `gadak edit` system flags (--summary, --label, --component, --fix-version, --priority, --parent, --due, -m) apply regardless; custom aliases appear here after `gadak fields --apply`")
		return
	}
	for _, f := range rows {
		inner := f.Kind
		if f.Required {
			inner = f.Kind + ", required"
		}
		line := fmt.Sprintf("%s (%s)", f.Alias, inner)
		names := make([]string, 0, len(f.Options))
		for _, o := range f.Options {
			if o.Name != "" {
				names = append(names, o.Name)
			}
		}
		if len(names) > 0 {
			line += " — options: " + strings.Join(names, ", ")
		}
		fmt.Println(line)
	}
}

// printIssueLink is `gadak issue KEY --link`: the same gadak:// composer
// views open uses (deepLinkURL → deeplink.Compose), plus the http form when
// a serve is discoverable the same way. --json names the fields deeplink and
// web so the two commands agree. The key must already be in the mirror — a
// typo should not produce a dead link that looks real.
func printIssueLink(db *store.DB, key string, asJSON bool) error {
	lites, err := lookup(db, []string{key})
	if err != nil {
		return err
	}
	if len(lites) == 0 {
		// GDK-1612: same scope verdict as cmdIssue's not-found path — a dead
		// link for a key outside this workspace's scope should say so.
		if hint := zeroRowScopeHint(db, "", key); hint != "" {
			return errors.New(hint)
		}
		return fmt.Errorf("%s is not in the mirror — check the key, or run `gadak sync`", key)
	}
	hash := "issue=" + key
	link := deepLinkURL(config.Profile(), hash)
	web := serveFocusURL(hash)
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"deeplink": link,
			"web":      web,
		})
	}
	if link != "" {
		fmt.Printf("deeplink\t%s\n", link)
	}
	if web != "" {
		fmt.Printf("web\t%s\n", web)
	}
	return nil
}

func printIssue(l store.IssueLite, d *store.Detail, dur store.Spans, phrases map[string][2]string) {
	fmt.Printf("%s\t%s\n", l.IssueKey, l.Summary)
	kv := func(label, value string) {
		if value != "" {
			fmt.Printf("%-13s %s\n", label, value)
		}
	}
	kv("project", l.ProjectKey)
	kv("type", l.IssueType)
	kv("status", fmt.Sprintf("%s (%s)", l.Status, l.StatusCategory))
	kv("priority", derefOrEmpty(l.Priority, ""))
	kv("assignee", derefOrEmpty(l.Assignee, "(unassigned)"))
	kv("reporter", derefOrEmpty(l.Reporter, ""))
	kv("parent", derefOrEmpty(l.ParentKey, ""))
	kv("epic", derefOrEmpty(l.EpicKey, ""))
	kv("labels", strings.Join(l.Labels, ", "))
	kv("components", strings.Join(l.Components, ", "))
	kv("fix versions", strings.Join(l.FixVersions, ", "))
	kv("duedate", derefOrEmpty(l.Duedate, ""))
	kv("resolution", derefOrEmpty(l.Resolution, ""))
	// Restricted issues only: kv skips empty, so unrestricted rows stay
	// indistinguishable in the text form. Prefer the display name; fall
	// back to the id when the origin sent id without name.
	if name := derefOrEmpty(l.SecurityLevel, ""); name != "" {
		kv("security", name)
	} else {
		kv("security", derefOrEmpty(l.SecurityLevelID, ""))
	}
	kv("created", derefOrEmpty(l.CreatedAt, ""))
	kv("updated", derefOrEmpty(l.UpdatedAt, ""))
	kv("status since", derefOrEmpty(l.StatusChangedAt, ""))
	kv("resolved", derefOrEmpty(l.ResolvedAt, ""))
	// Computed from the changelog, never stored (data-model.md keeps
	// time-in-status absent); kv skips the whole line when neither span
	// exists — an issue that never entered progress has nothing to say.
	kv("durations", dur.Line())
	if l.ReopenCount > 0 {
		kv("reopens", fmt.Sprintf("%d (last %s)", l.ReopenCount, derefOrEmpty(l.ReopenedAt, "?")))
	}
	// Sorted: map order would make two runs on the same issue differ.
	aliases := slices.Sorted(maps.Keys(l.Custom))
	for _, alias := range aliases {
		kv(alias, fmt.Sprint(l.Custom[alias]))
	}

	// The markdown source, not PlainText (GDK-1394): what this prints is
	// what `edit -m -` takes back, so a heading read here must still be a
	// heading after the round trip. Same owner as the web editor.
	desc := strings.TrimSpace(d.DescriptionMD)
	if desc != "" {
		fmt.Printf("\ndescription\n%s\n", indent(desc))
	} else {
		// GDK-1020: an omitted section read as a truncated answer — a caller
		// with no session ran the right command and judged it incomplete.
		// Absence is an answer, so it gets its line in the section's rhythm.
		fmt.Printf("\ndescription (none)\n")
	}
	if len(d.Comments) > 0 {
		fmt.Printf("\ncomments (%d)\n", len(d.Comments))
		for _, c := range d.Comments {
			body := strings.TrimSpace(c.BodyMD)
			if mark := commentMark(c); mark != "" {
				if body != "" {
					body = mark + " " + body
				} else {
					body = mark
				}
			}
			fmt.Printf("  %s  %s\n%s\n", c.CreatedAt, c.Author, indent(body))
		}
	} else {
		// Same contract as description (none): the count-0 header is the
		// present form's own shape with nothing under it.
		fmt.Printf("\ncomments (0)\n")
	}
	if len(d.Attachments) > 0 {
		fmt.Printf("\nattachments (%d)\n", len(d.Attachments))
		for _, a := range d.Attachments {
			fmt.Printf("  %s\t%s\t%d bytes%s\n", a.Filename, a.MimeType, a.Size, attachmentTruncationMark(a.Size))
		}
	}
	if len(d.LinkedIssues) > 0 {
		fmt.Printf("\nlinks (%d)\n", len(d.LinkedIssues))
		for _, k := range d.LinkedIssues {
			// The type's own sentence ("blocks"), not the wire pair ("Blocks
			// outward") — that is what the line says on the origin's own page
			// (GDK-1734). A catalog without the type keeps the pair, which is
			// what this line printed before the catalog existed.
			label := k.Type + " " + k.Direction
			if lt, ok := phrases[strings.ToLower(k.Type)]; ok {
				if p := origin.LinkPhrase(k.Direction, lt[0], lt[1]); p != "" {
					label = p
				}
			}
			fmt.Printf("  %s\t%s\t%s\n", label, k.Key, k.Summary)
		}
	}
	if prs := server.ListLinkedPRs(d.DevLinks, d.Attachments, d.Refs); len(prs) > 0 {
		fmt.Printf("\nLinked PRs (%d)\n", len(prs))
		for _, p := range prs {
			line := p.URL
			if p.Title != "" {
				line = p.Title + "\t" + p.URL
			}
			if p.State != "" {
				line += "\t" + p.State
			}
			fmt.Printf("  %s\n", line)
		}
	}
	printDevLinkKinds(d.DevLinks)
	if len(d.History) > 0 {
		fmt.Printf("\nhistory (%d)\n", len(d.History))
		for _, h := range d.History {
			fmt.Printf("  %s  %s\t%s: %s → %s\n", h.At, h.Author, h.Field, h.FromValue, h.ToValue)
		}
	}
}

func linkedPRsJSON(d *store.Detail) json.RawMessage {
	if d == nil {
		return json.RawMessage("[]")
	}
	raw := server.MergedPRLinks(d.DevLinks, d.Attachments, d.Refs)
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	return raw
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n    ")
}

/* ── issue --derive ── */

// Several issue columns are not stored by the source — they are computed from
// the changelog. `--derive` is the window into that: it rebuilds the inputs the
// connector had at sync time out of the mirror, hands them to the same
// store.Derive the sync path calls (internal/store/write.go, "d := Derive(...)"),
// and prints the rows each rule fired on. Nothing here re-implements a rule: a
// second copy would agree with the first only until one of them changed, and
// that disagreement is exactly what this command exists to surface.
//
// epic_key is the one derived column store.Derive does not own — its rule is the
// UPDATE in store.recomputeEpicKeys, which runs in SQL after every upsert batch
// and is not callable from here. So epic_key is explained by showing the parent
// chain and pointing at the hop the stored value names, never by walking the
// chain a second time and declaring a winner.

// deriveNull is what a NULL derived timestamp prints as. "" would look like a
// missing line rather than a computed absence, and absence is the answer the
// caller most often came for ("why is resolved_at empty?").
const deriveNull = "(null)"

// unmappedCategory labels a status id the mirror has no category for. Derive
// treats it as not-done, which can only ever miss a reopen, never invent one
// (docs/DERIVE.md, "Derived field rules").
const unmappedCategory = "(unmapped)"

// deriveContext is the sync-time DeriveInput rebuilt from the mirror. The
// category map and the priority list came from the site's own status and
// priority endpoints during sync and are not mirrored as tables, so both are
// reconstructed from the issue rows that carry them. Reconstruction is exact for
// any id or priority some mirrored issue still uses, and `missing` names the
// changelog ids it could not cover — the one case where the numbers below can
// legitimately differ from the stored columns.
type deriveContext struct {
	categories map[string]string // status id -> new | inprogress | done
	priorities []string          // site priority names, most urgent first
	missing    []string          // status ids in this changelog with no category
	chain      []deriveHop       // the issue, its parent, its grandparent
}

// deriveHop is one link of the parent chain epic_key is resolved along.
type deriveHop struct {
	key       string
	level     int
	parentKey string
}

// priorityGap fills a rank the mirror never observed. priorityRank matches by
// name and position, so the list has to keep unobserved ranks occupied or every
// priority below the gap would shift up one.
const priorityGap = "\x00gap"

// loadDeriveContext reads the two lookup tables and the parent chain. It uses
// the read-only connection for the same reason warnIfStale does: these are
// aggregate queries the store exposes no typed accessor for, and a read-only
// handle cannot disturb the mirror while the server holds the single writer.
func loadDeriveContext(l store.IssueLite, history []store.DetailChange) (*deriveContext, error) {
	db, err := openReadOnly()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	c := &deriveContext{categories: map[string]string{}}
	rows, err := db.Query(`SELECT DISTINCT status_id, status_category FROM issues
		WHERE status_id IS NOT NULL AND status_id <> '' AND status_category IS NOT NULL AND status_category <> ''`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, cat string
		if err := rows.Scan(&id, &cat); err != nil {
			rows.Close()
			return nil, err
		}
		c.categories[id] = cat
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, h := range history {
		if h.Field != "status" {
			continue
		}
		for _, id := range []string{h.FromID, h.ToID} {
			if id == "" || seen[id] || c.categories[id] != "" {
				continue
			}
			seen[id] = true
			c.missing = append(c.missing, id)
		}
	}

	byRank := map[int]string{}
	maxRank := 0
	prows, err := db.Query(`SELECT DISTINCT priority, priority_rank FROM issues
		WHERE priority IS NOT NULL AND priority <> '' AND priority_rank > 0`)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var name string
		var rank int
		if err := prows.Scan(&name, &rank); err != nil {
			prows.Close()
			return nil, err
		}
		byRank[rank] = name
		if rank > maxRank {
			maxRank = rank
		}
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		return nil, err
	}
	c.priorities = make([]string, maxRank)
	for i := range c.priorities {
		if name, ok := byRank[i+1]; ok {
			c.priorities[i] = name
			continue
		}
		c.priorities[i] = priorityGap
	}

	// Two hops is all epic_key ever looks at (parent, else grandparent).
	key := l.IssueKey
	for hop := 0; hop < 3 && key != ""; hop++ {
		var level int
		var parent string
		err := db.QueryRow(`SELECT COALESCE(hierarchy_level, 0), COALESCE(parent_key, '')
			FROM issues WHERE key = ?`, key).Scan(&level, &parent)
		if errors.Is(err, sql.ErrNoRows) {
			c.chain = append(c.chain, deriveHop{key: key, level: 0, parentKey: ""})
			break
		}
		if err != nil {
			return nil, err
		}
		c.chain = append(c.chain, deriveHop{key: key, level: level, parentKey: parent})
		key = parent
	}
	return c, nil
}

// deriveInput converts the mirrored detail back into the shape the connector
// handed store.Derive. Detail orders the changelog by (at, id) and Derive sorts
// stably by `at` alone, so the row numbers printed below are the order Derive
// itself walks.
func deriveInput(l store.IssueLite, d *store.Detail, c *deriveContext) store.DeriveInput {
	in := store.DeriveInput{
		Categories:      c.categories,
		CurrentCategory: l.StatusCategory,
		Priority:        derefOrEmpty(l.Priority, ""),
		Priorities:      c.priorities,
	}
	for _, h := range d.History {
		in.Changelog = append(in.Changelog, store.ChangeEntry{
			At: h.At, Author: h.Author, AuthorID: h.AuthorID, Field: h.Field,
			FromValue: h.FromValue, FromID: h.FromID, ToValue: h.ToValue, ToID: h.ToID,
		})
	}
	for _, cm := range d.Comments {
		body := cm.Body
		if strings.TrimSpace(body) == "" {
			body = adf.PlainText(cm.BodyADF)
		}
		in.Comments = append(in.Comments, store.Comment{
			ID: cm.ID, ExternalID: cm.ExternalID, Author: cm.Author,
			BodyText: body, CreatedAt: cm.CreatedAt,
		})
	}
	for _, k := range d.LinkedIssues {
		in.Links = append(in.Links, store.Link{Type: k.Type, Direction: k.Direction, TargetKey: k.Key})
	}
	return in
}

// categoryOf is the label for one side of a status transition. The rules key on
// this string and never on the display name beside it: `status = 'In Progress'`
// is silently zero rows on a Korean account (CLAUDE.md; contracts/sync.md,
// "Localization hazard").
func categoryOf(cats map[string]string, id string) string {
	if id == "" {
		return "(none)"
	}
	if c := cats[id]; c != "" {
		return c
	}
	return unmappedCategory
}

func printDerivation(l store.IssueLite, d *store.Detail) error {
	c, err := loadDeriveContext(l, d.History)
	if err != nil {
		return err
	}
	in := deriveInput(l, d, c)
	got := store.Derive(in)

	fmt.Printf("%s\t%s\n", l.IssueKey, l.Summary)
	fmt.Println("derived fields, recomputed by internal/store.Derive — the one function the sync")
	fmt.Println("path calls. Field names below are issues columns; indented lines are the evidence.")

	statusRows, assigneeRows := 0, 0
	for _, h := range d.History {
		switch h.Field {
		case "status":
			statusRows++
		case "assignee":
			assigneeRows++
		}
	}
	fmt.Println("\ninputs")
	fmt.Printf("  category now    %s (%s)\n", l.StatusCategory, l.Status)
	fmt.Printf("  changelog       %d rows (%d status, %d assignee)\n", len(d.History), statusRows, assigneeRows)
	fmt.Printf("  comments        %d\n", len(d.Comments))
	fmt.Printf("  links           %d\n", len(d.LinkedIssues))
	fmt.Printf("  category map    %d status %s, rebuilt from the mirror's own issue rows\n",
		len(c.categories), plural(len(c.categories), "id", "ids"))
	if len(c.missing) > 0 {
		// The only honest reason a number below may differ from the stored column.
		fmt.Printf("  NOT covered     %s — no mirrored issue still uses %s, so %s counted as not-done here\n",
			strings.Join(c.missing, ", "), plural(len(c.missing), "this id", "these ids"),
			plural(len(c.missing), "it is", "they are"))
	}

	// Fixed-width ASCII columns first, the site's own (possibly CJK) names last:
	// a display name's terminal width is not its rune count, so anything padded
	// after one would drift out of line.
	fmt.Println("\nchangelog, oldest first")
	fmt.Printf("  %-4s %-24s %-18s %-22s %s\n", "#", "at", "field", "category", "value")
	reopenRows := []string{}
	resolvedRow := ""
	statusRow, assigneeRow := "", ""
	for i, h := range d.History {
		n := fmt.Sprintf("#%d", i+1)
		cats := ""
		note := ""
		if h.Field == "status" {
			from, to := categoryOf(c.categories, h.FromID), categoryOf(c.categories, h.ToID)
			cats = from + " → " + to
			if h.At != "" {
				statusRow = n
				if to == store.CategoryDone {
					resolvedRow = fmt.Sprintf("%s (%s)", n, h.At)
				}
				if store.ReopenTransition(from, to) {
					note = "  ← reopen"
					reopenRows = append(reopenRows, fmt.Sprintf("  %-4s %-24s %s", n, h.At, cats))
				}
			}
		}
		if h.Field == "assignee" && h.At != "" {
			assigneeRow = n
		}
		if h.At == "" {
			// Derive skips an entry with no timestamp; say so rather than let the
			// reader assume it counted.
			note += "  ← no timestamp, skipped by every rule"
		}
		fmt.Printf("  %-4s %-24s %-18s %-22s %s%s\n", n, h.At, h.Field, cats,
			side(h.FromValue, h.FromID)+" → "+side(h.ToValue, h.ToID), note)
	}
	if len(d.History) == 0 {
		fmt.Println("  (the mirror holds no changelog for this issue)")
	}

	fmt.Println()
	fmt.Printf("status_changed_at = %s\n", derefOrEmpty(got.StatusChangedAt, deriveNull))
	fmt.Println("  the newest changelog row whose field is status" + rowRef(statusRow))

	fmt.Printf("reopen_count = %d\n", got.ReopenCount)
	fmt.Println("  status rows whose from-category is done and whose to-category is not")
	for _, r := range reopenRows {
		fmt.Println(r)
	}
	if len(reopenRows) == 0 {
		fmt.Println("  none — nothing left the done category")
	}

	fmt.Printf("reopened_at = %s\n", derefOrEmpty(got.ReopenedAt, deriveNull))
	if got.ReopenedAt == nil {
		fmt.Println("  no reopen row above")
	} else {
		fmt.Println("  the newest of the reopen rows above")
	}

	fmt.Printf("resolved_at = %s\n", derefOrEmpty(got.ResolvedAt, deriveNull))
	switch {
	case resolvedRow == "":
		fmt.Println("  no changelog row ever moved this issue into category done")
	case l.StatusCategory != store.CategoryDone:
		fmt.Printf("  %s was the newest row into category done, but this issue's category\n", resolvedRow)
		fmt.Printf("  is %s now — a resolution that was undone is not a resolution date\n", l.StatusCategory)
	default:
		fmt.Printf("  %s was the newest row into category done, and the issue is still done\n", resolvedRow)
	}

	fmt.Printf("reopen_reason = %s\n", oneLine(got.ReopenReason, "(empty)"))
	printReopenReason(in.Comments, got)

	fmt.Printf("assignee_changed_at = %s\n", derefOrEmpty(got.AssigneeChangedAt, deriveNull))
	fmt.Println("  the newest changelog row whose field is assignee" + rowRef(assigneeRow))

	fmt.Printf("comment_count = %d\n", got.CommentCount)
	fmt.Println("  rows in comments for this issue — a count, not a changelog rule")

	fmt.Printf("priority_rank = %d\n", got.PriorityRank)
	fmt.Printf("  1-based position of %q in the site's %d-priority list; 0 means unset or\n",
		derefOrEmpty(l.Priority, ""), len(c.priorities))
	fmt.Println("  not on the list. Sort on this, never on the priority name")

	fmt.Printf("cloned_from = %s\n", orNone(got.ClonedFrom))
	printClonedFrom(d.LinkedIssues, got.ClonedFrom)

	printEpicKey(l, c)
	printAgreement(l, got)
	return nil
}

// side renders one end of a changelog transition: the display name the account
// sees, plus the id every rule actually keys on. An empty end is the field
// being set or cleared, which is not the same as an empty name.
func side(value, id string) string {
	// A rich-text custom field carries a whole document, and a row of the table
	// is a row: without this, one edit to such a field pushed the rules the
	// reader came for off the screen — measured, 206 lines for one issue.
	// Two of these share a row, so each gets half the budget; the full value is
	// still one `gadak issue <KEY>` away.
	value = clip(value, 40)
	switch {
	case value == "" && id == "":
		return "—"
	case value == "":
		return "(" + id + ")"
	case id == "":
		return value
	}
	return value + " (" + id + ")"
}

// rowRef names the row a rule landed on, or says there was none.
func rowRef(row string) string {
	if row == "" {
		return " — there is none"
	}
	return " (" + row + ")"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// oneLine keeps a value printable on the `name = value` line: a comment body may
// hold newlines, and a wrapped value would break the one-name-per-line shape the
// output's own field-name check relies on. The untruncated text is printed in
// the evidence block underneath.
func oneLine(s, empty string) string {
	s = clip(s, 72)
	if s == "" {
		return empty
	}
	return s
}

// clip is package main's short name for term.Clip (GDK-1811), the single
// owner of collapse-then-truncate. No logic lives here.
func clip(s string, cols int) string { return fields.Clip(s, cols) }

func orNone(s string) string {
	if s == "" {
		return deriveNull
	}
	return s
}

func printReopenReason(comments []store.Comment, got store.Derived) {
	if got.ReopenedAt == nil {
		fmt.Println("  the issue was never reopened, so there is no comment to attribute")
		return
	}
	fmt.Printf("  the earliest comment written at or after reopened_at (%s)\n", *got.ReopenedAt)
	if got.ReopenReason == "" {
		fmt.Println("  no comment followed the reopen")
		return
	}
	for _, c := range comments {
		if c.CreatedAt == "" || c.CreatedAt < *got.ReopenedAt || c.BodyText != got.ReopenReason {
			continue
		}
		fmt.Printf("  comment %s by %s at %s\n", derefOrEmpty(&c.ExternalID, c.ID), c.Author, c.CreatedAt)
		break
	}
	fmt.Println(indent(got.ReopenReason))
}

func printClonedFrom(links []store.DetailLink, clonedFrom string) {
	fmt.Println("  target of an inward link whose type name contains \"clone\"")
	for _, k := range links {
		if k.Key == clonedFrom && clonedFrom != "" {
			fmt.Printf("  %s %s %s\n", k.Type, k.Direction, k.Key)
			return
		}
	}
	fmt.Printf("  none of this issue's %d links qualifies. A site whose clone link type\n", len(links))
	fmt.Println("  carries a non-English name derives nothing here — there is no id to key on")
}

// printEpicKey shows the chain, not a second walk of it. The rule lives in the
// UPDATE store.recomputeEpicKeys runs after every upsert batch; re-deciding the
// winner here would be the second derivation this command exists to prevent.
func printEpicKey(l store.IssueLite, c *deriveContext) {
	fmt.Printf("epic_key = %s\n", derefOrEmpty(l.EpicKey, deriveNull))
	fmt.Println("  the nearest hierarchy_level = 1 ancestor along parent_key, computed in SQL")
	fmt.Println("  after every upsert batch — shown here from the stored chain, not recomputed")
	stored := derefOrEmpty(l.EpicKey, "")
	for _, hop := range c.chain {
		label := fmt.Sprintf("  %s (hierarchy_level %d)", hop.key, hop.level)
		switch {
		case hop.key == stored && stored != "":
			label += "  ← the epic this row names"
		case hop.parentKey == "":
			label += "  — no parent_key, the chain ends here"
		default:
			label += "  → parent " + hop.parentKey
		}
		fmt.Println(label)
	}
}

// printAgreement is the payoff: the stored columns were written by store.Derive
// at sync time and the values above were produced by calling it again now. A
// difference means the mirror is behind the code, or that a status id in this
// changelog no longer belongs to any mirrored issue ("NOT covered" above).
func printAgreement(l store.IssueLite, got store.Derived) {
	// stored and fresh are compared whole and only shortened for printing, so two
	// long reopen_reason values that share a prefix cannot read as agreement.
	type pair struct {
		name          string
		stored, fresh string
		long          bool
	}
	pairs := []pair{
		{name: "status_changed_at", stored: derefOrEmpty(l.StatusChangedAt, deriveNull), fresh: derefOrEmpty(got.StatusChangedAt, deriveNull)},
		{name: "resolved_at", stored: derefOrEmpty(l.ResolvedAt, deriveNull), fresh: derefOrEmpty(got.ResolvedAt, deriveNull)},
		{name: "reopen_count", stored: fmt.Sprint(l.ReopenCount), fresh: fmt.Sprint(got.ReopenCount)},
		{name: "reopened_at", stored: derefOrEmpty(l.ReopenedAt, deriveNull), fresh: derefOrEmpty(got.ReopenedAt, deriveNull)},
		{name: "reopen_reason", stored: derefOrEmpty(l.ReopenReason, ""), fresh: got.ReopenReason, long: true},
		{name: "comment_count", stored: fmt.Sprint(l.CommentCount), fresh: fmt.Sprint(got.CommentCount)},
		{name: "priority_rank", stored: fmt.Sprint(l.PriorityRank), fresh: fmt.Sprint(got.PriorityRank)},
		{name: "cloned_from", stored: derefOrEmpty(l.ClonedFrom, deriveNull), fresh: orNone(got.ClonedFrom)},
	}
	var differ []pair
	for _, p := range pairs {
		if p.stored != p.fresh {
			differ = append(differ, p)
		}
	}
	fmt.Printf("\nagreement with the %d stored columns\n", len(pairs))
	if len(differ) == 0 {
		fmt.Println("  every value above matches what the last sync wrote")
		return
	}
	for _, p := range differ {
		stored, fresh := p.stored, p.fresh
		if p.long {
			stored, fresh = oneLine(stored, "(empty)"), oneLine(fresh, "(empty)")
		}
		fmt.Printf("  %s: stored %s, recomputed %s\n", p.name, stored, fresh)
	}
	fmt.Println("  a difference means the mirror predates the current rules, or a status id in")
	fmt.Println("  this changelog is no longer used by any mirrored issue — re-run `gadak sync`")
}

/* ── search ── */
