import { test, expect, type Page } from '@playwright/test'

/**
 * GDK-51 / GDK-55 — hosted first frame must be readable at 390×844.
 *
 * web/index.html pins <meta name="viewport" content="width=1100">. On a
 * 390-wide phone the layout viewport is 1100 CSS px and Chromium shrinks
 * the page so that width fits the device. visualViewport.width stays in
 * *layout* CSS px (~1100); the shrink is visualViewport.scale
 * (≈ 390/1100 ≈ 0.3545). Confirmed on Playwright isMobile 390×844:
 * innerWidth=1101, screen.width=390, visualViewport.scale=0.3545.
 *
 * getComputedStyle(el).fontSize and getBoundingClientRect() are layout
 * CSS px. Visual size on the device:
 *
 *   visualPx = layoutPx * visualViewport.scale
 *
 * A 12px demo banner / 13px list therefore paints at ~4.3 / ~4.6 visual
 * px — unreadable. This spec multiplies measured layout sizes by that
 * scale; it does not read the style="" attribute.
 *
 * Hosted-config only — not part of the CI e2e set.
 */

const DEMO = '/gadak/'

test.use({
  viewport: { width: 390, height: 844 },
  isMobile: true,
  hasTouch: true,
  deviceScaleFactor: 3,
})

type VisualType = {
  fontSize: number
  boxHeight: number
  innerWidth: number
  screenWidth: number
  scale: number
}

async function measureVisualType(page: Page, testId: string): Promise<VisualType> {
  return page.getByTestId(testId).evaluate((el) => {
    const layoutPx = parseFloat(getComputedStyle(el).fontSize)
    const boxHeight = el.getBoundingClientRect().height
    const innerWidth = window.innerWidth
    const screenWidth = window.screen.width
    const scale = window.visualViewport?.scale ?? screenWidth / innerWidth
    return {
      fontSize: layoutPx * scale,
      boxHeight: boxHeight * scale,
      innerWidth,
      screenWidth,
      scale,
    }
  })
}

test.describe('hosted first frame at 390×844', () => {
  test('is in the static HTML and visible without bootstrap.json', async ({ page }) => {
    let bootstrapServed = false
    await page.route('**/bootstrap.json', async (route) => {
      // Hold the snapshot so a pass cannot mean "we waited for the SPA".
      await new Promise((r) => setTimeout(r, 15_000))
      bootstrapServed = true
      await route.abort()
    })

    const started = Date.now()
    await page.goto(DEMO, { waitUntil: 'domcontentloaded' })
    const frame = page.getByTestId('hosted-first-frame')
    await expect(frame).toBeVisible({ timeout: 5_000 })
    await expect(page.getByTestId('hosted-first-frame-claim')).toBeVisible()
    await expect(page.getByTestId('hosted-first-frame-brew')).toBeVisible()
    expect(Date.now() - started, 'frame must paint before the 15s bootstrap hold ends').toBeLessThan(
      8_000,
    )
    expect(bootstrapServed, 'bootstrap.json must still be in flight when the frame is visible').toBe(
      false,
    )
  })

  test('claim and brew lines are ≥ 14 visual CSS px', async ({ page }) => {
    await page.goto(DEMO, { waitUntil: 'domcontentloaded' })
    await expect(page.getByTestId('hosted-first-frame')).toBeVisible()

    const claim = await measureVisualType(page, 'hosted-first-frame-claim')
    const brew = await measureVisualType(page, 'hosted-first-frame-brew')

    // Sanity: the width=1100 pin actually scaled (otherwise we are not
    // measuring the in-app-browser defect). visualViewport.width stays
    // ~1100; the shrink is .scale, matching screen.width / innerWidth.
    expect(claim.innerWidth, 'layout viewport should honour width=1100').toBeGreaterThanOrEqual(1000)
    expect(claim.screenWidth, 'emulated device should be the 390-wide phone').toBeLessThan(500)
    expect(claim.scale).toBeLessThan(0.5)

    expect(
      claim.fontSize,
      `claim visual font-size ${claim.fontSize.toFixed(2)}px (layout inner=${claim.innerWidth} screen=${claim.screenWidth} scale=${claim.scale.toFixed(4)})`,
    ).toBeGreaterThanOrEqual(14)
    expect(
      brew.fontSize,
      `brew visual font-size ${brew.fontSize.toFixed(2)}px (layout inner=${brew.innerWidth} screen=${brew.screenWidth} scale=${brew.scale.toFixed(4)})`,
    ).toBeGreaterThanOrEqual(14)
    expect(claim.boxHeight, 'claim box height must also clear 14 visual px').toBeGreaterThanOrEqual(14)
    expect(brew.boxHeight, 'brew box height must also clear 14 visual px').toBeGreaterThanOrEqual(14)
  })

  test('does not fetch web-demo.mp4 until the poster is tapped', async ({ page }) => {
    const mp4: string[] = []
    page.on('request', (req) => {
      if (/web-demo\.mp4(?:\?|$)/.test(req.url())) mp4.push(req.url())
    })

    await page.goto(DEMO, { waitUntil: 'domcontentloaded' })
    await expect(page.getByTestId('hosted-first-frame')).toBeVisible()
    await expect(page.getByTestId('hosted-first-frame-play')).toBeVisible()
    expect(mp4, 'mp4 must not preload').toEqual([])

    await page.getByTestId('hosted-first-frame-play').click()
    await expect.poll(() => mp4.length, { timeout: 10_000 }).toBeGreaterThan(0)
  })

  test('Open the demo removes the frame and leaves the hosted SPA', async ({ page }) => {
    await page.goto(DEMO, { waitUntil: 'domcontentloaded' })
    await page.getByTestId('hosted-first-frame-open').click()
    await expect(page.getByTestId('hosted-first-frame')).toHaveCount(0)
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await expect(page.getByTestId('demo-banner')).toBeVisible()
  })
})
