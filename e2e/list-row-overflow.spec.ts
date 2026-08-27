import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, DEMO_ISSUE_COUNT_EN_RE } from './helpers'

/*
 * GDK-1046: with the detail panel open, no trailing slot may paint past the
 * list scroller's right edge.
 *
 * Why nothing caught this before: list-row-columns.spec.ts proves slots
 * SHARE an x (GDK-128) — a column set can be perfectly aligned and entirely
 * outside the box; narrow-clip.spec.ts proves the 740-closed and the
 * docked-1100-open cells — at 1100 the viewport itself sits below the xl/lg
 * column breaks, so the panel-open 1280/1440 cells (a 570-678px row under a
 * viewport that still says "show everything", updated painting 190px past
 * the scroller, measured 2026-08-27) were covered by no gate. Column
 * visibility is row-width-keyed since GDK-1046 (@container issuerow), so
 * this assertion holds whatever made the row narrow.
 *
 * qa_impact is a default column behind the `qa` feature; e2e/serve.sh's
 * config.json enables only deploy + teamGroups, so without the route
 * override the painted set is one slot short and this gate under-covers
 * (narrow-clip.spec.ts's config.json route idiom).
 */

const COLUMN_SET = 'assignee,updated,labels,reopen,stale,qa_impact,deploy'
const PANEL_ISSUE = 'NMB-110'
// Float slack for deviceScaleFactor subpixel. narrow-clip.spec.ts uses 0.5
// for the same axis; GDK-1046's contract granted 1px.
const PAST_TOLERANCE_PX = 1

async function enableQaColumn(page: Page): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const resp = await route.fetch()
    const json = (await resp.json()) as { features?: Record<string, unknown> }
    json.features = { ...(json.features ?? {}), qa: true }
    await route.fulfill({ json })
  })
}

/** Boot onto an open panel — the shared-link path (cross-links.spec.ts
 *  gotoParams idiom) — and wait for the resting docked grid track
 *  (narrow-clip.spec.ts idiom: the panel width locks immediately, the slide
 *  is 160ms, so wait on the resting track, never a duration). */
async function gotoPanelOpen(page: Page): Promise<void> {
  await page.goto('about:blank')
  await forceLocale(page, 'en')
  await page.goto(`/#/?cl=${COLUMN_SET}&issue=${PANEL_ISSUE}`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(
    page.getByTestId('issue-list-scroller').locator('[data-issue-key]').first(),
  ).toBeVisible({ timeout: 30_000 })
  await expect(
    page.locator('[data-testid="issue-layout"][data-detail-open="true"]'),
  ).toBeVisible()
  await expect
    .poll(async () =>
      page
        .getByTestId('issue-detail-panel')
        .evaluate((el) => Math.round(el.getBoundingClientRect().width)),
    )
    .toBeGreaterThan(400)
}

type PastProbe = {
  rows: number
  rowW: number | null
  past: { key: string; col: string; right: number; scrollerRight: number }[]
}

/** Every visible trailing slot of every in-view row that paints past the
 *  scroller's right edge (beyond the tolerance), plus row liveness counts.
 *  An empty `past` list is the contract. */
async function trailPastScroller(page: Page): Promise<PastProbe> {
  return page.evaluate((tol) => {
    const round = (n: number) => Math.round(n * 10) / 10
    const scroller = document.querySelector<HTMLElement>('[data-testid="issue-list-scroller"]')
    if (!scroller) return { rows: 0, rowW: null, past: [] }
    const sRight = scroller.getBoundingClientRect().right
    const past: { key: string; col: string; right: number; scrollerRight: number }[] = []
    let rows = 0
    let rowW: number | null = null
    for (const row of scroller.querySelectorAll<HTMLElement>('[data-issue-key]')) {
      const r = row.getBoundingClientRect()
      if (r.bottom < 0 || r.top > innerHeight) continue
      rows++
      if (rowW === null) rowW = round(r.width)
      const trail = row.querySelector<HTMLElement>('[data-testid="issue-row-trail"]')
      if (!trail) continue
      for (const slot of [...trail.children] as HTMLElement[]) {
        const s = getComputedStyle(slot)
        if (s.display === 'none' || s.visibility === 'hidden') continue
        const b = slot.getBoundingClientRect()
        if (b.width < 1) continue
        if (b.right > sRight + tol) {
          past.push({
            key: row.dataset.issueKey ?? '',
            col: slot.dataset.col ?? slot.className.toString().slice(0, 40),
            right: round(b.right),
            scrollerRight: round(sRight),
          })
        }
      }
    }
    return { rows, rowW, past }
  }, PAST_TOLERANCE_PX)
}

for (const width of [1280, 1440]) {
  test.describe(`detail panel open @${width}`, () => {
    test.use({ viewport: { width, height: 900 } })

    test('no trailing slot paints past the list scroller', async ({ page }) => {
      const errors = attachConsoleErrors(page)
      await enableQaColumn(page)
      await gotoPanelOpen(page)

      const probe = await trailPastScroller(page)
      // Liveness: the measurement ran over a painted list, not an empty one —
      // otherwise the empty `past` below would pass vacuously.
      expect(probe.rows, 'need several rendered rows in view').toBeGreaterThan(4)

      expect(
        probe.past,
        `rowW=${probe.rowW}: trail slots past the scroller at ${width}: ${JSON.stringify(probe.past)}`,
      ).toEqual([])

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })
  })
}
