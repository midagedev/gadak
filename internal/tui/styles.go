package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Neon palette. High-chroma on dark terminals so the accents glow; one tone
// darker on light terminals so they stay readable. Truecolor hexes — lipgloss
// degrades them itself on 256/16-colour terminals, and drops them entirely
// under NO_COLOR or a pipe.
var (
	colPrimary   = lipgloss.AdaptiveColor{Light: "#1A1523", Dark: "#F5F0FF"}
	colSecondary = lipgloss.AdaptiveColor{Light: "#4A4458", Dark: "#C9C2DB"}
	colMuted     = lipgloss.AdaptiveColor{Light: "#8E8C99", Dark: "#6C6A7A"}
	colBorder    = lipgloss.AdaptiveColor{Light: "#C9C5D6", Dark: "#5B5470"} // lilac grey
	colSurface   = lipgloss.AdaptiveColor{Light: "#EEEAF8", Dark: "#241D40"} // faint purple surface
	colAccent    = lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#9D7CFF"} // signature purple
	colAccentFg  = lipgloss.AdaptiveColor{Light: "#6B3FD4", Dark: "#B392FF"} // violet text
	colAccentBg  = lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#4B3A8F"} // selection / active tab
	colSelFg     = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#F5F0FF"}
	colCyan      = lipgloss.AdaptiveColor{Light: "#0BA5C2", Dark: "#34E2E4"} // keys / links (aqua)
	colPink      = lipgloss.AdaptiveColor{Light: "#D6409F", Dark: "#FF6AC1"} // highlights (hot pink)
	colOK        = lipgloss.AdaptiveColor{Light: "#03A66A", Dark: "#27FFB0"} // neon mint
	colErr       = lipgloss.AdaptiveColor{Light: "#E5484D", Dark: "#FF6B6E"} // coral
	colNew       = lipgloss.AdaptiveColor{Light: "#0C8CE0", Dark: "#5EC8FF"} // sky
	colIP        = lipgloss.AdaptiveColor{Light: "#B8860B", Dark: "#FFD15C"} // gold
	colDone      = lipgloss.AdaptiveColor{Light: "#03A66A", Dark: "#27FFB0"} // mint
	colReopen    = lipgloss.AdaptiveColor{Light: "#E5484D", Dark: "#FF6B6E"} // coral
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

	// Issue key — aqua, the palette's "this is a link/id" colour.
	styleKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(colCyan)

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
