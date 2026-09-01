package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

const unlinkUsage = "usage: gadak unlink <A> <B> --type <name|inward|outward|id> [--json]"

// cmdUnlink removes the link `gadak link A B --type t` would have created —
// the one displayed on A as "A <t> B" (GDK-1205). The mirror's links rows
// carry no link id on purpose, so the id is fetched live from A's projection
// and handed to DELETE /issueLink/{id}; on local-origin that id is issuetap's
// synthetic one, which both projections agree on.
func cmdUnlink(args []string) error {
	fs := newFlagSet("unlink")
	typ := fs.String("type", "", "link type name, inward or outward description, or id")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("unlink", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 || strings.TrimSpace(*typ) == "" {
		return usageError("unlink", unlinkUsage)
	}
	a, b := normalizeKey(pos[0]), normalizeKey(pos[1])
	if a == "" || b == "" {
		return usageError("unlink", unlinkUsage)
	}
	if a == b {
		return fmt.Errorf("cannot unlink %s from itself", a)
	}
	token := strings.TrimSpace(*typ)

	return withKeyWriteSession(a, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
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
		links, err := linker.IssueLinks(ctx, a)
		if err != nil {
			return err
		}
		// The link `link A B --type t` created shows on A as "A <t> B": an
		// outwardIssue element for an outward token, an inwardIssue element
		// for an inward one. A symmetric type matches either side.
		symmetric := lt.Inward == lt.Outward
		var id string
		for _, l := range links {
			if l.Type.Name != lt.Name {
				continue
			}
			outwardHit := l.OutwardIssue != nil && l.OutwardIssue.Key == b
			inwardHit := l.InwardIssue != nil && l.InwardIssue.Key == b
			if (symmetric && (outwardHit || inwardHit)) ||
				(!symmetric && !inwardDescription && outwardHit) ||
				(!symmetric && inwardDescription && inwardHit) {
				if l.ID == "" {
					return fmt.Errorf("the origin's %s link between %s and %s carries no id — cannot delete it over this origin", lt.Name, a, b)
				}
				id = l.ID
				break
			}
		}
		if id == "" {
			phrase := lt.Outward
			if inwardDescription {
				phrase = lt.Inward
			}
			return fmt.Errorf("no link displayed on %s as %q — `gadak issue %s` lists what is there", a, a+" "+phrase+" "+b, a)
		}
		if err := linker.DeleteIssueLink(ctx, id); err != nil {
			return err
		}
		srcB, err := db.KeySource(ctx, b)
		if err != nil {
			return err
		}
		extra := map[string]any{
			"keys":    []string{a, b},
			"deleted": id,
			"type": map[string]string{
				"id":      lt.ID,
				"name":    lt.Name,
				"outward": lt.Outward,
				"inward":  lt.Inward,
			},
		}
		// B first so emitAfterWrite's single-key refresh covers A, exactly
		// like cmdLink.
		if err := syncer.RefreshIssue(ctx, cfg, db, b, srcB); err != nil {
			return emitWriteAppliedMirrorStaleFor(db, b, a, *asJSON, extra, err)
		}
		return emitAfterWrite(ctx, cfg, db, src, a, *asJSON, extra)
	})
}
