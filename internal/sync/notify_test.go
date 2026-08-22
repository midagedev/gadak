package sync

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

type fakeNotifier struct {
	calls [][2]string
	err   error
	// unsupported is the test knob for notifier.Supported. Default false so
	// existing fakes keep the pre-GDK-349 path (deliver, then advance).
	unsupported bool
}

func (f *fakeNotifier) Notify(title, body string) error {
	f.calls = append(f.calls, [2]string{title, body})
	return f.err
}

func (f *fakeNotifier) Supported() bool { return !f.unsupported }

func TestSummarizeFeedNotify(t *testing.T) {
	items := []store.FeedItem{
		{IssueKey: "NMB-12", EventType: "comment_added", ActorName: "Marco", Summary: "Fix login"},
		{IssueKey: "NMB-13", EventType: "status_changed", ActorName: "Ada", Summary: "Other"},
		{IssueKey: "NMB-14", EventType: "assigned", ActorName: "Bea", Summary: "Third"},
	}
	title, body := summarizeFeedNotify(items)
	wantTitle := "NMB-12 comment by Marco +2 more"
	if title != wantTitle {
		t.Errorf("title %q, want %q", title, wantTitle)
	}
	if body != "Fix login" {
		t.Errorf("body %q, want issue title only", body)
	}
}

