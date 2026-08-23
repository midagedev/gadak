package origin

import (
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/transition"
)

// Resolving a link-type token is the IssueLinker face's own vocabulary, so it
// lives with the face (GDK-85). It used to live in cmd/gadak/link.go, and the
// HTTP handler could not import it — package main is not importable — so the
// REST route arrived as a copy with a parity test guarding the two against
// each other. A parity test is a way to notice a drift, not a way to prevent
// one: there is one owner now and nothing to keep in step.

// LinkTypeHit is one catalog entry that matched a token, with the direction
// the match implies.
type LinkTypeHit struct {
	Type    jira.IssueLinkType
	Reverse bool
}

// ResolveLinkType matches token against the catalog. An all-digit token is a
// type id and keeps the outward convention. Otherwise the type name and its
// outward description keep A as the outward side; a match on the inward
// description swaps A and B. reverse reports that swap.
func ResolveLinkType(token string, catalog []jira.IssueLinkType) (lt jira.IssueLinkType, reverse bool, err error) {
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
		return jira.IssueLinkType{}, false, fmt.Errorf("no link type id %q — available: %s", token, FormatLinkTypes(catalog))
	}
	var hits []LinkTypeHit
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
			hits = append(hits, LinkTypeHit{Type: t, Reverse: false})
			continue
		}
		hits = append(hits, LinkTypeHit{Type: t, Reverse: inwardDir})
	}
	switch len(hits) {
	case 1:
		return hits[0].Type, hits[0].Reverse, nil
	case 0:
		return jira.IssueLinkType{}, false, fmt.Errorf("no link type matching %q — available: %s", token, FormatLinkTypes(catalog))
	default:
		return jira.IssueLinkType{}, false, fmt.Errorf("link type %q is ambiguous — matches: %s", token, formatLinkTypeHits(hits))
	}
}

// FormatLinkTypes renders a catalog for an error message or a listing.
func FormatLinkTypes(list []jira.IssueLinkType) string {
	if len(list) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(list))
	for _, t := range list {
		parts = append(parts, FormatLinkType(t))
	}
	return strings.Join(parts, "; ")
}

func formatLinkTypeHits(hits []LinkTypeHit) string {
	seen := map[string]bool{}
	parts := make([]string, 0, len(hits))
	for _, h := range hits {
		if seen[h.Type.ID] {
			continue
		}
		seen[h.Type.ID] = true
		parts = append(parts, FormatLinkType(h.Type))
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, "; ")
}

// FormatLinkType names one type the way both the CLI listing and the HTTP
// error body name it.
func FormatLinkType(t jira.IssueLinkType) string {
	name := t.Name
	if name == "" {
		name = t.ID
	}
	return fmt.Sprintf("%s (id %s, %s / %s)", name, t.ID, t.Outward, t.Inward)
}
