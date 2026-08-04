package jira

import (
	"context"
	"fmt"
	"net/http"
)

// Project is one row of the site's project list, as the onboarding picker needs
// it: the key sync will use, a name to recognise it by, and Jira's own type slug.
type Project struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	TypeKey string `json:"projectTypeKey"`
}

// Projects lists the projects this credential can browse, newest API first
// (`/project/search` is the only paged, permission-filtered listing).
//
// It stops after limit projects and reports truncated=true, because a very large
// site would otherwise turn a first-run picker into hundreds of requests.
func (c *Client) Projects(ctx context.Context, limit int) (list []Project, truncated bool, err error) {
	if limit <= 0 {
		limit = 500
	}
	for startAt := 0; ; {
		var page struct {
			Values []Project `json:"values"`
			IsLast bool      `json:"isLast"`
			Total  int       `json:"total"`
		}
		p := fmt.Sprintf("%s/project/search?startAt=%d&maxResults=50&orderBy=key", apiPath, startAt)
		if err := c.do(ctx, http.MethodGet, p, nil, &page); err != nil {
			return nil, false, err
		}
		list = append(list, page.Values...)
		startAt += len(page.Values)
		if len(list) >= limit {
			return list[:limit], startAt < page.Total || !page.IsLast, nil
		}
		if len(page.Values) == 0 || page.IsLast || startAt >= page.Total {
			return list, false, nil
		}
	}
}
