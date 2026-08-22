// Package confluence is a thin REST client for Confluence Cloud: enough of the
// API to fill the page mirror, plus the user-initiated page writes that go
// through the origin (create / update). It never writes to gadak's SQLite
// mirror — after a write, callers re-read from the origin (and, on a later
// surface, SyncPage the same way SyncIssue follows a Jira write).
//
// The token lives only in the Authorization header. It is never put in an error,
// a log line or a URL (constitution article 8).
package confluence

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/atlhttp"
	"github.com/midagedev/gadak/internal/httppolicy"
)

// Label is one entry under metadata.labels.results (Confluence REST v1).
type Label struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
	ID     string `json:"id"`
}

// LabelsPage is the expanded metadata.labels object. expand=metadata.labels
// returns only the first page (default limit 25). Real wiki pages almost always
// have single-digit labels, so paging is intentionally not followed.
type LabelsPage struct {
	Results []Label `json:"results"`
	Size    int     `json:"size"`
	Limit   int     `json:"limit"`
	Start   int     `json:"start"`
}

// PageMetadata is the expand=metadata.* payload on a content row.
type PageMetadata struct {
	Labels LabelsPage `json:"labels"`
}

const apiPath = "/rest/api"

// ErrNotFound marks a 404 on a specific content id. A page can vanish (or get
// view-restricted) between the CQL listing and the per-page fetch on a busy
// site; callers skip that page instead of aborting the run.
var ErrNotFound = errors.New("confluence: content not found")

// ErrAuth is the Confluence-named rejected credential. It unwraps to
// atlhttp.ErrAuth so Watch detects it without a per-source branch.
// Error() keeps the "confluence:" prefix so last_error names the source.
// Callers keep using errors.Is(err, confluence.ErrAuth).
var ErrAuth = atlhttp.Auth("confluence")

// APIError is a non-2xx Confluence answer with its HTTP status. Callers
// distinguish a stale version (409) from other rejections with Status or
// errors.Is(err, ErrConflict) — not by matching a human-readable substring.
// The shape follows jira.APIError.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	if e == nil {
		return "confluence: <nil>"
	}
	if e.Body != "" {
		return fmt.Sprintf("confluence: %d: %s", e.Status, e.Body)
	}
	return fmt.Sprintf("confluence: %d", e.Status)
}

// Is reports whether target is an APIError with the same Status. That is
// how ErrConflict matches any 409, including a wrapped one.
func (e *APIError) Is(target error) bool {
	t, ok := target.(*APIError)
	return ok && e != nil && t != nil && e.Status == t.Status
}

// ErrConflict is a 409. On update, that is a stale version.number: the
// caller refetches and decides. This client does not retry a 409.
var ErrConflict = &APIError{Status: http.StatusConflict}

// Client talks to Confluence Cloud under <site>/wiki.
type Client struct {
	base string // e.g. https://example.atlassian.net/wiki
	auth string

	HTTP *http.Client
	// Retries is the total number of attempts per request; Backoff is the first
	// wait, doubling per attempt and capped at 30 s.
	Retries int
	Backoff time.Duration
	// PauseBetween is slept after each Page fetch (rate politeness). Zero in tests.
	PauseBetween time.Duration

	// usage is process-local call volume; see Usage / TakeUsage. Never blocks
	// a request on instrumentation failure (counters are atomic).
	usage atlhttp.Meter
}

// New builds a client. site is the Atlassian origin (no /wiki suffix).
func New(site, email, token string) *Client {
	return &Client{
		base:         strings.TrimRight(site, "/") + "/wiki",
		auth:         "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token)),
		HTTP:         &http.Client{Timeout: httppolicy.DefaultTimeout},
		Retries:      httppolicy.DefaultRetries,
		Backoff:      httppolicy.DefaultBackoff,
		PauseBetween: 100 * time.Millisecond,
	}
}

// BaseURL is the wiki origin (…/wiki), used to build deep links.
func (c *Client) BaseURL() string { return c.base }

// SiteURL is the Atlassian origin without /wiki.
func (c *Client) SiteURL() string {
	return strings.TrimSuffix(c.base, "/wiki")
}

