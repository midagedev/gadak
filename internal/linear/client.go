// Package linear is the GraphQL client for a Linear workspace: viewer, teams,
// workflow states, cursor-paged issues with an updatedAt watermark filter, and
// — since GDK-360 — the three write verbs (create issue, update issue,
// comment) the origin.Writer adapter routes through. The constitution article
// is unchanged: writes pass through the origin, and these methods are that
// origin path for Linear; the mirror itself is still never written directly.
//
// The API key lives only in the Authorization header. It is never put in an
// error, a log line, or a URL — the same article-8 discipline internal/jira
// follows. This package contains no logging at all.
//
// Why not internal/atlhttp: that package is the shared transport for
// *Atlassian Cloud* clients, and its headline guarantee is path safety —
// joining a caller-supplied path onto a configured site so the Authorization
// header cannot wander off-host. Linear has one fixed endpoint and no path
// parameter, so that machinery has nothing to do. Retry/backoff, Retry-After,
// the 64 MiB response cap, and the usage counters come from httppolicy — the
// host-neutral owner — not a copy. Rate-limit visibility stays here: Linear
// states its budget in response headers (x-ratelimit-*), which Atlassian
// never did. The API key is sent bare (no Bearer prefix).
package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/midagedev/gadak/internal/httppolicy"
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

// authError is a rejected Linear credential (HTTP 401 or 403). Error() keeps
// the "linear:" prefix so last_error names the source. RejectedCredential
// satisfies the duck-typed marker IsRejectedCredential already publishes, so
// a future Watch wiring is detected without a per-source branch and without
// importing atlhttp (this package is not an Atlassian client; see the
// package comment).
//
// A comparable value type plus the package-level ErrAuth var keeps
// errors.Is(err, linear.ErrAuth) working for existing and future callers.
type authError struct{}

func (authError) Error() string       { return "linear: credential rejected" }
func (authError) RejectedCredential() {}

// ErrAuth is the Linear-named rejected credential. Callers keep using
// errors.Is(err, linear.ErrAuth). It deliberately does not unwrap to
// atlhttp.ErrAuth: that sentinel's identity is "Atlassian credential
// rejected", and importing the Atlassian transport package for a Linear
// client would couple this package to a host family it never talks to.
var ErrAuth error = authError{}

// Client talks to one Linear workspace over GraphQL.
type Client struct {
	// apiKey is sent bare in the Authorization header — no "Bearer" prefix.
	// Linear rejects the prefixed form with a 400 that says so outright
	// (measured 2026-08-18), which is why the header shape is pinned by a
	// test. Assigned only by New.
	apiKey string

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
	usage httppolicy.Meter
	rate  atomic.Pointer[RateLimit]
}

// New builds a Client for a personal API key. HTTP timeout, Retries, and
// Backoff come from httppolicy (same owner as jira and confluence New).
func New(apiKey string) *Client {
	c := &Client{
		apiKey:   apiKey,
		Endpoint: Endpoint,
		HTTP:     &http.Client{Timeout: httppolicy.DefaultTimeout},
		Retries:  httppolicy.DefaultRetries,
		Backoff:  httppolicy.DefaultBackoff,
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
// with an errors array; Linear also uses HTTP 400 plus the same envelope
// for rate limits (errors[].extensions.code == "RATELIMITED").
type graphResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string `json:"message"`
		Extensions struct {
			Code string `json:"code"`
		} `json:"extensions"`
	} `json:"errors"`
}

// retryableGraphQL is the single retry decision for every GraphQL response.
// Status alone is not enough: Linear rate-limits with HTTP 400 or 200 plus
// errors[].extensions.code == "RATELIMITED" (documented at
// https://linear.app/developers/rate-limiting, "Handling rate limit
// errors"). Other GraphQL codes stay final — this is not a new
// infinite-retry class.
//
// RATELIMITED is a pre-execution rejection: Linear does not run the
// document once the bucket is empty, the same class as 429/503 (the server
// states it did not act). Mutations therefore retry it too. A 500 on a
// mutation still does not retry.
func retryableGraphQL(status int, env graphResponse, mutating bool) bool {
	if graphQLRateLimited(env) {
		return true
	}
	if mutating {
		return httppolicy.IsRetryableWrite(status)
	}
	return httppolicy.IsRetryable(status)
}

