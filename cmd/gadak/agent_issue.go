package main

// The agent-facing read verbs: `gadak issue` answers from the mirror and
// never calls Jira, `gadak open` resolves a key to its origin URL and
// opens it, and the staleness gates at the top warn (never refuse) when
// the mirror is old (specs/000-product/contracts/agent.md). Part of the
// agent command set split from agent.go (GDK-1771): the human rendering
// of an issue answer is agent_print.go, search is agent_search.go, the
// write verbs are agent_write.go, comment @mention resolution is
// agent_mention.go.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/attachaudit"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/skillinstall"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// staleAfter is when a mirror stops being worth trusting silently. It is a
// warning, never a refusal: an old answer with a warning beats no answer.
const staleAfter = time.Hour

// skillDiffersWarned is the process-once latch for warnSkillDiffers: the
// read verbs all share warnIfStale as their preamble, and one process (a
// long-lived test, a future batch verb) must not repeat the line per read.
var skillDiffersWarned bool

// warnIfStale prints one stderr line when the last sync failed or is old, so a
// caller reading stdout knows how far behind the answer may be. stdout stays
// clean, which is what makes the output pipeable. It reads the caller's
// already-open connection — every caller has one, and a second open here
// doubled any diagnostic the open path prints (GDK-314).
//
// This function is the one funnel every read verb passes through, so it is
// also where the skill-differs trip speaks (warnSkillDiffers, GDK-493):
// an agent that never decides to call doctor still learns its loaded skill
// copy is not this build's.
//
// A live first sync explains the mirror's state better than any staleness
// verdict can (every source row is empty or half-written by design), so
// warnFirstSync goes first and the rest stands down while it speaks (GDK-1677).
// warnSkillDiffers is the bootstrap-gap trip (GDK-493): staleness detection
// used to live inside the skill's own body, so the generations that needed
// the sentence most — the stale ones — were exactly the ones without it. The
// binary is always current about itself, so it owns the check: when an
// installed skill copy exists and differs from this build's embedded text,
// one stderr line in the same place the freshness warning speaks, which a
// read verb reaches without ever deciding to call doctor. The threshold is
// "differs", never "stale": the binary cannot know which side is newer, and
// a hand-edited copy differs from this build as surely as an old one —
// `gadak skill install` (with --force for a hand edit) is the way out either
// way. A machine with no installed copy stays silent: nothing to compare
// against means nothing to say. Read errors are silence too — doctor owns
// the detailed verdict; this line only needs to exist.
func warnSkillDiffers() {
	if skillDiffersWarned {
		return
	}
	skillDiffersWarned = true
	env := skillinstall.OSEnv()
	content := gadak.SkillMarkdown()
	// User scope first, then the project scope some hosts have — table
	// order, so the first differing copy named is deterministic.
	for _, client := range skillinstall.Clients() {
		dests := make([]string, 0, 2)
		if dest, err := client.HomeDest(env); err == nil {
			dests = append(dests, dest)
		}
		if client.HasProjectScope() {
			if dest, err := client.ProjectDest(env); err == nil {
				dests = append(dests, dest)
			}
		}
		for _, dest := range dests {
			status, _, err := skillinstall.DestStatus(dest, content)
			if err != nil || status == skillinstall.StatusMissing || status == skillinstall.StatusIdentical {
				continue
			}
			fmt.Fprintf(os.Stderr, "warning: skill file differs from this build — run `gadak skill install` (%s)\n", tildeHome(dest))
			return
		}
	}
}

func warnIfStale(db interface {
	QueryRow(query string, args ...any) *sql.Row
}) {
	warnSkillDiffers()
	if warnFirstSync(db) {
		return
	}
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
			warn("last sync failed (%s): %s", formatSourceID(r.id), *r.lastError)
			return
		}
	}
	var oldest *time.Time
	var oldestID, oldestRaw string
	for _, r := range rows {
		if r.syncedAt == nil || *r.syncedAt == "" {
			continue
		}
		t, ok := config.ParseTimestamp(*r.syncedAt)
		if !ok {
			continue
		}
		if oldest == nil || t.Before(*oldest) {
			tt := t
			oldest = &tt
			oldestID = r.id
			oldestRaw = *r.syncedAt
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
		// GDK-810: name the source and echo its stored synced_at. "mirror
		// last synced" made a stale confluence row look like the whole
		// mirror (and its watermark) was that old.
		warn("%s", staleSourceWarning(oldestID, oldestRaw, time.Since(*oldest)))
	}
}

