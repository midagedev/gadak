package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// Column alignment is in terminal cells, not runes: a Korean summary occupies
// two cells per character, and rune-count padding shifts every later column.
func TestPadRightAndTruncateUseDisplayWidth(t *testing.T) {
	for _, c := range []struct{ in string }{
		{"ascii text"}, {"한글 요약"}, {"mixed 한글 text"}, {""},
	} {
		if got := lipgloss.Width(padRight(c.in, 20)); got != 20 {
			t.Errorf("padRight(%q, 20) width = %d, want 20", c.in, got)
		}
	}
	if got := lipgloss.Width(padRight("한글 요약이 아주 아주 길다", 10)); got > 10 {
		t.Errorf("padRight over-wide input = %d, want <= 10", got)
	}
	if got := lipgloss.Width(truncate("한글 요약이 아주 아주 길다", 10)); got > 10 {
		t.Errorf("truncate width = %d, want <= 10", got)
	}
}
