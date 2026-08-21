import { expect, test } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * GDK-567: deleting a saved view is a two-click arm (same as credential
 * delete). First click must not remove the row; a click elsewhere cancels.
 */
test.describe('view delete arm', () => {
  test('first click arms, elsewhere cancels, second click deletes', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const VIEW = `Delete-arm ${Date.now()}`
    await gotoApp(page)

    await page.getByTestId('filter-add').click()
    await page.getByTestId('filter-date-axis-created').click()
    await page.getByTestId('filter-date-from').fill('2026-01-01')
    await expect(page).toHaveURL(/cf=2026-01-01/, { timeout: 10_000 })

    await page.getByRole('button', { name: en['filter.saveAsView'] }).click()
    await page.getByPlaceholder(en['filter.viewName']).fill(VIEW)
    await page.getByRole('button', { name: en['filter.savePersonal'] }).click()
    await expect(page.getByPlaceholder(en['filter.viewName'])).toBeHidden()

    const row = page.locator('[data-testid="sidebar-view-row"]').filter({ hasText: VIEW })
    await expect(row).toBeVisible()
    const del = row.getByTestId('sidebar-view-delete')

    await row.hover()
    await del.click()
    await expect(del).toHaveAttribute('data-armed', 'true')
    await expect(row).toBeVisible()

    await page.getByTestId('list-count').click()
    await expect(del).not.toHaveAttribute('data-armed', 'true')
    await expect(row).toBeVisible()

    await row.hover()
    await del.click()
    await expect(del).toHaveAttribute('data-armed', 'true')
    await del.click()
    await expect(row).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
