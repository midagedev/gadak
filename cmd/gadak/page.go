package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/midagedev/gadak/internal/config"
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
	case "edit":
		return cmdPageEdit(args[1:])
	case "comment":
		return cmdPageComment(args[1:])
	default:
		return fmt.Errorf("page: unknown subcommand %q (try `gadak page edit` or `gadak page comment`)", args[0])
	}
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
	text := fs.String("m", "", "new body as plain text; `-` reads stdin. REPLACES the whole body — rich pages lose formatting, use --adf-file for those")
	adfFile := fs.String("adf-file", "", "new body as an ADF JSON document file; wins over -m")
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
		return fmt.Errorf("page edit: exactly one page id (usage: gadak page edit <ID> [--title T] [-m <text|->|--adf-file F])")
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
	var adf string
	if *adfFile != "" {
		b, err := os.ReadFile(*adfFile)
		if err != nil {
			return err
		}
		if !json.Valid(b) {
			return fmt.Errorf("page edit: %s is not valid JSON (an ADF document)", *adfFile)
		}
		adf = string(b)
	}
	if *title == "" && body == "" && adf == "" {
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
	case adf != "":
		newADF = adf
	case body != "":
		newADF = string(jira.Doc(body, nil))
	}
	if _, err := wc.UpdatePage(ctx, id, newTitle, newADF, cur.Version.Number+1); err != nil {
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
