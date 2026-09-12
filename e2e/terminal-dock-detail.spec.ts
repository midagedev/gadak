/*
 * The dock keeps the row when a detail panel opens (GDK-1833).
 *
 * Until 2026-09-12 `terminalIsNarrow` had a second clause: a docked detail
 * panel under VIEWPORT_DOCKED_MIN_PX + TERMINAL_MIN_WIDTH_PX (1420) turned
 * the pane into the overlay sheet. That was right on 2026-08-25, when the
 * pane was a column in the same row as the sidebar, the list and the panel —
 * at 1100 the pane's 320px min-width beat the list's percentage cap and left
 * it 70px. GDK-1352 moved the pane to `grid-column: 1 / -1; grid-row: 2` on
 * 2026-09-02, a band under all three columns that spends no horizontal
 * pixels, and the rule did not follow: opening an issue at a laptop width
 * covered the list with a full-height sheet, which is exactly the frame the
 * product is for — a terminal driving the issue list.
 *
 * This spec is the recurrence gate for the removal, and it measures the three
 * numbers the old clause was protecting rather than only the mode, so
 * restoring the clause and squeezing the list both come back red. It was red
 * on the source before the removal at all three widths (`data-overlay` was
 * "true" after the click).
 */
import { expect, test } from './helpers'
import { appConsoleErrors, attachConsoleErrors, forceLocale } from './helpers'
import {
  LAYOUT_DETAIL_MIN_PX,
  LAYOUT_LIST_MIN_PX,
  VIEWPORT_DOCKED_MIN_PX,
} from '../web/src/lib/viewport-regime'
import { TERMINAL_OVERLAY_MAX_PX } from '../web/src/lib/terminal/layout'

/* The band the old clause flipped: from the narrowest docked regime (below
 * it the panel is modal and the clause never fired) to the last px under the
 * 1420 floor it imposed. */
const BAND = [VIEWPORT_DOCKED_MIN_PX, 1200, 1419]

for (const width of BAND) {
  test(`the dock stays the dock when an issue opens at ${width}px (GDK-1833)`, async ({
    page,
  }) => {
    test.setTimeout(90_000)
    await page.setViewportSize({ width, height: 860 })
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible({ timeout: 30_000 })

    await page.keyboard.press('Control+Backquote')
    const pane = page.getByTestId('terminal-pane')
    await expect(pane).toBeVisible()
    await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 60_000 })
    await expect(pane).not.toHaveAttribute('data-overlay', 'true')

    await page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').first().click()
    await expect(page.getByTestId('detail-scroll')).toBeVisible({ timeout: 30_000 })

    // The mode: still the band, not the sheet.
    await expect(pane).not.toHaveAttribute('data-overlay', 'true')

    // The three widths the removed clause existed to protect. They hold
    // because the dock is grid-row 2 — it never shared the row it was
    // accused of crowding.
    const m = await page.evaluate(() => {
      const w = (s: string) => document.querySelector(s)?.getBoundingClientRect().width ?? -1
      return {
        list: w('[data-testid="issue-list-scroller"]'),
        detail: w('[data-testid="detail-scroll"]'),
        scrollW: document.documentElement.scrollWidth,
        innerW: window.innerWidth,
      }
    })
    expect(Math.round(m.list), `list width at ${width}`).toBeGreaterThanOrEqual(LAYOUT_LIST_MIN_PX)
    expect(Math.round(m.detail) + 1, `detail width at ${width}`).toBeGreaterThanOrEqual(
      LAYOUT_DETAIL_MIN_PX,
    )
    expect(m.scrollW, `no horizontal overflow at ${width}`).toBe(m.innerW)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
}

test('below the narrow step the pane is still the sheet (GDK-1833 keeps one rule)', async ({
  page,
}) => {
  test.setTimeout(90_000)
  await page.setViewportSize({ width: TERMINAL_OVERLAY_MAX_PX, height: 860 })
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })

  await page.keyboard.press('Control+Backquote')
  const pane = page.getByTestId('terminal-pane')
  await expect(pane).toBeVisible()
  await expect(pane).toHaveAttribute('data-overlay', 'true')
})
