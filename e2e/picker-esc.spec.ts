import { test, expect, type Page } from '@playwright/test'
import { appConsoleErrors, attachConsoleErrors, gotoApp } from './helpers'

/*
 * The detail pickers (status s / assignee a / priority p) are dismissable
 * surfaces like the list-header menus (GDK-132) and the breakdown menu
 * (GDK-617): one Esc closes only the topmost surface (GDK-604), so a picker
 * never takes the detail panel with it. And like dialogs (focus-trap
 * restores the opener), a closed picker hands focus back to the trigger it
 * opened from — before, focus fell to <body> and a keyboard user lost their
 * place.
 */

async function openDetail(page: Page) {
  // Docked three-track grid, same as breakdown-esc / list-menus-esc: below
  // 1440 the panel overlays the toolbar.
  await page.setViewportSize({ width: 1440, height: 900 })
  await gotoApp(page)
  await page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').first().click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  await expect(panel).toHaveClass(/is-open/)
  return panel
}

function activeTestid(page: Page): Promise<string | null> {
  return page.evaluate(() => document.activeElement?.getAttribute('data-testid') ?? null)
}

test.describe('detail pickers close on Esc', () => {
  test('one Esc closes the status picker and leaves the detail panel open', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const panel = await openDetail(page)

    // The e2e origin is fictional (nimbus.example.com), so the transitions
    // GET fails upstream and the menu would open as an error with nothing
    // focusable — the picker would never take focus, which is a different
    // state than this contract pins. Serve one transition, as write-through
    // does; the picker then focuses the option, and the Esc below is typed
    // from inside the menu.
    const key = await page
      .locator('[data-testid="issue-list-scroller"] [data-issue-key]')
      .first()
      .getAttribute('data-issue-key')
    await page.route(`**/api/v1/issues/${key}/transitions/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: { transitions: [{ id: '21', name: 'Start work', to_status: 'In Progress', to_category: 'indeterminate' }] },
      })
    })

    await expect(panel.getByTestId('status-transition')).toBeVisible()
    await page.keyboard.press('s')
    const menu = panel.getByRole('listbox')
    await expect(menu).toBeVisible()
    await expect(menu.getByRole('option').first()).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(menu).toBeHidden()
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)
    expect(await activeTestid(page)).toBe('status-transition')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('one Esc closes the assignee picker and leaves the detail panel open', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const panel = await openDetail(page)

    await expect(panel.getByTestId('assignee-picker')).toBeVisible()
    await page.keyboard.press('a')
    const dialog = panel.getByRole('dialog')
    await expect(dialog).toBeVisible()
    // The picker focuses its search input on open — that is where the Esc
    // below is typed from.
    await expect(dialog.getByRole('textbox')).toBeFocused()

    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)
    expect(await activeTestid(page)).toBe('assignee-picker')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('one Esc closes the priority picker and leaves the detail panel open', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const panel = await openDetail(page)

    await expect(panel.getByTestId('priority-picker')).toBeVisible()
    // Same reason as the status test: the e2e origin is fictional, so the
    // per-issue priorities GET hangs and the menu would sit in Loading…
    // with nothing focusable. Serve the catalog (detail-coaching's shape);
    // the picker then focuses its first option, and the Esc below is typed
    // from inside the menu.
    const key = await page
      .locator('[data-testid="issue-list-scroller"] [data-issue-key]')
      .first()
      .getAttribute('data-issue-key')
    await page.route(`**/api/v1/issues/${key}/priorities/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        json: { priorities: [{ id: '1', name: 'Highest' }, { id: '3', name: 'Medium' }] },
      })
    })
    await page.keyboard.press('p')
    const menu = panel.getByRole('listbox')
    await expect(menu).toBeVisible()
    await expect(menu.getByRole('option').first()).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(menu).toBeHidden()
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)
    expect(await activeTestid(page)).toBe('priority-picker')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
