package main

// The agent-facing commands: one issue, one search, and the writes, each
// self-contained enough for a coding agent to run from a shell with no session
// and no state beyond the mirror and the stored credential
// (specs/000-product/contracts/agent.md).
//
// Reads come from the mirror and never call Jira. Writes go to Jira first and
// re-read the issue into the mirror afterwards, in that order — the same
// write-through shape internal/server/write.go implements for the UI, because a
// write Jira rejected must not leave a trace locally.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// staleAfter is when a mirror stops being worth trusting silently. It is a
// warning, never a refusal: an old answer with a warning beats no answer.
const staleAfter = time.Hour

// warnIfStale prints one stderr line when the last sync failed or is old, so a
// caller reading stdout knows how far behind the answer may be. stdout stays
// clean, which is what makes the output pipeable.
func warnIfStale() {
	db, err := openReadOnly()
	if err != nil {
		return
	}
	defer db.Close()
	var syncedAt, lastErr *string
	if err := db.QueryRow(`SELECT src.synced_at, st.last_error
		FROM sync_state st LEFT JOIN sources src ON src.id = st.source_id
		WHERE st.source_id = 'jira'`).Scan(&syncedAt, &lastErr); err != nil {
		return
	}
	warn := func(format string, a ...any) { fmt.Fprintf(os.Stderr, "warning: "+format+"\n", a...) }
	switch {
	case lastErr != nil && *lastErr != "":
		warn("last sync failed: %s", *lastErr)
	case syncedAt == nil || *syncedAt == "":
		warn("the mirror has never finished a sync — run `gadak sync`")
	default:
		t, err := time.Parse(time.RFC3339, *syncedAt)
		if err == nil && time.Since(t) > staleAfter {
			warn("mirror last synced %s ago — run `gadak sync`", time.Since(t).Round(time.Minute))
		}
	}
}

// normalizeKey accepts a key in any case; Jira's are uppercase.
func normalizeKey(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// leading peels up to n non-flag arguments off the front so flags may follow
// them: Go's flag package stops parsing at the first non-flag, which would make
// `gadak comment NMB-1 -m ok` silently drop the -m. A bare `-` is a value, not a
// flag — it is how `assign` unassigns.
//
// Callers that take positionals should use parseAround instead (GDK-41): it
// applies the same peel and rejects an unknown dash-token. leading stays for
// the peel-only shape; it does not know the FlagSet, so it cannot reject.
func leading(args []string, n int) (positional, rest []string) {
	for len(args) > 0 && len(positional) < n && (args[0] == "-" || !strings.HasPrefix(args[0], "-")) {
		positional = append(positional, args[0])
		args = args[1:]
	}
	return positional, args
}

// lookup returns the IssueLite rows for the given keys, in the order asked, and
// skips keys the mirror does not have. The store exposes no single-key read, so
// this filters the full list — which is what the server's write path does too,
// and cheap enough at mirror scale.
func lookup(db *store.DB, keys []string) ([]store.IssueLite, error) {
	all, err := db.IssueLites(context.Background())
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]store.IssueLite, len(all))
	for _, l := range all {
		byKey[l.IssueKey] = l
	}
	out := make([]store.IssueLite, 0, len(keys))
	for _, k := range keys {
		if l, ok := byKey[k]; ok {
			out = append(out, l)
		}
	}
	return out, nil
}

// summaryLine is the one-line rendering shared by search results and the
// confirmation a write prints. Tab-separated for the same reason `sql` is:
// `cut -f1` has to work.
func summaryLine(l store.IssueLite) string {
	return strings.Join([]string{l.IssueKey, l.Status, deref(l.Assignee, "(unassigned)"), l.Summary}, "\t")
}

func deref(s *string, fallback string) string {
	if s == nil || *s == "" {
		return fallback
	}
	return *s
}

/* ── issue ── */

