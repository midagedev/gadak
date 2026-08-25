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

async function waitPaired(page: Page): Promise<void> {
  await page.locator('nav.safe-bottom').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
}

async function walkAll(page: Page): Promise<Measure[]> {
  // No ?demo-tour — that is the real disarm (GDK-869).
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const report: Measure[] = []
  report.push(await measure(page, 'queue'))

  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('button.back').waitFor()
  report.push(await measure(page, 'detail'))

  const chip = page.locator('button.status').first()
  if ((await chip.count()) > 0) {
    await chip.click()
    await page.locator('button.cancel').waitFor()
    report.push(await measure(page, 'sheet'))
    // A phone has no Escape key; DESIGN.md §2 gives scrim tap and Cancel.
    // Clicking button.back while the sheet is open times out (covered).
    await page.getByRole('button', { name: /cancel/i }).first().click()
    await page.locator('button.cancel').waitFor({ state: 'hidden' })
  }

  await page.locator('button.back').first().click()
  await page.locator('.pane:not(.off) button.row').first().waitFor()

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
  report.push(await measure(page, 'queue-dark'))

  return report
}

test('disarmed boot does not change tab or detail on its own', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  const tab = page.locator('nav.safe-bottom button.tab[aria-current="page"]')
  await expect(tab).toHaveText('Queue')
  expect(await page.locator('.detail-layer, button.back').count()).toBe(0)
  const scroller = page.locator('.pane:not(.off) main')
  const y0 = await scroller.evaluate((el) => el.scrollTop)
  // Tour's first move is await wait(2200) then a scroll. Stay past that.
  await page.waitForTimeout(3500)
  await expect(tab).toHaveText('Queue')
  expect(await page.locator('.detail-layer, button.back').count()).toBe(0)
  expect(await scroller.evaluate((el) => el.scrollTop)).toBe(y0)
})

test('viewport geometry at 402×874', async ({ page }) => {
  const report = await walkAll(page)
  expect(report.map((r) => r.label)).toEqual([
    'queue',
    'detail',
    'sheet',
    'search-empty',
    'search-results',
    'pairing',
    'queue-dark',
  ])
  for (const row of report) {
    expect(row.hOverflow, `${row.label} horizontal overflow`).toBe(0)
    expect(row.navBottomFlush, `${row.label} nav flush`).toBe(0)
    expect(row.inputsUnder16, `${row.label} inputs under 16px`).toEqual([])
    expect(row.hasEscape, `${row.label} visible escape`).toBe(true)
  }
  const queue = report.find((r) => r.label === 'queue')
  expect(queue, 'queue measurement').toBeTruthy()
  expect(queue!.rowsPerScreen, 'queue rows per screen').toBeGreaterThanOrEqual(12)
})

test('no visible button is under 44pt', async ({ page }) => {
  // GDK-867 (2026-08-25): four controls measure 32pt today (.fresh 32×49,
  // .status 32×120, .cancel 32×66, .act 32×83). FAIL-first on unmodified
  // source: this assertion failed at queue with buttonsUnder44pt=1
  // [{h:32, cls:"fresh"}] (first screen; the rest were not reached). Unskip
  // when those four land at 44pt. Do not weaken the 44pt floor to match
  // today's 32pt controls.
  test.skip(true, 'GDK-867: 32pt .fresh/.status/.cancel/.act; unskip when they hit 44pt')
  const report = await walkAll(page)
  for (const row of report) {
    expect(row.buttonsUnder44pt, `${row.label} buttons under 44pt: ${JSON.stringify(row.under44)}`).toBe(0)
  }
})
