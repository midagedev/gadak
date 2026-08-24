/**
 * History-focus promo for docs/media/history.{gif,mp4} (F2).
 *
 * The changelog is the interface: the clip opens the one issue whose
 * thread tells the whole story — NMB-139 "Notification digests duplicate
 * events that occurred near the midnight boundary" has status transitions,
 * an agent comment, a linked PR (fixture-injected, e2e/serve.sh — title
 * `fix(NMB-139): retry budget for upload`), and a duration chip. Palette →
 * Enter (the same affordance as the flagship), then a slow scroll of the
 * activity thread, then a hold on the PR row.
 *
 * Gated by GADAK_MEDIA=1. Viewport and video size must stay 1280×800
 * (see history.config.ts) or Playwright letterboxes the capture.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA

/** Pause between beats so a human can read the UI. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

test.describe('history demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('one issue thread: transitions, comment, PR, durations', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/#/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible({ timeout: 30_000 })
    await beat(page, 900)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('NMB-139', { delay: 90 })
    // Key-exact matches land in the key section, not the unified list —
    // assert the palette shows the key at all, then commit.
    await expect(palette.getByText(/NMB-139/i).first()).toBeVisible({ timeout: 10_000 })
    await beat(page, 600)
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    // GDK-763: identity is the issue title on title-editor. The old
    // getByText(/retry budget for upload/) matched the fixture-injected PR
    // title (e2e/serve.sh) while NMB-139's summary is Notification digests…
    const title = panel.getByTestId('title-editor')
    await expect(title).toHaveText(/Notification digests duplicate events/i)
    await beat(page, 1200)

    // Scroll the activity thread top-to-bottom: status changes, the agent
    // comment, the linked PR, durations. Slow enough to read each entry.
    const scroller = panel.getByTestId('detail-scroll')
    await expect(scroller).toBeVisible()
    for (let i = 0; i < 6; i++) {
      await scroller.hover()
      await page.mouse.wheel(0, 260)
      await beat(page, 550)
    }
    await beat(page, 900)
    // Back up so the hold lands mid-thread with the PR row in view.
    for (let i = 0; i < 2; i++) {
      await page.mouse.wheel(0, -260)
      await beat(page, 450)
    }
    await beat(page, 1500)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
