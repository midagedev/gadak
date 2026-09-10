/*
 * The bottom inset every bottom-most painted surface owes (GDK-911).
 *
 * The paint lives in app.css — `.safe-bottom, .detail-layer .sheet`
 * compose `max(var(--safe-bottom), <floor>px)` — and CSS cannot import a
 * constant, so the floor lives here as the single owner of the number and
 * the unit test beside this module pins the stylesheet to it: change the
 * CSS floor without this constant (or the reverse) and inset.test.ts is
 * red. The e2e walk (mobile/e2e/viewport.spec.ts) derives its expected
 * inset from this function rather than restating the 12, so a rig with a
 * real inset and a rig without one are both asserted against the same
 * formula.
 *
 * Why a floor at all: on a device with no reported inset (headless
 * capture, desktop browsers, notched device in a moment of reporting 0) a
 * bare `var(--safe-bottom)` would be 0 and a bottom sheet's last row would
 * sit against the screen edge — 12px is the minimum breathing room, the
 * same minimum `.safe-bottom` gives the tab bar.
 */

/** The minimum bottom inset, in px, when the reported inset is smaller or 0. */
export const SHEET_INSET_FLOOR_PX = 12

/**
 * The effective bottom inset for a reported `--safe-bottom` in px. The
 * reported value wins whenever it exceeds the floor (34px on the dev
 * shell's home indicator — App.svelte's safe-area note).
 */
export function sheetBottomInset(safeBottomPx: number): number {
  return Math.max(safeBottomPx || 0, SHEET_INSET_FLOOR_PX)
}
