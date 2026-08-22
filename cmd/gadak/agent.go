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
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/midagedev/gadak/internal/claim"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/jirafields"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
	"github.com/midagedev/gadak/internal/transition"
)

// staleAfter is when a mirror stops being worth trusting silently. It is a
// warning, never a refusal: an old answer with a warning beats no answer.
const staleAfter = time.Hour

// warnIfStale prints one stderr line when the last sync failed or is old, so a
// caller reading stdout knows how far behind the answer may be. stdout stays
// clean, which is what makes the output pipeable. It reads the caller's
// already-open connection — every caller has one, and a second open here
// doubled any diagnostic the open path prints (GDK-314).
func warnIfStale(db interface {
	QueryRow(query string, args ...any) *sql.Row
}) {
	type staleRow struct {
		id        string
		syncedAt  *string
		lastError *string
	}
	var rows []staleRow
	for off := 0; ; off++ {
		var r staleRow
		err := db.QueryRow(`SELECT st.source_id, src.synced_at, st.last_error
			FROM sync_state st LEFT JOIN sources src ON src.id = st.source_id
			ORDER BY st.source_id LIMIT 1 OFFSET ?`, off).Scan(&r.id, &r.syncedAt, &r.lastError)
		if err != nil {
			if off == 0 && !errors.Is(err, sql.ErrNoRows) {
				return
			}
			break
		}
		rows = append(rows, r)
	}
	warn := func(format string, a ...any) { fmt.Fprintf(os.Stderr, "warning: "+format+"\n", a...) }
	if len(rows) == 0 {
		warn("the mirror has never finished a sync — run `gadak sync`")
		return
	}
	for _, r := range rows {
		if r.lastError != nil && *r.lastError != "" {
			warn("last sync failed (%s): %s", r.id, *r.lastError)
			return
		}
	}
	var oldest *time.Time
	for _, r := range rows {
		if r.syncedAt == nil || *r.syncedAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, *r.syncedAt)
		if err != nil {
			continue
		}
		if oldest == nil || t.Before(*oldest) {
			tt := t
			oldest = &tt
		}
	}
	if oldest == nil {
		// Every source is empty. A leftover never-synced jira row next to
		// a fresh Linear source must not take this branch: that
		// is anyEmpty with oldest set from the Linear row.
		warn("the mirror has never finished a sync — run `gadak sync`")
		return
	}
	if time.Since(*oldest) > staleAfter {
		warn("mirror last synced %s ago — run `gadak sync --if-stale 1h`", time.Since(*oldest).Round(time.Minute))
	}
}

