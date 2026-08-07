package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// frozenNow is inside the 30-day window of the seeded timestamps below.
var frozenNow = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func feedMe() FeedIdentity {
	return FeedIdentity{
		AccountID:   "acc-me",
		Email:       "me@example.com",
		DisplayName: "Me User",
	}
}

// seedFeedMirror builds issues that exercise every event type and reason.
func seedFeedMirror(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	mentionADF := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[
		{"type":"mention","attrs":{"id":"acc-me","text":"@Me"}},
		{"type":"text","text":" please look"}
	]}]}`)
	if _, err := db.UpsertIssues(context.Background(), Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress", "10001": "done"},
		Priorities: []string{"High", "Medium"},
		Records: []IssueRecord{
			// Assigned to me — status change, field change, comment by other, self comment.
			{
				Item: Item{
					ID: "jira:1", SourceID: "jira", ExternalID: "1", Key: "NMB-1",
					Title: "my assigned bug", Author: "Reporter", AuthorID: "acc-rp",
					CreatedAt: "2026-07-20T00:00:00.000Z", UpdatedAt: "2026-08-04T00:00:00.000Z",
				},
				Issue: Issue{
					ProjectKey: "NMB", Status: "In Progress", StatusID: "3",
					StatusCategory: "inprogress", Priority: "High",
					Assignee: "Me User", AssigneeID: "acc-me", AssigneeEmail: "me@example.com",
					Reporter: "Reporter", ReporterID: "acc-rp", ReporterEmail: "rp@example.com",
				},
				Comments: []Comment{
					{
						ID: "jira:c-other", ExternalID: "c-other", Author: "Other", AuthorID: "acc-ot",
						BodyADF: mentionADF, BodyText: "@Me please look",
						CreatedAt: "2026-08-03T10:00:00.000Z",
					},
					{
						ID: "jira:c-self", ExternalID: "c-self", Author: "Me User", AuthorID: "acc-me",
						BodyText: "I replied to myself", CreatedAt: "2026-08-03T11:00:00.000Z",
					},
				},
				Attachments: []Attachment{{
					ID: "jira:a-1", ExternalID: "a-1", Filename: "trace.png",
					Author: "Other", CreatedAt: "2026-08-03T12:00:00.000Z",
				}},
				Changelog: []ChangeEntry{
					{
						ID: "jira:h-status", At: "2026-08-02T09:00:00.000Z", Author: "Other",
						Field: "status", FromValue: "To Do", ToValue: "In Progress",
					},
					{
						ID: "jira:h-prio", At: "2026-08-02T09:00:00.000Z", Author: "Other",
						Field: "priority", FromValue: "Medium", ToValue: "High",
					},
					{
						ID: "jira:h-labels", At: "2026-08-02T09:00:00.000Z", Author: "Other",
						Field: "labels", FromValue: "", ToValue: "batch",
					},
					{
						ID: "jira:h-assign", At: "2026-08-01T08:00:00.000Z", Author: "Reporter",
						Field: "assignee", FromValue: "Unassigned", ToValue: "Me User",
					},
					// Self status change — must be excluded.
					{
						ID: "jira:h-self", At: "2026-08-02T15:00:00.000Z", Author: "Me User",
						Field: "status", FromValue: "In Progress", ToValue: "In Progress",
					},
				},
			},
			// Reported by me, not assigned to me.
			{
				Item: Item{
					ID: "jira:2", SourceID: "jira", ExternalID: "2", Key: "NMB-2",
					Title: "I reported this", CreatedAt: "2026-07-25T00:00:00.000Z",
					UpdatedAt: "2026-08-01T00:00:00.000Z",
				},
				Issue: Issue{
					ProjectKey: "NMB", Status: "To Do", StatusID: "1", StatusCategory: "new",
					Reporter: "Me User", ReporterID: "acc-me", ReporterEmail: "me@example.com",
					Assignee: "Other", AssigneeID: "acc-ot",
				},
				Comments: []Comment{{
					ID: "jira:c-rp", ExternalID: "c-rp", Author: "Other", AuthorID: "acc-ot",
					BodyText: "looking", CreatedAt: "2026-08-01T10:00:00.000Z",
				}},
			},
			// Watched only — not assignee/reporter.
			{
				Item: Item{
					ID: "jira:3", SourceID: "jira", ExternalID: "3", Key: "NMB-3",
					Title: "watched elsewhere", CreatedAt: "2026-07-10T00:00:00.000Z",
					UpdatedAt: "2026-08-04T00:00:00.000Z",
				},
				Issue: Issue{
					ProjectKey: "NMB", Status: "Done", StatusID: "10001", StatusCategory: "done",
					Assignee: "Other", AssigneeID: "acc-ot",
				},
				Changelog: []ChangeEntry{{
					ID: "jira:h-watch", At: "2026-08-04T08:00:00.000Z", Author: "Other",
					Field: "status", FromValue: "In Progress", ToValue: "Done",
				}},
			},
			// Reopen: reopened_at equals status changelog at.
			{
				Item: Item{
					ID: "jira:4", SourceID: "jira", ExternalID: "4", Key: "NMB-4",
					Title: "reopened bug", CreatedAt: "2026-07-01T00:00:00.000Z",
					UpdatedAt: "2026-08-04T00:00:00.000Z",
				},
				Issue: Issue{
					ProjectKey: "NMB", Status: "To Do", StatusID: "1", StatusCategory: "new",
					Assignee: "Me User", AssigneeID: "acc-me", AssigneeEmail: "me@example.com",
				},
				// Derive will set reopened_at from changelog; seed with Force so we
				// control derived reopen via a done→new transition.
				Changelog: []ChangeEntry{
					{
						ID: "jira:h-done", At: "2026-07-15T00:00:00.000Z", Author: "Other",
						Field: "status", FromValue: "To Do", FromID: "1", ToValue: "Done", ToID: "10001",
					},
					{
						ID: "jira:h-reopen", At: "2026-08-04T07:00:00.000Z", Author: "Other",
						Field: "status", FromValue: "Done", FromID: "10001", ToValue: "To Do", ToID: "1",
					},
				},
			},
			// Unrelated issue — must never appear.
			{
				Item: Item{
					ID: "jira:9", SourceID: "jira", ExternalID: "9", Key: "NMB-9",
					Title: "noise", CreatedAt: "2026-08-01T00:00:00.000Z",
					UpdatedAt: "2026-08-01T00:00:00.000Z",
				},
				Issue: Issue{
					ProjectKey: "NMB", Status: "To Do", StatusID: "1", StatusCategory: "new",
					Assignee: "Stranger", AssigneeID: "acc-xx",
				},
				Comments: []Comment{{
					ID: "jira:c-noise", ExternalID: "c-noise", Author: "Stranger", AuthorID: "acc-xx",
					BodyText: "hi", CreatedAt: "2026-08-01T12:00:00.000Z",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetWatch(context.Background(), "NMB-3", true); err != nil {
		t.Fatal(err)
	}
}

func byEventID(items []FeedItem) map[string]FeedItem {
	out := make(map[string]FeedItem, len(items))
	for _, it := range items {
		out[it.EventID] = it
	}
	return out
}

func hasReason(it FeedItem, reason string) bool {
	for _, r := range it.Reasons {
		if r == reason {
			return true
		}
	}
	return false
}

func TestFeedEventMappingAndReasons(t *testing.T) {
	db := openTemp(t)
	seedFeedMirror(t, db)

	res, err := db.Feed(context.Background(), FeedOpts{Focus: FeedFocusAll, Limit: 100, Me: feedMe(), Now: frozenNow})
	if err != nil {
		t.Fatal(err)
	}
	m := byEventID(res.Items)

	// Noise issue must not appear.
	for _, it := range res.Items {
		if it.IssueKey == "NMB-9" {
			t.Fatalf("unrelated issue in feed: %+v", it)
		}
	}

	// Status change on my assigned issue.
	st, ok := m["cl:jira:h-status"]
	if !ok {
		t.Fatalf("missing status event; items=%v", keysOf(m))
	}
	if st.EventType != "status_changed" {
		t.Errorf("status type %q", st.EventType)
	}
	if !hasReason(st, "assignee") {
		t.Errorf("status reasons %v want assignee", st.Reasons)
	}
	if st.Payload["from"] != "To Do" || st.Payload["to"] != "In Progress" {
		t.Errorf("status payload %+v", st.Payload)
	}

	// Self action excluded.
	if _, ok := m["cl:jira:h-self"]; ok {
		t.Error("self status change should be excluded")
	}
	if _, ok := m["cm:jira:c-self"]; ok {
		t.Error("self comment should be excluded")
	}

	// fields_changed groups priority+labels at same at/author.
	fl, ok := m["fl:jira:1:2026-08-02T09:00:00.000Z"]
	if !ok {
		t.Fatalf("missing fields_changed; items=%v", keysOf(m))
	}
	if fl.EventType != "fields_changed" {
		t.Errorf("fields type %q", fl.EventType)
	}
	fields, _ := fl.Payload["fields"].([]string)
	if len(fields) < 2 {
		// JSON round-trip may produce []any — accept either.
		if raw, ok := fl.Payload["fields"].([]any); ok {
			if len(raw) < 2 {
				t.Errorf("fields payload %+v", fl.Payload["fields"])
			}
		} else {
			t.Errorf("fields payload %+v", fl.Payload["fields"])
		}
	}

	// assigned
	as, ok := m["cl:jira:h-assign"]
	if !ok || as.EventType != "assigned" {
		t.Errorf("assigned event: ok=%v type=%q", ok, as.EventType)
	}

	// comment with mention reason
	cm, ok := m["cm:jira:c-other"]
	if !ok {
		t.Fatal("missing mention comment")
	}
	if !hasReason(cm, "mention") || !hasReason(cm, "assignee") {
		t.Errorf("comment reasons %v", cm.Reasons)
	}
	if cm.Payload["excerpt"] == "" {
		t.Error("comment excerpt empty")
	}

	// attachment
	if at, ok := m["at:jira:a-1"]; !ok || at.EventType != "attachment_added" {
		t.Errorf("attachment: ok=%v", ok)
	} else if at.Payload["filename"] != "trace.png" {
		t.Errorf("filename %v", at.Payload["filename"])
	}

	// reporter reason on NMB-2 comment
	rp, ok := m["cm:jira:c-rp"]
	if !ok || !hasReason(rp, "reporter") {
		t.Errorf("reporter comment: ok=%v reasons=%v", ok, rp.Reasons)
	}

	// watched reason on NMB-3
	w, ok := m["cl:jira:h-watch"]
	if !ok || !hasReason(w, "watched") {
		t.Errorf("watched: ok=%v reasons=%v", ok, w.Reasons)
	}

	// reopened
	re, ok := m["cl:jira:h-reopen"]
	if !ok {
		t.Fatal("missing reopen event")
	}
	if re.EventType != "reopened" {
		t.Errorf("reopen type %q (reopened_at may not have matched)", re.EventType)
	}

	// created events for in-window creates that are relevant
	if cr, ok := m["cr:NMB-1"]; !ok || cr.EventType != "created" {
		t.Errorf("created NMB-1: ok=%v", ok)
	}
}

func TestFeedFocusFilter(t *testing.T) {
	db := openTemp(t)
	seedFeedMirror(t, db)

	all, err := db.Feed(context.Background(), FeedOpts{Focus: FeedFocusAll, Limit: 100, Me: feedMe(), Now: frozenNow})
	if err != nil {
		t.Fatal(err)
	}
	mention, err := db.Feed(context.Background(), FeedOpts{Focus: FeedFocusMention, Limit: 100, Me: feedMe(), Now: frozenNow})
	if err != nil {
		t.Fatal(err)
	}
	if len(mention.Items) == 0 {
		t.Fatal("mention focus empty")
	}
	for _, it := range mention.Items {
		if !hasReason(it, "mention") {
			t.Errorf("mention focus leaked %s reasons=%v", it.EventID, it.Reasons)
		}
	}
	// unread_counts always reflect the full set, not the focus slice.
	if mention.UnreadCounts.All != all.UnreadCounts.All {
		t.Errorf("unread all under mention focus %d, want %d", mention.UnreadCounts.All, all.UnreadCounts.All)
	}
	if mention.UnreadCounts.Mention == 0 {
		t.Error("unread mention count is 0")
	}
}

func TestFeedMarkReadAndUnread(t *testing.T) {
	db := openTemp(t)
	seedFeedMirror(t, db)
	me := feedMe()

	before, err := db.Feed(context.Background(), FeedOpts{Focus: FeedFocusAll, Limit: 100, Me: me, Now: frozenNow})
	if err != nil {
		t.Fatal(err)
	}
	if before.UnreadCounts.All == 0 {
		t.Fatal("expected unread events")
	}

	// Mark one event read.
	target := before.Items[0].EventID
	marked, err := db.MarkFeedRead(context.Background(), MarkFeedReadOpts{
		EventIDs: []string{target}, Me: me, Now: frozenNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if marked.Updated < 1 {
		t.Fatalf("updated %d", marked.Updated)
	}
	if marked.UnreadCounts.All != before.UnreadCounts.All-1 {
		t.Fatalf("unread %d → %d after one mark", before.UnreadCounts.All, marked.UnreadCounts.All)
	}

	after, err := db.Feed(context.Background(), FeedOpts{Focus: FeedFocusAll, Limit: 100, Me: me, Now: frozenNow})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range after.Items {
		if it.EventID == target {
			found = true
			if it.ReadAt == nil {
				t.Error("read_at not set on marked event")
			}
		}
	}
	if !found {
		t.Error("marked event missing from feed")
	}

	// Mark whole issue.
	issueKey := "NMB-2"
	_, err = db.MarkFeedRead(context.Background(), MarkFeedReadOpts{IssueKeys: []string{issueKey}, Me: me, Now: frozenNow})
	if err != nil {
		t.Fatal(err)
	}
	mid, err := db.Feed(context.Background(), FeedOpts{Focus: FeedFocusAll, Limit: 100, Me: me, Now: frozenNow})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range mid.Items {
		if it.IssueKey == issueKey && it.ReadAt == nil {
			t.Errorf("%s still unread", it.EventID)
		}
	}

	// Mark all.
	all, err := db.MarkFeedRead(context.Background(), MarkFeedReadOpts{All: true, Me: me, Now: frozenNow})
	if err != nil {
		t.Fatal(err)
	}
	if all.UnreadCounts.All != 0 {
		t.Fatalf("after mark-all unread=%d", all.UnreadCounts.All)
	}
}

func TestFeedMentionDisabledWithoutAccountID(t *testing.T) {
	db := openTemp(t)
	seedFeedMirror(t, db)
	me := FeedIdentity{Email: "me@example.com", DisplayName: "Me User"} // no AccountID
	res, err := db.Feed(context.Background(), FeedOpts{Focus: FeedFocusAll, Limit: 100, Me: me, Now: frozenNow})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		if hasReason(it, "mention") {
			t.Fatalf("mention reason without account_id: %+v", it)
		}
	}
	// Assignee still works via email.
	found := false
	for _, it := range res.Items {
		if it.IssueKey == "NMB-1" && hasReason(it, "assignee") {
			found = true
		}
	}
	if !found {
		t.Error("assignee via email failed")
	}
}

func keysOf(m map[string]FeedItem) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
