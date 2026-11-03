package main

// The assign stale check (GDK-2063), contract ↔ assertion map. The defect:
// a 15-hour-old mirror showed no assignee while a colleague had taken the
// issue on the origin, and `gadak assign` replaced them with no warning —
// the output named only the new assignee, and the overwrite surfaced later
// in the changelog. Every test here stages the mirror and the origin's
// GET /issue/{key}?fields=status,assignee answer on different facts, the
// way the incident had them.
//
//	S1 stale: mirror ≠ origin → exit 75, the holder named, --force named,
//	   nothing written, the issue refreshed into the mirror
//	   TestAssignStaleMirrorRefusesAndRefreshes (FAIL-first: pre-change the
//	   assign wrote — the PUT-count and exit-code rows cannot pass)
//	S2 stale + --force → the write goes out and still names the replaced
//	   holder
//	   TestAssignForceWritesPastStaleHolder
//	S3 fresh replace: mirror == origin → the write goes out and the human
//	   line names both sides; --json carries previous_assignee(_id)
//	   TestAssignFreshReplacesAndNamesHolder, TestAssignJSONPreviousAssignee
//	S4 unassign (`-`) reports the same way
//	   TestAssignUnassignNamesHolder
//	S5 target already holds it on the origin → no write, exit 0
//	   TestAssignAlreadyAssignedIsNoWrite
//	S6 the read fails, or the origin cannot answer it → refuse, never write
//	   blind; --force still writes
//	   TestAssignReadFailureRefuses, TestAssignOriginWithoutReadFaceRefuses,
//	   TestAssignForceWritesPastFailedRead
//	S7 batch: per-line stale error (the same message), per-line force, other
//	   lines continue
//	   TestAssignBatchStaleLineAmongThree
//	S8 dry run pays the read and reports the verdict; a would-refuse dry
//	   run refuses
//	   TestAssignDryRunStaleRefuses, TestAssignDryRunFreshPrintsVerdict
//	S9 an exact display name wins among substring hits; two exact matches
//	   stay refused
//	   TestResolveAccountExactDisplayNameWins,
//	   TestResolveAccountExactNameTwiceStillRefused

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// clearMirrorAssignee rewrites NMB-1's mirror row with no assignee — the
// shape the incident's stale mirror had — through UpsertIssues(Force), the
// store path a sync uses. The fake origin still answers the preflight read
// with Dana (acc-hc), so the two disagree exactly as they did live.
func clearMirrorAssignee(t *testing.T) {
	t.Helper()
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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

// staleExit asserts err is the stale refusal with exit 75.
func staleExit(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("stale assign must refuse")
	}
	var code *exitCodeError
	if !errors.As(err, &code) || code.ExitCode() != exitAssignStale {
		t.Fatalf("refusal must carry exit %d, got %v", exitAssignStale, err)
	}
	return err.Error()
}

func TestAssignStaleMirrorRefusesAndRefreshes(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	clearMirrorAssignee(t)

	out, err := capture(t, func() error {
		return cmdAssign([]string{"NMB-1", "marco@example.com"})
	})
	msg := staleExit(t, err)
	for _, want := range []string{"Dana Whitfield", "--force", "mirror showed (nobody)"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal %q missing %q", msg, want)
		}
	}
	if f.called("PUT /issue/NMB-1/assignee") {
		t.Fatalf("refused assign still wrote: %v", f.calls)
	}
	// The refusal refreshes the refused issue, so the next read shows the
	// holder the refusal just named.
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	row := lookupOne(db, "NMB-1")
	if row == nil || row.AssigneeID == nil || *row.AssigneeID != "acc-hc" {
		t.Fatalf("mirror after refusal = %+v (out %q); want the origin's holder acc-hc", row, out)
	}
}

func TestAssignForceWritesPastStaleHolder(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	clearMirrorAssignee(t)

	out, err := capture(t, func() error {
		return cmdAssign([]string{"NMB-1", "marco@example.com", "--force"})
	})
	if err != nil {
		t.Fatalf("force assign: %v\n%s", err, out)
	}
	if body := f.bodies["PUT /issue/NMB-1/assignee"]; !strings.Contains(body, `"accountId":"acc-mr"`) {
		t.Fatalf("force did not write: calls %v", f.calls)
	}
	if !strings.Contains(out, "NMB-1 assignee: Dana Whitfield → Marco Reyes") {
		t.Fatalf("force output must still name the replaced holder:\n%s", out)
	}
}

