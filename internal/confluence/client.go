// Package confluence is a thin REST client for Confluence Cloud: enough of the
// API to fill the page mirror, and nothing else. It never writes to Confluence.
//
// The token lives only in the Authorization header. It is never put in an error,
// a log line or a URL (constitution article 8).
package confluence

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

// ErrAuth aborts a run immediately: retrying a bad credential just burns the
// rate budget.
var ErrAuth = errors.New("confluence: credential rejected")

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
}

// New builds a client. site is the Atlassian origin (no /wiki suffix).
func New(site, email, token string) *Client {
	return &Client{
		base:         strings.TrimRight(site, "/") + "/wiki",
		auth:         "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token)),
		HTTP:         &http.Client{Timeout: 60 * time.Second},
		Retries:      5,
		Backoff:      time.Second,
		PauseBetween: 100 * time.Millisecond,
	}
}

// BaseURL is the wiki origin (…/wiki), used to build deep links.
func (c *Client) BaseURL() string { return c.base }

// SiteURL is the Atlassian origin without /wiki.
func (c *Client) SiteURL() string {
	return strings.TrimSuffix(c.base, "/wiki")
}

// Space is one space listing row.
type Space struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// User is a Confluence account reference on version.by.
type User struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
}

// Version is the content version stamp.
type Version struct {
	Number int    `json:"number"`
	When   string `json:"when"`
	By     User   `json:"by"`
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
func (c *Client) Spaces(ctx context.Context) ([]Space, error) {
	out := []Space{}
	for start := 0; ; {
		var page struct {
			Results []Space `json:"results"`
			Size    int     `json:"size"`
			Limit   int     `json:"limit"`
			Start   int     `json:"start"`
		}
		p := fmt.Sprintf("%s/space?limit=100&start=%d", apiPath, start)
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
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
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
			if attempt < c.Retries-1 {
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
		case retryable(res.StatusCode) && attempt < c.Retries-1:
			if werr := c.wait(ctx, attempt, res.Header.Get("Retry-After")); werr != nil {
				return werr
			}
			continue
		case res.StatusCode >= 300:
			return fmt.Errorf("%s %s: %s: %s", method, path, res.Status, snippet(data))
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
