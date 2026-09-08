// Package jira is the Jira REST client for every origin that speaks the Jira
// REST shape: Atlassian Cloud (v3), Jira Server / Data Center (v2, GDK-1636),
// and the built-in tracker, which implements the Cloud v3 shape and is driven
// through the same client. Read paths plus user-initiated writes.
//
// The token lives only in the Authorization header. It is never put in an error,
// a log line or a URL (constitution article 8), which is why transport reports
// the method and path but never the request itself.
package jira

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/midagedev/gadak/internal/atlhttp"
	"github.com/midagedev/gadak/internal/httppolicy"
	"github.com/midagedev/gadak/internal/statuscat"
)

// The two REST dialects this package speaks. Which one a Client uses is
// decided by its constructor, not by a package constant: Cloud and the
// built-in tracker serve v3, Jira Server / Data Center serves v2 and has no
// v3 at all, and what it answers a v3 route with depends on the
// credential: measured on 11.3.11, the same route gave 404 to a valid
// basic login, 401 to a Cloud-shaped one, and 302 to a bearer PAT or to
// no credential at all. None of those means "no such API" on its own
// (GDK-1636).
const (
	apiV3 = "/rest/api/3"
	apiV2 = "/rest/api/2"
)

// ErrAuth is the Jira-named rejected credential. It unwraps to
// atlhttp.ErrAuth so Watch detects it without a per-source branch.
// Error() keeps the "jira:" prefix so last_error names the source.
// Callers keep using errors.Is(err, jira.ErrAuth).
var ErrAuth = atlhttp.Auth("jira")

// Client talks to one Jira REST origin over HTTP. The credential is held
// only as an Authorization header value; it is never copied into an error, a
// log line, or a URL. Retries and Backoff apply to reads; writes use a
// narrower policy (see write).
type Client struct {
	base string
	auth string
	// apiBase is the REST path prefix this origin serves: apiV3 for Cloud
	// and the built-in tracker, apiV2 for Jira Server / Data Center
	// (GDK-1636). The endpoints whose v2 shape differs by more than the
	// version number branch on serverDialect, inside this package, once.
	apiBase string

	HTTP *http.Client
	// Retries is the total number of attempts per request; Backoff is the first
	// wait, doubling per attempt and capped at 30 s.
	Retries int
	Backoff time.Duration

	// usage is process-local call volume; see Usage / TakeUsage. Never blocks
	// a request on instrumentation failure (counters are atomic).
	usage atlhttp.Meter

	// budget spaces requests proactively from Jira Data Center's
	// X-RateLimit-* budget headers (GDK-1646). Only NewServer allocates
	// one — the headers are a DC feature; Cloud publishes none and the
	// built-in tracker never rate-limits — so every other constructor
	// leaves it nil and its request path is unchanged byte for byte.
	budget *httppolicy.RateBudget

	// epicLinkID is the Server Epic Link field id, resolved once from the
	// field catalog by epicLinkField (GDK-1645); loaded is true after one
	// successful lookup, including "the site has none".
	epicLinkMu     sync.Mutex
	epicLinkID     string
	epicLinkLoaded bool

	// nameCreatedVersions is true when this origin mints a project version
	// from a fixVersions add {"name": token} (issuetap). Cloud Jira rejects
	// unknown names with 400; origin.transportJira enables it. GDK-678.
	nameCreatedVersions bool
}

// DefaultRetries is the production retry budget New applies (total attempts).
// Tests may assign a smaller value and restore it with t.Cleanup. The value
// is httppolicy.DefaultRetries; this var exists so tests can override it.
var DefaultRetries = httppolicy.DefaultRetries

// DefaultBackoff is the first wait New applies, doubling per attempt and
// capped at httppolicy.MaxWait. Tests may assign 0 and restore it with t.Cleanup.
var DefaultBackoff = httppolicy.DefaultBackoff

