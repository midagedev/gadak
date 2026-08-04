// Package jira is a thin REST client for Jira Cloud: enough of the API to fill
// the mirror, and nothing else. It never writes to Jira from this file.
//
// The token lives only in the Authorization header. It is never put in an error,
// a log line or a URL (constitution article 8), which is why do() reports the
// method and path but never the request itself.
package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiPath = "/rest/api/3"

// ErrAuth aborts a run immediately: retrying a bad credential just burns the
// rate budget (contracts/sync.md, "Rate limits and backoff").
var ErrAuth = errors.New("jira: credential rejected")

type Client struct {
	base string
	auth string

	HTTP *http.Client
	// Retries is the total number of attempts per request; Backoff is the first
	// wait, doubling per attempt and capped at 30 s.
	Retries int
	Backoff time.Duration
}

func New(site, email, token string) *Client {
	return &Client{
		base:    strings.TrimRight(site, "/"),
		auth:    "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token)),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		Retries: 5,
		Backoff: time.Second,
	}
}

// BaseURL is the site origin, used to build deep links.
func (c *Client) BaseURL() string { return c.base }

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	return c.call(ctx, method, path, body, out, false)
}

// write is do() with the retry policy a state-changing request needs. A 500 or a
// dropped connection may mean Jira acted and the answer was lost, so retrying
// would post the comment twice; only 429 and 503, where Jira states it did not
// act, are retried.
func (c *Client) write(ctx context.Context, method, path string, body, out any) error {
	return c.call(ctx, method, path, body, out, true)
}

func (c *Client) call(ctx context.Context, method, path string, body, out any, mutating bool) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	retries := retryable
	if mutating {
		retries = func(code int) bool { return code == 429 || code == 503 }
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", c.auth)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.HTTP.Do(req)
		if err != nil {
			if attempt < c.Retries-1 && !mutating {
				if werr := c.wait(ctx, attempt, ""); werr != nil {
					return werr
				}
				continue
			}
			return fmt.Errorf("%s %s: %w", method, path, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, 64<<20))
		res.Body.Close()
		switch {
		case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
			return fmt.Errorf("%s %s: %w (%s)", method, path, ErrAuth, res.Status)
		case retries(res.StatusCode) && attempt < c.Retries-1:
			if werr := c.wait(ctx, attempt, res.Header.Get("Retry-After")); werr != nil {
				return werr
			}
			continue
		case res.StatusCode >= 300:
			return apiError(method, path, res.StatusCode, res.Status, data)
		}
		if readErr != nil {
			return fmt.Errorf("%s %s: %w", method, path, readErr)
		}
		if out == nil || len(data) == 0 {
			return nil
		}
		return json.Unmarshal(data, out)
	}
}

func retryable(code int) bool {
	switch code {
	case 429, 500, 502, 503, 504:
		return true
	}
	return false
}

func (c *Client) wait(ctx context.Context, attempt int, retryAfter string) error {
	d := c.Backoff << attempt
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	if s, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && s > 0 {
		d = time.Duration(s) * time.Second
	}
	if d <= 0 {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

type searchPage struct {
	Issues        []Issue `json:"issues"`
	NextPageToken string  `json:"nextPageToken"`
	IsLast        bool    `json:"isLast"`
}

// Search pages a JQL query and calls fn once per page, which is what lets sync
// commit page by page. Pagination is by nextPageToken: the legacy startAt search
// is deprecated and drifts under concurrent writes.
func (c *Client) Search(ctx context.Context, jql string, fields []string, withChangelog bool, fn func([]Issue) error) error {
	token := ""
	for {
		body := map[string]any{"jql": jql, "maxResults": 100, "fields": fields}
		if withChangelog {
			body["expand"] = "changelog"
		}
		if token != "" {
			body["nextPageToken"] = token
		}
		var page searchPage
		if err := c.do(ctx, http.MethodPost, apiPath+"/search/jql", body, &page); err != nil {
			return err
		}
		if len(page.Issues) > 0 {
			if err := fn(page.Issues); err != nil {
				return err
			}
		}
		token = page.NextPageToken
		if token == "" || page.IsLast {
			return nil
		}
	}
}

// Changelog pages the full history of one issue, for the issues whose inline
// expand=changelog came back truncated.
func (c *Client) Changelog(ctx context.Context, key string) ([]History, error) {
	out := []History{}
	for startAt := 0; ; {
		var page struct {
			Values []History `json:"values"`
			Total  int       `json:"total"`
			IsLast bool      `json:"isLast"`
		}
		p := fmt.Sprintf("%s/issue/%s/changelog?startAt=%d&maxResults=100", apiPath, url.PathEscape(key), startAt)
		if err := c.do(ctx, http.MethodGet, p, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Values...)
		startAt += len(page.Values)
		if len(page.Values) == 0 || page.IsLast || startAt >= page.Total {
			return out, nil
		}
	}
}

// Comments pages every comment on one issue, for the issues with more than the
// inline limit.
func (c *Client) Comments(ctx context.Context, key string) ([]Comment, error) {
	out := []Comment{}
	for startAt := 0; ; {
		var page CommentPage
		p := fmt.Sprintf("%s/issue/%s/comment?startAt=%d&maxResults=100", apiPath, url.PathEscape(key), startAt)
		if err := c.do(ctx, http.MethodGet, p, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Comments...)
		startAt += len(page.Comments)
		if len(page.Comments) == 0 || startAt >= page.Total {
			return out, nil
		}
	}
}

// Statuses maps every status id on the site to its category. This is the input
// the derived-field rules need, because a changelog entry carries ids only.
func (c *Client) Statuses(ctx context.Context) (map[string]string, error) {
	var list []Status
	if err := c.do(ctx, http.MethodGet, apiPath+"/status", nil, &list); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(list))
	for _, s := range list {
		out[s.ID] = Category(s.StatusCategory.Key)
	}
	return out, nil
}

// Priorities returns the site's priority names, most urgent first, which is the
// order priority_rank counts from.
func (c *Client) Priorities(ctx context.Context) ([]string, error) {
	var list []NamedID
	if err := c.do(ctx, http.MethodGet, apiPath+"/priority", nil, &list); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.Name)
	}
	return out, nil
}
