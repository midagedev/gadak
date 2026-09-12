/*
 * The terminal pane's geometry, as plain numbers and one pure predicate.
 *
 * Split out of pane.svelte.ts so a node test can ask these questions without
 * the runes runtime: the chrome class holds $state and constructs itself at
 * import time, which a plain .test.ts cannot evaluate.
 */

import { LAYOUT_NARROW_MAX_PX } from '../viewport-regime'

/*
 * GDK-1194 (2026-08-30): the split is horizontal — a dock across the bottom
 * of the window (the whole width since GDK-1352, sidebar included) — so the
 * persisted number is a height. A new key rather than a migrated one: a
 * stored width is a number in the wrong axis, and 44% of a window read as a
 * height is a dock that opens nearly half-screen on one machine and 300px on
 * another.
 */
/*
 * The reader's own choice of shape (GDK-1835), beside the height and for the
 * same reason the comment on TERMINAL_HEIGHT_KEY gives: this is a preference
 * of one viewer's chrome, not a fact about the workspace document the CLI and
 * other tabs share. Empty (absent) means "follow the window", which is what a
 * reader who has never touched the control gets.
 */
export const TERMINAL_MODE_KEY = 'gadak.terminal.mode'

/** The two shapes a reader can pin; '' is the width-derived default. */
export type TerminalMode = 'dock' | 'full' | ''

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
 * The pane's minimum *width*, and nothing else since GDK-1833 (2026-09-12).
 * It used to be the number a second narrow rule was derived from, kept
 * through GDK-1352 on the reasoning that "an overlay pane covers the content
 * track, so the width at which it can no longer coexist with a docked detail
 * panel is unchanged by the dock". That sentence measures the overlay's
 * geometry; the question it was answering is whether the *dock* can coexist
 * with a docked detail panel, and since GDK-1352 the dock is
 * `grid-column: 1 / -1; grid-row: 2` — a band under all three columns that
 * spends zero horizontal pixels. See terminalIsNarrow below.
 */
export const TERMINAL_MIN_WIDTH_PX = 320
/*
 * Below 900px the pane is a full-width overlay instead of a split. The
 * boundary is the layout's narrow sidebar step (GDK-1091 A-8 / GDK-1369),
 * re-exported rather than re-chosen: "under 900" is one window regime —
 * narrow sidebar, terminal sheet — and two numbers here could drift apart
 * and show a docked split beside a stepped sidebar.
 */
export const TERMINAL_OVERLAY_MAX_PX = LAYOUT_NARROW_MAX_PX

/**
 * Overlay instead of dock, for one reason: the window is narrower than the
 * layout's narrow step.
 *
 * There was a second reason until GDK-1833 (2026-09-12) — a docked detail
 * panel under VIEWPORT_DOCKED_MIN_PX + TERMINAL_MIN_WIDTH_PX (1420) — and it
 * was right for the layout it was written against on 2026-08-25, when the
 * pane was a column beside the list: `.issue-layout.detail-open` gives the
 * main track `minmax(--layout-list-min, 1fr)`, so at 1100px the track is
 * exactly the list's 390px floor, the pane's own 320px min-width then won
 * over any percentage cap, and the list was left with 70px. A list 70px wide
 * is not a list.
 *
 * GDK-1352 (2026-09-02) moved the pane out of that row: the dock is
 * `grid-column: 1 / -1; grid-row: 2`, a band under the sidebar, the list and
 * the detail panel alike, and it spends its budget in height, not width. The
 * rule outlived its premise and kept flipping a perfectly-fitting dock into a
 * sheet the moment an issue was opened — between 1100 (below which the detail
 * panel is modal and the clause never fired) and 1419, which is where a
 * laptop browser sits. Measured at 1100×860 with the pane open and an issue
 * opened: the list is 390 and the detail panel 437, their exact floors, and
 * `documentElement.scrollWidth` equals `innerWidth` — the three numbers the
 * old rule was protecting hold without it.
 *
 * Two surfaces had already paid for the premise rather than questioning it:
 * `e2e/issue-command.spec.ts` forces a 1600px viewport because the sheet
 * covered the ▶ it needed to click, and `e2e/demo/roundtrip.config.ts` had to
 * clear 1420 because at 1100 "beat 2 could not be in one frame at all".
 *
 * Pure so a test can ask it every combination without a window.
 */
export function terminalIsNarrow(viewportPx: number): boolean {
  return viewportPx <= TERMINAL_OVERLAY_MAX_PX
}
