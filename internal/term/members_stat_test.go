package term

import (
	"strings"
	"testing"
)

func TestParseProcStat(t *testing.T) {
	pid, tty, ok := parseProcStat("3043 (bash) S 3022 3043 3043 34816 3043 4210688\n")
	if !ok || pid != 3043 || tty != 34816 {
		t.Fatalf("plain comm: pid=%d tty=%d ok=%v", pid, tty, ok)
	}
	pid, tty, ok = parseProcStat("99 (sleep 300) S 1 1 1 34816 1\n")
	if !ok || pid != 99 || tty != 34816 {
		t.Fatalf("comm with space: pid=%d tty=%d ok=%v", pid, tty, ok)
	}
	pid, tty, ok = parseProcStat("123 (some (weird) name) S 1 2 3 0 4\n")
	if !ok || pid != 123 || tty != 0 {
		t.Fatalf("comm with parens: pid=%d tty=%d ok=%v", pid, tty, ok)
	}
	if _, _, ok := parseProcStat("not a stat line"); ok {
		t.Fatal("garbage parsed as stat")
	}
}

// TestParseProcStartTime covers the pure string half this file's header
// promises is testable everywhere. starttime is field 22 — index 19 after
// the comm — so the guard is "fewer than 20 fields after the last ')' reads
// as absent", and non-numeric or empty input is absent too. Before this
// test the function was U1000-dead on darwin and windows every time
// tools/staticcheck.sh ran, demanding a hand-written judgement (GDK-1578).
func TestParseProcStartTime(t *testing.T) {
	// A real-shaped tail: state..starttime (field 22) = 12345.
	const tail = "S 3022 3043 3043 34816 3043 4210688 0 0 0 0 0 0 0 0 0 0 0 0 12345"
	cases := []struct {
		name string
		stat string
		want uint64
		ok   bool
	}{
		{"plain comm", "3043 (bash) " + tail, 12345, true},
		{"comm with space and parens", "99 (some (weird) name) " + tail, 12345, true},
		{"no closing paren", "not a stat line", 0, false},
		{"empty", "", 0, false},
		{"too few fields after comm", "3043 (bash) S 1 2 3", 0, false},
		{"non-numeric starttime", "3043 (bash) S " + strings.Repeat("0 ", 18) + "soon", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseProcStartTime(tc.stat)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("%s: parseProcStartTime(%q) = %d, %v; want %d, %v",
				tc.name, tc.stat, got, ok, tc.want, tc.ok)
		}
	}
}
