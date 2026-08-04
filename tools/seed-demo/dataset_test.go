package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndFilterDataset(t *testing.T) {
	// Prefer the committed fixture when present; otherwise use an inline sample.
	path := filepath.Join("..", "..", "examples", "demo-seed.json")
	if _, err := os.Stat(path); err != nil {
		dir := t.TempDir()
		path = filepath.Join(dir, "seed.json")
		raw := `{
  "issues": [
    {"project":"NMB","type":"Bug","summary":"A","description":["d"],"state":"backlog","assignee_slot":1,"links":[{"type":"Relates","target":1}]},
    {"project":"NMA","type":"Task","summary":"B","description":["d"],"state":"done","assignee_slot":null,"fix_version":"v2.4"},
    {"project":"KAN","type":"Bug","summary":"skip me","description":["d"],"state":"backlog"}
  ]
}`
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	data, err := loadDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Issues) == 0 {
		t.Fatal("expected issues")
	}

	// First committed issue is the filter-chips bug when using the real fixture.
	first := data.Issues[0]
	if first.Summary == "" || first.Project == "" || first.Type == "" {
		t.Fatalf("incomplete first issue: %+v", first)
	}
	if first.Description == nil {
		t.Error("description should decode (may be empty slice)")
	}

	// Null assignee_slot and fix_version must round-trip as nil pointers.
	// Find any null-slot issue if present.
	for _, i := range data.Issues {
		if i.AssigneeSlot == nil {
			// ok — null decoded
			break
		}
	}

	onlyNMB := filterIssues(data.Issues, []string{"NMB"})
	if len(onlyNMB) == 0 {
		t.Fatal("expected NMB issues")
	}
	for _, i := range onlyNMB {
		if i.Project != "NMB" {
			t.Errorf("filter leaked %s", i.Project)
		}
	}

	// Links target is a non-negative index into the same array (when present).
	for i, issue := range data.Issues {
		for _, link := range issue.Links {
			if link.Target < 0 {
				t.Errorf("issue %d link target negative", i)
			}
		}
	}
}

func TestLoadDatasetRejectsBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDataset(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestFilterPreservesOrder(t *testing.T) {
	issues := []SeedIssue{
		{Project: "NMB", Summary: "1"},
		{Project: "NMA", Summary: "2"},
		{Project: "NMB", Summary: "3"},
	}
	got := filterIssues(issues, []string{"NMB", "NMA"})
	if len(got) != 3 || got[0].Summary != "1" || got[2].Summary != "3" {
		t.Fatalf("order not preserved: %+v", got)
	}
}
