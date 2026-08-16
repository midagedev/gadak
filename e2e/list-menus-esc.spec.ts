import { test, expect, type Page } from '@playwright/test'
import { appConsoleErrors, attachConsoleErrors, gotoApp } from './helpers'

/*
 * GDK-132: the three list-header menus (filter add / columns / sort) close on
 * Esc through the shared dom-actions owner, and spend the keystroke so the
 * detail panel below keeps its own.
 */

function filterAdd(page: Page) {
  return page.getByRole('button', { name: '+ Filter', exact: true })
}

function columnsTrigger(page: Page) {
  return page.getByRole('button', { name: 'Columns', exact: true })
}

function sortTrigger(page: Page) {
  return page.getByTitle('Sort options')
}

function filterPanel(page: Page) {
  return page.getByText('Properties', { exact: true })
}

function columnsPanel(page: Page) {
  return page.getByText('Visible columns', { exact: true })
}

function sortPanel(page: Page) {
  return page.getByRole('button', { name: '↓ Desc', exact: true })
}

test.describe('list-header menus close on Esc', () => {
  test('each menu closes on Esc', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await filterAdd(page).click()
    await expect(filterPanel(page)).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(filterPanel(page)).toBeHidden()

    await columnsTrigger(page).click()
    await expect(columnsPanel(page)).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(columnsPanel(page)).toBeHidden()

    await sortTrigger(page).click()
    await expect(sortPanel(page)).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(sortPanel(page)).toBeHidden()

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('one Esc closes the columns menu and leaves the detail panel open', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // Docked three-track grid: below 1440 the panel overlays the toolbar and
    // the Columns click never lands (narrow-viewport.spec.ts).
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoApp(page)

    await page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').first().click()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)

    await columnsTrigger(page).click()
    await expect(columnsPanel(page)).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(columnsPanel(page)).toBeHidden()
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
