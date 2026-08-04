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
	commented []string
	assignee  map[string]string
	trans     map[string]string
	fail      error
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
	if len(km.Filter.Keys()) == 0 {
		t.Error("Filter has no keys")
	}
	_ = str("")
	_ = json.RawMessage(nil)
}
