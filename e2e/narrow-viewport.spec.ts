/*
 * GDK-127: with an issue open below 1440px, the detail panel covers the list
 * and row titles are sliced mid-glyph at the panel's edge.
 *
 * The measured cause (2026-08-17, unmodified source): at ≤1439px the layout
 * switches to the overlay regime (`272px minmax(0,1fr)` grid + a
 * `position: fixed` panel), but the list column keeps the FULL remaining
 * width — at 1420 the column is 1148px while the panel covers its last 560px.
 * Row title boxes therefore lay out to the hidden column edge, their text
 * fits those boxes (scrollWidth == clientWidth), `truncate` never engages,
 * and the glyphs past the panel's left edge are cut by the panel itself,
 * with nothing telling the eye that covering is intended.
 *
 * The fix contract, one rule at every width: no title may be clipped without
 * an ellipsis, and the panel may never paint over list text silently. Wide
 * widths keep the docked three-track grid (titles ellipsize at the column
 * edge, which is the panel's edge). Narrow widths are the overlay regime of
 * shape (b): the covering is sanctioned exactly when a scrim is present
 * (the dialog backdrop token), and Esc still closes (keymap: browse → menu
 * → bulk → detail).
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/** 1440/1500 pin the docked regime; 1420/1360/1280 are the overlay regime
 * (1280 is the default logical width of a 13" MacBook — the daily case). */
const DOCKED_WIDTHS = [1500, 1440]
const OVERLAY_WIDTHS = [1420, 1360, 1280]

/** One row's title, with everything the two rules read. */
interface TitleMetric {
  key: string
  right: number
  panelLeft: number
  scrollW: number
  clientW: number
  overflow: string
  /** elementFromPoint at the title's right-edge midpoint: 'row', 'panel',
   * 'scrim', or something else (reported raw). */
  hit: string
}

async function measure(page: Page): Promise<{
  overlay: boolean
  scrim: boolean
  titles: TitleMetric[]
}> {
  return page.evaluate(() => {
    const panel = document.querySelector('[data-testid="issue-detail-panel"]')!
    const scrimEl = document.querySelector('[data-testid="issue-scrim"]')
    const panelLeft = Math.round(panel.getBoundingClientRect().left)
    const overlay = getComputedStyle(panel).position === 'fixed'
    // A scrim that exists but is display:none is not a scrim (the wide regime
    // keeps the element mounted and hidden on purpose).
    const scrim = !!scrimEl && getComputedStyle(scrimEl).visibility === 'visible'

    const scroller = document.querySelector('[data-testid="issue-list-scroller"]')!
    const titles: TitleMetric[] = []
    for (const row of scroller.querySelectorAll<HTMLElement>('[data-issue-key]')) {
      const r = row.getBoundingClientRect()
      if (r.bottom < 0 || r.top > innerHeight) continue // visible rows only
      const t = row.querySelector<HTMLElement>('span.flex-1')
      if (!t) continue
      const tr = t.getBoundingClientRect()
      const hitEl = document.elementFromPoint(tr.right - 2, tr.top + tr.height / 2)
      let hit = 'none'
      if (hitEl) {
        if (scrimEl && (hitEl === scrimEl || scrimEl.contains(hitEl))) hit = 'scrim'
        else if (panel.contains(hitEl)) hit = 'panel'
        else if (hitEl.closest('[data-issue-key]')) hit = 'row'
        else hit = hitEl.className?.toString().slice(0, 40) || hitEl.tagName
      }
      titles.push({
        key: row.dataset.issueKey ?? '',
        right: Math.round(tr.right),
        panelLeft,
        scrollW: t.scrollWidth,
        clientW: t.clientWidth,
        overflow: getComputedStyle(t).textOverflow,
        hit,
      })
    }
    return { overlay, scrim, titles }
  })
}

/**
 * Boot into the audited subject: search "Migrate", then open some OTHER
 * row's panel so every measured row is unpainted (detail.spec precedent —
 * the selected row's background is a second variable).
 */
async function openPanelBesideMigrateRows(page: Page): Promise<void> {
  await gotoApp(page)
  const search = searchInput(page)
  await search.click()
  await search.fill('Migrate')
  const rows = page.locator('[data-testid="issue-list-scroller"] [data-issue-key]')
  await expect(rows.nth(1)).toBeVisible()
  await rows.nth(1).click()
  await expect(
    page.locator('[data-testid="issue-layout"][data-detail-open="true"]'),
  ).toBeVisible()
  await expect(page.getByTestId('issue-detail-panel')).toHaveClass(/is-open/)
  // The panel slides in over 150ms; measure the resting state, not the frame
  // mid-transition.
  await page.waitForTimeout(350)
}

