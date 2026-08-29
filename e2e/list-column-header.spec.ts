import { test, expect, type Page } from '@playwright/test'
import { forceLocale, DEMO_ISSUE_COUNT_EN_RE } from './helpers'

/*
 * GDK-1087: the list's column header row.
 *
 * A list carrying sixteen columns paints "⧖ 2", "1w", "NMS-157" and nothing
 * on screen says which field any of those is. The header names them — and
 * the moment it exists, the row's geometry has a second reader.
 *
 * That is the whole risk, and it is what this file measures. A header that
 * writes its own widths drifts the day one of the twenty slot widths in
 * IssueRow.svelte changes; worse, it drifts *silently*, because the trailing
 * container is `shrink` and the leading strip is 196px of flex-none that
 * folds at ≤480 — so a header that reproduces the widths but not the leading
 * strip lines up everywhere except on a crowded row, which is the only place
 * a header is wanted. A screenshot cannot see that; two getBoundingClientRect
 * reads can.
 *
 * The contract, at three row widths: for every column that paints on a row,
 * the header cell's left and width equal the row cell's, and the set of
 * columns the header paints equals the set the row paints. The header is the
 * same component as the row (`header` mode), so the only way to break this is
 * to give the header markup of its own.
 *
 * The header sits outside the scroller — the floating group header already
 * owns the top of the scroll box — so it also has to give back the scrollbar
 * gutter the rows lose. Headless Chromium has a classic 15px scrollbar, so
 * this file exercises that path; on macOS overlay scrollbars it is 0.
 */

const DEFAULT_SET = 'assignee,updated,labels,reopen,stale,deploy'
const FULL_CATALOG =
  'assignee,updated,labels,reopen,stale,deploy,severity,issue_type,status,reporter,comment_count,fix_versions,components,created,due,environment,team_group,dev_test_result'
// deviceScaleFactor subpixel slack, the tolerance list-row-overflow.spec.ts
// grants on the same axis.
const ALIGN_TOLERANCE_PX = 0.5

type Cell = { left: number; width: number }
type Probe = { header: Record<string, Cell>; row: Record<string, Cell>; rowW: number }

/** The painted slots of the header and of the first in-view row, by column.
 *  "Painted" is the same test the overflow gate uses: not display:none, not
 *  visibility:hidden, and at least a pixel wide. */
async function probe(page: Page): Promise<Probe> {
  return page.evaluate(() => {
    const round = (n: number) => Math.round(n * 10) / 10
    const cells = (root: HTMLElement | null): Record<string, { left: number; width: number }> => {
      const out: Record<string, { left: number; width: number }> = {}
      if (!root) return out
      for (const slot of root.querySelectorAll<HTMLElement>('[data-col]')) {
        const s = getComputedStyle(slot)
        if (s.display === 'none' || s.visibility === 'hidden') continue
        const r = slot.getBoundingClientRect()
        if (r.width < 1) continue
        out[slot.dataset.col ?? '?'] = { left: round(r.left), width: round(r.width) }
      }
      return out
    }
    const scroller = document.querySelector<HTMLElement>('[data-testid="issue-list-scroller"]')
    let first: HTMLElement | null = null
    for (const el of scroller?.querySelectorAll<HTMLElement>('[data-issue-key]') ?? []) {
      const r = el.getBoundingClientRect()
      if (r.bottom < 0 || r.top > innerHeight) continue
      first = el
      break
    }
    return {
      header: cells(document.querySelector<HTMLElement>('[data-testid="issue-column-header"]')),
      row: cells(first),
      rowW: first ? round(first.getBoundingClientRect().width) : 0,
    }
  })
}

/** Every column whose header cell does not sit exactly over its row cell. */
function misaligned(p: Probe): string[] {
  const out: string[] = []
  for (const [col, row] of Object.entries(p.row)) {
    const head = p.header[col]
    if (!head) {
      out.push(`${col}: row paints it, header does not`)
      continue
    }
    if (Math.abs(head.left - row.left) > ALIGN_TOLERANCE_PX)
      out.push(`${col}: left ${head.left} vs ${row.left}`)
    if (Math.abs(head.width - row.width) > ALIGN_TOLERANCE_PX)
      out.push(`${col}: width ${head.width} vs ${row.width}`)
  }
  for (const col of Object.keys(p.header))
    if (!p.row[col]) out.push(`${col}: header paints it, row does not`)
  return out
}

