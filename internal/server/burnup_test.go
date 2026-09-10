package server

// internal/server burn-up endpoint — contract ↔ assertion map (GDK-1710).
//
//	C1 the wire days are the store's own reconstruction, field for
//	   field — the web sparkline and `gadak sprint show` read the
//	   same numbers the CLI path computes                → TestBurnupEndpoint
//	C2 an id the mirror does not hold is a 404 with a code the
//	   client can branch on                              → TestBurnupEndpoint
//	C3 a non-numeric id is a 400 before the store is asked → TestBurnupEndpoint
//	C4 a history-less origin (Linear) gets has_history=false
//	   and no days at all — the current-state projection is
//	   withheld, not drawn as a flat sprint              → TestBurnupEndpoint
//
// FAIL-first: before the route existed these got a 404 from handleNotFound;
// the handler and its registration are what turn them green, the same form
// retro_test.go records.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/store"
)

func burnupSeed(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.ReplaceAgile(context.Background(), "jira",
		[]store.BoardRow{{ID: 1, Name: "NMB board", Type: "scrum"}},
		[]store.SprintRow{{ID: 7, BoardID: 1, Name: "Sprint 7", State: "closed",
			StartAt: "2026-08-03T10:00:00.000Z", EndAt: "2026-08-14T10:00:00.000Z",
			CompleteAt: "2026-08-14T18:00:00.000Z"}}); err != nil {
		t.Fatal(err)
	}
	// One issue in scope from the start, done mid-sprint; one added on the
	// 6th and finished the same day. The concrete anchor rows below are what
	// the parity half cannot fake by comparing the store to itself.
	seed := func(n, key, arrival, done string) {
		t.Helper()
		if _, err := db.UpsertIssues(context.Background(), store.Batch{
			Categories: map[string]string{"1": "new", "3": "inprogress", "5": "done"},
			Records: []store.IssueRecord{{
				Item: store.Item{
					ID: "jira:" + n, SourceID: "jira", Kind: "issue", ExternalID: n, Key: key,
					Title: "burnup " + key, CreatedAt: "2026-07-01T00:00:00Z", UpdatedAt: "2026-08-10T00:00:00Z",
				},
				Issue: store.Issue{ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10001",
					Status: "완료", StatusID: "5", StatusCategory: "done"},
				Changelog: []store.ChangeEntry{
					{ID: "s-" + n, At: arrival, Field: "sprint", ToID: "7", ToValue: "Sprint 7"},
					{ID: "i-" + n, At: arrival, Field: "status", FromID: "1", ToID: "3", FromValue: "할 일", ToValue: "진행 중"},
					{ID: "d-" + n, At: done, Field: "status", FromID: "3", ToID: "5", FromValue: "진행 중", ToValue: "완료"},
				},
			}},
		}); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}
	seed("9001", "NMB-9001", "2026-08-01T12:00:00Z", "2026-08-05T09:00:00Z")
	seed("9002", "NMB-9002", "2026-08-06T08:00:00Z", "2026-08-06T23:59:00Z")
}

func TestBurnupEndpoint(t *testing.T) {
	db, cfg := fixture(t)
	burnupSeed(t, db)
	h := New(db, cfg)

	// C1: the wire doc is the store's own answer, field for field.
	rec := get(t, h, apiBase+"sprints/7/burnup/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("burnup: %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Burnup     store.BurnupDoc `json:"burnup"`
		HasHistory bool            `json:"has_history"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want, err := db.SprintBurnup(context.Background(), 7, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Burnup.ID != want.ID || got.Burnup.Name != want.Name || len(got.Burnup.Days) != len(want.Days) {
		t.Fatalf("wire header %+v, want %+v", got.Burnup, want)
	}
	for i := range want.Days {
		if got.Burnup.Days[i] != want.Days[i] {
			t.Errorf("wire day %d = %+v, want %+v", i, got.Burnup.Days[i], want.Days[i])
		}
	}
	if !got.HasHistory {
		t.Fatal("a jira sprint must say has_history=true")
	}
	// The concrete anchors: arrival grows scope on the 6th and the 23:59
	// completion still lands on the 6th — the day-bucket contract the
	// sparkline stacks on.
	byDate := map[string]store.BurnupDay{}
	for _, d := range got.Burnup.Days {
		byDate[d.Date] = d
	}
	if d := byDate["2026-08-05"]; d.Scope != 1 || d.Completed != 1 {
		t.Errorf("08-05 = %+v, want scope 1 completed 1", d)
	}
	if d := byDate["2026-08-06"]; d.Scope != 2 || d.Started != 2 || d.Completed != 2 {
		t.Errorf("08-06 = %+v, want 2/2/2 — mid-sprint arrival and the 23:59 done both land that day", d)
	}

	// C2: unknown id.
	rec = get(t, h, apiBase+"sprints/999/burnup/", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown sprint: %d %s", rec.Code, rec.Body.String())
	}
	if code := jsonField(t, rec.Body.Bytes(), "error"); code != "sprint_not_found" {
		t.Fatalf("error code = %q", code)
	}

	// C3: non-numeric id, refused before the store is asked.
	rec = get(t, h, apiBase+"sprints/abc/burnup/", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d %s", rec.Code, rec.Body.String())
	}

	// C4: a Linear cycle cannot answer the question — the endpoint says so
	// and withholds the projection instead of shipping flat days.
	if err := db.UpsertSource(context.Background(), store.Source{ID: "lin", Kind: "linear", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceAgile(context.Background(), "lin", nil,
		[]store.SprintRow{{ID: 3, BoardID: 1, Name: "Cycle 3", State: "active",
			StartAt: "2026-08-03T10:00:00.000Z", EndAt: "2026-08-14T10:00:00.000Z"}}); err != nil {
		t.Fatal(err)
	}
	rec = get(t, h, apiBase+"sprints/3/burnup/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("linear burnup: %d %s", rec.Code, rec.Body.String())
	}
	got = struct {
		Burnup     store.BurnupDoc `json:"burnup"`
		HasHistory bool            `json:"has_history"`
	}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode linear: %v", err)
	}
	if got.HasHistory {
		t.Fatal("a linear cycle must say has_history=false")
	}
	if got.Burnup.Days != nil {
		t.Fatalf("a history-less origin shipped days: %+v", got.Burnup.Days)
	}
}

func jsonField(t *testing.T, body []byte, key string) string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return m[key]
}
