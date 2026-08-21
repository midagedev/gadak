import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/**
 * GDK-322: field editors are gated by editmeta, not feature('qa').
 * GDK-323 coverage for description ADF wrapping is the server test
 * TestDescriptionSetAndClear — this file does not repeat that PUT.
 */

const KEY = 'NMB-110'

const FIELD_SPECS = [
  { alias: 'severity', label: 'Severity', role: 'facet', kind: 'option' },
  { alias: 'tags', label: 'Tags', role: 'facet', kind: 'multi_option' },
]

const EDITMETA = {
  fields: {
    severity: {
      kind: 'option',
      editable: true,
      options: [
        { id: '10', value: 'Sev-1' },
        { id: '20', value: 'Sev-2' },
      ],
    },
    tags: {
      kind: 'multi_option',
      editable: true,
      options: [
        { id: 'a', value: 'alpha' },
        { id: 'b', value: 'beta' },
      ],
    },
    components: {
      kind: 'component_array',
      editable: true,
      options: [
        { id: '10000', value: 'Dashboard' },
        { id: '10001', value: 'API' },
      ],
    },
  },
}

type IssueRow = Record<string, unknown> & { issue_key: string }

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

async function ignoreFixtureDeltas(page: Page): Promise<void> {
  await page.route((url) => url.pathname.includes('/delta/'), async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { upserted?: IssueRow[]; field_specs?: unknown[] }
    body.upserted = (body.upserted ?? []).filter((it) => it.issue_key !== KEY)
    // Keep the bootstrap-injected specs; an empty discovery payload would
    // wipe the rows this file is asserting on.
    body.field_specs = FIELD_SPECS
    await route.fulfill({ response, json: body })
  })
}

async function stubEditMeta(page: Page): Promise<void> {
  await page.route(`**/api/v1/issues/${KEY}/editmeta/`, async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await fulfillJSON(route, EDITMETA)
  })
}

async function bootWithEditors(page: Page): Promise<IssueRow> {
  await stubEditMeta(page)
  const held: { issue: IssueRow | null } = { issue: null }
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as {
      issues: IssueRow[]
      field_specs?: unknown[]
    }
    body.field_specs = FIELD_SPECS
    const issue = body.issues.find((it) => it.issue_key === KEY)
    if (issue) {
      issue.severity = 'Sev-2'
      issue.tags = ['alpha']
    }
    held.issue = issue ?? null
    await route.fulfill({ response, json: body })
  })
  await ignoreFixtureDeltas(page)
  await gotoApp(page)
  expect(held.issue, 'fixture bootstrap must include NMB-110').toBeTruthy()
  return held.issue!
}

async function openIssue(page: Page) {
  const input = searchInput(page)
  await input.fill(KEY)
  await page
    .locator('[data-testid="issue-list-scroller"] [role="button"]')
    .filter({ hasText: KEY })
    .first()
    .click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

test.describe('edit-fields', () => {
  test('without the qa flag, an option row opens editmeta allowedValues', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await bootWithEditors(page)
    const panel = await openIssue(page)

    const row = panel.getByTestId('field-row-severity')
    await expect(row).toBeVisible()
    await expect(row).toHaveAttribute('data-kind', 'option')
    await expect(row).toHaveAttribute('data-editable', 'true')

    await row.getByTestId('field-editor-trigger').click()
    const menu = page.getByTestId('field-editor-menu')
    await expect(menu).toBeVisible()
    await expect(menu.getByRole('option', { name: 'Sev-1' })).toBeVisible()
    await expect(menu.getByRole('option', { name: 'Sev-2' })).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('duedate set then clear: row updates and PUT body is the date / null', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const { issue } = { issue: await bootWithEditors(page) }
    const panel = await openIssue(page)

    const row = panel.getByTestId('field-row-duedate')
    await expect(row).toBeVisible()
    await expect(row).toHaveAttribute('data-kind', 'date')
    await expect(row).toHaveAttribute('data-editable', 'true')

    let put: { duedate?: string | null } | null = null
    await page.route(`**/api/v1/issues/${KEY}/duedate/`, async (route) => {
      if (route.request().method() !== 'PUT') return route.continue()
      put = route.request().postDataJSON() as { duedate?: string | null }
      await fulfillJSON(route, {
        issue: { ...issue, duedate: put.duedate ?? null },
      })
    })

    await row.getByTestId('field-editor-trigger').click()
    const dateInput = page.getByTestId('field-editor-date')
    await expect(dateInput).toBeVisible()
    await dateInput.fill('2026-09-01')

    await expect.poll(() => put?.duedate).toBe('2026-09-01')
    await expect(row).toContainText('2026-09-01')
    await expect(page.getByTestId('toast').and(page.getByRole('status'))).toHaveCount(0)

    await row.getByTestId('field-editor-trigger').click()
    await page.getByTestId('field-editor-date-clear').click()
    await expect.poll(() => put?.duedate).toBeNull()
    await expect(row.getByText('None')).toBeVisible()
    await expect(page.getByTestId('toast').and(page.getByRole('status'))).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('components row is editable and Apply PATCHes selected ids', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const issue = await bootWithEditors(page)
    const panel = await openIssue(page)

    const row = panel.getByTestId('field-row-components')
    await expect(row).toBeVisible()
    await expect(row).toHaveAttribute('data-kind', 'component_array')
    await expect(row).toHaveAttribute('data-editable', 'true')

    let patch: { field?: string; value?: string[] } | null = null
    await page.route(`**/api/v1/issues/${KEY}/fields/`, async (route) => {
      if (route.request().method() !== 'PATCH') return route.continue()
      patch = route.request().postDataJSON() as { field?: string; value?: string[] }
      await fulfillJSON(route, {
        issue: { ...issue, components: ['Dashboard', 'API'] },
      })
    })

    await row.getByTestId('field-editor-trigger').click()
    const menu = page.getByTestId('field-editor-menu')
    await expect(menu).toBeVisible()
    await expect(menu.getByRole('button', { name: 'Dashboard' })).toBeVisible()
    await expect(menu.getByRole('button', { name: 'API' })).toBeVisible()

    await menu.getByRole('button', { name: 'API' }).click()
    await menu.getByRole('button', { name: 'Apply' }).click()

    await expect.poll(() => patch).not.toBeNull()
    expect(patch!.field).toBe('components')
    expect(patch!.value).toEqual(['10000', '10001'])
    await expect(row).toContainText('API')
    await expect(page.getByTestId('toast').and(page.getByRole('status'))).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
