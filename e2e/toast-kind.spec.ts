import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/**
 * GDK-158: success and error toasts must stay distinguishable without colour.
 *
 * Triggers reuse existing journeys:
 *  - success: Enter on navigator JQL (jql.spec.ts) → filter.jqlApplied
 *  - error: failed transition (write-through.spec.ts) → write.jiraUnavailable
 */

const KEY = 'NMB-110'
const TRANSITION = {
  id: '21',
  name: 'Start work',
  to_status: 'In Progress',
  to_category: 'indeterminate',
} as const

type IssueRow = Record<string, unknown> & { issue_key: string }

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

async function captureIssue(page: Page): Promise<IssueRow> {
  const held: { issue: IssueRow | null } = { issue: null }
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { issues: IssueRow[] }
    held.issue = body.issues.find((it) => it.issue_key === KEY) ?? null
    await route.fulfill({ response, json: body })
  })
  await page.route((url) => url.pathname.includes('/delta/'), async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { upserted?: IssueRow[] }
    body.upserted = (body.upserted ?? []).filter((it) => it.issue_key !== KEY)
    await route.fulfill({ response, json: body })
  })
  await gotoApp(page)
  expect(held.issue, 'fixture bootstrap must include NMB-110').toBeTruthy()
  return held.issue!
}

test.describe('toast kind without colour', () => {
  test('a success toast carries a check-circle glyph', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const box = page.getByTestId('search-input')
    await box.click()
    await box.fill('project = NMA AND statusCategory = "In Progress"')
    await box.press('Enter')

    const success = page.getByTestId('toast').and(page.getByRole('status'))
    await expect(success).toBeVisible()
    await expect(success).toContainText('JQL filter applied')
    const icon = success.getByTestId('toast-icon')
    await expect(icon).toBeVisible()
    await expect(icon.locator('svg')).toBeVisible()
    await expect(icon).toHaveAttribute('data-icon', 'check-circle')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('an error toast carries a warning glyph that is not the success glyph', async ({
    page,
  }) => {
    // The 502 that surfaces the toast is the same write.go failJira body as
    // write-through.spec.ts; Chromium logs it, and that spec does not treat
    // the log as a failure.
    await captureIssue(page)

    const input = searchInput(page)
    await input.fill(KEY)
    await page
      .locator('[data-testid="issue-list-scroller"] [role="button"]')
      .filter({ hasText: KEY })
      .first()
      .click()
    const panel = page.getByTestId('issue-detail-panel')
    await expect(panel).toBeVisible()

    const chip = panel.getByTestId('status-transition')
    await expect(chip).toBeVisible()

    await page.route(`**/api/v1/issues/${KEY}/transitions/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      await fulfillJSON(route, { transitions: [TRANSITION] })
    })

    let posted = false
    await page.route(`**/api/v1/issues/${KEY}/transition/`, async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = true
      await fulfillJSON(route, { error: 'jira_unavailable' }, 502)
    })

    await chip.click()
    await page.getByRole('option', { name: TRANSITION.name }).click()
    await expect.poll(() => posted).toBe(true)

    const errorToast = page.getByTestId('toast').and(page.getByRole('alert'))
    await expect(errorToast).toBeVisible()
    await expect(errorToast).toContainText('Could not reach Jira.')
    const icon = errorToast.getByTestId('toast-icon')
    await expect(icon).toBeVisible()
    await expect(icon.locator('svg')).toBeVisible()
    await expect(icon).toHaveAttribute('data-icon', 'warning')
  })
})
