import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput, DEMO_ISSUE_COUNT_EN } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * The list's zero-rows sentence, by cause (moved from ux-f11.spec.ts in the
 * v0.21 audit ladder round — the audit-placement ux-fNN files were dissolved
 * by surface). A query-caused empty list clears the query, not the view
 * (GDK-478); a body-search zero names what was searched (GDK-478). Their
 * /tmp/f11-shots captures were deleted with the move.
 */

test.describe('list empty states (GDK-478)', () => {
  test('a query-caused empty list clears the query, not the view', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    // Stand on a view the sidebar owns (gotoApp lands on the pool by address
    // since the Epics built-in was cut, GDK-1493).
    await page.locator('aside').getByRole('button', { name: en['view.allOpen.name'] }).click()
    await expect(page).toHaveURL(/[#?&]sc=new%2Cinprogress/)
    await expect(page).not.toHaveURL(/[#?&]g=epic/)

    const input = searchInput(page)
    await input.click()
    await input.pressSequentially('zzzz-no-such-issue', { delay: 10 })
    await expect(page.getByText(en['list.noMatchTitle'], { exact: true })).toBeVisible()

    const action = page.getByTestId('empty-state-action')
    await expect(action).toHaveText(en['list.clearSearch'])
    await expect(page.getByRole('button', { name: en['filter.clear'] })).toHaveCount(0)

    await action.click()
    await expect(input).toHaveValue('')
    await expect(page.getByText(en['list.noMatchTitle'], { exact: true })).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller').locator('[role="button"]').first()).toBeVisible()
    // The view is All open (chosen above); clearing returns to it, not to a bare pool.
    await expect(page.locator('aside').getByRole('button', { name: en['view.allOpen.name'] })).toHaveAttribute(
      'aria-current',
      'true',
    )
    await expect(page.getByTestId('list-count')).not.toHaveText(DEMO_ISSUE_COUNT_EN)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Enter body-search zero names body and comments', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.click()
    await input.fill('zzzz-no-such-issue')
    await input.press('Enter')

    await expect(page.getByText(en['list.noMatchTitle'], { exact: true })).toBeVisible()
    await expect(page.getByText(en['list.noMatchBodyHint'])).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
