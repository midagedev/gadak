package store

import (
	"context"
	"testing"
)

// TestGDK1179ChangelogIDIsPerItem: a child-row id is only unique inside its
// item. issuetap mints history/comment/attachment ids from an in-memory
// counter seeded at open, so two gadak processes writing concurrently mint
// the same id for different issues — durably, in the origin's persist. A
// global PRIMARY KEY made the second item's re-read fail forever with
// "UNIQUE constraint failed: changelog.id", and `gadak sync` (the remedy the
// warning names) failed with it too.
func TestGDK1179ChangelogIDIsPerItem(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}

	rec := func(n, key string) IssueRecord {
		return IssueRecord{
			Item: Item{
				ID: "jira:" + n, SourceID: "jira", Kind: "issue", ExternalID: n,
				Key: key, Title: "shared child ids " + key,
				CreatedAt: ago(3), UpdatedAt: ago(1),
			},
			Issue: Issue{
				ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10001",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
			// Same ids on both items — what two concurrent origin sessions produce.
			Changelog:   []ChangeEntry{{ID: "h9", At: ago(1), Field: "status", FromValue: "To Do", ToValue: "In Progress"}},
			Comments:    []Comment{{ID: "c9", Author: "Ada", BodyText: "hi", CreatedAt: ago(1)}},
			Attachments: []Attachment{{ID: "a9", Filename: "trace.har", CreatedAt: ago(1)}},
		}
	}

	if _, err := db.UpsertIssues(ctx, Batch{Categories: fixtureCategories, Records: []IssueRecord{rec("20001", "NMB-91")}}); err != nil {
		t.Fatalf("first item: %v", err)
	}
	if _, err := db.UpsertIssues(ctx, Batch{Categories: fixtureCategories, Records: []IssueRecord{rec("20002", "NMB-92")}}); err != nil {
		t.Fatalf("second item with the same child ids: %v", err)
	}

	for _, tbl := range []string{"changelog", "comments", "attachments"} {
		var n int
		if err := db.sql.QueryRowContext(ctx, `SELECT count(*) FROM `+tbl).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Fatalf("%s has %d rows, want 2 (one per item)", tbl, n)
		}
	}
}