// warnFirstSync prints the one "first sync in progress" line when the mirror
// itself says a first full sync is running (GDK-1677), and reports whether it
// did. It reads plain SQL through the caller's handle — the same interface
// warnIfStale takes — so every read verb (sql, list, search, page, fields,
// memory, the agent verbs) gets the line with no per-verb edits, whatever
// kind of connection the verb opened. The liveness cutoff is
// store.SyncProgressCutoff, the single owner, so this and the store reader
// cannot disagree about what "live" means. A read error (including a mirror
// too old to have the table) just means no warning.
func warnFirstSync(db interface {
	QueryRow(query string, args ...any) *sql.Row
}) bool {
	var sourceID string
	var fetched int
	var total sql.NullInt64
	err := db.QueryRow(`SELECT source_id, fetched, total FROM sync_progress
		WHERE first = 1 AND updated_at >= ?
		ORDER BY started_at DESC LIMIT 1`, store.SyncProgressCutoff(time.Now())).
		Scan(&sourceID, &fetched, &total)
	if err != nil {
		return false
	}
	wikiNext := false
	if sourceID != syncer.ConfluenceSourceID {
		if cfg, cfgErr := config.Load(); cfgErr == nil && cfg.Confluence != nil {
			var n int
			if qErr := db.QueryRow(`SELECT COUNT(*) FROM sync_progress
				WHERE source_id = ? AND updated_at >= ?`,
				syncer.ConfluenceSourceID, store.SyncProgressCutoff(time.Now())).Scan(&n); qErr == nil && n == 0 {
				wikiNext = true
			}
		}
	}
	count := formatIntComma(fetched)
	if total.Valid {
		count += " / " + formatIntComma(int(total.Int64))
	}
	var line string
	if sourceID == syncer.ConfluenceSourceID {
		line = fmt.Sprintf("issues done, %s wiki pages so far — results are partial", count)
	} else {
		line = fmt.Sprintf("%s issues so far — results are partial", count)
		if wikiNext {
			line += "; wiki follows"
		}
	}
	fmt.Fprintf(os.Stderr, "warning: first sync in progress: %s\n", line)
	return true
}

// parseSyncedAt is gone: its RFC3339-then-ISOMilli ladder was one of the
// private timestamp tables folded into config.ParseTimestamp (GDK-1130),
// and warnIfStale calls the owner directly. Unparseable values are skipped
// by the caller so a corrupt row cannot crash a read, and cannot take the
// never-synced branch while a sibling parsed.

// sourceIDDisplayCols is the stderr budget for a sync_state.source_id.
// jira / linear / confluence fit; a planted multi-kilobyte or control-laden
// id must not wrap the one-line warning (GDK-810).
const sourceIDDisplayCols = 32

// formatSourceID makes a source_id safe to interpolate into one stderr line.
// Control runes become a space; clip (already this file's width authority)
// collapses remaining whitespace and truncates.
func formatSourceID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if unicode.IsControl(r) {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	s := clip(b.String(), sourceIDDisplayCols)
	if s == "" {
		return "?"
	}
	return s
}

// staleSourceWarning is the one-line age warning. Source, stored synced_at,
// and age are all on the line so `gadak status` text (`<id>.synced_at`) and
// this warning can be compared by the same string. GDK-598's
// `sync --if-stale 1h` teaching stays.
func staleSourceWarning(id, syncedAt string, age time.Duration) string {
	return formatSourceID(id) + " last synced " + age.Round(time.Minute).String() +
		" ago (synced_at " + syncedAt + ") — run `gadak sync --if-stale 1h`"
}

