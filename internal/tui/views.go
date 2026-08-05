package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/midagedev/scry/internal/store"
)

type viewsLoadedMsg struct {
	views []store.SavedView
	err   error
}

func (m Model) loadViewsCmd() tea.Cmd {
	db := m.db
	return func() tea.Msg {
		if db == nil {
			return viewsLoadedMsg{err: fmt.Errorf("no database")}
		}
		views, err := db.SavedViews()
		if err != nil {
			return viewsLoadedMsg{err: err}
		}
		return viewsLoadedMsg{views: views}
	}
}

func (m Model) handleViewsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Views), key.Matches(msg, k.Back), msg.String() == "esc":
		m.mode = modeList
		m.viewCursor = 0
		return m, nil
	case key.Matches(msg, k.Down):
		if n := len(m.savedViews); n > 0 {
			if m.viewCursor < n-1 {
				m.viewCursor++
			}
		}
		return m, nil
	case key.Matches(msg, k.Up):
		if m.viewCursor > 0 {
			m.viewCursor--
		}
		return m, nil
	case key.Matches(msg, k.Top):
		m.viewCursor = 0
		return m, nil
	case key.Matches(msg, k.Bottom):
		if n := len(m.savedViews); n > 0 {
			m.viewCursor = n - 1
		}
		return m, nil
	case key.Matches(msg, k.Enter):
		if len(m.savedViews) == 0 || m.viewCursor < 0 || m.viewCursor >= len(m.savedViews) {
			return m, nil
		}
		m.applySavedView(m.savedViews[m.viewCursor])
		m.mode = modeList
		return m, nil
	}
	return m, nil
}

func (m *Model) applySavedView(v store.SavedView) {
	av := parseSavedViewConfig(v.Name, v.Config)
	m.extraFilter = av.filter
	// Tab bar follows derived tab; text filter field shows q for / display.
	m.tab = av.filter.tab
	m.filter = av.filter.text
	m.viewName = av.name
	m.listSort = av.sort
	m.groupBy = av.groupBy
	if len(av.unsupported) > 0 {
		m.viewNote = "unsupported filter ignored: " + strings.Join(av.unsupported, ", ")
		m.toast = m.viewNote
		m.toastErr = false
		m.toastAt = m.clock()
	} else {
		m.viewNote = ""
		m.toast = "view: " + av.name
		m.toastErr = false
		m.toastAt = m.clock()
	}
	m.refilter()
}

// clearSavedViewFilters drops view-only constraints (statusCategories, unassigned,
// assigneeEmail, sort, group) while keeping the current tab and free-text filter.
func (m *Model) clearSavedViewFilters() {
	m.extraFilter = listFilter{}
	m.viewName = ""
	m.viewNote = ""
	m.listSort = listSort{}
	m.groupBy = ""
}

func (m Model) viewViews() string {
	var b strings.Builder
	w := m.width

	b.WriteString(m.renderHeader(w))
	b.WriteByte('\n')
	b.WriteString(fitWidth(" "+styleTabActive.Render(" saved views "), w))
	b.WriteByte('\n')

	listH := m.listHeight()
	if len(m.savedViews) == 0 {
		empty := styleMuted.Render("  no saved views — press v/esc to leave")
		for i := 0; i < listH; i++ {
			if i == 0 {
				b.WriteString(fitWidth(empty, w))
			}
			b.WriteByte('\n')
		}
	} else {
		// Simple scroll around cursor.
		offset := 0
		if m.viewCursor >= listH {
			offset = m.viewCursor - listH + 1
		}
		end := offset + listH
		if end > len(m.savedViews) {
			end = len(m.savedViews)
		}
		for i := offset; i < end; i++ {
			v := m.savedViews[i]
			label := "  " + v.Name
			if i == m.viewCursor {
				b.WriteString(styleSel.Width(w).MaxWidth(w).Render(styleSel.Render(label)))
			} else {
				b.WriteString(fitWidth(stylePrimary.Render(label), w))
			}
			b.WriteByte('\n')
		}
		for i := end - offset; i < listH; i++ {
			b.WriteByte('\n')
		}
	}

	footer := styleHelp.Render("j/k · enter apply · v/esc back · q")
	b.WriteString(fitWidth(footer, w))
	return b.String()
}
