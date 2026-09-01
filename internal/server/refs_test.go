package server

// GDK-1032: hydration reads the target's live state out of another
// workspace's own mirror. What must hold: a mirrored target hydrates, an
// unmirrored one is a pointer with no live half (never an error), and a
// plain external URL is passed through untouched.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

func seedTeamMirror(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, "profiles", "team")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"3": "inprogress"},
		Records: []store.IssueRecord{{
			Item: store.Item{ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "NMA-9", Title: "the team's issue",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z"},
			Issue: store.Issue{ProjectKey: "NMA", IssueType: "Task", IssueTypeID: "10001",
				Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
				Assignee: "Alice", AssigneeID: "acc-a"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHydrateRefs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	seedTeamMirror(t, home)

	refs := hydrateRefs(context.Background(), []store.RemoteLink{
		{ID: "60001", Relationship: "blocked by", URL: "gadak://team/NMA-9", Title: "NMA-9"},
		{ID: "60002", Relationship: "relates to", URL: "gadak://elsewhere/ABC-1", Title: "ABC-1"},
		{ID: "60003", URL: "https://example.com/doc", Title: "a doc"},
	})
	if len(refs) != 3 {
		t.Fatalf("want 3 refs, got %d", len(refs))
	}

	// Mirrored target: the live half is read now, from that workspace's file.
	live := refs[0]
	if !live.Hydrated {
		t.Fatalf("mirrored target did not hydrate: %+v", live)
	}
	if live.Workspace != "team" || live.Key != "NMA-9" {
		t.Fatalf("target not parsed: %+v", live)
	}
	if live.Status != "진행 중" || live.Category != "inprogress" {
		t.Fatalf("status not hydrated: %+v", live)
	}
	if live.Assignee != "Alice" || live.Summary != "the team's issue" {
		t.Fatalf("assignee/summary not hydrated: %+v", live)
	}

	// A workspace this machine does not mirror: still a valid pointer.
	miss := refs[1]
	if miss.Hydrated {
		t.Fatalf("unmirrored workspace must not claim hydration: %+v", miss)
	}
	if miss.Workspace != "elsewhere" || miss.Key != "ABC-1" || miss.URL == "" {
		t.Fatalf("unmirrored pointer lost its identity: %+v", miss)
	}

	// A plain URL has no local half and no workspace to guess at.
	ext := refs[2]
	if ext.Hydrated || ext.Workspace != "" || ext.Key != "" {
		t.Fatalf("external URL must stay external: %+v", ext)
	}
	if ext.URL != "https://example.com/doc" || ext.Title != "a doc" {
		t.Fatalf("external URL altered: %+v", ext)
	}
}

func TestParseRefURL(t *testing.T) {
	cases := []struct {
		in       string
		ws, key  string
		expectOK bool
	}{
		{"gadak://work/NMA-9", "work", "NMA-9", true},
		{"gadak://work/", "", "", false},
		{"gadak:///NMA-9", "", "", false},
		{"https://example.com/x", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		ws, key, ok := parseRefURL(c.in)
		if ok != c.expectOK || ws != c.ws || key != c.key {
			t.Errorf("parseRefURL(%q) = %q,%q,%v want %q,%q,%v", c.in, ws, key, ok, c.ws, c.key, c.expectOK)
		}
	}
}
