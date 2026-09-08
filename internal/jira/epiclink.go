package jira

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GDK-1645. Jira Server keys a standard issue's epic in the Epic Link custom
// field, not fields.parent — parent there belongs to sub-tasks only, and a
// parent sent for a standard issue is answered 204 and dropped (measured on
// 11.3.11: PUT {"fields":{"parent":{"key":"SCR-7"}}} → 204, and the re-read
// carries no parent field at all). Cloud folds every level into parent. The
// rewrite lives here, at the dialect owner, so create, edit and the web's
// parent editor all pass through it and no caller learns the field id.
//
// GhEpicLinkCustom is the schema.custom key of that field; sync reads the
// same key back into parent_key (internal/sync/sprint.go).
const GhEpicLinkCustom = "com.pyxis.greenhopper.jira:gh-epic-link"

// ErrNoEpicLinkField is the refusal when a Server has no Epic Link field at
// all — a Jira Core site (no Jira Software) — and a standard issue is given
// a parent. Sending fields.parent there would be the silent 204 this file
// exists to stop.
var ErrNoEpicLinkField = errors.New("jira: this Jira Server has no Epic Link field, so a standard issue cannot be given a parent here (Jira Software is what adds it; a sub-task's parent still works)")

// epicLinkField resolves the Epic Link custom field id once per client from
// the field catalog. An empty id with a nil error means the site has none.
func (c *Client) epicLinkField(ctx context.Context) (string, error) {
	c.epicLinkMu.Lock()
	defer c.epicLinkMu.Unlock()
	if c.epicLinkLoaded {
		return c.epicLinkID, nil
	}
	list, err := c.Fields(ctx)
	if err != nil {
		return "", err
	}
	for _, f := range list {
		if strings.HasSuffix(f.Schema.Custom, GhEpicLinkCustom) {
			c.epicLinkID = f.ID
			break
		}
	}
	c.epicLinkLoaded = true
	return c.epicLinkID, nil
}

// isSubtask reads the one bit that decides which field a Server parent
// goes to: GET /issue/{key}?fields=issuetype → fields.issuetype.subtask.
func (c *Client) isSubtask(ctx context.Context, key string) (bool, error) {
	var out struct {
		Fields struct {
			IssueType struct {
				Subtask bool `json:"subtask"`
			} `json:"issuetype"`
		} `json:"fields"`
	}
	p := fmt.Sprintf("%s/issue/%s?fields=issuetype", c.apiBase, url.PathEscape(key))
	if err := c.do(ctx, http.MethodGet, p, nil, &out); err != nil {
		return false, err
	}
	return out.Fields.IssueType.Subtask, nil
}

// isSubtaskType is isSubtask for a create, where the issue does not exist
// yet and the type id is in the fields: GET /issuetype/{id}.
func (c *Client) isSubtaskType(ctx context.Context, fields map[string]any) (bool, error) {
	id := ""
	switch t := fields["issuetype"].(type) {
	case map[string]string:
		id = t["id"]
	case map[string]any:
		id, _ = t["id"].(string)
	}
	if id == "" {
		return false, nil
	}
	var out struct {
		Subtask bool `json:"subtask"`
	}
	if err := c.do(ctx, http.MethodGet, c.apiBase+"/issuetype/"+url.PathEscape(id), nil, &out); err != nil {
		return false, err
	}
	return out.Subtask, nil
}

// serverParent rewrites fields["parent"] in place for the Server dialect
// when the issue is not a sub-task: the value moves to the Epic Link field
// (an epic key string; nil clears). Cloud, a sub-task, and a body without
// parent are untouched. subtask is the answer of isSubtask/isSubtaskType.
func (c *Client) serverParent(ctx context.Context, fields map[string]any, subtask func() (bool, error)) error {
	if !c.serverDialect() {
		return nil
	}
	raw, has := fields["parent"]
	if !has {
		return nil
	}
	sub, err := subtask()
	if err != nil {
		return err
	}
	if sub {
		return nil
	}
	id, err := c.epicLinkField(ctx)
	if err != nil {
		return err
	}
	if id == "" {
		return ErrNoEpicLinkField
	}
	delete(fields, "parent")
	if raw == nil {
		fields[id] = nil
		return nil
	}
	key := ""
	switch p := raw.(type) {
	case map[string]string:
		key = p["key"]
	case map[string]any:
		key, _ = p["key"].(string)
	}
	if key == "" {
		return errors.New("jira: a Server epic link needs the parent's key, not an id")
	}
	fields[id] = key
	return nil
}
