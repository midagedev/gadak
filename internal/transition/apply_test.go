package transition

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

type stubOrigin struct {
	list        []jira.Transition
	listErr     error
	postID      string
	postFields  map[string]any
	postComment json.RawMessage
	posted      bool
	postErr     error
	resolutions []jira.NamedID
	resErr      error
	status      jira.Status
	statusErr   error
	statusN     int
}

func (s *stubOrigin) Transitions(context.Context, string) ([]jira.Transition, error) {
	return s.list, s.listErr
}

func (s *stubOrigin) Transition(_ context.Context, _, id string, fields map[string]any, comment json.RawMessage) error {
	s.posted = true
	s.postID = id
	s.postFields = fields
	s.postComment = comment
	return s.postErr
}

func (s *stubOrigin) Resolutions(context.Context) ([]jira.NamedID, error) {
	return s.resolutions, s.resErr
}

func (s *stubOrigin) IssueStatus(context.Context, string) (jira.Status, *jira.User, error) {
	s.statusN++
	return s.status, nil, s.statusErr
}

func doneStatus() jira.Status {
	var st jira.Status
	st.ID = "10001"
	st.Name = "완료"
	st.StatusCategory.Key = "done"
	return st
}

func inProgressStatus() jira.Status {
	var st jira.Status
	st.ID = "3"
	st.Name = "진행 중"
	st.StatusCategory.Key = "indeterminate"
	return st
}

type noCatalog struct {
	list   []jira.Transition
	posted bool
}

func (n *noCatalog) Transitions(context.Context, string) ([]jira.Transition, error) {
	return n.list, nil
}

func (n *noCatalog) Transition(context.Context, string, string, map[string]any, json.RawMessage) error {
	n.posted = true
	return nil
}

func doneClose() jira.Transition {
	return jira.Transition{
		ID:   "31",
		Name: "Close",
		To: jira.Status{
			ID:   "10001",
			Name: "완료",
			StatusCategory: struct {
				Key string `json:"key"`
			}{Key: "done"},
		},
	}
}

func requiredResolution() jira.Transition {
	t := doneClose()
	t.ID = "41"
	t.Name = "Resolve"
	f := jira.TransitionField{
		Required: true,
		Name:     "Resolution",
		AllowedValues: []jira.NamedID{
			{ID: "10099", Name: "Won't Do"},
			{ID: "10000", Name: "Done"},
		},
	}
	f.Schema.Type = "resolution"
	t.Fields = map[string]jira.TransitionField{"resolution": f}
	return t
}

func TestApplyPicksCategoryAndOmitsEmpty(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{doneClose()}}
	res, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("write path must report changed")
	}
	if !s.posted || s.postID != "31" {
		t.Fatalf("posted id %q, posted=%v", s.postID, s.posted)
	}
	if s.postFields != nil {
		t.Fatalf("fields %v, want omitted", s.postFields)
	}
	if len(s.postComment) != 0 {
		t.Fatalf("comment %s, want omitted", s.postComment)
	}
	// GDK-632 rewrote this pin (was 0): a category target pays one status
	// read even on a pick hit, because a self-loop workflow keeps a
	// same-category transition available and a retry must still no-op.
	if s.statusN != 1 {
		t.Fatalf("status lookups %d, want 1 for a category target", s.statusN)
	}
}

func TestApplyRequiredResolutionRefusesWithoutValue(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{requiredResolution()}}
	_, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done"})
	if err == nil {
		t.Fatal("required resolution must refuse")
	}
	if s.posted {
		t.Fatal("must not POST")
	}
	if !IsRefused(err) {
		t.Fatalf("IsRefused=false for %T %v", err, err)
	}
	var req *RequiredFieldsError
	if !errors.As(err, &req) {
		t.Fatalf("want RequiredFieldsError, got %T", err)
	}
	msg := err.Error()
	for _, want := range []string{"resolution", "Won't Do", "Done"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestApplyResolutionNameUsesAllowedValues(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{requiredResolution()}}
	if _, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Resolution: "Won't Do"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.postFields["resolution"].(map[string]string)
	if got["id"] != "10099" {
		t.Fatalf("resolution %v, want id 10099 from allowedValues", s.postFields["resolution"])
	}
}