func (c *Client) transport() atlhttp.Config {
	return atlhttp.Config{
		Base:      c.base,
		Auth:      c.auth,
		HTTP:      c.HTTP,
		Retries:   c.Retries,
		Backoff:   c.Backoff,
		ErrPrefix: "confluence",
		Usage:     &c.usage,
	}
}

// Space is one space listing row. Homepage is present when expand=homepage
// was requested; its id is the content id of the space's root page (same
// id scheme as Page.ID / pages.parent_id).
type Space struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Homepage *struct {
		ID string `json:"id"`
	} `json:"homepage"`
}

// User is a Confluence account reference on version.by.
type User struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
}

// Version is the content version stamp. Message and MinorEdit are filled on
// GET /content/{id}/version rows; the expand=version object on a Page fetch
// typically omits them.
type Version struct {
	Number    int    `json:"number"`
	When      string `json:"when"`
	Message   string `json:"message"`
	MinorEdit bool   `json:"minorEdit"`
	By        User   `json:"by"`
}

// SpaceRef is the embedded space on a content row.
type SpaceRef struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// BodyADF holds body.atlas_doc_format; Value is the ADF document as a JSON string.
type BodyADF struct {
	Value          string `json:"value"`
	Representation string `json:"representation"`
}

// ContentBody is the expand=body.atlas_doc_format payload.
type ContentBody struct {
	AtlasDocFormat *BodyADF `json:"atlas_doc_format"`
}

// Page is a content row (search hit or full fetch).
type Page struct {
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	Status  string      `json:"status"`
	Title   string      `json:"title"`
	Space   SpaceRef    `json:"space"`
	Version Version     `json:"version"`
	Body    ContentBody `json:"body"`
	// Metadata is present when expand includes metadata.labels (full Page fetch).
	Metadata PageMetadata `json:"metadata"`
	// Ancestors, when expanded, lists the parent chain; the last entry is the direct parent.
	Ancestors []struct {
		ID string `json:"id"`
	} `json:"ancestors"`
	Links struct {
		WebUI string `json:"webui"`
	} `json:"_links"`
}

// LabelNames returns label names from metadata.labels.results in API order.
// Always returns a non-nil empty slice when none are present. Does not sort —
// callers that need determinism (sync → store) sort themselves.
//
// Limit: expand=metadata.labels only includes the first results page (≤25).
// Pages with more labels than that will be truncated; paging is not followed
// because production pages typically have single-digit labels.
func (p Page) LabelNames() []string {
	out := make([]string, 0, len(p.Metadata.Labels.Results))
	for _, l := range p.Metadata.Labels.Results {
		if l.Name == "" {
			continue
		}
		out = append(out, l.Name)
	}
	return out
}

// Comment is a child comment (or reply) on a page.
type Comment struct {
	ID      string      `json:"id"`
	Title   string      `json:"title"`
	Body    ContentBody `json:"body"`
	Version Version     `json:"version"`
}

// ADFRaw returns the atlas_doc_format value as json.RawMessage for storage/FTS.
// Value is a JSON string of the ADF document.
func (b ContentBody) ADFRaw() json.RawMessage {
	if b.AtlasDocFormat == nil || b.AtlasDocFormat.Value == "" {
		return nil
	}
	v := strings.TrimSpace(b.AtlasDocFormat.Value)
	// Value is typically a JSON-encoded ADF object string.
	if json.Valid([]byte(v)) {
		return json.RawMessage(v)
	}
	// Rare: already double-encoded or plain text — store as a JSON string.
	enc, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return enc
}

// Spaces lists every space the credential can see (start/limit paging).
// expand=homepage fills Space.Homepage so callers can store the root page id.
func (c *Client) Spaces(ctx context.Context) ([]Space, error) {
	out := []Space{}
	for start := 0; ; {
		var page struct {
			Results []Space `json:"results"`
			Size    int     `json:"size"`
			Limit   int     `json:"limit"`
			Start   int     `json:"start"`
		}
		p := fmt.Sprintf("%s/space?limit=100&start=%d&expand=homepage", apiPath, start)
		if err := c.do(ctx, http.MethodGet, p, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Results...)
		if len(page.Results) == 0 || len(page.Results) < 100 {
			return out, nil
		}
		start += len(page.Results)
	}
}

