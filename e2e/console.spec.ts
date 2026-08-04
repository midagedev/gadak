import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, openServerSettings, searchInput } from './helpers'

test.describe('console hygiene', () => {
  test('boot + search + detail + settings produce zero console errors', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Client search
    const input = searchInput(page)
    await input.fill('NMB-16')
    await expect(page.getByText('NMB-16').first()).toBeVisible()

    // Open detail
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-16' })
      .first()
      .click()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Details' })).toBeVisible()

    // Open settings and close
    await openServerSettings(page)
    await page.getByRole('dialog', { name: 'Settings' }).getByRole('button', { name: 'Close' }).click()
    await expect(page.getByRole('dialog', { name: 'Settings' })).toHaveCount(0)

    expect(errors, `console errors during interaction:\n${errors.join('\n')}`).toEqual([])
  })
})
