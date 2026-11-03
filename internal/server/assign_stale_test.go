package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// The server side of the stale-assignee refusal (GDK-2073), contract ↔
// assertion map. The web and phone editors write through this route, and a
// person acting on a stale cache used to silently replace whoever took the
// issue in between — the same defect `gadak assign` closed with its
// preflight read (GDK-2063; cmd/gadak/assign_stale_test.go holds the CLI
// rows). Every row here stages the mirror and the fake origin's
// GET /issue/{key}?fields=status,assignee answer on different facts, the way
// the incident had them; the verdict itself is origin.DecideAssign's, table
// in internal/origin/assign_check_test.go.
//
//	W1 stale: cache ≠ origin → 409 assignee_changed, the holder named, the
//	   refreshed row in the body, the mirror now holding the origin's
//	   assignee, and no PUT reached the origin
//	   TestAssigneeStaleRefusesAndRefreshes (FAIL-first: pre-change the route
//	   wrote — the status and no-PUT rows cannot pass)
//	W2 the target already holds it on the origin → 200, noop:true, no PUT
//	   TestAssigneeNoopIs200AndWritesNothing
//	W3 matching cache → 200, exactly one PUT
//	   TestAssigneeFreshWritesExactlyOnce
//	W4 stale + force:true → the write goes out
//	   TestAssigneeForceWritesPastStaleHolder
//	W5 the preflight read fails → 502 assignee_read_failed, no PUT
//	   TestAssigneeReadFailureRefuses

// assigneeFixture is writable(t) with the db handle handed back: the stale
// rows rewrite the mirror row through the same store the server holds.
func assigneeFixture(t *testing.T) (*fakeJira, http.Handler, *store.DB) {
	t.Helper()
	f := newFakeJira(t)
	db, cfg := fixture(t)
	cfg.Site = f.URL
	return f, New(db, cfg), db
}

