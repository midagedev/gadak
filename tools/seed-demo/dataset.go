package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// SeedDataset is the shape of examples/demo-seed.json.
type SeedDataset struct {
	Issues []SeedIssue `json:"issues"`
}

// SeedIssue is one authored backlog item projected onto Jira.
type SeedIssue struct {
	Project      string     `json:"project"`
	Type         string     `json:"type"`
	Summary      string     `json:"summary"`
	Description  []string   `json:"description"`
	Priority     string     `json:"priority"`
	Components   []string   `json:"components"`
	FixVersion   *string    `json:"fix_version"`
	Labels       []string   `json:"labels"`
	Environment  *string    `json:"environment"`
	State        string     `json:"state"`
	Reopened     bool       `json:"reopened"`
	AssigneeSlot *int       `json:"assignee_slot"`
	Comments     []string   `json:"comments"`
	Links        []SeedLink `json:"links"`
}

// SeedLink targets another issue by index into the same issues array.
type SeedLink struct {
	Type   string `json:"type"`
	Target int    `json:"target"`
}

func loadDataset(path string) (*SeedDataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data SeedDataset
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &data, nil
}

// filterIssues keeps items whose project is in the allow-list (order preserved).
func filterIssues(issues []SeedIssue, projects []string) []SeedIssue {
	allow := make(map[string]bool, len(projects))
	for _, p := range projects {
		allow[p] = true
	}
	out := make([]SeedIssue, 0, len(issues))
	for _, i := range issues {
		if allow[i.Project] {
			out = append(out, i)
		}
	}
	return out
}
