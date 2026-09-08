package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

// The Jira Server user axis (GDK-1638). A Server user object carries no
// accountId key at all — name and key are both the username, and the email
// is present rather than hidden (payload measured on a live Server
// 11.3.11). A sync that decodes only accountId therefore stores
// assignee_id empty next to a filled assignee_email. This is the FAIL-first
// test for that: the Server origin's own user id — the username — is what
// assignee_id, reporter_id and comment author_id must hold, with no new
// column anywhere.
func TestServerSyncKeysUsersByName(t *testing.T) {
	serverUser := func(name string) map[string]any {
		return map[string]any{
			"self":         "https://server.example/jira/rest/api/2/user?username=" + name,
			"key":          name,
			"name":         name,
			"emailAddress": name + "@server.example",
			"displayName":  map[string]string{"dkim": "Dana Kim", "skim": "Sam Kim"}[name],
			"active":       true,
		}
	}
	issue := map[string]any{
		"id": "9001", "key": "NMB-1",
		"fields": map[string]any{
			"summary":   "a Server issue whose users key by name",
			"project":   map[string]any{"key": "NMB"},
			"issuetype": map[string]any{"id": "10004", "name": "Bug"},
			"status": map[string]any{
				"id": "3", "name": "In Progress",
				"statusCategory": map[string]any{"key": "indeterminate"},
			},
			"created":  "2026-09-01T10:00:00.000+0000",
			"updated":  "2026-09-02T10:00:00.000+0000",
			"assignee": serverUser("dkim"),
			"reporter": serverUser("skim"),
			"creator":  serverUser("skim"),
			"comment": map[string]any{"total": 1, "comments": []any{
				map[string]any{"id": "9101", "author": serverUser("skim"),
					"body": "plain v2 body", "created": "2026-09-01T11:00:00.000+0000",
					"updated": "2026-09-01T11:00:00.000+0000"},
			}},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/2/search":
			// One route serves the count probe (maxResults 0) and every page
			// walk (sync, reconcile key scan) alike: Server has no separate
			// approximate-count route.
			var body struct {
				MaxResults int `json:"maxResults"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("bad search body: %v", err)
				return
			}
			if body.MaxResults == 0 {
				_, _ = w.Write([]byte(`{"startAt":0,"maxResults":0,"total":1,"issues":[]}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"startAt": 0, "maxResults": 100, "total": 1, "issues": []any{issue},
			})
		case "/rest/api/2/status":
			_, _ = w.Write([]byte(`[{"id":"3","name":"In Progress","statusCategory":{"key":"indeterminate"}}]`))
		case "/rest/api/2/priority":
			_, _ = w.Write([]byte(`[{"id":"2","name":"High"}]`))
		case "/rest/api/2/issueLinkType":
			_, _ = w.Write([]byte(`{"issueLinkTypes":[]}`))
		case "/rest/api/2/filter/favourite": // Server has no /filter/my (GDK-1652)
			_, _ = w.Write([]byte(`[]`))
		case "/rest/api/2/field":
			// The lazy sprint-field catalog lookup reads the full field
			// list; an empty one answers "no GreenHopper sprint field".
			_, _ = w.Write([]byte(`[]`))
		case "/rest/api/2/project/NMB/versions":
			_, _ = w.Write([]byte(`[]`))
		case "/rest/agile/1.0/board":
			// A site with no Jira Software answers 404 here (GDK-1654).
			http.Error(w, `{"errorMessages":["no agile"]}`, http.StatusNotFound)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "pat-token")
	client.Retries, client.Backoff = 1, 0 // no sleeping in tests

	// The config a Server workspace actually holds (GDK-1640): site + PAT,
	// no email. FieldMap set so the pass is not discovery mode (which would
	// call GET /field and save the config).
	cfg := &config.Config{
		Kind: config.OriginJiraServer, Site: srv.URL, Token: "pat-token",
		Projects: []string{"NMB"}, FieldMap: map[string]string{"severity": "customfield_10050"},
	}
	db := newMirror(t)
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}

	if got := db.column(t, "issues", "assignee_id", "NMB-1"); got != "dkim" {
		t.Errorf("issues.assignee_id = %q, want the Server username %q", got, "dkim")
	}
	if got := db.column(t, "issues", "reporter_id", "NMB-1"); got != "skim" {
		t.Errorf("issues.reporter_id = %q, want %q", got, "skim")
	}
	if got := db.column(t, "issues", "assignee_email", "NMB-1"); got != "dkim@server.example" {
		t.Errorf("issues.assignee_email = %q, want %q — Server does not hide the email", got, "dkim@server.example")
	}
	var commentAuthor *string
	if err := db.DB.QueryRow(
		`SELECT c.author_id FROM comments c JOIN items i ON i.id = c.item_id WHERE i.key = 'NMB-1'`,
	).Scan(&commentAuthor); err != nil {
		t.Fatalf("comment author_id: %v", err)
	}
	if commentAuthor == nil || *commentAuthor != "skim" {
		t.Errorf("comments.author_id = %v, want the username %q", commentAuthor, "skim")
	}
	// The account catalog (bot axis, GDK-590) keys on the same id: a Server
	// user whose id never lands there is invisible to is_bot.
	var catalogName string
	if err := db.DB.QueryRow(
		`SELECT name FROM users WHERE source_id = ? AND account_id = 'dkim'`, SourceID,
	).Scan(&catalogName); err != nil {
		t.Fatalf("users catalog row for the Server username: %v", err)
	}
	if catalogName != "Dana Kim" {
		t.Errorf("user_accounts.name = %q, want Dana Kim", catalogName)
	}
}