func cmdIssue(args []string) error {
	fs := newFlagSet("issue")
	asJSON := fs.Bool("json", false, "emit the detail document as JSON")
	derive := fs.Bool("derive", false, "instead of the detail, show how the derived fields were computed: the changelog by status category, and the rows behind reopen_count, resolved_at, reopen_reason and epic_key")
	link := fs.Bool("link", false, "print the gadak:// issue link (and the http form when a serve is listening)")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("issue", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageError("issue", "usage: gadak issue <KEY> [--json] [--derive] [--link]")
	}
	// The --json document is what agents parse; --derive is prose for a person.
	// Folding one into the other would reshape a contract that already has
	// consumers, so the combination is refused rather than silently ignored.
	if *asJSON && *derive {
		return usageError("issue", "--derive and --json cannot be combined: --derive is a human-readable explanation, and --json is the document agents parse")
	}
	if *link && *derive {
		return usageError("issue", "--derive and --link cannot be combined: --derive explains the stored columns, and --link prints the issue's address")
	}
	key := normalizeKey(pos[0])

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale()

	if *link {
		return printIssueLink(db, key, *asJSON)
	}

	d, err := db.Detail(context.Background(), key)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("%s is not in the mirror — check the key, or run `gadak sync`", key)
	}
	if err != nil {
		return err
	}
	lites, err := lookup(db, []string{key})
	if err != nil {
		return err
	}
	if len(lites) == 0 {
		return fmt.Errorf("%s has a detail row but no issue row — the mirror is inconsistent, re-sync", key)
	}

	if *asJSON {
		// The detail fields are flattened alongside `issue` so the document reads
		// like `GET <key>/detail/` with the list row included.
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Issue store.IssueLite `json:"issue"`
			*store.Detail
		}{lites[0], d})
	}
	if *derive {
		return printDerivation(lites[0], d)
	}
	printIssue(lites[0], d)
	return nil
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

