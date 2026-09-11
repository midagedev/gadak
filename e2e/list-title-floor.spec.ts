import { type Page } from '@playwright/test'
import { test, expect } from './helpers'
import { forceLocale, gotoApp } from './helpers'

/*
 * GDK-1791: the title's comfort floor, at the widths where it used to break.
 *
 * The row's always-on strip (reopen, stale, deploy, labels, assignee,
 * updated) holds fixed widths, so every pixel a narrowing window took came
 * off the title — the one element that says which issue the row is.
 * Measured on this tree before the change (demo fixture, en):
 *
 *   viewport   row    title   truncated (top 6)
 *   1440      1168      564         0/6
 *   1000       728      222         6/6
 *    800       592      166         6/6
 *
 * 30% less window, 60% less title, and ~160px of strip sitting to its right
 * with room to spare. The fix prices the strip's fold rungs against the
 * title instead of against overflow (row-column-thresholds.ts,
 * alwaysOnFoldLadder): below a rung a level of the strip drops whole — the
 * rule the file already had for the option columns.
 *
 * This file is the contract. It asserts the floor in the title's OWN font
 * (a ch probe, not a pixel count measured on one machine — CI's Linux face
 * is narrower and run 33318658199 went red on exactly that mistake in
 * row-narrow.spec.ts), and it asserts that 1440 keeps the full strip, so
 * "fold everything always" cannot pass it.
 */

/** 36ch in the title's own face — TITLE_COMFORT_PX models this at 293px. */
async function comfortFloorPx(page: Page): Promise<number> {
  return page.evaluate(() => {
    const title = document.querySelector('.row-title') as HTMLElement
    const probe = document.createElement('span')
    probe.style.cssText = 'position:absolute;visibility:hidden;white-space:pre'
    probe.style.font = getComputedStyle(title).font
    probe.textContent = '0'.repeat(36)
    document.body.appendChild(probe)
    const w = probe.getBoundingClientRect().width
    probe.remove()
    return w
  })
}

async function rowProbe(page: Page): Promise<{
  row: number
  titles: number[]
  slots: string[]
}> {
  return page.evaluate(() => {
    const scroller = document.querySelector('[data-testid="issue-list-scroller"]') as HTMLElement
    const rows = [...scroller.querySelectorAll<HTMLElement>('[data-issue-key]')].slice(0, 6)
    const first = rows[0]
    return {
      row: Math.round(first.getBoundingClientRect().width),
      titles: rows.map((r) =>
        Math.round((r.querySelector('.row-title') as HTMLElement).getBoundingClientRect().width),
      ),
      slots: [...first.querySelectorAll<HTMLElement>('[data-col]')]
        .filter((el) => getComputedStyle(el).display !== 'none')
        .map((el) => el.getAttribute('data-col') as string),
    }
  })
}

test('the strip yields before the title does (GDK-1791)', async ({ page }) => {
  const report: string[] = []

  for (const width of [1440, 1000, 800] as const) {
    await page.setViewportSize({ width, height: 900 })
    await forceLocale(page, 'en')
    await gotoApp(page)
    await expect(
      page.getByTestId('issue-list-scroller').locator('[data-issue-key]').first(),
    ).toBeVisible({ timeout: 30_000 })

    const floor = await comfortFloorPx(page)
    const probe = await rowProbe(page)
    const narrowest = Math.min(...probe.titles)
    report.push(
      `${width}: row ${probe.row} titles ${probe.titles.join('/')} floor ${floor.toFixed(1)} slots [${probe.slots.join(' ')}]`,
    )

    // 0.95: the floor is a ch quantity and the model that prices the rungs is
    // px, so allow one face's worth of disagreement — not a whole column's.
    expect(
      narrowest,
      `the narrowest of the top six titles is under the 36ch floor at ${width}px\n${report.join('\n')}`,
    ).toBeGreaterThanOrEqual(Math.floor(floor * 0.95))

    // `stale` is the fact a narrow row keeps: it never folds.
    expect(probe.slots, `stale must survive at ${width}px\n${report.join('\n')}`).toContain('stale')

    if (width === 1440) {
      // …and the other direction: a wide row is not allowed to buy the floor
      // by folding. Every always-on slot is on the 1168px row.
      for (const col of ['reopen', 'deploy', 'labels', 'assignee', 'updated']) {
        expect(probe.slots, `${col} must stay at 1440px\n${report.join('\n')}`).toContain(col)
      }
    }
  }

  console.log(`[GDK-1791]\n${report.join('\n')}`)
})
