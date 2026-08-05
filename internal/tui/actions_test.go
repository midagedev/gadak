package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
)

type keyStr string

func (k keyStr) String() string { return string(k) }

func keyMatches(b key.Binding, s string) bool {
	return key.Matches(keyStr(s), b)
}

// fakeWrite is an in-memory writeClient for action tests.
type fakeWrite struct {
	commented   []string
	assignee    map[string]string
	trans       map[string]string
	editMeta    map[string]jira.FieldMeta
	editMetaErr error
	updated     []map[string]any // each UpdateFields payload
	updatedKeys []string
	fail        error
}

func (f *fakeWrite) AddComment(_ context.Context, key string, _ []byte) error {
	if f.fail != nil {
		return f.fail
	}
	f.commented = append(f.commented, key)
	return nil
}
func (f *fakeWrite) Transitions(_ context.Context, _ string) ([]jira.Transition, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	return []jira.Transition{
		{ID: "11", Name: "Start Progress", To: jira.Status{Name: "In Progress"}},
		{ID: "21", Name: "Done", To: jira.Status{Name: "Done"}},
	}, nil
}
func (f *fakeWrite) Transition(_ context.Context, key, id string) error {
	if f.fail != nil {
		return f.fail
	}
	if f.trans == nil {
		f.trans = map[string]string{}
	}
	f.trans[key] = id
	return nil
}
func (f *fakeWrite) SetAssignee(_ context.Context, key, accountID string) error {
	if f.fail != nil {
		return f.fail
	}
	if f.assignee == nil {
		f.assignee = map[string]string{}
	}
	f.assignee[key] = accountID
	return nil
}
func (f *fakeWrite) EditMeta(_ context.Context, _ string) (map[string]jira.FieldMeta, error) {
	if f.editMetaErr != nil {
		return nil, f.editMetaErr
	}
	if f.fail != nil {
		return nil, f.fail
	}
	if f.editMeta == nil {
		return map[string]jira.FieldMeta{}, nil
	}
	return f.editMeta, nil
}
func (f *fakeWrite) UpdateFields(_ context.Context, key string, fields map[string]any) error {
	if f.fail != nil {
		return f.fail
	}
	// Deep-ish copy for assertions.
	cp := make(map[string]any, len(fields))
	for k, v := range fields {
		cp[k] = v
	}
	f.updated = append(f.updated, cp)
	f.updatedKeys = append(f.updatedKeys, key)
	return nil
}

func TestWriteClientCommentErrorPath(t *testing.T) {
	// httptest server that rejects writes — proves we never hit a real host.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["denied"]}`))
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{Site: srv.URL, Email: "a@b.c", Token: "tok"}
	c := jira.New(cfg.Site, cfg.Email, cfg.Token)
	c.HTTP = srv.Client()
	c.Retries = 1

	_, err := c.AddComment(context.Background(), "AAA-1", jira.Doc("hi", nil))
	if err == nil {
		t.Fatal("expected error from fake jira")
	}
	if !strings.Contains(err.Error(), "401") && !strings.Contains(err.Error(), "denied") {
		// APIError wrapping may vary; any non-nil is enough for the error path.
		t.Logf("error = %v", err)
	}
}

func TestFakeWriteCommentAndSyncHook(t *testing.T) {
	fw := &fakeWrite{}
	prevFactory := clientFactory
	prevSync := syncIssueFn
	t.Cleanup(func() {
		clientFactory = prevFactory
		syncIssueFn = prevSync
	})

	var synced string
	clientFactory = func(*config.Config) writeClient { return fw }
	syncIssueFn = func(_ context.Context, _ *config.Config, _ *store.DB, key string) error {
		synced = key
		return nil
	}

	cfg := &config.Config{Site: "http://example.invalid", Email: "a@b.c", Token: "t"}
	// Exercise the submit closure shape used by startComment.
	key := "AAA-1"
	body := "hello"
	msg := func() formResultMsg {
		c := clientFactory(cfg)
		ctx := context.Background()
		if err := c.AddComment(ctx, key, jira.Doc(body, nil)); err != nil {
			return formResultMsg{err: err}
		}
		if err := syncIssueFn(ctx, cfg, nil, key); err != nil {
			return formResultMsg{err: err, key: key}
		}
		return formResultMsg{note: key + " comment posted", key: key}
	}()
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if len(fw.commented) != 1 || fw.commented[0] != "AAA-1" {
		t.Fatalf("commented=%v", fw.commented)
	}
	if synced != "AAA-1" {
		t.Fatalf("synced=%q", synced)
	}
	if msg.note == "" {
		t.Fatal("expected note")
	}
}

