import { expect, test, type Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { writeFileSync } from 'node:fs'
import { CLIP_SCALE, CLIP_VIEWPORT, mediaLocale, mediaLocaleTag } from '../clip.config'

/*
 * The phone exhibit, in motion (GDK-1955). One take, three locales, cut by
 * clip/export-phone-clip.sh into docs/media/phone[.<locale>].mp4 + .gif +
 * phone-poster[.<locale>].png — the bytes the landing plays and the READMEs
 * show.
 *
 * It replaces the three stills (phone-list / phone-palette / phone-detail)
 * on the user's call of 2026-09-16, and the beats are deliberately the same
 * three surfaces in the order a person actually walks them, so nothing the
 * stills said is lost:
 *
 *   list → the same list scrolled → the palette in the heading →
 *   a key typed into it → NMB-110's fields and attachments → back to the list
 *
 * The take ends where it started so the landing's `loop` has no seam.
 *
 * Every beat asserts its state before the camera holds on it. A published
 * clip is the one artifact nobody re-reads before it ships, and the failure
 * mode is not a crash — it is a frame of a half-loaded list that photographs
 * exactly like the product.
 */

/** Where the take is, in seconds, so the log reads as a shot list. */
let t0 = 0
const beats: string[] = []

function mark(label: string): void {
  beats.push(`${((Date.now() - t0) / 1000).toFixed(1)}s ${label}`)
}

/**
 * Settle, then hold. Same two-pass animation wait as the stills camera
 * (mobile/media/phone.spec.ts): some animations start as others finish, so
 * one getAnimations() pass is not enough. Here it also keeps the camera from
 * moving on while a fly() is still in the air — a viewer sees the smear the
 * still camera would have photographed.
 */
async function hold(page: Page, ms: number, label: string): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  await page.waitForTimeout(250)
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  mark(label)
  await page.waitForTimeout(ms)
}

/*
 * Scroll the way a thumb does, not the way scrollIntoView does. A jump is a
 * single frame in the recording and reads as a cut; the claim this clip makes
 * is that the phone is the same cache *navigated*, which needs the motion to
 * be visible. Eased, ~16ms a step, and it returns the distance it actually
 * moved so the assertion after it measures the scroll instead of assuming it.
 */
async function glide(page: Page, selector: string, distance: number, ms: number): Promise<number> {
  return page.evaluate(
    async ({ selector, distance, ms }) => {
      const el = document.querySelector(selector)
      if (!(el instanceof HTMLElement)) return 0
      const from = el.scrollTop
      const start = performance.now()
      await new Promise<void>((done) => {
        const step = (now: number) => {
          const p = Math.min(1, (now - start) / ms)
          // easeInOutQuad: the thumb starts and stops, it does not teleport.
          const e = p < 0.5 ? 2 * p * p : 1 - (-2 * p + 2) ** 2 / 2
          el.scrollTop = from + distance * e
          if (p < 1) requestAnimationFrame(step)
          else done()
        }
        requestAnimationFrame(step)
      })
      return Math.round(el.scrollTop - from)
    },
    { selector, distance, ms },
  )
}

/** The list's scroller — the pane that is not pushed off to the side. */
const LIST = '.pane:not(.off) main, .pane:not(.off) .scroll'

/*
 * The resolution lives in the layout, not in the capture: the viewport is
 * CLIP_SCALE times the phone's and the document is zoomed by the same
 * factor, so the app is back at its own 402x874 while every glyph is painted
 * three times as large (clip.config.ts says why). The cost is that the page
 * now answers in two coordinate spaces: getBoundingClientRect is in zoomed
 * px, clientHeight and scrollTop in layout px. So no distance below is a
 * constant — each is a fraction of something the page just measured, and the
 * one place the two spaces meet divides by the zoom, out loud.
 */
async function zoomToScale(page: Page): Promise<void> {
  // Applied after the navigation, not from an init script: an init script
  // runs against the document that the navigation then replaces, so the
  // style lands on a document nobody ever sees (measured 2026-09-16 —
  // computed zoom read back as "1" and the take was phone-sized again).
  await page.evaluate((z) => {
    document.documentElement.style.zoom = String(z)
  }, CLIP_SCALE)
}