func TestAssignFreshReplacesAndNamesHolder(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	// Both sides say Dana holds it (the fake's defaults), so reassigning is
	// the deliberate, seen act that stays allowed.

	out, err := capture(t, func() error {
		return cmdAssign([]string{"NMB-1", "marco@example.com"})
	})
	if err != nil {
		t.Fatalf("assign: %v\n%s", err, out)
	}
	if !strings.Contains(out, "NMB-1 assignee: Dana Whitfield → Marco Reyes") {
		t.Fatalf("output must name the replaced assignee:\n%s", out)
	}
	if !strings.Contains(out, "Marco Reyes") || !strings.Contains(out, "NMB-1") {
		t.Fatalf("output lost the row:\n%s", out)
	}
}

func TestAssignJSONPreviousAssignee(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdAssign([]string{"NMB-1", "marco@example.com", "--json"})
	})
	if err != nil {
		t.Fatalf("assign --json: %v\n%s", err, out)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &body); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, out)
	}
	if body["previous_assignee"] != "Dana Whitfield" || body["previous_assignee_id"] != "acc-hc" {
		t.Fatalf("previous assignee keys = %v / %v", body["previous_assignee"], body["previous_assignee_id"])
	}
	if body["issue"] == nil {
		t.Fatalf("JSON lost the issue row: %v", body)
	}
}

func TestAssignUnassignNamesHolder(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error { return cmdAssign([]string{"NMB-1", "-"}) })
	if err != nil {
		t.Fatalf("unassign: %v\n%s", err, out)
	}
	if body := f.bodies["PUT /issue/NMB-1/assignee"]; !strings.Contains(body, `"accountId":null`) {
		t.Fatalf("unassign sent %s", body)
	}
	if !strings.Contains(out, "NMB-1 assignee: Dana Whitfield → (none)") {
		t.Fatalf("unassign must name the holder it removed:\n%s", out)
	}
}

func TestAssignAlreadyAssignedIsNoWrite(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	// The origin's holder is Dana (the fake's default read); resolving
	// "Dana Whitfield" must land on her accountId, so the write is a no-op.
	f.searchUsers = func(string) string {
		return `[{"accountId":"acc-hc","displayName":"Dana Whitfield","emailAddress":"","active":true}]`
	}

	out, err := capture(t, func() error {
		return cmdAssign([]string{"NMB-1", "Dana Whitfield"})
	})
	if err != nil {
		t.Fatalf("already-assigned must be exit 0: %v\n%s", err, out)
	}
	if !strings.Contains(out, "NMB-1 already assigned to Dana Whitfield — nothing to do") {
		t.Fatalf("already-assigned line:\n%s", out)
	}
	if f.called("PUT /issue/NMB-1/assignee") {
		t.Fatalf("no-op assign still wrote: %v", f.calls)
	}

	// The unassign twin: nobody holds it on the origin, `-` asks for that.
	f.issueStatusJSON = `{"fields":{"status":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}},"assignee":null}}`
	clearMirrorAssignee(t)
	out, err = capture(t, func() error { return cmdAssign([]string{"NMB-1", "-"}) })
	if err != nil {
		t.Fatalf("already-unassigned must be exit 0: %v\n%s", err, out)
	}
	if !strings.Contains(out, "NMB-1 already unassigned — nothing to do") {
		t.Fatalf("already-unassigned line:\n%s", out)
	}
	if f.called("PUT /issue/NMB-1/assignee") {
		t.Fatalf("no-op unassign still wrote: %v", f.calls)
	}
}

// assignProbeWriter drives assignOne's preflight without the HTTP fake:
// SetAssignee records instead of writing, IssueAssignee answers readErr or
// the staged holder. searchUsersStub supplies the rest of Writer.
type assignProbeWriter struct {
	*searchUsersStub
	holder  *jira.User
	readErr error
	writes  []string
}

func (w *assignProbeWriter) IssueAssignee(context.Context, string) (*jira.User, error) {
	if w.readErr != nil {
		return nil, w.readErr
	}
	return w.holder, nil
}

