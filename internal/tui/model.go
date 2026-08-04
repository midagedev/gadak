package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
)

// mode is the top-level UI state.
type mode int

const (
	modeList mode = iota
	modeFilter
	modeDetail
	modeForm
)

// Model is the bubbletea model for the issue navigator.
type Model struct {
	cfg *config.Config
	db  *store.DB

	all     []row
	visible []int // indices into all
	cursor  int
	offset  int // scroll top for the list viewport

	filter    string
	tab       Tab
	mode      mode
	detail    *store.Detail
	detailKey string
	// detailLite is the list row shown in the detail header (status, assignee…).
	detailLite *store.IssueLite

	form       *huh.Form
	formSubmit func() tea.Cmd
	formTitle  string

	width, height int
	toast         string
	toastErr      bool
	toastAt       time.Time
	loadErr       error
	syncedLabel   string
	now           time.Time // injectable for tests
	keys          keyMap
}

// newModel builds a model without loading data (tests inject rows).
func newModel(cfg *config.Config, db *store.DB) Model {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return Model{
		cfg:  cfg,
		db:   db,
		tab:  TabAll,
		mode: modeList,
		now:  time.Now(),
		keys: defaultKeys(),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return m.reloadCmd()
}

type loadedMsg struct {
	rows        []row
	syncedLabel string
	err         error
}

type detailMsg struct {
	key  string
	lite store.IssueLite
	d    *store.Detail
	err  error
}

func (m Model) reloadCmd() tea.Cmd {
	db := m.db
	return func() tea.Msg {
		if db == nil {
			return loadedMsg{err: fmt.Errorf("no database")}
		}
		lites, err := db.IssueLites()
		if err != nil {
			return loadedMsg{err: err}
		}
		label := "never synced"
		if st, err := db.SyncState("jira"); err == nil {
			if st.SyncedAt != nil && *st.SyncedAt != "" {
				label = "synced " + relativeTime(*st.SyncedAt, time.Now())
			} else if st.Watermark != "" {
				label = "watermark " + relativeTime(st.Watermark, time.Now())
			}
		}
		return loadedMsg{rows: buildRows(lites), syncedLabel: label}
	}
}

func (m Model) loadDetailCmd(key string, lite store.IssueLite) tea.Cmd {
	db := m.db
	return func() tea.Msg {
		if db == nil {
			return detailMsg{key: key, lite: lite, err: fmt.Errorf("no database")}
		}
		d, err := db.Detail(key)
		return detailMsg{key: key, lite: lite, d: d, err: err}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loadedMsg:
		m.loadErr = msg.err
		if msg.err == nil {
			m.all = msg.rows
			m.syncedLabel = msg.syncedLabel
			m.refilter()
		}
		return m, nil

	case detailMsg:
		if msg.err != nil {
			m.toast = msg.err.Error()
			m.toastErr = true
			m.toastAt = m.clock()
			m.mode = modeList
			return m, nil
		}
		m.detail = msg.d
		m.detailKey = msg.key
		lite := msg.lite
		m.detailLite = &lite
		m.mode = modeDetail
		return m, nil

	case openFormMsg:
		m.form = msg.form.WithWidth(min(m.width-4, 72)).WithHeight(min(m.height-6, 16))
		m.formSubmit = msg.submit
		m.formTitle = msg.title
		m.mode = modeForm
		return m, m.form.Init()

	case formResultMsg:
		m.mode = modeList
		m.form = nil
		m.formSubmit = nil
		if msg.err != nil {
			m.toast = msg.err.Error()
			m.toastErr = true
		} else if msg.note != "" {
			m.toast = msg.note
			m.toastErr = false
		}
		m.toastAt = m.clock()
		if msg.key != "" {
			return m, m.reloadCmd()
		}
		return m, nil

	case toastMsg:
		m.toast = msg.text
		m.toastErr = msg.err
		m.toastAt = m.clock()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward other messages to an active huh form (e.g. blur events).
	if m.mode == modeForm && m.form != nil {
		return m.updateForm(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeForm && m.form != nil {
		return m.updateForm(msg)
	}

	key := msg.String()

	// Global quit always works outside forms (form uses ctrl+c via huh).
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case modeFilter:
		return m.handleFilterKey(msg)
	case modeDetail:
		return m.handleDetailKey(msg)
	default:
		return m.handleListKey(msg)
	}
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Refresh):
		return m, m.reloadCmd()
	case key.Matches(msg, k.Filter):
		m.mode = modeFilter
		return m, nil
	case key.Matches(msg, k.ClearFilter):
		if m.filter != "" {
			m.filter = ""
			m.refilter()
		}
		return m, nil
	case key.Matches(msg, k.Down):
		m.moveCursor(1)
		return m, nil
	case key.Matches(msg, k.Up):
		m.moveCursor(-1)
		return m, nil
	case key.Matches(msg, k.Top):
		m.cursor = 0
		m.ensureVisible()
		return m, nil
	case key.Matches(msg, k.Bottom):
		if n := len(m.visible); n > 0 {
			m.cursor = n - 1
			m.ensureVisible()
		}
		return m, nil
	case msg.String() == "pgdown" || msg.String() == "ctrl+d":
		m.moveCursor(m.pageSize())
		return m, nil
	case msg.String() == "pgup" || msg.String() == "ctrl+u":
		m.moveCursor(-m.pageSize())
		return m, nil
	case key.Matches(msg, k.TabAll):
		m.setTab(TabAll)
		return m, nil
	case key.Matches(msg, k.TabOpen):
		m.setTab(TabOpen)
		return m, nil
	case key.Matches(msg, k.TabInProgress):
		m.setTab(TabInProgress)
		return m, nil
	case key.Matches(msg, k.TabDone):
		m.setTab(TabDone)
		return m, nil
	case key.Matches(msg, k.Enter):
		issueKey, ok := m.selectedKey()
		if !ok {
			return m, nil
		}
		lite := m.all[m.visible[m.cursor]].lite
		return m, m.loadDetailCmd(issueKey, lite)
	case key.Matches(msg, k.Comment):
		return m, m.startComment()
	case key.Matches(msg, k.Transition):
		return m, m.startTransition()
	case key.Matches(msg, k.Assignee):
		return m, m.startAssignee()
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.mode = modeList
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if m.filter != "" {
			r := []rune(m.filter)
			m.filter = string(r[:len(r)-1])
			m.refilter()
		}
		return m, nil
	case "ctrl+u":
		m.filter = ""
		m.refilter()
		return m, nil
	default:
		// Printable runes only — ignore chords.
		if msg.Type == tea.KeyRunes {
			m.filter += string(msg.Runes)
			m.refilter()
		}
		return m, nil
	}
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Back), msg.String() == "esc":
		m.mode = modeList
		m.detail = nil
		m.detailKey = ""
		m.detailLite = nil
		return m, nil
	case key.Matches(msg, k.Comment):
		return m, m.startComment()
	case key.Matches(msg, k.Transition):
		return m, m.startTransition()
	case key.Matches(msg, k.Assignee):
		return m, m.startAssignee()
	case key.Matches(msg, k.Refresh):
		if m.detailKey != "" && m.detailLite != nil {
			return m, m.loadDetailCmd(m.detailKey, *m.detailLite)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}
	switch m.form.State {
	case huh.StateCompleted:
		submit := m.formSubmit
		m.form = nil
		m.formSubmit = nil
		m.mode = modeList
		if submit != nil {
			return m, submit()
		}
		return m, nil
	case huh.StateAborted:
		m.form = nil
		m.formSubmit = nil
		m.mode = modeList
		m.toast = "cancelled"
		m.toastErr = false
		m.toastAt = m.clock()
		return m, nil
	}
	return m, cmd
}

