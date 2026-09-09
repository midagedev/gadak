package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path/filepath"
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
	return u, c.do(ctx, http.MethodGet, c.apiBase+"/myself", nil, &u)
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
	p := fmt.Sprintf("%s/issue/%s/transitions?expand=transitions.fields", c.apiBase, url.PathEscape(key))
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
	return c.write(ctx, http.MethodPost, fmt.Sprintf("%s/issue/%s/transitions", c.apiBase, url.PathEscape(key)), body, nil)
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
	p := fmt.Sprintf("%s/issue/%s/claim", c.apiBase, url.PathEscape(key))
	err := c.write(ctx, http.MethodPost, p, body, &out)
	return out, err
}

// Resolutions is GET /resolution — the site catalog. Names are in
// the account language; writes should send the id.
func (c *Client) Resolutions(ctx context.Context) ([]NamedID, error) {
	var list []NamedID
	return list, c.do(ctx, http.MethodGet, c.apiBase+"/resolution", nil, &list)
}

// Version is one row of GET /project/{key}/versions. Writes send
// the id: names can be renamed on the site.
type Version struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Released    bool   `json:"released"`
	Archived    bool   `json:"archived"`
	ReleaseDate string `json:"releaseDate"`
}

// ProjectVersions is GET /project/{key}/versions — the project's
// version catalog. Names can be renamed; writes should send the id.
func (c *Client) ProjectVersions(ctx context.Context, projectKey string) ([]Version, error) {
	var list []Version
	p := fmt.Sprintf("%s/project/%s/versions", c.apiBase, url.PathEscape(projectKey))
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

// IssueLinkType is one row of GET /issueLinkType. Names and
// inward/outward descriptions can be renamed and localized; writes send the id.
type IssueLinkType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

// IssueLinkTypes is GET /issueLinkType — the site catalog.
func (c *Client) IssueLinkTypes(ctx context.Context) ([]IssueLinkType, error) {
	var out struct {
		IssueLinkTypes []IssueLinkType `json:"issueLinkTypes"`
	}
	return out.IssueLinkTypes, c.do(ctx, http.MethodGet, c.apiBase+"/issueLinkType", nil, &out)
}

// LinkIssues is POST /issueLink. On an issue response, the issue
// at outwardIssue displays the type's inward description; inwardIssue displays
// the outward description — measured, with the evidence in
// origin.ResolveLinkType (GDK-1599). 201/200 with an empty body is success.
func (c *Client) LinkIssues(ctx context.Context, typeID, outwardKey, inwardKey string) error {
	body := map[string]any{
		"type":         map[string]string{"id": typeID},
		"outwardIssue": map[string]string{"key": outwardKey},
		"inwardIssue":  map[string]string{"key": inwardKey},
	}
	return c.write(ctx, http.MethodPost, c.apiBase+"/issueLink", body, nil)
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
	err := c.do(ctx, http.MethodGet, c.apiBase+"/issue/"+url.PathEscape(key)+"?fields=issuelinks", nil, &out)
	return out.Fields.IssueLinks, err
}

// DeleteIssueLink is DELETE /issueLink/{id}. 204 with an empty body is
// success; an unknown or already-removed id is a 404.
func (c *Client) DeleteIssueLink(ctx context.Context, id string) error {
	return c.write(ctx, http.MethodDelete, c.apiBase+"/issueLink/"+url.PathEscape(id), nil, nil)
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
	return out, c.write(ctx, http.MethodPost, fmt.Sprintf("%s/issue/%s/comment", c.apiBase, url.PathEscape(key)), body, &out)
}

// UpdateComment replaces a comment's body: PUT /issue/{key}/comment/{id},
// the edit-side sibling of AddComment (GDK-1647). body is what a post
// sends — an ADF document on Cloud and the built-in tracker, the wiki
// markup string on a Jira Server origin (GDK-1637); this client does not
// convert either way, the caller routes through origin.BodyValue. 200 with
// the stored comment is success.
func (c *Client) UpdateComment(ctx context.Context, key, id string, body json.RawMessage) (Comment, error) {
	var out Comment
	p := fmt.Sprintf("%s/issue/%s/comment/%s", c.apiBase, url.PathEscape(key), url.PathEscape(id))
	return out, c.write(ctx, http.MethodPut, p, map[string]any{"body": body}, &out)
}

// DeleteComment is DELETE /issue/{key}/comment/{id}. 204 with an empty body
// is success; an unknown or already-removed id is a 404.
func (c *Client) DeleteComment(ctx context.Context, key, id string) error {
	p := fmt.Sprintf("%s/issue/%s/comment/%s", c.apiBase, url.PathEscape(key), url.PathEscape(id))
	return c.write(ctx, http.MethodDelete, p, nil, nil)
}

// SetAssignee assigns or, with an empty id, unassigns. Jira distinguishes "no
// assignee" (null) from "default assignee" (-1); the UI only ever asks for the
// former. The body key is the deployment's own user axis (GDK-1638): Cloud
// reads accountId, Server reads name — and the null that unassigns must sit
// under the same key the deployment reads, or Server leaves the assignee
// untouched.
func (c *Client) SetAssignee(ctx context.Context, key, id string) error {
	k := "accountId"
	if c.serverDialect() {
		k = "name"
	}
	body := map[string]any{k: nil}
	if id != "" {
		body[k] = id
	}
	return c.write(ctx, http.MethodPut, fmt.Sprintf("%s/issue/%s/assignee", c.apiBase, url.PathEscape(key)), body, nil)
}

// UpdateFields sets raw field values. The caller is responsible for the shape
// each field id expects, which EditMeta describes.
func (c *Client) UpdateFields(ctx context.Context, key string, fields map[string]any) error {
	if err := c.serverParent(ctx, fields, func() (bool, error) { return c.isSubtask(ctx, key) }); err != nil {
		return err
	}
	body := map[string]any{"fields": fields}
	return c.write(ctx, http.MethodPut, fmt.Sprintf("%s/issue/%s", c.apiBase, url.PathEscape(key)), body, nil)
}

// EditIssue PUTs /issue/{key} with fields and/or update. Either map may be
// empty; empty maps are omitted so a labels-only edit is {"update":…} only.
func (c *Client) EditIssue(ctx context.Context, key string, fields, update map[string]any) error {
	if err := c.serverParent(ctx, fields, func() (bool, error) { return c.isSubtask(ctx, key) }); err != nil {
		return err
	}
	body := map[string]any{}
	if len(fields) > 0 {
		body["fields"] = fields
	}
	if len(update) > 0 {
		body["update"] = update
	}
	return c.write(ctx, http.MethodPut, fmt.Sprintf("%s/issue/%s", c.apiBase, url.PathEscape(key)), body, nil)
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
	p := fmt.Sprintf("%s/issue/%s/editmeta", c.apiBase, url.PathEscape(key))
	return out.Fields, c.do(ctx, http.MethodGet, p, nil, &out)
}

// CreateIssue returns the new issue's key.
func (c *Client) CreateIssue(ctx context.Context, fields map[string]any) (string, error) {
	var out struct {
		Key string `json:"key"`
	}
	if err := c.serverParent(ctx, fields, func() (bool, error) { return c.isSubtaskType(ctx, fields) }); err != nil {
		return "", err
	}
	return out.Key, c.write(ctx, http.MethodPost, c.apiBase+"/issue", map[string]any{"fields": fields}, &out)
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

// ErrServerCreateMetaScope is the named refusal when a Jira Server / Data
// Center client is asked for create metadata without a project scope
// (GDK-1636): the bulk createmeta Cloud serves is gone there, and the
// per-project route has nothing to ask without a key. A Cloud client with
// no scope still gets the site-wide list.
var ErrServerCreateMetaScope = errors.New("jira: Jira Server lists create metadata per project only — configure this workspace's projects")

// CreateMeta lists what can be created. Restricted to the configured projects:
// the site-wide answer is large and most of it is unreachable from this UI.
func (c *Client) CreateMeta(ctx context.Context, projects []string) ([]CreateMetaProject, error) {
	if c.serverDialect() {
		return c.createMetaServer(ctx, projects)
	}
	var out struct {
		Projects []CreateMetaProject `json:"projects"`
	}
	p := c.apiBase + "/issue/createmeta"
	if len(projects) > 0 {
		p += "?projectKeys=" + url.QueryEscape(strings.Join(projects, ","))
	}
	return out.Projects, c.do(ctx, http.MethodGet, p, nil, &out)
}

// createMetaServer is Server's createmeta (GDK-1636): the bulk route is gone,
// so each configured project is asked on its own
// /issue/createmeta/{projectIdOrKey}/issuetypes route and the pages are
// flattened into the same []CreateMetaProject every caller consumes. The
// route returns no project name — Key is the identity callers key on, and
// inventing a name here would be a display value the origin never sent.
func (c *Client) createMetaServer(ctx context.Context, projects []string) ([]CreateMetaProject, error) {
	if len(projects) == 0 {
		return nil, ErrServerCreateMetaScope
	}
	out := make([]CreateMetaProject, 0, len(projects))
	for _, key := range projects {
		types, err := c.createMetaIssueTypes(ctx, key)
		if err != nil {
			return nil, err
		}
		out = append(out, CreateMetaProject{Key: key, IssueTypes: types})
	}
	return out, nil
}

// createMetaIssueTypes pages one project's creatable issue types out of
// Server's values envelope ({maxResults,startAt,total,isLast,values}).
func (c *Client) createMetaIssueTypes(ctx context.Context, project string) ([]CreateMetaIssueType, error) {
	out := []CreateMetaIssueType{}
	for startAt := 0; ; {
		var page struct {
			MaxResults int                   `json:"maxResults"`
			StartAt    int                   `json:"startAt"`
			Total      int                   `json:"total"`
			IsLast     bool                  `json:"isLast"`
			Values     []CreateMetaIssueType `json:"values"`
		}
		p := fmt.Sprintf("%s/issue/createmeta/%s/issuetypes?startAt=%d&maxResults=100",
			c.apiBase, url.PathEscape(project), startAt)
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
// the current Cloud list (fields[], startAt/maxResults/total). Server serves
// the same route with the v2 createmeta family's values envelope (GDK-1636);
// the row shape inside it is the fieldId list Cloud returns.
func (c *Client) CreateFields(ctx context.Context, projectIDOrKey, issueTypeID string) ([]CreateFieldMeta, error) {
	out := []CreateFieldMeta{}
	for startAt := 0; ; {
		var page struct {
			Fields     []CreateFieldMeta `json:"fields"`
			Values     []CreateFieldMeta `json:"values"`
			Total      int               `json:"total"`
			MaxResults int               `json:"maxResults"`
			StartAt    int               `json:"startAt"`
		}
		p := fmt.Sprintf("%s/issue/createmeta/%s/issuetypes/%s?startAt=%d&maxResults=50",
			c.apiBase, url.PathEscape(projectIDOrKey), url.PathEscape(issueTypeID), startAt)
		if err := c.do(ctx, http.MethodGet, p, nil, &page); err != nil {
			return nil, err
		}
		rows := page.Fields
		if c.serverDialect() {
			rows = page.Values
		}
		out = append(out, rows...)
		startAt += len(rows)
		if len(rows) == 0 || startAt >= page.Total {
			return out, nil
		}
	}
}

// SearchUsers backs the assignee picker. Jira's own endpoint decides what
// matches; there is no local user table to search. The parameter is the
// deployment's own (GDK-1638): Server reads username= and answers query=
// with a silent empty list, so the dialect branches once here — never a
// fallback from one parameter to the other.
func (c *Client) SearchUsers(ctx context.Context, query string) ([]User, error) {
	var out []User
	param := "query"
	if c.serverDialect() {
		param = "username"
	}
	p := fmt.Sprintf("%s/user/search?%s=%s&maxResults=20", c.apiBase, param, url.QueryEscape(query))
	return out, c.do(ctx, http.MethodGet, p, nil, &out)
}

// Upload attaches one file. Jira requires the nosniff header on this endpoint and
// answers with the created attachments.
//
// ponytail: buffers the whole file in memory. Fine for the screenshots this is
// for; stream with io.Pipe if someone starts attaching video.
// attachmentPartHeader is CreateFormFile's header with a real
// Content-Type. An extension with no known type keeps the generic one,
// which is the honest answer rather than a guess.
func attachmentPartHeader(filename string) textproto.MIMEHeader {
	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if ct == "" {
		ct = "application/octet-stream"
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(filename)))
	h.Set("Content-Type", ct)
	return h
}

// escapeQuotes is mime/multipart's own, which it does not export.
var escapeQuotes = strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace

func (c *Client) Upload(ctx context.Context, key, filename string, file io.Reader) ([]Attachment, error) {
	// Build the multipart body into a pipe rather than a buffer: an
	// attachment can be hundreds of megabytes (measured: a real
	// workspace's largest is 884 MiB), and buffering it whole was a
	// ceiling on `gadak attach` that had nothing to do with the origin's
	// own limit (GDK-1617).
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		// Declare the type from the filename. CreateFormFile hardcodes
		// application/octet-stream, and an origin that keeps what it is
		// told — gadak's own tracker does, and it holds the only copy —
		// then stores every screenshot and every video as a generic
		// download: `is_image` and `is_video` are false, so the app shows
		// a file row instead of a thumbnail and a player, forever
		// (GDK-1617, measured: `gadak attach` of a .png and an .mp4).
		part, err := mw.CreatePart(attachmentPartHeader(filename))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.CloseWithError(mw.Close())
	}()

	p := fmt.Sprintf("%s/issue/%s/attachments", c.apiBase, url.PathEscape(key))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+p, pr)
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

// ErrServerNoMediaRef is the named refusal when a Jira Server / Data Center
// client is asked for an attachment's media id (GDK-1636): the Cloud route
// this resolves — /attachment/content/{id} redirecting to a pre-signed
// media URL — has no Server counterpart, and asking a base that does not
// serve the route can only produce an error, never a media id.
var ErrServerNoMediaRef = errors.New("jira: Jira Server has no attachment media route (Cloud only)")

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
	if c.serverDialect() {
		return "", "", ErrServerNoMediaRef
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+c.apiBase+"/attachment/content/"+url.PathEscape(attachmentID), nil)
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
// URL did not name the file. The path carries the API base now — born without
// one, it requested <site>/attachment/{id}, which no Jira deployment serves,
// so the fallback could only ever fail (GDK-1636).
func (c *Client) attachmentFilename(ctx context.Context, attachmentID string) (string, error) {
	var meta struct {
		Filename string `json:"filename"`
	}
	if err := c.do(ctx, http.MethodGet, c.apiBase+"/attachment/"+url.PathEscape(attachmentID), nil, &meta); err != nil {
		return "", err
	}
	return meta.Filename, nil
}
