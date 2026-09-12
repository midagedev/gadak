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

/*
 * The shape is the reader's (GDK-1835).
 *
 * Before this, the only way to change the terminal's shape was to resize the
 * window, and the window could change it back. These cases pin the three
 * things that makes different: the control flips it, the choice survives a
 * resize that would have decided otherwise, and in the full shape the roster
 * is a block in the app sidebar rather than a second rail beside it.
 */
test('the reader pins the shape, and a resize does not unpin it (GDK-1835)', async ({ page }) => {
  test.setTimeout(90_000)
  await page.setViewportSize({ width: 1400, height: 860 })
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })

  await page.keyboard.press('Control+Backquote')
  const pane = page.getByTestId('terminal-pane')
  await expect(pane).toBeVisible()
  await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 60_000 })
  await expect(pane).not.toHaveAttribute('data-overlay', 'true')

  // The dock's roster is the column inside the pane.
  await expect(page.locator('[data-testid="terminal-chrome"][data-variant="dock"]')).toBeVisible()
  await expect(page.locator('[data-testid="terminal-chrome"][data-variant="sidebar"]')).toHaveCount(0)

  await page.getByTestId('terminal-shape').click()
  await expect(pane).toHaveAttribute('data-overlay', 'true')

  // In the full shape the rows moved into the sidebar and the pane has no
  // rail of its own — the whole point of the move.
  const inSidebar = page.locator('[data-testid="terminal-chrome"][data-variant="sidebar"]')
  await expect(inSidebar).toBeVisible()
  await expect(page.locator('[data-testid="terminal-chrome"][data-variant="dock"]')).toHaveCount(0)
  await expect(inSidebar.locator('xpath=ancestor::aside[contains(@class,"issue-sidebar")]')).toHaveCount(1)

  // The roster's verbs cross a boundary now: the rows are in the sidebar and
  // the session they start belongs to the pane. `+` is the one that does, so
  // press it here rather than trusting that the dock's press covers it.
  const rows = inSidebar.getByTestId('terminal-strip-row')
  const before = await rows.count()
  await inSidebar.getByTestId('terminal-new').click()
  await expect(rows).toHaveCount(before + 1)

  // A width that would have chosen the dock does not take the choice back.
  await page.setViewportSize({ width: 1600, height: 860 })
  await expect(pane).toHaveAttribute('data-overlay', 'true')

  // And back, by the same control, from its sidebar home.
  await page.getByTestId('terminal-shape').click()
  await expect(pane).not.toHaveAttribute('data-overlay', 'true')
  await expect(page.locator('[data-testid="terminal-chrome"][data-variant="dock"]')).toBeVisible()

  // A width that would have chosen the sheet does not take *that* back either.
  await page.setViewportSize({ width: 820, height: 860 })
  await expect(pane).not.toHaveAttribute('data-overlay', 'true')
})

/*
 * The way out is reachable from under the sheet (GDK-1835).
 *
 * The narrow regime makes the sidebar inert and raises a live scrim when a
 * detail panel is open — both correct when the panel is the top surface, and
 * both wrong once the terminal sheet is, because the sheet's own chrome now
 * lives in that sidebar. Measured before the fix at this exact width and
 * order: the shape and close controls rendered, and `elementFromPoint` on
 * the shape control returned `issue-scrim`. A control that is drawn and does
 * not answer is worse than one that is not drawn.
 */
test('narrow: a detail panel does not lock the sheet chrome away (GDK-1835)', async ({ page }) => {
  test.setTimeout(90_000)
  await page.setViewportSize({ width: 860, height: 860 })
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('issue-list-scroller')).toBeVisible({ timeout: 30_000 })

  // Open the issue first: after the pane is up the sheet covers the list, so
  // this is the order that reaches the state at all.
  await page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').first().click()
  await expect(page.getByTestId('detail-scroll')).toBeVisible({ timeout: 30_000 })

  await page.keyboard.press('Control+Backquote')
  const pane = page.getByTestId('terminal-pane')
  await expect(pane).toBeVisible()
  await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 60_000 })
  await expect(pane).toHaveAttribute('data-overlay', 'true')
  await expect(page.locator('[data-testid="terminal-chrome"][data-variant="sidebar"]')).toBeVisible()

  // Nothing between the press and the control.
  const hit = await page.evaluate(() => {
    const el = document.querySelector('[data-testid="terminal-shape"]')
    const r = el?.getBoundingClientRect()
    if (!r) return 'no control'
    const at = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)
    return at?.closest('[data-testid="terminal-shape"]') ? 'the control' : (at as HTMLElement)?.dataset?.testid || at?.tagName || 'something else'
  })
  expect(hit, 'what a press on the shape control actually lands on').toBe('the control')

  // And it works: the pane goes back to the dock from here.
  await page.getByTestId('terminal-shape').click()
  await expect(pane).not.toHaveAttribute('data-overlay', 'true')
})

