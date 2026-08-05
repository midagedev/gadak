package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// seedEpics loads an epics dataset and creates Epic issues, then parents
// matching child issues under them. dry prints a plan and never hits the network.
func (c *Client) seedEpics(path string, dry bool) int {
	data, err := loadEpicsDataset(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR load epics dataset: %v\n", err)
		return 1
	}
	return c.seedEpicsData(data, dry)
}

func (c *Client) seedEpicsData(data *EpicsDataset, dry bool) int {
	if data == nil || len(data.Epics) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR epics dataset is empty")
		return 1
	}

	byProject := map[string][]EpicEntry{}
	childTotal := 0
	for _, e := range data.Epics {
		byProject[e.Project] = append(byProject[e.Project], e)
		childTotal += len(e.ChildSummaries)
	}

	fmt.Printf("epics plan: %d epics, %d child assignments across %d projects\n",
		len(data.Epics), childTotal, len(byProject))
	// Stable-ish project order for humans (NMB, NMA, NMS appear in dataset order).
	seenProj := map[string]bool{}
	var projOrder []string
	for _, e := range data.Epics {
		if !seenProj[e.Project] {
			seenProj[e.Project] = true
			projOrder = append(projOrder, e.Project)
		}
	}
	for _, p := range projOrder {
		nChildren := 0
		for _, e := range byProject[p] {
			nChildren += len(e.ChildSummaries)
			fmt.Printf("  epic [%s] %s  children=%d\n", e.Project, e.Summary, len(e.ChildSummaries))
		}
		fmt.Printf("  project %s: %d epics, %d children\n", p, len(byProject[p]), nChildren)
	}
	if dry {
		fmt.Printf("dry-run: no Jira calls\n")
		return 0
	}

	created, skippedEpic, parented, skippedParent, unmatched := 0, 0, 0, 0, 0
	epicTypeCache := map[string]string{}

	for _, e := range data.Epics {
		typeID := epicTypeCache[e.Project]
		if typeID == "" {
			typeID = c.epicTypeID(e.Project)
			if typeID == "" {
				fmt.Fprintf(os.Stderr, "ERROR no Epic issue type (hierarchyLevel==1) for %s\n", e.Project)
				return 1
			}
			epicTypeCache[e.Project] = typeID
			fmt.Printf("  [%s] epic type id=%s\n", e.Project, typeID)
		}

		epicKey, found, ok := c.findIssueBySummary(e.Project, e.Summary)
		if !ok {
			fmt.Fprintf(os.Stderr, "ERROR search for epic %q in %s failed\n", e.Summary, e.Project)
			return 1
		}
		if found && epicKey != "" {
			fmt.Printf("  skip existing epic [%s] %s\n", epicKey, e.Summary)
			skippedEpic++
			c.pace()
		} else {
			key, cok := c.createEpic(e.Project, typeID, e.Summary, e.Description)
			if !cok || key == "" {
				fmt.Fprintf(os.Stderr, "ERROR create epic [%s] %s\n", e.Project, e.Summary)
				return 1
			}
			epicKey = key
			created++
			fmt.Printf("  created epic [%s] %s\n", epicKey, e.Summary)
			c.pace()
		}

		for _, childSum := range e.ChildSummaries {
			childKey, foundChild, sok := c.findIssueBySummary(e.Project, childSum)
			if !sok {
				fmt.Fprintf(os.Stderr, "  WARN search failed for child %q in %s\n", childSum, e.Project)
				unmatched++
				c.pace()
				continue
			}
			if !foundChild || childKey == "" {
				fmt.Fprintf(os.Stderr, "  WARN no match for child summary %q in %s\n", childSum, e.Project)
				unmatched++
				c.pace()
				continue
			}
			parentKey, hasParent, pok := c.issueParent(childKey)
			if !pok {
				fmt.Fprintf(os.Stderr, "  WARN could not read parent for %s\n", childKey)
				unmatched++
				c.pace()
				continue
			}
			if hasParent && parentKey != "" {
				fmt.Printf("  skip parented %s (parent %s)\n", childKey, parentKey)
				skippedParent++
				c.pace()
				continue
			}
			if c.setParent(childKey, epicKey) {
				parented++
				fmt.Printf("  parent %s → %s\n", childKey, epicKey)
			} else {
				fmt.Fprintf(os.Stderr, "  WARN set parent failed for %s → %s\n", childKey, epicKey)
				unmatched++
			}
			c.pace()
		}
	}

	fmt.Printf("done. epics created=%d skipped=%d parented=%d already-parented=%d unmatched=%d\n",
		created, skippedEpic, parented, skippedParent, unmatched)
	return 0
}

