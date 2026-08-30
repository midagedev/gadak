package term

import "testing"

/*
 * What counts as a bell (GDK-1163, 2026-08-30).
 *
 * FAIL-first, measured against the previous implementation — a plain
 * bytes.IndexByte(chunk, bellByte) in emit — the OSC cases below all reported
 * a bell: "a window title terminated by BEL is not a bell" failed with
 * rang = true, and so did the split-chunk case. That was the whole defect:
 * Ubuntu's stock prompt writes such a title before every prompt, so the strip
 * marked every Linux session as wanting a person and nothing could lower it.
 */

func TestBellScannerGroundBell(t *testing.T) {
	var s bellScanner
	if !s.scan([]byte("ding\x07\n")) {
		t.Fatal("a bare BEL in ordinary output is a bell")
	}
	if s.scan([]byte("quiet\n")) {
		t.Fatal("output with no BEL rang")
	}
}

func TestBellScannerOSCTerminatorIsNotABell(t *testing.T) {
	// The exact bytes Ubuntu's /etc/skel/.bashrc prompt writes.
	title := []byte("\x1b]0;runner@runnervmgx7h7: ~\x07runner@runnervmgx7h7:~$ ")
	var s bellScanner
	if s.scan(title) {
		t.Fatal("a window title terminated by BEL is not a bell")
	}
	// …and the scanner is back in ground afterwards, so a real bell that
	// follows it still counts.
	if !s.scan([]byte("\x07")) {
		t.Fatal("the BEL after the title should have rung")
	}
}

func TestBellScannerSplitAcrossChunks(t *testing.T) {
	// A PTY read can end anywhere, including between the ESC and the ].
	var s bellScanner
	if s.scan([]byte("\x1b")) {
		t.Fatal("a lone ESC rang")
	}
	if s.scan([]byte("]0;title")) {
		t.Fatal("the middle of an OSC string rang")
	}
	if s.scan([]byte("\x07$ ")) {
		t.Fatal("the OSC terminator in a later chunk rang")
	}
	if !s.scan([]byte("\x07")) {
		t.Fatal("a bell after the string closed should ring")
	}
}

func TestBellScannerESCBackslashAlsoClosesTheString(t *testing.T) {
	var s bellScanner
	if s.scan([]byte("\x1b]0;title\x1b\\")) {
		t.Fatal("an ST-terminated title rang")
	}
	if !s.scan([]byte("\x07")) {
		t.Fatal("the scanner did not return to ground after ESC \\")
	}
}

func TestBellScannerOtherStringOpeners(t *testing.T) {
	// DCS, SOS, PM, APC all end on a string terminator too.
	for _, opener := range []byte{'P', 'X', '^', '_'} {
		var s bellScanner
		if s.scan([]byte{escByte, opener, 'a', 'b', bellByte}) {
			t.Fatalf("ESC %q string terminator read as a bell", opener)
		}
		if !s.scan([]byte{bellByte}) {
			t.Fatalf("ESC %q left the scanner outside ground", opener)
		}
	}
}

func TestBellScannerCSIDoesNotSwallowABell(t *testing.T) {
	// A CSI ends on its own final byte, so a BEL after one is still a bell —
	// and the scanner must not treat `[` as a string opener.
	var s bellScanner
	if !s.scan([]byte("\x1b[1;31m\x07")) {
		t.Fatal("a BEL after a CSI colour did not ring")
	}
}

func TestBellScannerESCBellRings(t *testing.T) {
	// ESC opens nothing here, so the 0x07 that follows is a bell.
	var s bellScanner
	if !s.scan([]byte{escByte, bellByte}) {
		t.Fatal("ESC BEL did not ring")
	}
	if !s.scan([]byte{bellByte}) {
		t.Fatal("the scanner did not return to ground after ESC BEL")
	}
}
