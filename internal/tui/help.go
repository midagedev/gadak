package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// overlayHelp draws the ? help panel over the current frame.
// Only keys from defaultKeys / keyMap.helpLines are listed.
func (m Model) overlayHelp(base string) string {
	lines := m.keys.helpLines()
	var body strings.Builder
	body.WriteString(styleSection.Render("Keys"))
	body.WriteByte('\n')
	keyCol := 0
	for _, pair := range lines {
		if w := lipgloss.Width(pair[0]); w > keyCol {
			keyCol = w
		}
	}
	if keyCol < 6 {
		keyCol = 6
	}
	for _, pair := range lines {
		body.WriteString("  ")
		body.WriteString(styleKey.Render(padRight(pair[0], keyCol)))
		body.WriteString("  ")
		body.WriteString(stylePrimary.Render(pair[1]))
		body.WriteByte('\n')
	}
	body.WriteByte('\n')
	// Honest parity notes (TUI_PRINCIPLES §10 — silence is the only wrong answer).
	body.WriteString(styleMuted.Render("  docs / filter: title & space only"))
	body.WriteByte('\n')
	body.WriteString(styleMuted.Render("  full-text search: web/CLI (not TUI)"))
	body.WriteByte('\n')
	body.WriteString(styleMuted.Render("  Viewed recency lives in the web UI's browser storage; the TUI does not track visits yet."))
	body.WriteByte('\n')
	body.WriteByte('\n')
	body.WriteString(styleMuted.Render("  ? or esc to close"))

	panelW := min(m.width-4, 48)
	if panelW < 24 {
		panelW = max(16, m.width-2)
	}
	panel := styleDetailPanel.Width(panelW).Render(strings.TrimRight(body.String(), "\n"))

	// Stack: keep header of base, then panel, then rest truncated by height.
	baseLines := strings.Split(base, "\n")
	var out strings.Builder
	if len(baseLines) > 0 {
		out.WriteString(baseLines[0])
		out.WriteByte('\n')
	}
	out.WriteString(panel)
	// Pad remaining height so the frame stays stable.
	used := 1 + strings.Count(panel, "\n") + 1
	for used < m.height {
		out.WriteByte('\n')
		used++
	}
	return out.String()
}
