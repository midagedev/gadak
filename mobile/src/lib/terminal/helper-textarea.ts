/*
 * xterm's helper textarea, on a phone (GDK-1986).
 *
 * The phone declares everywhere that xterm is a *renderer* and not an input
 * surface: the terminal is constructed `disableStdin: true`, keystrokes come
 * from the screen's own `.ime` field, special keys from the KeyBar, and
 * scrolling from the touch gesture in Shell.svelte. xterm paints; nothing
 * else.
 *
 * One thing did not get the message. xterm keeps a hidden textarea of its
 * own and focuses it from its own mousedown handler — and on iOS the tap the
 * user makes is followed, ~50ms after touchend, by a synthesized mouse event
 * that reaches it. Measured on an iPhone (iOS 18.7, Safari) against /m/ on
 * 2026-09-17, one tap on the terminal:
 *
 *   7236ms  pointerdown  on DIV.xterm-screen
 *   7241ms  FOCUSIN      TEXTAREA.ime#shell-ime          ← ours, from the tap
 *   7329ms  touchend
 *   7344ms  vv-resize    band=320                        ← the keyboard IS up
 *   7377ms  FOCUSOUT     TEXTAREA.ime#shell-ime          ← stolen
 *   7378ms  FOCUSIN      TEXTAREA.xterm-helper-textarea
 *   7836ms  vv-resize    band=0                          ← and it goes back down
 *
 * The keyboard was never failing to appear. It appeared, and then the focus
 * moved to a textarea that `disableStdin` had made readOnly — and iOS does
 * not keep a keyboard up for a readOnly field. The reported symptom, "키보드
 * 나올 것 같이 영역이 생겼다가 없어져", is that 459ms exactly.
 *
 * `inert` alone was not enough, and the second trace is why it is not the
 * whole fix. With the helper inert the FOCUSIN above is gone — the steal has
 * no target — but the field still lost focus at the same offset, to nothing
 * at all:
 *
 *   9038ms  pointerdown  on DIV.xterm-screen
 *   9045ms  FOCUSIN      TEXTAREA.ime#shell-ime
 *   9141ms  vv-resize    band=320
 *   9145ms  touchend
 *   9178ms  FOCUSOUT     TEXTAREA.ime#shell-ime          ← and no FOCUSIN after
 *   9613ms  vv-resize    band=0
 *
 * xterm's handler is `e.preventDefault(); this.focus()`, and `focus()` ends
 * at `this.textarea.focus()`. The preventDefault means the browser's own
 * "click elsewhere, blur" never runs, so that call is the only thing left in
 * the frame that can move focus — and the measurement says it moved it off
 * our field without landing anywhere. So the close is at the call, not at
 * the target's interactivity: the element's own `focus` becomes a no-op, and
 * the steal stops being something that has to fail gracefully.
 *
 * `inert` stays beside it. It is the same sentence said to the rest of the
 * platform — tab order, the accessibility tree, hit testing — and nothing
 * here is taken away: every job that textarea would do is already somebody
 * else's on this screen, which is what `disableStdin: true` declared.
 *
 * Deliberately not done on the desktop pane: there xterm's textarea IS the
 * input surface, and there is no second field to protect.
 */

/** The element xterm parks off-screen to take keystrokes into. */
const HELPER_SELECTOR = 'textarea.xterm-helper-textarea'

/** The helper, as narrow as this module needs it (the seam idiom — a test
 *  hands in a plain object, no DOM). */
export type HelperLike = {
  inert?: boolean
  focus: (...args: unknown[]) => void
  setAttribute(name: string, value: string): void
  dataset?: Record<string, string>
}

/**
 * Take xterm's helper textarea out of the focus conversation inside `host`.
 *
 * Returns whether one was found — false is a real answer (a renderer that
 * has not opened yet, or an xterm that stopped shipping the element), and
 * the caller's test pins it rather than letting a silent miss read as done.
 */
export function neutraliseHelperTextarea(host: {
  querySelector(sel: string): HelperLike | null
}): boolean {
  const el = host.querySelector(HELPER_SELECTOR)
  if (!el) return false
  // The close: xterm may keep calling focus() on its own field from its own
  // handlers — this makes every one of those calls do nothing, here, where
  // the field was never the input surface.
  el.focus = () => {}
  // And the same thing said to the platform: not interactive, not in the tab
  // order, not in the accessibility tree.
  el.inert = true
  el.setAttribute('inert', '')
  // So a device probe can read back that this ran, without guessing from the
  // absence of an event.
  if (el.dataset) el.dataset.gadakNeutralised = ''
  return true
}
