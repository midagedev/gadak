package jira

// Transition-identifier resolution shared by the CLI (gadak transition) and
// the REST write surface (GDK-341): transition id, then target status id,
// then transition/status name, then a status category token — ambiguity is
// refused with every candidate named, never resolved by picking the first.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/statuscat"
)

// AmbiguousTransitionError is the refusal when want lands on more than one
// transition (GDK-1174: a board with two in-progress statuses makes every
// bare `gadak claim` land here). The message is unchanged from the old
// fmt.Errorf; the type exists so a caller that owns a disambiguation flag
// can name its recourse — this package still never names CLI flags.
type AmbiguousTransitionError struct {
	Key        string
	Want       string
	Candidates []Transition
	// Folded are the candidates a Candidate stands in for: same destination
	// name, same category, so naming one of them could not have told the two
	// readings apart (GDK-1356). Empty unless a category token folded. They
	// are still reachable by transition id or target status id, and the
	// message says so — a reader who saw them in `gadak transition KEY` must
	// not conclude gadak stopped seeing them.
	Folded []Transition
}

func (e *AmbiguousTransitionError) Error() string {
	msg := fmt.Sprintf("transition %q is ambiguous on %s — %d transitions land there: %s",
		e.Want, e.Key, len(e.Candidates), JoinTransitions(e.Candidates))
	if len(e.Folded) > 0 {
		msg += fmt.Sprintf("\nfolded into those (same destination name and category): %s — say a transition id or a target status id to pick one of them",
			JoinTransitions(e.Folded))
	}
	return msg
}

// PickOptions carries the two facts the transitions payload does not hold.
// Both are optional: the zero value is name folding on payload order, which
// is what a caller with no mirror and no status read gets.
type PickOptions struct {
	// CurrentStatusID is the issue's status id at resolution time. Inside a
	// group of same-named destinations it rules that member out: "put me in
	// In Progress" cannot mean the In Progress the issue already sits in.
	// A lone candidate is never ruled out this way — dropping it would let
	// a category token mean some *other* status of that category, which is a
	// worse answer than the self-loop the workflow actually offers.
	CurrentStatusID string

	// StatusUse reports how many issues the local mirror holds in statusID.
	// It breaks the tie inside a folded group only: of two statuses that
	// display the same name in the same category, the one the project is
	// actually using is the one the board shows and the reader means. It is
	// consulted at most once per member of a group larger than one, so the
	// common path never calls it. nil leaves payload order deciding.
	StatusUse func(statusID string) int
}

// PickTransition resolves want with no mirror and no current-status read.
// See PickTransitionWith for the resolution order.
func PickTransition(key, want string, list []Transition) (string, error) {
	return PickTransitionWith(key, want, list, PickOptions{})
}

// PickTransitionWith resolves want against the issue's available transitions.
// Order: transition id, target status id, transition name / target status
// name, then category tokens (new|inprogress|done). Two *distinguishable*
// landings in the same category refuse rather than picking the first; two
// landings the reader cannot tell apart — same destination status name, same
// category — are one landing and fold (GDK-1356). A token that is one
// transition's id and a different transition's to.id is also refused.
func PickTransitionWith(key, want string, list []Transition, opt PickOptions) (string, error) {
	var idHit *Transition
	var toHits []Transition
	for i := range list {
		t := &list[i]
		if t.ID == want && idHit == nil {
			idHit = t
		}
		if t.To.ID != "" && t.To.ID == want {
			toHits = append(toHits, *t)
		}
	}
	if idHit != nil {
		var others []Transition
		for _, t := range toHits {
			if t.ID != idHit.ID {
				others = append(others, t)
			}
		}
		if len(others) > 0 {
			// GDK-1305: on the built-in tracker transition ids (1..n) and
			// status ids (3, 10000, …) overlap for every small workflow, so a
			// bare number lands here every time. The refusal stays — picking
			// the transition id would move an issue whose user typed the
			// status_id they read in SQL — but it names the two forms that
			// cannot collide.
			return "", fmt.Errorf("%q matches a transition id and a different target status id on %s — transition id: %s; target status id: %s\nsay it by name (%q) or by status category (%s) — a bare number is ambiguous when transition ids and status ids overlap",
				want, key, FormatTransition(*idHit), JoinTransitions(others), idHit.Name, strings.Join(ReachableCategories(list), "|"))
		}
		return idHit.ID, nil
	}
	switch len(toHits) {
	case 1:
		return toHits[0].ID, nil
	case 0:
		// names, then category
	default:
		return "", &AmbiguousTransitionError{Key: key, Want: want, Candidates: toHits}
	}
	for _, t := range list {
		if strings.EqualFold(t.Name, want) || strings.EqualFold(t.To.Name, want) {
			return t.ID, nil
		}
	}
	if token, ok := StatusCategoryToken(want); ok {
		var hits []Transition
		for _, t := range list {
			if cat, ok := transitionCategory(t); ok && cat == token {
				hits = append(hits, t)
			}
		}
		if len(hits) > 0 {
			picks, folded := foldSameNamed(hits, opt)
			if len(picks) == 1 {
				return picks[0].ID, nil
			}
			return "", &AmbiguousTransitionError{Key: key, Want: want, Candidates: picks, Folded: folded}
		}
		// fall through to the shared miss error, which names reachable tokens
	}
	return "", noTransitionMatch(key, want, list)
}

