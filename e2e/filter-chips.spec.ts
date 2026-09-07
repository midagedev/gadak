import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, DEMO_ISSUE_COUNT_EN } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * The filter chip row: what a chip is, and what it is not (moved from
 * ux-f11.spec.ts in the v0.21 audit ladder round — the audit-placement ux-fNN
 * files were dissolved by surface). Per-value exclude is every axis's
 * capability now (GDK-771); view defaults are the highlight, never chips
 * (GDK-479). Their /tmp/f11-shots captures were deleted with the move.
 */

async function addProjectChip(page: Page, project: string): Promise<void> {
  await page.getByTestId('filter-add').click()
  await page.getByTestId('filter-axis-jira_project').click()
  // The value row by its data attribute — an issue row in the list behind the
  // popover is also a button whose name starts with the project key.
  await page.locator(`[data-testid="filter-value-row"][data-filter-value="${project}"]`).click()
  await page.keyboard.press('Escape')
}

test.describe('filter chips', () => {
  // Contract rewrite (GDK-771, 2026-08-24): the GDK-474 version of this test
  // pinned the half-adoption UI — a modal Exclude toggle on the two project
  // axes and a "No exclude" caption everywhere else (the label users read as
  // noise). It was green on the pre-change source. Every visible axis now
  // excludes through a per-value ⊘, and no axis carries a capability caption.
  test('GDK-771: every axis excludes per value; the caption noise is gone', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    await page.getByTestId('filter-add').click()
    const menu = page.locator('.anim-enter').filter({ has: page.getByText('Properties') })
    await expect(menu).toBeVisible()

    const statusRow = page.getByTestId('filter-axis-status')
    await expect(statusRow).toBeVisible()
    // The old per-axis captions are gone from the field list.
    await expect(statusRow).not.toContainText('No exclude')
    await expect(statusRow).not.toContainText('Exclude')

    // Status — an axis that was include-only — now excludes per value.
    await statusRow.click()
    await expect(page.getByTestId('filter-exclude-mode')).toHaveCount(0)
    await expect(page.getByTestId('filter-include-only')).toHaveCount(0)
    const firstRow = page.getByTestId('filter-value-row').first()
    await expect(firstRow).toBeVisible()
    const excludeBtn = page.getByTestId('filter-value-exclude').first()
    await excludeBtn.click()
    await expect(firstRow).toHaveAttribute('data-state', 'excluded')

    // The chip renders the negated form; clicking ⊘ again clears it.
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('filter-chip').filter({ hasText: /not/i })).toBeVisible()
    await page.getByTestId('filter-add').click()
    await statusRow.click()
    await page.getByTestId('filter-value-exclude').first().click()
    await expect(page.getByTestId('filter-value-row').first()).toHaveAttribute('data-state', 'off')
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('GDK-479: view defaults are the highlight, not chips; Reset is user-only', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    // Stand on a view the sidebar owns (gotoApp lands on the pool by address
    // since the Epics built-in was cut, GDK-1493).
    await page.locator('aside').getByRole('button', { name: en['view.allOpen.name'] }).click()
    await expect(page).toHaveURL(/[#?&]sc=new%2Cinprogress/)
    await expect(page).not.toHaveURL(/[#?&]g=epic/)

    // The view is All open (chosen above); clearing returns to it, not to a bare pool.
    await expect(page.locator('aside').getByRole('button', { name: en['view.allOpen.name'] })).toHaveAttribute(
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
    // The view is All open (chosen above); clearing returns to it, not to a bare pool.
    await expect(page.locator('aside').getByRole('button', { name: en['view.allOpen.name'] })).toHaveAttribute(
      'aria-current',
      'true',
    )
    await expect(page.getByTestId('list-count')).not.toHaveText(DEMO_ISSUE_COUNT_EN)
    await expect(page.getByText(/Done \d+/)).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