func TestNotifyAfterSyncBootstrapNoFire(t *testing.T) {
	db := openSyncDB(t)
	seedFeedIssue(t, db, "NMB-1", "acc-me", nil)
	n := &fakeNotifier{}
	cfg := &config.Config{
		Email: "me@example.com", AccountID: "acc-me", TokenOwner: "Me",
	}
	if err := notifyAfterSync(context.Background(), db, cfg, n); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 0 {
		t.Fatalf("bootstrap should not notify, got %+v", n.calls)
	}
	st, err := db.SyncState(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastNotifiedAt == nil || *st.LastNotifiedAt == "" {
		t.Fatal("expected last_notified_at seeded")
	}
}

func TestNotifyAfterSyncFiresForNewEvents(t *testing.T) {
	db := openSyncDB(t)
	now := time.Now().UTC().Format(config.ISOMilli)
	seedFeedIssue(t, db, "NMB-9", "acc-me", []store.Comment{{
		ID: "jira:c-new", ExternalID: "c-new", Author: "Marco", AuthorID: "acc-marco",
		BodyText: "secret body must not appear", CreatedAt: now, UpdatedAt: now,
	}})
	cfg := &config.Config{
		Email: "me@example.com", AccountID: "acc-me", TokenOwner: "Me",
	}
	past := time.Now().UTC().Add(-2 * time.Hour).Format(config.ISOMilli)
	if err := db.SetLastNotifiedAt(context.Background(), SourceID, past); err != nil {
		t.Fatal(err)
	}

	n := &fakeNotifier{}
	if err := notifyAfterSync(context.Background(), db, cfg, n); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 1 {
		t.Fatalf("want 1 notification, got %+v", n.calls)
	}
	title, body := n.calls[0][0], n.calls[0][1]
	if !strings.Contains(title, "NMB-9") || !strings.Contains(title, "comment") || !strings.Contains(title, "Marco") {
		t.Errorf("title %q missing key pieces", title)
	}
	if strings.Contains(body, "secret") {
		t.Errorf("body leaked comment text: %q", body)
	}
	if body != "Watched title" {
		t.Errorf("body %q, want issue title", body)
	}
	// feed_reads must stay empty — notify ≠ read.
	res, err := db.Feed(context.Background(), store.FeedOpts{Focus: store.FeedFocusAll, Me: store.FeedIdentity{
		AccountID: "acc-me", Email: "me@example.com",
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		if it.ReadAt != nil {
			t.Errorf("event %s has read_at after notify", it.EventID)
		}
	}
	st, _ := db.SyncState(context.Background(), SourceID)
	if st.LastNotifiedAt == nil || *st.LastNotifiedAt <= past {
		t.Errorf("last_notified_at not advanced: %v", st.LastNotifiedAt)
	}
}

func TestNotifyDisabled(t *testing.T) {
	db := openSyncDB(t)
	off := false
	cfg := &config.Config{Notify: &off, Email: "me@example.com", AccountID: "acc-me"}
	n := &fakeNotifier{}
	if err := notifyAfterSync(context.Background(), db, cfg, n); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 0 {
		t.Fatalf("notify:false must skip, got %+v", n.calls)
	}
}

func TestNotifyFailureDoesNotAdvanceWatermark(t *testing.T) {
	db := openSyncDB(t)
	now := time.Now().UTC().Format(config.ISOMilli)
	seedFeedIssue(t, db, "NMB-2", "acc-me", []store.Comment{{
		ID: "jira:c-fail", ExternalID: "c-fail", Author: "Marco", AuthorID: "acc-m",
		BodyText: "x", CreatedAt: now, UpdatedAt: now,
	}})
	past := time.Now().UTC().Add(-time.Hour).Format(config.ISOMilli)
	if err := db.SetLastNotifiedAt(context.Background(), SourceID, past); err != nil {
		t.Fatal(err)
	}
	n := &fakeNotifier{err: errBoom}
	cfg := &config.Config{Email: "me@example.com", AccountID: "acc-me"}
	if err := notifyAfterSync(context.Background(), db, cfg, n); err == nil {
		t.Fatal("expected error from notifier")
	}
	st, _ := db.SyncState(context.Background(), SourceID)
	if st.LastNotifiedAt == nil || *st.LastNotifiedAt != past {
		t.Errorf("watermark should stay at %q, got %v", past, st.LastNotifiedAt)
	}
}

// GDK-349: an OS that cannot deliver (Windows no-op) must not consume the
// watermark. Pre-fix, OSNotifier.Notify returned nil on that path, so
// notifyAfterSync treated it as success and advanced. A later toast
// implementation then still sees the pending events.
func TestNotifyAfterSyncUnsupportedDoesNotAdvanceWatermark(t *testing.T) {
	db := openSyncDB(t)
	now := time.Now().UTC().Format(config.ISOMilli)
	seedFeedIssue(t, db, "NMB-3", "acc-me", []store.Comment{{
		ID: "jira:c-win", ExternalID: "c-win", Author: "Marco", AuthorID: "acc-m",
		BodyText: "x", CreatedAt: now, UpdatedAt: now,
	}})
	past := time.Now().UTC().Add(-time.Hour).Format(config.ISOMilli)
	if err := db.SetLastNotifiedAt(context.Background(), SourceID, past); err != nil {
		t.Fatal(err)
	}
	n := &fakeNotifier{unsupported: true}
	cfg := &config.Config{Email: "me@example.com", AccountID: "acc-me"}
	if err := notifyAfterSync(context.Background(), db, cfg, n); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 0 {
		t.Errorf("unsupported must not notify, got %+v", n.calls)
	}
	st, err := db.SyncState(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	got := ""
	if st.LastNotifiedAt != nil {
		got = *st.LastNotifiedAt
	}
	if got != past {
		t.Errorf("watermark should stay at %q, got %q", past, got)
	}
}

func TestNotifyAfterSyncUnsupportedStillBootstraps(t *testing.T) {
	db := openSyncDB(t)
	seedFeedIssue(t, db, "NMB-1", "acc-me", nil)
	n := &fakeNotifier{unsupported: true}
	cfg := &config.Config{
		Email: "me@example.com", AccountID: "acc-me", TokenOwner: "Me",
	}
	if err := notifyAfterSync(context.Background(), db, cfg, n); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 0 {
		t.Fatalf("bootstrap should not notify, got %+v", n.calls)
	}
	st, err := db.SyncState(context.Background(), SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastNotifiedAt == nil || *st.LastNotifiedAt == "" {
		t.Fatal("expected last_notified_at seeded even when unsupported")
	}
}

func TestOSNotifyCommandSupportMatchesSupported(t *testing.T) {
	cases := map[string]bool{
		"darwin":  true,
		"linux":   true,
		"windows": false,
		"js":      false,
		"freebsd": false,
		"android": false,
	}
	for goos, want := range cases {
		if got := osNotifyCommand(goos, "t", "b") != nil; got != want {
			t.Errorf("osNotifyCommand(%s) supported=%v, want %v", goos, got, want)
		}
	}
	if (osNotifyCommand(runtime.GOOS, "t", "b") != nil) != (OSNotifier{}.Supported()) {
		t.Fatalf("OSNotifier.Supported() drifted from osNotifyCommand(%s)", runtime.GOOS)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

const errBoom = errString("boom")

func openSyncDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/gadak.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(context.Background(), store.Source{ID: SourceID, Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedFeedIssue(t *testing.T, db *store.DB, key, assigneeID string, comments []store.Comment) {
	t.Helper()
	itemID := "jira:" + key
	now := time.Now().UTC().Format(config.ISOMilli)
	old := time.Now().UTC().AddDate(0, 0, -5).Format(config.ISOMilli)
	batch := store.Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress"},
		Priorities: []string{"Medium"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: itemID, SourceID: SourceID, Kind: "issue", ExternalID: key,
				Key: key, Title: "Watched title", Author: "Reporter", AuthorID: "acc-r",
				CreatedAt: old, UpdatedAt: now,
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "1",
				Status: "In Progress", StatusID: "3", StatusCategory: "inprogress",
				Priority: "Medium", Assignee: "Me", AssigneeID: assigneeID,
				AssigneeEmail: "me@example.com",
			},
			Comments: comments,
		}},
	}
	if _, err := db.UpsertIssues(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
}
