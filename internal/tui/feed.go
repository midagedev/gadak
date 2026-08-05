package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/midagedev/scry/internal/store"
)

type feedLoadedMsg struct {
	items  []store.FeedItem
	counts store.FeedUnreadCounts
	err    error
}

type feedMarkedMsg struct {
	counts store.FeedUnreadCounts
	err    error
}

func (m Model) feedMe() store.FeedIdentity {
	if m.cfg == nil {
		return store.FeedIdentity{}
	}
	return store.FeedIdentity{
		AccountID:   m.cfg.AccountID,
		Email:       m.cfg.Email,
		DisplayName: m.cfg.TokenOwner,
	}
}

func (m Model) effectiveFeedFocus() store.FeedFocus {
	if m.feedFocus == "" {
		return store.FeedFocusAll
	}
	return m.feedFocus
}

func (m Model) loadFeedCmd() tea.Cmd {
	db := m.db
	me := m.feedMe()
	focus := m.effectiveFeedFocus()
	return func() tea.Msg {
		if db == nil {
			return feedLoadedMsg{err: fmt.Errorf("no database")}
		}
		res, err := db.Feed(store.FeedOpts{Me: me, Focus: focus})
		if err != nil {
			return feedLoadedMsg{err: err}
		}
		return feedLoadedMsg{items: res.Items, counts: res.UnreadCounts}
	}
}

func (m Model) markFeedAllReadCmd() tea.Cmd {
	db := m.db
	me := m.feedMe()
	return func() tea.Msg {
		if db == nil {
			return feedMarkedMsg{err: fmt.Errorf("no database")}
		}
		res, err := db.MarkFeedRead(store.MarkFeedReadOpts{All: true, Me: me})
		if err != nil {
			return feedMarkedMsg{err: err}
		}
		return feedMarkedMsg{counts: res.UnreadCounts}
	}
}

// setFeedFocus switches the personal-feed focus tab and reloads.
func (m Model) setFeedFocus(f store.FeedFocus) (tea.Model, tea.Cmd) {
	if m.effectiveFeedFocus() == f {
		return m, nil
	}
	m.feedFocus = f
	m.feedCursor = 0
	m.feedOffset = 0
	m.loading = true
	return m, tea.Batch(m.spin.Tick, m.loadFeedCmd())
}

func (m Model) handleFeedKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.showHelp = !m.showHelp
		return m, nil
	case key.Matches(msg, k.Feed), key.Matches(msg, k.Back), msg.String() == "esc":
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		m.mode = modeList
		m.feedCursor = 0
		m.feedOffset = 0
		return m, nil
	case key.Matches(msg, k.TabAll):
		return m.setFeedFocus(store.FeedFocusAll)
	case key.Matches(msg, k.TabOpen):
		return m.setFeedFocus(store.FeedFocusAssignee)
	case key.Matches(msg, k.TabInProgress):
		return m.setFeedFocus(store.FeedFocusReporter)
	case key.Matches(msg, k.TabDone):
		return m.setFeedFocus(store.FeedFocusMention)
	case key.Matches(msg, k.Refresh):
		// In feed mode, r marks all events read then reloads the feed list.
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.markFeedAllReadCmd())
	case key.Matches(msg, k.Down):
		m.moveFeedCursor(1)
		return m, nil
	case key.Matches(msg, k.Up):
		m.moveFeedCursor(-1)
		return m, nil
	case key.Matches(msg, k.Top):
		m.feedCursor = 0
		m.ensureFeedVisible()
		return m, nil
	case key.Matches(msg, k.Bottom):
		if n := len(m.feedItems); n > 0 {
			m.feedCursor = n - 1
			m.ensureFeedVisible()
		}
		return m, nil
	case key.Matches(msg, k.Enter):
		return m.openFeedSelection()
	}
	return m, nil
}

// openFeedSelection opens the detail for the feed row under the cursor.
// Shared by Enter and a mouse click on the selected row.
func (m Model) openFeedSelection() (tea.Model, tea.Cmd) {
	if len(m.feedItems) == 0 || m.feedCursor < 0 || m.feedCursor >= len(m.feedItems) {
		return m, nil
	}
	item := m.feedItems[m.feedCursor]
	if item.IssueKey == "" {
		return m, nil
	}
	lite := store.IssueLite{
		IssueKey: item.IssueKey,
		Summary:  item.Summary,
		Status:   item.CurrentStatus,
	}
	// Prefer the list row if we already have it (fuller fields).
	for _, r := range m.all {
		if r.lite.IssueKey == item.IssueKey {
			lite = r.lite
			break
		}
	}
	m.detailFrom = modeFeed
	return m, m.loadDetailCmd(item.IssueKey, lite)
}

func (m *Model) moveFeedCursor(delta int) {
	n := len(m.feedItems)
	if n == 0 {
		m.feedCursor = 0
		return
	}
	m.feedCursor += delta
	if m.feedCursor < 0 {
		m.feedCursor = 0
	}
	if m.feedCursor >= n {
		m.feedCursor = n - 1
	}
	m.ensureFeedVisible()
}

func (m *Model) ensureFeedVisible() {
	h := m.listHeight()
	if m.feedCursor < m.feedOffset {
		m.feedOffset = m.feedCursor
	}
	if m.feedCursor >= m.feedOffset+h {
		m.feedOffset = m.feedCursor - h + 1
	}
	if m.feedOffset < 0 {
		m.feedOffset = 0
	}
}

