package sync

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// GDK-590. accountType must survive the whole pipe: origin payload →
// jira.User → IssueRecord.Users → users catalog. Two origins, two spellings,
// one axis: issuetap local-origin mints "agent" for the accounts behind
// X-Issuetap-Actor (internal/origin/actor_test.go pins that wire shape),
// Atlassian Cloud sends "app" for Connect bots. The mirror caches both the
// same way so the bot judgement never branches on origin kind.
func TestUserCatalogIngest(t *testing.T) {
	site := newSite(t, "en")
	site.issues = []json.RawMessage{raw(t, map[string]any{
		"id": "9001", "key": "NMB-1",
		"fields": map[string]any{
			"summary":   "bot round trip",
			"project":   map[string]any{"key": "NMB"},
			"issuetype": map[string]any{"id": "10002", "name": "Task"},
			"status":    statusObj("1", "en"),
			// Cloud Connect bot as reporter: accountType "app".
			"reporter": map[string]any{"accountId": "712020:abc", "displayName": "GitHub sync",
				"accountType": "app"},
			"comment": map[string]any{"total": 1, "comments": []any{
				// Local-origin agent as commenter: accountType "agent", no email —
				// exactly the object issuetap returns.
				map[string]any{"id": "9101",
					"author":  map[string]any{"accountId": "claude:354bff2b", "displayName": "Claude (build 1)", "accountType": "agent"},
					"body":    adfDoc("claimed and reproduced"),
					"created": "2026-08-10T10:00:00.000+0900", "updated": "2026-08-10T10:00:00.000+0900"},
			}},
			"created": "2026-08-09T10:00:00.000+0900",
			"updated": "2026-08-10T10:00:00.000+0900",
		},
		"changelog": map[string]any{"total": 0, "histories": []any{}},
	})}
	db := newMirror(t)
	cfg := testConfig()
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: site.start()}); err != nil {
		t.Fatal(err)
	}

	users, err := db.UserCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]store.UserAccount{}
	for _, u := range users {
		byID[u.AccountID] = u
	}
	agent, ok := byID["claude:354bff2b"]
	if !ok {
		t.Fatalf("agent missing from catalog: %+v", users)
	}
	if agent.AccountType != "agent" || agent.Name != "Claude (build 1)" || agent.Email != "" {
		t.Fatalf("agent row = %+v", agent)
	}
	app, ok := byID["712020:abc"]
	if !ok {
		t.Fatalf("app account missing from catalog: %+v", users)
	}
	if app.AccountType != "app" || app.Name != "GitHub sync" {
		t.Fatalf("app row = %+v", app)
	}

	// The agent's touch must also reach the actor axis, so "issues this bot
	// touched" resolves without touching display names.
	actors, err := db.QueryIssueActors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range actors {
		if a.IssueKey == "NMB-1" && a.ActorID == "claude:354bff2b" && a.Via == "comment" {
			found = true
		}
	}
	if !found {
		t.Fatalf("agent comment missing from issue_actors: %+v", actors)
	}
}
