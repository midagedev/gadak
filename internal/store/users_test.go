package store

import (
	"context"
	"testing"
)

// GDK-590. The users table is the cached origin account catalog — same
// status as status_catalog (v34): accountType rides on user payloads sync
// already reads, and dropping it meant the mirror could not answer "which
// actor is a bot". UserAccount is source-neutral (Constitution Article 6);
// the bot judgement on account_type values lives in the jira package, not
// here.

func seedUserCatalog(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://j.example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1", Key: "NMB-1",
				Title: "bot touched this", CreatedAt: "2026-07-01T00:00:00.000Z",
				UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: Issue{ProjectKey: "NMB", IssueType: "Bug", StatusID: "1", StatusCategory: "new"},
			Users: []UserAccount{
				{AccountID: "acc-bot", Name: "Claude (build 1)", AccountType: "agent"},
				{AccountID: "acc-sam", Name: "Sam", Email: "sam@example.com", AccountType: "atlassian"},
				// No account id: nothing to key on, must be skipped.
				{Name: "Ghost", AccountType: "app"},
			},
			Comments: []Comment{{
				ID: "jira:c-1", ExternalID: "c1", Author: "Claude (build 1)", AuthorID: "acc-bot",
				BodyText: "claimed and reproduced", CreatedAt: "2026-08-02T10:00:00.000Z",
			}},
			Changelog: []ChangeEntry{{
				ID: "jira:h-1", At: "2026-08-03T10:00:00.000Z", Author: "Claude (build 1)", AuthorID: "acc-bot",
				Field: "status", ToValue: "In Progress", ToID: "3",
			}},
			DevLinks: &DevLinksUpdate{Links: []DevLink{{
				Kind: "pullrequest", URL: "https://scm/1", Title: "fix the bot path",
				Actor: "acc-bot", ActorName: "Claude (build 1)",
			}}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestUserCatalogUpsertAndMerge(t *testing.T) {
	db := openTemp(t)
	seedUserCatalog(t, db)

	rows, err := db.UserCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("users rows = %+v, want 2 (empty account id skipped)", rows)
	}
	byID := map[string]UserAccount{}
	for _, u := range rows {
		byID[u.AccountID] = u
	}
	if got := byID["acc-bot"]; got.Name != "Claude (build 1)" || got.AccountType != "agent" || got.Email != "" {
		t.Fatalf("bot row = %+v", got)
	}
	if got := byID["acc-sam"]; got.Email != "sam@example.com" || got.AccountType != "atlassian" {
		t.Fatalf("human row = %+v", got)
	}

	// A later payload that omits name/accountType (a dev-panel actor, say)
	// must not clobber what the catalog already knows — non-empty wins.
	if _, err := db.UpsertIssues(context.Background(), Batch{Force: true, Records: []IssueRecord{{
		Item: Item{
			ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1", Key: "NMB-1",
			Title: "bot touched this", CreatedAt: "2026-07-01T00:00:00.000Z",
			UpdatedAt: "2026-08-05T00:00:00.000Z",
		},
		Issue: Issue{ProjectKey: "NMB", IssueType: "Bug", StatusID: "1", StatusCategory: "new"},
		Users: []UserAccount{{AccountID: "acc-bot", Name: "Claude (build 2)"}},
	}}}); err != nil {
		t.Fatal(err)
	}
	rows, err = db.UserCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range rows {
		if u.AccountID == "acc-bot" {
			if u.Name != "Claude (build 2)" || u.AccountType != "agent" {
				t.Fatalf("merge kept wrong values: %+v", u)
			}
		}
	}
}

func TestIssueActorsView(t *testing.T) {
	db := openTemp(t)
	seedUserCatalog(t, db)

	// One row per touch: the bot commented, moved the status, and linked a PR.
	rows, err := db.QueryIssueActors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	via := map[string]bool{}
	for _, r := range rows {
		if r.IssueKey != "NMB-1" || r.ActorID != "acc-bot" || r.ActorName != "Claude (build 1)" {
			t.Fatalf("unexpected row %+v", r)
		}
		via[r.Via] = true
	}
	// All three sources feed the axis; the documented bot query joins the
	// catalog on account ids, never on display names.
	for _, want := range []string{"comment", "changelog", "dev_link"} {
		if !via[want] {
			t.Fatalf("issue_actors via = %v, want a %s row", via, want)
		}
	}
}
