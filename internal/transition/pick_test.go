package transition

import (
	"errors"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
)

// GDK-1356. The `gdk` workspace carries two statuses both displayed
// "In Progress" (status_id 10001 and 3) — residue of the 2026-09-01 cutover
// that merged the Jira workflow into the built-in tracker's default catalog.
// Measured payload (gadak --workspace gdk transition GDK-1356 --json):
//
//	{"id":"1","name":"In Progress","to":{"id":"10001","name":"In Progress","statusCategory":{"key":"indeterminate"}}}
//	{"id":"2","name":"In Review",  "to":{"id":"10002","name":"In Review",  "statusCategory":{"key":"indeterminate"}}}
//	{"id":"3","name":"Done",       "to":{"id":"10003","name":"Done",       "statusCategory":{"key":"done"}}}
//	{"id":"4","name":"In Progress","to":{"id":"3",    "name":"In Progress","statusCategory":{"key":"indeterminate"}}}
//
// Two candidates that put the issue in a status the reader cannot tell apart
// are not a choice, so they fold into one. Two candidates a reader *can* tell
// apart still refuse — the rule folds duplicates, it never guesses.

func tr(id, name, toID, toName, cat string) jira.Transition {
	t := jira.Transition{ID: id, Name: name}
	t.To.ID = toID
	t.To.Name = toName
	t.To.StatusCategory.Key = cat
	return t
}

// gdkWorkflow is the measured payload above.
func gdkWorkflow() []jira.Transition {
	return []jira.Transition{
		tr("1", "In Progress", "10001", "In Progress", "indeterminate"),
		tr("2", "In Review", "10002", "In Review", "indeterminate"),
		tr("3", "Done", "10003", "Done", "done"),
		tr("4", "In Progress", "3", "In Progress", "indeterminate"),
	}
}

// Two same-named destinations in one category are one destination as far as
// the reader is concerned: fold, do not refuse. FAIL-first on the pre-fix
// source, which answers AmbiguousTransitionError here.
func TestPickCategoryFoldsSameNamedDuplicates(t *testing.T) {
	list := []jira.Transition{
		tr("1", "In Progress", "10001", "In Progress", "indeterminate"),
		tr("3", "Done", "10003", "Done", "done"),
		tr("4", "In Progress", "3", "In Progress", "indeterminate"),
	}
	id, err := PickTransition("GDK-1", "inprogress", list)
	if err != nil {
		t.Fatalf("same-named duplicates must fold, got %v", err)
	}
	if id != "1" {
		t.Fatalf("id = %q, want the first payload member of the folded group", id)
	}
}

// The fold keys on the destination status name, not on the transition name:
// a workflow that labels the two moves differently still lands the reader on
// one indistinguishable status.
func TestPickCategoryFoldsOnDestinationName(t *testing.T) {
	list := []jira.Transition{
		tr("1", "Start work", "10001", "In Progress", "indeterminate"),
		tr("4", "Resume", "3", "In Progress", "indeterminate"),
	}
	if _, err := PickTransition("GDK-1", "inprogress", list); err != nil {
		t.Fatalf("destination-name fold: %v", err)
	}
}

// Distinct destination names still refuse. The rule must never silently pick
// between two statuses the reader can tell apart.
func TestPickCategoryDistinctNamesStillAmbiguous(t *testing.T) {
	list := []jira.Transition{
		tr("1", "In Progress", "10001", "In Progress", "indeterminate"),
		tr("2", "In Review", "10002", "In Review", "indeterminate"),
		tr("5", "Blocked", "10004", "Blocked", "indeterminate"),
	}
	_, err := PickTransition("GDK-1", "inprogress", list)
	var amb *AmbiguousTransitionError
	if !errors.As(err, &amb) {
		t.Fatalf("three distinct names must refuse, got %v", err)
	}
	if len(amb.Candidates) != 3 {
		t.Fatalf("candidates = %d, want 3", len(amb.Candidates))
	}
	if len(amb.Folded) != 0 {
		t.Fatalf("nothing to fold, got %v", amb.Folded)
	}
}

