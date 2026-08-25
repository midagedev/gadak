/*
 * The terminal pane's geometry, as plain numbers and one pure predicate.
 *
 * Split out of pane.svelte.ts so a node test can ask these questions without
 * the runes runtime: the chrome class holds $state and constructs itself at
 * import time, which a plain .test.ts cannot evaluate.
 */

import { VIEWPORT_DOCKED_MIN_PX } from '../viewport-regime'

export const TERMINAL_WIDTH_KEY = 'gadak.terminal.width'
export const TERMINAL_MIN_WIDTH_PX = 320
export const TERMINAL_DEFAULT_RATIO = 0.44
/*
 * The split's ceiling, as a percentage of the *track it lives in* — not of
 * the window (2026-08-25, GDK-864 lead review). The pane is a flex child of
 * the layout's main track, which is capped at 1360px, while the default
 * width is 44% of the window. On a wide display those two disagree: 44% of
 * 2560px is 1126px inside a 1360px track, leaving the issue list ~230px.
 * A percentage max-width resolves against the parent, so the list keeps its
 * share no matter how wide the display is, and dragging past the cap simply
 * saturates.
 */
export const TERMINAL_SPLIT_MAX_PCT = 60
/** Below 900px the pane is a full-width overlay instead of a split. */
export const TERMINAL_OVERLAY_MAX_PX = 899

/*
 * The width below which a split cannot coexist with a docked detail panel,
 * derived rather than chosen: VIEWPORT_DOCKED_MIN_PX is already the floor
 * where sidebar + list + detail fit at their minimums (viewport-regime.ts
 * owns those three numbers), and the terminal wants one more minimum beside
 * them.
 *
 * Without this the four surfaces do fit — on paper. `.issue-layout.detail-open`
 * gives the main track `minmax(--layout-list-min, 1fr)`, so at 1100px the
 * track is exactly the list's 390px floor; the pane's own 320px min-width
 * then wins over any percentage cap and the list is left with 70px. A list
 * 70px wide is not a list. Above this floor the CSS cap keeps the list at
 * its minimum on its own; below it, the terminal stops being a split and
 * becomes the overlay it already knows how to be — no third mode.
 */
export const TERMINAL_SPLIT_WITH_DETAIL_MIN_PX =
  VIEWPORT_DOCKED_MIN_PX + TERMINAL_MIN_WIDTH_PX

/**
 * Overlay instead of split, for either of two reasons. Pure so a test can
 * ask it every combination without a window.
 */
export function terminalIsNarrow(viewportPx: number, detailDocked: boolean): boolean {
  if (viewportPx <= TERMINAL_OVERLAY_MAX_PX) return true
  return detailDocked && viewportPx < TERMINAL_SPLIT_WITH_DETAIL_MIN_PX
}
