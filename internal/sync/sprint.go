package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// ghSprintCustom is the schema.custom suffix Jira Software uses for the
// sprint field. The field id itself is per-site and must come from GET /field.
const ghSprintCustom = "com.pyxis.greenhopper.jira:gh-sprint"

// sprintFieldCache holds the discovered gh-sprint field id for one Watch
// (or one Run). A successful lookup — including "this site has no sprint
// field" — is reused; a network error is not, so the next tick retries.
type sprintFieldCache struct {
	mu     sync.Mutex
	id     string
	loaded bool
}

func (s *sprintFieldCache) reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.loaded = false
	s.id = ""
	s.mu.Unlock()
}

func (s *sprintFieldCache) resolve(ctx context.Context, c *jira.Client, opts Options) string {
	if s == nil {
		id, _ := lookupSprintField(ctx, c, opts)
		return id
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return s.id
	}
	id, ok := lookupSprintField(ctx, c, opts)
	if !ok {
		return ""
	}
	s.id = id
	s.loaded = true
	return s.id
}

func lookupSprintField(ctx context.Context, c *jira.Client, opts Options) (id string, loaded bool) {
	catalog, err := c.Fields(ctx)
	if err != nil {
		opts.logf("sprint: field discovery skipped: %v", err)
		return "", false
	}
	return findGhSprintField(catalog), true
}

func findGhSprintField(catalog []jira.FieldInfo) string {
	for _, f := range catalog {
		if strings.HasSuffix(f.Schema.Custom, ghSprintCustom) {
			return f.ID
		}
	}
	return ""
}

func appendSprintField(ids []string, sprintID string) []string {
	if sprintID == "" {
		return ids
	}
	if len(ids) == 1 && ids[0] == "*all" {
		return ids
	}
	for _, id := range ids {
		if id == sprintID {
			return ids
		}
	}
	out := make([]string, len(ids)+1)
	copy(out, ids)
	out[len(ids)] = sprintID
	return out
}

func applySprint(issue *store.Issue, extra map[string]json.RawMessage, fieldID string) {
	if issue == nil || fieldID == "" {
		return
	}
	raw, ok := extra[fieldID]
	if !ok {
		return
	}
	issue.SprintID, issue.SprintName, issue.SprintState = pickSprint(raw)
}

// pickSprint projects a Jira Cloud sprint array onto one (id, name, state).
// Priority is active > future > closed, then larger id. A non-object
// element empties the result so a parse failure cannot abort sync.
func pickSprint(raw json.RawMessage) (id *int64, name, state string) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, "", ""
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, "", ""
	}
	if len(elems) == 0 {
		return nil, "", ""
	}
	type cand struct {
		id    int64
		name  string
		state string
		rank  int
	}
	var best *cand
	for _, e := range elems {
		e = bytes.TrimSpace(e)
		if len(e) == 0 || e[0] != '{' {
			return nil, "", ""
		}
		var o struct {
			ID    json.RawMessage `json:"id"`
			Name  string          `json:"name"`
			State string          `json:"state"`
		}
		if err := json.Unmarshal(e, &o); err != nil {
			return nil, "", ""
		}
		sid, ok := parseSprintID(o.ID)
		if !ok {
			continue
		}
		st := strings.ToLower(strings.TrimSpace(o.State))
		c := cand{id: sid, name: o.Name, state: st, rank: sprintStateRank(st)}
		if best == nil || c.rank > best.rank || (c.rank == best.rank && c.id > best.id) {
			cc := c
			best = &cc
		}
	}
	if best == nil {
		return nil, "", ""
	}
	idv := best.id
	return &idv, best.name, best.state
}

func sprintStateRank(state string) int {
	switch state {
	case "active":
		return 3
	case "future":
		return 2
	case "closed":
		return 1
	default:
		return 0
	}
}

func parseSprintID(raw json.RawMessage) (int64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return n, err == nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		n := int64(f)
		if float64(n) == f {
			return n, true
		}
	}
	return 0, false
}