func printIssue(l store.IssueLite, d *store.Detail) {
	fmt.Printf("%s\t%s\n", l.IssueKey, l.Summary)
	kv := func(label, value string) {
		if value != "" {
			fmt.Printf("%-13s %s\n", label, value)
		}
	}
	kv("project", l.ProjectKey)
	kv("type", l.IssueType)
	kv("status", fmt.Sprintf("%s (%s)", l.Status, l.StatusCategory))
	kv("priority", deref(l.Priority, ""))
	kv("assignee", deref(l.Assignee, "(unassigned)"))
	kv("reporter", deref(l.Reporter, ""))
	kv("parent", deref(l.ParentKey, ""))
	kv("epic", deref(l.EpicKey, ""))
	kv("labels", strings.Join(l.Labels, ", "))
	kv("components", strings.Join(l.Components, ", "))
	kv("fix versions", strings.Join(l.FixVersions, ", "))
	kv("duedate", deref(l.Duedate, ""))
	kv("resolution", deref(l.Resolution, ""))
	kv("created", deref(l.CreatedAt, ""))
	kv("updated", deref(l.UpdatedAt, ""))
	kv("status since", deref(l.StatusChangedAt, ""))
	kv("resolved", deref(l.ResolvedAt, ""))
	if l.ReopenCount > 0 {
		kv("reopens", fmt.Sprintf("%d (last %s)", l.ReopenCount, deref(l.ReopenedAt, "?")))
	}
	// Sorted: map order would make two runs on the same issue differ.
	aliases := make([]string, 0, len(l.Custom))
	for alias := range l.Custom {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		kv(alias, fmt.Sprint(l.Custom[alias]))
	}

	if body := strings.TrimSpace(jira.PlainText(d.DescriptionADF)); body != "" {
		fmt.Printf("\ndescription\n%s\n", indent(body))
	} else if text := strings.TrimSpace(d.DescriptionText); text != "" {
		fmt.Printf("\ndescription\n%s\n", indent(text))
	}
	if len(d.Comments) > 0 {
		fmt.Printf("\ncomments (%d)\n", len(d.Comments))
		for _, c := range d.Comments {
			body := strings.TrimSpace(c.Body)
			if body == "" {
				body = strings.TrimSpace(jira.PlainText(c.BodyADF))
			}
			fmt.Printf("  %s  %s\n%s\n", c.CreatedAt, c.Author, indent(body))
		}
	}
	if len(d.Attachments) > 0 {
		fmt.Printf("\nattachments (%d)\n", len(d.Attachments))
		for _, a := range d.Attachments {
			fmt.Printf("  %s\t%s\t%d bytes\n", a.Filename, a.MimeType, a.Size)
		}
	}
	if len(d.LinkedIssues) > 0 {
		fmt.Printf("\nlinks (%d)\n", len(d.LinkedIssues))
		for _, k := range d.LinkedIssues {
			fmt.Printf("  %s %s\t%s\t%s\n", k.Type, k.Direction, k.Key, k.Summary)
		}
	}
	if len(d.History) > 0 {
		fmt.Printf("\nhistory (%d)\n", len(d.History))
		for _, h := range d.History {
			fmt.Printf("  %s  %s\t%s: %s → %s\n", h.At, h.Author, h.Field, h.FromValue, h.ToValue)
		}
	}
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
		Priority:        deref(l.Priority, ""),
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
			body = jira.PlainText(cm.BodyADF)
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
				if from == store.CategoryDone && to != store.CategoryDone {
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
	fmt.Printf("status_changed_at = %s\n", deref(got.StatusChangedAt, deriveNull))
	fmt.Println("  the newest changelog row whose field is status" + rowRef(statusRow))

	fmt.Printf("reopen_count = %d\n", got.ReopenCount)
	fmt.Println("  status rows whose from-category is done and whose to-category is not")
	for _, r := range reopenRows {
		fmt.Println(r)
	}
	if len(reopenRows) == 0 {
		fmt.Println("  none — nothing left the done category")
	}

	fmt.Printf("reopened_at = %s\n", deref(got.ReopenedAt, deriveNull))
	if got.ReopenedAt == nil {
		fmt.Println("  no reopen row above")
	} else {
		fmt.Println("  the newest of the reopen rows above")
	}

	fmt.Printf("resolved_at = %s\n", deref(got.ResolvedAt, deriveNull))
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

	fmt.Printf("assignee_changed_at = %s\n", deref(got.AssigneeChangedAt, deriveNull))
	fmt.Println("  the newest changelog row whose field is assignee" + rowRef(assigneeRow))

	fmt.Printf("comment_count = %d\n", got.CommentCount)
	fmt.Println("  rows in comments for this issue — a count, not a changelog rule")

	fmt.Printf("priority_rank = %d\n", got.PriorityRank)
	fmt.Printf("  1-based position of %q in the site's %d-priority list; 0 means unset or\n",
		deref(l.Priority, ""), len(c.priorities))
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

// clip flattens a value onto one line and cuts it to a column budget. Columns,
// not runes: a Hangul or CJK rune occupies two cells, so a rune-counted cut
// renders twice as wide as the same cut in ASCII — which is how a 72-"character"
// column landed at 144 on a Korean issue. runewidth is already this binary's
// width authority (fields.go pads its tables with it).
func clip(s string, cols int) string {
	s = strings.Join(strings.Fields(s), " ")
	if runewidth.StringWidth(s) <= cols {
		return s
	}
	return runewidth.Truncate(s, cols, "…")
}

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
		fmt.Printf("  comment %s by %s at %s\n", deref(&c.ExternalID, c.ID), c.Author, c.CreatedAt)
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
	fmt.Printf("epic_key = %s\n", deref(l.EpicKey, deriveNull))
	fmt.Println("  the nearest hierarchy_level = 1 ancestor along parent_key, computed in SQL")
	fmt.Println("  after every upsert batch — shown here from the stored chain, not recomputed")
	stored := deref(l.EpicKey, "")
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
		{name: "status_changed_at", stored: deref(l.StatusChangedAt, deriveNull), fresh: deref(got.StatusChangedAt, deriveNull)},
		{name: "resolved_at", stored: deref(l.ResolvedAt, deriveNull), fresh: deref(got.ResolvedAt, deriveNull)},
		{name: "reopen_count", stored: fmt.Sprint(l.ReopenCount), fresh: fmt.Sprint(got.ReopenCount)},
		{name: "reopened_at", stored: deref(l.ReopenedAt, deriveNull), fresh: deref(got.ReopenedAt, deriveNull)},
		{name: "reopen_reason", stored: deref(l.ReopenReason, ""), fresh: got.ReopenReason, long: true},
		{name: "comment_count", stored: fmt.Sprint(l.CommentCount), fresh: fmt.Sprint(got.CommentCount)},
		{name: "priority_rank", stored: fmt.Sprint(l.PriorityRank), fresh: fmt.Sprint(got.PriorityRank)},
		{name: "cloned_from", stored: deref(l.ClonedFrom, deriveNull), fresh: orNone(got.ClonedFrom)},
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

func cmdSearch(args []string) error {
	// Flags may sit before or after the query. `gadak search "flaky" --limit 5`
	// and `gadak search --jql 'project = NMA' --json` both have to work;
	// FlagSet alone swallows a trailing --json into the JQL.
	fs := newFlagSet("search")
	limit := fs.Int("limit", 20, "maximum matches")
	asJSON := fs.Bool("json", false, "emit matching IssueLite rows as JSON")
	forceJQL := fs.Bool("jql", false, "treat the query as JQL (or a Jira URL with jql=)")
	emitOnly := fs.Bool("emit", false, "print the canonical JQL and exit (no search)")
	explain := fs.Bool("explain", false, "print why each hit ranked: key-exact, key-prefix, or fts with bm25 score and column; --json adds elapsed_ms")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("search", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(pos, " "))
	if query == "" {
		return usageError("search", `usage: gadak search [--jql] [--emit] [--limit N] [--json] [--explain] "text|JQL|URL"`)
	}
	asJQL := *forceJQL || jql.LooksLike(query)
	// An unquoted multi-word FTS query swallows the flags that follow it.
	// JQL uses `-7d` and must not trip this.
	if !asJQL && strings.Contains(query, " -") {
		return fmt.Errorf("quote the search text: %q reads a flag as part of the query", query)
	}
	if asJQL {
		return searchJQL(query, *limit, *asJSON, *emitOnly, *forceJQL)
	}
	if *emitOnly {
		return fmt.Errorf("--emit needs JQL (pass --jql or paste a Jira URL / JQL clause)")
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale()

	var res store.SearchResult
	if *explain {
		res, err = db.SearchExplain(context.Background(), query, *limit)
	} else {
		res, err = db.Search(context.Background(), query, *limit)
	}
	if err != nil {
		return err
	}
	// Best match first: lookup preserves the order Search ranked the keys in.
	lites, err := lookup(db, res.Keys)
	if err != nil {
		return err
	}
	matches := res.Matches
	if matches == nil {
		matches = map[string]store.SearchMatch{}
	}
	if *asJSON {
		pages := res.Pages
		if pages == nil {
			pages = []store.PageLite{}
		}
		body := map[string]any{
			"total": res.Total, "issues": lites, "pages": pages, "matches": matches,
		}
		if *explain {
			ex := res.Explain
			if ex == nil {
				ex = []store.SearchExplain{}
			}
			body["explain"] = ex
			body["elapsed_ms"] = res.ElapsedMS
		}
		return json.NewEncoder(os.Stdout).Encode(body)
	}
	byExplain := indexSearchExplain(res.Explain)
	for _, l := range lites {
		line := summaryLine(l)
		if m, ok := matches[l.IssueKey]; ok && (m.Field == "comment" || m.Field == "body") {
			line += fmt.Sprintf(" (%s: %s)", m.Field, m.Snippet)
		}
		if *explain {
			line += formatSearchExplain(byExplain[l.IssueKey])
		}
		fmt.Println(line)
	}
	for _, p := range res.Pages {
		// page  <space>/<title>  <url>
		line := fmt.Sprintf("page  %s/%s  %s", p.SpaceKey, p.Title, p.URL)
		if m, ok := matches[p.Key]; ok && (m.Field == "comment" || m.Field == "body") {
			line += fmt.Sprintf(" (%s: %s)", m.Field, m.Snippet)
		}
		if *explain {
			line += formatSearchExplain(byExplain[p.Key])
		}
		fmt.Println(line)
	}
	if *explain {
		fmt.Printf("query %.1fms\n", res.ElapsedMS)
	}
	return nil
}

func indexSearchExplain(rows []store.SearchExplain) map[string]store.SearchExplain {
	out := make(map[string]store.SearchExplain, len(rows))
	for _, e := range rows {
		if _, ok := out[e.Key]; !ok {
			out[e.Key] = e
		}
	}
	return out
}

// formatSearchExplain is the text suffix for --explain. Reasons are the
// store values key-exact, key-prefix, and fts; fts also prints the winning
// column and bm25 score when the store supplied them.
func formatSearchExplain(e store.SearchExplain) string {
	if e.Reason == "" {
		return ""
	}
	if e.Reason == "fts" {
		if e.Score != nil && e.Field != "" {
			return fmt.Sprintf(" (%s %s bm25=%.4f)", e.Reason, e.Field, *e.Score)
		}
		if e.Field != "" {
			return fmt.Sprintf(" (%s %s)", e.Reason, e.Field)
		}
	}
	return fmt.Sprintf(" (%s)", e.Reason)
}

func searchJQL(query string, limit int, asJSON, emitOnly, force bool) error {
	opts := jql.Opts{Email: configuredEmail()}
	parsed := jql.Parse(query, opts)
	if parsed.Error == jql.ErrNotJQL && !force {
		return fmt.Errorf("not JQL: %s", parsed.Message)
	}
	if parsed.Error != "" {
		switch parsed.Error {
		case jql.ErrFilterID:
			return fmt.Errorf("%s", parsed.Message)
		case jql.ErrParse:
			return fmt.Errorf("cannot parse JQL: %s", parsed.Message)
		default:
			return fmt.Errorf("jql: %s", parsed.Message)
		}
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale()

	lites, err := db.IssueLites(context.Background())
	if err != nil {
		return err
	}
	people := jql.PeopleFromIssues(jqlIssues(lites))
	jql.ResolvePeople(&parsed, people, opts.Email)

	if emitOnly {
		if asJSON {
			return json.NewEncoder(os.Stdout).Encode(parsed)
		}
		if parsed.JQL != "" {
			fmt.Println(parsed.JQL)
		}
		warnJQL(parsed)
		return nil
	}
	if len(parsed.Applied) == 0 && len(parsed.Unsupported) > 0 {
		return fmt.Errorf("cannot apply JQL — %s", strings.Join(parsed.Unsupported, "; "))
	}

	matched := make([]store.IssueLite, 0)
	for _, l := range lites {
		if jql.Match(jqlIssue(l), parsed.Filters) {
			matched = append(matched, l)
		}
	}
	sortJQL(matched, parsed.Display)
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	warnJQL(parsed)

	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"total":       len(matched),
			"issues":      matched,
			"pages":       []store.PageLite{},
			"jql":         parsed.JQL,
			"applied":     parsed.Applied,
			"unsupported": parsed.Unsupported,
		})
	}
	for _, l := range matched {
		fmt.Println(summaryLine(l))
	}
	return nil
}

func warnJQL(parsed jql.Result) {
	if len(parsed.Unsupported) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: JQL skipped %s\n", strings.Join(parsed.Unsupported, "; "))
}

func configuredEmail() string {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.Email
}

func jqlIssue(l store.IssueLite) jql.Issue {
	return jql.Issue{
		Key:            l.IssueKey,
		Project:        l.ProjectKey,
		Status:         l.Status,
		StatusCategory: l.StatusCategory,
		Type:           l.IssueType,
		Priority:       deref(l.Priority, ""),
		Assignee:       deref(l.Assignee, ""),
		AssigneeEmail:  deref(l.AssigneeEmail, ""),
		AssigneeID:     deref(l.AssigneeID, ""),
		Reporter:       deref(l.Reporter, ""),
		ReporterEmail:  deref(l.ReporterEmail, ""),
		Labels:         l.Labels,
		Components:     l.Components,
		FixVersions:    l.FixVersions,
		CreatedAt:      deref(l.CreatedAt, ""),
		UpdatedAt:      deref(l.UpdatedAt, ""),
		Duedate:        deref(l.Duedate, ""),
		ResolvedAt:     deref(l.ResolvedAt, ""),
	}
}

func jqlIssues(lites []store.IssueLite) []jql.Issue {
	out := make([]jql.Issue, len(lites))
	for i, l := range lites {
		out[i] = jqlIssue(l)
	}
	return out
}

func sortJQL(list []store.IssueLite, d jql.Display) {
	dir := 1
	if d.Dir != "asc" {
		dir = -1
	}
	lessTime := func(a, b *string) bool {
		av, bv := deref(a, ""), deref(b, "")
		if dir < 0 {
			return av > bv
		}
		return av < bv
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		switch d.Sort {
		case "created":
			return lessTime(a.CreatedAt, b.CreatedAt)
		case "priority":
			if a.PriorityRank != b.PriorityRank {
				if dir < 0 {
					return a.PriorityRank < b.PriorityRank
				}
				return a.PriorityRank > b.PriorityRank
			}
			return deref(a.UpdatedAt, "") > deref(b.UpdatedAt, "")
		default:
			return lessTime(a.UpdatedAt, b.UpdatedAt)
		}
	})
}

