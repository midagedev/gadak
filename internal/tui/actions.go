package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
	syncer "github.com/midagedev/scry/internal/sync"
)

// writeClient is the Jira surface the write actions need. Tests inject a fake.
type writeClient interface {
	AddComment(ctx context.Context, key string, adf []byte) error
	Transitions(ctx context.Context, key string) ([]jira.Transition, error)
	Transition(ctx context.Context, key, transitionID string) error
	SetAssignee(ctx context.Context, key, accountID string) error
}

// liveClient adapts *jira.Client to writeClient.
type liveClient struct{ c *jira.Client }

func (l liveClient) AddComment(ctx context.Context, key string, adf []byte) error {
	_, err := l.c.AddComment(ctx, key, adf)
	return err
}
func (l liveClient) Transitions(ctx context.Context, key string) ([]jira.Transition, error) {
	return l.c.Transitions(ctx, key)
}
func (l liveClient) Transition(ctx context.Context, key, id string) error {
	return l.c.Transition(ctx, key, id)
}
func (l liveClient) SetAssignee(ctx context.Context, key, accountID string) error {
	return l.c.SetAssignee(ctx, key, accountID)
}

// clientFactory builds a write client from config. Overridden in tests.
var clientFactory = func(cfg *config.Config) writeClient {
	return liveClient{c: jira.New(cfg.Site, cfg.Email, cfg.Token)}
}

// syncIssueFn re-reads one issue into the mirror after a write. Overridden in tests.
var syncIssueFn = func(ctx context.Context, cfg *config.Config, db *store.DB, key string) error {
	return syncer.SyncIssue(ctx, cfg, db, key, syncer.Options{})
}

type openFormMsg struct {
	title  string
	form   *huh.Form
	submit func() tea.Cmd
}

type formResultMsg struct {
	note string
	err  error
	// key, when set, triggers a list reload after a successful write.
	key string
}

type toastMsg struct {
	text string
	err  bool
}

func actionForm(fields ...huh.Field) *huh.Form {
	return huh.NewForm(huh.NewGroup(fields...)).
		WithShowHelp(true).
		WithShowErrors(true)
}

func (m *Model) requireWrite() (writeClient, error) {
	if m.cfg == nil || !m.cfg.HasCredential() {
		return nil, fmt.Errorf("credential required — run `scry init`")
	}
	return clientFactory(m.cfg), nil
}

func (m *Model) selectedKey() (string, bool) {
	if len(m.visible) == 0 || m.cursor < 0 || m.cursor >= len(m.visible) {
		return "", false
	}
	return m.all[m.visible[m.cursor]].lite.IssueKey, true
}

func (m *Model) startComment() tea.Cmd {
	key, ok := m.selectedKey()
	if !ok {
		return toast("no issue selected", true)
	}
	if _, err := m.requireWrite(); err != nil {
		return toast(err.Error(), true)
	}
	var body string
	form := actionForm(
		huh.NewText().
			Title(key + " · comment").
			Placeholder("Comment body (ctrl+c to cancel)").
			Value(&body).
			Lines(6).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("comment cannot be empty")
				}
				return nil
			}),
	)
	cfg, db := m.cfg, m.db
	return func() tea.Msg {
		return openFormMsg{
			title: key + " · comment",
			form:  form,
			submit: func() tea.Cmd {
				return func() tea.Msg {
					text := strings.TrimSpace(body)
					if text == "" {
						return formResultMsg{note: "empty comment — cancelled"}
					}
					c := clientFactory(cfg)
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if err := c.AddComment(ctx, key, jira.Doc(text, nil)); err != nil {
						return formResultMsg{err: err}
					}
					if err := syncIssueFn(ctx, cfg, db, key); err != nil {
						return formResultMsg{err: fmt.Errorf("comment posted, mirror refresh failed: %w", err), key: key}
					}
					return formResultMsg{note: key + " comment posted", key: key}
				}
			},
		}
	}
}

func (m *Model) startTransition() tea.Cmd {
	key, ok := m.selectedKey()
	if !ok {
		return toast("no issue selected", true)
	}
	if _, err := m.requireWrite(); err != nil {
		return toast(err.Error(), true)
	}
	cfg, db := m.cfg, m.db
	return func() tea.Msg {
		c := clientFactory(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		list, err := c.Transitions(ctx, key)
		if err != nil {
			return formResultMsg{err: err}
		}
		if len(list) == 0 {
			return formResultMsg{note: key + " has no available transitions"}
		}
		opts := make([]huh.Option[string], 0, len(list))
		for _, t := range list {
			label := t.Name
			if t.To.Name != "" {
				label += " → " + t.To.Name
			}
			opts = append(opts, huh.NewOption(label, t.ID))
		}
		var tid string
		form := actionForm(
			huh.NewSelect[string]().
				Title(key + " · transition").
				Options(opts...).
				Value(&tid),
		)
		return openFormMsg{
			title: key + " · transition",
			form:  form,
			submit: func() tea.Cmd {
				return func() tea.Msg {
					if tid == "" {
						return formResultMsg{note: "no transition selected"}
					}
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if err := c.Transition(ctx, key, tid); err != nil {
						return formResultMsg{err: err}
					}
					if err := syncIssueFn(ctx, cfg, db, key); err != nil {
						return formResultMsg{err: fmt.Errorf("transition applied, mirror refresh failed: %w", err), key: key}
					}
					return formResultMsg{note: key + " transitioned", key: key}
				}
			},
		}
	}
}

func (m *Model) startAssignee() tea.Cmd {
	key, ok := m.selectedKey()
	if !ok {
		return toast("no issue selected", true)
	}
	if _, err := m.requireWrite(); err != nil {
		return toast(err.Error(), true)
	}
	members := m.cfg.Members
	opts := []huh.Option[string]{
		huh.NewOption("(unassign)", ""),
	}
	for _, mem := range members {
		id := mem.JiraAccountID
		if id == "" {
			continue
		}
		label := mem.DisplayName
		if label == "" {
			label = mem.Name
		}
		if label == "" {
			label = mem.Email
		}
		if label == "" {
			label = id
		}
		opts = append(opts, huh.NewOption(label, id))
	}
	var accountID string
	form := actionForm(
		huh.NewSelect[string]().
			Title(key + " · assignee").
			Options(opts...).
			Value(&accountID),
	)
	cfg, db := m.cfg, m.db
	return func() tea.Msg {
		return openFormMsg{
			title: key + " · assignee",
			form:  form,
			submit: func() tea.Cmd {
				return func() tea.Msg {
					c := clientFactory(cfg)
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if err := c.SetAssignee(ctx, key, accountID); err != nil {
						return formResultMsg{err: err}
					}
					if err := syncIssueFn(ctx, cfg, db, key); err != nil {
						return formResultMsg{err: fmt.Errorf("assignee set, mirror refresh failed: %w", err), key: key}
					}
					note := key + " unassigned"
					if accountID != "" {
						note = key + " assignee updated"
					}
					return formResultMsg{note: note, key: key}
				}
			},
		}
	}
}

func toast(text string, isErr bool) tea.Cmd {
	return func() tea.Msg {
		return toastMsg{text: text, err: isErr}
	}
}
