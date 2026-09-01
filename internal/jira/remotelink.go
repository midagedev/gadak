package jira

// Remote issue links (GDK-1032): Cloud's
// /rest/api/3/issue/{key}/remotelink. Both origins gadak points this at —
// Atlassian Cloud and issuetap — speak the same shape; gadak only ever
// writes them on issuetap-backed origins (localOrigin / paired), where a
// gadak://<workspace>/<KEY> URL points one workspace's issue at another's.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// RemoteLink is one remote issue link. ID is normalized to a string — Cloud
// serves a number, a fixture-authored issuetap id can be a string.
type RemoteLink struct {
	ID           string
	GlobalID     string
	Relationship string
	URL          string
	Title        string
	Summary      string
}

type remoteLinkWire struct {
	ID           json.RawMessage `json:"id"`
	GlobalID     string          `json:"globalId"`
	Relationship string          `json:"relationship"`
	Object       struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
	} `json:"object"`
}

func rawID(raw json.RawMessage) string {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// RemoteLinks lists key's remote issue links.
func (c *Client) RemoteLinks(ctx context.Context, key string) ([]RemoteLink, error) {
	var wire []remoteLinkWire
	if err := c.do(ctx, "GET", apiPath+"/issue/"+url.PathEscape(key)+"/remotelink", nil, &wire); err != nil {
		return nil, err
	}
	out := make([]RemoteLink, 0, len(wire))
	for _, w := range wire {
		out = append(out, RemoteLink{
			ID: rawID(w.ID), GlobalID: w.GlobalID, Relationship: w.Relationship,
			URL: w.Object.URL, Title: w.Object.Title, Summary: w.Object.Summary,
		})
	}
	return out, nil
}

// SetRemoteLink creates or updates one remote link on key. A non-empty
// GlobalID is the upsert identity (Cloud's documented contract).
func (c *Client) SetRemoteLink(ctx context.Context, key string, rl RemoteLink) error {
	body := map[string]any{
		"object": map[string]any{
			"url":     rl.URL,
			"title":   rl.Title,
			"summary": rl.Summary,
		},
	}
	if rl.GlobalID != "" {
		body["globalId"] = rl.GlobalID
	}
	if rl.Relationship != "" {
		body["relationship"] = rl.Relationship
	}
	return c.write(ctx, "POST", apiPath+"/issue/"+url.PathEscape(key)+"/remotelink", body, nil)
}

// DeleteRemoteLink removes one remote link by id.
func (c *Client) DeleteRemoteLink(ctx context.Context, key, id string) error {
	path := apiPath + "/issue/" + url.PathEscape(key) + "/remotelink/" + url.PathEscape(id)
	status, data, err := c.Raw(ctx, "DELETE", path, nil, true)
	if err != nil {
		return err
	}
	if status >= 300 {
		return apiError("DELETE", path, status, fmt.Sprintf("%d", status), data)
	}
	return nil
}
