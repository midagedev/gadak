// Viewport geometry gate (GDK-868) at the iPhone 17 Pro layout size the
// lead measured (402×874). Selectors and the sheet-exit trap come from
// scratch/mobile-viewport-probe.mjs. The demo tour is disarmed by omitting
// `?demo-tour` (GDK-869) — do not abort `/__demo-tour__`; that workaround
// existed only while HEAD 200 meant "armed".
import { expect, test, type Page } from '@playwright/test'

type Measure = {
  label: string
  hOverflow: number
  navBottomFlush: number | null
  rowCount: number
  rowH: number | null
  rowsPerScreen: number | null
  inputsUnder16: { tag: string; fs: string }[]
  buttonsUnder44pt: number
  under44: { h: number; cls: string }[]
  hasEscape: boolean
}

async function measure(page: Page, label: string): Promise<Measure> {
  return page.evaluate((label) => {
    const isShown = (el: Element | null): el is Element => {
      if (!el) return false
      const r = el.getBoundingClientRect()
      if (r.width === 0 || r.height === 0) return false
      const s = getComputedStyle(el)
      return s.display !== 'none' && s.visibility !== 'hidden'
    }
    const pane = [...document.querySelectorAll('.pane')].find((p) => p.getBoundingClientRect().height > 0)
    const rows = pane ? [...pane.querySelectorAll('button.row')] : []
    const inputs = [...document.querySelectorAll('input, textarea')]
      .filter((el) => el.getBoundingClientRect().height > 0)
      .map((el) => ({ tag: el.tagName.toLowerCase(), fs: getComputedStyle(el).fontSize }))
    const nav = document.querySelector('nav.safe-bottom')
    const navBox = nav ? nav.getBoundingClientRect() : null
    const main = pane?.querySelector('main')
    const rowH = rows.length ? rows[0].getBoundingClientRect().height : null
    const rowsPerScreen =
      rowH && main && rowH > 0 ? Math.floor(main.clientHeight / rowH) : null
    const under44 = [...document.querySelectorAll('button')]
      .map((b) => {
        const r = b.getBoundingClientRect()
        return { h: r.height, cls: String(b.className).split(' ')[0] }
      })
      .filter((x) => x.h > 0 && x.h < 44)
    return {
      label,
      hOverflow: document.documentElement.scrollWidth - window.innerWidth,
      navBottomFlush: navBox ? window.innerHeight - (navBox.y + navBox.height) : null,
      rowCount: rows.length,
      rowH,
      rowsPerScreen,
      inputsUnder16: inputs.filter((i) => parseFloat(i.fs) < 16),
      buttonsUnder44pt: under44.length,
      under44,
      hasEscape:
        isShown(document.querySelector('nav.safe-bottom')) ||
        [...document.querySelectorAll('button.back')].some(isShown) ||
        [...document.querySelectorAll('button.cancel')].some(isShown),
    }
  }, label)
}

/**
 * Wait for a sheet's rise to finish before measuring it. Svelte's `fly` runs
 * as a CSS animation on a composited layer, and a rect read mid-transform at
 * deviceScaleFactor 3 comes back a hair under the laid-out size — a 44px
 * control measured 43.99993896484375 and failed the floor for a reason that
 * was never on screen. Measuring a settled sheet is the honest reading; the
 * 44pt assertion itself is untouched.
 */
async function settleSheet(page: Page): Promise<void> {
  await page.locator('.sheet').first().evaluate(async (el) => {
    await Promise.all(el.getAnimations().map((a) => a.finished.catch(() => {})))
  })
}

async function waitPaired(page: Page): Promise<void> {
  await page.locator('nav.safe-bottom').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
}

