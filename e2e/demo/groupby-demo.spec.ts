/**
 * Group-by promo for docs/media/groupby.{gif,mp4}.
 *
 * Paper list (first-run epic breakdown), then the Breakdown menu the
 * product actually has — assignee, priority, epic — and a hold on the
 * bar. No URL `g=` as the switch, no DOM caption, no app edits.
 *
 * Gated by GADAK_MEDIA=1. Viewport and video size must stay 1280×800
 * (see groupby.config.ts) or Playwright letterboxes the capture.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA

/** Pause between beats so a human can read the UI. Same default as web-demo. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

/**
 * Open the Breakdown menu and pick an axis the way a person does.
 * Same affordance as e2e/epics.spec.ts groupBy / e2e/bots.spec.ts.
 */
async function pickBreakdown(page: Page, label: string, groupParam: string): Promise<void> {
  await page.getByRole('button', { name: /Breakdown/ }).click()
  const option = page.getByRole('button', { name: label, exact: true })
  await expect(option).toBeVisible()
  await beat(page, 500)
  await option.click()
  await expect(page).toHaveURL(new RegExp(`[?&#]g=${groupParam}(?:&|$)`))
  await expect(page.getByTestId('group-header').first()).toBeVisible()
}

test.describe('group-by demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('breakdown menu regroups assignee → priority → epic', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    // First-run default is the epic breakdown (GDK-100), not a flat list.
    await expect(page).toHaveURL(/g=epic/)
    await expect(page.getByRole('button', { name: /Breakdown/ })).toBeVisible()
    await expect(page.getByTestId('group-header').first()).toBeVisible()
    await beat(page, 900)

    await pickBreakdown(page, 'Assignee', 'assignee')
    await beat(page, 800)

    await pickBreakdown(page, 'Priority', 'priority')
    await beat(page, 800)

    await pickBreakdown(page, 'Epic', 'epic')
    await expect(page.getByRole('button', { name: /Breakdown/ })).toBeVisible()
    await expect(page.getByTestId('group-header').first()).toBeVisible()
    // Hold the bar + epic sections long enough to read (≥1s).
    await beat(page, 1600)
    await beat(page, 600)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
