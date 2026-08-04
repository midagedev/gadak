package main

import (
	"testing"
)

func TestKeyAssigneeIndexMatchesPython(t *testing.T) {
	// sum(ord(c) for c in key) % n
	// "NMB-1" → N=78 M=77 B=66 -=45 1=49 → 315; 315 % 3 = 0
	if got := KeyAssigneeIndex("NMB-1", 3); got != 0 {
		t.Errorf("NMB-1 → %d, want 0", got)
	}
	// "NMB-2" → 315-49+50 = 316; 316 % 3 = 1
	if got := KeyAssigneeIndex("NMB-2", 3); got != 1 {
		t.Errorf("NMB-2 → %d, want 1", got)
	}
	// "NMA-10" : N=78 M=77 A=65 -=45 1=49 0=48 → 362; 362 % 4 = 2
	if got := KeyAssigneeIndex("NMA-10", 4); got != 2 {
		t.Errorf("NMA-10 → %d, want 2", got)
	}
}

func TestKeyAssigneeIndexIdempotent(t *testing.T) {
	keys := []string{"NMB-1", "NMB-99", "NMA-7", "NMS-42", "ABC-1000"}
	for _, key := range keys {
		a := KeyAssigneeIndex(key, 4)
		b := KeyAssigneeIndex(key, 4)
		if a != b {
			t.Errorf("%s: %d != %d", key, a, b)
		}
		if a < 0 || a >= 4 {
			t.Errorf("%s: out of range %d", key, a)
		}
	}
}

func TestResolveAssigneeTargetDatasetSlot(t *testing.T) {
	slot1 := 1
	slots := map[string]*int{
		"assigned issue":   &slot1,
		"unassigned issue": nil,
	}
	pool := []string{"acc-0", "acc-1", "acc-2"}

	target, in := ResolveAssigneeTarget("assigned issue", slots, "NMB-1", "old", pool)
	if !in || target != "acc-1" {
		t.Fatalf("slot 1 → %q in=%v, want acc-1", target, in)
	}

	target, in = ResolveAssigneeTarget("unassigned issue", slots, "NMB-2", "someone", pool)
	if !in || target != "" {
		t.Fatalf("null slot → %q in=%v, want unassigned", target, in)
	}
}

func TestResolveAssigneeTargetNonDatasetSpread(t *testing.T) {
	slots := map[string]*int{} // empty: nothing from dataset
	pool := []string{"acc-0", "acc-1", "acc-2"}

	// Assigned non-dataset: spread by key hash.
	target, in := ResolveAssigneeTarget("not in dataset", slots, "NMB-1", "acc-9", pool)
	if in {
		t.Fatal("should not be in dataset")
	}
	want := pool[KeyAssigneeIndex("NMB-1", len(pool))]
	if target != want {
		t.Fatalf("got %q want %q", target, want)
	}

	// Unassigned non-dataset: stay unassigned.
	target, _ = ResolveAssigneeTarget("other", slots, "NMB-2", "", pool)
	if target != "" {
		t.Fatalf("unassigned non-dataset should stay null, got %q", target)
	}
}

func TestResolveAssigneeTargetIdempotent(t *testing.T) {
	// After a first repair, current already equals the computed target —
	// a second run must no-op.
	slot0 := 0
	slots := map[string]*int{"s": &slot0}
	pool := []string{"a", "b", "c"}

	target, _ := ResolveAssigneeTarget("s", slots, "K-1", "a", pool)
	if target != "a" {
		t.Fatalf("first resolve %q", target)
	}
	// Simulate already-correct current.
	again, _ := ResolveAssigneeTarget("s", slots, "K-1", target, pool)
	if again != target {
		t.Fatalf("idempotent fail: %q vs %q", again, target)
	}

	// Non-dataset assigned.
	slots = map[string]*int{}
	t1, _ := ResolveAssigneeTarget("x", slots, "NMB-42", "whatever", pool)
	t2, _ := ResolveAssigneeTarget("x", slots, "NMB-42", t1, pool)
	if t1 != t2 {
		t.Fatalf("hash spread not stable: %q vs %q", t1, t2)
	}
}

func TestAssigneeSlotWraps(t *testing.T) {
	slot := 5 // 5 % 3 = 2
	slots := map[string]*int{"big": &slot}
	pool := []string{"a", "b", "c"}
	target, _ := ResolveAssigneeTarget("big", slots, "K", "", pool)
	if target != "c" {
		t.Fatalf("got %q want c", target)
	}
}
