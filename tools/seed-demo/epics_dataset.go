package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// EpicsDataset is the shape of examples/demo-epics.json.
type EpicsDataset struct {
	GeneratedFor string      `json:"generated_for"`
	Epics        []EpicEntry `json:"epics"`
}

// EpicEntry is one epic to create and the child issue summaries to parent under it.
type EpicEntry struct {
	Project        string   `json:"project"`
	Summary        string   `json:"summary"`
	Description    string   `json:"description"`
	ChildSummaries []string `json:"child_summaries"`
}

func loadEpicsDataset(path string) (*EpicsDataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data EpicsDataset
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &data, nil
}
