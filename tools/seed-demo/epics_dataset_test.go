package main

import (
	"os"
	"path/filepath"
	"testing"
)

func epicsPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "examples", "demo-epics.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("demo-epics.json required at %s: %v", path, err)
	}
	return path
}

func seedSummariesPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "examples", "demo-seed.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("demo-seed.json required at %s: %v", path, err)
	}
	return path
}

func TestEpicsDatasetContract(t *testing.T) {
	epicsData, err := loadEpicsDataset(epicsPath(t))
	if err != nil {
		t.Fatal(err)
	}
	seedData, err := loadDataset(seedSummariesPath(t))
	if err != nil {
		t.Fatal(err)
	}

	seedSummaries := make(map[string]bool, len(seedData.Issues))
	seedByProject := map[string]int{}
	for _, issue := range seedData.Issues {
		seedSummaries[issue.Summary] = true
		seedByProject[issue.Project]++
	}

	byProject := map[string][]EpicEntry{}
	for _, e := range epicsData.Epics {
		if e.Project == "" {
			t.Fatalf("epic missing project: %+v", e)
		}
		if e.Summary == "" {
			t.Fatalf("epic missing summary in %s", e.Project)
		}
		if e.Description == "" {
			t.Errorf("epic %q: empty description", e.Summary)
		}
		n := len(e.ChildSummaries)
		if n < 5 || n > 15 {
			t.Errorf("epic %q: child_summaries=%d, want 5–15", e.Summary, n)
		}
		byProject[e.Project] = append(byProject[e.Project], e)
	}

	// 4–6 epics per project for the three demo projects.
	for _, proj := range []string{"NMB", "NMA", "NMS"} {
		n := len(byProject[proj])
		if n < 4 || n > 6 {
			t.Errorf("project %s: %d epics, want 4–6", proj, n)
		}
	}

	// Child summaries must exist in demo-seed.json; no duplicate assignments.
	seenChild := map[string]string{} // summary → epic summary
	matched := 0
	for _, e := range epicsData.Epics {
		for _, child := range e.ChildSummaries {
			if child == "" {
				t.Errorf("epic %q: empty child summary", e.Summary)
				continue
			}
			if !seedSummaries[child] {
				t.Errorf("epic %q: child_summary not in demo-seed.json: %q", e.Summary, child)
				continue
			}
			if prev, ok := seenChild[child]; ok {
				t.Errorf("duplicate child assignment %q under %q and %q", child, prev, e.Summary)
				continue
			}
			seenChild[child] = e.Summary
			matched++
		}
	}

	totalSeed := len(seedData.Issues)
	if totalSeed == 0 {
		t.Fatal("demo-seed.json has no issues")
	}
	coverage := float64(matched) / float64(totalSeed) * 100
	if coverage < 55 {
		t.Errorf("coverage = %.1f%% (%d/%d), want ≥ 55%%", coverage, matched, totalSeed)
	}
	t.Logf("epics=%d children=%d coverage=%.1f%% seed=%d",
		len(epicsData.Epics), matched, coverage, totalSeed)
}

func TestLoadEpicsDatasetRejectsBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadEpicsDataset(path); err == nil {
		t.Fatal("expected parse error")
	}
}
