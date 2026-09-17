package transition

import (
	"context"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
)

// builtInDefaultTransitions is the transitions list the built-in tracker
// answers for an issue in To Do on the workflow a cutover leaves behind
// (GDK-1982, measured on a live home): the default catalog beside a
// migrated one, so "In Progress" answers to two ids. Transition ids are
// positional (1..n over the destinations in status-id order), which makes
// transition id 3 (Done) colliding with transition 4's target status id 3
// (the migrated In Progress) — the one transition a surface that sends the
// picked id as Target could never fire.
func builtInDefaultTransitions() []jira.Transition {
	st := func(id, name, cat string) jira.Status {
		return jira.Status{
			ID: id, Name: name,
			StatusCategory: struct {
				Key string `json:"key"`
			}{Key: cat},
		}
	}
	return []jira.Transition{
		{ID: "1", Name: "In Progress", To: st("10001", "In Progress", "indeterminate")},
		{ID: "2", Name: "In Review", To: st("10002", "In Review", "indeterminate")},
		{ID: "3", Name: "Done", To: st("10003", "Done", "done")},
		{ID: "4", Name: "In Progress", To: st("3", "In Progress", "indeterminate")},
	}
}

// GDK-1982: TransitionID is the machine path — an id out of the list the
// same origin just answered, matched byte for byte. On the colliding
// workflow above it must fire transition 3 (Done) where a Target of "3"
// is refused, and it must not read the issue status on the way (the
// category machinery is not consulted at all).
func TestApplyTransitionIDFiresTheOfferedID(t *testing.T) {
	s := &stubOrigin{list: builtInDefaultTransitions()}
	res, err := Apply(context.Background(), s, nil, Request{Key: "STD-1", TransitionID: "3"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("a fired transition reports changed, like an id Target today")
	}
	if !s.posted || s.postID != "3" {
		t.Fatalf("posted=%v id=%q, want transition 3 — the machine path must not resolve 3 as a target status id (transition 4)", s.posted, s.postID)
	}
	if s.statusN != 0 {
		t.Fatalf("machine path read the issue status %d times; a picked id needs no category read", s.statusN)
	}
}

// TransitionID wins when both it and Target are set — the documented
// Request contract. Target "Done" alone would resolve by name; the machine
// id is the caller saying which object it picked.
func TestApplyTransitionIDWinsOverTarget(t *testing.T) {
	s := &stubOrigin{list: builtInDefaultTransitions()}
	if _, err := Apply(context.Background(), s, nil, Request{Key: "STD-1", Target: "Done", TransitionID: "3"}); err != nil {
		t.Fatal(err)
	}
	if !s.posted || s.postID != "3" {
		t.Fatalf("posted=%v id=%q, want transition 3: TransitionID must win over Target", s.posted, s.postID)
	}
}

// The CLI contract does not move (GDK-1305): a human typing a bare 3 at
// Target still gets the ambiguity refusal, because on this workflow 3 is
// a transition id and a different transition's target status id.
func TestApplyTargetBareNumberStillRefused(t *testing.T) {
	s := &stubOrigin{list: builtInDefaultTransitions()}
	_, err := Apply(context.Background(), s, nil, Request{Key: "STD-1", Target: "3"})
	if !IsRefused(err) {
		t.Fatalf("Target 3 must stay a refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), "a bare number is ambiguous") {
		t.Fatalf("refusal must keep the bare-number sentence: %s", err.Error())
	}
	if s.posted {
		t.Fatal("a refused Target must not post")
	}
}

// A TransitionID the issue does not offer is a refusal that names the id
// sent and the ids the issue actually offers — a machine caller learns
// what to list next, in the format every other miss already uses.
func TestApplyTransitionIDMissNamesTheOfferedIDs(t *testing.T) {
	s := &stubOrigin{list: builtInDefaultTransitions()}
	_, err := Apply(context.Background(), s, nil, Request{Key: "STD-1", TransitionID: "9"})
	if !IsRefused(err) {
		t.Fatalf("an unoffered TransitionID must be a refusal, got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, `"9"`) || !strings.Contains(msg, "not offered") {
		t.Fatalf("refusal must name the id sent: %s", msg)
	}
	if !strings.Contains(msg, "id 3") || !strings.Contains(msg, "Done") {
		t.Fatalf("refusal must list the offered transitions: %s", msg)
	}
	if s.posted {
		t.Fatal("a miss must not post")
	}
}