func (m *Model) setTab(t Tab) {
	if m.tab == t {
		return
	}
	m.tab = t
	m.refilter()
}

func (m *Model) refilter() {
	needle := strings.ToLower(strings.TrimSpace(m.filter))
	m.visible = applyFilter(m.all, m.tab, needle)
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
	m.ensureVisible()
}

func (m *Model) moveCursor(delta int) {
	n := len(m.visible)
	if n == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	m.ensureVisible()
}

func (m *Model) pageSize() int {
	h := m.listHeight()
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) listHeight() int {
	// header(1) + tabs(1) + status(1) + help(1) ≈ 4 chrome lines
	h := m.height - 4
	if h < 3 {
		h = 3
	}
	return h
}

func (m *Model) ensureVisible() {
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) clock() time.Time {
	if !m.now.IsZero() {
		return m.now
	}
	return time.Now()
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		m.width = 80
	}
	if m.height == 0 {
		m.height = 24
	}

	if m.mode == modeForm && m.form != nil {
		return m.viewForm()
	}
	if m.mode == modeDetail {
		return m.viewDetail()
	}
	return m.viewList()
}

func (m Model) viewList() string {
	var b strings.Builder
	w := m.width

	// Header
	title := styleHeader.Render("scry · issues")
	sync := styleMuted.Render(m.syncedLabel)
	right := sync
	if m.loadErr != nil {
		right = styleToastErr.Render(m.loadErr.Error())
	}
	gap := w - lipgloss.Width(title) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
	}
	b.WriteString(fitWidth(title+spaces(gap)+right, w))
	b.WriteByte('\n')

	// Tabs
	b.WriteString(fitWidth(m.renderTabs(), w))
	b.WriteByte('\n')

	// List body
	listH := m.listHeight()
	if len(m.visible) == 0 {
		empty := "  no issues"
		if m.filter != "" || m.tab != TabAll {
			empty = "  no matches"
		}
		for i := 0; i < listH; i++ {
			if i == 0 {
				b.WriteString(fitWidth(styleMuted.Render(empty), w))
			}
			b.WriteByte('\n')
		}
	} else {
		end := m.offset + listH
		if end > len(m.visible) {
			end = len(m.visible)
		}
		for i := m.offset; i < end; i++ {
			r := m.all[m.visible[i]]
			line := m.renderRow(r, i == m.cursor, w)
			b.WriteString(line)
			b.WriteByte('\n')
		}
		// Pad remaining viewport lines.
		for i := end - m.offset; i < listH; i++ {
			b.WriteByte('\n')
		}
	}

	// Status / filter / toast
	b.WriteString(fitWidth(m.renderStatus(), w))
	b.WriteByte('\n')
	b.WriteString(fitWidth(m.renderHelp(), w))
	return b.String()
}

