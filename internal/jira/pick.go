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
}

func (e *AmbiguousTransitionError) Error() string {
	return fmt.Sprintf("transition %q is ambiguous on %s — %d transitions land there: %s",
		e.Want, e.Key, len(e.Candidates), JoinTransitions(e.Candidates))
}

// PickTransition resolves want against the issue's available transitions.
// Order: transition id, target status id, transition name / target status
// name, then category tokens (new|inprogress|done). Two landings in the same
// category refuse rather than picking the first. A token that is one
// transition's id and a different transition's to.id is also refused.
func PickTransition(key, want string, list []Transition) (string, error) {
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
		switch len(hits) {
		case 1:
			return hits[0].ID, nil
		case 0:
			// fall through to the shared miss error, which names reachable tokens
		default:
			return "", &AmbiguousTransitionError{Key: key, Want: want, Candidates: hits}
		}
	}
	return "", noTransitionMatch(key, want, list)
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