/* ── writes ── */

// errNoCredential is the refusal mutate and create share: writes go to Jira.
var errNoCredential = errors.New("no Jira credential — run `gadak init` first (writes go to Jira, not to the mirror)")

// writeNotMirroredError is the lookup miss after a write Jira already accepted.
// mutate returns it (non-zero). create prints the new key with this wording
// and exits 0 — the write happened.
type writeNotMirroredError struct{ Key string }

func (e writeNotMirroredError) Error() string {
	return fmt.Sprintf("write applied to %s, but it is not in the mirror — is it outside the configured projects?", e.Key)
}

// withWriteSession loads the credential, opens the store, and hands the caller
// a Jira client. Shared by mutate and create so the refusal string cannot drift.
func withWriteSession(fn func(context.Context, *config.Config, *store.DB, *jira.Client) error) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasCredential() {
		return errNoCredential
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale()
	c, err := origin.Client(cfg)
	if err != nil {
		return err
	}
	return fn(context.Background(), cfg, db, c)
}

// withKeyWriteSession is withWriteSession routed per key: the mirror says
// which origin owns the row (store.KeySource — a "MID-5" can be Linear or
// Jira, the shape cannot tell), and the credential gate is that origin's.
func withKeyWriteSession(key string, fn func(context.Context, *config.Config, *store.DB, origin.Writer, string) error) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale()
	ctx := context.Background()
	src, err := db.KeySource(ctx, key)
	if err != nil {
		if errors.Is(err, store.ErrKeyAmbiguous) {
			return err
		}
		src = ""
	}
	if src != "linear" && !cfg.HasCredential() {
		return errNoCredential
	}
	c, err := origin.WriterFor(cfg, src)
	if err != nil {
		return err
	}
	return fn(ctx, cfg, db, c, src)
}