// foldSameNamed collapses candidates the reader cannot tell apart into one
// each, and returns the representatives in payload order plus everything they
// stand in for. Two candidates are indistinguishable when their destination
// status carries the same display name and the same category — which is the
// only case where picking for the user cannot land them somewhere they did
// not ask for. A destination with no name never folds: matching empties are
// a damaged payload, not a duplicate (GDK-1356).
func foldSameNamed(hits []Transition, opt PickOptions) (picks, folded []Transition) {
	type group struct{ members []Transition }
	var groups []*group
	index := map[string]*group{}
	for _, t := range hits {
		name := strings.TrimSpace(t.To.Name)
		cat, _ := transitionCategory(t)
		if name == "" {
			groups = append(groups, &group{members: []Transition{t}})
			continue
		}
		k := strings.ToLower(name) + "\x00" + cat
		g := index[k]
		if g == nil {
			g = &group{}
			index[k] = g
			groups = append(groups, g)
		}
		g.members = append(g.members, t)
	}
	for _, g := range groups {
		members := dropCurrent(g.members, opt.CurrentStatusID)
		best := preferInUse(members, opt.StatusUse)
		picks = append(picks, best)
		for _, t := range g.members {
			if t.ID != best.ID {
				folded = append(folded, t)
			}
		}
	}
	return picks, folded
}

// dropCurrent rules the issue's own status out of a group of duplicates. It
// leaves a lone candidate alone (see PickOptions.CurrentStatusID) and never
// empties a group — a payload where every duplicate is the current status
// still has to answer with one of them.
func dropCurrent(members []Transition, currentStatusID string) []Transition {
	if currentStatusID == "" || len(members) < 2 {
		return members
	}
	kept := make([]Transition, 0, len(members))
	for _, t := range members {
		if t.To.ID != currentStatusID {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		return members
	}
	return kept
}

// preferInUse picks the member whose destination the project actually uses.
// Ties and a nil hook keep the first in payload order, so the choice is
// stable when nothing distinguishes the duplicates.
func preferInUse(members []Transition, statusUse func(string) int) Transition {
	best := members[0]
	if statusUse == nil || len(members) < 2 {
		return best
	}
	bestUse := statusUse(best.To.ID)
	for _, t := range members[1:] {
		if use := statusUse(t.To.ID); use > bestUse {
			best, bestUse = t, use
		}
	}
	return best
}

// StatusCategoryToken accepts only the three values data-model.md documents.
// Category and jql.mapStatusCategory both fold aliases (todo, indeterminate)
// onto those values; applying either to the user token would reopen the
// localization trap this command is closing. Apply's category no-op uses
// this same function so a token PickTransition would refuse cannot no-op.
func StatusCategoryToken(s string) (string, bool) {
	switch strings.ToLower(s) {
	case "new", "inprogress", "done":
		return strings.ToLower(s), true
	default:
		return "", false
	}
}

// transitionCategory maps a transition's Jira statusCategory key onto the
// three documented tokens. Empty and unknown keys are refused: Category
// folds those to "new", which would move the issue on a damaged payload.
func transitionCategory(t Transition) (string, bool) {
	return statuscat.KnownCategory(t.To.StatusCategory.Key)
}

func FormatTransition(t Transition) string {
	if t.To.ID == "" {
		return fmt.Sprintf("%s (id %s, → %s)", t.Name, t.ID, t.To.Name)
	}
	return fmt.Sprintf("%s (id %s, → %s [status_id %s])", t.Name, t.ID, t.To.Name, t.To.ID)
}

func JoinTransitions(list []Transition) string {
	parts := make([]string, 0, len(list))
	for _, t := range list {
		parts = append(parts, FormatTransition(t))
	}
	return strings.Join(parts, "; ")
}

func noTransitionMatch(key, want string, list []Transition) error {
	if len(list) == 0 {
		return fmt.Errorf("%s has no available transitions for this credential", key)
	}
	msg := fmt.Sprintf("no transition matching %q on %s — available: %s",
		want, key, JoinTransitions(list))
	if cats := ReachableCategories(list); len(cats) > 0 {
		msg += "\nalso accepts a status category: " + strings.Join(cats, ", ")
	}
	return errors.New(msg)
}

func ReachableCategories(list []Transition) []string {
	seen := map[string]bool{}
	var out []string
	for _, token := range []string{"new", "inprogress", "done"} {
		for _, t := range list {
			cat, ok := transitionCategory(t)
			if ok && cat == token && !seen[token] {
				seen[token] = true
				out = append(out, token)
			}
		}
	}
	return out
}

// DuplicateDestinations reports the groups of transitions whose destination
// status shares a display name and a category — the shape that makes a
// category token fold (GDK-1356), and the shape a reader staring at two
// identical rows in `gadak transition KEY` needs named rather than inferred.
// Groups are in payload order and always hold two or more members; a workflow
// with no duplicates returns nothing.
func DuplicateDestinations(list []Transition) [][]Transition {
	order := make([]string, 0, len(list))
	byKey := map[string][]Transition{}
	for _, t := range list {
		name := strings.TrimSpace(t.To.Name)
		if name == "" {
			continue
		}
		cat, ok := transitionCategory(t)
		if !ok {
			continue
		}
		k := strings.ToLower(name) + "\x00" + cat
		if _, seen := byKey[k]; !seen {
			order = append(order, k)
		}
		byKey[k] = append(byKey[k], t)
	}
	var out [][]Transition
	for _, k := range order {
		if g := byKey[k]; len(g) > 1 {
			out = append(out, g)
		}
	}
	return out
}

// FormatDuplicateDestinations renders DuplicateDestinations as one line per
// group, or "" when there are none.
func FormatDuplicateDestinations(list []Transition) string {
	groups := DuplicateDestinations(list)
	if len(groups) == 0 {
		return ""
	}
	lines := make([]string, 0, len(groups))
	for _, g := range groups {
		lines = append(lines, fmt.Sprintf("%q: %s", g[0].To.Name, JoinTransitions(g)))
	}
	return strings.Join(lines, "\n")
}
