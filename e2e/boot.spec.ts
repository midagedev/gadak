import { test, expect } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, DEMO_ISSUE_COUNT_EN_RE } from './helpers'

test.describe('boot', () => {
  test('renders issue list with 534 count and English copy', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/')

    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    // list.countIssues — en.ts: '{n} issues'
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
    // Sidebar wordmark and built-in nav labels (en.ts)
    await expect(page.getByText('gadak', { exact: true }).first()).toBeVisible()
    await expect(page.getByRole('button', { name: 'Settings', exact: true })).toBeVisible()
    await expect(page.getByTestId('search-input')).toBeVisible()
    // Filter add chip
    await expect(page.getByRole('button', { name: '+ Filter' })).toBeVisible()
    // At least one issue row in the virtual list
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await expect(page.locator('[data-testid="issue-list-scroller"] [role="button"]').first()).toBeVisible()

    expect(errors, `console errors on boot:\n${errors.join('\n')}`).toEqual([])
  })
})
