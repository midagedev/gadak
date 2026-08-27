package main

// GDK-1047 gates for the mint QR: the half-block shape contract (rectangle,
// ≤80 columns, painted quiet zone, four cell runes + ANSI only), the
// too-narrow refusal (fallback line, never the offer), the shouldDrawQR
// truth table, and the mint-level suppression matrix through cmdPairing.
// The PNG test doubles as the standing scanner-repro tool.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/pairing"
)

// qrFixtureOffer builds the fixed 238-byte test offer — the size a real
// serve-scope mint measured at. The 67-char token is what lands the JSON at
// 178 bytes → base64url at 238; the assertion keeps the fixture honest if
// anyone edits the other fields.
func qrFixtureOffer(t *testing.T) string {
	t.Helper()
	tok := "Qr7" + strings.Repeat("k1o9", 16) // 3 + 64 = 67 chars
	offer, err := pairing.EncodeOffer(pairing.Offer{
		V:         pairing.OfferV1,
		Endpoint:  "https://home.example.ts.net",
		Token:     tok,
		ExpiresAt: "2026-11-25T00:00:00Z",
		Label:     "phone",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offer) != 238 {
		t.Fatalf("fixture offer is %d bytes, want the measured 238 — adjust the token length", len(offer))
	}
	if _, err := pairing.DecodeOffer(offer); err != nil {
		t.Fatalf("fixture is not a decodeable offer: %v", err)
	}
	return offer
}

// qrANSI strips the SGR sequences writeQRHalfblock emits.
var qrANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func qrHasBlockRune(s string) bool {
	return strings.ContainsAny(s, "▀▄█")
}

// TestRenderQRHalfblockShape pins the terminal shape of the 238-byte offer:
// a rectangle of half-block cells, every line opening with both colors and
// closing with the reset (theme independence), the 4-module quiet zone
// painted as the first and last two all-white terminal rows, nothing but
// the four cell runes + ANSI + newlines in the output, and the whole code
// inside 80 columns.
func TestRenderQRHalfblockShape(t *testing.T) {
	offer := qrFixtureOffer(t)
	mods, err := pairingQRModules(offer)
	if err != nil {
		t.Fatal(err)
	}
	// Measured for this fixture: EC Medium picks version 11 → 61 modules
	// + 8 quiet-zone = 69 wide, 35 terminal rows (the spec's own estimate
	// said "version 10-11"; the library decides, the test pins it). A
	// change here means the EC level (or the fixture) moved.
	if len(mods) != 69 {
		t.Fatalf("module matrix is %d wide, want 69 (version 11 @ Medium + 4-module quiet zone)", len(mods))
	}

	var buf bytes.Buffer
	if err := renderQRHalfblock(offer, &buf); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != (len(mods)+1)/2 {
		t.Fatalf("%d terminal rows, want %d (two modules per row)", len(lines), (len(mods)+1)/2)
	}
	width := utf8RuneCount(strings.TrimPrefix(strings.TrimSuffix(lines[0], qrReset), qrDarkFG+qrLightBG))
	for i, line := range lines {
		if !strings.HasPrefix(line, qrDarkFG+qrLightBG) {
			t.Fatalf("line %d does not open with both colors painted: %q", i, line)
		}
		if !strings.HasSuffix(line, qrReset) {
			t.Fatalf("line %d does not end in a reset: %q", i, line)
		}
		cells := qrANSI.ReplaceAllString(line, "")
		if n := utf8RuneCount(cells); n != width {
			t.Fatalf("line %d has %d cells, want %d — not a rectangle", i, n, width)
		}
		if width > 80 {
			t.Fatalf("QR is %d columns wide, want ≤ 80", width)
		}
		for _, r := range cells {
			if !strings.ContainsRune("▀▄█ ", r) {
				t.Fatalf("line %d carries a non-cell rune %q", i, r)
			}
		}
	}
	// The quiet zone is painted, not implied: modules 0-3 and the last 4
	// are quiet, i.e. the first two and last two terminal rows are solid
	// white (spaces) at full width.
	for _, i := range []int{0, 1, len(lines) - 2, len(lines) - 1} {
		cells := qrANSI.ReplaceAllString(lines[i], "")
		if strings.TrimLeft(cells, " ") != "" || utf8RuneCount(cells) != width {
			t.Fatalf("terminal row %d is not painted quiet-zone white: %q", i, cells)
		}
	}
	// A blank rectangle would pass every check above; the code itself must
	// be present in the middle rows.
	if !qrHasBlockRune(qrANSI.ReplaceAllString(lines[len(lines)/2], "")) {
		t.Fatalf("middle row carries no dark module: %q", lines[len(lines)/2])
	}
}

func utf8RuneCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// TestDrawPairingQRTooNarrow names the refusal and never the offer: a QR
// wider than the terminal is skipped with one stderr line, and the offer —
// a credential — must not appear in it.
func TestDrawPairingQRTooNarrow(t *testing.T) {
	offer := qrFixtureOffer(t)
	var buf bytes.Buffer
	if err := drawPairingQR(&buf, offer, 40); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "QR skipped") || !strings.Contains(out, "40") {
		t.Fatalf("narrow terminal must get the one-line refusal, got %q", out)
	}
	if strings.Contains(out, offer) {
		t.Fatal("the refusal line leaked the offer")
	}
	if qrHasBlockRune(out) {
		t.Fatalf("narrow terminal must not draw a wrapped QR: %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("the refusal is one line, got %d", strings.Count(out, "\n"))
	}
}

// Width 0 means "unknown" and is answered as 80 — the 65-column fixture
// fits, so it draws.
func TestDrawPairingQRUnknownWidthIs80(t *testing.T) {
	var buf bytes.Buffer
	if err := drawPairingQR(&buf, qrFixtureOffer(t), 0); err != nil {
		t.Fatal(err)
	}
	if !qrHasBlockRune(buf.String()) {
		t.Fatal("unknown width must fall back to 80 and draw")
	}
}

// TestShouldDrawQRTruthTable walks all 32 input combinations: every
// suppressor vetoes independently, and nothing draws without a TTY.
func TestShouldDrawQRTruthTable(t *testing.T) {
	for mask := 0; mask < 32; mask++ {
		tty := mask&1 != 0
		noQR := mask&2 != 0
		asJSON := mask&4 != 0
		noColor := mask&8 != 0
		dumb := mask&16 != 0
		want := tty && !noQR && !asJSON && !noColor && !dumb
		if got := shouldDrawQR(tty, noQR, asJSON, noColor, dumb); got != want {
			t.Fatalf("shouldDrawQR(tty=%v,no-qr=%v,json=%v,NO_COLOR=%v,dumb=%v) = %v, want %v",
				tty, noQR, asJSON, noColor, dumb, got, want)
		}
	}
}

// fakePairingTTY points the QR's terminal probes at fakes for one test —
// the same pattern searchIsTTY uses.
func fakePairingTTY(t *testing.T, tty bool, width int) {
	t.Helper()
	savedTTY, savedW := pairingStderrIsTerminal, pairingTerminalWidth
	pairingStderrIsTerminal = func() bool { return tty }
	pairingTerminalWidth = func() int { return width }
	t.Cleanup(func() {
		pairingStderrIsTerminal, pairingTerminalWidth = savedTTY, savedW
	})
}

// TestPairingMintQRSuppressionMatrix runs the real mint (cmdPairing) under
// every suppressor. Under go test stderr is a pipe, so the unfaked run is
// the non-TTY branch; the faked-TTY cases exercise the draw path and its
// too-narrow sibling through the same wiring production uses.
func TestPairingMintQRSuppressionMatrix(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		fakeTTY bool
		width   int
		env     [2]string
		wantQR  bool
	}{
		{name: "piped stderr", args: nil, wantQR: false},
		{name: "no-qr flag", args: []string{"--no-qr"}, fakeTTY: true, width: 100, wantQR: false},
		{name: "json", args: []string{"--json"}, fakeTTY: true, width: 100, wantQR: false},
		{name: "NO_COLOR", args: nil, fakeTTY: true, width: 100, env: [2]string{"NO_COLOR", "1"}, wantQR: false},
		{name: "TERM=dumb", args: nil, fakeTTY: true, width: 100, env: [2]string{"TERM", "dumb"}, wantQR: false},
		{name: "terminal wide enough", args: nil, fakeTTY: true, width: 100, wantQR: true},
		{name: "terminal too narrow", args: nil, fakeTTY: true, width: 40, wantQR: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pairingHome(t)
			fakePairingTTY(t, tc.fakeTTY, tc.width)
			if tc.env[0] != "" {
				t.Setenv(tc.env[0], tc.env[1])
			}
			out, errout, err := captureErr(t, func() error {
				args := append([]string{"mint", "--label", "phone", "--ttl", "1h", "--endpoint", "http://127.0.0.1:9"}, tc.args...)
				return cmdPairing(args)
			})
			if err != nil {
				t.Fatal(err)
			}
			// stdout is the contract the QR rides on top of: exactly the
			// offer line, QR or no QR.
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if len(lines) != 1 || strings.TrimSpace(lines[0]) == "" {
				t.Fatalf("mint stdout must stay exactly the offer line, got %q", out)
			}
			offer := lines[0]
			switch {
			case tc.wantQR:
				if !qrHasBlockRune(errout) {
					t.Fatalf("TTY mint must draw the QR on stderr:\n%s", errout)
				}
				// The QR rides among mint's other stderr lines (the
				// ensureHomeRoutingToken note may follow); each of its
				// own lines still closes with the reset.
				for _, line := range strings.Split(errout, "\n") {
					if qrHasBlockRune(line) && !strings.HasSuffix(line, qrReset) {
						t.Fatalf("QR line missing its reset: %q", line)
					}
				}
			default:
				if qrHasBlockRune(errout) {
					t.Fatalf("%s must not draw a QR:\n%s", tc.name, errout)
				}
			}
			// The too-narrow case says so without the offer; every other
			// suppressed case is silent about it (the offer lives on
			// stdout and in the QR modules only).
			if tc.name == "terminal too narrow" {
				if !strings.Contains(errout, "QR skipped") {
					t.Fatalf("narrow terminal must get the refusal line:\n%s", errout)
				}
				if strings.Contains(errout, offer) {
					t.Fatal("the refusal line leaked the offer")
				}
			}
		})
	}
}