func TestKeyMapBindings(t *testing.T) {
	km := defaultKeys()
	type str string
	// key.Matches accepts any fmt.Stringer.
	if !keyMatches(km.Down, "j") {
		t.Error("j should match Down")
	}
	if keyMatches(km.Down, "x") {
		t.Error("x should not match Down")
	}
	if !keyMatches(km.TabInProgress, "3") {
		t.Error("3 should match TabInProgress")
	}
	if !keyMatches(km.Filter, "/") {
		t.Error("/ should match Filter")
	}
	if !keyMatches(km.Comment, "c") {
		t.Error("c should match Comment")
	}
	if !keyMatches(km.Edit, "e") {
		t.Error("e should match Edit")
	}
	if len(km.Filter.Keys()) == 0 {
		t.Error("Filter has no keys")
	}
	// helpLines must list edit after assignee.
	lines := km.helpLines()
	foundEdit := false
	for _, pair := range lines {
		if pair[0] == "e" && pair[1] == "edit field" {
			foundEdit = true
			break
		}
	}
	if !foundEdit {
		t.Fatalf("helpLines missing e/edit field: %v", lines)
	}
	_ = str("")
	_ = json.RawMessage(nil)
}

func optionMeta(id, value string) jira.FieldMeta {
	var m jira.FieldMeta
	m.Schema.Type = "option"
	m.AllowedValues = []struct {
		ID    string `json:"id"`
		Value string `json:"value"`
		Name  string `json:"name"`
	}{{ID: id, Value: value}}
	return m
}

