package jira

import (
	"encoding/json"
	"testing"
)

// GDK-590. accountType is the one axis that separates bot workers from
// humans: built-in issuetap mints "agent" for actor accounts (the wire
// shape internal/origin/actor_test.go pins), Atlassian Cloud uses "app" for
// Connect/app accounts and "atlassian"/"customer" for humans. The judgement
// lives in this package only — web, CLI and MCP all go through
// IsBotAccountType, never through a display name.

func TestUserAccountTypeParsed(t *testing.T) {
	// Built-in agent, exactly as issuetap writes it: no email.
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

// TestUserIDAnswersTheOriginsUserAxis (GDK-1638): the two deployments key
// users differently. A Server user object carries no accountId key at all
// — name and key are both the username, and the email is present rather
// than hidden (measured on a live Server 11.3.11). A Cloud user object
// carries accountId and no name. ID is the single accessor for "the id
// this origin keys users by"; surfaces stop reading AccountID directly.
func TestUserIDAnswersTheOriginsUserAxis(t *testing.T) {
	// The measured Server payload (GDK-1638 spec), verbatim in shape.
	var server User
	if err := json.Unmarshal([]byte(`{
		"self": "https://server.example/jira/rest/api/2/user?username=dkim",
		"key": "dkim", "name": "dkim",
		"emailAddress": "dkim@server.example",
		"displayName": "Dana Kim", "active": true
	}`), &server); err != nil {
		t.Fatal(err)
	}
	if server.AccountID != "" {
		t.Errorf("Server payload must not mint an accountId: %+v", server)
	}
	if server.Name != "dkim" || server.Key != "dkim" || server.Email != "dkim@server.example" {
		t.Errorf("Server decode = %+v", server)
	}
	if got := server.ID(); got != "dkim" {
		t.Errorf("Server ID() = %q, want the username dkim", got)
	}
	// Cloud: accountId present, no name key.
	var cloud User
	if err := json.Unmarshal([]byte(
		`{"accountId":"5b10a2844c20165700ede21g","displayName":"Sam","accountType":"atlassian"}`), &cloud); err != nil {
		t.Fatal(err)
	}
	if got := cloud.ID(); got != "5b10a2844c20165700ede21g" {
		t.Errorf("Cloud ID() = %q, want the accountId", got)
	}
	// ID never falls back to a display name: a payload carrying only a
	// display name answers empty, so callers refuse rather than key on it.
	var nobody User
	if err := json.Unmarshal([]byte(`{"displayName":"Some Display"}`), &nobody); err != nil {
		t.Fatal(err)
	}
	if got := nobody.ID(); got != "" {
		t.Errorf("ID() = %q, want empty — a display name is not a user id", got)
	}
}
