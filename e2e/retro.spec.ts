import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/*
 * Weekly retro as a column view (GDK-1660). The demo fixture carries no
 * visits of its own, so what a fresh e2e home yields is measured first
 * (GET /api/v1/issues/retro/ on the e2e serve: buckets with sessions 0 and
 * closed / in progress counts from the mirror) and the assertions read that:
 * the table renders, the closed cell is a door when it has keys, the URL
 * round-trips, Esc returns the column to the list.
 */
test.describe('weekly retro view', () => {
  test('opens from the palette, renders one column per week, and round-trips its URL', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await palette.getByTestId('palette-action-retro').click()

    const view = page.getByTestId('retro-view')
    await expect(view).toBeVisible()
    await expect(view.getByText('Weekly retro').first()).toBeVisible()
    // "4w" is four whole ISO weeks plus the partial current one (measured on
    // the e2e serve: five buckets, the last with partial=true).
    await expect(page.getByTestId('retro-week')).toHaveCount(5)
    await expect(page.getByTestId('retro-week').last()).toContainText('this week')
    await expect(page.getByTestId('retro-table')).toBeVisible()
    expect(page.url()).toContain('retro=1')

    // The address restores the view on its own.
    await page.reload()
    await expect(page.getByTestId('retro-view')).toBeVisible()

    // Esc hands the column back to the list.
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('retro-view')).toBeHidden()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(page.url()).not.toContain('retro=1')

    expect(errors.filter((e) => !e.includes('409') && !e.includes('502'))).toEqual([])
  })

  test('a cell with issues behind it opens them on the list', async ({ page }) => {
    await gotoApp(page)
    await page.goto('/#/?retro=1')
    await expect(page.getByTestId('retro-view')).toBeVisible()
    const cells = page.getByTestId('retro-cell')
    // The demo mirror has issues in progress and closed inside four weeks
    // (measured on the fixture's spread), so at least one door exists.
    await expect(cells.first()).toBeVisible()
    const metric = await cells.first().getAttribute('data-metric')
    await cells.first().click()
    await expect(page.getByTestId('retro-view')).toBeHidden()
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    // A keys view: the URL carries the key list, not a filter.
    await expect.poll(() => page.url()).toContain('ks=')
    expect(metric).toBeTruthy()
  })
})
