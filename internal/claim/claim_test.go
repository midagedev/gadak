package claim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
)

// fakeOrigin stands in for *jira.Client with enough state to answer the
// fallback's reads and to record the writes in order. Its Transition and
// SetAssignee mutate the state a re-read would see, so Result reflects the
// post-write truth like the real client's re-read does.
type fakeOrigin struct {
	me   jira.User
	st   jira.Status
	hold *jira.User
	// claimAnswer is returned when claimErr is nil.
	claimAnswer jira.ClaimResult
	claimErr    error
	readErr     error
	transitions []jira.Transition
	calls       []string
}

func (f *fakeOrigin) Transitions(_ context.Context, _ string) ([]jira.Transition, error) {
	return f.transitions, nil
}

func (f *fakeOrigin) Transition(_ context.Context, _, id string, _ map[string]any, _ json.RawMessage) error {
	f.calls = append(f.calls, "transition:"+id)
	for _, t := range f.transitions {
		if t.ID == id {
			f.st = t.To
		}
	}
	return nil
}

func (f *fakeOrigin) Claim(_ context.Context, _, _ string, _ bool) (jira.ClaimResult, error) {
	f.calls = append(f.calls, "claim")
	if f.claimErr != nil {
		return jira.ClaimResult{}, f.claimErr
	}
	return f.claimAnswer, nil
}

func (f *fakeOrigin) Myself(_ context.Context) (jira.User, error) {
	return f.me, nil
}

func (f *fakeOrigin) IssueStatus(_ context.Context, _ string) (jira.Status, *jira.User, error) {
	if f.readErr != nil {
		return jira.Status{}, nil, f.readErr
	}
	return f.st, f.hold, nil
}

func (f *fakeOrigin) SetAssignee(_ context.Context, _, id string) error {
	f.calls = append(f.calls, "assignee:"+id)
	f.hold = &jira.User{AccountID: id, DisplayName: f.me.DisplayName}
	return nil
}

var me = jira.User{AccountID: "me-1", DisplayName: "Me"}

func inProgressStatus() jira.Status {
	st := jira.Status{ID: "20", Name: "In Progress"}
	st.StatusCategory.Key = "indeterminate"
	return st
}

func toDoStatus() jira.Status {
	st := jira.Status{ID: "10", Name: "To Do"}
	st.StatusCategory.Key = "new"
	return st
}

func doneStatus() jira.Status {
	st := jira.Status{ID: "30", Name: "Done"}
	st.StatusCategory.Key = "done"
	return st
}

func cloudTransitions() []jira.Transition {
	return []jira.Transition{
		{ID: "11", Name: "Start", To: inProgressStatus()},
		{ID: "31", Name: "Done", To: doneStatus()},
	}
}

