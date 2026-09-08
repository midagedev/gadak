package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// The Agile API is the one surface where Cloud and Server agree: same
// /rest/agile/1.0 prefix, same {values,isLast,startAt} envelope, same
// lowercase sprint state. Measured on Jira Software 11.3.11 DC against
// Cloud (GDK-1654). It is not under apiBase for that reason.
const agileBase = "/rest/agile/1.0"

// Board is one Jira Software board.
type Board struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	ProjectKey string `json:"-"`
	Location   *struct {
		ProjectKey string `json:"projectKey"`
	} `json:"location"`
}

// Sprint is one sprint as the Agile API states it.
type Sprint struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Goal          string `json:"goal"`
	State         string `json:"state"`
	StartDate     string `json:"startDate"`
	EndDate       string `json:"endDate"`
	CompleteDate  string `json:"completeDate"`
	ActivatedDate string `json:"activatedDate"`
	OriginBoardID int64  `json:"originBoardId"`
}

// ErrNoAgile is what a site without Jira Software answers with. It is not a
// failure of the sync — the mirror simply has no boards.
var ErrNoAgile = errors.New("jira: this site has no Jira Software (no Agile API)")

// Boards lists every board the account can see.
func (c *Client) Boards(ctx context.Context) ([]Board, error) {
	var out []Board
	err := c.agilePage(ctx, agileBase+"/board", func(raw json.RawMessage) error {
		var page []Board
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		for _, b := range page {
			if b.Location != nil {
				b.ProjectKey = b.Location.ProjectKey
			}
			out = append(out, b)
		}
		return nil
	})
	return out, err
}

// Sprints lists one board's sprints, every state.
func (c *Client) Sprints(ctx context.Context, boardID int64) ([]Sprint, error) {
	var out []Sprint
	err := c.agilePage(ctx, fmt.Sprintf("%s/board/%d/sprint", agileBase, boardID), func(raw json.RawMessage) error {
		var page []Sprint
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	})
	return out, err
}

// agilePage walks the values envelope. isLast is authoritative on both
// Jiras; total is absent from Server's board/{id}/sprint, so the empty page
// is the other stop.
func (c *Client) agilePage(ctx context.Context, path string, take func(json.RawMessage) error) error {
	for startAt := 0; ; {
		var page struct {
			MaxResults int             `json:"maxResults"`
			StartAt    int             `json:"startAt"`
			IsLast     bool            `json:"isLast"`
			Values     json.RawMessage `json:"values"`
		}
		p := fmt.Sprintf("%s?startAt=%d&maxResults=50", path, startAt)
		if err := c.do(ctx, http.MethodGet, p, nil, &page); err != nil {
			// do wraps the APIError with the request path, so the status
			// has to be unwrapped for — a plain assertion misses every
			// real 404 and only ever matched in tests (GDK-1654).
			var e *APIError
			if errors.As(err, &e) && (e.Status == http.StatusNotFound || e.Status == http.StatusForbidden) {
				return ErrNoAgile
			}
			return err
		}
		var n int
		if len(page.Values) > 0 {
			var probe []json.RawMessage
			if err := json.Unmarshal(page.Values, &probe); err != nil {
				return err
			}
			n = len(probe)
		}
		if n > 0 {
			if err := take(page.Values); err != nil {
				return err
			}
		}
		startAt += n
		if n == 0 || page.IsLast {
			return nil
		}
	}
}

// MoveToSprint puts issues into a sprint. Jira caps a call at 50 keys.
func (c *Client) MoveToSprint(ctx context.Context, sprintID int64, keys []string) error {
	return c.agileMove(ctx, fmt.Sprintf("%s/sprint/%d/issue", agileBase, sprintID), keys)
}

// MoveToBacklog takes issues out of whatever sprint they are in.
func (c *Client) MoveToBacklog(ctx context.Context, keys []string) error {
	return c.agileMove(ctx, agileBase+"/backlog/issue", keys)
}

func (c *Client) agileMove(ctx context.Context, path string, keys []string) error {
	for len(keys) > 0 {
		n := min(len(keys), 50)
		// write marshals the value it is given; handing it pre-marshalled
		// bytes sends them as a base64 string (measured: Jira 400s naming
		// IssueAssignRequestBean).
		if err := c.write(ctx, http.MethodPost, path, map[string]any{"issues": keys[:n]}, nil); err != nil {
			return err
		}
		keys = keys[n:]
	}
	return nil
}

// CreateSprint opens a future sprint on a board.
func (c *Client) CreateSprint(ctx context.Context, boardID int64, name, goal string) (Sprint, error) {
	payload := map[string]any{"originBoardId": boardID, "name": name}
	if goal != "" {
		payload["goal"] = goal
	}
	var out Sprint
	if err := c.write(ctx, http.MethodPost, agileBase+"/sprint", payload, &out); err != nil {
		return Sprint{}, err
	}
	return out, nil
}

// UpdateSprint sets a sprint's state and, when starting one, its dates.
// Jira refuses `active` without both dates, so the caller supplies them.
func (c *Client) UpdateSprint(ctx context.Context, sprintID int64, fields map[string]any) (Sprint, error) {
	var out Sprint
	if err := c.write(ctx, http.MethodPost, fmt.Sprintf("%s/sprint/%d", agileBase, sprintID), fields, &out); err != nil {
		return Sprint{}, err
	}
	return out, nil
}