test(`phone clip (${mediaLocale})`, async ({ page }, testInfo) => {
  t0 = Date.now()

  // The locale travels both roads the app reads it by, exactly as the stills
  // camera sends it (localStorage first, navigator.language second), and the
  // assertion below is the proof it arrived: a locale that never reached the
  // app reads back as the en-US default.
  await page.addInitScript((code) => localStorage.setItem('gadak_locale', code), mediaLocale)

  /* ── beat 1: the list ── */
  // No ?demo-tour: the tour would move the app out from under the camera
  // (GDK-869).
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await zoomToScale(page)
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  // The active sprint's line is part of what this frame says — it is the
  // whole of what the phone-list still was there for.
  await page.locator('.pane:not(.off) [data-testid="sprint-line"]').waitFor()

  const lang = await page.evaluate(() => document.documentElement.lang)
  expect(lang, `<html lang> after the first navigation (locale ${mediaLocale})`).toBe(
    mediaLocaleTag,
  )
  const rows = await page.locator('.pane:not(.off) button.row').count()
  expect(rows, 'the opening frame must be a populated list').toBeGreaterThan(4)

  // The zoom has to have landed, or the take is a phone-sized app in the
  // corner of a 1206x2622 frame — which is exactly what the padded-capture
  // version of this rig wrote, silently, before a vision round on the
  // rendered page caught it. The layout viewport is the recorded surface;
  // divided by the zoom it must be the phone's own size, which is the same
  // statement as "the app is laid out as a phone and painted three times as
  // large".
  const layout = await page.evaluate(() => ({
    zoom: Number(getComputedStyle(document.documentElement).zoom),
    surfaceWidth: document.documentElement.clientWidth,
    surfaceHeight: document.documentElement.clientHeight,
  }))
  const appWidth = Math.round(layout.surfaceWidth / layout.zoom)
  const appHeight = Math.round(layout.surfaceHeight / layout.zoom)
  console.log(
    `media-phone-clip: locale ${mediaLocale} — <html lang> "${lang}", ${rows} rows, ` +
      `surface ${layout.surfaceWidth}x${layout.surfaceHeight} at zoom ${layout.zoom} = app ${appWidth}x${appHeight}`,
  )
  expect(layout.zoom, 'the zoom did not land — the take would be a 1x phone').toBe(CLIP_SCALE)
  expect(appWidth, 'the app is not laid out at the phone width').toBe(CLIP_VIEWPORT.width)
  expect(appHeight, 'the app is not laid out at the phone height').toBe(CLIP_VIEWPORT.height)
  await hold(page, 1600, 'list')

  /* ── beat 2: the same list, scrolled ── */
  // Six tenths of a screen: enough rows to pass that the list is real, not
  // so many that the eye loses its place. A fraction, not a px constant —
  // the coordinate space is zoomed.
  const step = await page.evaluate((sel) => {
    const el = document.querySelector(sel)
    return el instanceof HTMLElement ? Math.round(el.clientHeight * 0.6) : 0
  }, LIST)
  const moved = await glide(page, LIST, step, 900)
  expect(moved, 'the list did not scroll — the scroller selector has moved').toBeGreaterThan(
    step * 0.5,
  )
  await hold(page, 900, `list scrolled ${moved}px`)
  await glide(page, LIST, -moved, 700)
  await hold(page, 400, 'list back at the top')

  /* ── beat 3: the palette, where the tab bar is not ── */
  await page.locator('.pane:not(.off) h1 button.scope').click()
  const field = page.locator('.palette-field input')
  await field.waitFor()
  await expect(page.locator('nav.safe-bottom'), 'there is no tab bar to photograph').toHaveCount(0)
  await hold(page, 1400, 'palette open')

  /* ── beat 4: a key typed into it ── */
  // Typed rather than filled: the beat is the search happening, and `fill`
  // is one frame. NMB-110 is the stills camera's issue — a label, a
  // component, a parent and three image attachments, which is what makes
  // the detail frame worth holding on.
  await field.pressSequentially('NMB-110', { delay: 120 })
  const hit = page.locator('.pane:not(.off) button.row', { hasText: 'NMB-110' }).first()
  await hit.waitFor()
  await hold(page, 900, 'NMB-110 found')

  /* ── beat 5: the issue ── */
  await hit.click()
  await page.locator('.detail-layer button.back').waitFor()
  // Assert the issue before the camera holds on it: a stale list or a
  // mis-keyed search must not become the published picture of "the detail
  // screen".
  await expect(page.locator('.detail-layer .bar-key')).toHaveText('NMB-110')
  await page.locator('[data-testid="detail-fields"]').waitFor()
  await hold(page, 1300, 'NMB-110 detail, top')

  // Down to Fields and the attachment grid. Three thumbnails is also the
  // check that this serve hands out attachment bytes: a fixture copy without
  // its attachments directory demotes every image to a ledger row, and the
  // clip would photograph that failure as if it were the product.
  await expect(page.locator('[data-testid="detail-attachments-count"]')).toHaveText('3')
  // Two coordinate spaces meet here. getBoundingClientRect is in *zoomed* px
  // (the recorded surface) while scrollTop is in layout px, so a rect delta
  // handed straight to a scroller overshoots by the zoom factor and the
  // glide slams into the bottom before its easing is done. Divide once,
  // here, where the two meet.
  const fieldsTop = await page.evaluate(() => {
    const scroller = document.querySelector('.detail-layer main')
    const fields = document.querySelector('[data-testid="detail-fields"]')
    if (!(scroller instanceof HTMLElement) || !fields) return 0
    const zoom = Number(getComputedStyle(document.documentElement).zoom) || 1
    const delta = fields.getBoundingClientRect().top - scroller.getBoundingClientRect().top
    return Math.round(delta / zoom)
  })
  const detailMoved = await glide(page, '.detail-layer main', fieldsTop, 1200)
  console.log(
    `media-phone-clip: detail glide — fields were ${fieldsTop}px below the fold, moved ${detailMoved}px`,
  )
  await expect(page.locator('[data-testid="attachment-thumb"] img')).toHaveCount(3)
  // The grid has to be inside the frame at the moment the camera holds, or
  // the beat holds on a scroll position that shows neither block whole.
  const gridInFrame = await page.evaluate(() => {
    const scroller = document.querySelector('.detail-layer main')
    const grid = document.querySelector('[data-testid="detail-attachments"]')
    if (!(scroller instanceof HTMLElement) || !grid) return null
    const s = scroller.getBoundingClientRect()
    const g = grid.getBoundingClientRect()
    return { top: Math.round(g.top - s.top), bottom: Math.round(g.bottom - s.top), frame: Math.round(s.height) }
  })
  console.log(`media-phone-clip: attachment grid ${JSON.stringify(gridInFrame)}`)
  expect(gridInFrame, 'the attachment grid is not in the held frame').not.toBeNull()
  expect(gridInFrame!.top, 'the attachment grid starts below the fold').toBeLessThan(
    gridInFrame!.frame,
  )
  await hold(page, 1800, 'fields and attachments')

  /* ── beat 6: back to where the take started ── */
  await page.locator('.detail-layer button.back').first().click()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  // The query goes with it: the last frame has to be the first frame, or the
  // loop cuts from a search result back to the full list in one step.
  const cancel = page.locator('button.palette-cancel')
  if ((await cancel.count()) > 0) {
    await cancel.click()
    await page.locator('.palette-field input').waitFor({ state: 'detached' })
  }
  await page.locator('.pane:not(.off) [data-testid="sprint-line"]').waitFor()
  const closingRows = await page.locator('.pane:not(.off) button.row').count()
  expect(closingRows, 'the take must close on the list it opened on').toBeGreaterThan(4)
  await hold(page, 1500, `back on the list (${closingRows} rows)`)

  /* ── provenance ── */
  console.log(`media-phone-clip: shot list — ${beats.join(' · ')}`)
  const head = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
  const porcelain = execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim()
  console.log(
    `media-phone-clip: recorded at ${head} (${porcelain === '' ? 'clean tree' : 'dirty tree'})`,
  )

  // Written beside the video, and read by export-phone-clip.sh before it
  // names an output file. A take says which language it is in; the exporter
  // takes the name from an environment variable, and on 2026-09-16 the two
  // disagreed for one run — a Korean take was written out as phone.mp4 and
  // every gate stayed green, because nothing in the pipeline had ever been
  // asked to say what was in the frames. That is the same class of defect as
  // the English poster that shipped under two locales (GDK-1836).
  writeFileSync(
    testInfo.outputPath('take.json'),
    JSON.stringify({ locale: mediaLocale, lang, rows, head, beats }, null, 2),
  )
})