// normalizeKey accepts a key in any case; Jira's are uppercase.
func normalizeKey(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

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

const issueUsageLine = "usage: gadak issue <KEY> [KEY...] [--json] [--derive] [--link] [--editmeta]"

// issueDoc is the --json document: GET <key>/detail/ with the list row included.
// A single-key call encodes one of these as an object; two or more keys encode
// a JSON array of the same shape.
type issueDoc struct {
	Issue store.IssueLite `json:"issue"`
	*store.Detail
	LinkedPRs json.RawMessage `json:"linked_prs"`

	// durations is the text output's wait/progress line (GDK-591).
	// Unexported on purpose: --json is a parsed contract and keeps its
	// shape; the spans are computed, not stored, and the server gets its
	// own surface in GDK-590.
	durations store.Spans
}

// MarshalJSON adds `key` as an alias of `issue_key` (GDK-255). Detail itself
// cannot implement MarshalJSON: encoding/json then emits only that method's
// object and drops Issue / LinkedPRs (anonymous Marshaler embed).
func (d issueDoc) MarshalJSON() ([]byte, error) {
	type wire issueDoc
	key := d.Issue.IssueKey
	if d.Detail != nil && d.Detail.IssueKey != "" {
		key = d.Detail.IssueKey
	}
	return store.MarshalWithIssueKeyAlias(key, wire(d))
}

func cmdIssue(args []string) error {
	fs := newFlagSet("issue")
	asJSON := fs.Bool("json", false, "emit JSON (the detail document; with --editmeta, the editable-fields document)")
	derive := fs.Bool("derive", false, "instead of the detail, show how the derived fields were computed: the changelog by status category, and the rows behind reopen_count, resolved_at, reopen_reason and epic_key")
	link := fs.Bool("link", false, "print the gadak:// issue link (and the http form when a serve is listening)")
	editMeta := fs.Bool("editmeta", false, "ask the origin which configured fields this issue can edit (GET editmeta ∩ allowlist; not stored in the mirror)")
	keysFlag := fs.String("keys", "", "issue keys (comma or whitespace); - reads stdin")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("issue", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	keysRaw := strings.TrimSpace(*keysFlag)
	if keysRaw != "" && len(pos) > 0 {
		return usageError("issue", "--keys cannot be combined with positional issue keys")
	}
	var keys []string
	if keysRaw != "" {
		keys, err = readKeysFlag(keysRaw)
		if err != nil {
			return err
		}
	} else {
		keys = jql.SplitKeys(strings.Join(pos, " "))
	}
	if len(keys) == 0 {
		return usageError("issue", issueUsageLine)
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
	if *editMeta && *derive {
		return usageError("issue", "--derive and --editmeta cannot be combined: --derive explains the stored columns, and --editmeta asks the origin which fields this issue can edit")
	}
	if *editMeta && *link {
		return usageError("issue", "--link and --editmeta cannot be combined: --link prints the issue's address, and --editmeta asks the origin which fields this issue can edit")
	}
	if len(keys) > 1 {
		if *derive {
			return usageError("issue", "--derive cannot be combined with multiple keys: --derive explains the stored columns for one issue")
		}
		if *link {
			return usageError("issue", "--link cannot be combined with multiple keys: --link prints one issue's address")
		}
		if *editMeta {
			return usageError("issue", "--editmeta cannot be combined with multiple keys: --editmeta asks the origin which fields this issue can edit")
		}
	}
	if *editMeta {
		return printIssueEditMeta(keys[0], *asJSON)
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)

	if *link {
		return printIssueLink(db, keys[0], *asJSON)
	}

	docs, notFound, err := loadIssueDocs(db, keys)
	if err != nil {
		return err
	}
	if len(keys) == 1 && len(notFound) == 1 {
		return fmt.Errorf("%s is not in the mirror — check the key, or run `gadak sync`", notFound[0])
	}
	for _, k := range notFound {
		fmt.Fprintf(os.Stderr, "warning: %s is not in the mirror — check the key, or run `gadak sync`\n", k)
	}

	if *asJSON {
		if err := writeIssueJSON(docs, len(keys)); err != nil {
			return err
		}
	} else if *derive {
		if err := printDerivation(docs[0].Issue, docs[0].Detail); err != nil {
			return err
		}
	} else {
		printIssueDocs(docs)
	}
	if len(notFound) > 0 {
		return fmt.Errorf("%d of %d keys not in the mirror", len(notFound), len(keys))
	}
	return nil
}

// recordVisitBestEffort and recordSearchBestEffort append the personal-history
// row for one read, the same row the UI's POST /history/visits|searches would
// append (internal/server/history.go). Reading is the command; history is a
// side effect, so a local.db that cannot take the row must not fail the read:
// one stderr line, stdout untouched. Neither warning echoes its payload — the
// search rule that governs the server ("Search query text is not written to
// the process log") governs the CLI too.
func recordVisitBestEffort(db *store.DB, kind, key string) {
	if _, err := db.RecordVisit(context.Background(), kind, key); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not record this visit in local history: %v\n", err)
	}
}

func recordSearchBestEffort(db *store.DB, query string, resultCount int) {
	if _, err := db.RecordSearch(context.Background(), query, resultCount, "", ""); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not record this search in local history: %v\n", err)
	}
}

func loadIssueDocs(db *store.DB, keys []string) ([]issueDoc, []string, error) {
	lites, err := lookup(db, keys)
	if err != nil {
		return nil, nil, err
	}
	byKey := make(map[string]store.IssueLite, len(lites))
	for _, l := range lites {
		byKey[l.IssueKey] = l
	}
	// The changelog carries status ids only; the id -> category map is one
	// read for every key asked about, not one per doc.
	cats, err := db.StatusCategories(context.Background())
	if err != nil {
		return nil, nil, err
	}
	docs := make([]issueDoc, 0, len(keys))
	var notFound []string
	for _, key := range keys {
		d, err := db.Detail(context.Background(), key)
		if errors.Is(err, store.ErrNotFound) {
			notFound = append(notFound, key)
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		l, ok := byKey[key]
		if !ok {
			return nil, nil, fmt.Errorf("%s has a detail row but no issue row — the mirror is inconsistent, re-sync", key)
		}
		docs = append(docs, issueDoc{
			Issue:     l,
			Detail:    d,
			LinkedPRs: linkedPRsJSON(d),
			durations: store.Durations(store.DurationsInput{
				Created:    deref(l.CreatedAt, ""),
				Changelog:  d.History,
				Categories: cats,
				Now:        time.Now(),
			}),
		})
		// Personal history rides at the load point, not on cmdIssue's surface:
		// every caller that gets a doc (default, --json, --derive, multi-key)
		// records, and the notFound keys above never reach here. (--link and
		// --editmeta never call loadIssueDocs — no detail load, no visit.)
		recordVisitBestEffort(db, store.VisitKindIssue, key)
	}
	return docs, notFound, nil
}

func writeIssueJSON(docs []issueDoc, requested int) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if requested == 1 {
		return enc.Encode(docs[0])
	}
	return enc.Encode(jsonList(docs))
}

func printIssueDocs(docs []issueDoc) {
	for i, doc := range docs {
		if i > 0 {
			fmt.Printf("--- %s ---\n", doc.Issue.IssueKey)
		}
		printIssue(doc.Issue, doc.Detail, doc.durations)
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
	aliases := make([]string, 0, len(allow))
	for alias := range allow {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
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

func printIssue(l store.IssueLite, d *store.Detail, dur store.Spans) {
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
	// Restricted issues only: kv skips empty, so unrestricted rows stay
	// indistinguishable in the text form. Prefer the display name; fall
	// back to the id when the origin sent id without name.
	if name := deref(l.SecurityLevel, ""); name != "" {
		kv("security", name)
	} else {
		kv("security", deref(l.SecurityLevelID, ""))
	}
	kv("created", deref(l.CreatedAt, ""))
	kv("updated", deref(l.UpdatedAt, ""))
	kv("status since", deref(l.StatusChangedAt, ""))
	kv("resolved", deref(l.ResolvedAt, ""))
	// Computed from the changelog, never stored (data-model.md keeps
	// time-in-status absent); kv skips the whole line when neither span
	// exists — an issue that never entered progress has nothing to say.
	kv("durations", dur.Line())
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
			if mark := commentMark(c); mark != "" {
				if body != "" {
					body = mark + " " + body
				} else {
					body = mark
				}
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
	if prs := server.ListLinkedPRs(d.DevLinks, d.Attachments); len(prs) > 0 {
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
	raw := server.MergedPRLinks(d.DevLinks, d.Attachments)
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
	warnIfStale(db)

	var res store.SearchResult
	if *explain {
		res, err = db.SearchExplain(context.Background(), query, *limit)
	} else {
		res, err = db.Search(context.Background(), query, *limit)
	}
	if err != nil {
		return err
	}
	// res.Total is the count this command's own --json reports and the one
	// the web client posts (filters.svelte.ts passes res.total) — same input,
	// same row on both surfaces.
	recordSearchBestEffort(db, query, res.Total)
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
			"total": res.Total, "issues": jsonList(lites), "pages": pages, "matches": matches,
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
	printSearchText(lites, res.Pages, matches, res.Explain, *explain, res.ElapsedMS)
	return nil
}

// stdoutIsTerminal reports whether stdout is a character device. Search uses
// this so a pipe stays empty on 0 matches (AGENTS.md TSV contract) while a
// TTY gets "0 matches" on stderr (GDK-466).
func stdoutIsTerminal() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// searchIsTTY is the injection point so tests can exercise the TTY branch
// through a pipe (the same pattern as initIsTerminal).
var searchIsTTY = stdoutIsTerminal

const searchTSVHeader = "key\tstatus\tassignee\tsummary"

func printSearchText(lites []store.IssueLite, pages []store.PageLite, matches map[string]store.SearchMatch, explain []store.SearchExplain, withExplain bool, elapsedMS float64) {
	if len(lites) == 0 && len(pages) == 0 {
		if searchIsTTY() {
			fmt.Fprintln(os.Stderr, "0 matches")
		}
		return
	}
	if searchIsTTY() && len(lites) > 0 {
		fmt.Println(searchTSVHeader)
	}
	byExplain := indexSearchExplain(explain)
	for _, l := range lites {
		line := summaryLine(l)
		if m, ok := matches[l.IssueKey]; ok && (m.Field == "comment" || m.Field == "body") {
			line += fmt.Sprintf(" (%s: %s)", m.Field, m.Snippet)
		}
		if withExplain {
			line += formatSearchExplain(byExplain[l.IssueKey])
		}
		fmt.Println(line)
	}
	for _, p := range pages {
		line := fmt.Sprintf("page  %s/%s  %s", p.SpaceKey, p.Title, p.URL)
		if m, ok := matches[p.Key]; ok && (m.Field == "comment" || m.Field == "body") {
			line += fmt.Sprintf(" (%s: %s)", m.Field, m.Snippet)
		}
		if withExplain {
			line += formatSearchExplain(byExplain[p.Key])
		}
		fmt.Println(line)
	}
	if withExplain {
		fmt.Printf("query %.1fms\n", elapsedMS)
	}
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
	warnIfStale(db)

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
	// len(matched) is the total this path's own --json reports, post-cap —
	// record the same number. --emit returned above before any matching ran.
	recordSearchBestEffort(db, query, len(matched))
	warnJQL(parsed)

	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"total":       len(matched),
			"issues":      jsonList(matched),
			"pages":       []store.PageLite{},
			"jql":         parsed.JQL,
			"applied":     jsonList(parsed.Applied),
			"unsupported": jsonList(parsed.Unsupported),
		})
	}
	printSearchText(matched, nil, nil, nil, false, 0)
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
		ParentKey:      deref(l.ParentKey, ""),
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
		SprintID:       sprintIDString(l.SprintID),
		SprintState:    deref(l.SprintState, ""),
	}
}

func sprintIDString(id *int64) string {
	if id == nil {
		return ""
	}
	return fmt.Sprintf("%d", *id)
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
// Sentence owner is config.ErrNotConfigured (GDK-454); the addendum is this
// verb's, because a write that cannot reach an origin is not a local edit.
var errNoCredential = config.NotConfiguredWith("writes go to the origin, not to the mirror")

// writeNotMirroredError is the lookup miss after a write Jira already accepted.
// mutate returns it (non-zero). create prints the new key with this wording
// and exits 0 — the write happened.
type writeNotMirroredError struct{ Key string }

func (e writeNotMirroredError) Error() string {
	return fmt.Sprintf("write applied to %s, but it is not in the mirror — is it outside the configured projects?", e.Key)
}

// withCreateSession is create's write session: HasCredential (which counts
// a Linear apiKey) then WriterFor routed by --project / Linear-only.
// Mutate uses withKeyWriteSession — it already has a key.
func withCreateSession(project string, fn func(context.Context, *config.Config, *store.DB, origin.Writer, string) error) error {
	warnWorkspaceIfEnv()
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
	warnIfStale(db)
	ctx := context.Background()
	src, err := resolveCreateSource(ctx, cfg, db, project)
	if err != nil {
		return err
	}
	c, err := origin.WriterFor(cfg, src)
	if err != nil {
		return origin.FoldPairedError(cfg, err)
	}
	return origin.FoldPairedError(cfg, fn(ctx, cfg, db, c, src))
}

// resolveCreateSource picks the origin create files to. A project the
// mirror already knows as Linear routes there (same idea as KeySource).
// A Linear-only workspace (no Atlassian credential) always routes to
// Linear, even before the first team is mirrored.
func resolveCreateSource(ctx context.Context, cfg *config.Config, db *store.DB, project string) (string, error) {
	if proj := strings.TrimSpace(project); proj != "" && db != nil {
		src, err := db.ProjectSource(ctx, proj)
		if err != nil {
			return "", err
		}
		if src == "linear" {
			return "linear", nil
		}
	}
	if cfg.HasLinearCredential() && !cfg.HasAtlassianCredential() {
		return "linear", nil
	}
	return "", nil
}

// withKeyWriteSession is create's sibling routed per key: the mirror says
// which origin owns the row (store.KeySource — a "MID-5" can be Linear or
// Jira, the shape cannot tell), and the credential gate is that origin's.
func withKeyWriteSession(key string, fn func(context.Context, *config.Config, *store.DB, origin.Writer, string) error) error {
	warnWorkspaceIfEnv()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)
	ctx := context.Background()
	src, err := db.KeySource(ctx, key)
	if err != nil {
		if errors.Is(err, store.ErrKeyAmbiguous) {
			return err
		}
		src = ""
	}
	if src != "linear" && !cfg.HasAtlassianCredential() {
		return errNoCredential
	}
	c, err := origin.WriterFor(cfg, src)
	if err != nil {
		return origin.FoldPairedError(cfg, err)
	}
	return origin.FoldPairedError(cfg, fn(ctx, cfg, db, c, src))
}

// emitAfterWrite is the write-through tail: re-read the issue into the mirror
// and print the refreshed row. A failure between the write and the re-read is
// reported as such, because retrying it would repeat the write Jira already
// accepted.
func emitAfterWrite(ctx context.Context, cfg *config.Config, db *store.DB, src, key string, asJSON bool, extra map[string]any) error {
	if err := syncer.RefreshIssue(ctx, cfg, db, key, src); err != nil {
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

// mutate is the whole write-through shape: call the origin that owns the
// key, re-read the issue into the mirror, then print the refreshed row.
func mutate(key string, asJSON bool, fn func(context.Context, origin.Writer, string) (map[string]any, error)) error {
	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		extra, err := fn(ctx, c, src)
		if err != nil {
			return err
		}
		return emitAfterWrite(ctx, cfg, db, src, key, asJSON, extra)
	})
}

// maxMentionWords is the longest @-candidate we will ask the origin about.
// Three words covers "Dana Whitfield" plus one trailing word we may have to
// reject; unbounded combinatorics are not allowed.
const maxMentionWords = 3

// resolveCommentMentions turns typed `@Name` tokens into the map jira.Doc
// expects: key = the exact substring the user typed (no `@`), value = account
// id. The body is not rewritten — Jira renders the display name from the id.
//
// Hits: exactly one → mention node; two or more → refuse the write; zero →
// leave the token as plain text and name it for the caller to warn about.
func resolveCommentMentions(ctx context.Context, c origin.Writer, body string) (mentions map[string]string, unresolved []string, err error) {
	sites := mentionSites(body)
	if len(sites) == 0 {
		return nil, nil, nil
	}
	cache := make(map[string][]jira.User)
	mentions = make(map[string]string)
	seenUnresolved := map[string]bool{}
	for _, candidates := range sites {
		token, id, users, err := resolveMentionSite(ctx, c, cache, candidates)
		if err != nil {
			return nil, nil, err
		}
		if len(users) >= 2 {
			return nil, nil, ambiguousMention(token, users)
		}
		if id != "" {
			mentions[token] = id
			continue
		}
		if token != "" && !seenUnresolved[token] {
			seenUnresolved[token] = true
			unresolved = append(unresolved, token)
		}
	}
	return mentions, unresolved, nil
}

// resolveMentionSite walks the candidates shortest-first and stops at the
// first that names exactly one user.
//
// Shortest-first is the whole contract. Longest-first looks safer and is
// wrong against a real origin: Jira's user search is fuzzy, so a query of
// "김현철 GDK-510 멘션" still returns 김현철, the three-word candidate wins,
// and the mention node swallows the two words the author actually wrote
// (measured on a live site 2026-08-21, before this rule existed). Extending
// past a shorter name is only justified when that name failed to identify
// one person — which is what the web UI's autocomplete does too.
func resolveMentionSite(ctx context.Context, c origin.Writer, cache map[string][]jira.User, candidates []string) (token, id string, users []jira.User, err error) {
	var ambiguousToken string
	var ambiguousUsers []jira.User
	for _, cand := range candidates {
		hits, err := lookupMentionUsers(ctx, c, cache, cand)
		if err != nil {
			return "", "", nil, err
		}
		if len(hits) == 1 && hits[0].AccountID != "" {
			return cand, hits[0].AccountID, hits, nil
		}
		// Two hits mean this name is short of one person; a longer name may
		// still resolve. Keep the shortest ambiguity to report if none does.
		if len(hits) >= 2 && ambiguousToken == "" {
			ambiguousToken, ambiguousUsers = cand, hits
		}
	}
	if ambiguousToken != "" {
		return ambiguousToken, "", ambiguousUsers, nil
	}
	// Nothing matched: name the shortest candidate, which is the token the
	// author typed rather than the words that follow it.
	if len(candidates) > 0 {
		return candidates[0], "", nil, nil
	}
	return "", "", nil, nil
}

func lookupMentionUsers(ctx context.Context, c origin.Writer, cache map[string][]jira.User, q string) ([]jira.User, error) {
	if u, ok := cache[q]; ok {
		return u, nil
	}
	u, err := c.SearchUsers(ctx, q)
	if err != nil {
		return nil, err
	}
	if u == nil {
		u = []jira.User{}
	}
	cache[q] = u
	return u, nil
}

func ambiguousMention(token string, users []jira.User) error {
	names := make([]string, 0, len(users))
	for _, u := range users {
		name := u.DisplayName
		if name == "" {
			name = u.AccountID
		}
		names = append(names, name)
	}
	// Two or more hits is the opposite of "no user matching": the refusal has
	// to say the name is over-specified, not absent, or the next attempt is a
	// longer search for someone who was already found twice.
	return fmt.Errorf("@%s matches %d users on this origin (%s) — nothing was posted. Type enough of the name that exactly one matches",
		token, len(users), strings.Join(names, "; "))
}

func warnUnresolvedMentions(tokens []string) {
	if len(tokens) == 0 {
		return
	}
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = "@" + t
	}
	fmt.Fprintf(os.Stderr, "gadak: %s did not resolve to a user on this origin — left as plain text\n", strings.Join(parts, ", "))
}

// mentionSites returns one candidate list per `@` that starts a mention
// (string start or immediately after whitespace). Each list is shortest-first,
// at most maxMentionWords exact substrings of the body after `@` — see
// resolveMentionSite for why the order is not the other way round.
func mentionSites(body string) [][]string {
	var sites [][]string
	for i := 0; i < len(body); {
		r, size := utf8.DecodeRuneInString(body[i:])
		if r == '@' && mentionStartsAt(body, i) {
			if cands := mentionWordCandidates(body[i+size:]); len(cands) > 0 {
				sites = append(sites, cands)
			}
		}
		i += size
	}
	return sites
}

func mentionStartsAt(body string, i int) bool {
	if i == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(body[:i])
	return unicode.IsSpace(prev)
}

func mentionWordCandidates(rest string) []string {
	ends := wordEndOffsets(rest, maxMentionWords)
	if len(ends) == 0 {
		return nil
	}
	out := make([]string, 0, len(ends))
	for n := 1; n <= len(ends); n++ {
		tok := strings.TrimRight(rest[:ends[n-1]], ",.;:!?")
		if tok == "" {
			continue
		}
		out = append(out, tok)
	}
	return out
}

func wordEndOffsets(s string, n int) []int {
	var ends []int
	inWord := false
	last := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == '@' && !inWord {
			// A later `@Name` is its own site, not a word of this one.
			// Consuming it would spend the 3-query budget on "Dana @Kim"
			// and can make two clear mentions look ambiguous.
			return ends
		}
		if unicode.IsSpace(r) {
			if inWord {
				ends = append(ends, i)
				inWord = false
				if len(ends) == n {
					return ends
				}
			}
		} else {
			inWord = true
			last = i + size
		}
		i += size
	}
	if inWord && len(ends) < n {
		ends = append(ends, last)
	}
	return ends
}

