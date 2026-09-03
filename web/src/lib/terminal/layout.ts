/*
 * The terminal pane's geometry, as plain numbers and one pure predicate.
 *
 * Split out of pane.svelte.ts so a node test can ask these questions without
 * the runes runtime: the chrome class holds $state and constructs itself at
 * import time, which a plain .test.ts cannot evaluate.
 */

import { VIEWPORT_DOCKED_MIN_PX } from '../viewport-regime'

/*
 * GDK-1194 (2026-08-30): the split is horizontal — a dock across the bottom
 * of the window (the whole width since GDK-1352, sidebar included) — so the
 * persisted number is a height. A new key rather than a migrated one: a
 * stored width is a number in the wrong axis, and 44% of a window read as a
 * height is a dock that opens nearly half-screen on one machine and 300px on
 * another.
 */
export const TERMINAL_HEIGHT_KEY = 'gadak.terminal.height'
export const TERMINAL_MIN_HEIGHT_PX = 160
/*
 * GDK-1352 (2026-09-02): a quarter of the window, down from 40%. The dock is
 * a band under the tracker, not a second workspace: a gadak one-liner and its
 * answer fit in ~9 rows at 900px tall, which is the height the promo clips'
 * paper terminal has always drawn (168px on 808), and the first open should
 * look like that. A session that wants more drags the handle once and the
 * height persists.
 */
export const TERMINAL_DEFAULT_HEIGHT_RATIO = 0.25
/*
 * The dock's ceiling, as a fraction of the window. Unlike the old width cap
 * this has no track to resolve against — the dock spans the whole window
 * width — so the clamp lives in persistHeight (pane.svelte.ts), the single
 * owner, and this is the number it uses.
 */
export const TERMINAL_MAX_HEIGHT_RATIO = 0.7

/** The dock's first-open height for a window this tall: a quarter, never
 *  under the minimum. Pure, so it is a unit test, not an e2e (GDK-1376). */
export function dockDefaultHeight(innerHeight: number): number {
  return Math.max(TERMINAL_MIN_HEIGHT_PX, Math.round(innerHeight * TERMINAL_DEFAULT_HEIGHT_RATIO))
}

/** The dock's ceiling for a window this tall. */
export function dockMaxHeight(innerHeight: number): number {
  return Math.max(TERMINAL_MIN_HEIGHT_PX, Math.round(innerHeight * TERMINAL_MAX_HEIGHT_RATIO))
}

/** The roster column in the overlay regime (<900px): no sidebar above it to
 *  align with, so a fixed narrow track. Owned here with the other pane
 *  dimensions rather than as a CSS literal (GDK-1370). */
export const TERMINAL_OVERLAY_ROSTER_PX = 160

/*
 * Still the pane's minimum *width*, and still the number the narrow rule
 * below is derived from: an overlay pane covers the content track, so the
 * width at which it can no longer coexist with a docked detail panel is
 * unchanged by the dock.
 */
export const TERMINAL_MIN_WIDTH_PX = 320
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
