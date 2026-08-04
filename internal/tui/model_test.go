package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

// feed applies a message and returns the updated model (discards cmds).
func feed(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func feedKey(m Model, key string) Model {
	return feed(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func feedSpecial(m Model, t tea.KeyType) Model {
	return feed(m, tea.KeyMsg{Type: t})
}

func seededModel() Model {
	m := newModel(&config.Config{}, nil)
	m.width, m.height = 120, 40
	m.now = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	m.all = sampleRows()
	m.refilter()
	return m
}

func TestTabKeys(t *testing.T) {
	m := seededModel()
	if len(m.visible) != 4 {
		t.Fatalf("start all: %d", len(m.visible))
	}

	m = feedKey(m, "3") // in progress
	if m.tab != TabInProgress || len(m.visible) != 1 {
		t.Fatalf("tab 3: tab=%v n=%d", m.tab, len(m.visible))
	}
	if m.all[m.visible[0]].lite.IssueKey != "AAA-2" {
		t.Fatalf("expected AAA-2, got %s", m.all[m.visible[0]].lite.IssueKey)
	}

	m = feedKey(m, "4") // done
	if m.tab != TabDone || len(m.visible) != 1 {
		t.Fatalf("tab 4: tab=%v n=%d", m.tab, len(m.visible))
	}

	m = feedKey(m, "2") // open
	if m.tab != TabOpen || len(m.visible) != 3 {
		t.Fatalf("tab 2: tab=%v n=%d", m.tab, len(m.visible))
	}

	m = feedKey(m, "1") // all
	if m.tab != TabAll || len(m.visible) != 4 {
		t.Fatalf("tab 1: tab=%v n=%d", m.tab, len(m.visible))
	}
}

func TestFilterKeystroke(t *testing.T) {
	m := seededModel()
	m = feedKey(m, "/")
	if m.mode != modeFilter {
		t.Fatalf("expected filter mode, got %v", m.mode)
	}
	// type "bob"
	for _, ch := range "bob" {
		m = feed(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if m.filter != "bob" {
		t.Fatalf("filter=%q", m.filter)
	}
	if len(m.visible) != 1 || m.all[m.visible[0]].lite.IssueKey != "AAA-2" {
		t.Fatalf("filtered keys: %v", visibleKeys(m))
	}
	m = feedSpecial(m, tea.KeyEnter)
	if m.mode != modeList {
		t.Fatalf("enter should leave filter mode")
	}
	// esc clears filter from list mode
	m = feedSpecial(m, tea.KeyEscape)
	if m.filter != "" || len(m.visible) != 4 {
		t.Fatalf("esc clear: filter=%q n=%d", m.filter, len(m.visible))
	}
}

func TestCursorKeys(t *testing.T) {
	m := seededModel()
	if m.cursor != 0 {
		t.Fatalf("start cursor %d", m.cursor)
	}
	m = feedKey(m, "j")
	if m.cursor != 1 {
		t.Fatalf("j → %d", m.cursor)
	}
	m = feedKey(m, "k")
	if m.cursor != 0 {
		t.Fatalf("k → %d", m.cursor)
	}
	m = feedKey(m, "G")
	if m.cursor != len(m.visible)-1 {
		t.Fatalf("G → %d want %d", m.cursor, len(m.visible)-1)
	}
	m = feedKey(m, "g")
	if m.cursor != 0 {
		t.Fatalf("g → %d", m.cursor)
	}
}

func TestWriteKeysWithoutCredential(t *testing.T) {
	m := seededModel()
	// c / t / a should toast, not panic, when no credential
	for _, key := range []string{"c", "t", "a"} {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = next.(Model)
		if cmd == nil {
			t.Fatalf("%s: expected toast cmd", key)
		}
		toast, ok := findToast(cmd)
		if !ok {
			t.Fatalf("%s: no toastMsg in cmd tree", key)
		}
		if !toast.err || !strings.Contains(toast.text, "credential") {
			t.Fatalf("%s toast: %+v", key, toast)
		}
	}
}

// findToast walks a tea.Cmd (including BatchMsg trees) for the first toastMsg.
func findToast(cmd tea.Cmd) (toastMsg, bool) {
	if cmd == nil {
		return toastMsg{}, false
	}
	msg := cmd()
	switch m := msg.(type) {
	case toastMsg:
		return m, true
	case tea.BatchMsg:
		for _, c := range m {
			if t, ok := findToast(c); ok {
				return t, true
			}
		}
	}
	return toastMsg{}, false
}

func TestRenderSmoke(t *testing.T) {
	m := seededModel()
	m = feed(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	view := m.View()
	if view == "" {
		t.Fatal("empty view")
	}
	// Strip ANSI so assertions stay stable under style changes.
	plain := stripANSI(view)
	for _, want := range []string{"scry", "AAA-1", "all", "open"} {
		if !strings.Contains(plain, want) {
			t.Errorf("view missing %q\n%s", want, plain)
		}
	}

	// Detail path without a real DB: inject detail and render.
	m.detailKey = "AAA-1"
	lite := m.all[0].lite
	m.detailLite = &lite
	m.detail = &store.Detail{
		IssueKey: "AAA-1",
		Comments: []store.DetailComment{
			{Author: "Ada", Body: "Looks good", CreatedAt: "2026-08-03T10:00:00Z"},
		},
		History: []store.DetailChange{
			{At: "2026-08-02T10:00:00Z", Author: "Bob", Field: "status", FromValue: "Backlog", ToValue: "Done"},
		},
	}
	m.mode = modeDetail
	dview := stripANSI(m.View())
	for _, want := range []string{"AAA-1", "Description", "Comments", "History", "Ada", "status"} {
		if !strings.Contains(dview, want) {
			t.Errorf("detail view missing %q\n%s", want, dview)
		}
	}

	// Esc returns to list
	m = feedSpecial(m, tea.KeyEscape)
	if m.mode != modeList {
		t.Fatalf("esc → mode %v", m.mode)
	}
}

// stripANSI removes CSI sequences so View() smoke tests can match plain text
// even when lipgloss wraps every glyph in colour codes.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			// CSI: ESC [ ... final byte in @-~
			if i+1 < len(s) && s[i+1] == '[' {
				i += 2
				for i < len(s) && (s[i] < '@' || s[i] > '~') {
					i++
				}
				continue
			}
			// OSC / other: skip until BEL or ST
			i++
			for i < len(s) && s[i] != '\x07' && s[i] != '\x1b' {
				i++
			}
			if i < len(s) && s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func visibleKeys(m Model) []string {
	out := make([]string, len(m.visible))
	for i, idx := range m.visible {
		out[i] = m.all[idx].lite.IssueKey
	}
	return out
}
