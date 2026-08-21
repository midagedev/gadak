package jira

import (
	"encoding/json"
	"testing"
)

// GDK-590. accountType is the one axis that separates bot workers from
// humans: standalone issuetap mints "agent" for actor accounts (the wire
// shape internal/origin/actor_test.go pins), Atlassian Cloud uses "app" for
// Connect/app accounts and "atlassian"/"customer" for humans. The judgement
// lives in this package only — web, CLI and MCP all go through
// IsBotAccountType, never through a display name.

func TestUserAccountTypeParsed(t *testing.T) {
	// Standalone agent, exactly as issuetap writes it: no email.
	var agent User
	if err := json.Unmarshal([]byte(
		`{"accountId":"claude:354bff2b","displayName":"Claude (build 1)","accountType":"agent"}`), &agent); err != nil {
		t.Fatal(err)
	}
	if agent.AccountID != "claude:354bff2b" || agent.DisplayName != "Claude (build 1)" || agent.AccountType != "agent" {
		t.Fatalf("agent = %+v", agent)
	}
	// Connected Cloud app account.
	var app User
	if err := json.Unmarshal([]byte(
		`{"accountId":"712020:abc","displayName":"GitHub sync","accountType":"app","emailAddress":"bots@connect.example"}`), &app); err != nil {
		t.Fatal(err)
	}
	if app.AccountType != "app" {
		t.Fatalf("app = %+v", app)
	}
	// A human, and a payload that carries no accountType at all.
	var human User
	if err := json.Unmarshal([]byte(`{"accountId":"5b10a2844c20165700ede21g","displayName":"Sam","accountType":"atlassian"}`), &human); err != nil {
		t.Fatal(err)
	}
	if human.AccountType != "atlassian" {
		t.Fatalf("human = %+v", human)
	}
}

func TestIsBotAccountType(t *testing.T) {
	for tc, want := range map[string]bool{
		"agent": true, "app": true,
		"atlassian": false, "customer": false, "": false,
	} {
		if got := IsBotAccountType(tc); got != want {
			t.Errorf("IsBotAccountType(%q) = %v, want %v", tc, got, want)
		}
	}
}
