import { test, expect } from '@playwright/test'
import { attachConsoleErrors, forceLocale, openServerSettings, DEMO_ISSUE_COUNT_EN_RE, DEMO_ISSUE_COUNT_KO, DEMO_ISSUE_COUNT_JA } from './helpers'

test.describe('locale', () => {
  test('switching to ko reloads with Korean copy', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/')
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })

    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await expect(dialog.getByText('Language')).toBeVisible()

    // setLocale writes localStorage and hard-reloads (URL hash unchanged).
    // Language is the labeled select in the footer — not the interval presets on Sync.
    await Promise.all([
      page.waitForEvent('load'),
      dialog.getByLabel('Language').selectOption('ko'),
    ])

    // ko.ts: sidebar.issueCount = '{n}건' (pool size; list count may be filtered).
    // .first(): the settings dialog is still open across the reload and its
    // runtime section now counts in the same 건 unit (GDK-1226), so a bare
    // getByText doubles up and trips strict mode — same as the en assertion.
    await expect(page.getByText(DEMO_ISSUE_COUNT_KO).first()).toBeVisible({ timeout: 30_000 })
    // <html lang> follows the locale so screen readers switch pronunciation.
    await expect(page.locator('html')).toHaveAttribute('lang', 'ko-KR')
    // ko.ts: sidebar.settings
    await expect(page.getByRole('button', { name: '설정', exact: true })).toBeVisible()
    // ko.ts: filter.add
    await expect(page.getByRole('button', { name: '+ 필터' })).toBeVisible()
    // Default "All open" view still shows a filtered list count in Korean.
    // list-count testid: sidebar now uses the same 건 unit, so a bare /\d+건/
    // locator would match both and trip Playwright strict mode.
    await expect(page.getByTestId('list-count')).toHaveText(/\d+건/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('switching to ja reloads with Japanese copy', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await forceLocale(page, 'en')
    await page.goto('/')
    await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })

    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await expect(dialog.getByText('Language')).toBeVisible()

    await Promise.all([
      page.waitForEvent('load'),
      dialog.getByLabel('Language').selectOption('ja'),
    ])

    // ja.ts: sidebar.issueCount = '{n}件' (pool size; list count may be filtered).
    // .first() for the same reason as the ko assertion above: the open
    // settings dialog counts issues in 件 too (GDK-1226).
    await expect(page.getByText(DEMO_ISSUE_COUNT_JA).first()).toBeVisible({ timeout: 30_000 })
    await expect(page.locator('html')).toHaveAttribute('lang', 'ja-JP')
    // ja.ts: sidebar.settings
    await expect(page.getByRole('button', { name: '設定', exact: true })).toBeVisible()
    // ja.ts: filter.add
    await expect(page.getByRole('button', { name: '+ フィルター' })).toBeVisible()
    await expect(page.getByTestId('list-count')).toHaveText(/\d+件/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
