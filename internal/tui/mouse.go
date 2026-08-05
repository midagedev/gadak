package tui

// Mouse support: wheel scrolls the list / feed / detail, a click moves the
// cursor, a click on the already-selected row opens it, and the tab bar is
// clickable. The layout constants here mirror viewList: header (1 line) +
// tabs (1 line), then listHeight() rows, then the status bar.

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const listTopLines = 2 // header + tab bar, see viewList

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Palette open: wheel moves the palette cursor, anything else is ignored.
	if m.pal != nil {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.pal.cursor > 0 {
				m.pal.cursor--
			}
		case tea.MouseButtonWheelDown:
			if m.pal.cursor < len(m.pal.filtered)-1 {
				m.pal.cursor++
			}
		}
		return m, nil
	}

	switch m.mode {
	case modeForm:
		return m, nil // huh owns the form; keyboard only
	case modeDetail:
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			// Detail has no scroll state (fits or truncates); wheel goes back to
			// the list so a mouse-only reader is never stuck.
			return m, nil
		}
		return m, nil
	case modeFeed:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.moveFeedCursor(-1)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.moveFeedCursor(1)
			return m, nil
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if i := m.feedOffset + msg.Y - listTopLines; i >= 0 && i < len(m.feedItems) {
				if m.feedCursor == i {
					return m.openFeedSelection()
				}
				m.feedCursor = i
				m.ensureFeedVisible()
			}
		}
		return m, nil
	}

	// List (and views picker falls back to list behaviour for the wheel).
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.moveCursor(-1)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.moveCursor(1)
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft || m.mode != modeList {
		return m, nil
	}

	// Tab bar click: resolve which label the X position falls on.
	if msg.Y == 1 {
		if t, ok := m.tabAt(msg.X); ok {
			m.setTab(t)
		}
		return m, nil
	}

	// Row click: screen line → visible index (grouping inserts header lines).
	line := msg.Y - listTopLines
	if line < 0 || line >= m.listHeight() {
		return m, nil
	}
	idx := m.offset + line
	lines := m.listLines()
	var visIdx int
	if lines != nil {
		if idx >= len(lines) || lines[idx].kind == lineKindHeader {
			return m, nil
		}
		visIdx = lines[idx].visIdx
	} else {
		if idx >= len(m.visible) {
			return m, nil
		}
		visIdx = idx
	}
	if visIdx == m.cursor {
		// Second click on the selected row opens it.
		r := m.all[m.visible[visIdx]]
		m.detailFrom = modeList
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.loadDetailCmd(r.lite.IssueKey, r.lite))
	}
	m.cursor = visIdx
	m.ensureVisible()
	return m, nil
}

// tabAt maps an X position on the tab bar to the tab under it, using the same
// label layout renderTabs produces (1-cell left margin, 1 space between tabs,
// 1-cell padding inside each label).
func (m Model) tabAt(x int) (Tab, bool) {
	tabs := []Tab{TabAll, TabOpen, TabInProgress, TabDone}
	counts := make(map[Tab]int, 4)
	for _, r := range m.all {
		for _, t := range tabs {
			if matchTab(t, r.lite.StatusCategory) {
				counts[t]++
			}
		}
	}
	pos := 1 // leading space in renderTabs
	for i, t := range tabs {
		label := renderTabLabel(i, t, counts[t])
		w := lipgloss.Width(label) + 2 // Padding(0,1) on both tab styles
		if x >= pos && x < pos+w {
			return t, true
		}
		pos += w + 1 // joined with a single space
	}
	return TabAll, false
}