func TestApplyResolutionDigitsAreID(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{doneClose()}}
	if _, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Resolution: "10002"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.postFields["resolution"].(map[string]string)
	if got["id"] != "10002" {
		t.Fatalf("resolution %v, want typed id", s.postFields["resolution"])
	}
}

func TestApplyResolutionNameUsesCatalog(t *testing.T) {
	s := &stubOrigin{
		list:        []jira.Transition{doneClose()},
		resolutions: []jira.NamedID{{ID: "10000", Name: "Done"}, {ID: "10002", Name: "Won't Do"}},
	}
	if _, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Resolution: "Won't Do"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.postFields["resolution"].(map[string]string)
	if got["id"] != "10002" {
		t.Fatalf("resolution %v, want catalog id 10002", s.postFields["resolution"])
	}
}

func TestApplyResolutionUnknownIsRefused(t *testing.T) {
	s := &stubOrigin{
		list:        []jira.Transition{doneClose()},
		resolutions: []jira.NamedID{{ID: "10000", Name: "Done"}, {ID: "10002", Name: "Won't Do"}},
	}
	_, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Resolution: "Mystery"})
	if err == nil {
		t.Fatal("unknown resolution must refuse")
	}
	if s.posted {
		t.Fatal("must not POST")
	}
	if !IsRefused(err) {
		t.Fatalf("IsRefused=false for %v", err)
	}
}

func TestApplyNoCatalogOriginRefusesName(t *testing.T) {
	n := &noCatalog{list: []jira.Transition{doneClose()}}
	_, err := Apply(context.Background(), n, nil, Request{Key: "NMB-1", Target: "done", Resolution: "Won't Do"})
	if err == nil {
		t.Fatal("name without catalog must refuse")
	}
	if n.posted {
		t.Fatal("must not POST")
	}
	if !IsRefused(err) {
		t.Fatalf("IsRefused=false for %v", err)
	}
}

func TestApplyCommentSendsADF(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{doneClose()}}
	if _, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Comment: "closing out"}); err != nil {
		t.Fatal(err)
	}
	want := string(jira.Doc("closing out", nil))
	if string(s.postComment) != want {
		t.Fatalf("comment ADF %s, want %s", s.postComment, want)
	}
}

func TestApplyRemapsFieldAlias(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{doneClose()}}
	cfg := &config.Config{
		Fields: []config.FieldSpec{
			{Alias: "severity", Label: "Severity", IDs: []string{"customfield_10001"}, Role: "facet", Kind: "option"},
		},
	}
	_, err := Apply(context.Background(), s, cfg, Request{
		Key:    "NMB-1",
		Target: "31",
		Fields: map[string]any{"severity": "High"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.postFields["customfield_10001"]; !ok {
		t.Fatalf("alias not remapped: %v", s.postFields)
	}
	if _, ok := s.postFields["severity"]; ok {
		t.Fatalf("alias leaked: %v", s.postFields)
	}
}

func TestApplyPickMissIsRefused(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{doneClose()}}
	_, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "nonsense"})
	if err == nil {
		t.Fatal("miss must refuse")
	}
	if s.posted {
		t.Fatal("must not POST")
	}
	if !IsRefused(err) {
		t.Fatalf("IsRefused=false for %v", err)
	}
	if !strings.Contains(err.Error(), "Close") {
		t.Fatalf("refusal must name candidates: %v", err)
	}
}

func TestApplyDoesNotMutateCallerFields(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{doneClose()}}
	cfg := &config.Config{
		Fields: []config.FieldSpec{
			{Alias: "severity", IDs: []string{"customfield_10001"}, Kind: "option"},
		},
	}
	in := map[string]any{"severity": "High"}
	if _, err := Apply(context.Background(), s, cfg, Request{Key: "NMB-1", Target: "31", Fields: in}); err != nil {
		t.Fatal(err)
	}
	if _, ok := in["severity"]; !ok {
		t.Fatal("caller map lost severity")
	}
	if _, ok := in["customfield_10001"]; ok {
		t.Fatal("caller map gained remapped key")
	}
}

// GDK-500: a category token whose landing is already the origin's status
// used to refuse ("no transition matching…") — a false failure for an
// agent retry. After the fix this is a no-op success (Changed=false) and
// must not POST, including a -m comment that would otherwise duplicate.
func TestApplyCategoryTokenAlreadyThereIsNoop(t *testing.T) {
	s := &stubOrigin{list: nil, status: doneStatus()}
	res, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Comment: "retry"})
	if err != nil {
		t.Fatalf("already in target category must succeed, got %v", err)
	}
	if res.Changed {
		t.Fatal("want changed=false")
	}
	if s.posted {
		t.Fatal("must not POST")
	}
	if len(s.postComment) != 0 {
		t.Fatal("must not post comment")
	}
	if s.statusN != 1 {
		t.Fatalf("status lookups %d, want 1 after pick miss", s.statusN)
	}
}

