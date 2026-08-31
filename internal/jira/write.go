package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// This file is the only one that changes anything in Jira. Every call here is a
// user action taken in the UI: nothing writes on a schedule, and there is no
// queue — a failed write is reported to the person who asked for it
// (contracts/api.md, "Write-through").

// APIError is a non-2xx answer with its body parsed. Errors is Jira's per-field
// rejection map, which the server passes to the client as `jira_errors` so the
// message lands on the input that caused it. Neither the request nor the
// credential is ever recorded here (constitution article 8).
type APIError struct {
	Status   int
	Messages []string
	Errors   map[string]string
	Body     string
}

func (e *APIError) Error() string {
	if msg := e.Message(); msg != "" {
		return fmt.Sprintf("jira: %d: %s", e.Status, msg)
	}
	return fmt.Sprintf("jira: %d", e.Status)
}

// Message is the first thing worth showing a person: Jira's own message when it
// sent one, the field errors when it only rejected fields, the raw body last.
func (e *APIError) Message() string {
	if len(e.Messages) > 0 {
		return strings.Join(e.Messages, " ")
	}
	if len(e.Errors) > 0 {
		parts := make([]string, 0, len(e.Errors))
		for k, v := range e.Errors {
			parts = append(parts, k+": "+v)
		}
		return strings.Join(parts, "; ")
	}
	return e.Body
}