// emitAfterWrite is the write-through tail: re-read the issue into the mirror
// and print the refreshed row. A failure between the write and the re-read is
// reported as such, because retrying it would repeat the write Jira already
// accepted.
func emitAfterWrite(ctx context.Context, cfg *config.Config, db *store.DB, src, key string, asJSON bool, extra map[string]any) error {
	if err := refreshAfterWrite(ctx, cfg, db, src, key); err != nil {
		return fmt.Errorf("write applied to %s, but the mirror did not refresh (run `gadak sync`): %w", key, err)
	}
	lites, err := lookup(db, []string{key})
	if err != nil {
		return err
	}
	if len(lites) == 0 {
		return writeNotMirroredError{Key: key}
	}
	if asJSON {
		body := map[string]any{"issue": lites[0]}
		for k, v := range extra {
			body[k] = v
		}
		return json.NewEncoder(os.Stdout).Encode(body)
	}
	fmt.Println(summaryLine(lites[0]))
	return nil
}

// refreshAfterWrite re-reads one issue from the origin that owns it.
func refreshAfterWrite(ctx context.Context, cfg *config.Config, db *store.DB, src, key string) error {
	if src == "linear" {
		lc, err := origin.Linear(cfg)
		if err != nil {
			return err
		}
		return syncer.SyncLinearIssue(ctx, db, lc, key)
	}
	return syncer.SyncIssue(ctx, cfg, db, key, syncer.Options{})
}