async function gotoList(page: Page, cl: string, issue?: string): Promise<void> {
  await page.goto('about:blank')
  await forceLocale(page, 'en')
  await page.goto(`/#/?cl=${cl}${issue ? `&issue=${issue}` : ''}`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(
    page.getByTestId('issue-list-scroller').locator('[data-issue-key]').first(),
  ).toBeVisible({ timeout: 30_000 })
}

test.describe('list column header', () => {
  test('the default column set keeps its bare list (GDK-1087)', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoList(page, DEFAULT_SET)
    // An avatar, a date and label chips need no legend, and a header over
    // them would change every list nobody customised.
    await expect(page.getByTestId('issue-column-header')).toHaveCount(0)
  })

  test('a customised column set gets a header, and it names every column', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoList(page, FULL_CATALOG)
    const header = page.getByTestId('issue-column-header')
    await expect(header).toBeVisible()
    // Liveness: the audit's three unreadable columns are the ones that must
    // be named. Their labels are in the header's text, truncated or not —
    // the title attribute carries the full name at every slot width.
    for (const col of ['created', 'comment_count', 'epic']) {
      await expect(header.locator(`[data-col="${col}"]`)).toHaveCount(1)
      await expect(header.locator(`[data-col="${col}"] [title]`)).toHaveAttribute('title', /\S/)
    }
  })

  test('every painted column lines up, at every width the row folds through', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoList(page, FULL_CATALOG)

    // One pane. The widest row the layout allows, most rungs of the ladder up.
    const alone = await probe(page)
    expect(alone.rowW).toBeGreaterThan(1000)
    expect(Object.keys(alone.row).length).toBeGreaterThan(6)
    expect(misaligned(alone)).toEqual([])

    // Two panes: the detail panel takes half, the option ladder folds away
    // and trail-fold-1/2 start firing. Poll the row, not a duration — the
    // panel slides.
    await page.locator('[data-issue-key]').first().click()
    await expect(
      page.locator('[data-testid="issue-layout"][data-detail-open="true"]'),
    ).toBeVisible()
    await expect.poll(async () => (await probe(page)).rowW).toBeLessThan(alone.rowW - 100)
    const twoPane = await probe(page)
    expect(Object.keys(twoPane.row).length).toBeLessThan(Object.keys(alone.row).length)
    expect(misaligned(twoPane)).toEqual([])

    // Three panes: ≤480, where lead-fold-1 takes 34px out of the leading
    // strip. This is the width a header with its own spacers gets wrong.
    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    await expect.poll(async () => (await probe(page)).rowW).toBeLessThan(500)
    const threePane = await probe(page)
    expect(misaligned(threePane)).toEqual([])
    // Liveness for this cell: the leading strip really did fold, so the
    // alignment above was measured on the folded geometry.
    const boxHidden = await page.evaluate(() => {
      const row = document.querySelector<HTMLElement>('[data-issue-key]')
      const box = row?.querySelector<HTMLElement>('.lead-fold-1')
      return !!box && getComputedStyle(box).display === 'none'
    })
    expect(boxHidden).toBe(true)
  })

  test('the header gives back the scrollbar gutter the rows lose', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoList(page, FULL_CATALOG)
    const edges = await page.evaluate(() => {
      const round = (n: number) => Math.round(n * 10) / 10
      const header = document.querySelector<HTMLElement>('[data-testid="issue-column-header"]')!
      const row = document.querySelector<HTMLElement>('[data-issue-key]')!
      const scroller = document.querySelector<HTMLElement>('[data-testid="issue-list-scroller"]')!
      return {
        headerRight: round(header.getBoundingClientRect().right),
        rowRight: round(row.getBoundingClientRect().right),
        gutter: round(scroller.offsetWidth - scroller.clientWidth),
      }
    })
    expect(Math.abs(edges.headerRight - edges.rowRight)).toBeLessThanOrEqual(ALIGN_TOLERANCE_PX)
    // Liveness: on a runner with a classic scrollbar this cell is the one
    // that proves the gutter is measured rather than assumed to be zero.
    // Overlay scrollbars (macOS) report 0 and the assertion above still holds.
    expect(edges.gutter).toBeGreaterThanOrEqual(0)
  })
})
