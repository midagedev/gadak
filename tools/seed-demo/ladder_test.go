package main

import (
	"testing"
)

func TestPickLadderStepAlreadyThere(t *testing.T) {
	step := PickLadderStep("100", "new", "100", "new", nil)
	if !step.AlreadyThere || !step.OK {
		t.Fatalf("got %+v, want already there", step)
	}
}

func TestPickLadderStepSameCategoryDifferentStatus(t *testing.T) {
	// Backlog (new) → Selected for Development (new)
	opts := []TransitionOption{
		{ID: "11", ToID: "101", ToCategory: "new"},
		{ID: "21", ToID: "200", ToCategory: "indeterminate"},
		{ID: "31", ToID: "300", ToCategory: "done"},
	}
	step := PickLadderStep("100", "new", "101", "new", opts)
	if !step.OK || step.TransitionID != "11" {
		t.Fatalf("got %+v, want transition 11", step)
	}
}

func TestPickLadderStepWalksOneRung(t *testing.T) {
	// From backlog (new) toward done: must step to inprogress first,
	// not take the direct Backlog→Done edge.
	opts := []TransitionOption{
		{ID: "direct", ToID: "300", ToCategory: "done"},
		{ID: "start", ToID: "200", ToCategory: "indeterminate"},
		{ID: "other", ToID: "201", ToCategory: "indeterminate"},
	}
	step := PickLadderStep("100", "new", "300", "done", opts)
	if !step.OK {
		t.Fatal("expected a step")
	}
	if step.TransitionID == "direct" {
		t.Fatal("must not take direct done edge when a rung exists")
	}
	if step.TransitionID != "start" {
		// Prefer first candidate on next rung when target is not among them.
		t.Fatalf("got transition %q, want first indeterminate candidate", step.TransitionID)
	}
}

func TestPickLadderStepPrefersTargetOnNextRung(t *testing.T) {
	opts := []TransitionOption{
		{ID: "a", ToID: "200", ToCategory: "indeterminate"},
		{ID: "b", ToID: "201", ToCategory: "indeterminate"}, // target
	}
	step := PickLadderStep("100", "new", "201", "indeterminate", opts)
	if !step.OK || step.TransitionID != "b" {
		t.Fatalf("got %+v, want transition b (target on next rung)", step)
	}
}

func TestPickLadderStepInprogressTokenEqualsIndeterminate(t *testing.T) {
	// FAIL-first (GDK-577): Jira REST key "indeterminate" and gadak token
	// "inprogress" are one rung. Treating them as unknown skips the ladder
	// and takes Backlog→Done in one hop.
	optsKey := []TransitionOption{
		{ID: "direct", ToID: "300", ToCategory: "done"},
		{ID: "start", ToID: "200", ToCategory: "indeterminate"},
	}
	optsToken := []TransitionOption{
		{ID: "direct", ToID: "300", ToCategory: "done"},
		{ID: "start", ToID: "200", ToCategory: "inprogress"},
	}
	viaKey := PickLadderStep("100", "new", "300", "done", optsKey)
	viaToken := PickLadderStep("100", "new", "300", "done", optsToken)
	if viaKey.TransitionID != "start" || viaToken.TransitionID != "start" {
		t.Fatalf("rung walk: Jira key %+v, gadak token %+v (want start, not direct)", viaKey, viaToken)
	}
}

func TestPickLadderStepFallsBackToDirect(t *testing.T) {
	// Workflow with no intermediate category edges.
	opts := []TransitionOption{
		{ID: "jump", ToID: "300", ToCategory: "done"},
	}
	step := PickLadderStep("100", "new", "300", "done", opts)
	if !step.OK || step.TransitionID != "jump" {
		t.Fatalf("got %+v, want direct jump", step)
	}
}

func TestPickLadderStepBackward(t *testing.T) {
	// done → new (reopen path): step through indeterminate if available.
	opts := []TransitionOption{
		{ID: "to-progress", ToID: "200", ToCategory: "indeterminate"},
		{ID: "to-todo", ToID: "100", ToCategory: "new"},
	}
	step := PickLadderStep("300", "done", "100", "new", opts)
	if !step.OK || step.TransitionID != "to-progress" {
		t.Fatalf("got %+v, want one rung down to indeterminate", step)
	}
}

func TestPickLadderStepNoPath(t *testing.T) {
	opts := []TransitionOption{
		{ID: "x", ToID: "200", ToCategory: "indeterminate"},
	}
	step := PickLadderStep("100", "new", "999", "done", opts)
	// Next rung exists but target is not reachable in one hop and no direct —
	// still takes next rung (OK). After that hop the outer loop re-decides.
	if !step.OK || step.TransitionID != "x" {
		t.Fatalf("got %+v, want next-rung candidate", step)
	}

	// No options at all.
	step = PickLadderStep("100", "new", "300", "done", nil)
	if step.OK {
		t.Fatal("empty options should fail")
	}
}

func TestMapStatusesFromOrdered(t *testing.T) {
	ordered := []statusCat{
		{ID: "1", Category: "new"},
		{ID: "2", Category: "new"},
		{ID: "3", Category: "indeterminate"},
		{ID: "4", Category: "done"},
	}
	got := MapStatusesFromOrdered(ordered)
	want := map[string]string{
		"backlog": "1", "selected": "2", "inprogress": "3", "done": "4",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q want %q", k, got[k], v)
		}
	}
}

func TestMapStatusesSingleTodo(t *testing.T) {
	got := MapStatusesFromOrdered([]statusCat{
		{ID: "1", Category: "new"},
		{ID: "3", Category: "done"},
	})
	if got["backlog"] != "1" || got["selected"] != "1" {
		t.Errorf("single todo should map both backlog and selected: %v", got)
	}
	if _, ok := got["inprogress"]; ok {
		t.Error("no indeterminate status expected")
	}
}