// DefaultCreatesVersionsByName is copied onto Client at New. Production is
// false (Cloud Jira). Tests that drive the issuetap mint-by-name path through
// a Connected httptest client set this and restore with t.Cleanup (same
// seam as DefaultRetries). origin.transportJira then enables it for real
// issuetap clients regardless of this default.
var DefaultCreatesVersionsByName = false

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
		base:                strings.TrimRight(site, "/"),
		auth:                "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token)),
		apiBase:             apiV3,
		HTTP:                &http.Client{Timeout: httppolicy.DefaultTimeout},
		Retries:             DefaultRetries,
		Backoff:             DefaultBackoff,
		nameCreatedVersions: DefaultCreatesVersionsByName,
	}
}

// NewServer builds a Client for a Jira Server / Data Center base URL using a
// Personal Access Token (GDK-1640). Server has no Atlassian API token and no
// email in the credential: the PAT goes in the Authorization header as a
// bearer, and there is nothing to pair it with.
//
// base may carry a context path ("http://host:2990/jira") — Server is
// commonly deployed under one, and atlhttp concatenates the site-relative
// path onto it.
func NewServer(base, token string) *Client {
	c := &Client{
		base:                strings.TrimRight(base, "/"),
		auth:                "Bearer " + token,
		apiBase:             apiV2,
		HTTP:                &http.Client{Timeout: httppolicy.DefaultTimeout},
		Retries:             DefaultRetries,
		Backoff:             DefaultBackoff,
		nameCreatedVersions: DefaultCreatesVersionsByName,
	}
	// The one constructor that throttles proactively (GDK-1646): DC states
	// its token bucket on every authenticated response, and the meter is
	// the client's own so budget waits show up in Usage like retry waits.
	c.budget = &httppolicy.RateBudget{Meter: &c.usage}
	return c
}

// NewAnonymous builds a credential-less Client. It exists for the one
// question that must be answerable before a credential is known to be
// good: which Jira this is (GDK-1635). Both deployments serve
// /rest/api/2/serverInfo to anonymous callers, and without this an
// authentication failure hides the deployment mismatch that caused it.
//
// Use it for nothing else: every other route needs auth, and a 401 from
// one of them says nothing about the credential the user typed.
func NewAnonymous(base string) *Client {
	return &Client{
		base:    strings.TrimRight(base, "/"),
		apiBase: apiV3,
		HTTP:    &http.Client{Timeout: httppolicy.DefaultTimeout},
		Retries: DefaultRetries,
		Backoff: DefaultBackoff,
	}
}

// serverDialect reports whether this client speaks Jira Server / Data
// Center REST, where several endpoints differ by more than the version
// number (GDK-1636). Derived from apiBase rather than kept as a second
// flag: only NewServer builds a v2 client, so the prefix is the dialect —
// and the branch stays at path construction instead of leaking to callers.
func (c *Client) serverDialect() bool { return c.apiBase == apiV2 }

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
		Budget:    c.budget,
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

// call is the JSON envelope over atlhttp.Call; the only Jira-specific half
// is how a non-2xx status maps to an error (apiError: the parsed
// errorMessages/errors document, falling back to the status line).
func (c *Client) call(ctx context.Context, method, path string, body, out any, mutating bool) error {
	return atlhttp.Call(ctx, c.transport(), method, path, body, out, mutating,
		func(status int, data []byte) error {
			statusLine := fmt.Sprintf("%d %s", status, http.StatusText(status))
			return apiError(method, path, status, statusLine, data)
		})
}

// snippet is kept for write.go error formatting; logic lives in atlhttp.
func snippet(b []byte) string { return atlhttp.Snippet(b) }

type searchPage struct {
	Issues        []Issue `json:"issues"`
	NextPageToken string  `json:"nextPageToken"`
	IsLast        bool    `json:"isLast"`
	// Total is Server's pagination field: its POST /search carries no
	// nextPageToken, so the walk advances by startAt until it reaches the
	// total (GDK-1636).
	Total int `json:"total"`
}

// searchPath is the JQL search route. Cloud (and the built-in tracker, which
// implements the Cloud shape) serves /search/jql; Jira Server has only the
// classic /search (GDK-1636).
func (c *Client) searchPath() string {
	if c.serverDialect() {
		return c.apiBase + "/search"
	}
	return c.apiBase + "/search/jql"
}

