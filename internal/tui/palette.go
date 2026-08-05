package tui

// Command palette (ctrl+k): one fuzzy box over everything the navigator can
// do — switch tabs, run actions, apply saved views, and jump straight to an
// issue by key or summary. Actions come first while the query is empty;
// issues rank up as soon as the query narrows them.

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type paletteItem struct {
	id    string // dispatch token: "tab:1", "act:comment", "view:3", "issue:KEY-1"
	label string
	hint  string
}

type paletteState struct {
	query    string
	items    []paletteItem // static half: tabs + actions + saved views
	filtered []paletteItem
	cursor   int
}

const paletteMaxRows = 10

// paletteActions is the static action list; ids map onto the same commands
// the single keys dispatch.
func (m *Model) paletteItems() []paletteItem {
	items := []paletteItem{
		{id: "tab:1", label: "Tab · All", hint: "1"},
		{id: "tab:2", label: "Tab · Open", hint: "2"},
		{id: "tab:3", label: "Tab · In progress", hint: "3"},
		{id: "tab:4", label: "Tab · Done", hint: "4"},
		{id: "act:comment", label: "Comment on issue", hint: "c"},
		{id: "act:transition", label: "Transition issue", hint: "t"},
		{id: "act:assignee", label: "Set assignee", hint: "a"},
		{id: "act:edit", label: "Edit field", hint: "e"},
		{id: "act:watch", label: "Watch / unwatch", hint: "w"},
		{id: "act:feed", label: "Personal feed", hint: "F"},
		{id: "act:docs", label: "Docs (mirrored pages)", hint: "D"},
		{id: "act:views", label: "Saved views", hint: "v"},
		{id: "act:refresh", label: "Reload mirror", hint: "r"},
		{id: "act:help", label: "Help", hint: "?"},
	}
	for i, v := range m.savedViews {
		items = append(items, paletteItem{
			id:    "view:" + v.ID,
			label: "View · " + v.Name,
			hint:  "v",
		})
		if i >= 20 {
			break
		}
	}
	return items
}

func (m *Model) openPalette() {
	m.pal = &paletteState{items: m.paletteItems()}
	m.refilterPalette()
}

func (m *Model) closePalette() { m.pal = nil }

// refilterPalette rebuilds the visible rows: matching static items first,
// then matching issues (key prefix beats summary substring).
func (m *Model) refilterPalette() {
	p := m.pal
	if p == nil {
		return
	}
	q := strings.ToLower(strings.TrimSpace(p.query))
	out := make([]paletteItem, 0, paletteMaxRows*2)
	for _, it := range p.items {
		if q == "" || strings.Contains(strings.ToLower(it.label), q) || strings.Contains(strings.ToLower(it.hint), q) {
			out = append(out, it)
		}
	}
	if q != "" {
		// Issues: key-prefix matches first, then summary substrings.
		var keyHits, sumHits []paletteItem
		for _, r := range m.all {
			key := strings.ToLower(r.lite.IssueKey)
			sum := strings.ToLower(r.lite.Summary)
			it := paletteItem{
				id:    "issue:" + r.lite.IssueKey,
				label: r.lite.IssueKey + "  " + truncate(r.lite.Summary, 48),
				hint:  r.lite.Status,
			}
			switch {
			case strings.HasPrefix(key, q):
				keyHits = append(keyHits, it)
			case strings.Contains(sum, q) || strings.Contains(key, q):
				sumHits = append(sumHits, it)
			}
			if len(keyHits) >= paletteMaxRows {
				break
			}
		}
		out = append(out, keyHits...)
		out = append(out, sumHits...)
	}
	p.filtered = out
	if p.cursor >= len(p.filtered) {
		p.cursor = len(p.filtered) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

// handlePaletteKey consumes every key while the palette is open.
func (m Model) handlePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.pal
	switch msg.String() {
	case "esc", "ctrl+k":
		m.closePalette()
		return m, nil
	case "enter":
		if p == nil || len(p.filtered) == 0 {
			m.closePalette()
			return m, nil
		}
		id := p.filtered[p.cursor].id
		m.closePalette()
		return m.dispatchPalette(id)
	case "up", "ctrl+p":
		if p.cursor > 0 {
			p.cursor--
		}
		return m, nil
	case "down", "ctrl+n":
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
		}
		return m, nil
	case "backspace":
		if p.query != "" {
			r := []rune(p.query)
			p.query = string(r[:len(r)-1])
			m.refilterPalette()
		}
		return m, nil
	case "ctrl+u":
		p.query = ""
		m.refilterPalette()
		return m, nil
	}
	if msg.Type == tea.KeyRunes || msg.String() == " " {
		p.query += string(msg.Runes)
		m.refilterPalette()
	}
	return m, nil
}