func apiError(method, path string, code int, status string, body []byte) error {
	e := &APIError{Status: code, Body: snippet(body)}
	var parsed struct {
		Messages []string          `json:"errorMessages"`
		Errors   map[string]string `json:"errors"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		e.Messages, e.Errors = parsed.Messages, parsed.Errors
	}
	if e.Message() == "" {
		e.Body = status
	}
	// The method and path stay in the wrapper so a log line still says what failed.
	return fmt.Errorf("%s %s: %w", method, path, e)
}

// Myself verifies a credential and identifies its owner. It is the only call
// `PUT credential/` makes before storing a token.
func (c *Client) Myself(ctx context.Context) (User, error) {
	var u User
	return u, c.do(ctx, http.MethodGet, apiPath+"/myself", nil, &u)
}

// TransitionField is the subset of a transition screen field the write path
// uses: required, a display name, the schema type, and closed-set values.
// Names in AllowedValues are localized per account; writes send the id.
type TransitionField struct {
	Required bool   `json:"required"`
	Name     string `json:"name"`
	Schema   struct {
		Type string `json:"type"`
	} `json:"schema"`
	AllowedValues []NamedID `json:"allowedValues"`
}

// Transition is one available status change, with the target's category so the
// UI can colour it without knowing the site's status names. Fields is the
// screen Jira returned for expand=transitions.fields (often empty).
type Transition struct {
	ID     string                     `json:"id"`
	Name   string                     `json:"name"`
	To     Status                     `json:"to"`
	Fields map[string]TransitionField `json:"fields"`
}

// Transitions lists the status changes Jira will currently accept on key.
// Each entry's To carries StatusCategory so callers can key on the stable
// category; names are localized per account.
func (c *Client) Transitions(ctx context.Context, key string) ([]Transition, error) {
	var out struct {
		Transitions []Transition `json:"transitions"`
	}
	p := fmt.Sprintf("%s/issue/%s/transitions?expand=transitions.fields", apiPath, url.PathEscape(key))
	return out.Transitions, c.do(ctx, http.MethodGet, p, nil, &out)
}

// Transition performs the transition id on key. fields and comment are omitted
// from the body when empty — a screen that does not list a field rejects it
// with 400, so an empty map is not sent. comment is ADF (callers use Doc).
// Only 429 and 503 are retried, because a 500 may mean Jira already acted.
func (c *Client) Transition(ctx context.Context, key, transitionID string, fields map[string]any, comment json.RawMessage) error {
	body := map[string]any{"transition": map[string]string{"id": transitionID}}
	if len(fields) > 0 {
		body["fields"] = fields
	}
	if len(comment) > 0 {
		body["update"] = map[string]any{
			"comment": []any{
				map[string]any{"add": map[string]any{"body": comment}},
			},
		}
	}
	return c.write(ctx, http.MethodPost, fmt.Sprintf("%s/issue/%s/transitions", apiPath, url.PathEscape(key)), body, nil)
}

// ClaimResult is the answer from POST /issue/{key}/claim: who holds the
// issue now, in which status, since when.
type ClaimResult struct {
	Key       string `json:"key"`
	Assignee  User   `json:"assignee"`
	Status    Status `json:"status"`
	ClaimedAt string `json:"claimedAt"`
}

// Claim is POST /issue/{key}/claim — issuetap's claim extension (GDK-591):
// assignee plus the in-progress transition as one mutation, so of two agents
// claiming concurrently exactly one wins. An empty transitionID lets the
// origin pick the first destination whose category is in-progress; takeOver
// replaces another assignee that already holds the issue in progress.
// Atlassian Cloud has no claim route and answers 404 — internal/claim falls
// back to the two calls this route fuses there. The acting account is the
// credential (or X-Issuetap-Actor on an issuetap origin), never the body.
func (c *Client) Claim(ctx context.Context, key, transitionID string, takeOver bool) (ClaimResult, error) {
	var out ClaimResult
	// A struct, not a map: the two options keep a stable key order on the
	// wire (a map would flip per process) and an empty transitionId is the
	// origin's "pick the first in-progress destination".
	body := struct {
		TransitionID string `json:"transitionId"`
		TakeOver     bool   `json:"takeOver"`
	}{transitionID, takeOver}
	p := fmt.Sprintf("%s/issue/%s/claim", apiPath, url.PathEscape(key))
	err := c.write(ctx, http.MethodPost, p, body, &out)
	return out, err
}

// Resolutions is GET /rest/api/3/resolution — the site catalog. Names are in
// the account language; writes should send the id.
func (c *Client) Resolutions(ctx context.Context) ([]NamedID, error) {
	var list []NamedID
	return list, c.do(ctx, http.MethodGet, apiPath+"/resolution", nil, &list)
}

// Version is one row of GET /rest/api/3/project/{key}/versions. Writes send
// the id: names can be renamed on the site.
type Version struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Released    bool   `json:"released"`
	Archived    bool   `json:"archived"`
	ReleaseDate string `json:"releaseDate"`
}

// ProjectVersions is GET /rest/api/3/project/{key}/versions — the project's
// version catalog. Names can be renamed; writes should send the id.
func (c *Client) ProjectVersions(ctx context.Context, projectKey string) ([]Version, error) {
	var list []Version
	p := fmt.Sprintf("%s/project/%s/versions", apiPath, url.PathEscape(projectKey))
	return list, c.do(ctx, http.MethodGet, p, nil, &list)
}

// CreatesVersionsByName is true when a fixVersions add {"name": token} mints
// the version on this origin (issuetap). Cloud Jira is false: unknown names
// 400, and creating a version is a separate project-admin permission (GDK-678).
func (c *Client) CreatesVersionsByName() bool {
	return c.nameCreatedVersions
}

// EnableNameCreatedVersions marks this client as talking to an origin that
// mints a project version from {"name": token} on a fixVersions add.
// origin.transportJira is the production caller (issuetap in-process, routed
// serve, and paired home).
func (c *Client) EnableNameCreatedVersions() {
	c.nameCreatedVersions = true
}

// IssueLinkType is one row of GET /rest/api/3/issueLinkType. Names and
// inward/outward descriptions can be renamed and localized; writes send the id.
type IssueLinkType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

// IssueLinkTypes is GET /rest/api/3/issueLinkType — the site catalog.
func (c *Client) IssueLinkTypes(ctx context.Context) ([]IssueLinkType, error) {
	var out struct {
		IssueLinkTypes []IssueLinkType `json:"issueLinkTypes"`
	}
	return out.IssueLinkTypes, c.do(ctx, http.MethodGet, apiPath+"/issueLinkType", nil, &out)
}

// LinkIssues is POST /rest/api/3/issueLink. On an issue response, the issue
// at outwardIssue displays the type's inward description; inwardIssue displays
// the outward description. 201/200 with an empty body is success.
func (c *Client) LinkIssues(ctx context.Context, typeID, outwardKey, inwardKey string) error {
	body := map[string]any{
		"type":         map[string]string{"id": typeID},
		"outwardIssue": map[string]string{"key": outwardKey},
		"inwardIssue":  map[string]string{"key": inwardKey},
	}
	return c.write(ctx, http.MethodPost, apiPath+"/issueLink", body, nil)
}

// IssueLinks is GET /issue/{key}?fields=issuelinks — the live projection,
// with the link ids the mirror deliberately does not carry. Deleting needs
// an id, so it always starts here (GDK-1205).
func (c *Client) IssueLinks(ctx context.Context, key string) ([]IssueLink, error) {
	var out struct {
		Fields struct {
			IssueLinks []IssueLink `json:"issuelinks"`
		} `json:"fields"`
	}
	err := c.do(ctx, http.MethodGet, apiPath+"/issue/"+url.PathEscape(key)+"?fields=issuelinks", nil, &out)
	return out.Fields.IssueLinks, err
}

// DeleteIssueLink is DELETE /issueLink/{id}. 204 with an empty body is
// success; an unknown or already-removed id is a 404.
func (c *Client) DeleteIssueLink(ctx context.Context, id string) error {
	return c.write(ctx, http.MethodDelete, apiPath+"/issueLink/"+url.PathEscape(id), nil, nil)
}

// AddComment posts an ADF body (not plain text). Mentions must already be
// mention nodes — a leftover "@Name" string notifies nobody.
// visibility is sent as Jira's visibility object when non-nil. internal
// adds the JSM sd.public.comment property. Neither key is present when
// the corresponding argument is unset, so a flagless CLI comment matches
// the previous POST body.
func (c *Client) AddComment(ctx context.Context, key string, adf json.RawMessage, visibility *CommentVisibility, internal bool) (Comment, error) {
	var out Comment
	body := map[string]any{"body": adf}
	if visibility != nil {
		body["visibility"] = visibility
	}
	if internal {
		body["properties"] = []any{
			map[string]any{
				"key":   "sd.public.comment",
				"value": map[string]any{"internal": true},
			},
		}
	}
	return out, c.write(ctx, http.MethodPost, fmt.Sprintf("%s/issue/%s/comment", apiPath, url.PathEscape(key)), body, &out)
}

// SetAssignee assigns or, with an empty id, unassigns. Jira distinguishes "no
// assignee" (null) from "default assignee" (-1); the UI only ever asks for the
// former.
func (c *Client) SetAssignee(ctx context.Context, key, accountID string) error {
	body := map[string]any{"accountId": nil}
	if accountID != "" {
		body["accountId"] = accountID
	}
	return c.write(ctx, http.MethodPut, fmt.Sprintf("%s/issue/%s/assignee", apiPath, url.PathEscape(key)), body, nil)
}

// UpdateFields sets raw field values. The caller is responsible for the shape
// each field id expects, which EditMeta describes.
func (c *Client) UpdateFields(ctx context.Context, key string, fields map[string]any) error {
	body := map[string]any{"fields": fields}
	return c.write(ctx, http.MethodPut, fmt.Sprintf("%s/issue/%s", apiPath, url.PathEscape(key)), body, nil)
}

// EditIssue PUTs /issue/{key} with fields and/or update. Either map may be
// empty; empty maps are omitted so a labels-only edit is {"update":…} only.
func (c *Client) EditIssue(ctx context.Context, key string, fields, update map[string]any) error {
	body := map[string]any{}
	if len(fields) > 0 {
		body["fields"] = fields
	}
	if len(update) > 0 {
		body["update"] = update
	}
	return c.write(ctx, http.MethodPut, fmt.Sprintf("%s/issue/%s", apiPath, url.PathEscape(key)), body, nil)
}

// FieldMeta is one editable field as Jira describes it: what it accepts and, for
// a closed set, every value it accepts.
type FieldMeta struct {
	Required   bool     `json:"required"`
	Operations []string `json:"operations"`
	Schema     struct {
		Type   string `json:"type"`
		Items  string `json:"items"`
		Custom string `json:"custom"`
	} `json:"schema"`
	AllowedValues []struct {
		ID    string `json:"id"`
		Value string `json:"value"`
		Name  string `json:"name"`
	} `json:"allowedValues"`
}

// EditMeta returns the fields this user may edit on this issue, keyed by field id.
func (c *Client) EditMeta(ctx context.Context, key string) (map[string]FieldMeta, error) {
	var out struct {
		Fields map[string]FieldMeta `json:"fields"`
	}
	p := fmt.Sprintf("%s/issue/%s/editmeta", apiPath, url.PathEscape(key))
	return out.Fields, c.do(ctx, http.MethodGet, p, nil, &out)
}

// CreateIssue returns the new issue's key.
func (c *Client) CreateIssue(ctx context.Context, fields map[string]any) (string, error) {
	var out struct {
		Key string `json:"key"`
	}
	return out.Key, c.write(ctx, http.MethodPost, apiPath+"/issue", map[string]any{"fields": fields}, &out)
}

// CreateMetaIssueType is one creatable issue type. Distinct from NamedID:
// priorities, resolutions, components, and versions share {id,name} but have
// no hierarchy, and putting subtask/hierarchyLevel/untranslatedName on NamedID
// would leak meaningless fields onto those catalogs. JSON names match Jira's
// createmeta object (id, name, untranslatedName, subtask, hierarchyLevel).
// omitempty keeps a standard type (false, 0, empty untranslatedName) looking
// the way it did before these fields existed.
type CreateMetaIssueType struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	UntranslatedName string `json:"untranslatedName,omitempty"`
	Subtask          bool   `json:"subtask,omitempty"`
	HierarchyLevel   int    `json:"hierarchyLevel,omitempty"`
}

// NamedID is the id/name pair FormatTypes, NeedTypeError, and Priority
// matching use. Hierarchy is dropped on purpose: those catalogs share
// {id,name} and have no rank. create.Type matches CreateMetaIssueType
// directly so it can see subtask, hierarchyLevel, and untranslatedName.
func (t CreateMetaIssueType) NamedID() NamedID {
	return NamedID{ID: t.ID, Name: t.Name}
}

// CreateMetaProject is one project a person may file into, with its issue types.
type CreateMetaProject struct {
	Key        string                `json:"key"`
	Name       string                `json:"name"`
	IssueTypes []CreateMetaIssueType `json:"issuetypes"`
}

// NamedTypes is the id/name catalog FormatTypes and NeedTypeError consume.
func (p CreateMetaProject) NamedTypes() []NamedID {
	out := make([]NamedID, len(p.IssueTypes))
	for i, t := range p.IssueTypes {
		out[i] = t.NamedID()
	}
	return out
}

// CreateMeta lists what can be created. Restricted to the configured projects:
// the site-wide answer is large and most of it is unreachable from this UI.
func (c *Client) CreateMeta(ctx context.Context, projects []string) ([]CreateMetaProject, error) {
	var out struct {
		Projects []CreateMetaProject `json:"projects"`
	}
	p := apiPath + "/issue/createmeta"
	if len(projects) > 0 {
		p += "?projectKeys=" + url.QueryEscape(strings.Join(projects, ","))
	}
	return out.Projects, c.do(ctx, http.MethodGet, p, nil, &out)
}

// CreateFieldMeta is one field Jira lists at create time. Distinct from
// FieldMeta: createmeta fields are a list with fieldId on each object and
// carry hasDefaultValue; editmeta is a map keyed by field id with no fieldId
// on the value (GDK-254). Schema matches FieldMeta's anonymous shape.
type CreateFieldMeta struct {
	FieldID         string `json:"fieldId"`
	Name            string `json:"name"`
	Required        bool   `json:"required"`
	HasDefaultValue bool   `json:"hasDefaultValue"`
	Schema          struct {
		Type   string `json:"type"`
		Items  string `json:"items"`
		Custom string `json:"custom"`
	} `json:"schema"`
	AllowedValues []struct {
		ID    string `json:"id"`
		Value string `json:"value"`
		Name  string `json:"name"`
	} `json:"allowedValues"`
}

// CreateFields pages GET /issue/createmeta/{project}/issuetypes/{type}.
// The expand=projects.issuetypes.fields form is the discarded path; this is
// the current Cloud list (fields[], startAt/maxResults/total).
func (c *Client) CreateFields(ctx context.Context, projectIDOrKey, issueTypeID string) ([]CreateFieldMeta, error) {
	out := []CreateFieldMeta{}
	for startAt := 0; ; {
		var page struct {
			Fields     []CreateFieldMeta `json:"fields"`
			Total      int               `json:"total"`
			MaxResults int               `json:"maxResults"`
			StartAt    int               `json:"startAt"`
		}
		p := fmt.Sprintf("%s/issue/createmeta/%s/issuetypes/%s?startAt=%d&maxResults=50",
			apiPath, url.PathEscape(projectIDOrKey), url.PathEscape(issueTypeID), startAt)
		if err := c.do(ctx, http.MethodGet, p, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Fields...)
		startAt += len(page.Fields)
		if len(page.Fields) == 0 || startAt >= page.Total {
			return out, nil
		}
	}
}

// SearchUsers backs the assignee picker. Jira's own endpoint decides what
// matches; there is no local user table to search.
func (c *Client) SearchUsers(ctx context.Context, query string) ([]User, error) {
	var out []User
	p := fmt.Sprintf("%s/user/search?query=%s&maxResults=20", apiPath, url.QueryEscape(query))
	return out, c.do(ctx, http.MethodGet, p, nil, &out)
}

// Upload attaches one file. Jira requires the nosniff header on this endpoint and
// answers with the created attachments.
//
// ponytail: buffers the whole file in memory. Fine for the screenshots this is
// for; stream with io.Pipe if someone starts attaching video.
func (c *Client) Upload(ctx context.Context, key, filename string, file io.Reader) ([]Attachment, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	p := fmt.Sprintf("%s/issue/%s/attachments", apiPath, url.PathEscape(key))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+p, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	// Jira rejects an attachment upload without this header, by design: it is what
	// stops a cross-site form post from uploading on the user's behalf.
	req.Header.Set("X-Atlassian-Token", "no-check")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", p, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	switch {
	case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("POST %s: %w (%s)", p, ErrAuth, res.Status)
	case res.StatusCode >= 300:
		return nil, apiError(http.MethodPost, p, res.StatusCode, res.Status, data)
	case err != nil:
		return nil, fmt.Errorf("POST %s: %w", p, err)
	}
	var out []Attachment
	return out, json.Unmarshal(data, &out)
}

// mediaIDPattern pulls the media UUID out of the pre-signed media URL Jira
// redirects an attachment download to. The path looks like
// `/file/<uuid>/binary` (or `/file/<uuid>/artifact/...`).
var mediaIDPattern = regexp.MustCompile(`/file/([0-9a-fA-F-]{36})`)

// MediaRef resolves an attachment id to both the media UUID Jira needs in an ADF
// node and the filename our own renderer matches on (`alt`), which is what makes
// an inline image resolve without persisting the UUID anywhere.
//
// There is no documented endpoint for this. Requesting the attachment's content
// answers 3xx to a pre-signed media URL that carries the UUID, so the redirect is
// read rather than followed — following it would download the whole file for a
// string, and the credential must not travel to the media host.
func (c *Client) MediaRef(ctx context.Context, attachmentID string) (mediaID, filename string, err error) {
	return c.mediaRef(ctx, attachmentID)
}

func (c *Client) mediaRef(ctx context.Context, attachmentID string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+apiPath+"/attachment/content/"+url.PathEscape(attachmentID), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", c.auth)

	// A client that refuses redirects, so the Location header survives.
	noFollow := &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	if c.HTTP != nil {
		noFollow.Transport = c.HTTP.Transport
	}
	res, err := noFollow.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return "", "", ErrAuth
	}
	loc := res.Header.Get("Location")
	if loc == "" {
		return "", "", fmt.Errorf("jira: attachment %s did not redirect to a media URL (status %d)",
			attachmentID, res.StatusCode)
	}
	m := mediaIDPattern.FindStringSubmatch(loc)
	if m == nil {
		return "", "", fmt.Errorf("jira: no media id in the attachment redirect for %s", attachmentID)
	}
	// The pre-signed URL carries the original name; fall back to metadata only if
	// it does not, so the common case stays one request.
	name := filenameFromMediaURL(loc)
	if name == "" {
		name, _ = c.attachmentFilename(ctx, attachmentID)
	}
	return m[1], name, nil
}

// filenameFromMediaURL reads the `name=` query parameter Jira's media URLs carry.
func filenameFromMediaURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Query().Get("name")
}

// attachmentFilename is the documented metadata call, used only when the media
// URL did not name the file.
func (c *Client) attachmentFilename(ctx context.Context, attachmentID string) (string, error) {
	var meta struct {
		Filename string `json:"filename"`
	}
	if err := c.do(ctx, http.MethodGet, "/attachment/"+url.PathEscape(attachmentID), nil, &meta); err != nil {
		return "", err
	}
	return meta.Filename, nil
}
