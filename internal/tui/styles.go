package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Palette mirrors web/src/app.css @theme (dark-first). 256-color indices are
// the Dark values; Light counterparts keep a light terminal readable.
//
//	bg layers / border  #343d49 → 238
//	accent indigo       #6366f1 → 105, #a5b4fc → 147
//	text                primary 252 / secondary 249 / muted 242
//	status              new 39 / inprogress 220 / done 42 / reopen 203
var (
	colPrimary   = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
	colSecondary = lipgloss.AdaptiveColor{Light: "238", Dark: "249"}
	colMuted     = lipgloss.AdaptiveColor{Light: "244", Dark: "242"}
	colBorder    = lipgloss.AdaptiveColor{Light: "250", Dark: "238"}
	colSurface   = lipgloss.AdaptiveColor{Light: "254", Dark: "236"} // elevated chip bg
	colAccent    = lipgloss.AdaptiveColor{Light: "62", Dark: "105"}  // indigo
	colAccentFg  = lipgloss.AdaptiveColor{Light: "55", Dark: "147"}  // violet text
	colAccentBg  = lipgloss.AdaptiveColor{Light: "189", Dark: "61"}  // selection / active tab
	colSelFg     = lipgloss.AdaptiveColor{Light: "235", Dark: "255"}
	colOK        = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}
	colErr       = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	colNew       = lipgloss.AdaptiveColor{Light: "27", Dark: "39"}
	colIP        = lipgloss.AdaptiveColor{Light: "136", Dark: "220"}
	colDone      = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}
	colReopen    = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
)

// Component styles — shared by list, detail, form, and status bar.
var (
	stylePrimary = lipgloss.NewStyle().Foreground(colPrimary)
	styleMuted   = lipgloss.NewStyle().Foreground(colMuted)
	styleDim     = lipgloss.NewStyle().Foreground(colSecondary)

	// Header brand bar
	styleBrand = lipgloss.NewStyle().
			Bold(true).
			Foreground(colAccentFg)
	styleHeaderBar = lipgloss.NewStyle().
			Foreground(colSecondary).
			Padding(0, 1)

	// Issue key
	styleKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(colAccentFg)

	// Selection: full-row indigo highlight
	styleSel = lipgloss.NewStyle().
			Foreground(colSelFg).
			Background(colAccentBg)
	styleSelKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(colAccentFg).
			Background(colAccentBg)
	styleSelMuted = lipgloss.NewStyle().
			Foreground(colSecondary).
			Background(colAccentBg)

	// Tabs
	styleTabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(colSelFg).
			Background(colAccentBg).
			Padding(0, 1)
	styleTabInactive = lipgloss.NewStyle().
				Foreground(colMuted).
				Padding(0, 1)

	// Label chips (list + detail)
	styleChip = lipgloss.NewStyle().
			Foreground(colMuted).
			Background(colSurface).
			Padding(0, 1)

	// Detail card
	styleDetailPanel = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colBorder).
				Padding(0, 1)
	styleDetailLabel = lipgloss.NewStyle().
				Foreground(colMuted).
				Width(10)
	// Discovered custom fields carry Jira display names (often longer, often
	// CJK); a wider column keeps them on one line.
	styleCustomLabel = lipgloss.NewStyle().
				Foreground(colMuted).
				Width(18)
	styleSection = lipgloss.NewStyle().
			Bold(true).
			Foreground(colPrimary).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colBorder).
			MarginBottom(0)

	// Status / help / filter / toast
	styleHelp   = lipgloss.NewStyle().Foreground(colMuted)
	styleFilter = lipgloss.NewStyle().
			Foreground(colAccentFg).
			Bold(true)
	styleToastOK = lipgloss.NewStyle().
			Bold(true).
			Foreground(colOK)
	styleToastErr = lipgloss.NewStyle().
			Bold(true).
			Foreground(colErr)
	styleSpinner = lipgloss.NewStyle().Foreground(colAccentFg)

	// Status-category colours (and reopen override)
	catNew    = lipgloss.NewStyle().Foreground(colNew)
	catIP     = lipgloss.NewStyle().Foreground(colIP)
	catDone   = lipgloss.NewStyle().Foreground(colDone)
	catReopen = lipgloss.NewStyle().Foreground(colReopen)
	catOther  = lipgloss.NewStyle().Foreground(colMuted)
)

// statusStyle maps a status_category (never a localized name) to a colour.
// reopenCount > 0 overrides to the reopen semantic colour.
func statusStyle(category string, reopenCount int) lipgloss.Style {
	if reopenCount > 0 {
		return catReopen
	}
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

// padRight and truncate work in terminal cells, not runes: CJK characters
// occupy two cells, and rune-count math misaligns every column after a Korean
// summary or assignee name.
func padRight(s string, n int) string {
	return runewidth.FillRight(runewidth.Truncate(s, n, ""), n)
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return runewidth.Truncate(s, n, "…")
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

// fitWidth pads or leaves s alone. It does not hard-cut ANSI-styled strings
// (visual width ≠ rune count); callers size gaps with lipgloss.Width first.
func fitWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := lipgloss.Width(s)
	if sw > w {
		// Last resort: plain-rune trim for unstyled content only.
		r := []rune(s)
		if len(r) > w {
			return string(r[:w])
		}
		return s
	}
	if sw < w {
		return s + spaces(w-sw)
	}
	return s
}

// joinBar lays out left and right segments with a gap so the line fills w.
func joinBar(left, right string, w int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	gap := w - lw - rw
	if gap < 1 {
		gap = 1
	}
	return fitWidth(left+spaces(gap)+right, w)
}
