import { test, expect } from '@playwright/test'
import { attachConsoleErrors, forceLocale, openServerSettings } from './helpers'

test.describe('locale', () => {
  test('switching to ko reloads with Korean copy', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/')
    await expect(page.getByText(/534 issues/).first()).toBeVisible({ timeout: 30_000 })

    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await expect(dialog.getByText('Language')).toBeVisible()

    // setLocale writes localStorage and hard-reloads (URL hash unchanged).
    // Language is the labeled select in the footer — not the interval presets on Sync.
    await Promise.all([
      page.waitForEvent('load'),
      dialog.getByLabel('Language').selectOption('ko'),
    ])

    // ko.ts: sidebar.issueCount = '{n} 이슈' (pool size; list count may be filtered)
    await expect(page.getByText('534 이슈')).toBeVisible({ timeout: 30_000 })
    // <html lang> follows the locale so screen readers switch pronunciation.
    await expect(page.locator('html')).toHaveAttribute('lang', 'ko-KR')
    // ko.ts: sidebar.settings
    await expect(page.getByRole('button', { name: '설정', exact: true })).toBeVisible()
    // ko.ts: filter.add
    await expect(page.getByRole('button', { name: '+ 필터' })).toBeVisible()
    // Default "All open" view still shows a filtered list count in Korean.
    await expect(page.getByText(/\d+건/)).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
