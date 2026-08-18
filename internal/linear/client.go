// Package linear is the read-only GraphQL client for a Linear workspace:
// viewer, teams, workflow states, and cursor-paged issues with an updatedAt
// watermark filter. It issues no mutations — this connector mirrors, and a
// mirror never writes to the origin (gadak constitution, "writes pass through
// the origin"; Linear writes, if ever, are a separate decision).
//
// The API key lives only in the Authorization header. It is never put in an
// error, a log line, or a URL — the same article-8 discipline internal/jira
// follows. This package contains no logging at all.
//
// Why not internal/atlhttp: that package is the shared transport for
// *Atlassian Cloud* clients, and its headline guarantee is path safety —
// joining a caller-supplied path onto a configured site so the Authorization
// header cannot wander off-host. Linear has one fixed endpoint and no path
// parameter, so that machinery has nothing to do; what is worth keeping
// (retry/backoff policy, Retry-After handling, the 64 MiB response cap, the
// usage counters) is mirrored here with the same semantics, and the
// rate-limit visibility is Linear-specific: Linear states its budget in
// response headers (x-ratelimit-*), which Atlassian never did.
package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Endpoint is the single Linear GraphQL URL. Personal API keys are the only
// credential shape this client supports.
const Endpoint = "https://api.linear.app/graphql"

// maxPageSize is the largest first: value the API accepted in testing
// (2026-08-18: first: 250 returned 200). Values above it are clamped, not
// rejected, so a caller guessing "1000" still gets a working page size.
const maxPageSize = 250

// defaultPageSize keeps request complexity well under the complexity budget
// while making a page worth committing.
const defaultPageSize = 50

// ErrAuth is a rejected credential (HTTP 401 or 403). It deliberately does
// not unwrap to atlhttp.ErrAuth: that sentinel's identity is "Atlassian
// credential rejected", and importing the Atlassian transport package for a
// Linear client would couple this package to a host family it never talks to.
// A future sync wiring keys on errors.Is(err, linear.ErrAuth).
var ErrAuth = errors.New("linear: credential rejected")

// Client talks to one Linear workspace over GraphQL, read-only.
type Client struct {
	// APIKey is sent bare in the Authorization header — no "Bearer" prefix.
	// Linear rejects the prefixed form with a 400 that says so outright
	// (measured 2026-08-18), which is why the header shape is pinned by a
	// test.
	APIKey string

	// Endpoint is overridable so tests can point at an httptest server.
	// Production callers get the constant from New.
	Endpoint string

	HTTP *http.Client
	// Retries is the total number of attempts per request; Backoff is the
	// first wait, doubling per attempt and capped at 30 s.
	Retries int
	Backoff time.Duration

	// usage and rate are process-local observability; see Usage and
	// LastRateLimit. They never block a request.
	usage meter
	rate  atomic.Pointer[RateLimit]
}

// New builds a Client for a personal API key. The HTTP client times out at
// 60 s; Retries is 5; the first Backoff is 1 s — the same defaults
// internal/jira ships.
func New(apiKey string) *Client {
	c := &Client{
		APIKey:   apiKey,
		Endpoint: Endpoint,
		HTTP:     &http.Client{Timeout: 60 * time.Second},
		Retries:  5,
		Backoff:  time.Second,
	}
	return c
}

// IssueOpts scopes an Issues call. All fields are optional; the zero value
// pages every issue the credential can see, live ones only.
type IssueOpts struct {
	// TeamID restricts the page to one team (filter.team.id.eq). IDs come
	// from Teams; keys are display-facing and never a filter key.
	TeamID string
	// UpdatedAfter is an ISO-8601 timestamp and becomes filter.updatedAt.gte
	// — the incremental-sync watermark. Verified live: the gte and gt
	// comparators both exist on IssueFilter.updatedAt.
	UpdatedAfter string
	// IncludeArchived asks for archived rows too (archivedAt set), which a
	// reconcile pass needs so an archived issue is not misread as deleted.
	IncludeArchived bool
	// PageSize clamps to [1, maxPageSize]; 0 means defaultPageSize.
	PageSize int
}

// Viewer returns the authenticated user. It is the minimal credential check
// (Jira's /myself equivalent). The result carries personal data (name,
// email): log neither.
func (c *Client) Viewer(ctx context.Context) (User, error) {
	var page struct {
		Viewer User `json:"viewer"`
	}
	if err := c.gql(ctx, queryViewer, nil, &page); err != nil {
		return User{}, err
	}
	return page.Viewer, nil
}