// The GDK shape: the two "In Progress" fold, "In Review" survives, so the
// refusal names two readings instead of three — and says where the third
// went, because a reader who saw id 4 in `gadak transition KEY` must not
// think gadak stopped seeing it.
func TestPickCategoryRefusalNamesFoldedDuplicates(t *testing.T) {
	_, err := PickTransition("GDK-1353", "inprogress", gdkWorkflow())
	var amb *AmbiguousTransitionError
	if !errors.As(err, &amb) {
		t.Fatalf("two distinct names must refuse, got %v", err)
	}
	if len(amb.Candidates) != 2 || len(amb.Folded) != 1 {
		t.Fatalf("candidates %d folded %d, want 2 and 1", len(amb.Candidates), len(amb.Folded))
	}
	msg := amb.Error()
	for _, want := range []string{
		`transition "inprogress" is ambiguous on GDK-1353`,
		"2 transitions land there",
		"In Progress (id 1, → In Progress [status_id 10001])",
		"In Review (id 2, → In Review [status_id 10002])",
		"folded into those",
		"In Progress (id 4, → In Progress [status_id 3])",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

// The tiebreak inside a folded group is which status the project actually
// uses. On `gdk` the mirror holds 8 issues in 10001 and 0 in 3, so the fold
// must land on 10001 even when the unused duplicate comes first in the
// payload.
func TestPickCategoryFoldPrefersStatusInMirrorUse(t *testing.T) {
	list := []jira.Transition{
		tr("4", "In Progress", "3", "In Progress", "indeterminate"),
		tr("1", "In Progress", "10001", "In Progress", "indeterminate"),
	}
	use := map[string]int{"3": 0, "10001": 8}
	id, err := PickTransitionWith("GDK-1", "inprogress", list, PickOptions{
		StatusUse: func(statusID string) int { return use[statusID] },
	})
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	if id != "1" {
		t.Fatalf("id = %q, want 1 (status 10001, the id in real use)", id)
	}
}

// Without a usage hook the fold is stable on payload order — it never
// reorders on its own.
func TestPickCategoryFoldWithoutUsageKeepsPayloadOrder(t *testing.T) {
	list := []jira.Transition{
		tr("4", "In Progress", "3", "In Progress", "indeterminate"),
		tr("1", "In Progress", "10001", "In Progress", "indeterminate"),
	}
	id, err := PickTransition("GDK-1", "inprogress", list)
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	if id != "4" {
		t.Fatalf("id = %q, want 4 (first in payload)", id)
	}
}

// The status the issue already sits in is not a candidate inside a folded
// group: "move me to In Progress" cannot mean the In Progress it is in.
func TestPickCategoryFoldDropsCurrentStatus(t *testing.T) {
	list := []jira.Transition{
		tr("1", "In Progress", "10001", "In Progress", "indeterminate"),
		tr("4", "In Progress", "3", "In Progress", "indeterminate"),
	}
	id, err := PickTransitionWith("GDK-1", "inprogress", list, PickOptions{CurrentStatusID: "10001"})
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	if id != "4" {
		t.Fatalf("id = %q, want 4 — 10001 is where the issue already is", id)
	}
}

// The drop applies only inside a group of duplicates. A lone candidate that
// happens to be the current status stays a candidate, so a self-loop
// workflow cannot make `inprogress` mean some other in-progress status.
func TestPickCategoryCurrentStatusAloneIsStillACandidate(t *testing.T) {
	list := []jira.Transition{
		tr("1", "In Progress", "10001", "In Progress", "indeterminate"),
		tr("2", "In Review", "10002", "In Review", "indeterminate"),
	}
	_, err := PickTransitionWith("GDK-1", "inprogress", list, PickOptions{CurrentStatusID: "10001"})
	var amb *AmbiguousTransitionError
	if !errors.As(err, &amb) {
		t.Fatalf("want the two-name refusal, got %v", err)
	}
}

// A payload with no destination name (a damaged or stale shape) must not
// fold two unrelated statuses into one on the strength of matching empties.
func TestPickCategoryEmptyDestinationNamesDoNotFold(t *testing.T) {
	list := []jira.Transition{
		tr("1", "A", "10001", "", "indeterminate"),
		tr("2", "B", "10002", "", "indeterminate"),
	}
	_, err := PickTransitionWith("GDK-1", "inprogress", list, PickOptions{})
	var amb *AmbiguousTransitionError
	if !errors.As(err, &amb) {
		t.Fatalf("nameless destinations must refuse, got %v", err)
	}
	if len(amb.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(amb.Candidates))
	}
}

// Folding is scoped to the category branch. A bare status id that two
// transitions reach still refuses: that pick is not about display names.
// The refusal is a caller-side one — IsRefused — never an origin error:
// a miss means nothing was written, whatever surface asked.
func TestPickStatusIDWithTwoTransitionsStillAmbiguous(t *testing.T) {
	list := []jira.Transition{
		tr("1", "Start", "10001", "In Progress", "indeterminate"),
		tr("9", "Resume", "10001", "In Progress", "indeterminate"),
	}
	_, err := PickTransition("GDK-1", "10001", list)
	var amb *AmbiguousTransitionError
	if !errors.As(err, &amb) {
		t.Fatalf("two transitions onto one status id must still refuse, got %v", err)
	}
	if !IsRefused(err) {
		t.Fatalf("pick misses must be refusals (IsRefused), got %v", err)
	}
}

// A plain miss is a Refused too — Apply returns pick errors verbatim (the
// rewrap that used to live there is gone), so the pick itself owns the class.
func TestPickMissIsRefused(t *testing.T) {
	list := []jira.Transition{tr("1", "Start", "10001", "In Progress", "indeterminate")}
	_, err := PickTransition("GDK-1", "nonexistent", list)
	if err == nil {
		t.Fatal("want a miss error")
	}
	if !IsRefused(err) {
		t.Fatalf("miss must be a refusal (IsRefused), got %T %v", err, err)
	}
}

// The id/to.id collision refusal (GDK-1305) is a caller-side refusal as well.
func TestPickIDAndStatusIDCollisionIsRefused(t *testing.T) {
	list := []jira.Transition{
		tr("11", "Start", "10001", "In Progress", "indeterminate"),
		tr("2", "Elsewhere", "11", "Blocked", "indeterminate"),
	}
	_, err := PickTransition("GDK-1", "11", list)
	if err == nil {
		t.Fatal("want the collision refusal")
	}
	if !IsRefused(err) {
		t.Fatalf("collision must be a refusal (IsRefused), got %T %v", err, err)
	}
}

// The paths ahead of the category branch are untouched by the fold.
func TestPickPrecedenceUnchangedByFold(t *testing.T) {
	list := gdkWorkflow()
	for _, tc := range []struct{ want, id string }{
		{"2", "2"},         // transition id
		{"10002", "2"},     // target status id
		{"In Review", "2"}, // transition / status name
		{"done", "3"},      // a category with one landing
	} {
		id, err := PickTransition("GDK-1", tc.want, list)
		if err != nil {
			t.Fatalf("%q: %v", tc.want, err)
		}
		if id != tc.id {
			t.Errorf("%q → %q, want %q", tc.want, id, tc.id)
		}
	}
}
