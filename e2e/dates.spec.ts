import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * GDK-249 / GDK-250: date range filters on the read surface, due column,
 * and the resolved-this-week builtin using resolved_from (not updated_from).
 */

test.describe('date filters and due column', () => {
  test('add-filter created range writes cf= and a range chip', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByTestId('filter-add').click()
    await page.getByTestId('filter-date-axis-created').click()
    await page.getByTestId('filter-date-from').fill('2026-01-01')

    await expect(page).toHaveURL(/cf=2026-01-01/, { timeout: 10_000 })
    await expect(page.getByTestId('filter-chip').filter({ hasText: /Created/ })).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('due column is off by default and can be enabled', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await expect(page.locator('[data-col="due"]')).toHaveCount(0)

    await page.getByTestId('columns-menu').click()
    await page.getByTestId('column-toggle-due').click()
    await expect(page).toHaveURL(/cl=/, { timeout: 10_000 })
    await expect(page.locator('[data-col="due"]').first()).toBeAttached()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('resolved this week writes rf= (resolved_from), not only uf=', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByRole('button', { name: en['view.resolvedWeek.name'] }).click()
    await expect(page).toHaveURL(/rf=\d{4}-\d{2}-\d{2}/, { timeout: 10_000 })
    await expect(page).not.toHaveURL(/uf=/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