// The atomic path is one call and a faithful translation of the answer.
func TestApplyAtomic(t *testing.T) {
	f := &fakeOrigin{
		me: me,
		claimAnswer: jira.ClaimResult{
			Key:       "NMB-1",
			Assignee:  jira.User{AccountID: "me-1", DisplayName: "Me"},
			Status:    inProgressStatus(),
			ClaimedAt: "2026-08-22T10:00:00.000Z",
		},
	}
	res, err := Apply(context.Background(), f, nil, Request{Key: "NMB-1"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Atomic || res.Key != "NMB-1" || res.Assignee != "Me" || res.StatusCategory != "inprogress" || res.ClaimedAt == "" {
		t.Fatalf("result: %+v", res)
	}
	if len(f.calls) != 1 || f.calls[0] != "claim" {
		t.Fatalf("calls: %v — the atomic path must be exactly one origin call", f.calls)
	}
}

// The 409 becomes a TakenError whose Holder is parsed out of the origin's
// own sentence, so the CLI prints the origin's words either way.
func TestApplyConflict(t *testing.T) {
	f := &fakeOrigin{me: me, claimErr: &jira.APIError{
		Status:   409,
		Messages: []string{"NMB-1 is already claimed by Agent A"},
	}}
	_, err := Apply(context.Background(), f, nil, Request{Key: "NMB-1"})
	var taken *TakenError
	if !errors.As(err, &taken) {
		t.Fatalf("want TakenError, got %v", err)
	}
	if taken.Holder != "Agent A" {
		t.Fatalf("holder: %q (from %q)", taken.Holder, taken.Error())
	}
	if !strings.Contains(taken.Error(), "already claimed by Agent A") {
		t.Fatalf("sentence: %q", taken.Error())
	}
}

// A local TakenError with no origin sentence still names someone.
func TestTakenErrorNoHolder(t *testing.T) {
	e := &TakenError{Key: "NMB-1"}
	if got := e.Error(); got != "NMB-1 is already claimed by someone else" {
		t.Fatalf("sentence: %q", got)
	}
}

// Cloud: 404 on the claim route, issue new and unassigned — the fallback
// transitions to in-progress first, then assigns, and says Atomic false.
func TestApplyCloudFallbackFresh(t *testing.T) {
	f := &fakeOrigin{
		me:          me,
		st:          toDoStatus(),
		transitions: cloudTransitions(),
		claimErr:    &jira.APIError{Status: 404, Messages: []string{"no route"}},
	}
	res, err := Apply(context.Background(), f, nil, Request{Key: "NMB-1"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Atomic {
		t.Fatal("fallback must not claim atomicity")
	}
	if res.AssigneeID != "me-1" || res.StatusCategory != "inprogress" {
		t.Fatalf("result: %+v", res)
	}
	want := []string{"claim", "transition:11", "assignee:me-1"}
	if fmt.Sprint(f.calls) != fmt.Sprint(want) {
		t.Fatalf("call order %v, want %v (transition before assignee)", f.calls, want)
	}
}

// Cloud, in progress and held by someone else: refused locally, no writes —
// the same judgment the atomic route makes, with no route to hide behind.
func TestApplyCloudFallbackTaken(t *testing.T) {
	f := &fakeOrigin{
		me:          me,
		st:          inProgressStatus(),
		hold:        &jira.User{AccountID: "other-1", DisplayName: "Agent A"},
		transitions: cloudTransitions(),
		claimErr:    &jira.APIError{Status: 404, Messages: []string{"no route"}},
	}
	_, err := Apply(context.Background(), f, nil, Request{Key: "NMB-1"})
	var taken *TakenError
	if !errors.As(err, &taken) {
		t.Fatalf("want TakenError, got %v", err)
	}
	if taken.Holder != "Agent A" {
		t.Fatalf("holder: %q", taken.Holder)
	}
	if len(f.calls) != 1 {
		t.Fatalf("a refusal must not write: %v", f.calls)
	}
}

// Cloud, in progress and held by someone else, --take-over: no transition
// (already in progress), just the assignee replacement.
func TestApplyCloudFallbackTakeOver(t *testing.T) {
	f := &fakeOrigin{
		me:          me,
		st:          inProgressStatus(),
		hold:        &jira.User{AccountID: "other-1", DisplayName: "Agent A"},
		transitions: cloudTransitions(),
		claimErr:    &jira.APIError{Status: 404, Messages: []string{"no route"}},
	}
	res, err := Apply(context.Background(), f, nil, Request{Key: "NMB-1", TakeOver: true})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Atomic || res.AssigneeID != "me-1" {
		t.Fatalf("result: %+v", res)
	}
	want := []string{"claim", "assignee:me-1"}
	if fmt.Sprint(f.calls) != fmt.Sprint(want) {
		t.Fatalf("calls %v, want %v — take-over of an in-progress issue must not re-transition", f.calls, want)
	}
}

// Cloud, in progress and already mine: nothing writes.
func TestApplyCloudFallbackIdempotent(t *testing.T) {
	f := &fakeOrigin{
		me:          me,
		st:          inProgressStatus(),
		hold:        &jira.User{AccountID: "me-1", DisplayName: "Me"},
		transitions: cloudTransitions(),
		claimErr:    &jira.APIError{Status: 404, Messages: []string{"no route"}},
	}
	res, err := Apply(context.Background(), f, nil, Request{Key: "NMB-1"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Atomic || res.AssigneeID != "me-1" || res.StatusCategory != "inprogress" {
		t.Fatalf("result: %+v", res)
	}
	if len(f.calls) != 1 {
		t.Fatalf("an already-held claim must not write: %v", f.calls)
	}
}

// An in-progress-but-unassigned issue is claimable by anyone: the atomic
// route only refuses when another *holder* exists.
func TestApplyCloudFallbackUnassignedInProgress(t *testing.T) {
	f := &fakeOrigin{
		me:          me,
		st:          inProgressStatus(),
		transitions: cloudTransitions(),
		claimErr:    &jira.APIError{Status: 404, Messages: []string{"no route"}},
	}
	res, err := Apply(context.Background(), f, nil, Request{Key: "NMB-1"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.AssigneeID != "me-1" {
		t.Fatalf("result: %+v", res)
	}
	want := []string{"claim", "assignee:me-1"}
	if fmt.Sprint(f.calls) != fmt.Sprint(want) {
		t.Fatalf("calls %v, want %v — no transition when already in progress", f.calls, want)
	}
}

// A 404 that is not "no route" but "no issue" surfaces honestly: the
// fallback's read fails with the read's own error.
func TestApplyCloudFallbackMissingIssue(t *testing.T) {
	f := &fakeOrigin{
		me:          me,
		transitions: cloudTransitions(),
		claimErr:    &jira.APIError{Status: 404, Messages: []string{"no route"}},
		readErr:     fmt.Errorf("jira: 404: Issue does not exist"),
	}
	_, err := Apply(context.Background(), f, nil, Request{Key: "GONE-1"})
	if err == nil || !strings.Contains(err.Error(), "Issue does not exist") {
		t.Fatalf("want the read's own error, got %v", err)
	}
}

// Any other origin error (400 field rejection, 500) passes through.
func TestApplyOtherErrorPassesThrough(t *testing.T) {
	f := &fakeOrigin{me: me, claimErr: &jira.APIError{Status: 400, Messages: []string{"no in-progress transition available"}}}
	if _, err := Apply(context.Background(), f, nil, Request{Key: "NMB-1"}); err == nil || !strings.Contains(err.Error(), "no in-progress transition available") {
		t.Fatalf("want pass-through, got %v", err)
	}
}
