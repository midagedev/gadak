package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/fields"
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
	EditMeta(ctx context.Context, key string) (map[string]jira.FieldMeta, error)
	UpdateFields(ctx context.Context, key string, fields map[string]any) error
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
func (l liveClient) EditMeta(ctx context.Context, key string) (map[string]jira.FieldMeta, error) {
	return l.c.EditMeta(ctx, key)
}
func (l liveClient) UpdateFields(ctx context.Context, key string, fields map[string]any) error {
	return l.c.UpdateFields(ctx, key, fields)
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
	// Docs surfaces have no issue key — issue write paths must not fall through
	// to a hidden list cursor.
	if m.mode == modeDocs || m.mode == modeDocDetail ||
		(m.mode == modeFilter && m.filterFrom == modeDocs) {
		return "", false
	}
	// Detail (list or feed) always wins — the list cursor may not match the open issue.
	if m.mode == modeDetail && m.detailKey != "" {
		return m.detailKey, true
	}
	if m.mode == modeFeed {
		if len(m.feedItems) == 0 || m.feedCursor < 0 || m.feedCursor >= len(m.feedItems) {
			return "", false
		}
		k := m.feedItems[m.feedCursor].IssueKey
		if k == "" {
			return "", false
		}
		return k, true
	}
	if len(m.visible) == 0 || m.cursor < 0 || m.cursor >= len(m.visible) {
		return "", false
	}
	return m.all[m.visible[m.cursor]].lite.IssueKey, true
}

