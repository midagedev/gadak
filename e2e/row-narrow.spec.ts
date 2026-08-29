import { test, expect, type Page } from '@playwright/test'
import { forceLocale } from './helpers'

/*
 * The list row when the list is not the only thing on screen (GDK-1089).
 *
 * A row is mostly furniture: a checkbox, a priority glyph, a category dot,
 * the key, then a trailing strip of chips. The title is the only part that
 * says which issue this is, and it was the only part that paid for a narrow
 * row — everything else holds a fixed width, so the title absorbs all of
 * the shrink until it hits its CSS floor and stops.
 *
 * Measured on the demo fixture at viewport 1440 before the fix:
 *
 *   panes                       row    title
 *   list only                  1168      638
 *   list + detail               678      172
 *   terminal + list + detail    358      106   ← the 13ch floor, ~12 chars
 *
 * This file is that measurement, kept. It asserts widths rather than text,
 * because the question is layout: a fixture whose first row has a shorter
 * summary would still pass a text assertion while the row was unreadable.
 *
 * The 678px row is deliberately NOT asserted tighter than it measures. Its
 * fix means giving the title a share of the row rather than a constant
 * floor, and 620 is where row-column-thresholds.ts takes over — that module
 * models this very floor as a constant, so a proportional one moves every
 * option column's rung. (Tried below 620, where the ladder is not in
 * charge: it never binds there, and at 570 it pushed the label chips past
 * the scroller — chipfold-labels has a min-width of its own. app.css
 * carries the measurement.) That half is its own issue.
 */

/** Widths of the row's parts, from the first rendered row. */
async function rowMetrics(page: Page): Promise<{
  row: number
  title: number
  checkbox: boolean
}> {
  return page.evaluate(() => {
    const row = document.querySelector('[data-issue-key]') as HTMLElement
    const line = row.querySelector(':scope > div') as HTMLElement
    // `span.flex-1` is the fallback so this measures the same element
    // against a source that predates the .row-title class — a FAIL-first
    // run has to fail on the WIDTH, not on a null selector.
    const title = (line.querySelector('.row-title') ??
      line.querySelector('span.flex-1')) as HTMLElement
    const box = (line.querySelector('.lead-fold-1') ??
      line.querySelector('button[aria-pressed]')) as HTMLElement | null
    return {
      row: Math.round(row.getBoundingClientRect().width),
      title: Math.round(title.getBoundingClientRect().width),
      checkbox: !!box && getComputedStyle(box).display !== 'none',
    }
  })
}

test.describe('narrow list rows', () => {
  test('the title keeps a readable share as panes take the width (GDK-1089)', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await forceLocale(page, 'en')
    await page.goto('/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })

    // One pane: the title has the leftover and the floor never binds.
    const alone = await rowMetrics(page)
    expect(alone.row).toBeGreaterThan(1000)
    expect(alone.title).toBeGreaterThan(500)
    expect(alone.checkbox).toBe(true)

    // Two panes: the detail panel takes half. Measured 678 / 172 — recorded
    // here as the state of play, not as a target (see the header).
    await page.locator('[data-issue-key]').first().click()
    await expect(page.getByTestId('issue-layout')).toHaveAttribute('data-detail-open', 'true')
    // The panel animates in, so the assertion IS the wait: poll the row
    // until it has actually given up the width, then measure that state.
    await expect.poll(async () => (await rowMetrics(page)).row).toBeLessThan(alone.row - 100)
    const withDetail = await rowMetrics(page)
    expect(withDetail.title).toBeGreaterThan(150)

    // Three panes: terminal split as well. The row is ~358px, the trailing
    // folds have already fired, and this is where the title used to sit at
    // its 13ch floor. It must not: the leading strip folds first.
    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    await expect.poll(async () => (await rowMetrics(page)).row).toBeLessThan(500)
    const withTerminal = await rowMetrics(page)
    // 146 measured after the leading fold, 106 before it — the floor itself.
    expect(withTerminal.title).toBeGreaterThan(130)
    expect(withTerminal.checkbox).toBe(false)

    // And it comes back: the fold is a response to width, not a mode.
    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toBeHidden()
    await expect.poll(async () => (await rowMetrics(page)).checkbox).toBe(true)
  })
})
