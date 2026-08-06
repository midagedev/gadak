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

	// usage is process-local call volume; see Usage / TakeUsage. Never blocks
	// a request on instrumentation failure (counters are atomic).
	usage usage
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

// Raw sends a request and returns the HTTP status and response body without
// JSON decoding. Path must be site-relative (leading "/"); absolute URLs and
// scheme-relative paths are rejected so the Authorization header never leaves
// the configured site. mutating selects the write retry policy (429/503 only).
//
// A completed HTTP response always returns err == nil with the status and body
// (including non-2xx). err is reserved for transport failures and bad paths.
func (c *Client) Raw(ctx context.Context, method, path string, body []byte, mutating bool) (status int, out []byte, err error) {
	return c.doRaw(ctx, method, path, body, len(body) > 0, mutating)
}

func (c *Client) call(ctx context.Context, method, path string, body, out any, mutating bool) error {
	var payload []byte
	hasBody := body != nil
	if hasBody {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	status, data, err := c.doRaw(ctx, method, path, payload, hasBody, mutating)
	if err != nil {
		return err
	}
	statusLine := fmt.Sprintf("%d %s", status, http.StatusText(status))
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return fmt.Errorf("%s %s: %w (%s)", method, path, ErrAuth, statusLine)
	case status >= 300:
		return apiError(method, path, status, statusLine, data)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// doRaw is the single HTTP path for call and Raw: retries, backoff, and usage.
func (c *Client) doRaw(ctx context.Context, method, path string, payload []byte, hasBody, mutating bool) (int, []byte, error) {
	fullURL, err := c.resolveURL(path)
	if err != nil {
		return 0, nil, err
	}
	retries := retryable
	if mutating {
		retries = func(code int) bool { return code == 429 || code == 503 }
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(payload))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", c.auth)
		req.Header.Set("Accept", "application/json")
		if hasBody {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.HTTP.Do(req)
		// Count every attempt that left the process; retries each draw rate budget.
		c.usage.noteRequest()
		if err != nil {
			if attempt < c.Retries-1 && !mutating {
				if werr := c.wait(ctx, attempt, ""); werr != nil {
					return 0, nil, werr
				}
				c.usage.noteRetry()
				continue
			}
			return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, 64<<20))
		res.Body.Close()
		c.usage.noteStatus(res.StatusCode)
		if retries(res.StatusCode) && attempt < c.Retries-1 {
			if werr := c.wait(ctx, attempt, res.Header.Get("Retry-After")); werr != nil {
				return 0, nil, werr
			}
			c.usage.noteRetry()
			continue
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 && readErr != nil {
			return 0, nil, fmt.Errorf("%s %s: %w", method, path, readErr)
		}
		return res.StatusCode, data, nil
	}
}

// resolveURL joins path onto the configured site and refuses anything that
// would send the Authorization header off-host (absolute URL, scheme-relative
// "//host", or a ResolveReference host change).
func (c *Client) resolveURL(path string) (string, error) {
	if err := rejectAbsolutePath(path); err != nil {
		return "", err
	}
	base, err := url.Parse(c.base)
	if err != nil {
		return "", fmt.Errorf("jira: bad site URL: %w", err)
	}
	// Concatenate like the original call() did. ResolveReference would replace
	// any base path (and is unnecessary for a site-relative path).
	full := c.base + path
	resolved, err := url.Parse(full)
	if err != nil {
		return "", fmt.Errorf("jira: bad path %q: %w", path, err)
	}
	if resolved.Scheme != base.Scheme || !strings.EqualFold(resolved.Host, base.Host) {
		return "", fmt.Errorf("refusing request: resolved host %q is not the configured site", resolved.Host)
	}
	if resolved.User != nil {
		return "", fmt.Errorf("refusing request: userinfo in URL is not allowed")
	}
	return full, nil
}

// rejectAbsolutePath blocks paths that would re-target the request before
// url.ResolveReference (https://…, http://…, //host/…).
func rejectAbsolutePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is required and must start with /")
	}
	lower := strings.ToLower(path)
	// Order: absolute / scheme-relative first so the error names the real risk
	// (token on a foreign host), then require a site-relative leading slash.
	if strings.HasPrefix(path, "//") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") {
		return fmt.Errorf("absolute URLs are not allowed — pass a path starting with / so the request stays on your configured site")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with / (got %q)", path)
	}
	return nil
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
	start := time.Now()
	select {
	case <-ctx.Done():
		c.usage.noteWait(time.Since(start))
		return ctx.Err()
	case <-time.After(d):
		c.usage.noteWait(time.Since(start))
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

// Count returns Jira's approximate issue count for a JQL. It exists only to give
// progress output a denominator, so a failure is not the caller's problem:
// callers treat any error as "unknown" and keep going.
func (c *Client) Count(ctx context.Context, jql string) (int, error) {
	var out struct {
		Count int `json:"count"`
	}
	if err := c.do(ctx, http.MethodPost, apiPath+"/search/approximate-count", map[string]any{"jql": jql}, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
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

// FieldInfo is one row from GET /rest/api/3/field — the site-wide field catalog.
// Distinct from FieldMeta (editmeta for one issue); do not reuse that type here.
type FieldInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Custom bool   `json:"custom"`
	Schema struct {
		Type   string `json:"type"`
		Custom string `json:"custom"`
		Items  string `json:"items"`
	} `json:"schema"`
}

// Fields returns every system and custom field the site exposes to this user.
func (c *Client) Fields(ctx context.Context) ([]FieldInfo, error) {
	var list []FieldInfo
	if err := c.do(ctx, http.MethodGet, apiPath+"/field", nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}