func cmdComment(args []string) error {
	fs := newFlagSet("comment")
	text := fs.String("m", "", "comment body; `-` reads it from stdin")
	asJSON := fs.Bool("json", false, "emit JSON")
	internal := fs.Bool("internal", false, "post as a JSM internal comment")
	var visRaw labelFlags
	fs.Var(&visRaw, "visibility", "restrict to role=NAME or group=NAME (once)")
	batch := fs.String("batch", "", "JSON lines from stdin (`-` only); each object needs key and body")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("comment", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	vis, err := parseCommentVisibility(visRaw)
	if err != nil {
		return err
	}
	if *batch != "" {
		if err := rejectBatchFlag(*batch, *text); err != nil {
			return err
		}
		if len(pos) != 0 {
			return usageError("comment", "usage: gadak comment: --batch and a key/body are mutually exclusive")
		}
		return runCommentBatch(*asJSON, *internal, vis, *text)
	}
	if len(pos) == 0 {
		return usageError("comment", commentUsage)
	}
	key := normalizeKey(pos[0])
	body := *text
	// Trailing positional words are the body, like create's positional
	// SUMMARY (GDK-315). With -m too it is ambiguous — refuse.
	if len(pos) > 1 {
		if body != "" {
			return usageError("comment", "comment body given twice — positional text and -m; pick one")
		}
		body = strings.Join(pos[1:], " ")
	}
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("empty comment — pass -m <text>, or -m - to read stdin, or gadak comment KEY <text>")
	}
	return mutate(key, *asJSON, func(ctx context.Context, c origin.Writer, _ string) (map[string]any, error) {
		return postComment(ctx, c, key, body, vis, *internal)
	})
}

