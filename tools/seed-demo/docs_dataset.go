package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// DocsDataset is the shape of examples/demo-docs.json.
type DocsDataset struct {
	GeneratedFor string      `json:"generated_for"`
	Spaces       []DocsSpace `json:"spaces"`
	Pages        []DocsPage  `json:"pages"`
}

// DocsSpace is a Confluence space to ensure before creating pages.
type DocsSpace struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// DocsPage is one wiki page in storage format.
type DocsPage struct {
	Space       string        `json:"space"`
	Title       string        `json:"title"`
	Parent      string        `json:"parent,omitempty"`
	BodyStorage string        `json:"body_storage"`
	Comments    []DocsComment `json:"comments,omitempty"`
	// Labels are Confluence global labels (lowercase-hyphen). Empty or omitted
	// means no labels; the seeder never removes existing labels on the site.
	Labels []string `json:"labels,omitempty"`
}

// DocsComment is a page comment in storage format.
type DocsComment struct {
	BodyStorage string `json:"body_storage"`
}

func loadDocsDataset(path string) (*DocsDataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data DocsDataset
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &data, nil
}
