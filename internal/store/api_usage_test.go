package store

import (
	"context"
	"testing"
)

func TestAPIUsageAccumulatesAndOrders(t *testing.T) {
	db := openTemp(t)
	if db.SchemaVersion() != len(migrations) {
		t.Fatalf("schema version %d, want %d", db.SchemaVersion(), len(migrations))
	}

	if err := db.AddAPIUsage(context.Background(), "2026-08-04", APIUsageDelta{
		Requests: 10, Throttled: 1, ServerErrors: 2, Retries: 3, WaitMS: 100,
		LastThrottledAt: "2026-08-04T10:00:00.000Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddAPIUsage(context.Background(), "2026-08-04", APIUsageDelta{
		Requests: 5, Throttled: 1, ServerErrors: 0, Retries: 1, WaitMS: 50,
		LastThrottledAt: "2026-08-04T14:00:00.000Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddAPIUsage(context.Background(), "2026-08-05", APIUsageDelta{
		Requests: 7, Retries: 0,
	}); err != nil {
		t.Fatal(err)
	}
	// Empty last_throttled_at on a later flush must not wipe the stored value.
	if err := db.AddAPIUsage(context.Background(), "2026-08-04", APIUsageDelta{Requests: 1}); err != nil {
		t.Fatal(err)
	}

	days, err := db.APIUsage(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 {
		t.Fatalf("len = %d, want 2", len(days))
	}
	if days[0].Day != "2026-08-05" || days[1].Day != "2026-08-04" {
		t.Fatalf("order = %s, %s; want newest first", days[0].Day, days[1].Day)
	}
	d4 := days[1]
	if d4.Requests != 16 || d4.Throttled != 2 || d4.ServerErrors != 2 || d4.Retries != 4 || d4.WaitMS != 150 {
		t.Errorf("2026-08-04 accum = %+v", d4)
	}
	if d4.LastThrottledAt == nil || *d4.LastThrottledAt != "2026-08-04T14:00:00.000Z" {
		t.Errorf("last_throttled_at = %v, want 14:00Z", d4.LastThrottledAt)
	}
	if days[0].Requests != 7 {
		t.Errorf("2026-08-05 requests = %d", days[0].Requests)
	}
}

func TestNewDBSchemaVersionMatchesMigrations(t *testing.T) {
	db := openTemp(t)
	want := len(migrations)
	if got := db.SchemaVersion(); got != want {
		t.Fatalf("SchemaVersion = %d, want %d", got, want)
	}
	var uv int
	if err := db.sql.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != want {
		t.Fatalf("PRAGMA user_version = %d, want %d", uv, want)
	}
}

// GDK-628: PageCommentCount is the wiki share of the comments table — the
// counterpart of IssueCommentCount's issue share. Together they split what
// TableCount("comments") mixes; on this fixture the raw table count is the
// sum of the two on purpose, the same divergence the settings runtime test
// pins (internal/server TestSettingsRuntimeCountsMatchIssueLites).
func TestPageCommentCountSplitsCommentsTable(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira", BaseURL: "https://j.example"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(ctx, Source{ID: "confluence", Kind: "confluence", BaseURL: "https://j.example/wiki"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(ctx, Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "NMB-1", Title: "billing webhook retry",
				CreatedAt: ago(2), UpdatedAt: ago(1),
			},
			Issue: Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
			Comments: []Comment{{ID: "jira:c-1", Author: "Dana", BodyText: "repro", CreatedAt: ago(1)}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertPages(ctx, []PageRecord{{
		Item: Item{
			ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
			Key: "100", Title: "로그인이 실패할 때", BodyText: "로그인 가이드",
			CreatedAt: ago(2), UpdatedAt: ago(1),
		},
		Page:     Page{SpaceKey: "ENG", Version: 1, Status: "current"},
		Comments: []Comment{{ID: "confluence:c1", Author: "Lee", BodyText: "ok", CreatedAt: ago(1)}},
	}}); err != nil {
		t.Fatal(err)
	}

	issueComments, err := db.IssueCommentCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pageComments, err := db.PageCommentCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if issueComments != 1 || pageComments != 1 {
		t.Fatalf("issue_comments = %d, page_comments = %d; want 1 and 1", issueComments, pageComments)
	}
	raw, err := db.TableCount(ctx, "comments")
	if err != nil {
		t.Fatal(err)
	}
	if raw != 2 {
		t.Fatalf("comments table = %d, want 2 (one issue + one page comment)", raw)
	}
}
