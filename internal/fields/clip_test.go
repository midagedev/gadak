package fields

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// The defect Clip exists for: a Hangul or CJK rune is two cells wide, so a
// rune-counted cut renders at twice the column budget. Every case here
// asserts the measured width, not the rune count.
func TestClipCutsByDisplayWidthNotRunes(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		cols int
		want string
	}{
		{"fits", "hello", 10, "hello"},
		{"exactly fits", "hello", 5, "hello"},
		{"ascii cut", "hello world", 8, "hello w…"},
		// 5 Hangul runes = 10 cells. A rune-counted cut to 6 would keep
		// all five; the width-counted cut keeps two and the ellipsis.
		{"hangul cut", "가나다라마", 6, "가나…"},
		{"hangul fits at its own width", "가나다라마", 10, "가나다라마"},
		{"cjk cut", "漢字漢字漢字", 5, "漢字…"},
		{"whitespace collapsed", "  a\n\tb   c  ", 20, "a b c"},
		{"whitespace collapsed then cut", "a\nvery   long   line", 8, "a very …"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Clip(tc.in, tc.cols)
			if got != tc.want {
				t.Fatalf("Clip(%q, %d) = %q, want %q", tc.in, tc.cols, got, tc.want)
			}
			if w := runewidth.StringWidth(got); w > tc.cols {
				t.Errorf("result is %d cells wide, over the %d budget: %q", w, tc.cols, got)
			}
		})
	}
}

// The rune count alone would have said the Hangul case fit — this is the
// assertion the three copies existed to make, stated once.
func TestClipHangulIsWiderThanItsRuneCount(t *testing.T) {
	const in = "가나다라마" // 5 runes, 10 cells
	if n := len([]rune(in)); n != 5 {
		t.Fatalf("fixture drifted: %d runes", n)
	}
	got := Clip(in, 6)
	if len([]rune(got)) >= 5 {
		t.Errorf("a 6-cell budget must drop runes from a 10-cell string, got %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a truncated value must end in an ellipsis, got %q", got)
	}
}
