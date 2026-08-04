package main

import (
	"fmt"
	"net/url"
	"os"
)

// siteIssue is the subset of fields needed for repair passes.
type siteIssue struct {
	Key        string
	Summary    string
	AssigneeID string
	StatusID   string
}

// iterSiteIssues pages every issue in scope and yields decoded rows.
func (c *Client) iterSiteIssues(projects []string, fields string, fn func(siteIssue) bool) {
	for _, project := range projects {
		token := ""
		for page := 0; page <= 40; page++ {
			pathQ := fmt.Sprintf("/rest/api/3/search/jql?jql=project%%3D%s&maxResults=100&fields=%s",
				url.QueryEscape(project), fields)
			if token != "" {
				pathQ += "&nextPageToken=" + url.QueryEscape(token)
			}
			var res struct {
				Issues []struct {
					Key    string `json:"key"`
					Fields struct {
						Summary  string `json:"summary"`
						Assignee *struct {
							AccountID string `json:"accountId"`
						} `json:"assignee"`
						Status *struct {
							ID string `json:"id"`
						} `json:"status"`
					} `json:"fields"`
				} `json:"issues"`
				NextPageToken string `json:"nextPageToken"`
				IsLast        bool   `json:"isLast"`
			}
			if !c.call("GET", pathQ, nil, &res) {
				break
			}
			for _, issue := range res.Issues {
				si := siteIssue{
					Key:     issue.Key,
					Summary: issue.Fields.Summary,
				}
				if issue.Fields.Assignee != nil {
					si.AssigneeID = issue.Fields.Assignee.AccountID
				}
				if issue.Fields.Status != nil {
					si.StatusID = issue.Fields.Status.ID
				}
				if !fn(si) {
					return
				}
			}
			token = res.NextPageToken
			if token == "" || res.IsLast {
				break
			}
		}
	}
}

// repairStates re-drives workflow states for issues already present in Jira.
// Matching is by summary, which the dataset guarantees is unique.
func (c *Client) repairStates(path string, projects []string) int {
	data, err := loadDataset(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ERROR load dataset: %v\n", err)
		return 0
	}
	wanted := map[string]SeedIssue{}
	allow := map[string]bool{}
	for _, p := range projects {
		allow[p] = true
	}
	for _, i := range data.Issues {
		if allow[i.Project] {
			wanted[i.Summary] = i
		}
	}

	perProject := map[string]projectTypeStatus{}
	for _, p := range projects {
		perProject[p] = projectTypeStatus{statuses: c.projectStatusIDs(p)}
	}

	var keys []string
	var order []SeedIssue
	c.iterSiteIssues(projects, "summary,status", func(issue siteIssue) bool {
		item, ok := wanted[issue.Summary]
		if ok {
			keys = append(keys, issue.Key)
			order = append(order, item)
		}
		return true
	})
	fmt.Printf("matched %d issues to the dataset\n", len(keys))
	moved, reopened := c.applyStates(keys, order, perProject)
	fmt.Printf("transitions: %d, reopened: %d\n", moved, reopened)
	return len(keys)
}

// repairAssignees redistributes assignees across the given accounts.
// Dataset issues follow assignee_slot; other issues keep assigned/unassigned
// status but get spread by key hash so repeated runs are a no-op.
func (c *Client) repairAssignees(path string, projects, assignees []string) int {
	data, err := loadDataset(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ERROR load dataset: %v\n", err)
		return 0
	}
	slots := map[string]*int{}
	for _, i := range data.Issues {
		slots[i.Summary] = i.AssigneeSlot
	}

	changed, skipped := 0, 0
	c.iterSiteIssues(projects, "summary,assignee", func(issue siteIssue) bool {
		target, _ := ResolveAssigneeTarget(issue.Summary, slots, issue.Key, issue.AssigneeID, assignees)
		if target == issue.AssigneeID {
			skipped++
			return true
		}
		// Jira accepts null accountId to unassign.
		body := map[string]any{"accountId": nil}
		if target != "" {
			body["accountId"] = target
		}
		if c.call("PUT", "/rest/api/3/issue/"+url.PathEscape(issue.Key)+"/assignee", body, nil) {
			changed++
		}
		return true
	})
	fmt.Printf("assignees changed: %d, already correct: %d\n", changed, skipped)
	return changed
}
