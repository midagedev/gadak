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
	if err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done"}); err != nil {
		t.Fatal(err)
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
}

func TestApplyRequiredResolutionRefusesWithoutValue(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{requiredResolution()}}
	err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done"})
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
	if err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Resolution: "Won't Do"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.postFields["resolution"].(map[string]string)
	if got["id"] != "10099" {
		t.Fatalf("resolution %v, want id 10099 from allowedValues", s.postFields["resolution"])
	}
}

func TestApplyResolutionDigitsAreID(t *testing.T) {
	s := &stubOrigin{list: []jira.Transition{doneClose()}}
	if err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Resolution: "10002"}); err != nil {
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
	if err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Resolution: "Won't Do"}); err != nil {
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
	err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Resolution: "Mystery"})
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
	err := Apply(context.Background(), n, nil, Request{Key: "NMB-1", Target: "done", Resolution: "Won't Do"})
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
	if err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "done", Comment: "closing out"}); err != nil {
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
	err := Apply(context.Background(), s, cfg, Request{
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
	err := Apply(context.Background(), s, nil, Request{Key: "NMB-1", Target: "nonsense"})
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
	if err := Apply(context.Background(), s, cfg, Request{Key: "NMB-1", Target: "31", Fields: in}); err != nil {
		t.Fatal(err)
	}
	if _, ok := in["severity"]; !ok {
		t.Fatal("caller map lost severity")
	}
	if _, ok := in["customfield_10001"]; ok {
		t.Fatal("caller map gained remapped key")
	}
}
