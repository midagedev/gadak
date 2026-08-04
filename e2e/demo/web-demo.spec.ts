/**
 * Web UI demo recording for docs/media/web-demo.{gif,mp4}.
 *
 * Gated by SCRY_MEDIA=1 so the main suite skips it. This is a hero asset: a
 * reader gives it two seconds, so it shows one thing first — the list collapsing
 * under the keystroke — and only then the palette and detail. Total ~17s.
 *
 * Two rules learned the hard way:
 *  - Never `waitForLoadState('networkidle')`. The client polls for a delta every
 *    15s, so it returns 15s later and the first half of the GIF is a still frame.
 *  - No DOM caption overlays. App code stays clean; pacing carries the story.
 */
import { test, expect, type Page } from '@playwright/test'
import { gotoApp, searchInput } from '../helpers'

const isMedia = !!process.env.SCRY_MEDIA

/** Pause between beats so a human can read the UI. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

/** List toolbar count ("N issues") — not the sidebar pool total. */
function listCount(page: Page) {
  return page.locator('span').filter({ hasText: /^\d+ issues?$/ })
}

test.describe('web UI demo', () => {
  test.skip(!isMedia, 'SCRY_MEDIA=1 only — media pipeline recording')

  test('instant search, palette, detail, saved view', async ({ page }) => {
    // ── Boot ──────────────────────────────────────────────────────────────
    await gotoApp(page)
    await expect(page.getByText('519 issues')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    // Short settle for the trailing boot requests. Deliberately not networkidle.
    await beat(page, 900)

    // ── The pitch: type, and hundreds of issues become a handful ──────────
    const input = searchInput(page)
    await input.click()
    await beat(page, 250)
    // Slow enough to read each narrowing step; the count and the <mark>
    // highlights are the whole point of this beat.
    await input.pressSequentially('pagination', { delay: 150 })
    await expect(page.locator('mark').first()).toBeVisible({ timeout: 10_000 })
    await beat(page, 1600)

    // A second query, so it reads as "typing is instant", not one lucky result.
    await input.fill('')
    await beat(page, 250)
    await input.pressSequentially('timezone', { delay: 150 })
    await expect(page.locator('mark').first()).toBeVisible({ timeout: 10_000 })
    await beat(page, 1400)
    await input.fill('')
    await beat(page, 500)

    // ── ⌘K palette → jump straight to an issue ────────────────────────────
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await beat(page, 500)
    await page.keyboard.type('NMB-110', { delay: 130 })
    await expect(palette.getByRole('option').first()).toContainText('NMB-110')
    await beat(page, 800)
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByText('NMB-110').first()).toBeVisible()
    await beat(page, 1100)

    // ── Detail: description, then the comment with inline screenshots ─────
    // The images are served from the local attachment cache, so they appear
    // without a network round trip — worth showing, since "attachments work
    // offline" is not obvious from a list of filenames.
    await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()
    await panel.evaluate((el) => {
      const scroller = el.querySelector('.overflow-y-auto') ?? el
      scroller.scrollTo({ top: scroller.scrollHeight * 0.45, behavior: 'smooth' })
    })
    await beat(page, 900)
    await expect(panel.locator('.adf-media-image img').first()).toBeVisible({ timeout: 10_000 })
    await panel.evaluate((el) => {
      const scroller = el.querySelector('.overflow-y-auto') ?? el
      scroller.scrollTo({ top: scroller.scrollHeight * 0.72, behavior: 'smooth' })
    })
    await beat(page, 1600)

    // ── One saved view, then rest on the list for a clean loop ────────────
    await page.keyboard.press('Escape')
    await beat(page, 400)
    await page.getByRole('button', { name: /Reopened/i }).first().click()
    await expect(listCount(page)).toBeVisible()
    await beat(page, 1600)
  })
})
