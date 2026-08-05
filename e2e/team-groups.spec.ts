import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

test.describe('teamGroups surface', () => {
  test('backend group from groupRules appears in Team filter and grouping', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // Filter menu exposes Team (field.team_group) when features.teamGroups is on.
    // Accessible name is "Team ›" (label + chevron), so match by prefix.
    await page.getByRole('button', { name: '+ Filter' }).click()
    const teamField = page.getByRole('button', { name: /^Team/ })
    await expect(teamField).toBeVisible()
    await teamField.click()
    // Value is the group key assigned by server rules (performance/api → backend).
    await expect(page.getByRole('button', { name: /backend/i }).first()).toBeVisible()
    // Dismiss filter menu (force: floating group header can intercept normal clicks).
    await page.locator('body').click({ position: { x: 8, y: 8 }, force: true })

    // Breakdown "Team" axis also surfaces the backend bucket.
    await page.getByRole('button', { name: /Breakdown/ }).click()
    await page.getByRole('button', { name: 'Team', exact: true }).click()
    await expect(page.getByText('backend', { exact: true }).first()).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
