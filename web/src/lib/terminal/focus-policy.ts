/*
 * Who may take the terminal's keyboard focus, and when (GDK-1986).
 *
 * The pane used to focus the renderer from the socket's `onOpen`
 * (driver.ts, `onAttached`) — a moment with no user gesture anywhere near
 * it. On a desktop that is exactly right: the shell you just opened should
 * be ready to type into.
 *
 * On a touch device it is the defect. Measured on an iPhone over the tailnet
 * (2026-09-17, four probe rounds served from this repo's scratch):
 *
 *   - A textarea focused from inside a real touch gesture raises the
 *     software keyboard — from touchstart, from the synthesized mousedown,
 *     and from click alike.
 *   - The same textarea focused one task later (`setTimeout(…, 0)` out of
 *     pointerdown) does NOT. Focus lands; no keyboard. That is the only
 *     probe case of the round that failed.
 *   - xterm.js 6.0.0 standing on its own, tapped with a finger, DOES raise
 *     it. The library is not the problem.
 *
 * `onAttached` is that failing shape: it focuses outside a gesture, so it
 * cannot raise a keyboard, and it leaves the element focused — after which
 * a tap is no longer a focus change. The steal buys nothing on a phone and
 * costs the only thing the pane is for.
 *
 * So the rule is the narrow one: the attach-time focus is for pointers that
 * came with a keyboard. A coarse pointer gets focus from its own tap, which
 * is xterm's own path and the one the probe measured working — and the same
 * split the phone app already lives by (`mobile/src/screens/Shell.svelte`
 * answers onAttached with `renderer.reset()`, never a focus, and focuses
 * its field from `onHostPointerDown`).
 *
 * Pure and matchMedia-injected so it is a unit test, not a device.
 */

/** The media query that separates a finger from a mouse. One spelling. */
export const COARSE_POINTER_QUERY = '(pointer: coarse)'

/**
 * May the pane take focus for itself when a socket attaches?
 *
 * `mql` is the match result for COARSE_POINTER_QUERY. A browser too old to
 * answer, or a headless renderer with no matchMedia, is treated as fine —
 * the desktop behaviour, which is what every non-touch surface had before
 * this predicate existed.
 */
export function mayStealFocusOnAttach(mql: { matches: boolean } | null | undefined): boolean {
  return !mql?.matches
}

/** The live answer, for the pane. Separated from the predicate so the
 *  predicate stays testable without a window. */
export function attachFocusAllowed(win: Window | undefined = globalThis.window): boolean {
  if (typeof win?.matchMedia !== 'function') return true
  return mayStealFocusOnAttach(win.matchMedia(COARSE_POINTER_QUERY))
}