func graphQLRateLimited(env graphResponse) bool {
	for _, e := range env.Errors {
		if e.Extensions.Code == "RATELIMITED" {
			return true
		}
	}
	return false
}

func graphQLErrorMessages(env graphResponse) string {
	msgs := make([]string, 0, len(env.Errors))
	for _, e := range env.Errors {
		msgs = append(msgs, e.Message)
	}
	return strings.Join(msgs, "; ")
}

// gql sends one query and unmarshals data into out. Errors never contain the
// API key: the key travels only in the Authorization header, and response
// bodies (the only foreign text in an error) cannot echo a header they never
// saw.
func (c *Client) gql(ctx context.Context, query string, vars map[string]any, out any) error {
	return c.gqlCall(ctx, query, vars, out, false)
}

// gqlWrite is gql with the retry policy a state-changing request needs — the
// same discipline jira's write() applies: a 500 or a dropped connection may
// mean Linear already acted and the answer was lost, so a retry could create
// the issue twice. Only 429 and 503, where the server states it did not act,
// are retried (httppolicy.IsRetryableWrite), plus GraphQL RATELIMITED — Linear
// documents that as a pre-execution rejection (the mutation does not run).
// Transport failures stay final. Everything else — auth header, usage
// counters, rate-limit notes — is shared with reads, because both are the
// same gqlCall.
func (c *Client) gqlWrite(ctx context.Context, query string, vars map[string]any, out any) error {
	return c.gqlCall(ctx, query, vars, out, true)
}