// inDocsSurface reports whether the navigator is showing docs (list, detail, or
// a / filter opened from docs). Issue-only actions should refuse honestly.
func (m Model) inDocsSurface() bool {
	return m.mode == modeDocs || m.mode == modeDocDetail ||
		(m.mode == modeFilter && m.filterFrom == modeDocs)
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

// fieldEditCandidate is one alias that is editable on the selected issue.
type fieldEditCandidate struct {
	alias   string
	label   string
	fieldID string
	kind    string
	meta    jira.FieldMeta
	current string // display of current value, may be empty
}

func (m *Model) startFieldEdit() tea.Cmd {
	key, ok := m.selectedKey()
	if !ok {
		return toast("no issue selected", true)
	}
	if _, err := m.requireWrite(); err != nil {
		return toast(err.Error(), true)
	}
	cfg, db := m.cfg, m.db
	var custom map[string]any
	if m.detail != nil && m.detail.Custom != nil {
		custom = m.detail.Custom
	}
	// Label lookup from field specs (alias → display label).
	labelByAlias := map[string]string{}
	for _, s := range cfg.FieldSpecs() {
		if s.Alias == "" {
			continue
		}
		label := s.Label
		if label == "" {
			label = s.Alias
		}
		labelByAlias[s.Alias] = label
	}
	return func() tea.Msg {
		allow := fields.EditableAliases(cfg)
		if len(allow) == 0 {
			return formResultMsg{note: "no editable fields configured — see settings"}
		}
		c := clientFactory(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		meta, err := c.EditMeta(ctx, key)
		if err != nil {
			return formResultMsg{err: err}
		}
		cands := make([]fieldEditCandidate, 0, len(allow))
		for alias, ea := range allow {
			id, kind, present := fields.ResolveEditableID(ea.IDs, meta)
			if !present {
				continue
			}
			if kind == "" {
				kind = ea.Kind
			}
			if kind == "" {
				continue
			}
			label := labelByAlias[alias]
			if label == "" {
				label = alias
			}
			cur := ""
			if custom != nil {
				cur = customDisplay(custom[alias])
			}
			cands = append(cands, fieldEditCandidate{
				alias:   alias,
				label:   label,
				fieldID: id,
				kind:    kind,
				meta:    meta[id],
				current: cur,
			})
		}
		if len(cands) == 0 {
			return formResultMsg{note: key + " has no editable fields"}
		}
		sort.Slice(cands, func(i, j int) bool {
			return strings.ToLower(cands[i].label) < strings.ToLower(cands[j].label)
		})
		opts := make([]huh.Option[string], 0, len(cands))
		byAlias := map[string]fieldEditCandidate{}
		for _, cand := range cands {
			byAlias[cand.alias] = cand
			label := cand.label
			if cand.current != "" {
				label = cand.label + " · current: " + cand.current
			}
			opts = append(opts, huh.NewOption(label, cand.alias))
		}
		var chosen string
		form := actionForm(
			huh.NewSelect[string]().
				Title(key + " · edit field").
				Options(opts...).
				Value(&chosen),
		)
		return openFormMsg{
			title: key + " · edit field",
			form:  form,
			submit: func() tea.Cmd {
				return func() tea.Msg {
					cand, ok := byAlias[chosen]
					if !ok || chosen == "" {
						return formResultMsg{note: "no field selected"}
					}
					return openFieldValueForm(cfg, db, key, cand, custom)
				}
			},
		}
	}
}

// openFieldValueForm builds the second-stage value picker for one candidate.
// Returned as tea.Msg so openFormMsg / formResultMsg both work as outcomes.
func openFieldValueForm(cfg *config.Config, db *store.DB, key string, cand fieldEditCandidate, custom map[string]any) tea.Msg {
	title := key + " · " + cand.label
	switch cand.kind {
	case "multi_option", "version_array":
		opts := make([]huh.Option[string], 0, len(cand.meta.AllowedValues))
		for _, v := range cand.meta.AllowedValues {
			label := v.Value
			if label == "" {
				label = v.Name
			}
			opts = append(opts, huh.NewOption(label, v.ID))
		}
		selected := preselectMultiIDs(cand.meta, custom[cand.alias])
		form := actionForm(
			huh.NewMultiSelect[string]().
				Title(title).
				Options(opts...).
				Value(&selected),
		)
		alias, fieldID, kind := cand.alias, cand.fieldID, cand.kind
		return openFormMsg{
			title: title,
			form:  form,
			submit: func() tea.Cmd {
				return fieldEditSubmit(cfg, db, key, alias, fieldID, kind, selected)
			},
		}
	case "user":
		opts := []huh.Option[string]{
			huh.NewOption("(unassign/clear)", ""),
		}
		for _, mem := range cfg.Members {
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
		var picked string
		form := actionForm(
			huh.NewSelect[string]().
				Title(title).
				Options(opts...).
				Value(&picked),
		)
		alias, fieldID, kind := cand.alias, cand.fieldID, cand.kind
		return openFormMsg{
			title: title,
			form:  form,
			submit: func() tea.Cmd {
				return fieldEditSubmit(cfg, db, key, alias, fieldID, kind, []string{picked})
			},
		}
	default:
		// option / version (single) and any other single-id kind.
		opts := []huh.Option[string]{
			huh.NewOption("(clear)", ""),
		}
		for _, v := range cand.meta.AllowedValues {
			label := v.Value
			if label == "" {
				label = v.Name
			}
			opts = append(opts, huh.NewOption(label, v.ID))
		}
		var picked string
		form := actionForm(
			huh.NewSelect[string]().
				Title(title).
				Options(opts...).
				Value(&picked),
		)
		alias, fieldID, kind := cand.alias, cand.fieldID, cand.kind
		return openFormMsg{
			title: title,
			form:  form,
			submit: func() tea.Cmd {
				return fieldEditSubmit(cfg, db, key, alias, fieldID, kind, []string{picked})
			},
		}
	}
}

func fieldEditSubmit(cfg *config.Config, db *store.DB, key, alias, fieldID, kind string, ids []string) tea.Cmd {
	// Copy so the caller can reuse / mutate the slice after we return.
	picked := append([]string(nil), ids...)
	return func() tea.Msg {
		c := clientFactory(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		value := fields.ValueFromIDs(kind, picked)
		if err := c.UpdateFields(ctx, key, map[string]any{fieldID: value}); err != nil {
			return formResultMsg{err: err}
		}
		if err := syncIssueFn(ctx, cfg, db, key); err != nil {
			return formResultMsg{err: fmt.Errorf("%s %s applied, mirror refresh failed: %w", key, alias, err), key: key}
		}
		return formResultMsg{note: key + " " + alias + " updated", key: key}
	}
}

// preselectMultiIDs marks AllowedValues whose labels match current custom
// display tokens (case-insensitive). Mismatches are ignored quietly.
func preselectMultiIDs(meta jira.FieldMeta, current any) []string {
	tokens := customValueTokens(current)
	if len(tokens) == 0 {
		return nil
	}
	var out []string
	for _, v := range meta.AllowedValues {
		label := v.Value
		if label == "" {
			label = v.Name
		}
		for _, tok := range tokens {
			if strings.EqualFold(label, tok) {
				out = append(out, v.ID)
				break
			}
		}
	}
	return out
}

// customValueTokens expands a Custom map value into display tokens for matching.
func customValueTokens(v any) []string {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		return []string{s}
	case []any:
		out := make([]string, 0, len(t))
		for _, el := range t {
			s := strings.TrimSpace(fmt.Sprint(el))
			if s == "" || s == "<nil>" {
				continue
			}
			out = append(out, s)
		}
		return out
	case nil:
		return nil
	default:
		s := customDisplay(v)
		if s == "" {
			return nil
		}
		return []string{s}
	}
}

func toast(text string, isErr bool) tea.Cmd {
	return func() tea.Msg {
		return toastMsg{text: text, err: isErr}
	}
}
