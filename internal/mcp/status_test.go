package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// GDK-1113: gadak_status counts the shared comments table split by meaning,
// the same owners `gadak status --json` and doctor use (GDK-628). One
// "comments" figure mixes issue and wiki comments and disagrees with the
// settings runtime on every mirror with wiki comments — the one tool a
// shell-less host checks before trusting answers must not print that number.
func TestStatusCountsSplitCommentsByMeaning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	dbPath := filepath.Join(home, "gadak.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(ctx, store.Source{ID: "confluence", Kind: "confluence", BaseURL: "https://example.invalid/wiki"}); err != nil {
		t.Fatal(err)
	}
	// Two issue comments, one page comment: three rows in the shared table,
	// so any single "comments" figure is wrong for at least one meaning.
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "STD-1", Title: "seeding the status counts",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "STD", IssueType: "Bug", IssueTypeID: "10004",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
			Comments: []store.Comment{
				{ID: "jira:c-1", Author: "Dana", BodyText: "repro", CreatedAt: "2026-01-02T00:00:00.000Z"},
				{ID: "jira:c-2", Author: "Lee", BodyText: "confirmed", CreatedAt: "2026-01-03T00:00:00.000Z"},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertPages(ctx, []store.PageRecord{{
		Item: store.Item{
			ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
			Key: "100", Title: "회의록", BodyText: "논의 사항",
			CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z",
		},
		Page: store.Page{SpaceKey: "ENG", Version: 1, Status: "current"},
		Comments: []store.Comment{{
			ID: "confluence:c-1", Author: "Kim", BodyText: "ok",
			CreatedAt: "2026-01-02T00:00:00.000Z",
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync(ctx, "jira", store.SyncResult{Watermark: "2026-01-03T00:00:00.000Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	cr := callToolRaw(t, dbPath, toolStatus, map[string]any{})
	if cr.IsError {
		t.Fatalf("gadak_status errored: %s", cr.Content[0].Text)
	}
	var st struct {
		Issues        *int `json:"issues"`
		IssueComments *int `json:"issue_comments"`
		PageComments  *int `json:"page_comments"`
		Comments      *int `json:"comments"`
	}
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &st); err != nil {
		t.Fatalf("unmarshal status payload: %v\n%s", err, cr.Content[0].Text)
	}
	if st.IssueComments == nil || *st.IssueComments != 2 {
		t.Errorf("issue_comments = %v, want 2 — the issue share of the shared table:\n%s", st.IssueComments, cr.Content[0].Text)
	}
	if st.PageComments == nil || *st.PageComments != 1 {
		t.Errorf("page_comments = %v, want 1 — the wiki share:\n%s", st.PageComments, cr.Content[0].Text)
	}
	if st.Comments != nil {
		t.Errorf("a mixed \"comments\" figure is back (%d) — it disagrees with the settings runtime on every mirror with wiki comments:\n%s", *st.Comments, cr.Content[0].Text)
	}
	if st.Issues == nil || *st.Issues != 1 {
		t.Errorf("issues = %v, want 1:\n%s", st.Issues, cr.Content[0].Text)
	}

	// The description is what a shell-less agent plans around; it must teach
	// the split, not a figure the payload no longer carries.
	if !strings.Contains(toolStatusDescription, "issue_comments") || !strings.Contains(toolStatusDescription, "page_comments") {
		t.Error("gadak_status description does not name issue_comments/page_comments")
	}
	// The session helper must not be why the test passes: prove the db file
	// the tool read is the one we seeded.
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("seeded mirror vanished: %v", err)
	}
}