const commentUsage = "usage: gadak comment <KEY> [<text> | -m <text|->] [--visibility role=NAME|group=NAME] [--internal] [--json] | --batch -"

var commentBatchFields = []string{"key", "body", "internal", "visibility"}

func postComment(ctx context.Context, c origin.Writer, key, body string, vis *jira.CommentVisibility, internal bool) (map[string]any, error) {
	mentions, unresolved, err := resolveCommentMentions(ctx, c, body)
	if err != nil {
		return nil, err
	}
	warnUnresolvedMentions(unresolved)
	created, err := c.AddComment(ctx, key, jira.Doc(body, mentions), vis, internal)
	if err != nil {
		return nil, err
	}
	return map[string]any{"comment": map[string]any{
		"comment_id": created.ID,
		"author":     created.Author.DisplayName,
		"body":       jira.PlainText(created.Body),
	}}, nil
}

func runCommentBatch(asJSON, internalDefault bool, visDefault *jira.CommentVisibility, bodyDefault string) error {
	return runWriteBatch("comment", asJSON, func(raw string) batchResult {
		obj, key, err := parseBatchLine(raw, commentBatchFields)
		if err != nil {
			return batchErr(key, false, err)
		}
		body := bodyDefault
		if s, ok, err := jsonStringField(obj, "body"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			body = s
		}
		if strings.TrimSpace(body) == "" {
			return batchErr(key, false, errors.New("empty comment — JSON line needs \"body\""))
		}
		internal := internalDefault
		if v, ok, err := jsonBoolField(obj, "internal"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			internal = v
		}
		vis := visDefault
		if s, ok, err := jsonStringField(obj, "visibility"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			parsed, verr := parseCommentVisibility([]string{s})
			if verr != nil {
				return batchErr(key, false, verr)
			}
			vis = parsed
		}
		var wrote bool
		err = withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
			if _, err := postComment(ctx, c, key, body, vis, internal); err != nil {
				return err
			}
			wrote = true
			return syncer.RefreshIssue(ctx, cfg, db, key, src)
		})
		if err != nil {
			return batchErr(key, wrote, err)
		}
		return batchOK(key, true)
	})
}

