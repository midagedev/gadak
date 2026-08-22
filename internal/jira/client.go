// Package jira is the Atlassian Cloud REST client: read paths plus
// user-initiated writes.
//
// The token lives only in the Authorization header. It is never put in an error,
// a log line or a URL (constitution article 8), which is why transport reports
// the method and path but never the request itself.
package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/atlhttp"
	"github.com/midagedev/gadak/internal/httppolicy"
)

const apiPath = "/rest/api/3"

// ErrAuth is the Jira-named rejected credential. It unwraps to
// atlhttp.ErrAuth so Watch detects it without a per-source branch.
// Error() keeps the "jira:" prefix so last_error names the source.
// Callers keep using errors.Is(err, jira.ErrAuth).
var ErrAuth = atlhttp.Auth("jira")

// Client talks to one Atlassian Cloud site over REST. The credential is held
// only as an Authorization header value; it is never copied into an error, a
// log line, or a URL. Retries and Backoff apply to reads; writes use a
// narrower policy (see write).
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
	usage atlhttp.Meter
}

// DefaultRetries is the production retry budget New applies (total attempts).
// Tests may assign a smaller value and restore it with t.Cleanup. The value
// is httppolicy.DefaultRetries; this var exists so tests can override it.
var DefaultRetries = httppolicy.DefaultRetries

// DefaultBackoff is the first wait New applies, doubling per attempt and
// capped at httppolicy.MaxWait. Tests may assign 0 and restore it with t.Cleanup.
var DefaultBackoff = httppolicy.DefaultBackoff

// New builds a Client for site using Basic auth (email:token). The HTTP
// client times out at httppolicy.DefaultTimeout; Retries is DefaultRetries;
// the first Backoff is DefaultBackoff.
//
// origin.Client is the only production construction path for this
// workspace's Jira client. origin.Connected is the candidate-credential
// sibling; both live in internal/origin and call New. Tests may call New
// to stand up httptest servers. internal/origin/direct_new_gate_test.go
// fails if a new production call site appears outside that package.
func New(site, email, token string) *Client {
	return &Client{
		base:    strings.TrimRight(site, "/"),
		auth:    "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token)),
		HTTP:    &http.Client{Timeout: httppolicy.DefaultTimeout},
		Retries: DefaultRetries,
		Backoff: DefaultBackoff,
	}
}

// BaseURL is the site origin, used to build deep links.
func (c *Client) BaseURL() string { return c.base }

func (c *Client) transport() atlhttp.Config {
	return atlhttp.Config{
		Base:      c.base,
		Auth:      c.auth,
		HTTP:      c.HTTP,
		Retries:   c.Retries,
		Backoff:   c.Backoff,
		ErrPrefix: "jira",
		Usage:     &c.usage,
	}
}

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
	return atlhttp.DoRaw(ctx, c.transport(), method, path, body, len(body) > 0, mutating)
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
	status, data, err := atlhttp.Do(ctx, c.transport(), method, path, payload, hasBody, mutating)
	if err != nil {
		return err
	}
	if status >= 300 {
		statusLine := fmt.Sprintf("%d %s", status, http.StatusText(status))
		return apiError(method, path, status, statusLine, data)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// snippet is kept for write.go error formatting; logic lives in atlhttp.
func snippet(b []byte) string { return atlhttp.Snippet(b) }

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

// IssueStatus is GET /issue/{key}?fields=status,assignee — the two facts a
// claim must judge locally on an origin with no atomic claim route (Cloud):
// is the issue in progress, and who holds it. Nothing else is fetched. The
// answer nests under "fields" like every GET /issue/{key} does.
func (c *Client) IssueStatus(ctx context.Context, key string) (Status, *User, error) {
	var out struct {
		Fields struct {
			Status   Status `json:"status"`
			Assignee *User  `json:"assignee"`
		} `json:"fields"`
	}
	p := fmt.Sprintf("%s/issue/%s?fields=status,assignee", apiPath, url.PathEscape(key))
	if err := c.do(ctx, http.MethodGet, p, nil, &out); err != nil {
		return Status{}, nil, err
	}
	return out.Fields.Status, out.Fields.Assignee, nil
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

// PriorityCatalog is the site's priority list, most urgent first. Names are in
// the account language; writes should send the id.
func (c *Client) PriorityCatalog(ctx context.Context) ([]NamedID, error) {
	var list []NamedID
	return list, c.do(ctx, http.MethodGet, apiPath+"/priority", nil, &list)
}

// Priorities returns the site's priority names, most urgent first, which is the
// order priority_rank counts from.
func (c *Client) Priorities(ctx context.Context) ([]string, error) {
	list, err := c.PriorityCatalog(ctx)
	if err != nil {
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
