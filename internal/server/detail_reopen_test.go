package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

/*
 * GDK-1753, server half: the reopen verdict on a changelog row is
 * store.ReopenTransition's answer, serialized as `is_reopen`. Before the
 * fix this file does not compile — historyEntry has no IsReopen (the same
 * FAIL-first form detail_visits_test.go documents for its Part A edit; run
 * `go test ./internal/server/ -run 'TestDetailHistoryReopen|TestFeedReopened' -count=1`
 * and read the build output). The web half (the deleted done-only rule, the
 * timeline reading the field) lives in
 * web/src/components/detail/HistoryTimeline.test.ts and e2e/detail.spec.ts.
 *
 * Clause table:
 *
 *   V1 wire carries the store rule — TestDetailHistoryReopenVerdict
 *        ① in-progress → new is a reopen (the row the old done-only web rule
 *          left grey while SQL counted it — NMB-3's In Progress → Backlog)
 *        ② done → in-progress still is; forward and non-status rows are not
 *        ③ an unknown target id stays not-done/not-new and invents nothing
 *   V2 verdict equals the derived count — same test, last block
 *        ④ reopen_count on the row equals the number of is_reopen history
 *          rows: the wire verdict and Derive read one function, so they
 *          cannot disagree
 *   V3 the key is spelled `is_reopen` — TestDetailHistoryReopenWireSpelling
 *        ⑤ the raw JSON carries the field on every history row (false
 *          included — absence is reserved for older servers)
 *   V4 the feed's reopened event names the same row — TestFeedReopenedEvent
 *        ⑥ the in-progress → new move (as the issue's last reopen) is a
 *          "reopened" feed event, not a bare "status_changed" — feed keys on
 *          ReopenedAt, which only ReopenTransition sets
 */
func TestDetailHistoryReopenVerdict(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	// Categories as the fixture defines them: 1=new, 3=inprogress,
	// 10001=done. "9" is deliberately absent — an uncatalogued status id.
	upsertReopenFixture(t, db)

	d := decode[detailResponse](t, get(t, h, apiBase+"NMB-7/detail/", nil))
	want := map[string]bool{
		"recent-a": false, // new → in-progress: forward
		"recent-b": false, // in-progress → done: forward
		"recent-c": true,  // done → in-progress: the classic reopen
		"recent-d": true,  // in-progress → new: the GDK-1753 row, and the last move
		"recent-e": false, // new → uncatalogued 9
		"recent-f": false, // assignee: never a reopen
	}
	at := stampOf(t)
	painted := 0
	for _, e := range d.History {
		got, ok := want[at(deref(e.At))]
		if !ok {
			t.Fatalf("unexpected history row at %q (%s)", deref(e.At), e.Field)
		}
		if e.IsReopen != got {
			t.Fatalf("history %s %q→%q: is_reopen = %v, want %v",
				deref(e.At), deref(e.From), deref(e.To), e.IsReopen, got)
		}
		if e.IsReopen {
			painted++
		}
	}
	if len(d.History) != len(want) {
		t.Fatalf("history rows = %d, want %d", len(d.History), len(want))
	}

	// ④ The wire verdict and the stored reopen_count agree — one function
	// feeds both (Derive for the column, this loop for the wire).
	rows, err := db.Query(`SELECT reopen_count FROM issues_full WHERE key = 'NMB-7'`)
	if err != nil {
		t.Fatalf("reopen_count: %v", err)
	}
	defer rows.Close()
	var count int
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			t.Fatalf("reopen_count scan: %v", err)
		}
	}
	if count != painted {
		t.Fatalf("reopen_count = %d but %d history rows carry is_reopen — the two consumers disagree", count, painted)
	}
}

func TestDetailHistoryReopenWireSpelling(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	upsertReopenFixture(t, db)

	raw := decode[map[string]json.RawMessage](t, get(t, h, apiBase+"NMB-7/detail/", nil))
	var rows []map[string]any
	if err := json.Unmarshal(raw["history"], &rows); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no history rows in the response")
	}
	for _, r := range rows {
		v, ok := r["is_reopen"]
		if !ok {
			t.Fatalf("history row %v carries no is_reopen — absence is reserved for older servers", r["at"])
		}
		if _, ok := v.(bool); !ok {
			t.Fatalf("is_reopen = %#v, want a bool", v)
		}
	}
	if !strings.Contains(string(raw["history"]), `"is_reopen":true`) {
		t.Fatal(`no "is_reopen":true in the history array — the reopen row did not paint`)
	}
}

