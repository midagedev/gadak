import { type Page } from '@playwright/test'
import { test, expect } from './helpers'
import { attachConsoleErrors, gotoApp } from './helpers'

/**
 * The desktop app hides the native title bar, so the macOS window controls land
 * inside the first row of the UI. That row has to reserve their corner and be
 * draggable — and a browser tab, which has neither, must not change at all.
 * Both halves are asserted here because the flag is the only thing separating
 * them: one bundle serves `gadak serve` and the app.
 *
 * Relational only (GDK-1147): the exact numbers (padding 90px/1rem, row 48px,
 * centre line 26px) are the product's to choose and live in app.css's
 * `.desktop-titlebar-row`; what a browser can and should hold is the relation —
 * app content starts at or past where the native buttons end, and a browser
 * tab reserves nothing. The vertical optics (centre line meeting the lights')
 * are a native question measured in capture rounds, not here.
 */

/**
 * Where the native traffic lights end, read off the running window through the
 * accessibility API — buttons at x=18/41/64 in 16px boxes, so 20…78 across;
 * app.css's `.desktop-titlebar-row` comment records the measurement. If the
 * native chrome ever moves, this const moves with it, and only it.
 */
const TRAFFIC_LIGHTS_END_X = 78

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
    // Mark + wordmark. There are no window controls here to confuse it with.
    await expect(row.getByTestId('sidebar-mark')).toBeVisible()
    await expect(row.getByText('gadak', { exact: true })).toBeVisible()
    // Nothing is reserved: the wordmark starts before the lights' line even
    // exists for a browser tab (16px pad + 18px mark + 8px gap ≈ 42px).
    const box = await row.getByText('gadak', { exact: true }).boundingBox()
    expect(box).not.toBeNull()
    expect(box!.x).toBeLessThan(TRAFFIC_LIGHTS_END_X)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the app reserves the window-controls corner and drags the window', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await pretendDesktop(page)
    await gotoApp(page)

    const row = page.getByTestId(LOGO_ROW)
    // The reserve is one corner, not a wholesale re-padding: the left padding
    // clears the lights' line while the right keeps the compact gutter. A
    // revert to plain px-4 fails the first half; a padding applied to both
    // sides (px-4 replaced wholesale) fails the second.
    const styles = await row.evaluate((el) => {
      const s = getComputedStyle(el)
      return { left: s.paddingLeft, right: s.paddingRight }
    })
    expect(parseFloat(styles.left)).toBeGreaterThanOrEqual(TRAFFIC_LIGHTS_END_X)
    expect(parseFloat(styles.right)).toBeLessThan(TRAFFIC_LIGHTS_END_X)

    // Wails reads this custom property to decide what drags the window. With
    // no title bar left, a row that does not carry it strands the window.
    const draggable = await row.evaluate((el) =>
      getComputedStyle(el).getPropertyValue('--wails-draggable').trim(),
    )
    expect(draggable).toBe('drag')

    // Wordmark only: a second small mark beside the traffic lights reads as a
    // fourth button, so the app omits it (the Dock already names the window).
    await expect(row.getByTestId('sidebar-mark')).toHaveCount(0)

    // Content starts at or past where the third button ends — the contract
    // the paddings exist to keep. (Row height and the 26px centre line are
    // native optics, measured in capture rounds against the real lights.)
    const box = await row.getByText('gadak', { exact: true }).boundingBox()
    expect(box).not.toBeNull()
    expect(box!.x).toBeGreaterThanOrEqual(TRAFFIC_LIGHTS_END_X)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
