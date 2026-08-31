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

const linkUsage = "usage: gadak link <A> <B> --type <name|inward|outward|id> [--json]"

func cmdLink(args []string) error {
	fs := newFlagSet("link")
	typ := fs.String("type", "", "link type name, inward or outward description, or id")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("link", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 || strings.TrimSpace(*typ) == "" {
		return usageError("link", linkUsage)
	}
	a, b := normalizeKey(pos[0]), normalizeKey(pos[1])
	if a == "" || b == "" {
		return usageError("link", linkUsage)
	}
	if a == b {
		return fmt.Errorf("cannot link %s to itself", a)
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
		// Jira displays type.outward when A is inwardIssue and type.inward
		// when A is outwardIssue. Put A on the end that makes the token the
		// phrase displayed on A.
		outward, inward := b, a
		if inwardDescription {
			outward, inward = a, b
		}
		if err := linker.LinkIssues(ctx, lt.ID, outward, inward); err != nil {
			return err
		}
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
	})
}
