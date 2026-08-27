package main

import (
	"errors"
	"testing"
)

// GDK-917: wails runs the ErrorHandler and then os.Exit(1) without unwinding a
// single defer, so run's `defer shutdown()` never fires on a fatal. Before this
// fix the fatal path hand-rolled a partial cleanup (origin.Close only) and left
// the terminal shells — their own process groups (GDK-892) — orphaned, because
// closeTerminals lives behind shutdown → rt.Close and nothing on this path
// called it.
//
// The real leak ends in os.Exit(1), so an end-to-end FAIL-first is not
// reachable in a test. What is reachable, and what actually prevents the
// regression, is the structural contract: the fatal path delegates to the one
// shutdown closer every clean exit uses instead of re-listing cleanup steps by
// hand. This pins that delegation — if desktopFatal stops running the closer,
// the terminals (and persist) it stands for go with it.
func TestDesktopFatalRunsTheFullShutdown(t *testing.T) {
	ran := false
	desktopFatal(func() { ran = true }, errors.New("boom"))
	if !ran {
		t.Fatal("desktopFatal did not run the shutdown closer — terminals would leak and persist would not flush on a fatal (GDK-917)")
	}
}
