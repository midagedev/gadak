package transition

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// foldedInProgressPayload is the GDK-1521 payload, identical to the one the
// REST (internal/server) and claim (internal/claim) tests resolve: two
// transitions onto statuses that display the same name in the same category,
// so PickTransitionWith folds them, and the pick inside the group is the
// mirror tiebreak. Destination 10099 is the cutover phantom with zero issues;
// 3 is the one the mirror holds an issue in. Payload order answers 81.
func foldedInProgressPayload() []jira.Transition {
	return []jira.Transition{
		{
			ID: "81", Name: "Phantom start",
			To: jira.Status{ID: "10099", Name: "In Progress", StatusCategory: struct {
				Key string `json:"key"`
			}{Key: "indeterminate"}},
		},
		{
			ID: "11", Name: "Start",
			To: jira.Status{ID: "3", Name: "In Progress", StatusCategory: struct {
				Key string `json:"key"`
			}{Key: "indeterminate"}},
		},
	}
}

// backloggedStatus is the issue's status at pick time: outside the
// in-progress category, so the category token resolves instead of no-op'ing,
// and not a member of the folded group, so dropCurrent keeps both members.
func backloggedStatus() jira.Status {
	var st jira.Status
	st.ID = "10"
	st.Name = "Backlog"
	st.StatusCategory.Key = "new"
	return st
}

// TestMirrorStatusUsePrefersInUseDestination is the owner test of the tiebreak
// the three write surfaces share (GDK-1521): the folded payload resolved
// against a real mirror seeded with one NMB issue in status 3. The counts come
// from the mirror, not a stub — this is the read all three surfaces inject.
//
// FAIL-first: pre-GDK-1521 this function lived beside the CLI only, and the
// same Apply with no StatusUse fired 81 — pinned by the without-tiebreak half
// below, which documents the defect rather than asserting it is fixed.
func TestMirrorStatusUsePrefersInUseDestination(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "mirror.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"3": "inprogress"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1001", SourceID: "jira", ExternalID: "1001", Key: "NMB-1",
				Title:     "batch worker drops the last page",
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
				Status: "In Progress", StatusID: "3", StatusCategory: "inprogress",
			},
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// The counts a real mirror gives the folded group's two destinations.
	use := MirrorStatusUse(ctx, db, "NMB-1")
	if use == nil {
		t.Fatal("MirrorStatusUse returned nil for a keyed issue with a mirror")
	}
	if got := use("3"); got != 1 {
		t.Fatalf("mirror holds %d issues in status 3, want the seeded 1", got)
	}
	if got := use("10099"); got != 0 {
		t.Fatalf("phantom status 10099 counts %d issues, want 0", got)
	}

	// Through the write the surfaces run: the category token on the folded
	// payload lands on the destination the project actually uses.
	s := &stubOrigin{list: foldedInProgressPayload(), status: backloggedStatus()}
	if _, err := Apply(ctx, s, nil, Request{
		Key: "NMB-1", Target: "inprogress", StatusUse: MirrorStatusUse(ctx, db, "NMB-1"),
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if s.postID != "11" {
		t.Fatalf("fired %q, want transition 11 — the in-use destination, not payload order's 81", s.postID)
	}

	// Without the tiebreak the same payload answers 81: that is the defect
	// the REST and claim surfaces carried until they injected the same
	// StatusUse (GDK-1521), kept here as the control.
	s2 := &stubOrigin{list: foldedInProgressPayload(), status: backloggedStatus()}
	if _, err := Apply(ctx, s2, nil, Request{Key: "NMB-1", Target: "inprogress"}); err != nil {
		t.Fatalf("apply without tiebreak: %v", err)
	}
	if s2.postID != "81" {
		t.Fatalf("payload order control: fired %q, want 81 — if the fold stopped preferring payload order, update this control", s2.postID)
	}
}

// TestMirrorStatusUseNilWithoutScope — the hook is optional by design: no
// mirror or no project key in the issue key means nil, and the pick falls
// back to payload order (PickOptions' documented zero value).
func TestMirrorStatusUseNilWithoutScope(t *testing.T) {
	if got := MirrorStatusUse(context.Background(), nil, "NMB-1"); got != nil {
		t.Fatal("no mirror must answer nil, so payload order decides")
	}
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "mirror.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if got := MirrorStatusUse(ctx, db, "NMB"); got != nil {
		t.Fatal("a key with no project separator must answer nil, so payload order decides")
	}
}
