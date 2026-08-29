// Size reconciliation after a shell socket opens (GDK-1154).
//
// Shared by both panes — web/src/components/terminal/TerminalPane.svelte and
// mobile/src/screens/Shell.svelte, which imports from here the way it
// already imports ./protocol. The defect below existed in both, character
// for character, which is the argument for one module rather than two
// copies of the schedule.
//
// The pane tells the server its size once, when it creates the session, and
// again when the socket opens. Both reads come from the same measurement of
// a pane that may not have been laid out yet, and the only correction path
// after that is edge-triggered: a ResizeObserver callback. Its first call
// arrives at observe() time — before the socket is live, where the sender
// bails — and if the pane reaches its final size before then, no element
// change ever fires again. The PTY keeps the pre-layout size for the life
// of the session.
//
// Measured 2026-08-29 (headless 390x844 and the iOS simulator, both):
// sessions created at cols 10 / rows 5 while the pane rendered 48x42. The
// client's own rendering is unaffected — xterm draws at its real width — so
// nothing looks wrong until a child asks the kernel, which is every TUI:
// SIGWINCH says 10x5 and the program lays itself out for it.
//
// The fix is level-triggered rather than a bigger initial guess: re-measure
// a few times across the window in which layout settles, and send only when
// the answer differs. Each tick is a no-op once the size agrees, so the
// cost of the extra ticks is one comparison.

/** Delays after the socket opens, in ms, at which the pane re-measures. */
export const RESIZE_SETTLE_MS = [0, 150, 600] as const

export type TimerHandle = ReturnType<typeof setTimeout>

/**
 * Runs `check` once per entry in RESIZE_SETTLE_MS and returns a cancel
 * function for the ticks that have not fired yet. `schedule`/`cancel` are
 * the test seam; production passes the window timers.
 */
export function settleResize(
  check: () => void,
  schedule: (fn: () => void, ms: number) => TimerHandle = setTimeout,
  cancel: (h: TimerHandle) => void = clearTimeout,
): () => void {
  const handles = RESIZE_SETTLE_MS.map((ms) => schedule(check, ms))
  return () => {
    for (const h of handles) cancel(h)
  }
}
