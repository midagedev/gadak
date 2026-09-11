package term

import (
	"strings"
	"testing"
)

/*
 * The window title as a session subtitle (GDK-1389).
 *
 * The stream scanner in bell.go already decides where an OSC string ends —
 * that decision is the hard part and it is measured (bell_test.go). These
 * tests are about the payload it used to throw away: which OSC carries a
 * title, what a title is allowed to contain, and what happens when the
 * sequence is split across PTY reads.
 *
 * FAIL-first: every assertion here was written against a scanner with no
 * takeTitle at all, so the package did not compile — the recorded failure
 * is `s.takeTitle undefined`. After the change each case was re-checked by
 * reverting the specific bound it names.
 */

// takeTitle must not fire for a chunk with no title in it.
func scanTitle(t *testing.T, s *bellScanner, chunks ...string) (string, bool) {
	t.Helper()
	var last string
	var ok bool
	for _, c := range chunks {
		s.scan([]byte(c))
		if got, set := s.takeTitle(); set {
			last, ok = got, true
		}
	}
	return last, ok
}

// ① Both terminators. Ubuntu's stock .bashrc ends the title with BEL; the
// other spelling is ESC \. A round here shipped a bug by handling one.
func TestTitleBothTerminators(t *testing.T) {
	for name, seq := range map[string]string{
		"BEL": "\x1b]0;build passing\x07$ ",
		"ST":  "\x1b]0;build passing\x1b\\$ ",
	} {
		var s bellScanner
		got, ok := scanTitle(t, &s, seq)
		if !ok {
			t.Fatalf("%s: no title captured", name)
		}
		if got != "build passing" {
			t.Fatalf("%s: title = %q, want %q", name, got, "build passing")
		}
	}
}

// ② OSC 2 is the window title too; OSC 0 sets icon name *and* window title.
// Everything else — OSC 1 (icon name only), OSC 7 (cwd), OSC 8 (hyperlink),
// OSC 133 (prompt marks) — is not a title and must not become one.
func TestTitleOnlyOSC0And2(t *testing.T) {
	want := map[string]string{
		"\x1b]0;zero\x07": "zero",
		"\x1b]2;two\x07":  "two",
	}
	for seq, title := range want {
		var s bellScanner
		got, ok := scanTitle(t, &s, seq)
		if !ok || got != title {
			t.Fatalf("%q: title = %q %v, want %q", seq, got, ok, title)
		}
	}
	for _, seq := range []string{
		"\x1b]1;icon\x07",
		"\x1b]7;file:///tmp\x07",
		"\x1b]8;;https://example.invalid\x07",
		"\x1b]133;A\x07",
		"\x1b]02;padded\x07",
		"\x1b]0\x07",
		"\x1b]\x07",
		"\x1bPq#0;2\x1b\\",
	} {
		var s bellScanner
		if got, ok := scanTitle(t, &s, seq); ok {
			t.Fatalf("%q became a title: %q", seq, got)
		}
	}
}

// ③ A PTY read can end anywhere. Split the same sequence at every byte
// boundary and the title must still arrive exactly once.
func TestTitleSplitAcrossReads(t *testing.T) {
	const seq = "\x1b]0;split me\x07"
	for i := 1; i < len(seq); i++ {
		var s bellScanner
		got, ok := scanTitle(t, &s, seq[:i], seq[i:])
		if !ok {
			t.Fatalf("split at %d: no title", i)
		}
		if got != "split me" {
			t.Fatalf("split at %d: title = %q", i, got)
		}
	}
	// And the nastiest split of all: between the ESC and the backslash of
	// an ST terminator.
	var s bellScanner
	got, ok := scanTitle(t, &s, "\x1b]2;st split\x1b", "\\rest")
	if !ok || got != "st split" {
		t.Fatalf("ESC/backslash split: title = %q %v", got, ok)
	}
}

// ④ takeTitle is take, not peek: a second call with nothing new in between
// reports nothing, so the session does not re-store the same title on every
// chunk of output.
func TestTakeTitleClears(t *testing.T) {
	var s bellScanner
	s.scan([]byte("\x1b]0;once\x07"))
	if got, ok := s.takeTitle(); !ok || got != "once" {
		t.Fatalf("first take = %q %v", got, ok)
	}
	if got, ok := s.takeTitle(); ok {
		t.Fatalf("second take returned %q", got)
	}
	s.scan([]byte("plain output\n"))
	if got, ok := s.takeTitle(); ok {
		t.Fatalf("plain output produced a title: %q", got)
	}
}

// ⑤ The title is attacker-adjacent: it is whatever the shell ran decided to
// print. Control characters cannot reach the roster, and the length is
// capped, so a crafted title cannot corrupt a row or hold memory.
func TestTitleIsBounded(t *testing.T) {
	var s bellScanner
	got, ok := scanTitle(t, &s, "\x1b]0;a\rb\nc\x00d\x7fe\x07")
	if !ok {
		t.Fatal("no title")
	}
	if strings.ContainsAny(got, "\r\n\x00\x1b\x7f") {
		t.Fatalf("control characters survived: %q", got)
	}
	if got != "a b c d e" {
		t.Fatalf("title = %q, want %q", got, "a b c d e")
	}

	long := strings.Repeat("가", maxTitleRunes*3)
	var s2 bellScanner
	got, ok = scanTitle(t, &s2, "\x1b]0;"+long+"\x07")
	if !ok {
		t.Fatal("no title for the long one")
	}
	if n := len([]rune(got)); n != maxTitleRunes {
		t.Fatalf("long title kept %d runes, want %d", n, maxTitleRunes)
	}

	// A shell that opens an OSC and never terminates it must not grow the
	// capture buffer without bound.
	var s3 bellScanner
	s3.scan([]byte("\x1b]0;"))
	for range 64 {
		s3.scan([]byte(strings.Repeat("x", 4096)))
	}
	if n := len(s3.osc); n > oscCaptureMax {
		t.Fatalf("capture buffer grew to %d, cap is %d", n, oscCaptureMax)
	}
	// It still finds the terminator afterwards.
	s3.scan([]byte("\x07"))
	if _, ok := s3.takeTitle(); !ok {
		t.Fatal("scanner lost the terminator after overflowing")
	}

	// Whitespace-only is nothing at all, not an empty subtitle.
	var s4 bellScanner
	if got, ok := scanTitle(t, &s4, "\x1b]0;   \t \x07"); ok {
		t.Fatalf("whitespace-only title captured: %q", got)
	}
}

// ⑥ A title arriving must never raise the "a person is wanted" bit, whether
// it ends with BEL or ST — the contract attention_test.go:90 pins, asserted
// here at the scanner so a regression is caught without a PTY.
func TestTitleNeverRingsTheBell(t *testing.T) {
	for _, seq := range []string{
		"\x1b]0;quiet title\x07",
		"\x1b]2;quiet title\x1b\\",
	} {
		var s bellScanner
		if s.scan([]byte(seq)) {
			t.Fatalf("%q rang the bell", seq)
		}
		if _, ok := s.takeTitle(); !ok {
			t.Fatalf("%q produced no title", seq)
		}
	}
}
