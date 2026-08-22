package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
	"github.com/midagedev/gadak/internal/transition"
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
		catalog, err := c.IssueLinkTypes(ctx)
		if err != nil {
			return err
		}
		lt, reverse, err := resolveLinkType(token, catalog)
		if err != nil {
			return err
		}
		outward, inward := a, b
		if reverse {
			outward, inward = b, a
		}
		if err := c.LinkIssues(ctx, lt.ID, outward, inward); err != nil {
			return err
		}
		srcB, err := db.KeySource(ctx, b)
		if err != nil {
			return err
		}
		// B first so emitAfterWrite's single-key refresh covers A: two
		// RefreshIssue calls, then the A summary line (or JSON).
		if err := syncer.RefreshIssue(ctx, cfg, db, b, srcB); err != nil {
			return fmt.Errorf("write applied to %s, but the mirror did not refresh (run `gadak sync`): %w", b, err)
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
		return emitAfterWrite(ctx, cfg, db, src, a, *asJSON, extra)
	})
}

type linkTypeHit struct {
	typ     jira.IssueLinkType
	reverse bool
}

// resolveLinkType matches token against the catalog. All-digit tokens are
// type ids (outward convention). Otherwise name and outward keep A as
// outward; an inward-description match swaps A and B.
func resolveLinkType(token string, catalog []jira.IssueLinkType) (jira.IssueLinkType, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return jira.IssueLinkType{}, false, fmt.Errorf("empty link type")
	}
	if transition.AllASCIIDigits(token) {
		for _, t := range catalog {
			if t.ID == token {
				return t, false, nil
			}
		}
		return jira.IssueLinkType{}, false, fmt.Errorf("no link type id %q — available: %s", token, formatLinkTypes(catalog))
	}
	var hits []linkTypeHit
	for _, t := range catalog {
		name := strings.EqualFold(strings.TrimSpace(t.Name), token)
		out := strings.EqualFold(strings.TrimSpace(t.Outward), token)
		in := strings.EqualFold(strings.TrimSpace(t.Inward), token)
		if !name && !out && !in {
			continue
		}
		outwardDir := name || out
		inwardDir := in
		if outwardDir && inwardDir {
			// Both descriptions of one type match only when they are equal
			// (a symmetric type like Relates) — direction is meaningless
			// there, so this is one hit, not an ambiguity.
			hits = append(hits, linkTypeHit{typ: t, reverse: false})
			continue
		}
		hits = append(hits, linkTypeHit{typ: t, reverse: inwardDir})
	}
	switch len(hits) {
	case 1:
		return hits[0].typ, hits[0].reverse, nil
	case 0:
		return jira.IssueLinkType{}, false, fmt.Errorf("no link type matching %q — available: %s", token, formatLinkTypes(catalog))
	default:
		return jira.IssueLinkType{}, false, fmt.Errorf("link type %q is ambiguous — matches: %s", token, formatLinkTypeHits(hits))
	}
}

func formatLinkTypes(list []jira.IssueLinkType) string {
	if len(list) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(list))
	for _, t := range list {
		parts = append(parts, formatLinkType(t))
	}
	return strings.Join(parts, "; ")
}

func formatLinkTypeHits(hits []linkTypeHit) string {
	seen := map[string]bool{}
	parts := make([]string, 0, len(hits))
	for _, h := range hits {
		if seen[h.typ.ID] {
			continue
		}
		seen[h.typ.ID] = true
		parts = append(parts, formatLinkType(h.typ))
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, "; ")
}

func formatLinkType(t jira.IssueLinkType) string {
	name := t.Name
	if name == "" {
		name = t.ID
	}
	return fmt.Sprintf("%s (id %s, %s / %s)", name, t.ID, t.Outward, t.Inward)
}
