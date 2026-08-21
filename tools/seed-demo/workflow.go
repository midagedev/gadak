package main

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"sort"

	"github.com/midagedev/gadak/internal/jira"
)

// projectTypeStatus holds per-project workflow metadata used while seeding.
type projectTypeStatus struct {
	types    map[string]string // Bug/Story/Task → id
	statuses map[string]string // backlog/selected/inprogress/done → id
}

// issueTypeIDs maps canonical English issue-type names to ids.
//
// issue/createmeta translates type names into the caller's account language
// (and ignores Accept-Language). project/{key}/statuses returns untranslated
// names, so it is the source of truth for the types we seed.
func (c *Client) issueTypeIDs(project string) map[string]string {
	var res []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Subtask bool   `json:"subtask"`
	}
	if !c.call("GET", "/rest/api/3/project/"+url.PathEscape(project)+"/statuses", nil, &res) {
		return nil
	}
	wanted := map[string]bool{}
	for _, t := range wantedTypes {
		wanted[t] = true
	}
	out := map[string]string{}
	for _, it := range res {
		if !it.Subtask && wanted[it.Name] {
			out[it.Name] = it.ID
		}
	}
	return out
}

// projectStatusIDs maps dataset state names to concrete status ids via
// statusCategory (stable) rather than localized status names.
func (c *Client) projectStatusIDs(project string) map[string]string {
	var res []struct {
		Statuses []struct {
			ID             string `json:"id"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"statuses"`
	}
	if !c.call("GET", "/rest/api/3/project/"+url.PathEscape(project)+"/statuses", nil, &res) {
		return nil
	}
	var ordered []statusCat
	seen := map[string]bool{}
	for _, it := range res {
		for _, status := range it.Statuses {
			if seen[status.ID] {
				continue
			}
			seen[status.ID] = true
			ordered = append(ordered, statusCat{
				ID:       status.ID,
				Category: canonicalCategory(status.StatusCategory.Key),
			})
		}
	}
	return MapStatusesFromOrdered(ordered)
}

type apiTransition struct {
	ID string `json:"id"`
	To struct {
		ID             string `json:"id"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"to"`
}

func (c *Client) listTransitions(issueKey string) []apiTransition {
	var res struct {
		Transitions []apiTransition `json:"transitions"`
	}
	if !c.call("GET", "/rest/api/3/issue/"+url.PathEscape(issueKey)+"/transitions", nil, &res) {
		return nil
	}
	return res.Transitions
}

func (c *Client) doTransition(issueKey, transitionID string) bool {
	return c.call("POST", "/rest/api/3/issue/"+url.PathEscape(issueKey)+"/transitions",
		map[string]any{"transition": map[string]string{"id": transitionID}}, nil)
}

func (c *Client) issueStatus(issueKey string) (id, category string, ok bool) {
	var res struct {
		Fields struct {
			Status struct {
				ID             string `json:"id"`
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			} `json:"status"`
		} `json:"fields"`
	}
	if !c.call("GET", "/rest/api/3/issue/"+url.PathEscape(issueKey)+"?fields=status", nil, &res) {
		return "", "", false
	}
	return res.Fields.Status.ID, canonicalCategory(res.Fields.Status.StatusCategory.Key), true
}

// transitionTo walks the workflow to targetID one category rung at a time.
func (c *Client) transitionTo(issueKey, targetID, targetCategory string, hops int) bool {
	if hops <= 0 {
		hops = 5
	}
	if targetCategory == "" {
		targetCategory = "done"
	}
	for range hops {
		opts := c.listTransitions(issueKey)
		if len(opts) == 0 {
			return false
		}
		currentID, currentCat, ok := c.issueStatus(issueKey)
		if !ok {
			return false
		}
		options := make([]TransitionOption, len(opts))
		for i, t := range opts {
			options[i] = TransitionOption{
				ID:         t.ID,
				ToID:       t.To.ID,
				ToCategory: canonicalCategory(t.To.StatusCategory.Key),
			}
		}
		step := PickLadderStep(currentID, currentCat, targetID, targetCategory, options)
		if step.AlreadyThere {
			return true
		}
		if !step.OK {
			return false
		}
		if !c.doTransition(issueKey, step.TransitionID) {
			return false
		}
	}
	// Final check: may have landed on target on the last hop.
	id, _, ok := c.issueStatus(issueKey)
	return ok && id == targetID
}

// applyStates drives each issue to its dataset state, producing real changelog
// history. A reopened issue goes to done then back to its target state.
func (c *Client) applyStates(keys []string, order []SeedIssue, perProject map[string]projectTypeStatus) (moved, reopened int) {
	for i, key := range keys {
		if i >= len(order) {
			break
		}
		item := order[i]
		statuses := perProject[item.Project].statuses
		state := item.State
		if state == "" {
			state = "backlog"
		}
		target := statuses[state]
		if item.Reopened {
			doneID := statuses["done"]
			if doneID != "" && c.transitionTo(key, doneID, "done", 5) {
				moved++
			}
			back := target
			if state == "done" {
				back = statuses["backlog"]
			}
			cat := stateCategory[state]
			if cat == "" {
				cat = "new"
			}
			if back != "" && c.transitionTo(key, back, cat, 5) {
				reopened++
			}
		} else if target != "" && state != "backlog" {
			if c.transitionTo(key, target, stateCategory[state], 5) {
				moved++
			}
		}
	}
	return moved, reopened
}

// walkWorkflow moves an issue a random distance along its workflow (procedural
// path). A minority are pushed to done then back to a todo status (reopen).
func (c *Client) walkWorkflow(issueKey string, rng *rand.Rand) int {
	moves := 0
	steps := weightedInt(rng, []int{0, 1, 2, 3}, []int{25, 30, 30, 15})
	for range steps {
		opts := c.listTransitions(issueKey)
		if len(opts) == 0 {
			break
		}
		var forward []apiTransition
		for _, t := range opts {
			if cat, ok := jira.KnownCategory(t.To.StatusCategory.Key); !ok || cat != "new" {
				forward = append(forward, t)
			}
		}
		if len(forward) == 0 {
			forward = opts
		}
		pick := forward[rng.Intn(len(forward))]
		if !c.doTransition(issueKey, pick.ID) {
			break
		}
		moves++
	}
	if moves > 0 && rng.Float64() < 0.12 {
		opts := c.listTransitions(issueKey)
		var back []apiTransition
		for _, t := range opts {
			if cat, ok := jira.KnownCategory(t.To.StatusCategory.Key); ok && cat == "new" {
				back = append(back, t)
			}
		}
		if len(back) > 0 && c.doTransition(issueKey, back[0].ID) {
			moves++
		}
	}
	return moves
}

func weightedInt(rng *rand.Rand, values, weights []int) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	n := rng.Intn(total)
	for i, w := range weights {
		n -= w
		if n < 0 {
			return values[i]
		}
	}
	return values[len(values)-1]
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func printTypeStatus(project string, pts projectTypeStatus) {
	fmt.Printf("  %s: types=%v statuses=%v\n", project, sortedKeys(pts.types), sortedKeys(pts.statuses))
}

// ensure we can report missing types without panicking
func warnSkip(index int, typeName, project string) {
	fmt.Fprintf(os.Stderr, "  skip #%d: type %s unavailable in %s\n", index, typeName, project)
}
