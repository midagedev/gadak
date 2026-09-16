import { expect, test, type Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { mkdirSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { mediaLocale, mediaLocaleTag } from '../media.config'

/*
 * The publication stills (GDK-1501's phone half). Three frames, the three
 * surfaces a landing page or a README can spend a picture on:
 *
 *   phone-list     the list a person lands on, active-sprint line and all
 *   phone-palette  the scope palette over it
 *   phone-detail   NMB-110 — fields and attachments, the two summary blocks
 *
 * Unlike shots/ (scratch/, throwaway), these land in docs/media/ as committed
 * assets under the locale-variant naming (docs/project/MEDIA.md: the tag is
 * the last segment before the extension — phone-list.ko.png). One locale per
 * run; `make media-phone` owns the loop over them.
 *
 * The picture is the app's own viewport and nothing else: no bezel, no status
 * bar, no shadow, no background plate — the 402×874 @3x rectangle the gate
 * measures is the rectangle that ships.
 */

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = join(here, '..', '..')
const MEDIA_DIR = join(repoRoot, 'docs', 'media')

/** '' for en, '.ko' / '.ja' for the variants — the committed-asset convention. */
const suffix = mediaLocale === 'en' ? '' : `.${mediaLocale}`

/** The config's viewport height, in css px — the fold measurements answer to it. */
const FRAME_HEIGHT = 874

const files: string[] = []

/*
 * Settle before every exposure. The failure mode is a fly() transition caught
 * mid-frame: the overlay half-way through its opacity and offset photographs
 * as a broken layout, and in the first capture cycle exactly that frame was
 * reported as a high-severity defect that was not in the code. Awaiting
 * getAnimations() once was measured not to be enough — some animations start
 * when others end — so: finish everything, let 400ms of late starters begin
 * and finish, finish everything again, then open the shutter.
 */
async function shoot(page: Page, stem: string): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  await page.waitForTimeout(400)
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  const file = join(MEDIA_DIR, `phone-${stem}${suffix}.png`)
  await page.screenshot({ path: file })
  files.push(file)
}

test(`phone publication stills (${mediaLocale})`, async ({ page }) => {
  mkdirSync(MEDIA_DIR, { recursive: true })

  // The locale travels both roads the app can read it by. Its resolution
  // order (web/src/lib/i18n detectLocale) is localStorage gadak_locale first,
  // navigator.language second — the init script carries the first, the
  // config's `locale` keeps the second from disagreeing with it, so no date
  // or collator quietly formats as en-US mid-frame. initLocale stamps
  // <html lang> from the same value, which makes the assertion below a real
  // check: a locale that never reached the app reads back as the en-US
  // default.
  await page.addInitScript((code) => localStorage.setItem('gadak_locale', code), mediaLocale)

  /* ── phone-list ── */
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  // The sprint line is part of the story a landing page tells with this
  // frame — the active sprint's strip over the list — so wait for it too.
  await page.locator('.pane:not(.off) [data-testid="sprint-line"]').waitFor()

  const lang = await page.evaluate(() => document.documentElement.lang)
  expect(lang, `<html lang> after the first navigation (locale ${mediaLocale})`).toBe(
    mediaLocaleTag,
  )
  console.log(`media-phone: locale ${mediaLocale} — document.documentElement.lang read "${lang}"`)

  await shoot(page, 'list')

  /* ── phone-palette ── */
  await page.locator('.pane:not(.off) h1 button.scope').click()
  await page.locator('.palette-field input').waitFor()
  await shoot(page, 'palette')

  /* ── phone-detail ── */
  // NMB-110, reached the way a person reaches it: palette search by key. It
  // carries a label, a component and a parent — enough rows for the Fields
  // block to be worth a picture — and three image attachments.
  await page.locator('.palette-field input').fill('NMB-110')
  const hit = page.locator('.pane:not(.off) button.row', { hasText: 'NMB-110' }).first()
  await hit.waitFor()
  await hit.click()
  await page.locator('.detail-layer button.back').waitFor()
  // Assert the issue before shooting: a stale list or a mis-keyed search must
  // not become the published picture of "the detail screen".
  await expect(page.locator('.detail-layer .bar-key')).toHaveText('NMB-110')
  await page.locator('[data-testid="detail-fields"]').waitFor()
  await expect(page.locator('[data-testid="detail-attachments-count"]')).toHaveText('3')
  // The grid renders an <img> only once its blob URL has been fetched, so
  // three of them is also the check that this serve really hands out
  // attachment bytes — a fixture copy without the attachment manifest beside
  // it demotes every image to a ledger row, and the frame would photograph
  // that failure as if it were the product.
  await expect(page.locator('[data-testid="attachment-thumb"] img')).toHaveCount(3)

  // Fields at the top of the frame, as much of Attachments below it as the
  // 874px frame holds — one viewport, no stitching, no taller viewport. The
  // numbers are printed so the log says exactly how much of the grid the
  // published frame contains.
  const fields = page.locator('[data-testid="detail-fields"]')
  await fields.evaluate((el) => el.scrollIntoView({ block: 'start' }))
  await page.waitForTimeout(400)
  const fieldsBox = await fields.boundingBox()
  const attHeadingBox = await page
    .locator('.detail-layer h3:has([data-testid="detail-attachments-count"])')
    .boundingBox()
  const gridBox = await page.locator('[data-testid="detail-attachments"]').boundingBox()
  console.log(
    `media-phone: detail framing (css px, frame ${FRAME_HEIGHT} tall) — ` +
      `fields y=${fieldsBox?.y.toFixed(0)} h=${fieldsBox?.height.toFixed(0)}, ` +
      `attachments heading y=${attHeadingBox?.y.toFixed(0)}, ` +
      `grid y=${gridBox?.y.toFixed(0)} h=${gridBox?.height.toFixed(0)} ` +
      `bottom=${(gridBox ? gridBox.y + gridBox.height : NaN).toFixed(0)} ` +
      `(${gridBox && gridBox.y + gridBox.height <= FRAME_HEIGHT ? 'whole grid inside the frame' : 'grid runs past the fold'})`,
  )
  await shoot(page, 'detail')

  /* ── provenance ── */
  // Printed, not written to a file: the PNGs are the committed assets; the
  // recording log is the record of which tree produced them.
  for (const f of files) {
    console.log(`media-phone: ${relative(repoRoot, f)} — ${statSync(f).size} bytes`)
  }
  const head = execFileSync('git', ['rev-parse', 'HEAD'], {
    cwd: repoRoot,
    encoding: 'utf8',
  }).trim()
  const porcelain = execFileSync('git', ['status', '--porcelain'], {
    cwd: repoRoot,
    encoding: 'utf8',
  }).trim()
  console.log(
    `media-phone: recorded at ${head} (${porcelain === '' ? 'clean tree' : 'dirty tree'})`,
  )
})