// mutate is the whole write-through shape: call the origin that owns the
// key, re-read the issue into the mirror, then print the refreshed row.
func mutate(key string, asJSON bool, fn func(context.Context, origin.Writer) (map[string]any, error)) error {
	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		extra, err := fn(ctx, c)
		if err != nil {
			return err
		}
		return emitAfterWrite(ctx, cfg, db, src, key, asJSON, extra)
	})
}

func cmdComment(args []string) error {
	fs := newFlagSet("comment")
	text := fs.String("m", "", "comment body; `-` reads it from stdin")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("comment", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageError("comment", "usage: gadak comment <KEY> -m <text> [--json]")
	}
	key := normalizeKey(pos[0])
	body := *text
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("empty comment — pass -m <text>, or -m - to read stdin")
	}
	return mutate(key, *asJSON, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		// No mention resolution: `@Name` in a CLI body stays plain text and notifies
		// nobody. ponytail: add it when someone asks, via the users endpoint the UI uses.
		created, err := c.AddComment(ctx, key, jira.Doc(body, nil))
		if err != nil {
			return nil, err
		}
		return map[string]any{"comment": map[string]any{
			"comment_id": created.ID,
			"author":     created.Author.DisplayName,
			"body":       jira.PlainText(created.Body),
		}}, nil
	})
}

func cmdTransition(args []string) error {
	fs := newFlagSet("transition")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("transition", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return usageError("transition", "usage: gadak transition <KEY> <transition-id|status-id|name|new|inprogress|done> [--json]")
	}
	key := normalizeKey(pos[0])
	// Trailing words join the target so an unquoted `In Review` still works.
	want := strings.TrimSpace(strings.Join(pos[1:], " "))

	return mutate(key, *asJSON, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
		list, err := c.Transitions(ctx, key)
		if err != nil {
			return nil, err
		}
		id, err := pickTransition(key, want, list)
		if err != nil {
			return nil, err
		}
		return nil, c.Transition(ctx, key, id)
	})
}

