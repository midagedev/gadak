package origin_test

// GDK-586 end-to-end over the serve-routing path: a CLI process whose
// workspace's persist is owned by a live serve attributes its writes to
// its own resolved actor. The actor header must survive the whole chain —
// serveOriginTransport stamps it, the serve's passthrough forwards it, the
// embedded issuetap authors the record as an agent account. The fixture is
// route_test.go's liveStandaloneServe (advertised serve, pinned passthrough).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
)

func TestRoutedWriteAttributesToActor(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "claude:354bff2b|Claude (build 1)")
	t.Setenv("CLAUDECODE", "")
	cfg, _, _, _ := liveStandaloneServe(t)

	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !origin.TransportIsServe(c.HTTP.Transport) {
		t.Fatalf("transport %T: test needs the routed serve shape", c.HTTP.Transport)
	}
	ctx := context.Background()
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "routed actor probe",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue over serve: %v", err)
	}
	if _, err := c.AddComment(ctx, key, jira.Doc("agent note via serve", nil), nil, false); err != nil {
		t.Fatalf("AddComment over serve: %v", err)
	}

	status, body, err := c.Raw(ctx, http.MethodGet,
		fmt.Sprintf("/rest/api/3/issue/%s/comment", key), nil, false)
	if err != nil || status != http.StatusOK {
		t.Fatalf("GET comments over serve: status=%d err=%v", status, err)
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
		t.Fatalf("routed comment author accountId=%q, want the actor slug claude:354bff2b", a.AccountID)
	}
	if a.AccountType != "agent" {
		t.Fatalf("routed comment author accountType=%q, want agent", a.AccountType)
	}
	if a.DisplayName != "Claude (build 1)" {
		t.Fatalf("routed comment author displayName=%q, want X-Issuetap-Actor-Name", a.DisplayName)
	}
}
