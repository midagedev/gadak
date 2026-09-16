import { expect, test } from './helpers'
import { UI_ORIGIN } from './serve'
import { CLIP_SCALE, CLIP_VIEWPORT } from '../clip.config'
import { openPalette, waitPaired } from './nav'
import type { Browser, Page } from '@playwright/test'

/*
 * The publication camera photographs the product (GDK-1955).
 *
 * mobile/clip.config.ts records the landing clip by making the viewport
 * CLIP_SCALE times the phone's and zooming the document by the same factor,
 * because Playwright's video is a css-pixel capture and a `video.size` above
 * the viewport is padded rather than rendered. That buys resolution, and it
 * spends a premise: that an app laid out at 402x874 and *zoomed* to 3x is
 * the same layout as one laid out at 402x874 and *rendered* at device scale
 * 3. Zoom is not a free multiplier — it is a layout-affecting property, and
 * a sheet that positions itself off visualViewport, a fixed bar, or a
 * percentage-height scroller can all land somewhere else under it. The clip
 * would then show a screen no phone shows, and every frame of it would look
 * perfectly fine.
 *
 * So the two cameras are pointed at the same screens in the same run and
 * their geometry is compared. What is measured is the rectangle of each
 * element that carries the clip's claims — the heading control, the sprint
 * line, the first row, and the palette field, which is the one this gate
 * exists for (mobile/src/lib/inset.ts does arithmetic to place that sheet).
 *
 * FAIL-first, 2026-09-16: with the zoom left at 1 on the recording context
 * (the shape of the bug this camera already had once — the init script's
 * style landed on a document the navigation replaced), every row of the
 * table below is off by a factor of three and the run fails on the first.
 */

const TOLERANCE_CSS_PX = 2

type Boxes = Record<string, { x: number; y: number; w: number; h: number }>

/** The rectangles the clip's beats hold on, in the app's own css px. */
async function measure(page: Page, divideBy: number): Promise<Boxes> {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  if (divideBy !== 1) {
    await page.evaluate((z) => {
      document.documentElement.style.zoom = String(z)
    }, divideBy)
  }
  await waitPaired(page)
  await page.locator('.pane:not(.off) [data-testid="sprint-line"]').waitFor()

  const read = async (selector: string): Promise<{ x: number; y: number; w: number; h: number }> => {
    const box = await page.locator(selector).first().boundingBox()
    if (!box) throw new Error(`clip-camera: ${selector} has no box`)
    return {
      x: Math.round(box.x / divideBy),
      y: Math.round(box.y / divideBy),
      w: Math.round(box.width / divideBy),
      h: Math.round(box.height / divideBy),
    }
  }

  const boxes: Boxes = {
    heading: await read('.pane:not(.off) h1 button.scope'),
    sprintLine: await read('.pane:not(.off) [data-testid="sprint-line"]'),
    firstRow: await read('.pane:not(.off) button.row'),
  }
  await openPalette(page)
  boxes.paletteField = await read('.palette-field input')
  boxes.paletteRow = await read('button.palette-row')
  return boxes
}

async function withContext(
  browser: Browser,
  viewport: { width: number; height: number },
  deviceScaleFactor: number,
  run: (page: Page) => Promise<Boxes>,
): Promise<Boxes> {
  const context = await browser.newContext({
    baseURL: UI_ORIGIN,
    viewport,
    deviceScaleFactor,
    isMobile: true,
    hasTouch: true,
  })
  try {
    return await run(await context.newPage())
  } finally {
    await context.close()
  }
}

test('the clip camera lays the app out exactly as the phone does', async ({ browser }) => {
  // The phone, as the gate and the stills camera see it.
  const phone = await withContext(browser, CLIP_VIEWPORT, CLIP_SCALE, (page) => measure(page, 1))
  // The same app under the clip camera: a viewport CLIP_SCALE times as large,
  // zoomed by CLIP_SCALE, measured back down into the app's own css px.
  const recorded = await withContext(
    browser,
    { width: CLIP_VIEWPORT.width * CLIP_SCALE, height: CLIP_VIEWPORT.height * CLIP_SCALE },
    1,
    (page) => measure(page, CLIP_SCALE),
  )

  const names = Object.keys(phone)
  for (const name of names) {
    console.log(
      `clip-camera: ${name} — phone ${JSON.stringify(phone[name])} recorded ${JSON.stringify(recorded[name])}`,
    )
  }
  for (const name of names) {
    for (const axis of ['x', 'y', 'w', 'h'] as const) {
      expect(
        Math.abs(phone[name][axis] - recorded[name][axis]),
        `${name}.${axis}: the clip camera puts it at ${recorded[name][axis]} css px, the phone at ${phone[name][axis]}`,
      ).toBeLessThanOrEqual(TOLERANCE_CSS_PX)
    }
  }
})
