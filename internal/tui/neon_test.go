package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestHighlightMatch(t *testing.T) {
	base := lipgloss.NewStyle()
	match := lipgloss.NewStyle().Bold(true)

	// No needle: plain passthrough via base.
	if got := highlightMatch("hello", "", base, match); got != "hello" {
		t.Fatalf("empty needle: %q", got)
	}
	// Highlighting never changes the visible text, only the styling.
	for _, tc := range []struct{ plain, needle string }{
		{"Payment retries fail", "retri"},
		{"로그인이 간헐적으로 실패합니다", "로그인"},
		{"aaa AAA aAa", "aa"},
		{"no match here", "zzz"},
	} {
		got := highlightMatch(tc.plain, tc.needle, base, match)
		stripped := stripANSI(got)
		if stripped != tc.plain {
			t.Fatalf("highlight(%q,%q) altered text: %q", tc.plain, tc.needle, stripped)
		}
	}
}

func TestShimmerKeepsText(t *testing.T) {
	if got := stripANSI(shimmer("scry", 0.37)); got != "scry" {
		t.Fatalf("shimmer altered text: %q", got)
	}
}

func TestAnimTickAdvancesPhase(t *testing.T) {
	m := seededModel()
	m.animOn = true
	res, cmd := m.Update(animTickMsg{})
	m2 := res.(Model)
	if m2.animPhase <= m.animPhase {
		t.Fatal("phase did not advance")
	}
	if cmd == nil {
		t.Fatal("tick did not re-arm")
	}
	// Disabled: no re-arm, no drift.
	m.animOn = false
	_, cmd = m.Update(animTickMsg{})
	if cmd != nil {
		t.Fatal("disabled tick re-armed")
	}
}

func TestPaletteFilterAndJump(t *testing.T) {
	m := seededModel()
	m.openPalette()
	if m.pal == nil || len(m.pal.filtered) == 0 {
		t.Fatal("palette opened empty")
	}
	// Typing narrows to an issue jump entry.
	res, _ := m.handlePaletteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("aaa-2")})
	m = res.(Model)
	found := false
	for _, it := range m.pal.filtered {
		if it.id == "issue:AAA-2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no issue hit for aaa-2: %+v", m.pal.filtered)
	}
	// Esc closes.
	res, _ = m.handlePaletteKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = res.(Model)
	if m.pal != nil {
		t.Fatal("esc did not close palette")
	}
}

func TestPaletteDispatchTab(t *testing.T) {
	m := seededModel()
	res, _ := m.dispatchPalette("tab:3")
	m = res.(Model)
	if m.tab != TabInProgress {
		t.Fatalf("tab dispatch: %v", m.tab)
	}
}

func TestMouseClickSelectsRow(t *testing.T) {
	m := seededModel()
	if m.cursor != 0 {
		t.Fatalf("start cursor %d", m.cursor)
	}
	// Click the second visible row (Y = listTopLines + 1).
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: listTopLines + 1}
	res, _ := m.Update(click)
	m = res.(Model)
	if m.cursor != 1 {
		t.Fatalf("click select: cursor=%d", m.cursor)
	}
	// Wheel scrolls.
	res, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	m = res.(Model)
	if m.cursor != 0 {
		t.Fatalf("wheel up: cursor=%d", m.cursor)
	}
}

func TestTabAtHitTest(t *testing.T) {
	m := seededModel()
	// X=1 lands inside the first tab's label.
	if tab, ok := m.tabAt(2); !ok || tab != TabAll {
		t.Fatalf("tabAt(2) = %v %v", tab, ok)
	}
	// Far right: no tab.
	if _, ok := m.tabAt(500); ok {
		t.Fatal("tabAt(500) hit something")
	}
}

func TestRowHighlightInList(t *testing.T) {
	m := seededModel()
	m.filter = strings.ToLower("AAA-2")[:3] // "aaa" matches every key
	view := m.viewList()
	if !strings.Contains(stripANSI(view), "AAA-2") {
		t.Fatal("filtered view lost row text")
	}
}
