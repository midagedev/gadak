import { expect, test, type Page, type Route } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/**
 * GDK-217: palette instant-create.
 *
 *  - Non-empty query offers create-now; choosing it POSTs {summary} only.
 *  - The toast shows the new key plus the filled type and project.
 *  - Server 400 keeps the typed text and surfaces the reason.
 *
 * The fixture credential is fake, so the success path fulfils POST create/
 * the way write.go does after a healthy Jira + mirror refresh.
 */

const SUMMARY = 'zzgdk217create'
const CREATED_KEY = 'GDK-224'
const CREATED_TYPE = 'Task'
const CREATED_PROJECT = 'GDK'
const SERVER_400 =
  'pass --type, available: Task (id 10000); Bug (id 10001)'

type IssueRow = Record<string, unknown> & { issue_key: string }
type CreateBody = Record<string, unknown>

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

async function captureTemplate(page: Page): Promise<IssueRow> {
  const held: { issue: IssueRow | null } = { issue: null }
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { issues: IssueRow[] }
    held.issue = body.issues[0] ?? null
    await route.fulfill({ response, json: body })
  })
  await gotoApp(page)
  expect(held.issue, 'fixture bootstrap must include an issue').toBeTruthy()
  return held.issue!
}

async function openPaletteWithSummary(page: Page) {
  await page.keyboard.press('ControlOrMeta+k')
  const palette = page.getByRole('dialog', { name: 'Command palette' })
  await expect(palette).toBeVisible()
  await palette.getByRole('combobox').fill(SUMMARY)
  const createNow = palette.getByTestId('palette-create-now')
  await expect(createNow).toBeVisible()
  await expect(createNow).toContainText(SUMMARY)
  return { palette, createNow }
}

test.describe('palette instant create', () => {
  test('create-now POSTs only the typed summary and shows the filled key', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const template = await captureTemplate(page)

    let posted: CreateBody | null = null
    await page.route('**/api/v1/issues/create/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as CreateBody
      await fulfillJSON(route, {
        issue: {
          ...template,
          issue_key: CREATED_KEY,
          summary: SUMMARY,
          issue_type: CREATED_TYPE,
          source_project: CREATED_PROJECT,
        },
        resolved: {
          project: { value: CREATED_PROJECT, source: 'config' },
          issue_type: { value: '10000', source: 'sole' },
        },
      })
    })

    const { palette, createNow } = await openPaletteWithSummary(page)
    await expect(palette.getByRole('option', { name: /New issue/ })).toBeVisible()
    await createNow.click()

    await expect.poll(() => posted).not.toBeNull()
    expect(Object.keys(posted!).sort()).toEqual(['summary'])
    expect(posted!.summary).toBe(SUMMARY)

    const toast = page.getByTestId('toast').and(page.getByRole('status'))
    await expect(toast).toBeVisible()
    await expect(toast).toContainText(CREATED_KEY)
    await expect(toast).toContainText(CREATED_TYPE)
    await expect(toast).toContainText(CREATED_PROJECT)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('create 400 keeps the typed text and shows the server reason', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    let posted: CreateBody | null = null
    await page.route('**/api/v1/issues/create/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      posted = route.request().postDataJSON() as CreateBody
      await fulfillJSON(route, { error: SERVER_400 }, 400)
    })

    const { palette, createNow } = await openPaletteWithSummary(page)
    await createNow.click()

    await expect.poll(() => posted).not.toBeNull()
    expect(Object.keys(posted!).sort()).toEqual(['summary'])
    expect(posted!.summary).toBe(SUMMARY)

    const toast = page.getByTestId('toast').and(page.getByRole('alert'))
    await expect(toast).toBeVisible()
    await expect(toast).toContainText(SERVER_400)

    await expect(palette).toBeVisible()
    await expect(palette.getByRole('combobox')).toHaveValue(SUMMARY)
    await expect(palette.getByTestId('palette-create-now')).toBeVisible()

    expect(
      errors.filter((e) => !e.includes('400')),
      `console errors:\n${errors.join('\n')}`,
    ).toEqual([])
  })
})
