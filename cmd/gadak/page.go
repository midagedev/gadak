package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// cmdPage is the wiki surface: mirror reads (get, list) and writes
// through the owning origin (GDK-380) — connected Confluence or the in-process
// issuetap — via origin.Wiki, never the mirror.
func cmdPage(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("page", nil))
		return nil
	}
	switch args[0] {
	case "get":
		return cmdPageGet(args[1:])
	case "list":
		return cmdPageList(args[1:])
	case "create":
		return cmdPageCreate(args[1:])
	case "edit":
		return cmdPageEdit(args[1:])
	case "comment":
		return cmdPageComment(args[1:])
	default:
		return fmt.Errorf("page: unknown subcommand %q (try `gadak page get|list|create|edit|comment`)", args[0])
	}
}

// cmdPageGet reads one page from the local mirror — the read-side sibling of
// the write verbs. Same shape as `gadak issue KEY`: no origin
// call, the detail document comes from the store, and the text form follows
// printIssue's rhythm so a session that knows one knows the other.
func cmdPageGet(args []string) error {
	fs := newFlagSet("page get")
	asJSON := fs.Bool("json", false, "emit JSON (the page detail document)")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("page", fs))
		return nil
	}
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("page get: exactly one page id (usage: gadak page get <ID> [--json]; ids: gadak page list)")
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)
	p, err := db.PageDetail(context.Background(), rest[0])
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("no page %s in the mirror — check the id, or list them: gadak page list", rest[0])
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(p)
	}
	printPage(*p)
	return nil
}

// printPage is the text form of a page detail: kv header, the body as an
// indented section, then comments — printIssue's shapes, with the GDK-1020
// empty markers so an absent section is an answer, not a truncation.
func printPage(p store.PageDetail) {
	fmt.Printf("%s\t%s\n", p.Key, p.Title)
	kv := func(label, value string) {
		if value != "" {
			fmt.Printf("%-13s %s\n", label, value)
		}
	}
	if p.SpaceName != "" {
		kv("space", fmt.Sprintf("%s (%s)", p.SpaceKey, p.SpaceName))
	} else {
		kv("space", p.SpaceKey)
	}
	if p.Version > 0 {
		kv("version", fmt.Sprintf("%d", p.Version))
	}
	kv("updated", p.UpdatedAt)
	if len(p.Labels) > 0 {
		kv("labels", strings.Join(p.Labels, ", "))
	}
	if body := strings.TrimSpace(p.BodyText); body != "" {
		fmt.Printf("\nbody\n%s\n", indent(body))
	} else {
		fmt.Printf("\nbody (none)\n")
	}
	if len(p.Comments) > 0 {
		fmt.Printf("\ncomments (%d)\n", len(p.Comments))
		for _, c := range p.Comments {
			fmt.Printf("  %s  %s\n%s\n", c.CreatedAt, c.Author, indent(strings.TrimSpace(c.BodyText)))
		}
	} else {
		fmt.Printf("\ncomments (0)\n")
	}
}

// cmdPageList answers "which id do I pass to page get/edit/comment"
// — mirrored pages newest first, list.go's shape: openReadOnly so a typo
// cannot write, writeSQLQuery for the shared --json/--csv/--no-header rows.
func cmdPageList(args []string) error {
	fs := newFlagSet("page list")
	space := fs.String("space", "", "only pages in this space key")
	limit := fs.Int("limit", defaultListLimit, "maximum rows to list")
	asJSON := fs.Bool("json", false, "emit one JSON object per row")
	asCSV := fs.Bool("csv", false, "emit CSV with a header row")
	noHeader := fs.Bool("no-header", false, "omit the TSV/CSV header row (no-op with --json)")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("page", fs))
		return nil
	}
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("page list: unexpected argument %q (usage: gadak page list [--space K] [--limit N] [--json|--csv|--no-header])", rest[0])
	}
	if *limit <= 0 {
		return fmt.Errorf("page list: --limit must be 1 or more")
	}
	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)
	var n int
	if err := db.QueryRow("select count(*)" + pageListWhere(*space)).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		// views-list stance (GDK-1020): one parenthesized, tab-free stdout
		// line instead of a bare header — but only for the TSV form; --json
		// and --csv keep their zero-row stream contracts (no objects, or the
		// header row alone) because those are parsed, not read.
		if !*asJSON && !*asCSV {
			fmt.Println("(no pages in the mirror — sync first, or create one: gadak page create --space K --title T -m TEXT)")
		}
		return nil
	}
	q := "select it.key as id, p.space_key as space, it.title, it.updated_at as updated_at" +
		pageListWhere(*space) + fmt.Sprintf(" order by it.updated_at desc limit %d", *limit)
	return writeSQLQuery(db, q, sqlOutput{JSON: *asJSON, CSV: *asCSV, NoHeader: *noHeader})
}

