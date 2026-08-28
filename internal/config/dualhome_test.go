package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The leftover-home line prints once per machine, not once per process:
// every CLI call is a fresh process, so the old per-process Once printed on
// every invocation and dirtied script output (GDK-1072). The marker under
// the active home is what makes it durable; doctor carries the standing
// report (home_leftover).
func TestDualHomeWarnsOncePerMachine(t *testing.T) {
	prev := t.TempDir()
	next := t.TempDir()

	var out bytes.Buffer
	if !dualHomeWarnDurable(prev, next, &out) {
		t.Fatal("first invocation must print")
	}
	if !strings.Contains(out.String(), prev) || !strings.Contains(out.String(), next) {
		t.Errorf("warning must name both homes: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(next, dualHomeNoticeName)); err != nil {
		t.Fatalf("marker not written: %v", err)
	}

	out.Reset()
	if dualHomeWarnDurable(prev, next, &out) {
		t.Error("second invocation must stay silent")
	}
	if out.Len() != 0 {
		t.Errorf("second invocation wrote %q", out.String())
	}
}