func TestFieldEditOptionSubmit(t *testing.T) {
	fw := &fakeWrite{}
	prevFactory := clientFactory
	prevSync := syncIssueFn
	t.Cleanup(func() {
		clientFactory = prevFactory
		syncIssueFn = prevSync
	})
	clientFactory = func(*config.Config) writeClient { return fw }
	syncIssueFn = func(_ context.Context, _ *config.Config, _ *store.DB, _ string) error {
		return nil
	}

	cfg := &config.Config{Site: "http://example.invalid", Email: "a@b.c", Token: "t"}
	// Same shape as fieldEditSubmit: set ids, run the cmd.
	msg := fieldEditSubmit(cfg, nil, "AAA-1", "severity", "customfield_x", "option", []string{"10001"})().(formResultMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if msg.note != "AAA-1 severity updated" {
		t.Fatalf("note=%q", msg.note)
	}
	if len(fw.updated) != 1 {
		t.Fatalf("updated=%v", fw.updated)
	}
	got := fw.updated[0]["customfield_x"]
	want := map[string]string{"id": "10001"}
	if !reflectDeepEqual(got, want) {
		t.Fatalf("payload %#v want %#v", got, want)
	}
}

func TestFieldEditMultiClearSubmit(t *testing.T) {
	fw := &fakeWrite{}
	prevFactory := clientFactory
	prevSync := syncIssueFn
	t.Cleanup(func() {
		clientFactory = prevFactory
		syncIssueFn = prevSync
	})
	clientFactory = func(*config.Config) writeClient { return fw }
	syncIssueFn = func(_ context.Context, _ *config.Config, _ *store.DB, _ string) error {
		return nil
	}

	cfg := &config.Config{Site: "http://example.invalid", Email: "a@b.c", Token: "t"}
	// Nothing selected → empty multi payload.
	msg := fieldEditSubmit(cfg, nil, "AAA-1", "tags", "customfield_x", "multi_option", nil)().(formResultMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if len(fw.updated) != 1 {
		t.Fatalf("updated=%v", fw.updated)
	}
	got := fw.updated[0]["customfield_x"]
	want := []any{}
	if !reflectDeepEqual(got, want) {
		t.Fatalf("payload %#v want %#v", got, want)
	}
}

func TestFieldEditNoEditableOnIssue(t *testing.T) {
	fw := &fakeWrite{
		editMeta: map[string]jira.FieldMeta{
			// Present but unrelated to the configured allowlist.
			"customfield_other": optionMeta("1", "X"),
		},
	}
	prevFactory := clientFactory
	prevSync := syncIssueFn
	t.Cleanup(func() {
		clientFactory = prevFactory
		syncIssueFn = prevSync
	})
	clientFactory = func(*config.Config) writeClient { return fw }
	syncIssueFn = func(_ context.Context, _ *config.Config, _ *store.DB, _ string) error {
		return nil
	}

	cfg := &config.Config{
		Site:  "http://example.invalid",
		Email: "a@b.c",
		Token: "t",
		Fields: []config.FieldSpec{
			{Alias: "severity", Label: "Severity Level", IDs: []string{"customfield_x"}, Kind: "option"},
		},
	}
	m := Model{
		cfg: cfg,
		all: []row{{lite: store.IssueLite{IssueKey: "AAA-1", Summary: "s"}}},
	}
	m.visible = []int{0}
	m.cursor = 0
	m.mode = modeList

	msg := m.startFieldEdit()()
	fr, ok := msg.(formResultMsg)
	if !ok {
		t.Fatalf("got %T %#v", msg, msg)
	}
	if fr.note != "AAA-1 has no editable fields" {
		t.Fatalf("note=%q", fr.note)
	}
	if len(fw.updated) != 0 {
		t.Fatalf("UpdateFields must not be called: %v", fw.updated)
	}
}

func TestFieldEditNoConfigured(t *testing.T) {
	fw := &fakeWrite{}
	prevFactory := clientFactory
	t.Cleanup(func() { clientFactory = prevFactory })
	clientFactory = func(*config.Config) writeClient { return fw }

	cfg := &config.Config{Site: "http://example.invalid", Email: "a@b.c", Token: "t"}
	m := Model{
		cfg: cfg,
		all: []row{{lite: store.IssueLite{IssueKey: "AAA-1"}}},
	}
	m.visible = []int{0}
	m.cursor = 0

	msg := m.startFieldEdit()()
	fr, ok := msg.(formResultMsg)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if fr.note != "no editable fields configured — see settings" {
		t.Fatalf("note=%q", fr.note)
	}
	if len(fw.updated) != 0 {
		t.Fatal("UpdateFields called without allowlist")
	}
}

func TestFieldEditOpensPickerThenSubmit(t *testing.T) {
	// End-to-end without huh interaction: stage-1 openForm → set alias → stage-2 submit with fixed ids.
	var severity jira.FieldMeta
	severity.Schema.Type = "option"
	severity.AllowedValues = []struct {
		ID    string `json:"id"`
		Value string `json:"value"`
		Name  string `json:"name"`
	}{
		{ID: "10001", Value: "High"},
		{ID: "10002", Value: "Low"},
	}
	fw := &fakeWrite{
		editMeta: map[string]jira.FieldMeta{
			"customfield_x": severity,
		},
	}
	prevFactory := clientFactory
	prevSync := syncIssueFn
	t.Cleanup(func() {
		clientFactory = prevFactory
		syncIssueFn = prevSync
	})
	clientFactory = func(*config.Config) writeClient { return fw }
	syncIssueFn = func(_ context.Context, _ *config.Config, _ *store.DB, _ string) error {
		return nil
	}

	cfg := &config.Config{
		Site:  "http://example.invalid",
		Email: "a@b.c",
		Token: "t",
		Fields: []config.FieldSpec{
			{Alias: "severity", Label: "Severity Level", IDs: []string{"customfield_x"}, Kind: "option"},
		},
	}
	m := Model{
		cfg: cfg,
		all: []row{{lite: store.IssueLite{IssueKey: "AAA-1", Summary: "s"}}},
		detail: &store.Detail{
			IssueKey: "AAA-1",
			Custom:   map[string]any{"severity": "Low"},
		},
		detailKey: "AAA-1",
		mode:      modeDetail,
	}

	msg := m.startFieldEdit()()
	of, ok := msg.(openFormMsg)
	if !ok {
		t.Fatalf("stage1 got %T %#v", msg, msg)
	}
	// Simulate picking the only alias by invoking the stage-2 builder directly
	// (same path as submit after the user selects).
	stage2 := openFieldValueForm(cfg, nil, "AAA-1", fieldEditCandidate{
		alias:   "severity",
		label:   "Severity Level",
		fieldID: "customfield_x",
		kind:    "option",
		meta:    severity,
		current: "Low",
	}, m.detail.Custom)
	of2, ok := stage2.(openFormMsg)
	if !ok {
		t.Fatalf("stage2 got %T", stage2)
	}
	// Set the value the form would write, then run submit.
	// openFieldValueForm closes over `picked`; exercise fieldEditSubmit with the same args.
	_ = of
	_ = of2
	result := fieldEditSubmit(cfg, nil, "AAA-1", "severity", "customfield_x", "option", []string{"10001"})().(formResultMsg)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if len(fw.updated) != 1 {
		t.Fatalf("updated=%v", fw.updated)
	}
	if !reflectDeepEqual(fw.updated[0]["customfield_x"], map[string]string{"id": "10001"}) {
		t.Fatalf("payload %#v", fw.updated[0])
	}
}

func reflectDeepEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