// Space fetches one space by key with expand=homepage. 404 wraps ErrNotFound
// (same as Page); callers that tolerate a missing/restricted space skip it.
func (c *Client) Space(ctx context.Context, key string) (Space, error) {
	var out Space
	p := fmt.Sprintf("%s/space/%s?expand=homepage", apiPath, url.PathEscape(key))
	if err := c.do(ctx, http.MethodGet, p, nil, &out); err != nil {
		return Space{}, err
	}
	return out, nil
}

// SearchPages runs a CQL search with expand=version,space and follows _links.next.
// fn is called once per page of results (may be empty only on the final empty page).
func (c *Client) SearchPages(ctx context.Context, cql string, fn func([]Page) error) error {
	path := fmt.Sprintf("%s/content/search?cql=%s&limit=50&expand=version,space",
		apiPath, url.QueryEscape(cql))
	for path != "" {
		var page struct {
			Results []Page `json:"results"`
			Links   struct {
				Next string `json:"next"`
			} `json:"_links"`
		}
		if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
			return err
		}
		if len(page.Results) > 0 {
			if err := fn(page.Results); err != nil {
				return err
			}
		}
		path = c.nextPath(page.Links.Next)
	}
	return nil
}

// Page fetches one content id with ADF body, version, space, ancestors, and
// labels. metadata.labels is the first page only (≤25 results); see LabelNames.
func (c *Client) Page(ctx context.Context, id string) (Page, error) {
	var out Page
	p := fmt.Sprintf("%s/content/%s?expand=body.atlas_doc_format,version,space,ancestors,metadata.labels",
		apiPath, url.PathEscape(id))
	if err := c.do(ctx, http.MethodGet, p, nil, &out); err != nil {
		return Page{}, err
	}
	if c.PauseBetween > 0 {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(c.PauseBetween):
		}
	}
	return out, nil
}

// PageVersions lists every history stamp for a content id
// (GET /content/{id}/version). Bodies are not requested. Pagination follows
// _links.next the same way SearchPages does. The server's order is not
// trusted: results are sorted by Number ascending before return.
func (c *Client) PageVersions(ctx context.Context, id string) ([]Version, error) {
	var out []Version
	path := fmt.Sprintf("%s/content/%s/version?limit=100", apiPath, url.PathEscape(id))
	for path != "" {
		var page struct {
			Results []Version `json:"results"`
			Links   struct {
				Next string `json:"next"`
			} `json:"_links"`
		}
		if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Results...)
		path = c.nextPath(page.Links.Next)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

// Comments returns every comment on a page plus one level of replies
// (start/limit paging on each parent).
func (c *Client) Comments(ctx context.Context, pageID string) ([]Comment, error) {
	top, err := c.childComments(ctx, pageID)
	if err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(top))
	for _, cm := range top {
		out = append(out, cm)
		replies, err := c.childComments(ctx, cm.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, replies...)
	}
	return out, nil
}

func (c *Client) childComments(ctx context.Context, contentID string) ([]Comment, error) {
	out := []Comment{}
	for start := 0; ; {
		var page struct {
			Results []Comment `json:"results"`
			Size    int       `json:"size"`
			Limit   int       `json:"limit"`
		}
		p := fmt.Sprintf("%s/content/%s/child/comment?expand=body.atlas_doc_format,version&limit=100&start=%d",
			apiPath, url.PathEscape(contentID), start)
		if err := c.do(ctx, http.MethodGet, p, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Results...)
		if len(page.Results) == 0 || len(page.Results) < 100 {
			return out, nil
		}
		start += len(page.Results)
	}
}

// nextPath turns a _links.next value into a path relative to c.base.
// Confluence returns host-absolute paths (/wiki/rest/…) or full URLs.
func (c *Client) nextPath(next string) string {
	if next == "" {
		return ""
	}
	if strings.HasPrefix(next, "http://") || strings.HasPrefix(next, "https://") {
		u, err := url.Parse(next)
		if err != nil {
			return ""
		}
		next = u.Path
		if u.RawQuery != "" {
			next += "?" + u.RawQuery
		}
	}
	// Strip /wiki prefix when present so base+path is correct.
	if strings.HasPrefix(next, "/wiki/") {
		return strings.TrimPrefix(next, "/wiki")
	}
	if strings.HasPrefix(next, "wiki/") {
		return "/" + strings.TrimPrefix(next, "wiki")
	}
	if !strings.HasPrefix(next, "/") {
		next = "/" + next
	}
	return next
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	return c.call(ctx, method, path, body, out, false)
}

