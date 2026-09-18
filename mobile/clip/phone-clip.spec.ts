import { expect, test, type Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { writeFileSync } from 'node:fs'
import { CLIP_SCALE, CLIP_VIEWPORT, mediaLocale, mediaLocaleTag } from '../clip.config'

/*
 * The comment the take types, per UI language (GDK-1962). Written by hand in
 * each language rather than translated from the English, because the beat is
 * a person writing on their own phone and a translated sentence reads like a
 * translated sentence. It says only what the fixture supports — no version
 * number, no release name: a clip must not assert facts the data behind it
 * does not have.
 */
const COMMENT: Record<string, string> = {
  en: 'Reproduced on my side — screenshot from right after the upgrade.',
  ko: '저도 재현됩니다. 업그레이드 직후 화면 첨부합니다.',
  ja: 'こちらでも再現しました。アップグレード直後の画面です。',
}

/*
 * The photo the take attaches: a PNG held here rather than committed, for the
 * reason e2e/attach.spec.ts gives — a binary fixture is one more thing to
 * keep in sync with nothing.
 *
 * It is shaped like what the beat claims it is: a 360x780 phone screenshot,
 * dark, with a header, an error band and a column of rows. The first version
 * was a 1x1 pixel, which the grid stretched into a flat coloured rectangle on
 * a tile taller than its three neighbours — a vision round on the rendered
 * clip called it out, and it was right: an attachment beat whose attachment
 * does not look like an attachment is the beat failing.
 *
 * It carries no glyphs on purpose, which also keeps it out of GDK-1957's way
 * (the snapshot's own attachment images are English pictures under every
 * locale; this one says the same thing in all three).
 */
const SHOT_PNG = Buffer.from(
    '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' +
    'OADhADjjPwphkFI3m9eaAAAAAElFTkSuQmCC',
    'base64',
)

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

/*
 * What the take actually wrote, read back from the app after each write —
 * not from the request. It goes into take.json, and the exporter refuses a
 * take that claims writes it cannot evidence: a clip whose selling point is
 * "the phone writes" must not be filmable against a serve that quietly
 * refused every one of them (the class of defect that shipped a clip ending
 * on "No issues match" for four days).
 */
const writes: string[] = []

function mark(label: string): void {
  beats.push(`${((Date.now() - t0) / 1000).toFixed(1)}s ${label}`)
}

/*
 * Where the README's cut opens (GDK-1962).
 *
 * The landing plays the whole take; a GitHub README plays a GIF, and a GIF of
 * this whole take does not fit the budget at a frame rate that keeps typed
 * prose legible — measured on the first write take: 28.7s came out at
 * 4.59 MB at fps 9 / 360px, and the exporter's ladder dropped it to fps 8 and
 * 64 colours to get under 4 MB, which is exactly the setting that costs the
 * comment beat its letters. So the two artifacts say different things on
 * purpose: the GIF's job is "what is this", the video's is "watch it all".
 *
 * The cut is named here rather than in the exporter because the narrative is
 * the take's business: it opens on the issue already on screen, so the writes
 * that follow have something to be about. The number is seconds from the
 * start of the recording, which is the start of this test to within the
 * settle before the first beat — a fifth of a second either way, invisible
 * across a fourteen-second cut.
 */
let gifFrom = 0
function markGifCut(): void {
  gifFrom = (Date.now() - t0) / 1000
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
  // The top of the list, remembered: the take writes to an issue partway
  // through, and the pool is ordered by how recently each issue changed, so
  // "does the last frame still show what the first frame showed" is a
  // question about this row and not about the row count (GDK-1962).
  const openingTop = (await page.locator('.pane:not(.off) button.row').first().innerText()).trim()

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
  await page.locator('.pane:not(.off) button.search').click()
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
  // The README's cut opens on the frame after this hold: the issue is on
  // screen with its fields and its attachments, and everything after it is a
  // write.
  markGifCut()

  /* ── beat 6: the status moves ── */
  // Everything up to here was reading, which is the half a still could also
  // have shown. These three beats are the half it could not: the phone
  // writes, and the writes go to the origin (GDK-1962). The rig serves
  // `gadak demo --writable` for exactly this — on the read-only demo every
  // control below ships disabled and the camera would film grey buttons.
  const statusChip = page.locator('.composer-slab button.status')
  await expect(statusChip, 'the status control is disabled — the serve is not writable').toBeEnabled()
  const statusBefore = (await statusChip.innerText()).trim()
  await statusChip.click()
  const inProgress = page.locator('button.t-row').filter({ has: page.locator('.dot-inprogress') }).first()
  await inProgress.waitFor()
  await hold(page, 1100, 'transitions open')
  await inProgress.click()
  // The sheet closes when the write lands, so its absence is the receipt —
  // and the chip has to actually say something new, or the beat filmed a
  // click that did nothing.
  await page.locator('button.t-row').first().waitFor({ state: 'detached' })
  await expect
    .poll(async () => (await statusChip.innerText()).trim(), {
      message: 'the status chip never changed — the transition did not land',
    })
    .not.toBe(statusBefore)
  const statusAfter = (await statusChip.innerText()).trim()
  writes.push(`status ${JSON.stringify(statusBefore)} → ${JSON.stringify(statusAfter)}`)
  // The status the clip films has to be in the clip's language. It is not
  // the fixture's job here: the writable rig serves the snapshot through the
  // built-in tracker, which stores ids and overlays the display names by the
  // workspace's own locale — so a target left at the default prints
  // "Backlog" and "In Progress" over Korean prose while the ko fixture two
  // directories away carries 백로그 and 진행 중 (measured 2026-09-16, the
  // first ko write take; GDK-1561 named the mechanism, this is what it looks
  // like on a published surface). ko and ja status names are wholly CJK in
  // both fixtures, so "no Latin letters" is the exact test and needs no
  // table of expected strings to keep in step with the translations.
  if (mediaLocale !== 'en') {
    for (const [name, text] of [
      ['before', statusBefore],
      ['after', statusAfter],
    ] as const) {
      expect(
        text,
        `the ${name} status reads as English in the ${mediaLocale} take — the serve's workspace locale did not reach the display-name overlay (make media-phone-clip passes GADAK_MOBILE_API_LOCALE)`,
      ).not.toMatch(/[A-Za-z]/)
    }
  }
  await hold(page, 1200, `status now ${statusAfter.replace(/\s+/g, ' ')}`)

  /* ── beat 7: a comment, typed ── */
  const composer = page.locator('.composer input')
  await expect(composer, 'the comment field is disabled — the serve is not writable').toBeEnabled()
  const commentsBefore = await page.locator('.detail-layer .comment').count()
  // Typed, not filled: the beat is a person writing, and `fill` is one frame.
  // Slower than the palette's key, because prose reads slower than a key.
  await composer.pressSequentially(COMMENT[mediaLocale] ?? COMMENT.en, { delay: 55 })
  await hold(page, 700, 'comment typed')

  /* ── beat 8: the photo goes with it ── */
  // The picker is `hidden` by design (ui/AttachChips.svelte: the viewport
  // gate counts every input that paints), so the file is set on the input
  // rather than by clicking a control that opens a native sheet no recorder
  // can film.
  await page.locator('.composer-slab input.file').setInputFiles({
    name: 'after-upgrade.png',
    mimeType: 'image/png',
    buffer: SHOT_PNG,
  })
  await page.locator('[data-testid="composer-attachments"]').waitFor()
  await hold(page, 900, 'photo attached')

  const send = page.locator('.composer button.send')
  await expect(send, 'Send never armed — the composer does not think it can send').toBeEnabled()
  await send.click()
  // The comment count is the receipt. Polled rather than waited on a
  // spinner: the upload finishes when the origin says so, and the clip must
  // hold on the result, never on the request.
  await expect
    .poll(async () => page.locator('.detail-layer .comment').count(), {
      message: 'the comment never appeared in the thread — the write did not land',
      timeout: 30_000,
    })
    .toBeGreaterThan(commentsBefore)
  await expect(composer, 'the composer kept the text — the send did not complete').toHaveValue('')
  writes.push(`comment ${commentsBefore} → ${await page.locator('.detail-layer .comment').count()}`)

  // Show the comment, not just the empty box it left. A vision round on the
  // first write take found the viewport frozen on Fields from 12s to 27s:
  // every write landed, and the only on-screen proof of the last one was the
  // composer clearing. The thread is where a reader looks for a comment, so
  // the camera goes there — the same coordinate-space divide as the glide
  // above, at the one place the two spaces meet.
  const threadTop = await page.evaluate((phoneHeight) => {
    const scroller = document.querySelector('.detail-layer main')
    const last = [...document.querySelectorAll('.detail-layer .comment')].pop()
    if (!(scroller instanceof HTMLElement) || !last) return 0
    const zoom = Number(getComputedStyle(document.documentElement).zoom) || 1
    const delta = last.getBoundingClientRect().top - scroller.getBoundingClientRect().top
    // Leave the comment a third of a screen down rather than flush at the
    // top: the beat is "it landed in the thread", which needs the row above
    // it in frame to read as a thread at all.
    return Math.round(delta / zoom) - Math.round(phoneHeight / 3)
  }, CLIP_VIEWPORT.height)
  await glide(page, '.detail-layer main', threadTop, 900)
  const posted = await page.evaluate(() => {
    const scroller = document.querySelector('.detail-layer main')
    const last = [...document.querySelectorAll('.detail-layer .comment')].pop()
    if (!(scroller instanceof HTMLElement) || !last) return null
    const s = scroller.getBoundingClientRect()
    const r = last.getBoundingClientRect()
    return { top: Math.round(r.top - s.top), bottom: Math.round(r.bottom - s.top), frame: Math.round(s.height) }
  })
  console.log(`media-phone-clip: posted comment in frame ${JSON.stringify(posted)}`)
  expect(posted, 'the posted comment is not on screen').not.toBeNull()
  expect(posted!.top, 'the posted comment sits below the fold — the beat holds on nothing').toBeLessThan(
    posted!.frame,
  )
  expect(posted!.bottom, 'the posted comment sits above the frame').toBeGreaterThan(0)
  await hold(page, 2200, 'comment posted with its photo')

  /* ── beat 9: back to where the take started ── */
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
  const closingTop = (await page.locator('.pane:not(.off) button.row').first().innerText()).trim()
  // The loop's real contract, stated where it can be read rather than left
  // to the exporter's SSIM floor to discover: the landing plays this on
  // repeat, so the frame it ends on has to be the frame it began on. A write
  // that reorders the list breaks that, and the log says so here instead of
  // in an ffmpeg refusal six minutes later.
  console.log(
    `media-phone-clip: list top — opened on ${JSON.stringify(openingTop.split('\n')[0])}, ` +
      `closed on ${JSON.stringify(closingTop.split('\n')[0])}`,
  )
  expect(
    closingTop,
    'the list reordered under the take — the landing loop would cut, not seam',
  ).toBe(openingTop)
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
    JSON.stringify(
      { locale: mediaLocale, lang, rows, head, beats, writes, gifFrom: Number(gifFrom.toFixed(2)) },
      null,
      2,
    ),
  )
})
