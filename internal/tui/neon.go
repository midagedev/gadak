package tui

// Neon look engine: flowing gradients (shimmer), pulsing accents, ambient row
// waves, and filter-match highlighting. A global ~110ms tick advances one
// phase value; every effect derives from it, so the whole surface breathes in
// sync even when nothing else is happening.
//
// All effects are gated on animOn (set once at startup): piped output, tests,
// NO_COLOR, and SCRY_NO_ANIM all render the static styles instead. The colours
// are our own neon ramp tuned to the same hues as the static palette in
// styles.go, so a frozen frame and an animated one read as the same product.

import (
	"math"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
)

// animEnabled reports whether ambient animation should run at all.
// SCRY_NO_ANIM and NO_COLOR both force the static look.
func animEnabled() bool {
	return os.Getenv("SCRY_NO_ANIM") == "" && os.Getenv("NO_COLOR") == ""
}

func hexC(s string) colorful.Color {
	c, err := colorful.Hex(s)
	if err != nil {
		return colorful.Color{R: 1, G: 1, B: 1}
	}
	return c
}

// blendStops interpolates colour stops in Lab space at t ∈ [0,1].
func blendStops(stops []colorful.Color, t float64) colorful.Color {
	if t <= 0 {
		return stops[0]
	}
	if t >= 1 {
		return stops[len(stops)-1]
	}
	seg := t * float64(len(stops)-1)
	i := int(seg)
	if i >= len(stops)-1 {
		return stops[len(stops)-1]
	}
	return stops[i].BlendLab(stops[i+1], seg-float64(i)).Clamped()
}

// Cyclic gradient (last stop = first) so the flow never shows a seam:
// purple → pink → cyan → purple.
var shimmerStops = []colorful.Color{
	hexC("#9D7CFF"), hexC("#FF6AC1"), hexC("#34E2E4"), hexC("#9D7CFF"),
}

// shimmerSpan is how many characters one colour cycle spans (larger = subtler).
const shimmerSpan = 18.0

// shimmer paints a flowing gradient over s; as phase grows the colours drift
// left→right. Rune-based on purpose: width math stays with the caller.
func shimmer(s string, phase float64) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	var b strings.Builder
	for i, r := range runes {
		t := math.Mod(float64(i)/shimmerSpan-phase+1000.0, 1.0)
		c := blendStops(shimmerStops, t)
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(c.Hex())).Render(string(r)))
	}
	return b.String()
}

// pulseColor breathes between purple and pink on a slow sine. Borders, the
// focus dot, and the active tab use it so the frame reads alive at a glance.
func pulseColor(phase float64) lipgloss.Color {
	t := (math.Sin(phase*2*math.Pi) + 1) / 2
	c := hexC("#9D7CFF").BlendLab(hexC("#FF6AC1"), t).Clamped()
	return lipgloss.Color(c.Hex())
}

// glowColor is the selection background: same purple↔pink breath as
// pulseColor but twice as fast, and darkened so row text stays readable.
func glowColor(phase float64) lipgloss.Color {
	t := (math.Sin(phase*2*math.Pi*2.0) + 1) / 2
	c := hexC("#5B3DBE").BlendLab(hexC("#B0407E"), t).Clamped()
	return lipgloss.Color(c.Hex())
}

// ambientBg returns a per-row background for the idle wave: each row's phase
// is offset so a slow vertical ripple runs down the list. Deliberately very
// dark — foreground colours carry the content; the wave is felt, not read.
// Light terminals skip it (returns false): a dark ripple on a light ground
// would read as dirt, not ambience.
func ambientBg(phase float64, row int) (lipgloss.Color, bool) {
	if !lipgloss.HasDarkBackground() {
		return lipgloss.Color(""), false
	}
	t := (math.Sin((phase+float64(row)*0.30)*2*math.Pi) + 1) / 2
	c := hexC("#0B0A12").BlendLab(hexC("#1B1533"), t).Clamped()
	return lipgloss.Color(c.Hex()), true
}

// styleHighlight marks filter-query matches inside list text.
var styleHighlight = lipgloss.NewStyle().Bold(true).Foreground(colPink).Underline(true)

// highlightMatch renders plain through base, with case-insensitive occurrences
// of needle re-styled as matches. bg, when set, is kept behind both segments so
// selected rows stay a solid bar. Offsets are computed on the lowered string;
// for ASCII and CJK (no case) the byte offsets are identical to the original.
func highlightMatch(plain, needle string, base, match lipgloss.Style) string {
	if needle == "" {
		return base.Render(plain)
	}
	lowered := strings.ToLower(plain)
	needle = strings.ToLower(needle)
	if len(lowered) != len(plain) {
		// Case folding changed byte lengths (rare non-ASCII case pairs) —
		// offsets would lie, so skip highlighting rather than corrupt the row.
		return base.Render(plain)
	}
	var b strings.Builder
	for {
		i := strings.Index(lowered, needle)
		if i < 0 {
			b.WriteString(base.Render(plain))
			return b.String()
		}
		if i > 0 {
			b.WriteString(base.Render(plain[:i]))
		}
		b.WriteString(match.Render(plain[i : i+len(needle)]))
		plain = plain[i+len(needle):]
		lowered = lowered[i+len(needle):]
	}
}