// gqlCall is the one HTTP path every GraphQL document takes, read or
// mutation; mutating only selects the retry policy.
func (c *Client) gqlCall(ctx context.Context, query string, vars map[string]any, out any, mutating bool) error {
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
		req.Header.Set("Authorization", c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		res, err := c.HTTP.Do(req)
		c.usage.NoteRequest()
		if err != nil {
			// A mutating request is not retried on a transport failure
			// either: the request may have reached Linear and only the reply
			// was lost, so a retry could apply the write twice.
			if !mutating && attempt < c.Retries-1 {
				if werr := httppolicy.Wait(ctx, c.Backoff, attempt, "", &c.usage); werr != nil {
					return werr
				}
				c.usage.NoteRetry()
				continue
			}
			return fmt.Errorf("POST /graphql: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, httppolicy.MaxBody))
		res.Body.Close()
		c.usage.NoteStatus(res.StatusCode)
		c.noteRateLimit(res.Header)

		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
			// A rejected credential is final: retrying would burn the rate
			// budget for an answer that cannot change.
			return fmt.Errorf("POST /graphql: %w (%d %s)", ErrAuth, res.StatusCode, http.StatusText(res.StatusCode))
		}
		// Parse the envelope on every status so RATELIMITED inside a 400
		// body is visible to retryableGraphQL. A failed parse leaves env
		// empty and the decision falls back to the HTTP status.
		var env graphResponse
		envErr := json.Unmarshal(data, &env)
		// Wait reuses httppolicy's Retry-After path. Linear documents
		// x-ratelimit-*-reset as leaky-bucket window-end stamps (epoch-ms),
		// not a retry delay — converting those to Retry-After would stall
		// until the hour rolls over. Live presence of Retry-After on
		// RATELIMITED is unverified here (httptest only).
		if retryableGraphQL(res.StatusCode, env, mutating) && attempt < c.Retries-1 {
			if werr := httppolicy.Wait(ctx, c.Backoff, attempt, res.Header.Get("Retry-After"), &c.usage); werr != nil {
				return werr
			}
			c.usage.NoteRetry()
			continue
		}
		if readErr != nil && res.StatusCode >= 200 && res.StatusCode < 300 {
			return fmt.Errorf("POST /graphql: %w", readErr)
		}
		// GraphQL errors — including HTTP 400 + RATELIMITED after the
		// budget is spent — surface as messages only. The raw body is
		// not copied into the error: extensions can carry arbitrary
		// fields and must not land in last_error (article 8).
		if envErr == nil && len(env.Errors) > 0 {
			return fmt.Errorf("POST /graphql: linear: graphql error: %s", httppolicy.Snippet([]byte(graphQLErrorMessages(env))))
		}
		if res.StatusCode >= 300 {
			return fmt.Errorf("POST /graphql: linear: %d %s: %s", res.StatusCode, http.StatusText(res.StatusCode), httppolicy.Snippet(data))
		}
		if envErr != nil {
			return fmt.Errorf("POST /graphql: bad JSON: %w", envErr)
		}
		if out == nil || len(env.Data) == 0 {
			return nil
		}
		return json.Unmarshal(env.Data, out)
	}
}

// Usage is the host-neutral HTTP usage snapshot (httppolicy.Usage). The
// same type as atlhttp.Usage / jira.Usage / confluence.Usage, so
// TakeUsage satisfies sync.usageTaker without a conversion.
type Usage = httppolicy.Usage

// Usage returns the current counters without resetting them.
func (c *Client) Usage() Usage { return c.usage.Snapshot() }

// TakeUsage returns the counters and zeroes the numeric fields so a
// flusher can accumulate without double-counting.
//
// LastThrottledAt is a timestamp, not a counter, and is not cleared.
func (c *Client) TakeUsage() Usage { return c.usage.Take() }

// Issue fetches one issue by UUID or human identifier ("MID-5") — issue(id:)
// accepts both. The write adapter resolves user-typed keys through this.
func (c *Client) Issue(ctx context.Context, idOrIdentifier string) (Issue, error) {
	var res struct {
		Issue *Issue `json:"issue"`
	}
	if err := c.gql(ctx, queryIssue, map[string]any{"id": idOrIdentifier}, &res); err != nil {
		return Issue{}, err
	}
	if res.Issue == nil {
		return Issue{}, fmt.Errorf("linear: issue %q not found", idOrIdentifier)
	}
	return *res.Issue, nil
}

// maxCommentFollowUps caps CompleteComments so a stuck HasNextPage cannot
// loop forever. 40 extra pages is 2000 comments on top of the inline 50.
const maxCommentFollowUps = 40

// maxLabelFollowUps and maxAttachmentFollowUps are the same cap for the
// sibling Complete* methods. A shared number keeps the three connections
// from drifting; named constants keep each follow loop readable.
const maxLabelFollowUps = 40
const maxAttachmentFollowUps = 40

// CompleteComments follows Issue.Comments.PageInfo until HasNextPage is false
// (or the follow-up cap). The inline page is kept; later nodes are appended.
// A no-op when there is no next page. Callers replace the child list from
// the completed Nodes slice.
func (c *Client) CompleteComments(ctx context.Context, iss *Issue) error {
	if iss == nil {
		return nil
	}
	for n := 0; iss.Comments.PageInfo.HasNextPage; n++ {
		if n >= maxCommentFollowUps || iss.Comments.PageInfo.EndCursor == "" {
			break
		}
		page, err := c.commentsAfter(ctx, iss.ID, iss.Comments.PageInfo.EndCursor)
		if err != nil {
			return err
		}
		if len(page.Nodes) == 0 {
			iss.Comments.PageInfo.HasNextPage = false
			break
		}
		iss.Comments.Nodes = append(iss.Comments.Nodes, page.Nodes...)
		iss.Comments.PageInfo = page.PageInfo
	}
	return nil
}

func (c *Client) commentsAfter(ctx context.Context, issueID, after string) (CommentConn, error) {
	var res struct {
		Issue *struct {
			Comments CommentConn `json:"comments"`
		} `json:"issue"`
	}
	if err := c.gql(ctx, queryIssueComments, map[string]any{"id": issueID, "after": after}, &res); err != nil {
		return CommentConn{}, err
	}
	if res.Issue == nil {
		return CommentConn{}, fmt.Errorf("linear: issue %q not found", issueID)
	}
	return res.Issue.Comments, nil
}

// CompleteLabels follows Issue.Labels.PageInfo the same way CompleteComments
// follows comments. Truncation stays observable if the cap is hit.
func (c *Client) CompleteLabels(ctx context.Context, iss *Issue) error {
	if iss == nil {
		return nil
	}
	for n := 0; iss.Labels.PageInfo.HasNextPage; n++ {
		if n >= maxLabelFollowUps || iss.Labels.PageInfo.EndCursor == "" {
			break
		}
		page, err := c.labelsAfter(ctx, iss.ID, iss.Labels.PageInfo.EndCursor)
		if err != nil {
			return err
		}
		if len(page.Nodes) == 0 {
			iss.Labels.PageInfo.HasNextPage = false
			break
		}
		iss.Labels.Nodes = append(iss.Labels.Nodes, page.Nodes...)
		iss.Labels.PageInfo = page.PageInfo
	}
	return nil
}

func (c *Client) labelsAfter(ctx context.Context, issueID, after string) (LabelConn, error) {
	var res struct {
		Issue *struct {
			Labels LabelConn `json:"labels"`
		} `json:"issue"`
	}
	if err := c.gql(ctx, queryIssueLabels, map[string]any{"id": issueID, "after": after}, &res); err != nil {
		return LabelConn{}, err
	}
	if res.Issue == nil {
		return LabelConn{}, fmt.Errorf("linear: issue %q not found", issueID)
	}
	return res.Issue.Labels, nil
}

// CompleteAttachments follows Issue.Attachments.PageInfo the same way
// CompleteComments follows comments. Truncation stays observable if the
// cap is hit.
func (c *Client) CompleteAttachments(ctx context.Context, iss *Issue) error {
	if iss == nil {
		return nil
	}
	for n := 0; iss.Attachments.PageInfo.HasNextPage; n++ {
		if n >= maxAttachmentFollowUps || iss.Attachments.PageInfo.EndCursor == "" {
			break
		}
		page, err := c.attachmentsAfter(ctx, iss.ID, iss.Attachments.PageInfo.EndCursor)
		if err != nil {
			return err
		}
		if len(page.Nodes) == 0 {
			iss.Attachments.PageInfo.HasNextPage = false
			break
		}
		iss.Attachments.Nodes = append(iss.Attachments.Nodes, page.Nodes...)
		iss.Attachments.PageInfo = page.PageInfo
	}
	return nil
}

func (c *Client) attachmentsAfter(ctx context.Context, issueID, after string) (AttachmentConn, error) {
	var res struct {
		Issue *struct {
			Attachments AttachmentConn `json:"attachments"`
		} `json:"issue"`
	}
	if err := c.gql(ctx, queryIssueAttachments, map[string]any{"id": issueID, "after": after}, &res); err != nil {
		return AttachmentConn{}, err
	}
	if res.Issue == nil {
		return AttachmentConn{}, fmt.Errorf("linear: issue %q not found", issueID)
	}
	return res.Issue.Attachments, nil
}

// LooksLikeID reports whether s is a Linear UUID (the shape issues.assignee_id
// stores). Assign of a mirror id must reach Users() as id.eq, not only as
// displayName/email contains.
func LooksLikeID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}

// Users searches workspace members by name for assignee pickers. An empty
// query lists the first page unfiltered. A UUID query also matches id.eq so
// `gadak assign KEY <issues.assignee_id>` reaches the same user the hint
// names.
func (c *Client) Users(ctx context.Context, query string) ([]User, error) {
	vars := map[string]any{}
	if query != "" {
		or := []any{
			map[string]any{"displayName": map[string]any{"containsIgnoreCase": query}},
			map[string]any{"email": map[string]any{"containsIgnoreCase": query}},
		}
		if LooksLikeID(query) {
			or = append(or, map[string]any{"id": map[string]any{"eq": query}})
		}
		vars["filter"] = map[string]any{"or": or}
	}
	var res struct {
		Users struct {
			Nodes []User `json:"nodes"`
		} `json:"users"`
	}
	if err := c.gql(ctx, queryUsers, vars, &res); err != nil {
		return nil, err
	}
	return res.Users.Nodes, nil
}