// pickTransition resolves want against the issue's available transitions.
// Order: transition id, target status id, transition name / target status
// name, then category tokens (new|inprogress|done). Two landings in the same
// category refuse rather than picking the first. A token that is one
// transition's id and a different transition's to.id is also refused.
func pickTransition(key, want string, list []jira.Transition) (string, error) {
	var idHit *jira.Transition
	var toHits []jira.Transition
	for i := range list {
		t := &list[i]
		if t.ID == want && idHit == nil {
			idHit = t
		}
		if t.To.ID != "" && t.To.ID == want {
			toHits = append(toHits, *t)
		}
	}
	if idHit != nil {
		var others []jira.Transition
		for _, t := range toHits {
			if t.ID != idHit.ID {
				others = append(others, t)
			}
		}
		if len(others) > 0 {
			return "", fmt.Errorf("%q matches a transition id and a different target status id on %s — transition id: %s; target status id: %s",
				want, key, formatTransition(*idHit), joinTransitions(others))
		}
		return idHit.ID, nil
	}
	switch len(toHits) {
	case 1:
		return toHits[0].ID, nil
	case 0:
		// names, then category
	default:
		return "", fmt.Errorf("transition %q is ambiguous on %s — %d transitions land there: %s",
			want, key, len(toHits), joinTransitions(toHits))
	}
	for _, t := range list {
		if strings.EqualFold(t.Name, want) || strings.EqualFold(t.To.Name, want) {
			return t.ID, nil
		}
	}
	if token, ok := statusCategoryToken(want); ok {
		var hits []jira.Transition
		for _, t := range list {
			if cat, ok := transitionCategory(t); ok && cat == token {
				hits = append(hits, t)
			}
		}
		switch len(hits) {
		case 1:
			return hits[0].ID, nil
		case 0:
			// fall through to the shared miss error, which names reachable tokens
		default:
			return "", fmt.Errorf("transition %q is ambiguous on %s — %d transitions land there: %s",
				want, key, len(hits), joinTransitions(hits))
		}
	}
	return "", noTransitionMatch(key, want, list)
}

// statusCategoryToken accepts only the three values data-model.md documents.
// jira.Category and jql.mapStatusCategory both fold aliases (todo, indeterminate)
// onto those values; applying either to the user token would reopen the
// localization trap this command is closing.
func statusCategoryToken(s string) (string, bool) {
	switch strings.ToLower(s) {
	case "new", "inprogress", "done":
		return strings.ToLower(s), true
	default:
		return "", false
	}
}

// transitionCategory maps a transition's Jira statusCategory key onto the
// three documented tokens. Empty and unknown keys are refused: jira.Category
// folds those to "new", which would move the issue on a damaged payload.
func transitionCategory(t jira.Transition) (string, bool) {
	switch t.To.StatusCategory.Key {
	case "new", "indeterminate", "inprogress", "done":
		return jira.Category(t.To.StatusCategory.Key), true
	default:
		return "", false
	}
}

func formatTransition(t jira.Transition) string {
	if t.To.ID == "" {
		return fmt.Sprintf("%s (id %s, → %s)", t.Name, t.ID, t.To.Name)
	}
	return fmt.Sprintf("%s (id %s, → %s [status_id %s])", t.Name, t.ID, t.To.Name, t.To.ID)
}

func joinTransitions(list []jira.Transition) string {
	parts := make([]string, 0, len(list))
	for _, t := range list {
		parts = append(parts, formatTransition(t))
	}
	return strings.Join(parts, "; ")
}

func noTransitionMatch(key, want string, list []jira.Transition) error {
	if len(list) == 0 {
		return fmt.Errorf("%s has no available transitions for this credential", key)
	}
	msg := fmt.Sprintf("no transition matching %q on %s — available: %s",
		want, key, joinTransitions(list))
	if cats := reachableCategories(list); len(cats) > 0 {
		msg += "\nalso accepts a status category: " + strings.Join(cats, ", ")
	}
	return errors.New(msg)
}

func reachableCategories(list []jira.Transition) []string {
	seen := map[string]bool{}
	var out []string
	for _, token := range []string{"new", "inprogress", "done"} {
		for _, t := range list {
			cat, ok := transitionCategory(t)
			if ok && cat == token && !seen[token] {
				seen[token] = true
				out = append(out, token)
			}
		}
	}
	return out
}

