import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

test.describe('detail', () => {
  test('row click opens detail panel with summary/history/comments sections', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Open an issue known to exist with changelog entries (NMB-110 has history).
    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()

    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
      .click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    // Section titles from en.ts detail.*
    await expect(panel.getByRole('heading', { name: 'Details' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'Description' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'History' })).toBeVisible()
    // Issue key visible in the sticky header
    await expect(panel.getByText('NMB-110').first()).toBeVisible()

    // Write gate: the configured credential alone must unlock the write UI
    // (me/ → email → identified). Regression guard for the boot-time identity probe.
    await expect(panel.locator('textarea[placeholder*="Write a comment"]')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