func TestFeedReopenedEvent(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	upsertReopenFixture(t, db)

	// Recent stamps: the feed keeps a 30-day window, and a changelog row as
	// old as the detail fixture's July dates would never list.
	rec := get(t, h, apiBase+"feed/?focus=all&limit=50", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET feed → %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []struct {
			IssueKey  string         `json:"issue_key"`
			EventType string         `json:"event_type"`
			Payload   map[string]any `json:"payload"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, it := range body.Items {
		if it.IssueKey != "NMB-7" || it.EventType != "reopened" {
			continue
		}
		// The last reopen of NMB-7 is the in-progress → new move; a feed
		// event calling it "reopened" is the third consumer on the same
		// verdict (feed keys on ReopenedAt, which only ReopenTransition
		// sets — the done-only web rule never touched this path, and this
		// pins it that way).
		if it.Payload["from"] != "진행 중" || it.Payload["to"] != "할 일" {
			t.Fatalf("reopened feed event payload = %v, want 진행 중 → 할 일", it.Payload)
		}
		return
	}
	t.Fatalf("no reopened feed event for NMB-7 among %d items", len(body.Items))
}

// reopenFixtureStamps are the changelog timestamps of upsertReopenFixture,
// newest last. Recent (now-offset) so the feed's 30-day window lists them;
// keyed by suffix so the assertions above read as a story.
var reopenFixtureStamps = func() map[string]string {
	now := time.Now().UTC()
	at := func(daysAgo int) string {
		return now.Add(-time.Duration(daysAgo) * 24 * time.Hour).Format(config.ISOMilli)
	}
	return map[string]string{
		"recent-a": at(6), // new → in-progress
		"recent-b": at(5), // in-progress → done
		"recent-c": at(4), // done → in-progress (classic reopen)
		"recent-d": at(3), // in-progress → new (the GDK-1753 row, last move)
		"recent-e": at(2), // new → uncatalogued
		"recent-f": at(1), // assignee
	}
}()

// stampOf maps a wire `at` back to its fixture key; the exact millisecond
// stamps are run-relative, so the assertions key on the story, not the clock.
func stampOf(t *testing.T) func(string) string {
	t.Helper()
	byStamp := map[string]string{}
	for k, v := range reopenFixtureStamps {
		byStamp[v] = k
	}
	return func(at string) string {
		k, ok := byStamp[at]
		if !ok {
			t.Fatalf("history row at unknown stamp %q", at)
		}
		return k
	}
}

// upsertReopenFixture adds NMB-7: one issue whose changelog walks every
// shape the verdict has to distinguish, in a category map the fixture
// already carries (1=new, 3=inprogress, 10001=done). Assigned to the
// configured identity so the feed lists it; every row authored by someone
// else so isSelfActor does not drop them.
func upsertReopenFixture(t *testing.T, db *store.DB) {
	t.Helper()
	st := reopenFixtureStamps
	sc := func(id, at, fromID, fromVal, toID, toVal string) store.ChangeEntry {
		return store.ChangeEntry{
			ID: id, At: at, Author: "박보고", AuthorID: "acc-rp", Field: "status",
			FromID: fromID, FromValue: fromVal, ToID: toID, ToValue: toVal,
		}
	}
	created := st["recent-a"]
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress", "10001": "done"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1007", SourceID: "jira", ExternalID: "1007", Key: "NMB-7",
				Title: "reopen verdict fixture", CreatedAt: created, UpdatedAt: st["recent-f"],
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", Status: "할 일", StatusID: "1",
				StatusCategory: "new",
				Assignee:       "김현철", AssigneeID: "acc-hc", AssigneeEmail: "hc@example.com",
			},
			Changelog: []store.ChangeEntry{
				sc("jira:h-7a", st["recent-a"], "1", "할 일", "3", "진행 중"),
				sc("jira:h-7b", st["recent-b"], "3", "진행 중", "10001", "완료"),
				sc("jira:h-7c", st["recent-c"], "10001", "완료", "3", "진행 중"),
				sc("jira:h-7d", st["recent-d"], "3", "진행 중", "1", "할 일"),
				sc("jira:h-7e", st["recent-e"], "1", "할 일", "9", "알 수 없음"),
				{
					ID: "jira:h-7f", At: st["recent-f"], Author: "박보고", AuthorID: "acc-rp",
					Field: "assignee", FromValue: "", ToValue: "김현철",
				},
			},
		}},
	}); err != nil {
		t.Fatalf("upsert NMB-7: %v", err)
	}
}