func cmdAssign(args []string) error {
	fs := newFlagSet("assign")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("assign", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return usageError("assign", "usage: gadak assign <KEY> <email|-> [--json]")
	}
	key, who := normalizeKey(pos[0]), strings.TrimSpace(pos[1])

	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		id, err := resolveAccount(ctx, c, who, src)
		if err != nil {
			return err
		}
		if err := c.SetAssignee(ctx, key, id); err != nil {
			return err
		}
		return emitAfterWrite(ctx, cfg, db, src, key, *asJSON, nil)
	})
}

// resolveAccount turns an email into an origin account id: `-` unassigns.
// Jira rows may use the configured member directory (JiraAccountID) without
// a network call. Linear rows must not — that id is a Jira account, and
// Linear assign wants a Linear user UUID from Writer.SearchUsers.
func resolveAccount(ctx context.Context, c origin.Writer, who, source string) (string, error) {
	if who == "-" {
		return "", nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if source != "linear" {
		for _, m := range cfg.Members {
			if strings.EqualFold(m.Email, who) && m.JiraAccountID != "" {
				return m.JiraAccountID, nil
			}
		}
	}
	users, err := c.SearchUsers(ctx, who)
	if err != nil {
		return "", err
	}
	for _, u := range users {
		if strings.EqualFold(u.Email, who) {
			return u.AccountID, nil
		}
	}
	// A site that hides emails answers with no email to match on, so a single hit
	// is taken at its word and an ambiguous one is refused rather than guessed.
	if len(users) == 1 {
		return users[0].AccountID, nil
	}
	if len(users) == 0 {
		return "", fmt.Errorf("no user on this issue's origin matches %q", who)
	}
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, fmt.Sprintf("%s <%s>", u.DisplayName, u.Email))
	}
	return "", fmt.Errorf("%q matches %d users — be more specific: %s", who, len(users), strings.Join(names, "; "))
}

// cmdOpen jumps from a key in the terminal to the issue on the Jira site.
// gadak is the fast path for reading; this is the escape hatch for everything
// the mirror deliberately does not do (boards, admin, workflow).
func cmdOpen(args []string) error {
	fs := newFlagSet("open")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("open", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageError("open", "usage: gadak open <KEY>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	key := normalizeKey(pos[0])
	// Key first: a missing row used to fall through to the no-site error and
	// tell an already-inited standalone workspace to re-run init. With a
	// site, a missing key still opens the browse URL — `open` is the escape
	// hatch to Jira, and the mirror may simply lag a key that exists there.
	found, stored, lookErr := lookupItem(key)
	if lookErr == nil && !found && cfg.Site == "" {
		return fmt.Errorf("%s is not in the mirror — check the key, or run `gadak sync`", key)
	}
	if u := absoluteHTTPURL(stored); u != "" {
		return openIssueURL(u)
	}
	if cfg.Site != "" {
		return openIssueURL(strings.TrimRight(cfg.Site, "/") + "/browse/" + url.PathEscape(key))
	}
	if web := serveFocusURL("issue=" + key); web != "" {
		return openIssueURL(web)
	}
	return fmt.Errorf("this workspace has no Jira site to browse — use `gadak views open %s` (or `gadak serve`)", key)
}

func openIssueURL(u string) error {
	if err := startIssueOpen(u); err != nil {
		return fmt.Errorf("could not open a browser (%v) — the URL is %s", err, u)
	}
	fmt.Println(u)
	return nil
}

// startIssueOpen is the browser opener for `gadak open`. Tests replace it.
var startIssueOpen = openBrowser

// lookupItem reports whether key is in items and any stored browse URL.
// A lookup failure (no mirror yet) returns err so callers can still fall
// through to the site browse path — the old lookupItemURL swallowed that.
func lookupItem(key string) (found bool, itemURL string, err error) {
	db, err := openReadOnly()
	if err != nil {
		return false, "", err
	}
	defer db.Close()
	var u sql.NullString
	err = db.QueryRow(`SELECT url FROM items WHERE key = ? LIMIT 1`, key).Scan(&u)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, strings.TrimSpace(u.String), nil
}

// absoluteHTTPURL accepts only http(s) URLs with a host. Standalone origin
// stores /browse/KEY (empty BaseURL); handing that to macOS `open` is a
// false success.
func absoluteHTTPURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return raw
	default:
		return ""
	}
}

// openBrowser starts the platform's URL opener and does not wait for it.
func openBrowser(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}
