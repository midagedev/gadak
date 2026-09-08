package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// ghSprintCustom is the schema.custom suffix Jira Software uses for the
// sprint field. The field id itself is per-site and must come from GET /field.
const ghSprintCustom = "com.pyxis.greenhopper.jira:gh-sprint"

// ghEpicLinkCustom is the same story for the epic parent. On Cloud the
// hierarchy is fields.parent; on Server a standard issue's epic lives only
// in this field, so without it epic_key stays empty (GDK-1651).
const ghEpicLinkCustom = "com.pyxis.greenhopper.jira:gh-epic-link"

// agileFields are the per-site custom field ids gadak needs from Jira
// Software. Either may be empty on a site without Jira Software.
type agileFields struct {
	sprint   string
	epicLink string
}

func (a agileFields) ids() []string {
	out := make([]string, 0, 2)
	for _, id := range []string{a.sprint, a.epicLink} {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// sprintFieldCache holds the discovered gh-sprint field id for one Watch
// (or one Run). A successful lookup — including "this site has no sprint
// field" — is reused; a network error is not, so the next tick retries.
type sprintFieldCache struct {
	mu     sync.Mutex
	fields agileFields
	loaded bool
}

func (s *sprintFieldCache) reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.loaded = false
	s.fields = agileFields{}
	s.mu.Unlock()
}

func (s *sprintFieldCache) resolve(ctx context.Context, c *jira.Client, opts Options) agileFields {
	if s == nil {
		f, _ := lookupSprintField(ctx, c, opts)
		return f
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return s.fields
	}
	f, ok := lookupSprintField(ctx, c, opts)
	if !ok {
		return agileFields{}
	}
	s.fields = f
	s.loaded = true
	return s.fields
}

func lookupSprintField(ctx context.Context, c *jira.Client, opts Options) (f agileFields, loaded bool) {
	catalog, err := c.Fields(ctx)
	if err != nil {
		opts.logf("sprint: field discovery skipped: %v", err)
		return agileFields{}, false
	}
	return findGhSprintField(catalog), true
}

func findGhSprintField(catalog []jira.FieldInfo) agileFields {
	var f agileFields
	for _, fi := range catalog {
		switch {
		case strings.HasSuffix(fi.Schema.Custom, ghSprintCustom):
			if f.sprint == "" {
				f.sprint = fi.ID
			}
		case strings.HasSuffix(fi.Schema.Custom, ghEpicLinkCustom):
			if f.epicLink == "" {
				f.epicLink = fi.ID
			}
		}
	}
	return f
}

func appendSprintField(ids []string, extra agileFields) []string {
	add := extra.ids()
	if len(add) == 0 {
		return ids
	}
	if len(ids) == 1 && ids[0] == "*all" {
		return ids
	}
	// append on a clone: the extra slots must not write into ids' array.
	out := slices.Clone(ids)
	for _, id := range add {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func applySprint(issue *store.Issue, extra map[string]json.RawMessage, f agileFields) {
	if issue == nil {
		return
	}
	if f.sprint != "" {
		if raw, ok := extra[f.sprint]; ok {
			issue.SprintID, issue.SprintName, issue.SprintState = pickSprint(raw)
		}
	}
	// Server keys a standard issue's epic here, not in fields.parent, so
	// this is the only way epic_key gets a value there (GDK-1651). Cloud
	// has already set ParentKey; do not overwrite it.
	if f.epicLink != "" && issue.ParentKey == "" {
		if raw, ok := extra[f.epicLink]; ok {
			var key string
			if json.Unmarshal(raw, &key) == nil {
				issue.ParentKey = strings.TrimSpace(key)
			}
		}
	}
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
		sid, name, state, ok := sprintElement(e)
		if !ok {
			return nil, "", ""
		}
		if sid == 0 {
			continue
		}
		st := strings.ToLower(strings.TrimSpace(state))
		c := cand{id: sid, name: name, state: st, rank: sprintStateRank(st)}
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

// sprintElement reads one element of the gh-sprint array. Cloud sends an
// object; Server sends the Java toString of the Sprint bean
// (GDK-1650) — the same field, two wire shapes. ok is false when the
// element is neither, which empties the whole result as before.
func sprintElement(e json.RawMessage) (id int64, name, state string, ok bool) {
	switch {
	case len(e) == 0:
		return 0, "", "", false
	case e[0] == '{':
		var o struct {
			ID    json.RawMessage `json:"id"`
			Name  string          `json:"name"`
			State string          `json:"state"`
		}
		if err := json.Unmarshal(e, &o); err != nil {
			return 0, "", "", false
		}
		n, found := parseSprintID(o.ID)
		if !found {
			return 0, "", "", true
		}
		return n, o.Name, o.State, true
	case e[0] == '"':
		var s string
		if err := json.Unmarshal(e, &s); err != nil {
			return 0, "", "", false
		}
		return parseSprintToString(s)
	}
	return 0, "", "", false
}

// sprintToStringID and friends cut the Server toString, whose keys are
// alphabetical: …,goal=,id=,incompleteIssuesDestinationId=,name=,
// rapidViewId=,sequence=,startDate=,state=,… Anchoring name on the
// rapidViewId that always follows it is what survives a comma in the name.
// ponytail: a name containing the literal ",rapidViewId=" would still cut
// short; Jira itself has no escaping here, so neither do we.
var (
	sprintToStringID    = regexp.MustCompile(`(?:^|,)id=(\d+),`)
	sprintToStringState = regexp.MustCompile(`(?:^|,)state=([A-Za-z_]+)(?:,|$)`)
	sprintToStringName  = regexp.MustCompile(`(?:^|,)name=(.*?),rapidViewId=`)
)

func parseSprintToString(s string) (id int64, name, state string, ok bool) {
	if !strings.Contains(s, "[") || !strings.HasSuffix(strings.TrimSpace(s), "]") {
		return 0, "", "", false
	}
	inner := s[strings.Index(s, "[")+1:]
	inner = strings.TrimSuffix(strings.TrimSpace(inner), "]")
	m := sprintToStringID.FindStringSubmatch(inner)
	if m == nil {
		return 0, "", "", true
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, "", "", true
	}
	if m := sprintToStringName.FindStringSubmatch(inner); m != nil {
		name = m[1]
	}
	if m := sprintToStringState.FindStringSubmatch(inner); m != nil {
		state = m[1]
	}
	return n, name, state, true
}
