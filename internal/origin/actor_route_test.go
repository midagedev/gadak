package origin_test

// GDK-586 over the post-GDK-936 path: a second embedding (CLI-shaped
// ForgetLive) still attributes writes to the process actor. Pairing
// remotes keep the same header on serveOriginTransport
// (TestServeTransportCarriesActor).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
)

func TestSecondSessionWriteAttributesToActor(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "claude:354bff2b|Claude (build 1)")
	t.Setenv("CLAUDECODE", "")
	cfg, _ := standaloneHome(t)

	if _, err := origin.Client(cfg); err != nil {
		t.Fatal(err)
	}
	origin.ForgetLive()
	origin.ResetInProcess()

	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("transport %T: want embedded", c.HTTP.Transport)
	}
	ctx := context.Background()
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "second-session actor probe",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if _, err := c.AddComment(ctx, key, jira.Doc("agent note", nil), nil, false); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	status, body, err := c.Raw(ctx, http.MethodGet,
		fmt.Sprintf("/rest/api/3/issue/%s/comment", key), nil, false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("GET comments: status=%d err=%v", status, err)
	}
	var page struct {
		Comments []struct {
			Author struct {
				AccountID   string `json:"accountId"`
				DisplayName string `json:"displayName"`
				AccountType string `json:"accountType"`
			} `json:"author"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Comments) == 0 {
		t.Fatal("no comments stored")
	}
	a := page.Comments[len(page.Comments)-1].Author
	if a.AccountID != "claude:354bff2b" {
		t.Fatalf("comment author accountId=%q, want the actor slug claude:354bff2b", a.AccountID)
	}
	if a.AccountType != "agent" {
		t.Fatalf("comment author accountType=%q, want agent", a.AccountType)
	}
	if a.DisplayName != "Claude (build 1)" {
		t.Fatalf("comment author displayName=%q, want X-Issuetap-Actor-Name", a.DisplayName)
	}
}
