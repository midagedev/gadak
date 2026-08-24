package parenthint

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// The two errors below are what a real Jira Cloud site answered on
// 2026-08-21 for the same illegal parent, one per verb — rebuilt as the
// *jira.APIError the client actually returns (wrapped with the method and
// path, exactly like jira.write). The sentences are verbatim on purpose,
// but only the Errors keys are load-bearing: the sentence is localized per
// account, and GDK-525 existed because the create path tested for "parent"
// — a substring the edit answer does not contain.
func createParent400() error {
	return fmt.Errorf("POST /rest/api/3/issue: %w", &jira.APIError{
		Status: 400,
		Errors: map[string]string{
			"parent":   "유효한 상위 업무를 선택하세요.",
			"parentId": "유효한 상위 업무를 선택하세요.",
		},
	})
}

func editParent400() error {
	return fmt.Errorf("PUT /rest/api/3/issue/NMB-2: %w", &jira.APIError{
		Status: 400,
		Errors: map[string]string{
			"pid": "이 이슈 유형의 이슈는 상위 이슈와 같은 프로젝트에 만들어야 합니다.",
		},
	})
}

func TestRejectionKnowsBothVerbShapes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"create shape", createParent400(), true},
		{"edit shape", editParent400(), true},
		{"nil", nil, false},
		{"unrelated field 400", fmt.Errorf("PUT /rest/api/3/issue/NMB-2: %w", &jira.APIError{
			Status: 400,
			Errors: map[string]string{"summary": "You must specify a summary."},
		}), false},
		// GDK-819: the printed sentence is not evidence. A message that
		// merely mentions parent — or an untyped error that does — must not
		// classify; only the Errors keys parent/parentId/pid are the rejection.
		{"message mentioning parent", fmt.Errorf("POST /rest/api/3/issue: %w", &jira.APIError{
			Status:   400,
			Messages: []string{"parent picker input was malformed"},
		}), false},
		{"linear refusal naming parent", fmt.Errorf(`linear: field "parent" is not supported on create`), false},
		{"word containing pid", fmt.Errorf("GET /rest/api/3/issue/NMB-2: rapid retries exhausted"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Rejection(tc.err); got != tc.want {
				t.Fatalf("Rejection(%q) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestWrapAttachesForBothVerbs(t *testing.T) {
	db := seedDB(t, nil)

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"create shape", createParent400()},
		{"edit shape", editParent400()},
	} {
		got := Wrap(tc.err, "NMB-1", db)
		if !strings.Contains(got.Error(), "hint:") {
			t.Fatalf("no hint attached to %s; got %q", tc.name, got)
		}
		if !strings.Contains(got.Error(), "NMB-1") {
			t.Fatalf("hint does not name the parent; got %q", got)
		}
		var apiErr *jira.APIError
		if !errors.As(got, &apiErr) {
			t.Fatalf("origin error was replaced instead of wrapped: %v", got)
		}
	}

	base := fmt.Errorf("wrapped: %w", editParent400())
	if wrapped := Wrap(base, "NMB-1", db); !errors.Is(wrapped, base) {
		t.Fatalf("errors.Is lost the origin error: %v", wrapped)
	}
}

func TestWrapLeavesOtherErrorsAlone(t *testing.T) {
	db := seedDB(t, nil)

	other := fmt.Errorf("PUT /rest/api/3/issue/NMB-2: %w", &jira.APIError{
		Status: 400,
		Errors: map[string]string{"summary": "You must specify a summary."},
	})
	if got := Wrap(other, "NMB-1", db); got != other {
		t.Fatalf("unrelated error was decorated: %q", got)
	}
	// The Linear adapter refuses parent locally; its sentence names the field,
	// but it is a capability refusal, not an origin parent rejection (GDK-819).
	linear := fmt.Errorf(`linear: field "parent" is not supported on create`)
	if got := Wrap(linear, "NMB-1", db); got.Error() != linear.Error() {
		t.Fatalf("linear parent refusal was decorated: %q", got)
	}
	if got := Wrap(editParent400(), "", db); strings.Contains(got.Error(), "hint:") {
		t.Fatalf("hint attached with no parent key: %q", got)
	}
	if got := Wrap(nil, "NMB-1", db); got != nil {
		t.Fatalf("nil error became %v", got)
	}
	if got := Wrap(editParent400(), "NMB-1", nil); strings.Contains(got.Error(), "hint:") {
		t.Fatalf("nil querier attached a hint: %q", got)
	}
}

func TestHintCoversEpicParent(t *testing.T) {
	db := seedDB(t, []store.IssueRecord{{
		Item: store.Item{
			ID: "jira:2001", SourceID: "jira", Kind: "issue", ExternalID: "2001", Key: "NMB-900",
			Title: "the epic", CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
		},
		Issue: store.Issue{
			ProjectKey: "NMB", IssueType: "Epic", IssueTypeID: "10001", HierarchyLevel: 1,
			Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
		},
	}})

	hint := Hint(db, "NMB-900")
	if strings.TrimSpace(hint) == "" {
		t.Fatal("epic parent got no hint; want the one-level-above rule")
	}
	for _, want := range []string{"NMB-900", "Epic", "one level above"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint missing %q: %q", want, hint)
		}
	}

	low := Hint(db, "NMB-1")
	if !strings.Contains(low, "level-1 parent (an epic)") {
		t.Fatalf("level-0 hint changed: %q", low)
	}
}

func TestHintUnknownParentIsSilent(t *testing.T) {
	db := seedDB(t, nil)
	if got := Hint(db, "NMB-4242"); got != "" {
		t.Fatalf("unknown parent produced a hint: %q", got)
	}
}

