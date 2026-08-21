package jira

// The development-panel read (GDK-496/497). On Jira Cloud this is the
// internal /rest/dev-status API — Atlassian's own workaround for the panel
// having no public read (JSWCLOUD-16901): works with the site API token,
// explicitly unsupported and free to change. issuetap serves the same two
// GETs on purpose, so this one reader covers connected and standalone.
// Callers gate it behind config.DevStatus and treat every error as
// "skip this issue", never as a sync failure.

import (
	"context"
	"fmt"
	"strings"
)

// DevPR is one pull request from the dev-status detail payload.
type DevPR struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Name   string `json:"name"`
	Status string `json:"status"` // OPEN | MERGED | DECLINED
}

// DevStatusPRCount reads the summary's pullrequest count — one cheap call to
// decide whether the detail call is worth making. issueID is the origin's
// issue id verbatim (numeric on Cloud), never the key.
func (c *Client) DevStatusPRCount(ctx context.Context, issueID string) (int, error) {
	var out struct {
		Summary struct {
			PullRequest struct {
				Overall struct {
					Count int `json:"count"`
				} `json:"overall"`
			} `json:"pullrequest"`
		} `json:"summary"`
	}
	p := fmt.Sprintf("/rest/dev-status/latest/issue/summary?issueId=%s", issueID)
	if err := c.do(ctx, "GET", p, nil, &out); err != nil {
		return 0, err
	}
	return out.Summary.PullRequest.Overall.Count, nil
}

// DevStatusPRs reads the pull requests the panel holds for one issue.
// applicationType=GitHub is what Cloud's own frontend sends for the GitHub
// app (and issuetap accepts any value); the response is parsed defensively —
// unknown fields are ignored, missing ones stay zero.
func (c *Client) DevStatusPRs(ctx context.Context, issueID string) ([]DevPR, error) {
	var out struct {
		Detail []struct {
			PullRequests []DevPR `json:"pullRequests"`
		} `json:"detail"`
	}
	p := fmt.Sprintf("/rest/dev-status/1.0/issue/detail?issueId=%s&applicationType=GitHub&dataType=pullrequest", issueID)
	if err := c.do(ctx, "GET", p, nil, &out); err != nil {
		return nil, err
	}
	var prs []DevPR
	for _, d := range out.Detail {
		for _, pr := range d.PullRequests {
			pr.Status = strings.ToUpper(pr.Status)
			prs = append(prs, pr)
		}
	}
	return prs, nil
}

// LinkDevPR posts one pull-request link to the origin's dev-status store.
// Only issuetap (standalone) implements the endpoint — Jira Cloud's panel is
// written by its marketplace apps, so a connected origin answers 404 and the
// caller says so instead of pretending.
func (c *Client) LinkDevPR(ctx context.Context, issueID, prURL, name, status string) (DevPR, error) {
	var out DevPR
	body := map[string]any{"issueId": issueID, "url": prURL, "name": name, "status": status}
	err := c.write(ctx, "POST", "/rest/dev-status/1.0/issue/link", body, &out)
	return out, err
}
