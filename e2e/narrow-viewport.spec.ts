/*
 * GDK-127: with an issue open, no title may be clipped without an ellipsis,
 * and the panel may never paint over list text silently. Covering is
 * sanctioned exactly when a scrim is present.
 *
 * GDK-201 (2026-08-18) revises the split, it does not relax the rule.
 * The 1440px boundary was a fake-modal overlay: the panel covered the list
 * and a visual-only scrim (`pointer-events: none`) promised a dialog the
 * clicks did not honour. Docked floor is now derived:
 *   sidebar 272 + list min 390 + detail min 440 = 1102 → contract 1100px
 * so 1100–1439 is a three-track grid (no scrim), and <1100 is a real modal
 * (scrim click closes, background is inert). See web/src/lib/viewport-regime.ts.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'
import { VIEWPORT_DOCKED_MIN_PX } from '../web/src/lib/viewport-regime'

/*
 * Widths derived from VIEWPORT_DOCKED_MIN_PX (1100), not chosen:
 *   1500 / 1440  pin the previously-docked wide band still docks
 *   1280         13" MacBook default — the daily case, now docked (was overlay)
 *   1120         docked floor + 20
 *   1080         overlay ceiling − 20
 *   960          well below the floor
 */
const DOCKED_WIDTHS = [1500, 1440, 1280, VIEWPORT_DOCKED_MIN_PX + 20]
const OVERLAY_WIDTHS = [VIEWPORT_DOCKED_MIN_PX - 20, 960]

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
  regime: string | null
  titles: TitleMetric[]
}> {
  return page.evaluate(() => {
    const layout = document.querySelector('[data-testid="issue-layout"]')
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
    return {
      overlay,
      scrim,
      regime: layout?.getAttribute('data-viewport-regime') ?? null,
      titles,
    }
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
  // Panel width is locked immediately; opacity/transform slide after it.
  // Wait on the resting width (narrow-clip.spec.ts idiom, GDK-826), not a
  // duration — 350ms was a proxy for "slide finished", and the contract is
  // resting geometry.
  await expect
    .poll(async () =>
      page.locator('[data-testid="issue-detail-panel"]').evaluate((el) =>
        Math.round(el.getBoundingClientRect().width),
      ),
    )
    .toBeGreaterThan(400)
}

function expectRuleOne(m: { titles: TitleMetric[] }): void {
  /*
   * Rule 1 — docked geometry only (GDK-127, kept). Every visible title
   * either ends left of the panel, or its text fits its own box with
   * ellipsis armed. Overlay at <1100 covers the list by construction, so
   * this inequality cannot hold (measured 2026-08-18: at 1080, NMA-152
   * right=686 panelLeft=520 scrollW=396 clientW=228). Silent-cover there
   * is closed by rule 2 + the real-modal assertions, not by this check.
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

async function currentIssueKey(page: Page): Promise<string | null> {
  return page.locator('[data-testid="issue-list-scroller"] [data-issue-key][aria-current="true"]').getAttribute('data-issue-key')
}

for (const width of DOCKED_WIDTHS) {
  test.describe(`docked regime ${width}px`, () => {
    test.use({ viewport: { width, height: 900 } })

    test('titles ellipsize at the panel edge; nothing covers them', async ({ page }) => {
      const errors = attachConsoleErrors(page)
      await openPanelBesideMigrateRows(page)

      const m = await measure(page)
      // ≥1100px keeps the three-track grid: the panel is a grid item, not an
      // overlay. If this goes red, the docked floor moved — it must not.
      expect(m.overlay, 'the panel must be docked (grid track), not fixed').toBe(false)
      expect(m.regime, 'data-viewport-regime is the single owner').toBe('docked')
      expect(m.scrim, 'docked regime has nothing to cover, so no scrim').toBe(false)
      expectRuleOne(m)
      expectRuleTwo(m)

      // GDK-201: the list stays interactive. Click a row that is not the
      // selected one (the selected row is often nth(0) after the list
      // scrolls it into view).
      await expect(page.getByTestId('issue-scrim')).toBeHidden()
      const openKey = await currentIssueKey(page)
      const other = page
        .locator('[data-testid="issue-list-scroller"] [data-issue-key]:not([aria-current="true"])')
        .first()
      await expect(other).toBeVisible()
      const otherKey = await other.getAttribute('data-issue-key')
      expect(otherKey, 'fixture must have a second row').toBeTruthy()
      expect(otherKey).not.toBe(openKey)
      await other.click()
      await expect(
        page.locator(`[data-testid="issue-list-scroller"] [data-issue-key="${otherKey}"]`),
      ).toHaveAttribute('aria-current', 'true')

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })

    test('toolbar controls never paint over each other', async ({ page }) => {
      // GDK-201 vision verdict (2026-08-18): at 1120 the search field's fixed
      // innards (icon, `?`, kbd) shrank past their own width and painted under
      // the palette button. The contract: the `?` help button and the palette
      // opener occupy disjoint boxes at every docked width.
      await openPanelBesideMigrateRows(page)
      const help = await page.getByTestId('search-help').boundingBox()
      const open = await page.getByTestId('palette-open').boundingBox()
      expect(help, 'search-help must render').toBeTruthy()
      expect(open, 'palette-open must render').toBeTruthy()
      const disjoint =
        help!.x + help!.width <= open!.x ||
        open!.x + open!.width <= help!.x ||
        help!.y + help!.height <= open!.y ||
        open!.y + open!.height <= help!.y
      expect(disjoint, 'search help and palette opener overlap').toBe(true)
    })

    test('the palette opener shares the field row — no orphan wrap (GDK-1059)', async ({ page }) => {
      // 2026-08-27 walkthrough: at 1280 with the panel docked, the toolbar's
      // first row ran out of width and the "Search everything ⌘K" button
      // wrapped alone onto its own row. The contract: the opener stays on the
      // field's line at every docked width — compacting to icon+shortcut
      // (SearchBox GDK-1059) instead of wrapping.
      await openPanelBesideMigrateRows(page)
      const help = await page.getByTestId('search-help').boundingBox()
      const open = await page.getByTestId('palette-open').boundingBox()
      expect(help, 'search-help must render').toBeTruthy()
      expect(open, 'palette-open must render').toBeTruthy()
      // Centers, not tops: the opener is h-control and `?` is h-control-sm,
      // so a shared row still offsets the tops by a few px.
      const helpCy = help!.y + help!.height / 2
      const openCy = open!.y + open!.height / 2
      expect(
        Math.abs(openCy - helpCy),
        `palette opener cy=${openCy} vs field row cy=${helpCy}: the button wrapped to an orphan row`,
      ).toBeLessThan(2)
      // And it still sits to the right of the field, not under it.
      expect(open!.x).toBeGreaterThan(help!.x)
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
      expect(m.overlay, 'below 1100px the panel must be the fixed overlay').toBe(true)
      expect(m.regime, 'data-viewport-regime is the single owner').toBe('overlay')
      // Rule 1 is docked-only: overlay covers the list on purpose. Rule 2
      // plus the modal test below are what sanction the covering.
      expectRuleTwo(m)

      // Esc still closes (keymap: browse → menu → bulk → detail) — and takes
      // the scrim with it, so the list is not left dimmed behind nothing.
      await page.keyboard.press('Escape')
      await expect(page.getByTestId('issue-detail-panel')).toBeHidden()
      const scrim = page.getByTestId('issue-scrim')
      await expect(scrim).toBeHidden()

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })

    test('scrim click closes; the list under it does not receive the click', async ({ page }) => {
      /*
       * GDK-201: overlay is a real modal. The list is inert and the scrim
       * is the click target; a click on a row must not change the selection,
       * and a click on the scrim must close the panel.
       */
      const errors = attachConsoleErrors(page)
      await openPanelBesideMigrateRows(page)

      const openKey = await currentIssueKey(page)
      expect(openKey).toBeTruthy()

      const probe = await page.evaluate(() => {
        const row = document.querySelector<HTMLElement>(
          '[data-testid="issue-list-scroller"] [data-issue-key]',
        )
        const main = document.querySelector<HTMLElement>('.issue-main-column')
        const scrimEl = document.querySelector('[data-testid="issue-scrim"]')
        if (!row) return { hit: 'none', mainInert: !!main?.inert }
        const r = row.getBoundingClientRect()
        const el = document.elementFromPoint(r.left + 24, r.top + r.height / 2)
        let hit = 'none'
        if (el) {
          if (scrimEl && (el === scrimEl || scrimEl.contains(el))) hit = 'scrim'
          else if (el.closest('[data-issue-key]')) hit = 'row'
          else hit = el.className?.toString().slice(0, 40) || el.tagName
        }
        return { hit, mainInert: !!main?.inert }
      })
      expect(probe.hit, 'a click on the list must hit the scrim, not a row').toBe('scrim')
      expect(probe.mainInert, 'the list column is inert while the overlay is open').toBe(true)
      expect(await currentIssueKey(page)).toBe(openKey)

      // Left of the panel: sidebar is 272px, so x=300 is list-under-scrim.
      // The scrim element's center sits under the panel and is not clickable.
      await page.getByTestId('issue-scrim').click({ position: { x: 300, y: 400 } })
      await expect(page.getByTestId('issue-detail-panel')).toBeHidden()
      await expect(page.getByTestId('issue-scrim')).toBeHidden()

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })

    test('j/k still moves the cursor while the panel is open', async ({ page }) => {
      /*
       * Overlay is a pointer/AT modal (inert + scrim + aria-modal). The
       * global keymap remains the single owner of keys (browse → menu →
       * bulk → detail), so j/k still moves the list cursor — the same
       * handler that keeps s/a/l/c working on the open issue. Esc still
       * closes.
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