func TestHintNamesSameProjectOpenEpics(t *testing.T) {
	db := seedDB(t, []store.IssueRecord{
		epicRec("3010", "NMB-10", "NMB", "newest open epic", "2026-08-20T00:00:00.000Z", "3", "inprogress", 1),
		epicRec("3011", "NMB-11", "NMB", "middle open epic", "2026-08-19T00:00:00.000Z", "3", "inprogress", 1),
		epicRec("3012", "NMB-12", "NMB", "older open epic", "2026-08-18T00:00:00.000Z", "3", "inprogress", 1),
		epicRec("3013", "NMB-13", "NMB", "oldest open epic should drop", "2026-08-17T00:00:00.000Z", "3", "inprogress", 1),
		epicRec("3014", "NMB-14", "NMB", "done epic newest of all", "2026-08-21T00:00:00.000Z", "10001", "done", 1),
		epicRec("4010", "GDK-1", "GDK", "other project epic", "2026-08-22T00:00:00.000Z", "3", "inprogress", 1),
		epicRec("3015", "NMB-15", "NMB", "standard issue not an epic", "2026-08-23T00:00:00.000Z", "3", "inprogress", 0),
	})

	hint := Hint(db, "NMB-1")
	if !strings.Contains(hint, "Pick an epic as --parent") {
		t.Fatalf("level-0 sentence missing: %q", hint)
	}
	if !strings.Contains(hint, `open epics in NMB:`) {
		t.Fatalf("missing same-project epic line: %q", hint)
	}
	for _, want := range []string{
		`NMB-10 "newest open epic"`,
		`NMB-11 "middle open epic"`,
		`NMB-12 "older open epic"`,
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint missing %q: %q", want, hint)
		}
	}
	i10 := strings.Index(hint, "NMB-10")
	i11 := strings.Index(hint, "NMB-11")
	i12 := strings.Index(hint, "NMB-12")
	if i10 < 0 || i11 < 0 || i12 < 0 || !(i10 < i11 && i11 < i12) {
		t.Fatalf("open epics not in updated_at DESC order: %q", hint)
	}
	for _, drop := range []string{"NMB-13", "NMB-14", "GDK-1", "NMB-15", "oldest open", "done epic", "other project", "not an epic"} {
		if strings.Contains(hint, drop) {
			t.Fatalf("hint named excluded row %q: %q", drop, hint)
		}
	}
}

func TestHintOmitsEpicLineWhenNone(t *testing.T) {
	db := seedDB(t, []store.IssueRecord{
		epicRec("3014", "NMB-14", "NMB", "only a done epic", "2026-08-21T00:00:00.000Z", "10001", "done", 1),
		epicRec("4010", "GDK-1", "GDK", "open but other project", "2026-08-22T00:00:00.000Z", "3", "inprogress", 1),
	})

	hint := Hint(db, "NMB-1")
	if !strings.Contains(hint, "Pick an epic as --parent") {
		t.Fatalf("level-0 sentence missing: %q", hint)
	}
	if strings.Contains(hint, "open epics") {
		t.Fatalf("zero-candidate hint grew an epic list: %q", hint)
	}
	if strings.Contains(hint, "NMB-14") || strings.Contains(hint, "GDK-1") {
		t.Fatalf("hint named a non-candidate: %q", hint)
	}
}

func TestHintDoesNotSuggestEpicsForEpicParent(t *testing.T) {
	db := seedDB(t, []store.IssueRecord{
		{
			Item: store.Item{
				ID: "jira:2001", SourceID: "jira", Kind: "issue", ExternalID: "2001", Key: "NMB-900",
				Title: "the epic", CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Epic", IssueTypeID: "10001", HierarchyLevel: 1,
				Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
			},
		},
		epicRec("3010", "NMB-10", "NMB", "another open epic", "2026-08-20T00:00:00.000Z", "3", "inprogress", 1),
	})

	hint := Hint(db, "NMB-900")
	if !strings.Contains(hint, "one level above") {
		t.Fatalf("epic-parent rule missing: %q", hint)
	}
	if strings.Contains(hint, "open epics") || strings.Contains(hint, "NMB-10") {
		t.Fatalf("level>=1 hint must not suggest epics: %q", hint)
	}
}

func seedDB(t *testing.T, extra []store.IssueRecord) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://nimbus.example.com"}); err != nil {
		t.Fatal(err)
	}
	recs := []store.IssueRecord{{
		Item: store.Item{
			ID: "jira:1001", SourceID: "jira", Kind: "issue", ExternalID: "1001", Key: "NMB-1",
			Title: "batch worker drops the last page", CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Issue: store.Issue{
			ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004", HierarchyLevel: 0,
			Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
		},
	}}
	recs = append(recs, extra...)
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"3": "inprogress", "10001": "done", "10000": "new"},
		Records:    recs,
	}); err != nil {
		t.Fatal(err)
	}
	return db
}

func epicRec(id, key, project, title, updated, statusID, category string, level int) store.IssueRecord {
	// IssueType is a localized display name on purpose: the hint query must
	// key on hierarchy_level, never on "Epic".
	return store.IssueRecord{
		Item: store.Item{
			ID: "jira:" + id, SourceID: "jira", Kind: "issue", ExternalID: id, Key: key,
			Title: title, CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: updated,
		},
		Issue: store.Issue{
			ProjectKey: project, IssueType: "에픽", IssueTypeID: "10001", HierarchyLevel: level,
			Status: "진행 중", StatusID: statusID, StatusCategory: category,
		},
	}
}
