/**
 * Retro promo for docs/media/retro.{gif,mp4} — the 0.22 release cut's
 * second half.
 *
 * The claim is one sentence: the weekly report is not a dashboard you read
 * and close — every number in it is a door. So the take opens the retro
 * from the palette, widens the range, and then clicks a number, and the
 * issues behind it stand on the list as a keys view. No terminal, no CLI,
 * no DOM caption.
 *
 * Gated by GADAK_MEDIA=1. Viewport and video size must stay 1280×800
 * (retro.config.ts) or Playwright letterboxes the capture.
 */
import { test, expect, type Page, type TestInfo } from '@playwright/test'
import { writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import {
  attachConsoleErrors,
  catalogFor,
  forceLocale,
  mediaLocale,
  MEDIA_LOCALE_STAMP,
} from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA

/** The UI language this take records in. Same contract as sprint-demo. */
const LOCALE = mediaLocale()
const T = catalogFor(LOCALE)

/** The take says which language it is; the export refuses a mismatch (GDK-1501). */
async function stampLocale(testInfo: TestInfo): Promise<void> {
  await writeFile(join(testInfo.project.outputDir, MEDIA_LOCALE_STAMP), `${LOCALE}\n`, 'utf8')
}

/** Pause between beats so a human can read the UI. Same default as web-demo. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

test.describe('retro demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('the weekly report opens from the palette and a number opens the issues', async ({
    page,
  }, testInfo) => {
    await stampLocale(testInfo)
    const errors = attachConsoleErrors(page)
    await forceLocale(page, LOCALE)
    // Not gotoApp(): it pins the locale to en and waits on the English
    // sidebar count, so it can only ever boot an English frame (same reason
    // as sprint-demo.spec.ts).
    await page.goto('/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
    await beat(page, 900)

    // From the palette, the way it is reached — not a URL.
    await page.keyboard.press('ControlOrMeta+k')
    await beat(page, 600)
    await page.getByTestId('palette-action-retro').click()

    const view = page.getByTestId('retro-view')
    await expect(view).toBeVisible()
    // The title is this locale's, not English standing in a Korean frame.
    await expect(view).toContainText(T['retro.title'])
    await expect(page.getByTestId('retro-table')).toBeVisible()
    // Four whole ISO weeks plus the partial current one.
    await expect(page.getByTestId('retro-week')).toHaveCount(5)
    await expect(page.getByTestId('retro-week').last()).toContainText(T['retro.thisWeek'])
    await beat(page, 2000)

    // The range is a control on the header, and the columns follow it.
    await page.getByTestId('retro-range').filter({ hasText: T['retro.range8w'] }).click()
    await expect(page.getByTestId('retro-week')).toHaveCount(9)
    await beat(page, 1800)

    // The claim: a number is a door. The closed cell carries the keys of
    // what actually closed that week, so clicking it hands the list those
    // issues — a keys view, not a filter, which is why the URL says `ks=`.
    const cell = page.locator('[data-testid="retro-cell"][data-metric="closed"]').last()
    await expect(cell).toBeVisible()
    await cell.hover()
    await beat(page, 900)
    await cell.click()
    await expect(view).toBeHidden()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect.poll(() => page.url()).toContain('ks=')
    await expect(page.getByTestId('issue-row-trail').first()).toBeVisible()
    await beat(page, 2200)

    expect(errors.filter((e) => !e.includes('409') && !e.includes('502'))).toEqual([])
  })
})
