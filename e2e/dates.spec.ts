import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'
import { RANGE_KEY } from '../web/src/lib/view-config'

/*
 * GDK-249 / GDK-250: date range filters on the read surface and the due column.
 * resolved-this-week → rf= serialisation: web/src/lib/builtin-views.test.ts
 * (moved from this file).
 */

test.describe('date filters and due column', () => {
  test('add-filter created range writes cf= and a range chip', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByTestId('filter-add').click()
    await page.getByTestId('filter-date-axis-created').click()
    await page.getByTestId('filter-date-from').fill('2026-01-01')

    await expect(page).toHaveURL(new RegExp(`${RANGE_KEY.created_from}=2026-01-01`), {
      timeout: 10_000,
    })
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
})