async function walkAll(page: Page): Promise<Measure[]> {
  // No ?demo-tour — that is the real disarm (GDK-869).
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const report: Measure[] = []
  report.push(await measure(page, 'issues'))

  // GDK-885: the heading is the scope control. Open it, measure the sheet,
  // and leave by Cancel — the picker is not a stack and must not be a dead
  // end (DESIGN.md §2).
  await page.locator('.pane:not(.off) h1 button.scope').click()
  await page.locator('button.cancel').waitFor()
  await settleSheet(page)
  report.push(await measure(page, 'scope-sheet'))
  await page.getByRole('button', { name: /cancel/i }).first().click()
  await page.locator('button.cancel').waitFor({ state: 'hidden' })

  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('button.back').waitFor()
  report.push(await measure(page, 'detail'))

  // This used to open the transition sheet and measure it. `gadak demo` is a
  // serve with no origin credential: the transitions GET answers 409
  // credential_required (measured 2026-08-26 — PROBE HTTP 409
  // /api/v1/issues/NMS-134/transitions/). Before GDK-906 the sheet opened
  // anyway and painted that refusal inside itself, so what this step measured
  // was the empty-sheet dead end the fix removed. The control now refuses at
  // the control — it disables and carries the sentence — which is the state
  // this fixture can actually reach, so that is what is measured.
  //
  // Coverage this costs: `.detail-layer .sheet` geometry (the inset owner
  // added in GDK-907) has no measurement on this fixture, because the
  // transition sheet is the only sheet that lives in that layer. Tracked as
  // GDK-911 — do not close it by deleting the assertion.
  const chip = page.locator('button.status').first()
  if ((await chip.count()) > 0) {
    await chip.click()
    await chip.and(page.locator(':disabled')).waitFor()
    report.push(await measure(page, 'detail-writes-off'))
  }

  await page.locator('button.back').first().click()
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  // GDK-887: Updated (whole-mirror) documents plate, then one page detail.
  await page.locator('.pane:not(.off) h1 button.scope').click()
  await page.locator('button.cancel').waitFor()
  await settleSheet(page)
  await page.locator('.sheet .section', { hasText: 'Documents' }).waitFor()
  await page.locator('.sheet button.row', { hasText: 'Updated' }).click()
  await page.locator('button.cancel').waitFor({ state: 'hidden' })
  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().waitFor()
  report.push(await measure(page, 'docs'))

  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().click()
  await page.locator('.page-detail button.back').waitFor()
  report.push(await measure(page, 'page-detail'))
  await page.locator('.page-detail button.back').first().click()
  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().waitFor()

  const tabs = page.locator('nav.safe-bottom button.tab')
  await tabs.nth(1).click()
  await page.locator('.pane:not(.off) input').first().waitFor()
  report.push(await measure(page, 'search-empty'))

  await page.locator('.pane:not(.off) input').first().fill('tenant')
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  report.push(await measure(page, 'search-results'))

  await tabs.nth(2).click()
  await page.getByRole('heading', { name: 'Pairing' }).waitFor()
  report.push(await measure(page, 'pairing'))

  await tabs.nth(0).click()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.emulateMedia({ colorScheme: 'dark' })
  report.push(await measure(page, 'issues-dark'))

  return report
}

test('disarmed boot does not change tab or detail on its own', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  const tab = page.locator('nav.safe-bottom button.tab[aria-current="page"]')
  await expect(tab).toHaveText('Issues')
  expect(await page.locator('.detail-layer, button.back').count()).toBe(0)
  const scroller = page.locator('.pane:not(.off) main')
  const y0 = await scroller.evaluate((el) => el.scrollTop)
  // Tour's first move is await wait(2200) then a scroll. Stay past that.
  await page.waitForTimeout(3500)
  await expect(tab).toHaveText('Issues')
  expect(await page.locator('.detail-layer, button.back').count()).toBe(0)
  expect(await scroller.evaluate((el) => el.scrollTop)).toBe(y0)
})

test('viewport geometry at 402×874', async ({ page }) => {
  const report = await walkAll(page)
  expect(report.map((r) => r.label)).toEqual([
    'issues',
    'scope-sheet',
    'detail',
    'detail-writes-off',
    'docs',
    'page-detail',
    'search-empty',
    'search-results',
    'pairing',
    'issues-dark',
  ])
  for (const row of report) {
    expect(row.hOverflow, `${row.label} horizontal overflow`).toBe(0)
    expect(row.navBottomFlush, `${row.label} nav flush`).toBe(0)
    expect(row.inputsUnder16, `${row.label} inputs under 16px`).toEqual([])
    expect(row.hasEscape, `${row.label} visible escape`).toBe(true)
  }
  const issues = report.find((r) => r.label === 'issues')
  expect(issues, 'issues measurement').toBeTruthy()
  expect(issues!.rowsPerScreen, 'issues rows per screen').toBeGreaterThanOrEqual(12)
  const docs = report.find((r) => r.label === 'docs')
  expect(docs, 'docs measurement').toBeTruthy()
  expect(docs!.rowsPerScreen, 'docs rows per screen').toBeGreaterThanOrEqual(12)
  // GDK-885: opening the picker must not cost the list its density.
  const afterSheet = report.find((r) => r.label === 'issues-dark')
  expect(afterSheet!.rowsPerScreen, 'rows per screen after the picker closed').toBeGreaterThanOrEqual(12)
  // Recurrence: a page row has no status spine (DESIGN.md §3.4).
  expect(await page.locator('[data-testid="doc-row"] .spine').count()).toBe(0)
})

test('no visible button is under 44pt', async ({ page }) => {
  // GDK-867: 44pt floor on every visible button. FAIL-first on unmodified
  // source (2026-08-25): failed at the list screen with buttonsUnder44pt=1
  // [{h:32, cls:"fresh"}] (first screen; the rest were not reached). The
  // four 32pt chips shared --spacing-control-sm as a tap size; the owner
  // is now button { min-height: var(--spacing-control) } in app.css.
  const report = await walkAll(page)
  for (const row of report) {
    expect(row.buttonsUnder44pt, `${row.label} buttons under 44pt: ${JSON.stringify(row.under44)}`).toBe(0)
  }
})