function expectRuleOne(m: { titles: TitleMetric[] }): void {
  /*
   * Rule 1 (task text, verbatim reading): every visible title either ends
   * left of the panel, or its text fits its own box with ellipsis armed —
   * `scrollWidth <= clientWidth` is the "nothing is being cut by the box"
   * branch, not the "ellipsis is drawn" one (that would be the > sign); a
   * title that fits its box cannot lose glyphs to its own truncation.
   */
  expect(m.titles.length, 'the fixture must show rows').toBeGreaterThan(0)
  for (const t of m.titles) {
    const ok =
      t.right <= t.panelLeft + 0.5 ||
      (t.overflow === 'ellipsis' && t.scrollW <= t.clientW)
    expect(ok, `${t.key}: right=${t.right} panelLeft=${t.panelLeft} scrollW=${t.scrollW} clientW=${t.clientW} overflow=${t.overflow}`).toBe(true)
  }
}

function expectRuleTwo(m: { overlay: boolean; scrim: boolean; titles: TitleMetric[] }): void {
  /*
   * Rule 2: the panel never covers title pixels silently. Docked regime —
   * elementFromPoint at each title's right edge must return the title's own
   * row, never the panel. Overlay regime (shape b) — the same point returns
   * the panel or the scrim, which is sanctioned exactly when the scrim is
   * there to say so.
   */
  for (const t of m.titles) {
    if (!m.overlay) {
      expect(t.hit, `${t.key}: title right edge must hit its own row, not the panel`).toBe('row')
    } else {
      expect(m.scrim, `${t.key}: overlay covers titles, so the scrim must be present`).toBe(true)
      expect(['scrim', 'panel'], `${t.key}: hit=${t.hit}`).toContain(t.hit)
    }
  }
}

for (const width of DOCKED_WIDTHS) {
  test.describe(`docked regime ${width}px`, () => {
    test.use({ viewport: { width, height: 900 } })

    test('titles ellipsize at the panel edge; nothing covers them', async ({ page }) => {
      const errors = attachConsoleErrors(page)
      await openPanelBesideMigrateRows(page)

      const m = await measure(page)
      // ≥1440px keeps the three-track grid: the panel is a grid item, not an
      // overlay. If this goes red, the wide regime changed — it must not.
      expect(m.overlay, 'the panel must be docked (grid track), not fixed').toBe(false)
      expectRuleOne(m)
      expectRuleTwo(m)

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })
  })
}

for (const width of OVERLAY_WIDTHS) {
  test.describe(`overlay regime ${width}px`, () => {
    test.use({ viewport: { width, height: 900 } })

    test('the covering panel has a scrim, and Esc closes it', async ({ page }) => {
      const errors = attachConsoleErrors(page)
      await openPanelBesideMigrateRows(page)

      const m = await measure(page)
      expect(m.overlay, 'below 1439px the panel must be the fixed overlay').toBe(true)
      expectRuleOne(m)
      expectRuleTwo(m)

      // Esc still closes (keymap: browse → menu → bulk → detail) — and takes
      // the scrim with it, so the list is not left dimmed behind nothing.
      await page.keyboard.press('Escape')
      await expect(page.getByTestId('issue-detail-panel')).toBeHidden()
      const scrim = page.getByTestId('issue-scrim')
      await expect(scrim).toBeHidden()

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })

    test('j/k still moves the cursor while the panel is open', async ({ page }) => {
      /*
       * The panel is a peek, not a modal: the scrim is visual only
       * (pointer-events: none), so list keyboard navigation must be exactly
       * what it was. This pins the "keys unchanged" clause of the contract.
       */
      const errors = attachConsoleErrors(page)
      await openPanelBesideMigrateRows(page)

      // A click does not place the cursor (triage.spec idiom): the ring is a
      // keyboard thing, so the first j both asserts the list is listening and
      // parks the cursor on a known row.
      const cursor = page.locator('[data-testid="issue-list-scroller"] [data-cursor="true"]')
      await page.keyboard.press('j')
      await expect(cursor).toHaveCount(1)
      const before = await cursor.first().getAttribute('data-issue-key')
      await page.keyboard.press('j')
      await expect(cursor.first()).not.toHaveAttribute('data-issue-key', before!)
      await page.keyboard.press('k')
      await expect(cursor.first()).toHaveAttribute('data-issue-key', before!)

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })
  })
}
