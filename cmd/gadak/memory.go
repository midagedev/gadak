package main

// The agent-memory verbs: `memory add` leaves a note where the next session
// can find it, `memory search` scopes the mirror's full-text search to the
// memory space. A memory is a page — created through the same origin path
// page create uses (createPageViaOrigin), mirrored by the same refresh — so
// there is no second write path and nothing agent-specific in the store.
// The space is config-owned (memory.space): standalone defaults to the
// seeded personal space, connected refuses until set, because quietly
// writing agent notes into a team-visible space is not the CLI's call.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// memoryTitleCols caps a derived title: a note's first line is a summary
// candidate, not always a short one. Same display-columns idea as clip's
// other callers.
const memoryTitleCols = 80

// memorySearchFetchLimit is the db.Search window before the Go-side space
// filter. It must be wider than --limit so the memory space's hits can
// outrank other pages' without being crowded out of the window; a memory
// page ranked below it by a better-matching team page is the known edge,
// reported rather than hidden.
const memorySearchFetchLimit = 200

// errNoMemorySpace is the connected-without-memory.space refusal: the
// workspace has an origin, the verb just refuses to guess which of its
// spaces agent memory belongs in.
var errNoMemorySpace = fmt.Errorf("memory: memory.space is not set — a connected workspace will not quietly write notes to a team space; pick one with `gadak config set memory.space KEY`")

// memorySpace resolves where agent memory lives. An explicit memory.space
// wins on both workspace kinds. Standalone falls back to its seeded
// personal space (the origin owns that key; config cannot import it without
// a cycle). Connected has no default — the refusal is the point.
func memorySpace(cfg *config.Config) (string, error) {
	if s := cfg.MemorySpace(); s != "" {
		return s, nil
	}
	if cfg.IsStandalone() {
		return origin.DefaultSpaceKey, nil
	}
	return "", errNoMemorySpace
}

func cmdMemory(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("memory", nil))
		return nil
	}
	switch args[0] {
	case "add":
		return cmdMemoryAdd(args[1:])
	case "search":
		return cmdMemorySearch(args[1:])
	default:
		return fmt.Errorf("memory: unknown subcommand %q (try `gadak memory add|search`)", args[0])
	}
}

// cmdMemoryAdd writes one page into the memory space. The note is either a
// positional or -m (stdin with `-`), never both. The success line names
// what was written — id, title, space (GDK-1019's shape) — so a session can
// cite the page it just left.
func cmdMemoryAdd(args []string) error {
	fs := newFlagSet("memory add")
	title := fs.String("title", "", "page title (omitted = derived from the note's first line)")
	text := fs.String("m", "", "note as plain text; `-` reads stdin")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("memory", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 && strings.TrimSpace(*text) != "" {
		return fmt.Errorf("memory add: the note is given twice — pick one: pass it as an argument or via -m, not both")
	}
	body := *text
	if body == "" && len(pos) > 0 {
		body = strings.Join(pos, " ")
	}
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("memory add: nothing to add — pass the note text (or -m <text|->)")
	}
	warnWorkspaceIfEnv()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	space, err := memorySpace(cfg)
	if err != nil {
		return err
	}
	pageTitle := strings.TrimSpace(*title)
	if pageTitle == "" {
		pageTitle = deriveMemoryTitle(body)
	}

	created, _, mirrorStale, err := createPageViaOrigin(space, pageTitle, string(jira.Doc(body, nil)), "")
	if err != nil {
		return err
	}
	outSpace := created.Space.Key
	if outSpace == "" {
		outSpace = space
	}
	if *asJSON {
		body := map[string]any{"id": created.ID, "title": created.Title, "space": outSpace}
		if mirrorStale {
			body["mirror_stale"] = true
		}
		return json.NewEncoder(os.Stdout).Encode(body)
	}
	fmt.Printf("%s\t%s\t%s\n", created.ID, created.Title, outSpace)
	return nil
}

// deriveMemoryTitle takes the note's first line, whitespace-collapsed and
// clipped to memoryTitleCols, as the page title. Never empty — the caller
// already refused an all-whitespace note, and a first line is then at
// least one non-space rune.
func deriveMemoryTitle(body string) string {
	line := strings.TrimSpace(body)
	if cut := strings.IndexByte(line, '\n'); cut >= 0 {
		line = strings.TrimSpace(line[:cut])
	}
	return clip(line, memoryTitleCols)
}

// cmdMemorySearch is a mirror read: the same full-text search `gadak
// search` runs, scoped to the memory space. 0 hits is an answer — one line
// that says so and names the verb that fills the space (GDK-1020), never
// silence.
func cmdMemorySearch(args []string) error {
	fs := newFlagSet("memory search")
	limit := fs.Int("limit", 20, "maximum matches")
	noHeader := fs.Bool("no-header", false, "omit the TSV header row")
	asJSON := fs.Bool("json", false, "emit one JSON object per match")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("memory", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(pos, " "))
	if query == "" {
		return fmt.Errorf("memory search: a query is required (usage: gadak memory search <text> [--limit N] [--json])")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	space, err := memorySpace(cfg)
	if err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)

	res, err := db.Search(context.Background(), query, memorySearchFetchLimit)
	if err != nil {
		return err
	}
	matches := res.Matches
	if matches == nil {
		matches = map[string]store.SearchMatch{}
	}
	type row struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Space     string `json:"space"`
		Snippet   string `json:"snippet"`
		UpdatedAt string `json:"updated_at"`
	}
	var rows []row
	for _, p := range res.Pages {
		if len(rows) >= *limit {
			break
		}
		if !strings.EqualFold(p.SpaceKey, space) {
			continue
		}
		snippet := p.Excerpt
		if m, ok := matches[p.Key]; ok && m.Snippet != "" {
			snippet = m.Snippet
		}
		rows = append(rows, row{ID: p.Key, Title: p.Title, Space: p.SpaceKey, Snippet: snippet, UpdatedAt: p.UpdatedAt})
	}
	if len(rows) == 0 {
		if !*asJSON {
			fmt.Printf("(no memory in %s matches %q — leave one: gadak memory add %q)\n", space, query, query)
		}
		return nil
	}
	if *asJSON {
		for _, r := range rows {
			if err := json.NewEncoder(os.Stdout).Encode(r); err != nil {
				return err
			}
		}
		return nil
	}
	if !*noHeader {
		fmt.Println("id\ttitle\tsnippet\tupdated_at")
	}
	for _, r := range rows {
		fmt.Printf("%s\t%s\t%s\t%s\n", r.ID, r.Title, r.Snippet, r.UpdatedAt)
	}
	return nil
}