// clearMirrorAssignee rewrites NMB-1's mirror row with no assignee — the
// incident's stale-cache shape — through UpsertIssues(Force), the store path
// a sync uses. The fake origin still answers the preflight read with
// 김현철 (acc-hc), so the two disagree exactly as they did live.
func clearMirrorAssignee(t *testing.T, db *store.DB) {
	t.Helper()
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Force:      true,
		Categories: map[string]string{"3": "inprogress", "10001": "done"},
		Priorities: []string{"Highest", "High", "Medium"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1001", SourceID: "jira", Kind: "issue", ExternalID: "1001", Key: "NMB-1",
				Title: "batch worker drops the last page", CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
				Status: "진행 중", StatusID: "3", StatusCategory: "inprogress", Priority: "High",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// assigneeStaleBody is the 409 body as the web and phone parse it.
type assigneeStaleBody struct {
	Error           string `json:"error"`
	CurrentAssignee *struct {
		AccountID   string `json:"account_id"`
		DisplayName string `json:"display_name"`
	} `json:"current_assignee"`
	Issue *struct {
		AssigneeID *string `json:"assignee_id"`
	} `json:"issue"`
}

func TestAssigneeStaleRefusesAndRefreshes(t *testing.T) {
	f, h, db := assigneeFixture(t)
	clearMirrorAssignee(t, db)

	rec := send(t, h, http.MethodPut, apiBase+"NMB-1/assignee/", `{"account_id":"acc-cl"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale assignee → %d: %s", rec.Code, rec.Body.String())
	}
	var body assigneeStaleBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if body.Error != "assignee_changed" {
		t.Fatalf("error code %q, want assignee_changed", body.Error)
	}
	if body.CurrentAssignee == nil || body.CurrentAssignee.AccountID != "acc-hc" ||
		body.CurrentAssignee.DisplayName != "김현철" {
		t.Fatalf("current_assignee = %+v; want the origin's holder 김현철 (acc-hc)", body.CurrentAssignee)
	}
	if body.Issue == nil || body.Issue.AssigneeID == nil || *body.Issue.AssigneeID != "acc-hc" {
		t.Fatalf("issue = %+v; want the refreshed row naming the holder", body.Issue)
	}
	if f.called("PUT /issue/NMB-1/assignee") {
		t.Fatalf("refused assign still wrote: %v", f.calls)
	}
	// The refusal refreshes the refused issue, so the next pick compares
	// against the truth the refusal just named.
	rows, err := db.IssueLitesByKeys(context.Background(), []string{"NMB-1"})
	if err != nil || len(rows) == 0 || rows[0].AssigneeID == nil || *rows[0].AssigneeID != "acc-hc" {
		t.Fatalf("mirror after refusal = %+v (%v); want the origin's holder acc-hc", rows, err)
	}
}

func TestAssigneeNoopIs200AndWritesNothing(t *testing.T) {
	f, h, _ := assigneeFixture(t)
	// The fixture's cache and the fake's read both say acc-hc holds NMB-1,
	// so asking for acc-hc is the no-op whatever the cache says.

	rec := send(t, h, http.MethodPut, apiBase+"NMB-1/assignee/", `{"account_id":"acc-hc"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("no-op assignee → %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Noop  bool `json:"noop"`
		Issue *struct {
			AssigneeID *string `json:"assignee_id"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if !body.Noop {
		t.Fatalf("no-op must say noop:true: %s", rec.Body.String())
	}
	if body.Issue == nil || body.Issue.AssigneeID == nil || *body.Issue.AssigneeID != "acc-hc" {
		t.Fatalf("no-op issue = %+v; want the row naming the holder", body.Issue)
	}
	if f.called("PUT /issue/NMB-1/assignee") {
		t.Fatalf("no-op assign still wrote: %v", f.calls)
	}
}

func TestAssigneeFreshWritesExactlyOnce(t *testing.T) {
	f, h, _ := assigneeFixture(t)
	// Both sides say acc-hc holds it (the fixture's defaults), so
	// reassigning is the deliberate, seen act that stays allowed.

	rec := send(t, h, http.MethodPut, apiBase+"NMB-1/assignee/", `{"account_id":"acc-cl"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("fresh assignee → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1/assignee"]); got != `{"accountId":"acc-cl"}` {
		t.Fatalf("PUT body %s", got)
	}
	puts := 0
	for _, c := range f.calls {
		if c == "PUT /issue/NMB-1/assignee" {
			puts++
		}
	}
	if puts != 1 {
		t.Fatalf("assignee PUTs = %d, want exactly one; calls %v", puts, f.calls)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if _, present := body["noop"]; present {
		t.Fatalf("a replacing write must not carry noop: %s", rec.Body.String())
	}
}

func TestAssigneeForceWritesPastStaleHolder(t *testing.T) {
	f, h, db := assigneeFixture(t)
	clearMirrorAssignee(t, db)

	rec := send(t, h, http.MethodPut, apiBase+"NMB-1/assignee/", `{"account_id":"acc-cl","force":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("forced assignee → %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(f.bodies["PUT /issue/NMB-1/assignee"]); got != `{"accountId":"acc-cl"}` {
		t.Fatalf("PUT body %s; calls %v", got, f.calls)
	}
}

// NMA-9 is in the mirror but not one of the fake's known keys, so the
// preflight GET answers 404 — the read-failed shape (an unreachable origin
// answers the same way from the caller's side).
func TestAssigneeReadFailureRefuses(t *testing.T) {
	f, h, _ := assigneeFixture(t)

	rec := send(t, h, http.MethodPut, apiBase+"NMA-9/assignee/", `{"account_id":"acc-cl"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("read-failed assignee → %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "assignee_read_failed" {
		t.Fatalf("error %q, want assignee_read_failed", got)
	}
	if f.called("PUT /issue/NMA-9/assignee") {
		t.Fatalf("a failed read must not write blind: %v", f.calls)
	}
}