// parseCommentVisibility accepts --visibility role=NAME or group=NAME once.
// A second occurrence or a value that is not that shape is a usage error
// (FlagSet ExitOnError would os.Exit on Set error, so validation is here).
func parseCommentVisibility(vals []string) (*jira.CommentVisibility, error) {
	if len(vals) == 0 {
		return nil, nil
	}
	if len(vals) > 1 {
		return nil, usageError("comment", "--visibility may be given only once")
	}
	typ, val, ok := strings.Cut(vals[0], "=")
	if !ok || (typ != "role" && typ != "group") || strings.TrimSpace(val) == "" {
		return nil, usageError("comment", "--visibility needs role=NAME or group=NAME")
	}
	return &jira.CommentVisibility{Type: typ, Value: val}, nil
}

func commentMark(c store.DetailComment) string {
	var parts []string
	if c.VisibilityType != "" {
		parts = append(parts, fmt.Sprintf("[restricted: %s %s]", c.VisibilityType, c.VisibilityValue))
	}
	if c.JsdPublic != nil && !*c.JsdPublic {
		parts = append(parts, "[internal]")
	}
	return strings.Join(parts, " ")
}

const transitionUsage = "usage: gadak transition <KEY> <transition-id|status-id|name|new|inprogress|done> [--resolution name|id] [--field key=JSON]... [-m text] [--json] | --batch - [--dry-run]"

