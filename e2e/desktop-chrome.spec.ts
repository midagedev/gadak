import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/**
 * The desktop app hides the native title bar, so the macOS window controls land
 * inside the first row of the UI. That row has to reserve their corner and be
 * draggable — and a browser tab, which has neither, must not change at all.
 * Both halves are asserted here because the flag is the only thing separating
 * them: one bundle serves `scry serve` and the app.
 *
 * Geometry only. Whether the traffic lights sit where this reserves space is a
 * native question no browser can answer; the app itself is the check for that.
 */

/** Serve the config the desktop app serves: same document, plus `desktop`. */
async function pretendDesktop(page: Page): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const res = await route.fetch()
    const doc = JSON.parse(await res.text())
    doc.desktop = true
    await route.fulfill({ response: res, body: JSON.stringify(doc) })
  })
}

const LOGO_ROW = 'sidebar-logo-row'

test.describe('desktop title-bar row', () => {
  test('a browser tab keeps the plain row', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const row = page.getByTestId(LOGO_ROW)
    await expect(row).toHaveCSS('padding-left', '16px')
    // Mark + wordmark. There are no window controls here to confuse it with.
    await expect(row.locator('span')).toHaveCount(2)
    // The wordmark starts where the nav below it does — nothing is reserved.
    const box = await row.getByText('scry', { exact: true }).boundingBox()
    expect(box).not.toBeNull()
    expect(box!.x).toBeLessThan(40)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the app reserves the window-controls corner and drags the window', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await pretendDesktop(page)
    await gotoApp(page)

    const row = page.getByTestId(LOGO_ROW)
    // The window controls end at x=78 (measured off the running app through the
    // accessibility API); 90 leaves a gap after them.
    await expect(row).toHaveCSS('padding-left', '90px')
    // px-4 is replaced, not overridden — the right side must survive it.
    await expect(row).toHaveCSS('padding-right', '16px')
    // Full row height stays: the reclaimed space is the title bar, not this.
    await expect(row).toHaveCSS('height', '48px')

    // Wails reads this custom property to decide what drags the window. With
    // no title bar left, a row that does not carry it strands the window.
    const draggable = await row.evaluate((el) =>
      getComputedStyle(el).getPropertyValue('--wails-draggable').trim(),
    )
    expect(draggable).toBe('drag')

    // Wordmark only: the accent square is another small rounded shape on the
    // same baseline as the three buttons, which makes it read as a fourth one.
    await expect(row.locator('span')).toHaveCount(1)

    const box = await row.getByText('scry', { exact: true }).boundingBox()
    expect(box).not.toBeNull()
    expect(box!.x).toBeGreaterThanOrEqual(90)
    // Centre line at 26, matching the buttons' — 24 would be 2px high.
    expect(box!.y + box!.height / 2).toBeCloseTo(26, 0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