// pageListWhere is the shared filter of the count and the row query so the
// empty-state check and the listing can never disagree. Space matching is
// case-insensitive like the space catalog lookup in formatPageSpaceError —
// keys are uppercase by convention, and a wrong case returning zero rows
// silently is the display-name trap in miniature.
func pageListWhere(space string) string {
	q := " from pages p join items it on it.id = p.item_id where it.kind = 'page'"
	if s := strings.TrimSpace(space); s != "" {
		q += " and p.space_key = " + sqlLiteral(s) + " collate nocase"
	}
	return q
}

// errNoPageCredential is the empty-home refusal the page write verbs share
// (GDK-943): a workspace with no origin cannot edit pages. Sentence owner
// is config.ErrNotConfigured; the addendum is the wiki's, because a page
// write that cannot reach an origin is not a local edit. A workspace whose
// origin exists but lacks site+email+token keeps origin.Wiki's
// errNeedCredential instead.
var errNoPageCredential = config.NotConfiguredWith("wiki pages go to the origin, not to the mirror")

func cmdPageCreate(args []string) error {
	fs := newFlagSet("page create")
	space := fs.String("space", "", "space key (required)")
	title := fs.String("title", "", "page title (required)")
	parent := fs.String("parent", "", "parent page id (omitted = space root)")
	text := fs.String("m", "", "body as plain text; `-` reads stdin")
	adfFile := fs.String("adf-file", "", "body as an ADF JSON document file; wins over -m")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("page", fs))
		return nil
	}
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return fmt.Errorf("page create: unexpected argument %q (usage: gadak page create --space KEY --title T [-m <text|->|--adf-file F] [--parent ID])", rest[0])
	}
	if *space == "" || *title == "" {
		return fmt.Errorf("page create: --space and --title are required")
	}
	warnWorkspaceIfEnv()
	body := *text
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	adf := ""
	if *adfFile != "" {
		b, err := os.ReadFile(*adfFile)
		if err != nil {
			return err
		}
		if !json.Valid(b) {
			return fmt.Errorf("page create: %s is not valid JSON (an ADF document)", *adfFile)
		}
		adf = string(b)
	}
	if adf == "" {
		adf = string(jira.Doc(body, nil))
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasOrigin() {
		return errNoPageCredential
	}
	wc, err := origin.Wiki(cfg)
	if err != nil {
		return err
	}
	ctx := context.Background()
	created, err := wc.CreatePage(ctx, *space, *title, adf, *parent)
	if err != nil {
		return formatPageSpaceError(ctx, wc, *space, err)
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := syncer.RefreshPage(ctx, cfg, db, created.ID); err != nil {
		warnWriteAppliedMirrorStale(created.ID, err)
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"id":           created.ID,
				"mirror_stale": true,
			})
		}
		fmt.Printf("%s\t%s\n", created.ID, created.Title)
		return nil
	}
	if *asJSON {
		detail, err := db.PageDetail(ctx, created.ID)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"page": detail})
	}
	fmt.Printf("%s\t%s\n", created.ID, created.Title)
	return nil
}

// formatPageSpaceError replaces a missing-space 400 JSON dump with the
// same catalog sentence --type uses (GDK-467). A space that is in the
// catalog keeps the origin error (the 400 is then about something else).
func formatPageSpaceError(ctx context.Context, wc *confluence.Client, space string, err error) error {
	if wc == nil || err == nil {
		return err
	}
	spaces, serr := wc.Spaces(ctx)
	if serr != nil {
		return err
	}
	want := strings.TrimSpace(space)
	keys := make([]string, 0, len(spaces))
	found := false
	for _, s := range spaces {
		k := strings.TrimSpace(s.Key)
		if k == "" {
			continue
		}
		keys = append(keys, k)
		if strings.EqualFold(k, want) {
			found = true
		}
	}
	if found || len(keys) == 0 {
		return err
	}
	return fmt.Errorf("no space matching %q — available: %s", space, strings.Join(keys, ", "))
}