// Teams lists every team the credential can see.
func (c *Client) Teams(ctx context.Context) ([]Team, error) {
	out := []Team{}
	after := ""
	for {
		vars := map[string]any{}
		if after != "" {
			vars["after"] = after
		}
		var page struct {
			Teams TeamConnection `json:"teams"`
		}
		if err := c.gql(ctx, queryTeams, vars, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Teams.Nodes...)
		if !page.Teams.PageInfo.HasNextPage || page.Teams.PageInfo.EndCursor == "" {
			return out, nil
		}
		after = page.Teams.PageInfo.EndCursor
	}
}

// WorkflowStates returns a team's status catalog. This is the id → type map
// the status_category mapping needs (MAPPING.md): state names are display
// text, types are the stable axis.
func (c *Client) WorkflowStates(ctx context.Context, teamID string) ([]WorkflowState, error) {
	out := []WorkflowState{}
	after := ""
	for {
		vars := map[string]any{
			"filter": map[string]any{
				"team": map[string]any{"id": map[string]any{"eq": teamID}},
			},
		}
		if after != "" {
			vars["after"] = after
		}
		var page struct {
			WorkflowStates WorkflowStateConnection `json:"workflowStates"`
		}
		if err := c.gql(ctx, queryWorkflowStates, vars, &page); err != nil {
			return nil, err
		}
		out = append(out, page.WorkflowStates.Nodes...)
		if !page.WorkflowStates.PageInfo.HasNextPage || page.WorkflowStates.PageInfo.EndCursor == "" {
			return out, nil
		}
		after = page.WorkflowStates.PageInfo.EndCursor
	}
}

// Issues pages issues oldest-updated-first and calls fn once per page, which
// is what lets a sync commit page by page — the same contract as
// jira.Client.Search. Pagination is cursor-based (pageInfo.endCursor);
// verified live that following the cursor returns exactly the next rows.
func (c *Client) Issues(ctx context.Context, opts IssueOpts, fn func([]Issue) error) error {
	filter := map[string]any{}
	if opts.UpdatedAfter != "" {
		filter["updatedAt"] = map[string]any{"gte": opts.UpdatedAfter}
	}
	if opts.TeamID != "" {
		filter["team"] = map[string]any{"id": map[string]any{"eq": opts.TeamID}}
	}

	size := opts.PageSize
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}

	after := ""
	for {
		vars := map[string]any{
			"first":           size,
			"filter":          filter,
			"includeArchived": opts.IncludeArchived,
		}
		if after != "" {
			vars["after"] = after
		}
		var page struct {
			Issues IssueConnection `json:"issues"`
		}
		if err := c.gql(ctx, queryIssues, vars, &page); err != nil {
			return err
		}
		if len(page.Issues.Nodes) > 0 {
			if err := fn(page.Issues.Nodes); err != nil {
				return err
			}
		}
		if !page.Issues.PageInfo.HasNextPage || page.Issues.PageInfo.EndCursor == "" {
			return nil
		}
		after = page.Issues.PageInfo.EndCursor
	}
}