func (m Model) viewFeed() string {
	var b strings.Builder
	w := m.width

	b.WriteString(m.renderHeader(w))
	b.WriteByte('\n')
	b.WriteString(fitWidth(m.renderFeedTabs(), w))
	b.WriteByte('\n')

	listH := m.listHeight()
	if len(m.feedItems) == 0 {
		empty := styleMuted.Render("  no feed events — press F to leave · r to mark read")
		for i := 0; i < listH; i++ {
			if i == 0 {
				b.WriteString(fitWidth(empty, w))
			}
			b.WriteByte('\n')
		}
	} else {
		end := m.feedOffset + listH
		if end > len(m.feedItems) {
			end = len(m.feedItems)
		}
		for i := m.feedOffset; i < end; i++ {
			b.WriteString(m.renderFeedRow(m.feedItems[i], i == m.feedCursor, w))
			b.WriteByte('\n')
		}
		for i := end - m.feedOffset; i < listH; i++ {
			b.WriteByte('\n')
		}
	}

	b.WriteString(m.renderFeedStatusBar(w))
	if m.showHelp {
		return m.overlayHelp(b.String())
	}
	return b.String()
}

// renderFeedTabs draws focus tabs with unread badges (0 omitted).
// Reuses list tab styles; labels: all / assignee / reporter / mention.
func (m Model) renderFeedTabs() string {
	cur := m.effectiveFeedFocus()
	tabs := []struct {
		focus store.FeedFocus
		label string
		n     int
	}{
		{store.FeedFocusAll, "all", m.feedCounts.All},
		{store.FeedFocusAssignee, "assignee", m.feedCounts.Assignee},
		{store.FeedFocusReporter, "reporter", m.feedCounts.Reporter},
		{store.FeedFocusMention, "mention", m.feedCounts.Mention},
	}
	parts := make([]string, 0, 4)
	for i, t := range tabs {
		label := fmt.Sprintf("%d %s", i+1, t.label)
		if t.n > 0 {
			label += fmt.Sprintf(" %d", t.n)
		}
		if t.focus == cur {
			parts = append(parts, styleTabActive.Render(label))
		} else {
			parts = append(parts, styleTabInactive.Render(label))
		}
	}
	return " " + strings.Join(parts, " ")
}

func (m Model) renderFeedRow(item store.FeedItem, selected bool, w int) string {
	age := ""
	if item.OccurredAt != nil {
		age = relativeTime(*item.OccurredAt, m.clock())
	}
	etype := item.EventType
	if etype == "" {
		etype = "?"
	}
	unreadMark := " "
	if item.ReadAt == nil || *item.ReadAt == "" {
		unreadMark = "●"
	}

	const (
		ageW  = 9
		typeW = 16
		keyW  = 12
		actW  = 14
	)

	if w < 40 {
		// Narrow: unread + key + type
		line := fmt.Sprintf(" %s %s %s",
			unreadMark,
			padRight(item.IssueKey, min(keyW, max(4, w/3))),
			truncate(etype, max(4, w-keyW-4)),
		)
		if selected {
			return styleSel.Width(w).MaxWidth(w).Render(styleSel.Render(line))
		}
		return fitWidth(stylePrimary.Render(line), w)
	}

	agePlain := padRight(age, ageW)
	typePlain := padRight(truncate(etype, typeW), typeW)
	keyPlain := padRight(item.IssueKey, keyW)
	actPlain := padRight(truncate(item.ActorName, actW), actW)
	sumW := w - (2 + 1 + ageW + 1 + typeW + 1 + keyW + 1 + actW + 2)
	if sumW < 6 {
		sumW = 6
	}
	sumPlain := truncate(item.Summary, sumW)

	if selected {
		inner := " " + styleSel.Render(unreadMark) + " " +
			styleSelMuted.Render(agePlain) + " " +
			styleSel.Render(typePlain) + " " +
			styleSelKey.Render(keyPlain) + " " +
			styleSelMuted.Render(actPlain) + " " +
			styleSel.Render(sumPlain)
		return styleSel.Width(w).MaxWidth(w).Render(inner)
	}
	line := " " + styleAccentDot(unreadMark) + " " +
		styleMuted.Render(agePlain) + " " +
		styleDim.Render(typePlain) + " " +
		styleKey.Render(keyPlain) + " " +
		styleDim.Render(actPlain) + " " +
		stylePrimary.Render(sumPlain)
	return fitWidth(line, w)
}

func styleAccentDot(s string) string {
	if s == "●" {
		return styleBrand.Render(s)
	}
	return s
}

func (m Model) renderFeedStatusBar(w int) string {
	leftParts := make([]string, 0, 4)
	if m.loading || m.busy {
		leftParts = append(leftParts, m.spin.View())
	}
	leftParts = append(leftParts, styleHelp.Render("j/k · 1-4 · enter · r mark-read · F/esc back · ? · q"))
	if m.feedCounts.All > 0 {
		leftParts = append(leftParts, styleChip.Render(fmt.Sprintf("%d unread", m.feedCounts.All)))
	}
	if m.toast != "" && m.clock().Sub(m.toastAt) < 8*time.Second {
		if m.toastErr {
			leftParts = append(leftParts, styleToastErr.Render(m.toast))
		} else {
			leftParts = append(leftParts, styleToastOK.Render(m.toast))
		}
	}
	left := strings.Join(leftParts, "  ")
	count := fmt.Sprintf("%d/%d", 0, len(m.feedItems))
	if len(m.feedItems) > 0 {
		count = fmt.Sprintf("%d/%d", m.feedCursor+1, len(m.feedItems))
	}
	right := styleMuted.Render(count)
	return joinBar(left, right, w)
}