func cmdPageComment(args []string) error {
	fs := newFlagSet("page comment")
	text := fs.String("m", "", "comment body as plain text; `-` reads stdin")
	adfFile := fs.String("adf-file", "", "comment body as an ADF JSON document file; wins over -m")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("page", fs))
		return nil
	}
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("page comment: exactly one page id (usage: gadak page comment <ID> -m <text|-> | --adf-file F)")
	}
	id := rest[0]
	body := *text
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	adf := ""
	if *adfFile != "" {
		b, err := os.ReadFile(*adfFile)
		if err != nil {
			return err
		}
		if !json.Valid(b) {
			return fmt.Errorf("page comment: %s is not valid JSON (an ADF document)", *adfFile)
		}
		adf = string(b)
	}
	if adf == "" {
		if body == "" {
			return fmt.Errorf("page comment: nothing to post — pass -m or --adf-file")
		}
		adf = string(jira.Doc(body, nil))
	}
	warnWorkspaceIfEnv()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasOrigin() {
		return errNoPageCredential
	}
	wc, err := origin.Wiki(cfg)
	if err != nil {
		return err
	}
	ctx := context.Background()
	cm, err := wc.AddPageComment(ctx, id, adf)
	if err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := syncer.RefreshPage(ctx, cfg, db, id); err != nil {
		warnWriteAppliedMirrorStale(id, err)
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"comment":      cm,
				"mirror_stale": true,
			})
		}
		fmt.Printf("%s\tcomment %s added\n", id, cm.ID)
		return nil
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"comment": cm})
	}
	fmt.Printf("%s\tcomment %s added\n", id, cm.ID)
	return nil
}

func cmdPageEdit(args []string) error {
	fs := newFlagSet("page edit")
	title := fs.String("title", "", "new page title (omitted = keep)")
	text := fs.String("m", "", "new body as plain text; `-` reads stdin. REPLACES the whole body; refused on a rich page unless --force (use --adf-file to keep formatting)")
	adfFile := fs.String("adf-file", "", "new body as an ADF JSON document file; wins over -m")
	version := fs.Int("version", 0, "base version for optimistic lock (the mirror's pages.version); omit to last-write-wins from origin HEAD")
	force := fs.Bool("force", false, "replace a rich page with -m anyway")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("page", fs))
		return nil
	}
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("page edit: exactly one page id (usage: gadak page edit <ID> [--title T] [-m <text|->|--adf-file F] [--version N] [--force])")
	}
	id := rest[0]
	body := *text
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	var fileADF string
	if *adfFile != "" {
		b, err := os.ReadFile(*adfFile)
		if err != nil {
			return err
		}
		if !json.Valid(b) {
			return fmt.Errorf("page edit: %s is not valid JSON (an ADF document)", *adfFile)
		}
		fileADF = string(b)
	}
	if *title == "" && body == "" && fileADF == "" {
		return fmt.Errorf("page edit: nothing to change — pass --title, -m, or --adf-file")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasOrigin() {
		return errNoPageCredential
	}
	wc, err := origin.Wiki(cfg)
	if err != nil {
		return err
	}
	ctx := context.Background()
	cur, err := wc.Page(ctx, id)
	if err != nil {
		return err
	}
	newTitle := cur.Title
	if *title != "" {
		newTitle = *title
	}
	newADF := ""
	if cur.Body.AtlasDocFormat != nil {
		newADF = cur.Body.AtlasDocFormat.Value
	}
	switch {
	case fileADF != "":
		newADF = fileADF
	case body != "":
		if !*force && !adf.IsSimple(newADF) {
			return fmt.Errorf("page edit: -m replaces the whole body and would drop this page's formatting; pass --adf-file to keep it, or --force to replace it")
		}
		newADF = string(jira.Doc(body, nil))
	}
	next := cur.Version.Number + 1
	if *version > 0 {
		next = *version + 1
	}
	if _, err := wc.UpdatePage(ctx, id, newTitle, newADF, next); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := syncer.RefreshPage(ctx, cfg, db, id); err != nil {
		warnWriteAppliedMirrorStale(id, err)
		detail, _ := db.PageDetail(ctx, id)
		if *asJSON {
			body := map[string]any{"id": id, "mirror_stale": true}
			if detail != nil {
				body["page"] = detail
			}
			return json.NewEncoder(os.Stdout).Encode(body)
		}
		if detail != nil {
			fmt.Printf("%s\t%s\n", id, detail.Title)
		} else {
			fmt.Printf("%s edited (row not in the mirrored spaces)\n", id)
		}
		return nil
	}
	detail, err := db.PageDetail(ctx, id)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"page": detail})
	}
	if detail != nil {
		fmt.Printf("%s\t%s\n", id, detail.Title)
	} else {
		fmt.Printf("%s edited (row not in the mirrored spaces)\n", id)
	}
	return nil
}