func TestApplyInProgressTokenAlreadyThereIsNoop(t *testing.T) {
	s := &stubOrigin{list: nil, status: inProgressStatus()}
	res, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "inprogress"})
	if err != nil {
		t.Fatalf("already inprogress must succeed, got %v", err)
	}
	if res.Changed || s.posted {
		t.Fatalf("changed=%v posted=%v, want no-op", res.Changed, s.posted)
	}
}

func TestApplyNamedMissStaysErrorWhenAlreadyDone(t *testing.T) {
	s := &stubOrigin{list: nil, status: doneStatus()}
	_, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "Close"})
	if err == nil {
		t.Fatal("named miss must stay an error")
	}
	if s.posted {
		t.Fatal("must not POST")
	}
	if s.statusN != 0 {
		t.Fatalf("status lookups %d, want 0 for a non-token", s.statusN)
	}
}

func TestApplyCategoryTokenWrongCategoryStaysError(t *testing.T) {
	s := &stubOrigin{list: nil, status: doneStatus()}
	_, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "inprogress"})
	if err == nil {
		t.Fatal("wrong category must stay an error")
	}
	if s.posted {
		t.Fatal("must not POST")
	}
	if !IsRefused(err) {
		t.Fatalf("IsRefused=false for %v", err)
	}
}

func TestApplyNoCatalogOriginCannotNoop(t *testing.T) {
	n := &noCatalog{list: nil}
	_, err := Apply(context.Background(), n, nil, Request{Key: "NMB-1", Target: "done"})
	if err == nil {
		t.Fatal("origin without IssueStatus cannot no-op")
	}
	if n.posted {
		t.Fatal("must not POST")
	}
}

func TestApplyIssueStatusErrorSurfaces(t *testing.T) {
	s := &stubOrigin{list: nil, statusErr: errors.New("origin down")}
	_, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done"})
	if err == nil || err.Error() != "origin down" {
		t.Fatalf("want origin status error, got %v", err)
	}
	if s.posted {
		t.Fatal("must not POST")
	}
}

// GDK-632, caught on a real site: a self-loop workflow keeps a done→done
// transition available while the issue is already done, so the pick
// succeeds and the old gate (which ran only after a pick miss) never
// engaged — a retry fired the transition again and would re-post its
// comment. The category gate must hold on both pick outcomes.
func TestApplyCategorySelfLoopIsNoop(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{doneClose()}, status: doneStatus()}
	res, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Comment: "retry"})
	if err != nil {
		t.Fatalf("self-loop retry must succeed as a no-op, got %v", err)
	}
	if res.Changed {
		t.Fatal("want changed=false: the issue is already done")
	}
	if s.posted {
		t.Fatal("must not fire the self-loop transition again")
	}
}

func TestPreviewCategorySelfLoopIsNoop(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{doneClose()}, status: doneStatus()}
	id, changed, err := Preview(context.Background(), s, "NMB-1", "done")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if changed || id != "" {
		t.Fatalf("id=%q changed=%v, want the no-op", id, changed)
	}
}

// The gate's cost contract: a category target that is NOT already there
// still fires, paying exactly one status read; a named target never pays.
func TestApplyCategoryGateCosts(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{doneClose()}, status: inProgressStatus()}
	res, err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done"})
	if err != nil || !res.Changed || !s.posted {
		t.Fatalf("res=%+v err=%v posted=%v, want a real write", res, err, s.posted)
	}
	if s.statusN != 1 {
		t.Fatalf("status lookups %d, want exactly 1 for a category write", s.statusN)
	}

	named := &stubOrigin{list: []jira.Transition{doneClose()}, status: doneStatus()}
	if _, err := Apply(context.Background(), named, nil, Request{Key: "NMB-1", Target: "Close"}); err != nil {
		t.Fatalf("named: %v", err)
	}
	if named.statusN != 0 {
		t.Fatalf("named target paid %d status lookups, want 0", named.statusN)
	}
}
