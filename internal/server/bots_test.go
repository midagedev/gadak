package server

import (
	"context"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// GDK-590. A bot that only commented never showed up on the people axis —
// members were derived from assignee/reporter alone — and no REST payload
// said which actors are bots. The catalog's account_type (one axis for
// standalone "agent" and connected "app") now rides on member rows, the
// comment payload, and history carries author_id so attribution does not
// depend on display names.
func TestBotActorSurfaces(t *testing.T) {
	db, cfg, _ := fixtureAt(t)
	if _, err := db.UpsertIssues(context.Background(), store.Batch{Force: true, Records: []store.IssueRecord{{
		Item: store.Item{
			ID: "jira:1007", SourceID: "jira", Kind: "issue", ExternalID: "1007", Key: "NMB-7",
			Title: "bot claims and ships", CreatedAt: "2026-08-10T00:00:00.000Z",
			UpdatedAt: "2026-08-10T00:00:00.000Z",
		},
		Issue: store.Issue{
			ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
			Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
		},
		// What a sync pass collected: the standalone agent and a Cloud app
		// account, both keyed by account id, never by display name.
		Users: []store.UserAccount{
			{AccountID: "acc-bot", Name: "Claude (build 1)", AccountType: "agent"},
			{AccountID: "acc-gh", Name: "GitHub sync", AccountType: "app"},
		},
		Comments: []store.Comment{{
			ID: "jira:c-7", ExternalID: "c-7", Author: "Claude (build 1)", AuthorID: "acc-bot",
			BodyText: "claimed, reproduced, fix up", CreatedAt: "2026-08-10T01:00:00.000Z",
		}},
		Changelog: []store.ChangeEntry{{
			ID: "jira:h-7", At: "2026-08-10T02:00:00.000Z", Author: "Claude (build 1)",
			AuthorID: "acc-bot", Field: "status", FromValue: "할 일", FromID: "1",
			ToValue: "진행 중", ToID: "3",
		}},
	}}}); err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)

	// Bootstrap: the agent is on the people axis even though it is neither
	// assignee nor reporter and Jira hides no email for it at all — it has
	// none. Keyed by account id, same as every email-hidden member.
	boot := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	var bot *member
	for i := range boot.Members {
		if deref(boot.Members[i].JiraAccountID) == "acc-bot" {
			bot = &boot.Members[i]
		}
	}
	if bot == nil {
		t.Fatalf("agent missing from members: %+v", boot.Members)
	}
	if bot.AccountType != "agent" || !bot.IsBot {
		t.Fatalf("agent member = %+v, want account_type agent and is_bot", bot)
	}
	if bot.Email != "" || bot.Name != "Claude (build 1)" {
		t.Fatalf("agent member identity = %+v", bot)
	}

	// Detail: the comment says its author's account type, and history carries
	// author_id so a bot's status move is attributable without matching names.
	d := decode[detailResponse](t, get(t, h, apiBase+"NMB-7/detail/", nil))
	if len(d.Comments) != 1 {
		t.Fatalf("comments %+v", d.Comments)
	}
	if got := deref(d.Comments[0].AuthorAccountType); got != "agent" {
		t.Fatalf("author_account_type %q, want agent", got)
	}
	if len(d.History) != 1 {
		t.Fatalf("history %+v", d.History)
	}
	if got := deref(d.History[0].AuthorID); got != "acc-bot" {
		t.Fatalf("history author_id %q, want acc-bot", got)
	}
}