// TestQROfferPNGRepro is the standing scanner-repro tool for GDK-1047. It
// writes the fixture offer as a PNG — 8 px per module, quiet zone included,
// the same matrix writeQRHalfblock renders — plus a sidecar .txt holding
// the exact input, so any decoder (jsqr, a phone camera) can be pointed at
// the pair without a terminal:
//
//	GADAK_QR_PNG_OUT=/tmp/offer.png go test -run TestQROfferPNGRepro ./cmd/gadak
//
// Unset, both files land in a temp dir and the test still verifies the PNG
// encodes and has the expected square dimensions.
func TestQROfferPNGRepro(t *testing.T) {
	offer := qrFixtureOffer(t)
	mods, err := pairingQRModules(offer)
	if err != nil {
		t.Fatal(err)
	}
	const scale = 8
	n := len(mods) * scale
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	white, black := color.RGBA{R: 255, G: 255, B: 255, A: 255}, color.RGBA{A: 255}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if mods[y/scale][x/scale] {
				img.SetRGBA(x, y, black)
			} else {
				img.SetRGBA(x, y, white)
			}
		}
	}
	out := os.Getenv("GADAK_QR_PNG_OUT")
	if out == "" {
		out = filepath.Join(t.TempDir(), "gdk1047-offer.png")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecar := strings.TrimSuffix(out, ".png") + ".txt"
	if err := os.WriteFile(sidecar, []byte(offer), 0o644); err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("repro PNG does not decode: %v", err)
	}
	if decoded.Bounds().Dx() != n || decoded.Bounds().Dy() != n {
		t.Fatalf("repro PNG is %dx%d, want %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy(), n, n)
	}
	t.Logf("repro pair: %s + %s", out, sidecar)
}