func (w *assignProbeWriter) SetAssignee(_ context.Context, key, id string) error {
	w.writes = append(w.writes, key+"="+id)
	return nil
}

func TestAssignReadFailureRefuses(t *testing.T) {
	mirror(t, "https://example.invalid")
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	probe := &assignProbeWriter{searchUsersStub: &searchUsersStub{
		users: []jira.User{{AccountID: "acc-mr", DisplayName: "Marco Reyes"}},
	}, readErr: errors.New("connection reset")}

	_, err = assignOne(context.Background(), &config.Config{}, db, probe, "", "NMB-1", "marco@example.com", false)
	if err == nil {
		t.Fatal("a failed origin read must refuse, not write blind")
	}
	for _, want := range []string{"connection reset", "--force", "not writing blind"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q missing %q", err, want)
		}
	}
	if len(probe.writes) != 0 {
		t.Fatalf("refused write still sent: %v", probe.writes)
	}
}

// A writer with no IssueAssignee face at all (a future origin): the same
// failed-read refusal, naming the limitation.
func TestAssignOriginWithoutReadFaceRefuses(t *testing.T) {
	mirror(t, "https://example.invalid")
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stub := &searchUsersStub{users: []jira.User{{AccountID: "acc-mr", DisplayName: "Marco Reyes"}}}

	_, err = assignOne(context.Background(), &config.Config{}, db, stub, "", "NMB-1", "marco@example.com", false)
	if err == nil {
		t.Fatal("an origin that cannot answer the read must refuse")
	}
	if !strings.Contains(err.Error(), "cannot read one issue's current assignee") {
		t.Fatalf("refusal must name the missing read: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("refusal must name --force: %v", err)
	}
}

func TestAssignForceWritesPastFailedRead(t *testing.T) {
	mirror(t, "https://example.invalid")
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// The mirror row names Dana, so the report still names her as replaced —
	// what the caller believed they were replacing.
	probe := &assignProbeWriter{searchUsersStub: &searchUsersStub{
		users: []jira.User{{AccountID: "acc-mr", DisplayName: "Marco Reyes"}},
	}, readErr: errors.New("connection reset")}

	res, err := assignOne(context.Background(), &config.Config{}, db, probe, "", "NMB-1", "marco@example.com", true)
	if err != nil {
		t.Fatalf("force must write past a failed read: %v", err)
	}
	if len(probe.writes) != 1 || probe.writes[0] != "NMB-1=acc-mr" {
		t.Fatalf("force writes = %v", probe.writes)
	}
	if res.prevID != "acc-hc" || res.prevName != "Dana Whitfield" {
		t.Fatalf("previous = %q/%q; want the mirror's Dana Whitfield (acc-hc)", res.prevName, res.prevID)
	}
}

func TestAssignBatchStaleLineAmongThree(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	clearMirrorAssignee(t)
	f.searchUsers = func(string) string {
		return `[{"accountId":"acc-mr","displayName":"Marco Reyes","emailAddress":"marco@example.com","active":true},
			{"accountId":"acc-hc","displayName":"Dana Whitfield","emailAddress":"","active":true}]`
	}
	withStdin(t, ""+
		`{"key":"NMB-1","assignee":"marco@example.com"}`+"\n"+
		`{"key":"NMB-1","assignee":"marco@example.com","force":true}`+"\n"+
		`{"key":"NMB-1","assignee":"Dana Whitfield"}`+"\n")

	out, err := capture(t, func() error { return cmdAssign([]string{"--batch", "-"}) })
	if err == nil {
		t.Fatal("a batch with a refused line fails overall")
	}
	_, rows := parseTSVEnvelope(t, out)
	if len(rows) != 3 {
		t.Fatalf("rows %d in %q", len(rows), out)
	}
	// Line 1: stale — the refusal is that line's error, naming the holder.
	if rows[0].OK || rows[0].Changed {
		t.Fatalf("line 1 %+v", rows[0])
	}
	for _, want := range []string{"Dana Whitfield", "--force"} {
		if !strings.Contains(rows[0].Error, want) {
			t.Errorf("line 1 error %q missing %q", rows[0].Error, want)
		}
	}
	// Line 2: the per-line force writes anyway.
	if !rows[1].OK || !rows[1].Changed {
		t.Fatalf("line 2 %+v", rows[1])
	}
	// Line 3: Dana already holds it on the origin — a no-op row.
	if !rows[2].OK || rows[2].Changed {
		t.Fatalf("line 3 %+v", rows[2])
	}
	if n := countCalls(f, "PUT /issue/NMB-1/assignee"); n != 1 {
		t.Fatalf("assignee PUTs = %d, want exactly the forced one; calls %v", n, f.calls)
	}
}

func TestAssignDryRunStaleRefuses(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	clearMirrorAssignee(t)

	out, err := capture(t, func() error {
		return cmdAssign([]string{"NMB-1", "marco@example.com", "--dry-run"})
	})
	msg := staleExit(t, err)
	for _, want := range []string{"dry run", "Dana Whitfield", "--force"} {
		if !strings.Contains(msg, want) {
			t.Errorf("dry-run refusal %q missing %q", msg, want)
		}
	}
	noWriteTag(t, f)
	if strings.Contains(out, "dry_run") {
		t.Fatalf("a refused dry run prints no plan:\n%s", out)
	}
}

func TestAssignDryRunFreshPrintsVerdict(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, stderr, err := captureErr(t, func() error {
		return cmdAssign([]string{"NMB-1", "marco@example.com", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("fresh dry run: %v\n%s", err, out)
	}
	body := decodeDryRun(t, out)
	req, _ := body["request"].(map[string]any)
	if req["assignee"] != "acc-mr" {
		t.Fatalf("plan assignee: %v", req)
	}
	if _, forced := req["force"]; forced {
		t.Fatalf("plan must not carry force when the flag is absent: %v", req)
	}
	if !strings.Contains(stderr, "fresh") {
		t.Fatalf("dry run must report the check's verdict on stderr:\n%s", stderr)
	}
	noWriteTag(t, f)
}

// S9: an exact display name wins among substring hits (GDK-2063's second
// defect — "Ravi Okafor" was refused as matching twenty users while exactly
// one user is named that).
func TestResolveAccountExactDisplayNameWins(t *testing.T) {
	saveResolveAccountConfig(t, nil)
	users := []jira.User{{AccountID: "acc-exact", DisplayName: "Ravi Okafor", Email: ""}}
	for i := 0; i < 19; i++ {
		users = append(users, jira.User{
			AccountID: "acc-" + string(rune('a'+i)), DisplayName: "Ravi Okafor-" + string(rune('a'+i)), Email: "",
		})
	}
	stub := &searchUsersStub{users: users}
	id, err := resolveAccount(context.Background(), stub, "Ravi Okafor", "jira")
	if err != nil {
		t.Fatalf("exact display name must win among substring hits: %v", err)
	}
	if id != "acc-exact" {
		t.Fatalf("id = %q, want acc-exact", id)
	}
	// Trimmed and case-insensitive, the tolerance email matching gives.
	stub = &searchUsersStub{users: users}
	if id, err = resolveAccount(context.Background(), stub, "  ravi okafor ", "jira"); err != nil || id != "acc-exact" {
		t.Fatalf("trimmed/case-insensitive exact = %q, %v; want acc-exact", id, err)
	}
}

func TestResolveAccountExactNameTwiceStillRefused(t *testing.T) {
	saveResolveAccountConfig(t, nil)
	stub := &searchUsersStub{users: []jira.User{
		{AccountID: "acc-1", DisplayName: "Ravi Okafor", Email: "r1@example.com"},
		{AccountID: "acc-2", DisplayName: "Ravi Okafor", Email: "r2@example.com"},
		{AccountID: "acc-3", DisplayName: "Ravi Okafor-Jones", Email: "r3@example.com"},
	}}
	_, err := resolveAccount(context.Background(), stub, "Ravi Okafor", "jira")
	if err == nil {
		t.Fatal("two exact display names must stay refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "matches 2 users") {
		t.Fatalf("refusal %q", msg)
	}
	if !strings.Contains(msg, "r1@example.com") || !strings.Contains(msg, "r2@example.com") {
		t.Errorf("refusal must list the exact matches:\n%s", msg)
	}
	if strings.Contains(msg, "Okafor-Jones") {
		t.Errorf("refusal lists a non-exact substring hit:\n%s", msg)
	}
}