// dispatchPalette routes a selected item onto the same paths the keys use.
func (m Model) dispatchPalette(id string) (tea.Model, tea.Cmd) {
	switch {
	case strings.HasPrefix(id, "tab:"):
		switch id {
		case "tab:1":
			m.setTab(TabAll)
		case "tab:2":
			m.setTab(TabOpen)
		case "tab:3":
			m.setTab(TabInProgress)
		case "tab:4":
			m.setTab(TabDone)
		}
		m.mode = modeList
		return m, nil
	case strings.HasPrefix(id, "issue:"):
		key := strings.TrimPrefix(id, "issue:")
		for i, idx := range m.visible {
			if m.all[idx].lite.IssueKey == key {
				m.cursor = i
				m.ensureVisible()
				break
			}
		}
		for _, r := range m.all {
			if r.lite.IssueKey == key {
				m.detailFrom = m.mode
				return m, m.loadDetailCmd(key, r.lite)
			}
		}
		return m, nil
	case strings.HasPrefix(id, "view:"):
		vid := strings.TrimPrefix(id, "view:")
		for _, v := range m.savedViews {
			if v.ID == vid {
				m.applySavedView(v)
				m.mode = modeList
				break
			}
		}
		return m, nil
	}
	switch id {
	case "act:comment":
		if m.inDocsSurface() {
			return m, m.docsUnsupported("comment")
		}
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startComment())
	case "act:transition":
		if m.inDocsSurface() {
			return m, m.docsUnsupported("transition")
		}
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startTransition())
	case "act:assignee":
		if m.inDocsSurface() {
			return m, m.docsUnsupported("assignee")
		}
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startAssignee())
	case "act:edit":
		if m.inDocsSurface() {
			return m, m.docsUnsupported("edit")
		}
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startFieldEdit())
	case "act:watch":
		if m.inDocsSurface() {
			return m, m.docsUnsupported("watch")
		}
		return m, m.toggleWatchSelected()
	case "act:feed":
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.loadFeedCmd())
	case "act:docs":
		if m.mode == modeDocs || m.mode == modeDocDetail {
			m.leaveDocs()
		} else {
			m.enterDocs()
		}
		return m, nil
	case "act:views":
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.loadViewsCmd())
	case "act:refresh":
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.reloadCmd())
	case "act:help":
		m.showHelp = true
		return m, nil
	}
	return m, nil
}

// viewPalette renders the centered palette box. It replaces the frame while
// open (lipgloss.Place fills the rest), which reads as a modal.
func (m Model) viewPalette() string {
	p := m.pal
	var b strings.Builder
	title := "⌘ command palette"
	if m.animOn {
		b.WriteString(shimmer(title, m.animPhase))
	} else {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colPink).Render(title))
	}
	b.WriteString("\n")
	b.WriteString(styleFilter.Render("❯ ") + stylePrimary.Render(p.query) + styleFilter.Render("▌"))
	b.WriteString("\n\n")
	if len(p.filtered) == 0 {
		b.WriteString(styleMuted.Render("  no matches"))
		b.WriteString("\n")
	}
	for i, it := range p.filtered {
		if i >= paletteMaxRows {
			b.WriteString(styleMuted.Render("  …"))
			b.WriteString("\n")
			break
		}
		if i == p.cursor {
			row := "▸ " + it.label
			sel := lipgloss.NewStyle().Bold(true).Foreground(colSelFg).Background(colAccentBg)
			if m.animOn {
				sel = sel.Background(glowColor(m.animPhase))
			}
			b.WriteString(sel.Render(row))
			if it.hint != "" {
				b.WriteString(" " + styleMuted.Render(it.hint))
			}
		} else {
			b.WriteString(styleMuted.Render("  " + it.label))
			if it.hint != "" {
				b.WriteString("  " + styleMuted.Faint(true).Render(it.hint))
			}
		}
		b.WriteString("\n")
	}
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(1, 2).
		Width(min(64, max(30, m.width-8)))
	if m.animOn {
		border = border.BorderForeground(pulseColor(m.animPhase))
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, border.Render(b.String()))
}