// graphRequest is the GraphQL over HTTP envelope.
type graphRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphResponse is the reply envelope. GraphQL answers errors as HTTP 200
// with an errors array; those are surfaced as errors here.
type graphResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// gql sends one query and unmarshals data into out. Errors never contain the
// API key: the key travels only in the Authorization header, and response
// bodies (the only foreign text in an error) cannot echo a header they never
// saw.
func (c *Client) gql(ctx context.Context, query string, vars map[string]any, out any) error {
	payload, err := json.Marshal(graphRequest{Query: query, Variables: vars})
	if err != nil {
		return err
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, strings.NewReader(string(payload)))
		if err != nil {
			return err
		}
		// Bare key. "Authorization: Bearer <key>" is rejected by Linear with
		// a 400 telling you to remove the prefix — pinned by test.
		req.Header.Set("Authorization", c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		res, err := c.HTTP.Do(req)
		c.usage.noteRequest()
		if err != nil {
			if attempt < c.Retries-1 {
				if werr := c.wait(ctx, attempt, ""); werr != nil {
					return werr
				}
				c.usage.noteRetry()
				continue
			}
			return fmt.Errorf("POST /graphql: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, 64<<20))
		res.Body.Close()
		c.usage.noteStatus(res.StatusCode)
		c.noteRateLimit(res.Header)

		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
			// A rejected credential is final: retrying would burn the rate
			// budget for an answer that cannot change.
			return fmt.Errorf("POST /graphql: %w (%d %s)", ErrAuth, res.StatusCode, http.StatusText(res.StatusCode))
		}
		if isRetryable(res.StatusCode) && attempt < c.Retries-1 {
			if werr := c.wait(ctx, attempt, res.Header.Get("Retry-After")); werr != nil {
				return werr
			}
			c.usage.noteRetry()
			continue
		}
		if readErr != nil && res.StatusCode >= 200 && res.StatusCode < 300 {
			return fmt.Errorf("POST /graphql: %w", readErr)
		}
		if res.StatusCode >= 300 {
			return fmt.Errorf("POST /graphql: linear: %d %s: %s", res.StatusCode, http.StatusText(res.StatusCode), snippet(data))
		}

		var env graphResponse
		if err := json.Unmarshal(data, &env); err != nil {
			return fmt.Errorf("POST /graphql: bad JSON: %w", err)
		}
		if len(env.Errors) > 0 {
			msgs := make([]string, 0, len(env.Errors))
			for _, e := range env.Errors {
				msgs = append(msgs, e.Message)
			}
			return fmt.Errorf("POST /graphql: linear: graphql error: %s", snippet([]byte(strings.Join(msgs, "; "))))
		}
		if out == nil || len(env.Data) == 0 {
			return nil
		}
		return json.Unmarshal(env.Data, out)
	}
}

// isRetryable mirrors the read policy of internal/atlhttp (writes do not
// exist here): throttle and transient server errors are worth another
// attempt, everything else is an answer.
func isRetryable(code int) bool {
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

// snippet trims and truncates a response body for error messages — the same
// shape as atlhttp.Snippet, kept local because this package does not import
// atlhttp.
func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// Usage is a point-in-time snapshot of this process's outbound Linear HTTP
// traffic. Counters are process-local until a caller persists them; Requests
// counts every attempt including retries, which is the unit that draws from
// the rate budget.
type Usage struct {
	Requests      int64
	Retries       int64
	Throttled     int64     // 429 responses
	ServerErrors  int64     // 5xx responses, excluding 429
	LastThrottled time.Time // UTC; zero if never throttled
}

// Usage returns the current counters without resetting them.
func (c *Client) Usage() Usage { return c.usage.snapshot() }

// TakeUsage returns the counters and zeroes them so a flusher can accumulate
// without double-counting. LastThrottledAt is a timestamp, not a counter, and
// is not cleared.
func (c *Client) TakeUsage() Usage { return c.usage.take() }

// meter holds the atomic counters behind Usage.
type meter struct {
	requests     atomic.Int64
	retries      atomic.Int64
	throttled    atomic.Int64
	serverErrors atomic.Int64
	lastUnixNs   atomic.Int64
}

func (m *meter) snapshot() Usage {
	return Usage{
		Requests:      m.requests.Load(),
		Retries:       m.retries.Load(),
		Throttled:     m.throttled.Load(),
		ServerErrors:  m.serverErrors.Load(),
		LastThrottled: unixToTime(m.lastUnixNs.Load()),
	}
}

func (m *meter) take() Usage {
	u := Usage{
		Requests:      m.requests.Swap(0),
		Retries:       m.retries.Swap(0),
		Throttled:     m.throttled.Swap(0),
		ServerErrors:  m.serverErrors.Swap(0),
		LastThrottled: unixToTime(m.lastUnixNs.Load()), // not reset
	}
	return u
}

func (m *meter) noteRequest() { m.requests.Add(1) }

func (m *meter) noteRetry() { m.retries.Add(1) }

func (m *meter) noteStatus(code int) {
	switch {
	case code == 429:
		m.throttled.Add(1)
		m.lastUnixNs.Store(time.Now().UTC().UnixNano())
	case code >= 500 && code <= 599:
		m.serverErrors.Add(1)
	}
}

func unixToTime(ns int64) time.Time {
	if ns <= 0 {
		return time.Time{}
	}
	return time.Unix(0, ns).UTC()
}
