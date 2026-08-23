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
	syncer "github.com/midagedev/gadak/internal/sync"
)

// cmdPage is the wiki write surface (GDK-380). Writes pass through the
// owning origin — connected Confluence or the in-process issuetap — via
// origin.Wiki, never the mirror.
func cmdPage(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("page", nil))
		return nil
	}
	switch args[0] {
	case "create":
		return cmdPageCreate(args[1:])
	case "edit":
		return cmdPageEdit(args[1:])
	case "comment":
		return cmdPageComment(args[1:])
	default:
		return fmt.Errorf("page: unknown subcommand %q (try `gadak page create|edit|comment`)", args[0])
	}
}

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
	if err := syncer.SyncPage(ctx, cfg, db, created.ID); err != nil {
		return fmt.Errorf("page %s created, but the mirror did not refresh (run `gadak sync`): %w", created.ID, err)
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
	if err := syncer.SyncPage(ctx, cfg, db, id); err != nil {
		return fmt.Errorf("comment landed on page %s, but the mirror did not refresh (run `gadak sync`): %w", id, err)
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
	if err := syncer.SyncPage(ctx, cfg, db, id); err != nil {
		return fmt.Errorf("edit applied to page %s, but the mirror did not refresh (run `gadak sync`): %w", id, err)
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
