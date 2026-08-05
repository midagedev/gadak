package main

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

func docsPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "examples", "demo-docs.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("demo-docs.json required at %s: %v", path, err)
	}
	return path
}

func TestDocsDatasetContract(t *testing.T) {
	path := docsPath(t)
	data, err := loadDocsDataset(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(data.Spaces) == 0 {
		t.Fatal("expected at least one space")
	}
	spaceKeys := make(map[string]bool, len(data.Spaces))
	for _, s := range data.Spaces {
		if s.Key == "" || s.Name == "" {
			t.Fatalf("space missing key or name: %+v", s)
		}
		if spaceKeys[s.Key] {
			t.Fatalf("duplicate space key %s", s.Key)
		}
		spaceKeys[s.Key] = true
	}

	n := len(data.Pages)
	if n < 55 || n > 70 {
		t.Errorf("page count = %d, want 55–70", n)
	}

	// (space, title) unique; parents resolve; no cycles.
	type pageKey struct{ space, title string }
	byKey := make(map[pageKey]*DocsPage, n)
	for i := range data.Pages {
		p := &data.Pages[i]
		if p.Title == "" {
			t.Fatalf("page %d: empty title", i)
		}
		if !spaceKeys[p.Space] {
			t.Errorf("page %q uses undeclared space %q", p.Title, p.Space)
		}
		k := pageKey{p.Space, p.Title}
		if _, ok := byKey[k]; ok {
			t.Errorf("duplicate (space,title): %s / %s", p.Space, p.Title)
		}
		byKey[k] = p
		if p.BodyStorage == "" {
			t.Errorf("page %q: empty body_storage", p.Title)
		}
	}

	for _, p := range data.Pages {
		if p.Parent == "" {
			continue
		}
		if _, ok := byKey[pageKey{p.Space, p.Parent}]; !ok {
			t.Errorf("page %q parent %q not found in space %s", p.Title, p.Parent, p.Space)
		}
	}

	// Cycle detection via DFS on parent edges (child → parent).
	visiting := make(map[pageKey]bool)
	visited := make(map[pageKey]bool)
	var walk func(pageKey) bool
	walk = func(k pageKey) bool {
		if visiting[k] {
			return true // cycle
		}
		if visited[k] {
			return false
		}
		visiting[k] = true
		p := byKey[k]
		if p != nil && p.Parent != "" {
			if walk(pageKey{p.Space, p.Parent}) {
				return true
			}
		}
		visiting[k] = false
		visited[k] = true
		return false
	}
	for k := range byKey {
		if walk(k) {
			t.Errorf("parent cycle involving %s / %s", k.space, k.title)
		}
	}

	// body_storage must parse as XML when wrapped in a root element.
	issueRe := regexp.MustCompile(`NM[ABS]-\d+`)
	pagesWithIssue := 0
	koreanPages := 0
	for _, p := range data.Pages {
		if err := validateXMLFragment(p.BodyStorage); err != nil {
			t.Errorf("page %q body_storage: %v", p.Title, err)
		}
		for _, c := range p.Comments {
			if err := validateXMLFragment(c.BodyStorage); err != nil {
				t.Errorf("page %q comment: %v", p.Title, err)
			}
		}
		if issueRe.MatchString(p.BodyStorage) || issueRe.MatchString(p.Title) {
			pagesWithIssue++
		}
		if hasHangul(p.Title) || hasHangul(p.BodyStorage) {
			koreanPages++
		}
	}
	if pagesWithIssue < 40 {
		t.Errorf("pages mentioning NM[ABS]-\\d+ = %d, want ≥ 40", pagesWithIssue)
	}
	if koreanPages < 8 {
		t.Errorf("Korean pages = %d, want ≥ 8", koreanPages)
	}
}

func validateXMLFragment(fragment string) error {
	dec := xml.NewDecoder(strings.NewReader("<root>" + fragment + "</root>"))
	for {
		_, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func hasHangul(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hangul) {
			return true
		}
	}
	return false
}

func TestLoadDocsDatasetRejectsBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDocsDataset(path); err == nil {
		t.Fatal("expected parse error")
	}
}
