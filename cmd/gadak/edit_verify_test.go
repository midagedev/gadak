package main

import (
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// GDK-1645 FAIL-first: Jira Server answered 204 to a standard issue's
// parent and changed nothing; the CLI printed the unchanged row as success.
func TestVerifyEditLandedCatchesADroppedParent(t *testing.T) {
	sp := func(s string) *string { return &s }
	pre := &store.IssueLite{IssueKey: "SCR-3", Summary: "s"}
	post := &store.IssueLite{IssueKey: "SCR-3", Summary: "s"}
	ch := editChange{key: "SCR-3", hasParent: true, parentKey: "SCR-7"}
	err := verifyEditLanded(pre, post, ch)
	if err == nil || !strings.Contains(err.Error(), "parent did not change") {
		t.Fatalf("dropped parent passed as success: %v", err)
	}
	// Landed: the row moved to what was asked.
	post.ParentKey = sp("SCR-7")
	if err := verifyEditLanded(pre, post, ch); err != nil {
		t.Errorf("landed write reported as dropped: %v", err)
	}
	// Idempotent: already there before the write is not a drop.
	pre.ParentKey = sp("SCR-7")
	if err := verifyEditLanded(pre, post, ch); err != nil {
		t.Errorf("idempotent write reported as dropped: %v", err)
	}
	// Clear that did not clear.
	pre.ParentKey, post.ParentKey = sp("SCR-7"), sp("SCR-7")
	if err := verifyEditLanded(pre, post, editChange{key: "SCR-3", hasParent: true, clearParent: true}); err == nil {
		t.Error("a clear the origin ignored passed as success")
	}
	// Priority: token may be a name or an id.
	pre, post = &store.IssueLite{Priority: sp("Medium"), PriorityID: "3"}, &store.IssueLite{Priority: sp("Medium"), PriorityID: "3"}
	if err := verifyEditLanded(pre, post, editChange{hasPriority: true, priority: "High"}); err == nil {
		t.Error("a priority the origin ignored passed as success")
	}
	if err := verifyEditLanded(pre, post, editChange{hasPriority: true, priority: "medium"}); err != nil {
		t.Errorf("idempotent priority by name reported as dropped: %v", err)
	}
	// Not mirrored before the write: nothing to compare, no false alarm.
	if err := verifyEditLanded(nil, post, ch); err != nil {
		t.Errorf("unmirrored pre reported: %v", err)
	}
}