const closeUsage = "usage: gadak close <KEY> [--resolution name|id] [--field key=JSON]... [-m text] [--json]"

func newTransitionFlags(name string) (*flag.FlagSet, *bool, *string, *labelFlags, *string) {
	fs := newFlagSet(name)
	asJSON := fs.Bool("json", false, "emit JSON")
	resolution := fs.String("resolution", "", "resolution name or id; a name is resolved from the transition's allowedValues, else GET /resolution")
	var fieldFlags labelFlags
	fs.Var(&fieldFlags, "field", "screen field key from `gadak transition KEY` (not a configured alias); key=JSON (repeatable); a value that is not JSON is sent as a string")
	text := fs.String("m", "", "comment posted with the transition; `-` reads it from stdin")
	return fs, asJSON, resolution, &fieldFlags, text
}

func cmdTransition(args []string) error {
	fs, asJSON, resolution, fieldFlags, text := newTransitionFlags("transition")
	batch := fs.String("batch", "", "JSON lines from stdin (`-` only); each object needs key and target")
	dryRun := fs.Bool("dry-run", false, "with --batch -: print the resolved transition id or no-op per line without writing")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("transition", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if *batch != "" {
		if err := rejectBatchFlag(*batch, *text); err != nil {
			return err
		}
		if len(pos) != 0 {
			return usageError("transition", "usage: gadak transition: --batch and a key/target are mutually exclusive")
		}
		parsed, err := parseTransitionFieldFlags(*fieldFlags)
		if err != nil {
			return err
		}
		return runTransitionBatch(*asJSON, *dryRun, *resolution, parsed, *text)
	}
	if *dryRun {
		return fmt.Errorf("--dry-run requires --batch -")
	}
	if len(pos) < 1 {
		return usageError("transition", transitionUsage)
	}
	key := normalizeKey(pos[0])
	if len(pos) < 2 {
		if strings.TrimSpace(*resolution) != "" || len(*fieldFlags) > 0 || *text != "" {
			return usageError("transition", transitionUsage)
		}
		return listTransitions(key, *asJSON)
	}
	// Trailing words join the target so an unquoted `In Review` still works.
	want := strings.TrimSpace(strings.Join(pos[1:], " "))
	body, err := readTransitionComment(*text)
	if err != nil {
		return err
	}
	parsed, err := parseTransitionFieldFlags(*fieldFlags)
	if err != nil {
		return err
	}
	return applyTransitionWrite(key, want, *resolution, parsed, body, *asJSON)
}

func cmdClose(args []string) error {
	fs, asJSON, resolution, fieldFlags, text := newTransitionFlags("close")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("close", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("close", closeUsage)
	}
	key := normalizeKey(pos[0])
	body, err := readTransitionComment(*text)
	if err != nil {
		return err
	}
	parsed, err := parseTransitionFieldFlags(*fieldFlags)
	if err != nil {
		return err
	}
	return applyTransitionWrite(key, "done", *resolution, parsed, body, *asJSON)
}

func readTransitionComment(text string) (string, error) {
	if text != "-" {
		return text, nil
	}
	buf, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	body := string(buf)
	if strings.TrimSpace(body) == "" {
		return "", errors.New("empty comment — pass -m <text>, or -m - to read stdin")
	}
	return body, nil
}

func applyTransitionWrite(key, want, resolution string, fields map[string]any, comment string, asJSON bool) error {
	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		res, err := applyTransition(ctx, c, cfg, key, want, resolution, fields, comment)
		if err != nil {
			return err
		}
		return emitTransitionResult(ctx, cfg, db, src, key, want, comment, asJSON, res)
	})
}

func applyTransition(ctx context.Context, c origin.Writer, cfg *config.Config, key, want, resolution string, fields map[string]any, comment string) (transition.Result, error) {
	res, err := transition.Apply(ctx, c, cfg, transition.Request{
		Key:        key,
		Target:     want,
		Resolution: resolution,
		Fields:     fields,
		Comment:    comment,
	})
	if err := formatTransitionError(err); err != nil {
		return transition.Result{}, err
	}
	return res, nil
}

var transitionBatchFields = []string{"key", "target", "resolution", "fields", "comment"}

