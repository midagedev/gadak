package main

import (
	"context"
	"fmt"
	"github.com/midagedev/gadak/internal/fields"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

const linkUsage = "usage: gadak link <A> <B> --type <name|inward|outward|id> [--json] [--dry-run]\n" +
	"       gadak link <KEY> <url> [--title T] [--as <relationship>] [--json] [--dry-run]"

func cmdLink(args []string) error {
	fs := newFlagSet("link")
	typ := fs.String("type", "", "link type name, inward or outward description, or id")
	title := fs.String("title", "", "remote link title when the target is a URL (default: the URL itself)")
	as := fs.String("as", "", "relationship phrase when the target is a URL (default: relates to)")
	asJSON := fs.Bool("json", false, "emit JSON")
	dryRun := fs.Bool("dry-run", false, "print the link type id and sides this write would send and exit; nothing reaches the origin")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("link", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	// The URL form (GDK-530): a second positional carrying a scheme is a
	// remote link target, never an issue key — CanonicalKey only uppercases,
	// so without this dispatch a typed URL rides the issue-link path to the
	// origin as a garbage key. --type is the issue-link vocabulary; the two
	// grammars do not mix.
	if len(pos) == 2 && strings.Contains(pos[1], "://") {
		if strings.TrimSpace(*typ) != "" {
			return usageError("link", "usage: gadak link: --type names an issue-link type for `link <A> <B>`; a URL target is a remote link — pass --title or --as, not --type")
		}
		return foldDryRun(linkRemoteURL(pos[0], pos[1], strings.TrimSpace(*title), *as, *asJSON, *dryRun))
	}
	// The mirror of the refusal above (GDK-1816): --title and --as are the
	// remote-link vocabulary. Accepting them here and dropping them silently
	// is how one spelling of a write quietly does less than the other.
	if strings.TrimSpace(*title) != "" || strings.TrimSpace(*as) != "" {
		return usageError("link", "usage: gadak link: --title and --as describe a remote link for `link <KEY> <url>`; an issue link is named by --type")
	}
	if len(pos) != 2 || strings.TrimSpace(*typ) == "" {
		return usageError("link", linkUsage)
	}
	a, b := fields.CanonicalKey(pos[0]), fields.CanonicalKey(pos[1])
	if a == "" || b == "" {
		return usageError("link", linkUsage)
	}
	if a == b {
		return fmt.Errorf("cannot link %s to itself", a)
	}
	token := strings.TrimSpace(*typ)

	return foldDryRun(withKeyWriteSession(a, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		linker, err := origin.AsIssueLinker(c)
		if err != nil {
			return err
		}
		catalog, err := linker.IssueLinkTypes(ctx)
		if err != nil {
			return err
		}
		lt, inwardDescription, err := origin.ResolveLinkType(token, catalog)
		if err != nil {
			return err
		}
		// Jira displays type.outward when A is inwardIssue and type.inward
		// when A is outwardIssue. Put A on the end that makes the token the
		// phrase displayed on A.
		outward, inward := b, a
		if inwardDescription {
			outward, inward = a, b
		}
		if *dryRun {
			// The split sits after type resolution so the plan carries the
			// resolved id and the sides in their origin roles, not the typed
			// token (GDK-1446).
			if err := emitDryRun("link", map[string]any{
				"type_id": lt.ID,
				"type":    lt.Name,
				"outward": outward,
				"inward":  inward,
			}, a, b); err != nil {
				return err
			}
			return errDryRun
		}
		if err := linker.LinkIssues(ctx, lt.ID, outward, inward); err != nil {
			return err
		}
		recordAgentWrite(ctx, db, a, "link")
		srcB, err := db.KeySource(ctx, b)
		if err != nil {
			return err
		}
		extra := map[string]any{
			"keys": []string{a, b},
			"type": map[string]string{
				"id":      lt.ID,
				"name":    lt.Name,
				"outward": lt.Outward,
				"inward":  lt.Inward,
			},
		}
		// B first so emitAfterWrite's single-key refresh covers A: two
		// RefreshIssue calls, then the A summary line (or JSON).
		if err := syncer.RefreshIssue(ctx, cfg, db, b, srcB); err != nil {
			return emitWriteAppliedMirrorStaleFor(db, b, a, *asJSON, extra, err)
		}
		return emitAfterWrite(ctx, cfg, db, src, a, *asJSON, extra)
	}))
}

// linkRemoteURL is `gadak link KEY <url> [--title] [--as]` (GDK-530): a
// remote issue link — the origin-native home for a commit/PR URL — written
// through the same core `gadak ref` writes with (addRemoteLink). One write
// path, two front doors, and this one is a spelling of the other: every
// option goes to the owner, none is interpreted here. A PR-shaped URL then
// rides the linked_prs derivation (internal/server ListLinkedPRs) like a
// Linear URL attachment does. `gadak unlink KEY <url>` is the inverse.
func linkRemoteURL(keyRaw, target, title, relationship string, asJSON, dryRun bool) error {
	key := fields.CanonicalKey(keyRaw)
	if key == "" {
		return usageError("link", linkUsage)
	}
	return addRemoteLink("link", key, target, title, relationship, asJSON, dryRun)
}
