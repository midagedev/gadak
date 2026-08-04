package tui

import "github.com/charmbracelet/lipgloss"

// Status-category colours. Keys match store.IssueLite.StatusCategory
// (new | inprogress | done), never localized display names.
var (
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	styleKey     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	styleSel     = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255"))
	styleToastOK = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleToastErr = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleFilter  = lipgloss.NewStyle().Foreground(lipgloss.Color("213"))
	styleHelp    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))

	catNew = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))  // blue
	catIP  = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // yellow
	catDone = lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // green
	catOther = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

func statusStyle(category string) lipgloss.Style {
	switch category {
	case "new":
		return catNew
	case "indeterminate", "inprogress":
		// Jira's API key is "indeterminate"; the mirror stores "inprogress".
		return catIP
	case "done":
		return catDone
	default:
		return catOther
	}
}

func padRight(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + spaces(n-len(r))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

func fitWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s
}
