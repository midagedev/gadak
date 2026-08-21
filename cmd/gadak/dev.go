package main

// gadak dev — the development-panel verbs (GDK-497). `dev link` records a
// pull request on an issue, standalone-only: the embedded origin (issuetap)
// keeps the link and serves it back in Jira's dev-status shape. On a
// connected workspace the panel belongs to Jira's marketplace apps, so gadak
// refuses instead of pretending. The skill guidance is one line: an agent
// that opens a PR runs `gadak dev link KEY --pr <url>` right there.

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/sync"
)

func cmdDev(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("dev", nil))
		return nil
	}
	switch args[0] {
	case "link":
		return cmdDevLink(args[1:])
	default:
		return fmt.Errorf("dev: unknown subcommand %q (try `gadak dev link`)", args[0])
	}
}

func cmdDevLink(args []string) error {
	fs := newFlagSet("dev link")
	prURL := fs.String("pr", "", "pull request URL (required)")
	name := fs.String("name", "", "display title (omitted = the URL tail)")
	status := fs.String("status", "open", "open | merged | declined")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("dev", fs))
		return nil
	}
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 || *prURL == "" {
		return usageError("dev", "usage: gadak dev link <KEY> --pr <url> [--status open|merged|declined] [--name N]")
	}
	key := normalizeKey(rest[0])
	st := strings.ToUpper(strings.TrimSpace(*status))
	switch st {
	case "OPEN", "MERGED", "DECLINED":
	default:
		return fmt.Errorf("dev link: --status must be open, merged or declined (got %q)", *status)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.IsStandalone() {
		return fmt.Errorf("dev link is for standalone workspaces — a connected workspace's development panel is written by Jira's GitHub app; enable `devStatus` in config.json to mirror it")
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()

	issueID, err := db.ExternalID(ctx, key)
	if err != nil {
		return fmt.Errorf("dev link: %s is not in the mirror — run `gadak sync` first (%w)", key, err)
	}
	client, err := origin.Client(cfg)
	if err != nil {
		return err
	}
	created, err := client.LinkDevPR(ctx, issueID, *prURL, *name, st)
	if err != nil {
		return err
	}

	// Origin accepted — refresh the mirror's dev_links from the origin's
	// answer (write-through, never a direct mirror write of our own copy).
	if prs, err := client.DevStatusPRs(ctx, issueID); err == nil {
		links := sync.DevLinksFromPRs(prs)
		if err := db.ReplaceDevLinks(ctx, key, links); err != nil {
			fmt.Fprintf(os.Stderr, "gadak: warning: origin holds the link but the mirror refresh failed: %v\n", err)
		}
	}

	if *asJSON {
		fmt.Printf("{\"key\":%q,\"url\":%q,\"status\":%q}\n", key, created.URL, created.Status)
		return nil
	}
	fmt.Printf("%s\tPR linked: %s (%s)\n", key, created.URL, strings.ToLower(created.Status))
	return nil
}
