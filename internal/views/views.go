// Package views owns the interpretation of view names: how the Jira filters
// and saved views inside a store are listed, and how one name resolves to
// exactly one of them.
//
// Two surfaces speak views by name — `gadak views` (cmd/gadak) and the MCP
// gadak_show tool (internal/mcp) — and until GDK-612 each carried its own copy
// of this logic. The copies had already drifted: the same missing name drew
// "run `gadak views`" from the CLI and an available-views list from MCP, and
// MCP could not report a saved view's applied/unsupported clauses at all.
// Both surfaces now import this package; the 0-hit error is the MCP pair
// (empty-workspace diagnosis / available list) because it carries more
// information than the CLI hint it replaced.
package views

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
)

// ListedView is one view a name can resolve to: a synced Jira filter
// (Kind "jira") or a view saved by `gadak views save` (Kind "saved").
// Config rides along so callers that need the raw saved body (notably
// `views save` round-trips) do not re-read the store.
type ListedView struct {
	Kind        string          `json:"kind"` // jira | saved
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	JQL         string          `json:"jql,omitempty"`
	Hash        string          `json:"hash"`
	Favourite   bool            `json:"favourite,omitempty"`
	Owner       string          `json:"owner,omitempty"`
	Applied     []string        `json:"applied,omitempty"`
	Unsupported []string        `json:"unsupported,omitempty"`
	Config      json.RawMessage `json:"-"`
}

// LoadViews lists every resolvable view in the store: synced Jira filters
// first, then locally saved views. The saved rows are decoded through the same
// strict parse as HashFromConfig, so a config whose metadata cannot unmarshal
// surfaces as an empty Hash rather than half a view.
func LoadViews(db *store.DB) ([]ListedView, error) {
	out := make([]ListedView, 0)
	src, err := db.SourceQueries(context.Background(), "jira")
	if err != nil {
		return nil, err
	}
	for _, q := range src {
		out = append(out, ListedView{
			Kind: "jira", ID: q.ID, Name: q.Name, JQL: q.QueryText,
			Hash: HashFromConfig(q.Config), Favourite: q.Favourite, Owner: q.Owner,
			Applied: q.Applied, Unsupported: q.Unsupported, Config: q.Config,
		})
	}
	saved, err := db.SavedViews(context.Background())
	if err != nil {
		return nil, err
	}
	for _, s := range saved {
		h, jqlText, applied, unsupported := savedViewFields(s.Config)
		out = append(out, ListedView{
			Kind: "saved", ID: s.ID, Name: s.Name,
			JQL: jqlText, Hash: h, Applied: applied, Unsupported: unsupported,
			Config: s.Config,
		})
	}
	return out, nil
}

// FindView resolves one name to one view. A name matches exactly against the
// view's id, name, or id suffix (the part after the last ":"), case- and
// space-insensitively; only when nothing matches exactly does a substring of
// the name or id count. Exact beats substring; a single substring hit is
// accepted so prefixes stay usable.
func FindView(db *store.DB, name string) (ListedView, error) {
	list, err := LoadViews(db)
	if err != nil {
		return ListedView{}, err
	}
	want := strings.ToLower(strings.TrimSpace(name))
	var exact, sub []ListedView
	for _, v := range list {
		id := strings.ToLower(v.ID)
		nm := strings.ToLower(v.Name)
		ext := ""
		if i := strings.LastIndex(v.ID, ":"); i >= 0 {
			ext = strings.ToLower(v.ID[i+1:])
		}
		if id == want || nm == want || ext == want {
			exact = append(exact, v)
			continue
		}
		if strings.Contains(nm, want) || strings.Contains(id, want) {
			sub = append(sub, v)
		}
	}
	hits := exact
	if len(hits) == 0 {
		hits = sub
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		if len(list) == 0 {
			return ListedView{}, fmt.Errorf("no view matching %q — no saved views or synced Jira filters in this workspace", name)
		}
		names := make([]string, 0, len(list))
		for _, v := range list {
			if v.Name != "" {
				names = append(names, v.Name)
			} else {
				names = append(names, v.ID)
			}
		}
		return ListedView{}, fmt.Errorf("no view matching %q — available: %s", name, strings.Join(names, "; "))
	default:
		names := make([]string, len(hits))
		for i, h := range hits {
			names[i] = h.Name
		}
		return ListedView{}, fmt.Errorf("%q matches %d views — be more specific: %s", name, len(hits), strings.Join(names, "; "))
	}
}

// HashFromConfig returns the view hash stored in a saved-view config, or ""
// when the config is empty or does not unmarshal.
func HashFromConfig(raw json.RawMessage) string {
	h, _, _, _ := savedViewFields(raw)
	return h
}

func savedViewFields(raw json.RawMessage) (hash, jqlText string, applied, unsupported []string) {
	if len(raw) == 0 {
		return "", "", nil, nil
	}
	var c struct {
		Filters     jql.Filter  `json:"filters"`
		Display     jql.Display `json:"display"`
		JQL         string      `json:"jql"`
		Applied     []string    `json:"applied"`
		Unsupported []string    `json:"unsupported"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", "", nil, nil
	}
	return jql.Hash(c.Filters, c.Display), c.JQL, c.Applied, c.Unsupported
}