func (m Model) renderTabs() string {
	tabs := []struct {
		t Tab
		n int
	}{
		{TabAll, 0},
		{TabOpen, 0},
		{TabInProgress, 0},
		{TabDone, 0},
	}
	// Counts ignore the text filter so the tab bar stays stable while typing.
	for _, r := range m.all {
		for i := range tabs {
			if matchTab(tabs[i].t, r.lite.StatusCategory) {
				tabs[i].n++
			}
		}
	}
	parts := make([]string, 0, 4)
	for i, t := range tabs {
		label := fmt.Sprintf("%d:%s(%d)", i+1, t.t.Label(), t.n)
		if t.t == m.tab {
			parts = append(parts, styleHeader.Render("["+label+"]"))
		} else {
			parts = append(parts, styleDim.Render(label))
		}
	}
	return strings.Join(parts, "  ")
}

func (m Model) renderRow(r row, selected bool, w int) string {
	key := padRight(r.lite.IssueKey, 12)
	status := padRight(truncate(r.lite.Status, 12), 12)
	assignee := padRight(truncate(deref(r.lite.Assignee), 14), 14)
	age := relativeTime(deref(r.lite.UpdatedAt), m.clock())
	age = padRight(age, 10)

	// key(12) + sp + status(12) + sp + assignee(14) + sp + age(10) + sp = 52
	const fixed = 12 + 1 + 12 + 1 + 14 + 1 + 10
	sumW := w - fixed - 1
	if sumW < 8 {
		sumW = 8
	}
	summary := truncate(r.lite.Summary, sumW)

	statusCol := statusStyle(r.lite.StatusCategory).Render(status)
	line := styleKey.Render(key) + " " + statusCol + " " + summary + " " +
		styleDim.Render(assignee) + " " + styleMuted.Render(age)

	if selected {
		// Rebuild without nested styles so the selection background is solid.
		plain := padRight(key, 12) + " " + padRight(truncate(r.lite.Status, 12), 12) + " " +
			padRight(truncate(r.lite.Summary, sumW), sumW) + " " +
			padRight(truncate(deref(r.lite.Assignee), 14), 14) + " " +
			padRight(age, 10)
		return styleSel.Render(fitWidth(plain, w))
	}
	return fitWidth(line, w)
}

func (m Model) renderStatus() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("%d/%d", m.cursor+1, len(m.visible)))
	if m.filter != "" || m.mode == modeFilter {
		f := m.filter
		if m.mode == modeFilter {
			f += "▌"
		}
		parts = append(parts, styleFilter.Render("/"+f))
	}
	if m.toast != "" && m.clock().Sub(m.toastAt) < 8*time.Second {
		if m.toastErr {
			parts = append(parts, styleToastErr.Render(m.toast))
		} else {
			parts = append(parts, styleToastOK.Render(m.toast))
		}
	}
	if m.cfg != nil && !m.cfg.HasCredential() {
		parts = append(parts, styleMuted.Render("read-only (no credential)"))
	}
	return strings.Join(parts, "  ·  ")
}

func (m Model) renderHelp() string {
	return styleHelp.Render("j/k move  / filter  1-4 tabs  enter detail  c comment  t transition  a assignee  r refresh  q quit")
}

