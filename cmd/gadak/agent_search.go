package main

// gadak search — the agent's mirror-side query verb: text or JQL, TSV or
// JSON, with the explain lines that say why each row matched. Reads come
// from the mirror and never call Jira (specs/000-product/contracts/
// agent.md). Split from agent.go (GDK-1771).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
)

func cmdSearch(args []string) error {
	// Flags may sit before or after the query. `gadak search "flaky" --limit 5`
	// and `gadak search --jql 'project = NMA' --json` both have to work;
	// FlagSet alone swallows a trailing --json into the JQL.
	fs := newFlagSet("search")
	limit := fs.Int("limit", 20, "maximum matches")
	asJSON := fs.Bool("json", false, "emit matching issues and pages as JSON")
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
	if res.Total == 0 && len(res.Pages) == 0 {
		// GDK-1612: "0 matches" for a key this workspace does not mirror is a
		// scope fact, not a search miss — same verdict `gadak sql` prints, on
		// stderr so the TSV stdout contract stays untouched. Prints on a pipe
		// too: the reader with no TTY is exactly the agent who pasted the key.
		if hint := zeroRowScopeHint(db, "0 matches", query); hint != "" {
			fmt.Fprintln(os.Stderr, hint)
		}
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
// this so a pipe stays empty on 0 matches (docs/MIRROR.md TSV contract) while a
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
	// Page rows keep the TSV contract the issue rows set (docs/MIRROR.md:
	// cut -f1 gives keys): field 1 is PageLite.Key — items.key, the origin
	// page id `gadak page edit <ID>` takes (pages.item_id is a different,
	// internal id) — and field 2 is the literal kind marker so a pipe can
	// tell issues from pages without guessing from the key's shape.
	for _, p := range pages {
		line := fmt.Sprintf("%s\tpage\t%s/%s\t%s", p.Key, p.SpaceKey, p.Title, p.URL)
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

	// People first, from the narrow projection: the emit-only path (and a
	// JQL that resolves to nothing) never needs the full row set this way
	// (GDK-748).
	peopleRows, err := db.QueryActorPeople(context.Background())
	if err != nil {
		return err
	}
	jql.ResolvePeople(&parsed, store.ActorPeople(peopleRows), opts.Email)

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

	// Matching does need the full rows, but only after the cheap exits above
	// have passed.
	lites, err := db.IssueLites(context.Background())
	if err != nil {
		return err
	}
	matched := make([]store.IssueLite, 0)
	for _, l := range lites {
		if jql.Match(store.LiteToIssue(l), parsed.Filters) {
			matched = append(matched, l)
		}
	}
	store.SortDisplay(matched, parsed.Display)
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	// len(matched) is the total this path's own --json reports, post-cap —
	// record the same number. --emit returned above before any matching ran.
	recordSearchBestEffort(db, query, len(matched))
	warnJQL(parsed)
	if len(matched) == 0 {
		// GDK-1612: the text-search path's scope verdict, same rule here — a
		// JQL naming `key = D1-8228` on a workspace that never mirrors D1 gets
		// the one stderr line naming what was read and what it holds.
		if hint := zeroRowScopeHint(db, "0 matches", query); hint != "" {
			fmt.Fprintln(os.Stderr, hint)
		}
	}

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

/* ── writes ── */

// errNoCredential is the refusal mutate and create share: writes go to Jira.
// Sentence owner is config.ErrNotConfigured (GDK-454); the addendum is this
// verb's, because a write that cannot reach an origin is not a local edit.