// normalizeKey accepts a key in any case; Jira's are uppercase.
func normalizeKey(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// lookup returns the IssueLite rows for the given keys, in the order asked, and
// skips keys the mirror does not have. A keyed read: search/issue pay this per
// invocation, and filtering the full IssueLites() list here made every CLI read
// cost a whole-mirror scan (~240 ms fixed at 7k-20k issues, measured
// 2026-08-23 — the benchmark CLI rows had been inflated by it). The keyed
// store read preserves the caller's key order, which is Search's rank order.
func lookup(db *store.DB, keys []string) ([]store.IssueLite, error) {
	return db.IssueLitesByKeys(context.Background(), keys)
}

// summaryLine is the one-line rendering shared by search results and the
// confirmation a write prints. Tab-separated for the same reason `sql` is:
// `cut -f1` has to work.
func summaryLine(l store.IssueLite) string {
	return strings.Join([]string{l.IssueKey, l.Status, derefOrEmpty(l.Assignee, "(unassigned)"), l.Summary}, "\t")
}

// derefOrEmpty derefs and folds: nil AND a stored empty string both take
// the fallback. That fold is what "(unassigned)" and the deriveNull
// markers below rely on — a stored "" is a value the display replaces.
// The name says so since GDK-1579: an earlier same-named `deref` sibling
// in internal/dashboards kept "" (nil-only), and two helpers with one
// name and different truth tables hid which side a binding rode.
func derefOrEmpty(s *string, fallback string) string {
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
		// GDK-1612: a key from a project this workspace never mirrors is a
		// scope fact, and "run `gadak sync`" is the wrong advice for it — sync
		// pulls the configured scope, never a foreign project.
		if hint := zeroRowScopeHint(db, "", keys[0]); hint != "" {
			return errors.New(hint)
		}
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
		printIssueDocs(docs, linkTypePhrases(db))
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
	if _, err := db.RecordVisit(context.Background(), kind, key, store.VisitSourceCLI); err != nil {
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
				Created:    derefOrEmpty(l.CreatedAt, ""),
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

// attachmentTruncationMark tags an attachment whose size is the old built-in
// upload cap exactly (GDK-1615). The wording and the predicate belong to
// internal/attachaudit so doctor and this line cannot disagree. Only a
// built-in-origin workspace ever went through that cap — the same origin type
// the attachment proxy keys on — and config is read only when the size
// already matches.
func attachmentTruncationMark(size int64) string {
	if !attachaudit.Suspect(size) {
		return ""
	}
	cfg, err := config.Load()
	if err != nil || cfg.OriginType() != config.OriginGadak {
		return ""
	}
	return "\t" + attachaudit.Mark
}

// linkTypePhrases reads the mirror's link_types catalog (schemaV43; sync
// fills it) for the human link line's phrase source (GDK-1734). The SQL is
// the store's (DB.LinkTypePhrases, GDK-1215) — the same read the detail
// response's phrase field goes through, so the CLI line and the web panel
// cannot drift. A nil map — unreadable or empty catalog, like the demo
// fixture — keeps the wire pair wording, and the JSON contract is untouched
// either way.
func linkTypePhrases(db *store.DB) map[string][2]string {
	if db == nil {
		return nil
	}
	return db.LinkTypePhrases()
}

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
	// Unconfigured (no origin at all) is not the built-in no-site case.
	// Built-in HasCredential is true, so it still hits the lookup / live-
	// serve / views-open path below and is not told to re-run init (GDK-454).
	if !cfg.HasCredential() {
		return config.NotConfiguredWith(fmt.Sprintf("use `gadak views open %s` (or `gadak serve`)", key))
	}
	// Branch on the origin's type and never cross over (GDK-1308 — "Jira is
	// Jira, Linear is Linear", no fallbacks): a Linear row without a stored
	// url must not get a Jira URL built from a site this workspace does not
	// have.
	switch cfg.OriginType() {
	case config.OriginLinear:
		// The page Linear itself minted, as sync stored it (items.url). No
		// row or no url: the mirror lags — say so, do not invent a page.
		found, stored, err := lookupItem(key)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%s is not in the mirror — check the key, or run `gadak sync`", key)
		}
		u := absoluteHTTPURL(stored)
		if u == "" {
			return fmt.Errorf("%s has no Linear page stored yet — run `gadak sync`", key)
		}
		return openIssueURL(u)
	case config.OriginJira, config.OriginJiraServer:
		// `open` is the escape hatch to Jira: the mirror may lag a key that
		// exists on the site, so a missing key still opens the browse URL.
		// Cloud and Server agree on /browse/KEY — one of the few places
		// they do, so this branch is shared on purpose.
		if cfg.Site == "" {
			return fmt.Errorf("this workspace has no Jira site to browse — use `gadak views open %s` (or `gadak serve`)", key)
		}
		return openIssueURL(strings.TrimRight(cfg.Site, "/") + "/browse/" + url.PathEscape(key))
	}
	// The built-in tracker: its page is this app. A missing row used to
	// fall through to an error telling an already-inited workspace to
	// re-run init (GDK-454); it is a key problem, not a setup one.
	found, _, err := lookupItem(key)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%s is not in the mirror — check the key, or run `gadak sync`", key)
	}
	if web := serveFocusURL("issue=" + key); web != "" {
		return openIssueURL(web)
	}
	return fmt.Errorf("this workspace has no origin site to browse — use `gadak views open %s` (or `gadak serve`)", key)
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

// absoluteHTTPURL accepts only http(s) URLs with a host. Built-in origin
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
