package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

// feedFixture seeds a small mirror where cfg.Email is the assignee of NMB-1 and
// watches NMB-2. Timestamps sit inside the 30-day window relative to "now".
func feedFixture(t *testing.T) (*store.DB, *config.Config) {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/feed.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	// Keep created/updated recent so the 30-day window always includes them.
	ts := now.Add(-2 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	ts2 := now.Add(-1 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress"},
		Priorities: []string{"High"},
		Records: []store.IssueRecord{
			{
				Item: store.Item{
					ID: "jira:1", SourceID: "jira", ExternalID: "1", Key: "NMB-1",
					Title: "mine", CreatedAt: ts, UpdatedAt: ts2,
				},
				Issue: store.Issue{
					ProjectKey: "NMB", Status: "In Progress", StatusID: "3",
					StatusCategory: "inprogress",
					Assignee:       "김현철", AssigneeID: "acc-hc", AssigneeEmail: "hc@example.com",
				},
				Comments: []store.Comment{{
					ID: "jira:c1", ExternalID: "c1", Author: "Other", AuthorID: "acc-ot",
					BodyText: "ping", BodyADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
					CreatedAt: ts2,
				}},
				Changelog: []store.ChangeEntry{{
					ID: "jira:h1", At: ts2, Author: "Other",
					Field: "status", FromValue: "To Do", ToValue: "In Progress",
				}},
			},
			{
				Item: store.Item{
					ID: "jira:2", SourceID: "jira", ExternalID: "2", Key: "NMB-2",
					Title: "watched", CreatedAt: ts, UpdatedAt: ts2,
				},
				Issue: store.Issue{
					ProjectKey: "NMB", Status: "To Do", StatusID: "1", StatusCategory: "new",
					Assignee: "Other", AssigneeID: "acc-ot",
				},
				Changelog: []store.ChangeEntry{{
					ID: "jira:h2", At: ts2, Author: "Other",
					Field: "priority", FromValue: "Low", ToValue: "High",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetWatch(context.Background(), "NMB-2", true); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Site: "https://x.atlassian.net", Email: "hc@example.com", Token: "tok",
		AccountID: "acc-hc", TokenOwner: "김현철", Projects: []string{"NMB"},
		Features: map[string]bool{"feed": true},
	}
	return db, cfg
}

func TestFeedGetAndMarkRead(t *testing.T) {
	db, cfg := feedFixture(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"feed/?focus=all&limit=50", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET feed → %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []struct {
			EventID   string   `json:"event_id"`
			IssueKey  string   `json:"issue_key"`
			EventType string   `json:"event_type"`
			Reasons   []string `json:"reasons"`
			ReadAt    *string  `json:"read_at"`
		} `json:"items"`
		UnreadCounts struct {
			All      int `json:"all"`
			Assignee int `json:"assignee"`
		} `json:"unread_counts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) == 0 {
		t.Fatal("empty feed")
	}
	if body.UnreadCounts.All == 0 {
		t.Fatal("expected unread")
	}
	// Self-actions of the assignee identity should not list NMB-1 created-by-empty as me;
	// at least one item should carry assignee reason.
	hasAssignee := false
	for _, it := range body.Items {
		for _, r := range it.Reasons {
			if r == "assignee" {
				hasAssignee = true
			}
		}
	}
	if !hasAssignee {
		t.Fatal("no assignee-reason events")
	}

	// Mark one event read.
	eid := body.Items[0].EventID
	markBody := `{"event_ids":[` + jsonString(eid) + `]}`
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPost, apiBase+"feed/read/", strings.NewReader(markBody)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST read → %d %s", rec.Code, rec.Body.String())
	}
	var mark struct {
		Updated      int `json:"updated"`
		UnreadCounts struct {
			All int `json:"all"`
		} `json:"unread_counts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &mark); err != nil {
		t.Fatal(err)
	}
	if mark.Updated < 1 {
		t.Fatalf("updated %d", mark.Updated)
	}
	if mark.UnreadCounts.All != body.UnreadCounts.All-1 {
		t.Fatalf("unread %d → %d", body.UnreadCounts.All, mark.UnreadCounts.All)
	}

	// Mark all.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodPost, apiBase+"feed/read/", strings.NewReader(`{"all":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST all → %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &mark); err != nil {
		t.Fatal(err)
	}
	if mark.UnreadCounts.All != 0 {
		t.Fatalf("after all unread=%d", mark.UnreadCounts.All)
	}
}

func TestFeedFocusAssignee(t *testing.T) {
	db, cfg := feedFixture(t)
	h := New(db, cfg)
	rec := get(t, h, apiBase+"feed/?focus=assignee", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Items []struct {
			Reasons []string `json:"reasons"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, it := range body.Items {
		ok := false
		for _, r := range it.Reasons {
			if r == "assignee" {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("focus=assignee leaked reasons %v", it.Reasons)
		}
	}
}

func TestFeedDefaultFeatureFlag(t *testing.T) {
	// Empty features map → feed defaults on in webConfig / settings.
	doc := webConfig(&config.Config{})
	if !doc.Features["feed"] {
		t.Fatal("feed feature should default true")
	}
	if doc.Features["push"] {
		t.Fatal("push should stay false by default")
	}
	// Explicit off wins.
	doc = webConfig(&config.Config{Features: map[string]bool{"feed": false}})
	if doc.Features["feed"] {
		t.Fatal("explicit feed:false should stick")
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