func runTransitionBatch(asJSON, dryRun bool, resolutionDefault string, fieldsDefault map[string]any, commentDefault string) error {
	return runWriteBatch("transition", asJSON, func(raw string) batchResult {
		obj, key, err := parseBatchLine(raw, transitionBatchFields)
		if err != nil {
			return batchErr(key, false, err)
		}
		want, ok, err := jsonStringField(obj, "target")
		if err != nil {
			return batchErr(key, false, err)
		}
		if !ok || strings.TrimSpace(want) == "" {
			return batchErr(key, false, errors.New("JSON line needs \"target\""))
		}
		want = strings.TrimSpace(want)
		resolution := resolutionDefault
		if s, ok, err := jsonStringField(obj, "resolution"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			resolution = s
		}
		fields := fieldsDefault
		if v, ok, err := jsonAnyObjectField(obj, "fields"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			fields = v
		}
		comment := commentDefault
		if s, ok, err := jsonStringField(obj, "comment"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			comment = s
		}
		if dryRun {
			var id string
			var changed bool
			err = withKeyWriteSession(key, func(ctx context.Context, _ *config.Config, _ *store.DB, c origin.Writer, _ string) error {
				var perr error
				id, changed, perr = transition.Preview(ctx, c, key, want)
				return perr
			})
			if err != nil {
				return batchErr(key, false, err)
			}
			return batchDryRun(key, id, changed)
		}
		var changed bool
		err = withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
			res, err := applyTransition(ctx, c, cfg, key, want, resolution, fields, comment)
			if err != nil {
				return err
			}
			changed = res.Changed
			return syncer.RefreshIssue(ctx, cfg, db, key, src)
		})
		if err != nil {
			return batchErr(key, changed, err)
		}
		return batchOK(key, changed)
	})
}

func emitTransitionResult(ctx context.Context, cfg *config.Config, db *store.DB, src, key, want, comment string, asJSON bool, res transition.Result) error {
	extra := map[string]any{"changed": res.Changed}
	if res.Changed {
		return emitAfterWrite(ctx, cfg, db, src, key, asJSON, extra)
	}
	token, ok := jira.StatusCategoryToken(want)
	if !ok {
		token = want
	}
	if asJSON {
		if err := syncer.RefreshIssue(ctx, cfg, db, key, src); err != nil {
			return fmt.Errorf("already %s — nothing to do, but the mirror did not refresh (run `gadak sync`): %w", token, err)
		}
		lites, err := lookup(db, []string{key})
		if err != nil {
			return err
		}
		if len(lites) == 0 {
			return writeNotMirroredError{Key: key}
		}
		body := map[string]any{"issue": lites[0], "changed": false}
		return json.NewEncoder(os.Stdout).Encode(body)
	}
	fmt.Fprintf(os.Stdout, "already %s — nothing to do\n", token)
	if strings.TrimSpace(comment) != "" {
		fmt.Fprintln(os.Stdout, "comment not posted")
	}
	return nil
}

// formatTransitionError adds CLI flag names to core refusals that do not
// name them (the core is shared with REST).
func formatTransitionError(err error) error {
	if err == nil {
		return nil
	}
	var req *transition.RequiredFieldsError
	if errors.As(err, &req) {
		return fmt.Errorf("%w — pass --resolution NAME or --field resolution={\"id\":...}", req)
	}
	return err
}

// listTransitions is `gadak transition KEY` with no target: print what
// pickTransition would accept, instead of a usage dump (GDK-466).
func listTransitions(key string, asJSON bool) error {
	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		list, err := c.Transitions(ctx, key)
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"key":         key,
				"transitions": jsonList(list),
				"categories":  jsonList(jira.ReachableCategories(list)),
			})
		}
		if len(list) == 0 {
			fmt.Fprintf(os.Stdout, "%s has no available transitions for this credential\n", key)
			return nil
		}
		fmt.Printf("available: %s\n", jira.JoinTransitions(list))
		if cats := jira.ReachableCategories(list); len(cats) > 0 {
			fmt.Printf("also accepts a status category: %s\n", strings.Join(cats, ", "))
		}
		return nil
	})
}

func parseTransitionFieldFlags(raw []string) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(raw))
	for _, item := range raw {
		key, val, ok := strings.Cut(item, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("--field expects key=JSON, got %q", item)
		}
		var parsed any
		if err := json.Unmarshal([]byte(val), &parsed); err != nil {
			out[key] = val
		} else {
			out[key] = parsed
		}
	}
	return out, nil
}

const assignUsage = "usage: gadak assign <KEY> <email|name|accountId|-> [--json] | --batch -"

var assignBatchFields = []string{"key", "assignee"}

func cmdAssign(args []string) error {
	fs := newFlagSet("assign")
	asJSON := fs.Bool("json", false, "emit JSON")
	batch := fs.String("batch", "", "JSON lines from stdin (`-` only); each object needs key and assignee")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("assign", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if *batch != "" {
		if err := rejectBatchFlag(*batch, ""); err != nil {
			return err
		}
		if len(pos) != 0 {
			return usageError("assign", "usage: gadak assign: --batch and a key/assignee are mutually exclusive")
		}
		return runAssignBatch(*asJSON)
	}
	if len(pos) < 2 {
		return usageError("assign", assignUsage)
	}
	key, who := normalizeKey(pos[0]), strings.TrimSpace(strings.Join(pos[1:], " "))

	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		if err := assignTo(ctx, c, src, key, who); err != nil {
			return err
		}
		return emitAfterWrite(ctx, cfg, db, src, key, *asJSON, nil)
	})
}

func assignTo(ctx context.Context, c origin.Writer, src, key, who string) error {
	id, err := resolveAccount(ctx, c, who, src)
	if err != nil {
		return err
	}
	return c.SetAssignee(ctx, key, id)
}

