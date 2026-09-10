package origin

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The GDK-1004 divergence gate. The probe budget used to live as two
// literals — this package's default and cmd/gadak/port_fallback.go's own
// const — and nothing compared them, so a tune on one side would silently
// leave the port fallback probing on the other side's stale number. The
// structural fix is a single owner: ProbeTimeout here, exported, with the
// cmd tree reading it. The cmd wiring lands outside this package, so until
// and after it this gate scans both sources and fails on any pair that
// disagrees — the value lives on two sides, but disagreement now has a
// tripwire. (A cross-package value check has no other seam: cmd/gadak is a
// main package, nothing imports it to assert against.)
//
// Attribution: re-pinned 2026-09-10, round w11-server, FAIL-first by
// perturbing each literal alone (700→701ms in port_fallback.go, then in
// advertise.go) — both runs red before the restore, outputs in the round
// report.
//
// The gate passes in exactly two states: the interim one (cmd holds its own
// literal, equal to ProbeTimeout) and the wired one (cmd references
// origin.ProbeTimeout and holds no local literal). Anything else is red.
func TestProbeTimeoutHasOneOwnerAcrossPackages(t *testing.T) {
	fallbackPath := filepath.Join("..", "..", "cmd", "gadak", "port_fallback.go")
	originSrc := mustReadSource(t, "advertise.go")
	fallbackSrc := mustReadSource(t, fallbackPath)

	originMS := timeoutLiteralMS(t, "advertise.go", originSrc,
		`ProbeTimeout\s*=\s*(\d+)\s*\*\s*time\.Millisecond`)

	if strings.Contains(fallbackSrc, "origin.ProbeTimeout") {
		// The wired state: ProbeTimeout is the one owner, and a local literal
		// that survived next to the reference would be a second owner again.
		if m := regexp.MustCompile(`probeTimeout\s*=\s*(\d+)\s*\*\s*time\.Millisecond`).
			FindStringSubmatch(fallbackSrc); m != nil {
			t.Fatalf("port_fallback.go reads origin.ProbeTimeout but still declares probeTimeout = %sms — two owners again (GDK-1004)", m[1])
		}
		return
	}

	// The interim state: cmd holds its own literal; it must agree with the
	// single owner's value.
	fallbackMS := timeoutLiteralMS(t, fallbackPath, fallbackSrc,
		`probeTimeout\s*=\s*(\d+)\s*\*\s*time\.Millisecond`)
	if fallbackMS != originMS {
		t.Fatalf("probe budgets diverged: ProbeTimeout %dms vs port_fallback %dms (GDK-1004) — tune ProbeTimeout in advertise.go, the single owner", originMS, fallbackMS)
	}
}

func mustReadSource(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("GDK-1004 gate cannot read %s: %v", path, err)
	}
	return string(b)
}

// timeoutLiteralMS extracts the millisecond literal name's regex pins. A
// missing match is fatal, not "no opinion": the scanned file must carry
// either the literal or the origin.ProbeTimeout reference the caller already
// ruled out, so silence here would mean the gate stopped looking.
func timeoutLiteralMS(t *testing.T, path, src string, re string) int {
	t.Helper()
	m := regexp.MustCompile(re).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("%s carries no probe-timeout literal (%q) — the GDK-1004 gate has nothing to compare; if the constant moved, move the gate with it", path, re)
	}
	ms, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("%s: %q is not a number: %v", path, m[1], err)
	}
	return ms
}
