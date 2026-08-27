package main

// The scannable half-block QR for `pairing mint` (GDK-1047). The phone app
// already scans a QR whose TEXT is the offer line — mobile's PairGate hands
// the scanned string to the same offer decoder — so mint draws exactly that
// line as a QR: nothing wrapped, no URL scheme prefix.
//
// Why stderr, and only on a TTY: stdout is the GDK-456 pipe contract
// (exactly the offer line, or one JSON object with --json) and stays
// byte-identical; a piped or redirected stderr has no pixels to scan.
// --json never draws it (machine contract), --no-qr opts out, and there is
// no --qr force flag for the same reason as the TTY gate: a consumer that
// cannot show pixels has nothing to point a phone at.
//
// Contrast must not depend on the terminal theme, so both colors are
// painted explicitly on every line: SGR 30 (black) is the foreground that
// draws the half-block glyphs — the dark modules — and SGR 47 (white) is
// the background that shows through the gaps — the light modules and the
// quiet zone — with a reset at each line end. The 4-module quiet zone on
// all four sides comes from the library's default border and is painted
// white here rather than left to the terminal background: a QR on a dark
// theme without a painted quiet zone is unscannable.
//
// NO_COLOR or TERM=dumb skips the QR entirely: a colorless half-block QR
// inverts on dark themes, and better absent than unscannable-but-trusted.
//
// The offer is a credential (internal/pairing.Offer): it appears only as
// the stdout line and inside the QR modules — never in an error message, a
// log line, or the did-not-fit fallback.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/midagedev/gadak/internal/pairflow"
)

// Terminal colors for the QR. Every line opens with both and closes with
// the reset; the glyphs do the rest.
const (
	qrDarkFG  = "\x1b[30m" // black foreground: draws ▀ ▄ █ (dark modules)
	qrLightBG = "\x1b[47m" // white background: light modules + quiet zone
	qrReset   = "\x1b[0m"
)

// stderrIsTerminal reports whether stderr is a character device — the same
// detector as init's stdinIsTerminal and search's stdoutIsTerminal, so the
// package answers "is this a TTY" one way everywhere.
func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// Injection points so tests can exercise the TTY branch through a pipe
// (the same pattern as initIsTerminal / searchIsTTY).
var (
	pairingStderrIsTerminal = stderrIsTerminal
	pairingTerminalWidth    = terminalWidth
)

// shouldDrawQR is the single decision point for whether mint paints a QR on
// stderr. Pure so the whole truth table is testable under `go test`, where
// stderr is never a TTY. noColor is NO_COLOR set to a non-empty value;
// termIsDumb is TERM exactly "dumb".
func shouldDrawQR(stderrIsTTY, noQRFlag, asJSON, noColor, termIsDumb bool) bool {
	return stderrIsTTY && !noQRFlag && !asJSON && !noColor && !termIsDumb
}

// terminalWidth reports stderr's column count, 80 when it cannot be asked.
// stty reads the size of its own stdin, so stdin is wired to this process's
// stderr — the surface the QR would be drawn on. exec-ing a system tool is
// what keeps this file buildable on Windows too (scoop ships the CLI there;
// stdlib syscall has no TIOCGWINSZ): stty does not exist there, the run
// fails, and the 80-column fallback answers.
func terminalWidth() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return 80
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return 80
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil || n <= 0 {
		return 80
	}
	return n
}

// pairingQRModules moved to pairflow.QRModules (GDK-1047: the desktop
// renders the same matrix as a PNG); this wrapper keeps the package-main
// name qr_test.go pins. The matrix carries the quiet zone (the library's
// default 4-module border).
func pairingQRModules(offer string) ([][]bool, error) {
	return pairflow.QRModules(offer)
}

// renderQRHalfblock draws text's QR into out, two modules per terminal row
// via half-block cells. Each line is one row pair; see the file comment for
// the color contract.
func renderQRHalfblock(text string, out io.Writer) error {
	mods, err := pairingQRModules(text)
	if err != nil {
		return err
	}
	writeQRHalfblock(mods, out)
	return nil
}

// writeQRHalfblock renders the module matrix (bitmap[y][x], quiet zone
// included) as half-block rows. The module count is odd (4v+17), so the
// final cell row's lower half is padding: painted white like the quiet
// zone it extends, never a stretched module — scanners bin by grid, and a
// 2:1-tall last module row distorts that binning far more than an extra
// half-row of white.
func writeQRHalfblock(mods [][]bool, out io.Writer) {
	var b strings.Builder
	for row := 0; row < len(mods); row += 2 {
		b.WriteString(qrDarkFG)
		b.WriteString(qrLightBG)
		for col := 0; col < len(mods[row]); col++ {
			upper := mods[row][col]
			lower := row+1 < len(mods) && mods[row+1][col]
			switch {
			case upper && lower:
				b.WriteRune('█')
			case upper:
				b.WriteRune('▀')
			case lower:
				b.WriteRune('▄')
			default:
				b.WriteRune(' ')
			}
		}
		b.WriteString(qrReset)
		b.WriteByte('\n')
	}
	fmt.Fprint(out, b.String())
}

// drawPairingQR is mint's QR step with the terminal width injected (0 means
// unknown, answered as 80). A code wider than the terminal is refused with
// one line naming where the pairing code actually is, rather than drawn
// wrapped — a wrapped QR is an unscannable mess wearing a scannable shape.
// The offer is a credential: the fallback line never carries it.
func drawPairingQR(w io.Writer, offer string, termWidth int) error {
	mods, err := pairingQRModules(offer)
	if err != nil {
		return err
	}
	if termWidth <= 0 {
		termWidth = 80
	}
	if len(mods) > termWidth {
		fmt.Fprintf(w, "pairing: QR skipped — the code needs %d columns and this terminal has %d; the offer is already on stdout as a line (--json for the machine form)\n",
			len(mods), termWidth)
		return nil
	}
	writeQRHalfblock(mods, w)
	return nil
}