// epicTypeID returns the issue type id with hierarchyLevel == 1 (Epic) for the
// project. Names are ignored because Jira localizes them per account.
func (c *Client) epicTypeID(project string) string {
	var res struct {
		IssueTypes []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			HierarchyLevel int    `json:"hierarchyLevel"`
			Subtask        bool   `json:"subtask"`
		} `json:"issueTypes"`
	}
	if !c.call("GET", "/rest/api/3/project/"+url.PathEscape(project), nil, &res) {
		return ""
	}
	for _, it := range res.IssueTypes {
		if it.HierarchyLevel == 1 {
			return it.ID
		}
	}
	return ""
}

// findIssueBySummary looks up one issue by project + approximate summary match.
// Returns key, found, callOK. When multiple hits, prefers an exact summary match.
func (c *Client) findIssueBySummary(project, summary string) (key string, found, ok bool) {
	// Phrase search; escape embedded quotes for JQL.
	escaped := strings.ReplaceAll(summary, `"`, `\"`)
	jql := fmt.Sprintf(`project=%s AND summary ~ "\"%s\""`, project, escaped)
	pathQ := "/rest/api/3/search/jql?jql=" + url.QueryEscape(jql) +
		"&maxResults=10&fields=" + url.QueryEscape("summary")
	var res struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if !c.call("GET", pathQ, nil, &res) {
		return "", false, false
	}
	if len(res.Issues) == 0 {
		return "", false, true
	}
	for _, issue := range res.Issues {
		if issue.Fields.Summary == summary {
			return issue.Key, true, true
		}
	}
	// Fall back to first hit when phrase search is fuzzy.
	return res.Issues[0].Key, true, true
}

// issueParent returns the current parent key if any.
func (c *Client) issueParent(issueKey string) (parentKey string, hasParent, ok bool) {
	var res struct {
		Fields struct {
			Parent *struct {
				Key string `json:"key"`
			} `json:"parent"`
		} `json:"fields"`
	}
	if !c.call("GET", "/rest/api/3/issue/"+url.PathEscape(issueKey)+"?fields=parent", nil, &res) {
		return "", false, false
	}
	if res.Fields.Parent == nil || res.Fields.Parent.Key == "" {
		return "", false, true
	}
	return res.Fields.Parent.Key, true, true
}

func (c *Client) createEpic(project, typeID, summary, description string) (key string, ok bool) {
	paras := splitDescription(description)
	if len(paras) == 0 {
		paras = []string{summary}
	}
	payload := map[string]any{
		"fields": map[string]any{
			"project":     map[string]string{"key": project},
			"issuetype":   map[string]string{"id": typeID},
			"summary":     summary,
			"description": adf(paras),
		},
	}
	var res struct {
		Key string `json:"key"`
		ID  string `json:"id"`
	}
	if !c.call("POST", "/rest/api/3/issue", payload, &res) {
		return "", false
	}
	return res.Key, true
}

func (c *Client) setParent(childKey, epicKey string) bool {
	return c.call("PUT", "/rest/api/3/issue/"+url.PathEscape(childKey), map[string]any{
		"fields": map[string]any{
			"parent": map[string]string{"key": epicKey},
		},
	}, nil)
}

// splitDescription breaks a free-text description into ADF paragraphs on
// sentence boundaries when possible.
func splitDescription(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Prefer paragraph breaks; otherwise whole string as one paragraph.
	if strings.Contains(s, "\n\n") {
		var out []string
		for _, p := range strings.Split(s, "\n\n") {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return []string{s}
}
