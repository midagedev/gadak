import { test, expect, type Page } from '@playwright/test'
import { appConsoleErrors, attachConsoleErrors, gotoApp } from './helpers'

/*
 * GDK-617: the breakdown menu is a dismissable surface like the three
 * list-header menus (GDK-132), and closes on Esc through the shared
 * dom-actions owner, spending the keystroke so an open detail panel keeps
 * its selection. Before, Esc leaked to the shell keymap and cleared the
 * selection with the menu still open.
 */

function breakdownTrigger(page: Page) {
  return page.getByRole('button', { name: /Breakdown/ })
}

// An option that only exists inside the open menu (bots.spec.ts picks the
// same one). The trigger's own name carries the current axis, so an exact
// match on an option label cannot hit it.
function menuOnlyOption(page: Page) {
  return page.getByRole('button', { name: 'Actor', exact: true })
}

test.describe('breakdown menu closes on Esc', () => {
  test('one Esc closes the breakdown menu and leaves the detail panel open', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // Docked three-track grid: below 1440 the panel overlays the toolbar and
    // the breakdown click never lands (narrow-viewport.spec.ts).
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoApp(page)

    await page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').first().click()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)

    await breakdownTrigger(page).click()
    await expect(menuOnlyOption(page)).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(menuOnlyOption(page)).toBeHidden()
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
