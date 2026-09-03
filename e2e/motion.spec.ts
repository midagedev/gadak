import { test, expect, type Locator, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/**
 * GDK-1341 — the motion layer. Stills cannot judge motion, so the gate counts:
 * every visible button eases (no 0s transition), nothing eases longer than
 * 160ms, a pressed button dips while the pointer is down, and reduced-motion
 * turns all of it off.
 */

async function cssProp(el: Locator, name: string): Promise<string> {
  return el.evaluate((n, p) => getComputedStyle(n).getPropertyValue(p), name)
}

/** Longest duration in a computed transition-duration list, in ms. */
function maxMs(list: string): number {
  return Math.max(
    ...list.split(',').map((d) => {
      const t = d.trim()
      return t.endsWith('ms') ? parseFloat(t) : parseFloat(t) * 1000
    }),
  )
}

async function visibleButtons(page: Page, within: string): Promise<Locator[]> {
  const all = page.locator(`${within} button`)
  const out: Locator[] = []
  for (let i = 0; i < (await all.count()); i++) {
    const b = all.nth(i)
    if (await b.isVisible()) out.push(b)
  }
  return out
}

test.describe('motion layer (GDK-1341)', () => {
  test('every visible button eases, and none for longer than 160ms', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const buttons = [
      ...(await visibleButtons(page, 'aside')),
      ...(await visibleButtons(page, '[data-testid="list-toolbar"]')),
    ]
    expect(buttons.length, 'sample must be wide enough to mean something').toBeGreaterThanOrEqual(20)
    const slow: string[] = []
    const still: string[] = []
    for (const b of buttons) {
      const dur = await cssProp(b, 'transition-duration')
      const ms = maxMs(dur)
      const label = await b.evaluate((n) => n.outerHTML.slice(0, 90))
      if (ms <= 0) still.push(label)
      if (ms > 160) slow.push(`${ms}ms ${label}`)
    }
    expect(still, 'buttons that snap instead of easing').toEqual([])
    expect(slow, 'buttons that ease longer than --motion-slow').toEqual([])

    // The base pair Tailwind's utilities inherit: 100ms, not the stock 150.
    const first = buttons.find(Boolean)!
    expect(maxMs(await cssProp(first, 'transition-duration'))).toBeLessThanOrEqual(100)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a pressed button dips 1px while the pointer is down, and only then', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const btn = page.getByTestId('view-settings')
    await expect(btn).toBeVisible()
    expect(await cssProp(btn, 'translate')).toBe('none')

    const box = (await btn.boundingBox())!
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
    await page.mouse.down()
    expect(await cssProp(btn, 'translate')).toBe('0px 1px')
    await page.mouse.up()
    expect(await cssProp(btn, 'translate')).toBe('none')
    // The click still landed: the view-settings menu is open.
    await expect(page.locator('[data-testid^="column-toggle-"]').first()).toBeVisible()
    await page.keyboard.press('Escape')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('hover-reveal controls fade in rather than appearing', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const row = page.locator('[data-testid="issue-list-scroller"] [role="button"]').first()
    await expect(row).toBeVisible()
    // The multi-select checkbox is the row's first button; it sits at 40%
    // until the row is hovered.
    const box = row.locator('button').first()
    const prop = await cssProp(box, 'transition-property')
    expect(prop, 'row checkbox reveal must transition opacity').toMatch(/opacity|all/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test.describe('reduced motion', () => {
    test('turns every transition and the pressed dip off', async ({ page }) => {
      const errors = attachConsoleErrors(page)
      // Emulated on the page, not through test.use: this suite's page fixture
      // does not carry describe-level context options (measured).
      await page.emulateMedia({ reducedMotion: 'reduce' })
      await gotoApp(page)

      const btn = page.getByTestId('view-settings')
      await expect(btn).toBeVisible()
      expect(
        await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches),
        'the context must be emulating reduced motion',
      ).toBe(true)
      expect(maxMs(await cssProp(btn, 'transition-duration'))).toBe(0)

      const box = (await btn.boundingBox())!
      await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
      await page.mouse.down()
      expect(await cssProp(btn, 'translate')).toBe('none')
      await page.mouse.up()
      await page.keyboard.press('Escape')

      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })
  })
})
