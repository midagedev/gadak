package term

import "testing"

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
