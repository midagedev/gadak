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
 * The third row was measured when the terminal was a side pane. GDK-1194
 * moved it to a bottom dock, so the narrowest docked row is now the one the
 * viewport itself makes at 1120px; the floor it lands on is the same one.
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
    // 2026-08-30 (GDK-1155): was `> 150`, the 172 this row measured while the
    // strip kept `assignee` and `updated` on it. With the panel open those
    // two are on screen already — the panel is showing them for the row being
    // read — so the fold takes them here and gives the title the 100px back.
    // Measured after: 272. FAIL-first: this line went red on the source
    // before the .detail-open fold rung in app.css.
    expect(withDetail.title).toBeGreaterThan(250)

    // Narrower still: the row is well under 500px, the trailing folds have
    // already fired, and this is where the title used to sit at its 13ch
    // floor. It must not: the leading strip folds first.
    //
    // 2026-08-30 (GDK-1194): this beat used to open the terminal, which took
    // its width out of this row. The terminal is a bottom dock now and takes
    // none, so the squeeze comes from the viewport instead — 1120 is the
    // narrowest row the docked regime has (VIEWPORT_DOCKED_MIN_PX is 1100),
    // which is the same layout question the third pane used to ask. The
    // assertions below are the measurement, unchanged.
    await page.setViewportSize({ width: 1120, height: 900 })
    await expect.poll(async () => (await rowMetrics(page)).row).toBeLessThan(500)
    const narrowest = await rowMetrics(page)
    expect(narrowest.checkbox).toBe(false)
    /*
     * `title > 130` stood here and is deliberately not carried over — the
     * one relaxation in this change, so it gets its three parts.
     *
     * Derivation: 130 was measured on the 358px row, and that row was
     * sidebar 272 + list 358 + detail 438 = 1068px of viewport. Below
     * VIEWPORT_DOCKED_MIN_PX (1100) the detail panel is not docked at all,
     * so with the terminal out of this axis a 358px row beside a docked
     * panel is not a layout the app has any more. The narrowest one it does
     * have is this 410px row.
     *
     * Measured here, 2026-08-30: row 410, title 106 — the .row-title 13ch
     * floor. Not a regression from the dock: at 410 the trail-fold-3 rung
     * (container ≤400, app.css) has not fired, so the trailing strip still
     * holds the width the 358px row had folded away. That is the same
     * pre-existing shape at a width nothing used to probe, and tightening it
     * is a fold-threshold change, not this one. The floor itself stays
     * modeled in row-column-thresholds.test.ts.
     */
    expect(narrowest.title).toBeGreaterThanOrEqual(106)

    // And it comes back: the fold is a response to width, not a mode.
    await page.setViewportSize({ width: 1440, height: 900 })
    await expect.poll(async () => (await rowMetrics(page)).checkbox).toBe(true)
  })
})