func (m Model) viewForm() string {
	var b strings.Builder
	title := styleHeader.Render(m.formTitle)
	b.WriteString(fitWidth(title, m.width))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	return b.String()
}

func (m Model) viewDetail() string {
	var b strings.Builder
	w := m.width
	key := m.detailKey
	lite := m.detailLite

	// Prefer split view when the terminal is wide enough.
	split := w >= 100 && m.mode == modeDetail
	_ = split // full-screen detail for v1; split reserved for later

	header := styleKey.Render(key)
	if lite != nil {
		header += "  " + statusStyle(lite.StatusCategory).Render(lite.Status)
		header += "  " + styleHeader.Render(lite.Summary)
	}
	b.WriteString(fitWidth(header, w))
	b.WriteByte('\n')

	if lite != nil {
		meta := fmt.Sprintf("assignee %s  ·  labels %s  ·  updated %s",
			orDash(deref(lite.Assignee)),
			orDash(strings.Join(lite.Labels, ", ")),
			orDash(relativeTime(deref(lite.UpdatedAt), m.clock())),
		)
		b.WriteString(fitWidth(styleDim.Render(meta), w))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')

	if m.detail == nil {
		b.WriteString(styleMuted.Render("  loading…"))
		return b.String()
	}

	// Description (ADF → plain text)
	body := jira.PlainText(m.detail.DescriptionADF)
	b.WriteString(styleHeader.Render("Description"))
	b.WriteByte('\n')
	if body == "" {
		b.WriteString(styleMuted.Render("  (empty)"))
	} else {
		for _, line := range wrapLines(body, w-2, 12) {
			b.WriteString("  ")
			b.WriteString(fitWidth(line, w-2))
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')

	// Recent comments (last 5)
	b.WriteString(styleHeader.Render("Comments"))
	b.WriteByte('\n')
	comments := m.detail.Comments
	if len(comments) == 0 {
		b.WriteString(styleMuted.Render("  (none)"))
		b.WriteByte('\n')
	} else {
		start := 0
		if len(comments) > 5 {
			start = len(comments) - 5
		}
		for _, c := range comments[start:] {
			who := c.Author
			if who == "" {
				who = "?"
			}
			when := relativeTime(c.CreatedAt, m.clock())
			b.WriteString(styleDim.Render(fmt.Sprintf("  %s · %s", who, when)))
			b.WriteByte('\n')
			text := c.Body
			if text == "" {
				text = jira.PlainText(c.BodyADF)
			}
			for _, line := range wrapLines(text, w-4, 4) {
				b.WriteString("    ")
				b.WriteString(fitWidth(line, w-4))
				b.WriteByte('\n')
			}
		}
	}
	b.WriteByte('\n')

	// Recent history (last 5)
	b.WriteString(styleHeader.Render("History"))
	b.WriteByte('\n')
	hist := m.detail.History
	if len(hist) == 0 {
		b.WriteString(styleMuted.Render("  (none)"))
		b.WriteByte('\n')
	} else {
		start := 0
		if len(hist) > 5 {
			start = len(hist) - 5
		}
		for _, h := range hist[start:] {
			line := fmt.Sprintf("  %s  %s: %s → %s  (%s)",
				relativeTime(h.At, m.clock()),
				h.Field,
				orDash(h.FromValue),
				orDash(h.ToValue),
				orDash(h.Author),
			)
			b.WriteString(fitWidth(styleDim.Render(line), w))
			b.WriteByte('\n')
		}
	}

	b.WriteByte('\n')
	b.WriteString(fitWidth(styleHelp.Render("esc back  c comment  t transition  a assignee  r refresh"), w))
	if m.toast != "" && m.clock().Sub(m.toastAt) < 8*time.Second {
		b.WriteByte('\n')
		if m.toastErr {
			b.WriteString(styleToastErr.Render(m.toast))
		} else {
			b.WriteString(styleToastOK.Render(m.toast))
		}
	}
	return b.String()
}

func wrapLines(s string, width, maxLines int) []string {
	if width < 8 {
		width = 8
	}
	raw := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	var out []string
	for _, line := range raw {
		line = strings.TrimRight(line, " ")
		if line == "" {
			out = append(out, "")
			if maxLines > 0 && len(out) >= maxLines {
				break
			}
			continue
		}
		r := []rune(line)
		for len(r) > 0 {
			n := width
			if n > len(r) {
				n = len(r)
			}
			out = append(out, string(r[:n]))
			r = r[n:]
			if maxLines > 0 && len(out) >= maxLines {
				return out
			}
		}
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
