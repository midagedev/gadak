package server

import (
	"context"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/store"
)

// GDK-590. A bot that only commented never showed up on the people axis —
// members were derived from assignee/reporter alone — and no REST payload
// said which actors are bots. The catalog's account_type (one axis for
// local-origin "agent" and connected "app") now rides on member rows, the
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
		// What a sync pass collected: the local-origin agent and a Cloud app
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

// GDK-590 web slice, server half. Three payloads the UI renders: the detail
// response carries the lifecycle spans the CLI already prints (wait_ms /
// progress_ms), the list rows name the accounts that touched them so the
// actor filter can narrow without another query, and a dev link names who
// attached it — a bot linking a human's PR is the case that hides today.
func TestBotWebPayloads(t *testing.T) {
	db, cfg, _ := fixtureAt(t)
	// One deterministic lifecycle: created 04:00 → in progress 10:00 → done
	// 15:00, every step and the dev link by the agent. Wait and progress are
	// both closed-bounded, so the numbers are exact, not Now-relative.
	if _, err := db.UpsertIssues(context.Background(), store.Batch{Force: true, Records: []store.IssueRecord{{
		Item: store.Item{
			ID: "jira:1008", SourceID: "jira", Kind: "issue", ExternalID: "1008", Key: "NMB-8",
			Title: "agent claims, ships, closes", CreatedAt: "2026-08-10T04:00:00.000Z",
			UpdatedAt: "2026-08-10T15:00:00.000Z",
		},
		Issue: store.Issue{
			ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
			Status: "완료", StatusID: "10001", StatusCategory: "done",
		},
		Users: []store.UserAccount{
			{AccountID: "acc-bot", Name: "Claude (build 1)", AccountType: "agent"},
		},
		Comments: []store.Comment{{
			ID: "jira:c-8", ExternalID: "c-8", Author: "Claude (build 1)", AuthorID: "acc-bot",
			BodyText: "claimed and shipped", CreatedAt: "2026-08-10T04:30:00.000Z",
		}},
		Changelog: []store.ChangeEntry{
			{ID: "jira:h-8a", At: "2026-08-10T10:00:00.000Z", Author: "Claude (build 1)",
				AuthorID: "acc-bot", Field: "status", FromValue: "할 일", FromID: "1",
				ToValue: "진행 중", ToID: "3"},
			{ID: "jira:h-8b", At: "2026-08-10T15:00:00.000Z", Author: "Claude (build 1)",
				AuthorID: "acc-bot", Field: "status", FromValue: "진행 중", FromID: "3",
				ToValue: "완료", ToID: "10001"},
		},
		DevLinks: &store.DevLinksUpdate{Links: []store.DevLink{{
			Kind: "pullrequest", URL: "https://github.com/acme/api/pull/9",
			Title: "ship it", Status: "OPEN",
			Actor: "acc-bot", ActorName: "Claude (build 1)",
		}}},
	}}}); err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)

	// Detail: the spans the CLI's durations line prints, as fields.
	d := decode[detailResponse](t, get(t, h, apiBase+"NMB-8/detail/", nil))
	if d.WaitMs == nil || *d.WaitMs != (6*time.Hour).Milliseconds() {
		t.Fatalf("wait_ms %v, want 6h", d.WaitMs)
	}
	if d.ProgressMs == nil || *d.ProgressMs != (5*time.Hour).Milliseconds() {
		t.Fatalf("progress_ms %v, want 5h", d.ProgressMs)
	}
	// NMB-1 (fixture): created 07-01, into progress 07-03, still in progress —
	// wait is closed, progress runs to Now so only presence is assertable.
	d1 := decode[detailResponse](t, get(t, h, apiBase+"NMB-1/detail/", nil))
	if d1.WaitMs == nil || *d1.WaitMs != (48*time.Hour).Milliseconds() {
		t.Fatalf("NMB-1 wait_ms %v, want 48h", d1.WaitMs)
	}
	if d1.ProgressMs == nil {
		t.Fatal("NMB-1 progress_ms missing for an in-progress issue")
	}
	// NMB-2 (fixture): done with no changelog at all — the chip has nothing
	// to say, so the fields stay absent rather than arriving as null.
	d2 := decode[detailResponse](t, get(t, h, apiBase+"NMB-2/detail/", nil))
	if d2.WaitMs != nil || d2.ProgressMs != nil {
		t.Fatalf("NMB-2 spans = %v/%v, want both absent", d2.WaitMs, d2.ProgressMs)
	}

	// Dev link: who attached the PR rides on the row, separate from the PR's
	// own author (a bot linking a human's PR keeps both axes).
	raw := decode[struct {
		LinkedPRs []LinkedPR `json:"linked_prs"`
	}](t, get(t, h, apiBase+"NMB-8/detail/", nil))
	if len(raw.LinkedPRs) != 1 {
		t.Fatalf("linked_prs %+v", raw.LinkedPRs)
	}
	pr := raw.LinkedPRs[0]
	if got := deref(pr.LinkedByID); got != "acc-bot" {
		t.Fatalf("linked_by_id %q, want acc-bot", got)
	}
	if got := deref(pr.LinkedBy); got != "Claude (build 1)" {
		t.Fatalf("linked_by %q, want Claude (build 1)", got)
	}

	// Bootstrap: list rows carry the accounts that touched them — the actor
	// filter narrows client-side with no extra round trip.
	boot := decode[struct {
		Issues []struct {
			IssueKey string   `json:"issue_key"`
			ActorIDs []string `json:"actor_ids"`
		} `json:"issues"`
	}](t, get(t, h, apiBase+"bootstrap/", nil))
	var touched, untouched bool
	for _, row := range boot.Issues {
		switch row.IssueKey {
		case "NMB-8":
			touched = len(row.ActorIDs) == 1 && row.ActorIDs[0] == "acc-bot"
		case "NMB-2":
			untouched = len(row.ActorIDs) == 0
		}
	}
	if !touched {
		t.Fatal("NMB-8 row missing actor_ids [acc-bot]")
	}
	if !untouched {
		t.Fatal("NMB-2 row carries actor_ids with no actors")
	}
}