func runAssignBatch(asJSON bool) error {
	return runWriteBatch("assign", asJSON, func(raw string) batchResult {
		obj, key, err := parseBatchLine(raw, assignBatchFields)
		if err != nil {
			return batchErr(key, false, err)
		}
		who, ok, err := jsonStringField(obj, "assignee")
		if err != nil {
			return batchErr(key, false, err)
		}
		if !ok {
			return batchErr(key, false, errors.New("JSON line needs \"assignee\" (`\"-\"` unassigns)"))
		}
		who = strings.TrimSpace(who)
		if who == "" {
			return batchErr(key, false, errors.New("JSON line needs \"assignee\" (`\"-\"` unassigns)"))
		}
		var wrote bool
		err = withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
			if err := assignTo(ctx, c, src, key, who); err != nil {
				return err
			}
			wrote = true
			return syncer.RefreshIssue(ctx, cfg, db, key, src)
		})
		if err != nil {
			return batchErr(key, wrote, err)
		}
		return batchOK(key, true)
	})
}

// exitClaimConflict is the refusal exit: another actor holds the issue in
// progress. Its own code, not 1, so an agent branches without parsing
// stderr — the holder's name is still on stderr for the human. 75 is
// EX_TEMPFAIL: refused for now, retry after the holder finishes or pass
// --take-over.
const exitClaimConflict = 75

// cmdClaim is `gadak claim KEY`: take an issue as yours — assignee plus the
// in-progress transition in one step (internal/claim). On standalone and
// paired origins that is one atomic call; on connected Cloud there is no
// such route, the two writes run as a fallback, and the caller is told so.
// A claim someone else holds is refused with exit 75 rather than silently
// replacing them.
func cmdClaim(args []string) error {
	fs := newFlagSet("claim")
	asJSON := fs.Bool("json", false, "emit JSON")
	takeOver := fs.Bool("take-over", false, "claim even when another assignee holds the issue in progress (replaces them)")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("claim", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("claim", "usage: gadak claim <KEY> [--take-over] [--json]")
	}
	key := normalizeKey(pos[0])

	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		o, ok := c.(claim.Origin)
		if !ok {
			return fmt.Errorf("claim has no counterpart on this issue's origin (%s) — it is a Jira-workflow verb: assignee plus the in-progress transition; `gadak transition` and `gadak assign` are the two halves", src)
		}
		res, err := claim.Apply(ctx, o, cfg, claim.Request{Key: key, TakeOver: *takeOver})
		if err != nil {
			var taken *claim.TakenError
			if errors.As(err, &taken) {
				return &exitCodeError{code: exitClaimConflict, msg: taken.Error()}
			}
			return err
		}
		if !res.Atomic {
			fmt.Fprintf(os.Stderr, "warning: this origin has no atomic claim — assignee and in-progress transition were two calls, so a concurrent claim could interleave\n")
		}
		return emitAfterWrite(ctx, cfg, db, src, key, *asJSON, map[string]any{"claim": res})
	})
}

// resolveAccount turns an email, display name, or account id into an origin
// account id: `-` unassigns. Jira rows may use the configured member
// directory (JiraAccountID) without a network call — email first, then exact
// account id. Linear rows must not — that id is a Jira account, and Linear
// assign wants a Linear user UUID from Writer.SearchUsers. Account id
// comparison is case-sensitive; email is not.
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
		for _, m := range cfg.Members {
			if m.JiraAccountID != "" && m.JiraAccountID == who {
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
	for _, u := range users {
		if u.AccountID == who {
			return u.AccountID, nil
		}
	}
	// A site that hides emails answers with no email to match on, so a single hit
	// is taken at its word and an ambiguous one is refused rather than guessed.
	if len(users) == 1 {
		return users[0].AccountID, nil
	}
	if len(users) == 0 {
		// Linear SearchUsers is name/email contains; a UUID from the
		// mirror (issues.assignee_id) used to miss, and this hint told
		// the user to do the thing that just failed. Accept
		// the UUID as the id so the hint matches the behavior.
		if source == "linear" && linear.LooksLikeID(who) {
			return who, nil
		}
		return "", fmt.Errorf("no user on this issue's origin matches %q — look up issues.assignee_id in the mirror or `gadak issue KEY --editmeta`", who)
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
	// Unconfigured (no origin at all) is not the standalone no-site case.
	// Standalone HasCredential is true, so it still hits the lookup / live-
	// serve / views-open path below and is not told to re-run init (GDK-454).
	if !cfg.HasCredential() {
		return config.NotConfiguredWith(fmt.Sprintf("use `gadak views open %s` (or `gadak serve`)", key))
	}
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

// printDevLinkKinds renders the dev_links kinds that are not pull requests
// — deployments and builds (GDK-592) — each under its own kind-labelled
// section so they are distinguishable from the PR list above: a deployment
// line is environment→state, a build line #number→state, with the run URL
// last when the row has one.
func printDevLinkKinds(links []store.DevLink) {
	var deps, builds []store.DevLink
	for _, l := range links {
		switch l.Kind {
		case "deployment":
			deps = append(deps, l)
		case "build":
			builds = append(builds, l)
		}
	}
	if len(deps) > 0 {
		fmt.Printf("\ndeployments (%d)\n", len(deps))
		for _, l := range deps {
			line := l.Environment + "\t" + l.Status
			if l.URL != "" {
				line += "\t" + l.URL
			}
			fmt.Printf("  %s\n", line)
		}
	}
	if len(builds) > 0 {
		fmt.Printf("\nbuilds (%d)\n", len(builds))
		for _, l := range builds {
			id := l.ExternalID
			if id != "" {
				id = "#" + id
			}
			line := id + "\t" + l.Status
			if l.URL != "" && id != "" {
				line += "\t" + l.URL
			} else if id == "" {
				line = l.URL + "\t" + l.Status
			}
			fmt.Printf("  %s\n", line)
		}
	}
}
