package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// SavedFilter is a Jira filter the account owns or has starred.
type SavedFilter struct {
	ID        string
	Name      string
	JQL       string
	Favourite bool
	Owner     string
}

type filterJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	JQL       string `json:"jql"`
	Favourite bool   `json:"favourite"`
	Owner     *struct {
		DisplayName string `json:"displayName"`
	} `json:"owner"`
}

// MyFilters returns filters the user owns, plus visible favourites when
// includeFavourites is honoured (Cloud's GET /filter/my).
func (c *Client) MyFilters(ctx context.Context) ([]SavedFilter, error) {
	q := url.Values{}
	q.Set("includeFavourites", "true")
	q.Set("expand", "jql,owner,favourite")
	path := apiPath + "/filter/my?" + q.Encode()
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, err
	}
	return decodeFilterList(raw)
}

func decodeFilterList(raw json.RawMessage) ([]SavedFilter, error) {
	var arr []filterJSON
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("jira: decode filters: %w", err)
	}
	out := make([]SavedFilter, 0, len(arr))
	seen := map[string]bool{}
	for _, f := range arr {
		if f.ID == "" || f.Name == "" {
			continue
		}
		if seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		owner := ""
		if f.Owner != nil {
			owner = f.Owner.DisplayName
		}
		out = append(out, SavedFilter{
			ID:        f.ID,
			Name:      f.Name,
			JQL:       f.JQL,
			Favourite: f.Favourite,
			Owner:     owner,
		})
	}
	return out, nil
}
