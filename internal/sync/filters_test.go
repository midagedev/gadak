package sync

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
)

// I4 FAIL-first: email-hidden roster + currentUser() must compile to account_id,
// not the configured email.
func TestCompileFilterCurrentUserUsesAccountID(t *testing.T) {
	people := []jql.Person{{AccountID: "acc-me", Name: "Me", Email: ""}}
	body, applied, unsupported := compileFilter(`assignee = currentUser()`, people, jql.Identity{
		Email:     "me@hidden.example",
		AccountID: "acc-me",
	})
	if len(unsupported) != 0 {
		t.Fatalf("unsupported %v", unsupported)
	}
	var parsed struct {
		Filters jql.Filter `json:"filters"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	got := parsed.Filters.AssigneeEmail
	if len(got) != 1 || got[0] != "acc-me" {
		t.Fatalf("compiled assignee %+v applied %v (want acc-me, not the email)", got, applied)
	}
	if strings.Contains(strings.Join(got, ","), "@") {
		t.Fatalf("compiled to email: %+v", got)
	}
}

func TestPeopleFromDBCollectsReporterID(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://x.example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", ExternalID: "1", Key: "NMA-1",
				Title: "t", CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMA", IssueType: "Task", Status: "Open", StatusCategory: "new",
				Reporter: "Rep", ReporterID: "acc-rp",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	people := peopleFromDB(context.Background(), db)
	found := false
	for _, p := range people {
		if p.AccountID == "acc-rp" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("peopleFromDB missing reporter account id: %+v", people)
	}
}