// write is do() with the retry policy a state-changing request needs. A 500
// may mean Confluence already acted, so only 429 and 503 are retried. A 409
// (stale version) is never retried — it is returned as ErrConflict.
func (c *Client) write(ctx context.Context, method, path string, body, out any) error {
	return c.call(ctx, method, path, body, out, true)
}

// CreatePage POSTs a page to the origin. ADF is the atlas_doc_format value
// as a JSON string (the same representation the read path already handles).
// parentID, when non-empty, is sent as ancestors[0].id.
//
// The returned page is a GET of the new id, not the request body and not
// the POST envelope — the origin is the record.
func (c *Client) CreatePage(ctx context.Context, spaceKey, title, adf string, parentID string) (Page, error) {
	body := map[string]any{
		"type":  "page",
		"title": title,
		"space": map[string]any{"key": spaceKey},
		"body": map[string]any{
			"atlas_doc_format": map[string]any{
				"value":          adf,
				"representation": "atlas_doc_format",
			},
		},
	}
	if parentID != "" {
		body["ancestors"] = []map[string]string{{"id": parentID}}
	}
	var created Page
	if err := c.write(ctx, http.MethodPost, apiPath+"/content", body, &created); err != nil {
		return Page{}, err
	}
	if created.ID == "" {
		return created, nil
	}
	got, err := c.Page(ctx, created.ID)
	if err != nil {
		return created, fmt.Errorf("confluence: create landed, read failed: %w", err)
	}
	return got, nil
}

// UpdatePage PUTs a page. nextVersion must be the origin's current
// version.number + 1. A stale number is a 409 / ErrConflict — the caller
// refetches and decides; this method does not retry.
//
// The returned page is a GET after the write, same as CreatePage.
func (c *Client) UpdatePage(ctx context.Context, id, title, adf string, nextVersion int) (Page, error) {
	body := map[string]any{
		"type":  "page",
		"title": title,
		"version": map[string]any{
			"number": nextVersion,
		},
		"body": map[string]any{
			"atlas_doc_format": map[string]any{
				"value":          adf,
				"representation": "atlas_doc_format",
			},
		},
	}
	p := fmt.Sprintf("%s/content/%s", apiPath, url.PathEscape(id))
	var written Page
	if err := c.write(ctx, http.MethodPut, p, body, &written); err != nil {
		return Page{}, err
	}
	got, err := c.Page(ctx, id)
	if err != nil {
		return written, fmt.Errorf("confluence: update landed, read failed: %w", err)
	}
	return got, nil
}

// AddPageComment POSTs a top-level comment on a page (v1 content with
// type=comment and the page as container). ADF is the atlas_doc_format
// value as a JSON string, same as CreatePage.
func (c *Client) AddPageComment(ctx context.Context, pageID, adf string) (Comment, error) {
	body := map[string]any{
		"type":      "comment",
		"container": map[string]any{"id": pageID, "type": "page"},
		"body": map[string]any{
			"atlas_doc_format": map[string]any{
				"value":          adf,
				"representation": "atlas_doc_format",
			},
		},
	}
	var created Comment
	if err := c.write(ctx, http.MethodPost, apiPath+"/content", body, &created); err != nil {
		return Comment{}, err
	}
	return created, nil
}

// Raw sends a request and returns the HTTP status and response body without
// JSON decoding. Path is relative to the wiki origin (e.g. /rest/api/… or
// /api/v2/… — not the /wiki prefix). Absolute URLs are rejected. mutating
// selects the write retry policy (429/503 only).
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
	statusLine := fmt.Sprintf("%d %s", status, http.StatusText(status))
	switch {
	case status == http.StatusNotFound:
		return fmt.Errorf("%s %s: %w: %s", method, path, ErrNotFound, atlhttp.Snippet(data))
	case status >= 300:
		body := atlhttp.Snippet(data)
		if body == "" {
			body = statusLine
		}
		return fmt.Errorf("%s %s: %w", method, path, &APIError{Status: status, Body: body})
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
