import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * F11 UX defects (GDK-472, GDK-473, GDK-474, GDK-478, GDK-479).
 *
 * Each test waits on the state it names — not a proxy flag. Captures for the
 * visual pass land in /tmp/f11-shots/ after the assertions hold.
 */

function boxesOverlap(
  a: { x: number; y: number; width: number; height: number },
  b: { x: number; y: number; width: number; height: number },
): boolean {
  return !(
    a.x + a.width <= b.x ||
    b.x + b.width <= a.x ||
    a.y + a.height <= b.y ||
    b.y + b.height <= a.y
  )
}

async function addProjectChip(page: Page, project: string): Promise<void> {
  await page.getByTestId('filter-add').click()
  await page.getByTestId('filter-axis-jira_project').click()
  await page.getByRole('button', { name: new RegExp(`^${project}\\b`) }).click()
  await page.keyboard.press('Escape')
}

test.describe('F11 search / filter / empty state', () => {
  test('GDK-472: palette entry names its scope; empty palette is one phrase', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const entry = page.getByTestId('palette-open')
    await expect(entry).toContainText('Search everything')
    await expect(entry.locator('kbd')).toBeVisible()

    await entry.click()
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    const box = palette.getByRole('combobox')
    await expect(box).toHaveAttribute('placeholder', en['palette.placeholder'])
    await expect(palette.getByTestId('palette-empty-hint')).toHaveCount(0)

    await page.screenshot({ path: '/tmp/f11-shots/472-palette-empty.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-473: help panel at 1280px clears the chip row and stays short', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)

    await page.getByTestId('search-help').click()
    const panel = page.getByTestId('search-help-panel')
    await expect(panel).toBeVisible()

    const panelBox = await panel.boundingBox()
    const addBox = await page.getByTestId('filter-add').boundingBox()
    expect(panelBox, 'search-help-panel must render').toBeTruthy()
    expect(addBox, 'filter-add must render').toBeTruthy()
    expect(
      boxesOverlap(panelBox!, addBox!),
      'help panel overlaps +Filter at 1280px',
    ).toBe(false)

    const chips = page.getByTestId('filter-chip')
    const chipCount = await chips.count()
    for (let i = 0; i < chipCount; i++) {
      const chipBox = await chips.nth(i).boundingBox()
      if (!chipBox) continue
      expect(boxesOverlap(panelBox!, chipBox), `help panel overlaps chip ${i}`).toBe(false)
    }

    await expect(panel).not.toContainText(/Tokens:/)
    await expect(panel.getByTestId('search-help-shortcuts')).toBeVisible()

    await page.screenshot({ path: '/tmp/f11-shots/473-search-help.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-474: exclude is a toggle; include-only axes say so in the menu', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByTestId('filter-add').click()
    const menu = page.locator('.anim-enter').filter({ has: page.getByText('Properties') })
    await expect(menu).toBeVisible()

    const statusRow = page.getByTestId('filter-axis-status')
    const projectRow = page.getByTestId('filter-axis-jira_project')
    await expect(statusRow).toBeVisible()
    await expect(projectRow).toBeVisible()
    await expect(statusRow).toContainText(en['filter.includeOnly'])
    await expect(projectRow).toContainText(en['filter.excludeMode'])

    await projectRow.click()
    const exclude = page.getByTestId('filter-exclude-mode')
    await expect(exclude).toBeVisible()
    await expect(exclude).toHaveText(en['filter.excludeMode'])
    await expect(exclude.locator('svg')).toHaveCount(0)

    await page.screenshot({ path: '/tmp/f11-shots/474-project-exclude.png' })

    await page.getByLabel(en['onboarding.back']).click()
    await statusRow.click()
    await expect(page.getByTestId('filter-exclude-mode')).toHaveCount(0)
    await expect(page.getByTestId('filter-include-only')).toBeVisible()

    await page.screenshot({ path: '/tmp/f11-shots/474-status-include-only.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-478: a query-caused empty list clears the query, not the view', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.click()
    await input.pressSequentially('zzzz-no-such-issue', { delay: 10 })
    await expect(page.getByText(en['list.noMatchTitle'], { exact: true })).toBeVisible()

    const action = page.getByTestId('empty-state-action')
    await expect(action).toHaveText(en['list.clearSearch'])
    await expect(page.getByRole('button', { name: en['list.clearFilters'] })).toHaveCount(0)

    await action.click()
    await expect(input).toHaveValue('')
    await expect(page.getByText(en['list.noMatchTitle'], { exact: true })).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller').locator('[role="button"]').first()).toBeVisible()
    // Boot default is the Epics breakdown since GDK-100; clearing returns to it.
    await expect(page.getByRole('button', { name: /Epics/ })).toHaveAttribute(
      'aria-current',
      'true',
    )
    await expect(page.getByTestId('list-count')).not.toHaveText('534 issues')

    await page.screenshot({ path: '/tmp/f11-shots/478-after-clear-search.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-478: Enter body-search zero names body and comments', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.click()
    await input.fill('zzzz-no-such-issue')
    await input.press('Enter')

    await expect(page.getByText(en['list.noMatchTitle'], { exact: true })).toBeVisible()
    await expect(page.getByText(en['list.noMatchBodyHint'])).toBeVisible()

    await page.screenshot({ path: '/tmp/f11-shots/478-body-empty.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-479: view defaults are the highlight, not chips; Reset is user-only', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Boot default is the Epics breakdown since GDK-100; clearing returns to it.
    await expect(page.getByRole('button', { name: /Epics/ })).toHaveAttribute(
      'aria-current',
      'true',
    )
    await expect(page.getByTestId('filter-chip')).toHaveCount(0)
    await expect(page.getByTestId('filter-clear')).toHaveCount(0)

    await addProjectChip(page, 'NMB')
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'NMB' })).toBeVisible()
    await expect(page.getByTestId('filter-chip').filter({ hasText: /Category/ })).toHaveCount(0)
    await expect(page.getByTestId('filter-clear')).toBeVisible()

    await page.getByTestId('filter-clear').click()
    await expect(page.getByTestId('filter-chip')).toHaveCount(0)
    // Boot default is the Epics breakdown since GDK-100; clearing returns to it.
    await expect(page.getByRole('button', { name: /Epics/ })).toHaveAttribute(
      'aria-current',
      'true',
    )
    await expect(page.getByTestId('list-count')).not.toHaveText('534 issues')
    await expect(page.getByText(/Done \d+/)).toHaveCount(0)

    await page.screenshot({ path: '/tmp/f11-shots/479-after-reset.png' })
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
