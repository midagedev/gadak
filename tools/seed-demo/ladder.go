package main

import "github.com/midagedev/gadak/internal/statuscat"

// Category ladder used for multi-entry changelog history.
// Default Jira workflows offer Backlog→Done; taking it leaves a single
// changelog entry. Walking one rung at a time produces real history.
// Rungs are gadak tokens (new|inprogress|done); Jira's REST key
// "indeterminate" is folded by statuscat.KnownCategory.
var categoryLadder = []string{"new", "inprogress", "done"}

// stateCategory maps dataset state names onto gadak status_category tokens.
var stateCategory = map[string]string{
	"backlog":    "new",
	"selected":   "new",
	"inprogress": "inprogress",
	"done":       "done",
}

// TransitionOption is one available workflow edge from the current status.
type TransitionOption struct {
	ID         string
	ToID       string
	ToCategory string
}

// LadderStep is the pure decision made for one hop of transitionTo.
type LadderStep struct {
	// AlreadyThere is true when current status id equals the target.
	AlreadyThere bool
	// TransitionID is the transition to fire; empty if AlreadyThere or !OK.
	TransitionID string
	// OK is false when no usable path exists from the current options.
	OK bool
}

// PickLadderStep chooses the next transition toward targetID without skipping
// category rungs when a stepwise path exists. Pure: no I/O.
//
// Algorithm (ported from the original Python transition_to):
//  1. Same status id → done.
//  2. Same category, different status → take the edge to targetID if present.
//  3. Otherwise step one category rung toward the target; prefer an edge that
//     lands on targetID when it sits on the next rung.
//  4. If no rung-by-rung edge exists, accept a direct jump to targetID.
func PickLadderStep(currentID, currentCategory, targetID, targetCategory string, options []TransitionOption) LadderStep {
	if targetCategory == "" {
		targetCategory = "done"
	}
	if currentID == targetID {
		return LadderStep{AlreadyThere: true, OK: true}
	}
	wantRung, okWant := ladderIndex(targetCategory)
	hereRung, okHere := ladderIndex(currentCategory)
	if !okWant || !okHere {
		// Unknown category — fall through to direct target match only.
		if t := findByToID(options, targetID); t != nil {
			return LadderStep{TransitionID: t.ID, OK: true}
		}
		return LadderStep{OK: false}
	}
	if hereRung == wantRung {
		same := findByToID(options, targetID)
		if same == nil {
			return LadderStep{OK: false}
		}
		return LadderStep{TransitionID: same.ID, OK: true}
	}
	step := 1
	if wantRung < hereRung {
		step = -1
	}
	nextCategory := categoryLadder[hereRung+step]
	var candidates []TransitionOption
	for _, t := range options {
		if canonicalCategory(t.ToCategory) == nextCategory {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		direct := findByToID(options, targetID)
		if direct == nil {
			return LadderStep{OK: false}
		}
		return LadderStep{TransitionID: direct.ID, OK: true}
	}
	// Prefer the target itself when it happens to be on the next rung.
	for _, t := range candidates {
		if t.ToID == targetID {
			return LadderStep{TransitionID: t.ID, OK: true}
		}
	}
	return LadderStep{TransitionID: candidates[0].ID, OK: true}
}

func canonicalCategory(c string) string {
	if mapped, ok := statuscat.KnownCategory(c); ok {
		return mapped
	}
	return c
}

func ladderIndex(category string) (int, bool) {
	cat := canonicalCategory(category)
	for i, c := range categoryLadder {
		if c == cat {
			return i, true
		}
	}
	return 0, false
}

func findByToID(options []TransitionOption, toID string) *TransitionOption {
	for i := range options {
		if options[i].ToID == toID {
			return &options[i]
		}
	}
	return nil
}

// MapStatusesFromOrdered maps dataset state names to status ids using
// statusCategory only (never localized names). ordered is workflow order;
// first "new" is backlog, second "new" is selected.
func MapStatusesFromOrdered(ordered []statusCat) map[string]string {
	var todo, progress, done []string
	for _, s := range ordered {
		switch canonicalCategory(s.Category) {
		case "new":
			todo = append(todo, s.ID)
		case "inprogress":
			progress = append(progress, s.ID)
		case "done":
			done = append(done, s.ID)
		}
	}
	out := map[string]string{}
	if len(todo) > 0 {
		out["backlog"] = todo[0]
		if len(todo) > 1 {
			out["selected"] = todo[1]
		} else {
			out["selected"] = todo[0]
		}
	}
	if len(progress) > 0 {
		out["inprogress"] = progress[0]
	}
	if len(done) > 0 {
		out["done"] = done[0]
	}
	return out
}

type statusCat struct {
	ID       string
	Category string
}

// KeyAssigneeIndex spreads non-dataset assigned issues across a pool by key
// hash. sum(ord(c) for c in key) % n — same as the Python seeder, so re-runs
// are a no-op.
func KeyAssigneeIndex(key string, n int) int {
	if n <= 0 {
		return 0
	}
	sum := 0
	for _, r := range key {
		sum += int(r)
	}
	// Keep non-negative for Go's % on negative dividends.
	if sum < 0 {
		sum = -sum
	}
	return sum % n
}

// ResolveAssigneeTarget picks the accountId that repair_assignees should set.
// slot is non-nil for dataset issues (null slot → unassigned). current is the
// issue's current accountId (empty if unassigned). For non-dataset assigned
// issues, distribution is by KeyAssigneeIndex.
func ResolveAssigneeTarget(summary string, slots map[string]*int, key, current string, assignees []string) (target string, inDataset bool) {
	if slot, ok := slots[summary]; ok {
		if slot == nil || len(assignees) == 0 {
			return "", true
		}
		return assignees[*slot%len(assignees)], true
	}
	if current != "" && len(assignees) > 0 {
		return assignees[KeyAssigneeIndex(key, len(assignees))], false
	}
	return "", false
}
