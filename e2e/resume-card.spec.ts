import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

// Where resume-card.png lands: repo scratch by default (gitignored, CI-safe),
// overridable so a round can route the capture to its own session scratchpad
// for the lead's vision review — the same pattern team-flow.spec.ts uses.
const SHOT_DIR = process.env.RESUME_CARD_SHOT_DIR ?? join(dirname(fileURLToPath(import.meta.url)), '../scratch')
const SHOT = join(SHOT_DIR, 'resume-card.png')

/*
 * Resume card e2e (spec w1-resume, Part C). Clause table — the clauses this
 * file owns:
 *
 *   C2 the card exists exactly when a boundary and a delta both exist —
 *      (a) shows it, (b) no visit fields → 0 cards, (c) only-this-open
 *      visit → 0 cards. The absence is the design: no empty state element.
 *   C4 quiet chip form — (a) asserts the button carries the duration-chip
 *      classes (elevated bg, micro secondary text) and no border class.
 *   C5 click scrolls History into view — (a) asserts #detail-history-section
 *      lands at the scroll container's top edge after the click, and the
 *      hover title is the absolute previous-visit time (non-empty).
 *   C6 the rendered line is the en catalog's — (a) asserts the parts come
 *      from the detail response the test itself mocked, counts computed
 *      here from that same response (not from the fixture by hand).
 *
 * FAIL-first: against the pre-round tree (no ResumeCard mount in
 * DetailPanel) case (a) fails at its first assertion — getByTestId
 * ('resume-card') never appears; cases (b)/(c) would wrongly pass (0
 * cards either way), which is why (a) is the gate-bearing case.
 *
 * Visit fields are injected by route-mocking the detail response, not by
 * relying on the server's local.db: other specs in this run open NMB-110
 * first and POST real visits, so the server-side values are whatever the
 * run order left behind. (b)/(c) therefore strip/set the fields explicitly.
 */

/** The boundary (a) diffs from: before all of NMB-110's fixture activity
 *  (history and comments end 2026-07-28), so the card has something to
 *  count. The spec's illustrative "30 days ago" lands after the fixture's
 *  last change and would yield an empty delta — reported to the lead. */
const SINCE = '2026-07-25T00:00:00.000Z'
const DETAIL_ROUTE = '**/api/v1/issues/NMB-110/detail/'

type DetailJSON = {
  history?: Array<{ at: string | null; field: string }>
  comments?: Array<{ created_at: string | null }>
} & Record<string, unknown>

/** Intercept the detail GET and rewrite only the visit fields. The rewritten
 *  document is kept (servedDetail) so the test counts what the page actually
 *  received — page.request would bypass this route, and hand-counting the
 *  fixture is exactly what the count assertions exist to avoid. */
async function mockVisitFields(
  page: Page,
  rewrite: (detail: DetailJSON) => DetailJSON,
): Promise<() => DetailJSON> {
  let servedDetail: DetailJSON | null = null
  await page.route(DETAIL_ROUTE, async (route) => {
    const res = await route.fetch()
    const detail = (await res.json()) as DetailJSON
    const json = rewrite(detail)
    servedDetail = json
    await route.fulfill({ response: res, json })
  })
  return () => {
    if (!servedDetail) throw new Error('detail route was never hit')
    return servedDetail
  }
}

test.describe('resume card', () => {
  test('(a) a previous visit with changes shows the card; click scrolls to History', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const served = await mockVisitFields(page, (detail) => ({
      ...detail,
      last_visited_at: SINCE,
      previous_visit_at: null,
    }))
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    const card = panel.getByTestId('resume-card')
    await expect(card).toBeVisible()

    // Counts come from the response this test mocked — recompute, never
    // hard-code the fixture.
    const detail = served()
    const sinceMs = Date.parse(SINCE)
    const statusN = (detail.history ?? []).filter(
      (h) => h.at !== null && Date.parse(h.at as string) > sinceMs && h.field === 'status',
    ).length
    const commentN = (detail.comments ?? []).filter(
      (c) => c.created_at !== null && Date.parse(c.created_at as string) > sinceMs,
    ).length
    expect(statusN, 'the mocked boundary must cross status changes').toBeGreaterThan(0)
    expect(commentN, 'the mocked boundary must cross comments').toBeGreaterThan(0)

    const text = (await card.textContent()) ?? ''
    expect(text).toContain('Since last opened')
    expect(text).toContain(`${statusN} status change`)
    expect(text).toContain(`${commentN} new comment`)

    // C4: the duration-chip's classes on a native button — no border, no
    // icon, no new colour token.
    const cls = await card.getAttribute('class')
    expect(cls ?? '').toContain('bg-bg-elevated')
    expect(cls ?? '').toContain('text-micro')
    expect(cls ?? '').toContain('text-text-secondary')
    expect(cls ?? '').not.toContain('border')
    // C5: hover states the basis — the absolute time of the visit.
    expect((await card.getAttribute('title')) ?? '').not.toBe('')

    // C5: click scrolls History to the top of the panel's scroll container.
    const scroll = page.getByTestId('detail-scroll')
    await expect(scroll).toBeVisible()
    const history = panel.locator('#detail-history-section')
    await expect(history).toBeVisible()
    // Capture BEFORE the click: after it the panel has scrolled to History
    // and the card is out of frame (vision FIX 2026-09-06, framing not render).
    mkdirSync(SHOT_DIR, { recursive: true })
    await page.screenshot({ path: SHOT })
    await card.click()
    await expect
      .poll(async () => {
        const hs = await history.boundingBox()
        const ss = await scroll.boundingBox()
        return hs && ss ? Math.round(hs.y - ss.y) : null
      })
      .toBeLessThan(48)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('(b) no visit fields → no card, no empty state', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockVisitFields(page, (detail) => {
      const { last_visited_at: _l, previous_visit_at: _p, ...rest } = detail
      return rest
    })
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.locator('#detail-history-section')).toBeVisible()
    await expect(panel.getByTestId('resume-card')).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('(c) the only visit is this open → no card', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await mockVisitFields(page, (detail) => ({
      ...detail,
      last_visited_at: new Date().toISOString(),
      previous_visit_at: null,
    }))
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.locator('#detail-history-section')).toBeVisible()
    await expect(panel.getByTestId('resume-card')).toHaveCount(0)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