// Search pages a JQL query and calls fn once per page, which is what lets sync
// commit page by page. Pagination is by nextPageToken: the legacy startAt search
// is deprecated and drifts under concurrent writes. Server is the exception
// (GDK-1636): it never adopted tokens, so its pages walk by startAt/total with
// the same body.
func (c *Client) Search(ctx context.Context, jql string, fields []string, withChangelog bool, fn func([]Issue) error) error {
	token := ""
	for startAt := 0; ; {
		body := map[string]any{"jql": jql, "maxResults": 100, "fields": fields}
		if withChangelog {
			// v2 types SearchRequestBean.expand as a list; v3 takes the
			// comma-separated string. Sending Cloud's shape to Server is a
			// 400 naming ArrayList (measured on 11.3.11).
			if c.serverDialect() {
				body["expand"] = []string{"changelog"}
			} else {
				body["expand"] = "changelog"
			}
		}
		if c.serverDialect() {
			body["startAt"] = startAt
		} else if token != "" {
			body["nextPageToken"] = token
		}
		var page searchPage
		if err := c.do(ctx, http.MethodPost, c.searchPath(), body, &page); err != nil {
			return err
		}
		if len(page.Issues) > 0 {
			if err := fn(page.Issues); err != nil {
				return err
			}
		}
		if c.serverDialect() {
			startAt += len(page.Issues)
			if len(page.Issues) == 0 || startAt >= page.Total {
				return nil
			}
			continue
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
//
// Server has no approximate-count route (GDK-1636); its dialect reads the
// total of a zero-row /search instead. That is the Server answer, not a
// retry against another version.
func (c *Client) Count(ctx context.Context, jql string) (int, error) {
	if c.serverDialect() {
		var page struct {
			Total int `json:"total"`
		}
		if err := c.do(ctx, http.MethodPost, c.apiBase+"/search", map[string]any{"jql": jql, "maxResults": 0}, &page); err != nil {
			return 0, err
		}
		return page.Total, nil
	}
	var out struct {
		Count int `json:"count"`
	}
	if err := c.do(ctx, http.MethodPost, c.apiBase+"/search/approximate-count", map[string]any{"jql": jql}, &out); err != nil {
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
		p := fmt.Sprintf("%s/issue/%s/changelog?startAt=%d&maxResults=100", c.apiBase, url.PathEscape(key), startAt)
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
		p := fmt.Sprintf("%s/issue/%s/comment?startAt=%d&maxResults=100", c.apiBase, url.PathEscape(key), startAt)
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
	p := fmt.Sprintf("%s/issue/%s?fields=status,assignee", c.apiBase, url.PathEscape(key))
	if err := c.do(ctx, http.MethodGet, p, nil, &out); err != nil {
		return Status{}, nil, err
	}
	return out.Fields.Status, out.Fields.Assignee, nil
}

// Statuses maps every status id on the site to its category. This is the input
// the derived-field rules need, because a changelog entry carries ids only.
func (c *Client) Statuses(ctx context.Context) (map[string]string, error) {
	var list []Status
	if err := c.do(ctx, http.MethodGet, c.apiBase+"/status", nil, &list); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(list))
	for _, s := range list {
		out[s.ID] = statuscat.Category(s.StatusCategory.Key)
	}
	return out, nil
}

// PriorityCatalog is the site's priority list, most urgent first. Names are in
// the account language; writes should send the id.
func (c *Client) PriorityCatalog(ctx context.Context) ([]NamedID, error) {
	var list []NamedID
	return list, c.do(ctx, http.MethodGet, c.apiBase+"/priority", nil, &list)
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

// FieldInfo is one row from GET /field — the site-wide field catalog.
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
	if err := c.do(ctx, http.MethodGet, c.apiBase+"/field", nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// Stream is Raw for bytes: the response body is handed back unread so an
// attachment never has to fit in memory. The caller closes it. See
// atlhttp.Stream — hdr passes Range and conditional headers through, and a
// non-2xx status comes back as a response, not an error.
func (c *Client) Stream(ctx context.Context, method, path string, hdr http.Header) (*http.Response, error) {
	return atlhttp.Stream(ctx, c.transport(), method, path, hdr)
}
