import { test, expect } from './helpers'
import { gotoApp } from './helpers'

/*
 * GDK-231 regression lock: extra row width belongs to the title.
 *
 * The 2026-08-18 report measured the title rendering at ~105px and cutting at
 * the same place at both 1120 and 1280 — the 160px the wider window added went
 * entirely to the parent-key and label-chip columns, so the row read
 * "Customer attac… / Customer sees…" and was not worth scanning.
 *
 * That is no longer reproducible. Measured on this tree before any change in
 * this round (demo fixture, list alone, first row):
 *
 *   viewport   row    title   labels
 *   1120       848    342     64
 *   1280      1008    502     64
 *   1440      1168    638     76
 *   1680      1360    766    140
 *
 * 1120 → 1280 adds 160px of row and 160px of title: the whole gain, to the
 * pixel. The column work that landed in between is what did it — the trailing
 * strip became fixed-basis slots (GDK-128), the option ladder got a set-aware
 * generator (GDK-1049/1077), and the title became `row-title flex-1` with a
 * 13ch floor rather than a share of the leftovers.
 *
 * So this file is a lock, not a fix: there is no FAIL-first for it on this
 * tree, and it is written to fail if the growth priority is ever inverted
 * again. It asserts the shape of the relationship, not the four numbers —
 * pinning the pixels would break on any deliberate column change and teach
 * the next reader to relax the assertion.
 */

const WIDTHS = [1280, 1440, 1680] as const

async function titleWidth(page: import('@playwright/test').Page): Promise<number> {
  return page.evaluate(() => {
    const row = document.querySelector('[data-issue-key]')
    const title = row?.querySelector('.row-title')
    return title ? title.getBoundingClientRect().width : -1
  })
}

test('a wider window widens the title, monotonically', async ({ page }) => {
  const seen: { width: number; title: number }[] = []
  for (const width of WIDTHS) {
    await page.setViewportSize({ width, height: 900 })
    await gotoApp(page)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    const title = await titleWidth(page)
    expect(title, `no row title measured at ${width}`).toBeGreaterThan(0)
    seen.push({ width, title })
  }

  const table = seen.map((s) => `${s.width} → title ${Math.round(s.title)}`).join('\n')
  for (let i = 1; i < seen.length; i++) {
    expect(
      seen[i].title,
      `title must not shrink or stall as the window grows (GDK-231)\n${table}`,
    ).toBeGreaterThan(seen[i - 1].title)
  }

  // The specific failure the report described: 160px more window, zero more
  // title. Any gain the title takes must be a real share of the gain.
  const gainedWindow = WIDTHS[WIDTHS.length - 1] - WIDTHS[0]
  const gainedTitle = seen[seen.length - 1].title - seen[0].title
  expect(
    gainedTitle,
    `the title took ${Math.round(gainedTitle)}px of ${gainedWindow}px added window\n${table}`,
  ).toBeGreaterThan(gainedWindow * 0.4)
})
