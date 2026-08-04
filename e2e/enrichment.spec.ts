import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

test.describe('enrichment surface', () => {
  test('NMB-110 shows deploy badge and deploy timeline when features.deploy is on', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const input = searchInput(page)
    await input.fill('NMB-110')
    await expect(page.getByText('NMB-110').first()).toBeVisible()

    // List row deploy badge for state=qa (IssueRow shows "QA")
    const row = page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: 'NMB-110' })
      .first()
    await expect(row.getByRole('button', { name: 'QA' })).toBeVisible()

    await row.click()

    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()
    // detail.deploy section + DeployTimeline step labels (en.ts)
    await expect(panel.getByRole('heading', { name: 'Deploy status' })).toBeVisible()
    await expect(panel.getByText('qa swap · QA ready')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
